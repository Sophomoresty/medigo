package dongao

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Dongao encrypted HLS flow ported from decompiled Dongao_Course.pyc:
//   _extract_player_fields: parse <input> fields from lecture HTML (w_vkey_n, wok, w_id, w_time)
//   _dongao_ts_key: AES-ECB decrypt ts_key_seed using w_vkey_n as key → TS segment key
//   _build_signed_m3u8: fetch signed m3u8, extract EXT-X-KEY IV, rewrite segment URLs with sg= param
//   _sign_dongao_media_url: append ccode + w_p signature to each .ts segment URL

var (
	inputFieldRe = regexp.MustCompile(`(?is)<input\b([^>]*)>`)
	attrNameRe   = regexp.MustCompile(`(?is)\bname\s*=\s*["']([^"']+)["']`)
	attrIDRe     = regexp.MustCompile(`(?is)\bid\s*=\s*["']([^"']+)["']`)
	attrValRe    = regexp.MustCompile(`(?is)\bvalue\s*=\s*["']([^"']*)["']`)
	ivRe         = regexp.MustCompile(`(?is)IV\s*=\s*(0x[0-9a-fA-F]+)`)
	keyURIRe     = regexp.MustCompile(`(?is)URI\s*=\s*["']([^"']+)["']`)
)

const (
	dongaoTSKeySeed  = "h8k9npx&$yuYR0W1"
	dongaoCDNSignKey = "L31VSA2VFNGi6E68QNbIVsZ"
)

// extractPlayerFields parses the hidden <input> fields from the lecture HTML page.
// Returns a map of field name → value. Key fields: w_vkey_n, wok, w_id, w_time.
func extractPlayerFields(lectureHTML string) map[string]string {
	fields := map[string]string{}
	for _, m := range inputFieldRe.FindAllStringSubmatch(lectureHTML, -1) {
		attrs := m[1]
		nameMatch := attrNameRe.FindStringSubmatch(attrs)
		if nameMatch == nil {
			nameMatch = attrIDRe.FindStringSubmatch(attrs)
		}
		if nameMatch == nil {
			continue
		}
		name := strings.TrimSpace(nameMatch[1])
		val := ""
		if valMatch := attrValRe.FindStringSubmatch(attrs); valMatch != nil {
			val = valMatch[1]
		}
		if name != "" {
			fields[name] = val
		}
	}
	if rt := strings.TrimSpace(fields["rt"]); rt != "" {
		for _, part := range strings.Split(rt, "&") {
			if kv := strings.SplitN(part, "=", 2); len(kv) == 2 {
				fields["rt:"+strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
	}
	return fields
}

func completePlayerFields(fields map[string]string, headers map[string]string) map[string]string {
	if fields == nil {
		fields = map[string]string{}
	}
	if strings.TrimSpace(fields["w_id"]) == "" {
		fields["w_id"] = memberIDFromCookieHeader(headers["Cookie"])
	}
	if strings.TrimSpace(fields["w_time"]) == "" {
		fields["w_time"] = strconv.FormatInt(time.Now().UnixMilli(), 10)
	}
	return fields
}

func memberIDFromCookieHeader(cookieHeader string) string {
	cookies := cookieMapFromHeader(cookieHeader)
	if raw := cookies["memberinfo"]; raw != "" {
		if decoded, err := url.QueryUnescape(raw); err == nil {
			raw = decoded
		}
		var m map[string]any
		if json.Unmarshal([]byte(raw), &m) == nil {
			return firstNonEmpty(valueString(m, "uid", "memberId", "member_id", "id"))
		}
	}
	if raw := cookies["dongaoLogin"]; raw != "" {
		raw += strings.Repeat("=", (4-len(raw)%4)%4)
		if data, err := base64.StdEncoding.DecodeString(raw); err == nil {
			var m map[string]any
			if json.Unmarshal(data, &m) == nil {
				return firstNonEmpty(valueString(m, "memberId", "uid", "id"))
			}
		}
	}
	return ""
}

// dongaoTSKey decrypts the TS segment key. Source _dongao_ts_key:
// takes the w_vkey_n field as hex, AES-ECB decrypts ts_key_seed → raw key bytes.
func dongaoTSKey(fields map[string]string, tsKeySeed []byte) ([]byte, error) {
	vkeyHex := strings.TrimSpace(fields["w_vkey_n"])
	if vkeyHex == "" {
		return nil, fmt.Errorf("dongao: missing w_vkey_n field")
	}
	vkey, err := hex.DecodeString(vkeyHex)
	if err != nil {
		// Try as raw string (some variants use ASCII key)
		vkey = []byte(vkeyHex)
	}
	if len(vkey) != 16 && len(vkey) != 24 && len(vkey) != 32 {
		// Pad/truncate to nearest AES block size
		if len(vkey) > 32 {
			vkey = vkey[:32]
		} else if len(vkey) > 24 {
			vkey = padKey(vkey, 32)
		} else if len(vkey) > 16 {
			vkey = padKey(vkey, 24)
		} else {
			vkey = padKey(vkey, 16)
		}
	}

	block, err := aes.NewCipher(vkey)
	if err != nil {
		return nil, fmt.Errorf("dongao AES key: %w", err)
	}
	if len(tsKeySeed)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("dongao: ts_key_seed not block-aligned (%d bytes)", len(tsKeySeed))
	}
	plaintext := make([]byte, len(tsKeySeed))
	for i := 0; i < len(tsKeySeed); i += aes.BlockSize {
		block.Decrypt(plaintext[i:i+aes.BlockSize], tsKeySeed[i:i+aes.BlockSize])
	}
	return unpadPKCS7(plaintext)
}

func padKey(key []byte, size int) []byte {
	if len(key) >= size {
		return key[:size]
	}
	padded := make([]byte, size)
	copy(padded, key)
	return padded
}

func unpadPKCS7(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}
	pad := int(data[len(data)-1])
	if pad == 0 || pad > len(data) || pad > aes.BlockSize {
		return data, nil // no padding
	}
	for _, b := range data[len(data)-pad:] {
		if int(b) != pad {
			return data, nil // invalid padding, keep original plaintext
		}
	}
	return data[:len(data)-pad], nil
}

// signDongaoMediaURL appends Dongao's player signature params to a segment URL.
// Source _sign_dongao_media_url: builds w_p from wok/w_id/w_time and cdn_sign_key,
// then carries the player fields required by the signed CDN request.
func signDongaoMediaURL(rawURL string, fields map[string]string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	baseSG := u.Query().Get("sg")
	sg := buildDongaoWP(u.Path, fields)
	if strings.HasSuffix(strings.ToLower(u.Path), ".ts") && sg != "" {
		q := u.Query()
		if baseSG != "" {
			q.Set("sg", baseSG+"&"+sg)
		} else {
			q.Set("sg", sg)
		}
		u.RawQuery = q.Encode()
		return u.String()
	}
	q := u.Query()
	if sg != "" {
		for _, part := range strings.Split(sg, "&") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 && strings.TrimSpace(kv[0]) != "" {
				q.Set(kv[0], kv[1])
			}
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func buildDongaoWP(mediaPath string, fields map[string]string) string {
	uid := strings.TrimSpace(fields["w_id"])
	wtime := strings.TrimSpace(fields["w_time"])
	if wtime == "" {
		wtime = strconv.FormatInt(time.Now().UnixMilli(), 10)
	}
	sec := dongaoSeconds(wtime)
	token := firstNonEmpty(strings.TrimSpace(fields["w_auto_p"]), strings.TrimSpace(fields["wok"]), randomDongaoToken(9))
	if mediaPath == "" || uid == "" {
		return ""
	}
	signSource := fmt.Sprintf("%s-%s-%s-0-%s%s", mediaPath, uid, sec, token, dongaoCDNSignKey)
	sign := fmt.Sprintf("%x", md5.Sum([]byte(signSource)))
	now := time.Now()
	ccode := fmt.Sprintf("%d-%08d-%d-%s", now.Unix(), now.Nanosecond()%100000000, now.Nanosecond()%10, token)
	si := now.Nanosecond() % 1000
	return fmt.Sprintf("ccode=%s&expire=18000&vkey=%s&u_type=mp4%s&si=%03d&s=%s-%s-0-%s&time=%s&psource=player",
		url.QueryEscape(ccode), url.QueryEscape(token), dongaoShortToken(token), si, sec, url.QueryEscape(uid), sign, url.QueryEscape(wtime))
}

func dongaoSeconds(wtime string) string {
	n, err := strconv.ParseInt(wtime, 10, 64)
	if err != nil || n <= 0 {
		return strconv.FormatInt(time.Now().Unix(), 10)
	}
	if n > 9999999999 {
		n /= 1000
	}
	return strconv.FormatInt(n, 10)
}

func dongaoShortToken(token string) string {
	if len(token) <= 6 {
		return token
	}
	return token[:6]
}

func randomDongaoToken(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	now := time.Now().UnixNano()
	for i := range b {
		b[i] = alphabet[(int(now)+i*17)%len(alphabet)]
	}
	return string(b)
}

// buildSignedM3U8 fetches the signed m3u8, extracts IV from EXT-X-KEY,
// rewrites each segment URL with the sg= signature, and returns the
// rewritten manifest text ready for a downstream HLS downloader.
func buildSignedM3U8(m3u8Text string, fields map[string]string, baseURL string) (string, []byte, error) {
	if !strings.HasPrefix(strings.TrimSpace(m3u8Text), "#EXTM3U") {
		return "", nil, fmt.Errorf("dongao: not an m3u8 manifest")
	}

	var iv []byte
	var tsKey []byte
	var outLines []string

	for _, line := range strings.Split(m3u8Text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			outLines = append(outLines, line)
			continue
		}

		if strings.HasPrefix(line, "#EXT-X-KEY") {
			// Extract IV
			if m := ivRe.FindStringSubmatch(line); m != nil {
				ivHex := strings.TrimPrefix(m[1], "0x")
				if parsed, err := hex.DecodeString(ivHex); err == nil {
					iv = parsed
				}
			}
			rewrittenKeyLine := line
			// Extract key URI and decrypt. Python source uses the fixed
			// ts_key_seed with w_vkey_n; some manifests also expose a hex seed.
			keySeed := []byte(dongaoTSKeySeed)
			if m := keyURIRe.FindStringSubmatch(line); m != nil {
				if parsed, err := hex.DecodeString(strings.TrimSpace(m[1])); err == nil && len(parsed) > 0 {
					keySeed = parsed
				}
			}
			if k, err := dongaoTSKey(fields, keySeed); err == nil && len(k) > 0 {
				tsKey = k
				rewrittenKeyLine = replaceKeyURI(line, "data:application/octet-stream;base64,"+base64.StdEncoding.EncodeToString(k))
			}
			outLines = append(outLines, rewrittenKeyLine)
			continue
		}

		// Rewrite segment URLs
		if !strings.HasPrefix(line, "#") && line != "" {
			signed := signDongaoMediaURL(resolveM3U8LineURL(line, baseURL), fields)
			outLines = append(outLines, signed)
			continue
		}

		outLines = append(outLines, line)
	}

	result := strings.Join(outLines, "\n")
	return result, combineKeyAndIV(tsKey, iv), nil
}

func replaceKeyURI(line, uri string) string {
	if keyURIRe.MatchString(line) {
		return keyURIRe.ReplaceAllString(line, `URI="`+uri+`"`)
	}
	return line + `,URI="` + uri + `"`
}

func resolveM3U8LineURL(raw, baseURL string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "data:") {
		return raw
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	if baseURL == "" {
		return raw
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return base.ResolveReference(ref).String()
}

func m3u8DataURL(manifest string) string {
	return "data:application/vnd.apple.mpegurl;base64," + base64.StdEncoding.EncodeToString([]byte(manifest))
}

// combineKeyAndIV returns the AES key material for downstream decryption.
// If both key and IV are available, returns key; caller applies IV from m3u8.
func combineKeyAndIV(key, iv []byte) []byte {
	if len(key) > 0 {
		return key
	}
	return nil
}

// decryptCBCSegment decrypts one TS segment using AES-CBC with the given key and IV.
// Used by downstream HLS downloader when m3u8 has EXT-X-KEY:METHOD=AES-128.
func decryptCBCSegment(ciphertext, key, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(iv) != block.BlockSize() {
		return nil, fmt.Errorf("invalid AES-CBC IV length %d", len(iv))
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext not block-aligned")
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)
	return unpadPKCS7(plaintext)
}
