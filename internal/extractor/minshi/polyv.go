package minshi

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
	"strings"

	"github.com/Sophomoresty/mediago/internal/util"
)

// getPolyvM3U8 mirrors Minshi_Base.get_polyv_m3u8:
// secure/{formatted_vid}.json -> decrypt body when needed -> pick hls ->
// fetch manifest -> tokenise/decrypt the playsafe key -> inline key URI.
func getPolyvM3U8(c *util.Client, videoID, playSafeToken string, headers map[string]string) (manifest string, sourceURL string, err error) {
	formattedVID := formatPolyvVID(videoID)
	if formattedVID == "" {
		return "", "", fmt.Errorf("minshi polyv: empty video id")
	}
	secBody, err := c.GetString(fmt.Sprintf(polyv_secure_url, url.PathEscape(formattedVID)), polyvHeaders(headers))
	if err != nil {
		return "", "", err
	}
	var secure map[string]any
	if err := json.Unmarshal([]byte(secBody), &secure); err != nil {
		return "", "", err
	}
	info := decryptPolyvSecureInfo(videoID, secure)
	hlsList := polyvHLSList(info)
	m3u8URL := pickPolyvHLSURL(hlsList)
	if m3u8URL == "" {
		return "", "", fmt.Errorf("minshi polyv: no hls urls")
	}
	m3u8Text, err := c.GetString(m3u8URL, polyvHeaders(headers))
	if err != nil {
		return "", "", err
	}
	m3u8Text = absolutizeM3U8URLs(m3u8Text, m3u8URL)
	keyURL := buildPolyvTokenKeyURL(m3u8Text, m3u8URL, videoID, playSafeToken)
	keyReplacement := keyURL
	if keyURL != "" {
		if keyBytes, keyErr := c.GetBytes(keyURL, polyvHeaders(headers)); keyErr == nil {
			if decrypted := decryptPolyvKey(keyBytes, fmt.Sprint(info["seed_const"])); len(decrypted) == 16 {
				keyReplacement = hex.EncodeToString(decrypted)
			}
		}
	}
	if keyReplacement != "" && strings.Contains(m3u8Text, `URI="`) {
		m3u8Text = regexp.MustCompile(`URI=".*?"`).ReplaceAllString(m3u8Text, `URI="`+keyReplacement+`"`)
	}
	return m3u8Text, m3u8URL, nil
}

func polyvHeaders(base map[string]string) map[string]string {
	return map[string]string{
		"Accept":     "application/json, text/plain, */*",
		"Origin":     origin,
		"Referer":    first(base["Referer"], referer),
		"User-Agent": first(base["User-Agent"], "Mozilla/5.0"),
	}
}

func formatPolyvVID(videoID string) string {
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return ""
	}
	if strings.Contains(videoID, "_") {
		return strings.Split(videoID, "_")[0] + "_" + videoID[:1]
	}
	return videoID + "_" + videoID[:1]
}

func decryptPolyvSecureInfo(videoID string, info map[string]any) map[string]any {
	if info == nil {
		return map[string]any{}
	}
	if _, ok := info["hls"]; ok {
		if _, okSeed := info["seed_const"]; okSeed {
			return info
		}
	}
	if data, ok := info["data"].(map[string]any); ok {
		if _, okHLS := data["hls"]; okHLS {
			if _, okSeed := data["seed_const"]; okSeed {
				return data
			}
		}
		if _, okPaths := data["paths"]; okPaths {
			return data
		}
	}
	bodyHex := findFirst(info, "body")
	formattedVID := formatPolyvVID(videoID)
	if bodyHex == "" || formattedVID == "" {
		return map[string]any{}
	}
	ciphertext, err := hex.DecodeString(bodyHex)
	if err != nil || len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return map[string]any{}
	}
	sum := md5.Sum([]byte(formattedVID))
	keyHex := hex.EncodeToString(sum[:])
	block, err := aes.NewCipher([]byte(keyHex[:16]))
	if err != nil {
		return map[string]any{}
	}
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, []byte(keyHex[16:32])).CryptBlocks(plain, ciphertext)
	for _, candidate := range [][]byte{plain, []byte(strings.TrimRight(string(plain), "\x00"))} {
		if m := decodePolyvSecurePayload(candidate); len(m) > 0 {
			return m
		}
	}
	return map[string]any{}
}

func decodePolyvSecurePayload(raw []byte) map[string]any {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil
	}
	var decoded []byte
	if json.Valid(raw) {
		decoded = raw
	} else {
		b, err := base64.StdEncoding.DecodeString(string(raw))
		if err != nil {
			return nil
		}
		decoded = b
	}
	var out map[string]any
	if err := json.Unmarshal(decoded, &out); err != nil {
		return nil
	}
	return out
}

func polyvHLSList(info map[string]any) []string {
	var out []string
	if list, ok := info["hls"].([]any); ok {
		for _, item := range list {
			switch v := item.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					out = append(out, strings.TrimSpace(v))
				}
			case map[string]any:
				if u := firstTextMap(v, "url", "m3u8", "hls"); u != "" {
					out = append(out, u)
				}
			}
		}
	}
	if list, ok := info["paths"].([]any); ok {
		for _, item := range list {
			switch v := item.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					out = append(out, strings.TrimSpace(v))
				}
			case map[string]any:
				if u := firstTextMap(v, "url", "m3u8", "hls", "path"); u != "" {
					out = append(out, u)
				}
			}
		}
	}
	return out
}

func pickPolyvHLSURL(hlsList []string) string {
	if len(hlsList) == 0 {
		return ""
	}
	// Source ranks by mode: SD -> first, HD -> second-highest, FHD -> highest.
	return hlsList[len(hlsList)-1]
}

func absolutizeM3U8URLs(m3u8Text, m3u8URL string) string {
	uriRe := regexp.MustCompile(`URI="([^"]+)"`)
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(m3u8Text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if (strings.HasPrefix(trimmed, "#EXT-X-KEY") || strings.HasPrefix(trimmed, "#EXT-X-MAP")) && strings.Contains(line, `URI="`) {
			line = uriRe.ReplaceAllStringFunc(line, func(match string) string {
				parts := uriRe.FindStringSubmatch(match)
				if len(parts) < 2 {
					return match
				}
				return `URI="` + joinURL(m3u8URL, parts[1]) + `"`
			})
		} else if trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(strings.ToLower(trimmed), "http") {
			line = joinURL(m3u8URL, trimmed)
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func buildPolyvTokenKeyURL(m3u8Text, m3u8URL, videoID, playSafeToken string) string {
	if playSafeToken == "" {
		return ""
	}
	keyURL := ""
	if m := regexp.MustCompile(`URI="([^"]+)"`).FindStringSubmatch(m3u8Text); len(m) > 1 {
		keyURL = joinURL(m3u8URL, m[1])
	}
	if keyURL == "" {
		rawVID := strings.Split(strings.TrimSpace(videoID), "_")[0]
		bitrate := ""
		if m := regexp.MustCompile(`_(\d+)\.m3u8(?:\?|$)`).FindStringSubmatch(m3u8URL); len(m) > 1 {
			bitrate = m[1]
		}
		if rawVID != "" && bitrate != "" {
			path1 := rawVID
			if len(path1) > 10 {
				path1 = path1[:10]
			}
			path2 := rawVID[len(rawVID)-1:]
			keyURL = strings.NewReplacer(
				"{path1}", path1,
				"{path2}", path2,
				"{vid}", rawVID,
				"{bitrate}", bitrate,
				"{token}", url.QueryEscape(playSafeToken),
			).Replace(polyv_key_url)
		}
	}
	if keyURL == "" {
		return ""
	}
	u, err := url.Parse(keyURL)
	if err != nil {
		return keyURL
	}
	if !strings.Contains(u.Path, "/playsafe/") {
		if strings.HasPrefix(u.Path, "/") {
			u.Path = "/playsafe" + u.Path
		} else {
			u.Path = "playsafe/" + u.Path
		}
	}
	q := u.Query()
	q.Set("token", playSafeToken)
	u.RawQuery = q.Encode()
	return u.String()
}

func decryptPolyvKey(keyBytes []byte, seedConst string) []byte {
	if len(keyBytes) != 32 {
		return nil
	}
	if dec := tryDecryptPolyvKey(keyBytes, seedConst); len(dec) == 16 {
		return dec
	}
	for i := 0; i < 1000; i++ {
		seed := fmt.Sprint(i)
		if seed == seedConst {
			continue
		}
		if dec := tryDecryptPolyvKey(keyBytes, seed); len(dec) == 16 {
			return dec
		}
	}
	return nil
}

func tryDecryptPolyvKey(keyBytes []byte, seed string) []byte {
	sum := md5.Sum([]byte(seed))
	key := []byte(hex.EncodeToString(sum[:])[:16])
	iv, err := hex.DecodeString(polyvIVHex)
	if err != nil {
		return nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil
	}
	plain := make([]byte, len(keyBytes))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, keyBytes)
	plain = stripPolyvPadding(plain)
	return plain
}

func stripPolyvPadding(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	pad := int(data[len(data)-1])
	if pad > 0 && pad <= aes.BlockSize && pad <= len(data) {
		ok := true
		for _, b := range data[len(data)-pad:] {
			if int(b) != pad {
				ok = false
				break
			}
		}
		if ok {
			return data[:len(data)-pad]
		}
	}
	return []byte(strings.TrimRight(string(data), "\x00"))
}

func joinURL(base, ref string) string {
	b, errB := url.Parse(base)
	r, errR := url.Parse(ref)
	if errB != nil || errR != nil || b == nil || r == nil {
		return ref
	}
	return b.ResolveReference(r).String()
}
