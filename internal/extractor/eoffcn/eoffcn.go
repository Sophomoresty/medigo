// Package eoffcn implements the xue.eoffcn.com course / package extractor.
package eoffcn

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

const (
	// AES key/IV used to decrypt the RSA public key returned by pub_key_url.
	// Source: Eoffcn_Course._decrypt_video_key / _request_watch_demand_data
	// both use AESEncrypt("wwwoffcncloudcom", "wwwoffcncloudcom").aes_decrypt(pub_key).
	pubKeyAESKey = "wwwoffcncloudcom"
	pubKeyAESIV  = "wwwoffcncloudcom"

	// Prefix prepended to the random AES key before RSA encryption.
	// Source: rsa_encrypt(decrypted_pub_key, "offcn|||" + random_key)
	encryptPrefix = "offcn|||"
)

const (
	order_url        = "https://xue.eoffcn.com/api/order/complete"
	new_order_url    = "https://xue.eoffcn.com/api/new/goods/list"
	package_url      = "https://xue.eoffcn.com/api/package/list?system_order=%s&coding=%s"
	catagory_url     = "https://xue.eoffcn.com/api/lesson/catagory?package_id=%s&system_order=%s"
	course_list_url  = "https://xue.eoffcn.com/api/new/course/list?system_order=%s"
	lesson_url       = "https://xue.eoffcn.com/api/lesson/detail?lesson_id=%s&package_id=%s&module_type=%s&system_order=%s"
	check_member_url = "https://xue.eoffcn.com/api/check/member"
	pub_key_url      = "https://api-live.offcncloud.com/api/v1/public_key"
	encrypt_url      = "https://api-live.offcncloud.com/api/user/watch_demand"

	// Static AES-CBC key/IV for decrypting live_url / video_url in lesson detail responses.
	// Source: Eoffcn_Course._get_m3u8_info (line 330) / _get_file (line 697).
	aesKey = "1234567898882222"
	aesIV  = "8NONwyJtHesysWpM"

	// Whiteboard playback is rendered with the same wboffcn.js runtime used by
	// pcvod.offcncloud.com. The extractor fetches the pcvod board payload when
	// available, normalizes it into a manifest, and exposes an HTML data stream
	// that can render the board in a browser/downloader.
	eoffcnBoardReferer  = "https://pcvod.offcncloud.com/"
	eoffcnWbOffcnJSURL  = "https://vod-live.offcncloud.com/base/wb1004/wboffcn.js"
	eoffcnWbOffcnMemURL = "https://vod-live.offcncloud.com/base/wb1004/wboffcn.js.mem"
)

var patterns = []string{`(?:[\w-]+\.)?(?:eoffcn|offcncloud)\.com/`}

func init() {
	extractor.Register(&Eoffcn{}, extractor.SiteInfo{Name: "Eoffcn", URL: "eoffcn.com", NeedAuth: true})
}

type Eoffcn struct{}

func (e *Eoffcn) Patterns() []string { return patterns }

var (
	courseIDRe  = regexp.MustCompile(`(?i)(?:system_order|systemSn|order(?:_num)?|course[_-]?cid)=([A-Za-z0-9_-]+)|/(?:course|goods|package)/(\w+)`)
	lessonIDRe  = regexp.MustCompile(`(?i)(?:lesson_id|lessonId|lid)=([A-Za-z0-9_-]+)|/(?:lesson|detail)/(\w+)`)
	packageIDRe = regexp.MustCompile(`(?i)(?:package_id|packageId|cid)=([A-Za-z0-9_-]+)`)
	codingRe    = regexp.MustCompile(`(?i)(?:coding|code)=([A-Za-z0-9_-]+)`)
	spuIDRe     = regexp.MustCompile(`(?i)(?:spu_id|spuId|spuID|spu)=([A-Za-z0-9_-]+)|"spuId"\s*:\s*"?([A-Za-z0-9_-]+)"?`)
)

type eoffcnParams struct {
	SystemOrder string
	PackageID   string
	LessonID    string
	ModuleType  string
	Coding      string
	SpuID       string
	Title       string
	PayMoney    string
}

type eoffcnOrder struct {
	SystemOrder string
	SpuID       string
	Title       string
	PayMoney    string
}

type lessonNode struct {
	ID         string
	Title      string
	PackageID  string
	ModuleType string
	RoomID     string
	FileID     string
}

func (e *Eoffcn) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("eoffcn requires login cookies")
	}
	params := parseParams(rawURL)

	c := util.NewClient()
	c.SetCookieJar(opts.Cookies)
	headers := map[string]string{
		"Accept":  "application/json, text/plain, */*",
		"Referer": "https://www.eoffcn.com",
		"Origin":  "https://www.eoffcn.com",
	}

	// Source: Eoffcn_Base._check_cookie validates the session cookie against
	// https://xue.eoffcn.com/api/check/member and checks for "code":0 in the response.
	if body, err := c.GetString(check_member_url, headers); err == nil {
		if !strings.Contains(body, `"code":0`) && !strings.Contains(body, `"code": 0`) {
			return nil, fmt.Errorf("eoffcn: cookie validation failed (check/member did not return code 0)")
		}
	}

	if params.SpuID == "" {
		params.SpuID = fetchSpuIDFromPage(c, rawURL, headers)
	}
	if params.SystemOrder == "" && params.SpuID != "" {
		selected, err := selectOrder(c, headers, params)
		if err != nil {
			return nil, err
		}
		params = mergeOrderParams(params, selected)
	} else if params.SystemOrder != "" {
		if selected, err := selectOrder(c, headers, params); err == nil {
			params = mergeOrderParams(params, selected)
		}
	}

	if params.SystemOrder == "" && params.PackageID == "" && params.LessonID == "" && params.Coding == "" {
		return nil, fmt.Errorf("cannot parse eoffcn package/lesson id from URL: %s", rawURL)
	}

	if params.LessonID != "" {
		entry, err := resolveLesson(c, headers, params, "eoffcn_"+params.LessonID)
		if err != nil {
			return nil, err
		}
		return entry, nil
	}

	entries, title, err := resolveCourse(c, headers, params)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("eoffcn: no playable lessons found in parsed API responses")
	}
	return &extractor.MediaInfo{Site: "eoffcn", Title: util.SanitizeFilename(firstNonEmpty(title, "eoffcn_"+params.SystemOrder)), Entries: entries}, nil
}

func resolveCourse(c *util.Client, headers map[string]string, p eoffcnParams) ([]*extractor.MediaInfo, string, error) {
	title := p.Title
	var nodes []lessonNode

	if p.SystemOrder != "" {
		if body, err := c.GetString(fmt.Sprintf(course_list_url, url.QueryEscape(p.SystemOrder)), headers); err == nil {
			var payload any
			if json.Unmarshal([]byte(body), &payload) == nil {
				nodes = append(nodes, collectLessonNodes(payload, p)...)
				title = firstNonEmpty(title, pickTitle(payload))
			}
		}
	}

	if p.PackageID != "" {
		if body, err := c.GetString(fmt.Sprintf(catagory_url, url.QueryEscape(p.PackageID), url.QueryEscape(p.SystemOrder)), headers); err == nil {
			var payload any
			if json.Unmarshal([]byte(body), &payload) == nil {
				nodes = append(nodes, collectLessonNodes(payload, p)...)
				title = firstNonEmpty(title, pickTitle(payload))
			}
		}
	}

	if p.SystemOrder != "" && p.Coding != "" {
		body, err := c.GetString(fmt.Sprintf(package_url, url.QueryEscape(p.SystemOrder), url.QueryEscape(p.Coding)), headers)
		if err != nil {
			return nil, title, fmt.Errorf("fetch eoffcn package list: %w", err)
		}
		var payload any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			return nil, title, fmt.Errorf("parse eoffcn package list: %w", err)
		}
		nodes = append(nodes, collectLessonNodes(payload, p)...)
		title = firstNonEmpty(title, pickTitle(payload))
	}

	if len(nodes) == 0 && p.SystemOrder != "" {
		body, err := c.GetString(new_order_url, headers)
		if err != nil {
			return nil, title, fmt.Errorf("fetch eoffcn goods list: %w", err)
		}
		var payload any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			return nil, title, fmt.Errorf("parse eoffcn goods list: %w", err)
		}
		nodes = append(nodes, collectLessonNodes(payload, p)...)
		title = firstNonEmpty(title, pickTitle(payload))
	}

	seen := map[string]bool{}
	entries := make([]*extractor.MediaInfo, 0, len(nodes))
	for _, n := range nodes {
		if n.ID == "" || seen[n.ID] {
			continue
		}
		seen[n.ID] = true
		pp := p
		pp.LessonID = n.ID
		pp.PackageID = firstNonEmpty(n.PackageID, pp.PackageID)
		pp.ModuleType = firstNonEmpty(n.ModuleType, pp.ModuleType, "0")
		entry, err := resolveLesson(c, headers, pp, n.Title)
		if err == nil {
			entries = append(entries, entry)
		}
	}
	return entries, title, nil
}

func resolveLesson(c *util.Client, headers map[string]string, p eoffcnParams, fallbackTitle string) (*extractor.MediaInfo, error) {
	if p.ModuleType == "" {
		p.ModuleType = "0"
	}
	api := fmt.Sprintf(lesson_url, url.QueryEscape(p.LessonID), url.QueryEscape(p.PackageID), url.QueryEscape(p.ModuleType), url.QueryEscape(p.SystemOrder))
	body, err := c.GetString(api, headers)
	if err != nil {
		return nil, fmt.Errorf("fetch eoffcn lesson detail: %w", err)
	}
	var payload any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil, fmt.Errorf("parse eoffcn lesson detail: %w", err)
	}
	playback := resolveEoffcnPlayback(c, headers, payload)
	if playback.URL == "" {
		return nil, fmt.Errorf("eoffcn: no live_url/video_url in lesson %s (RSA watch_demand attempted but returned no media)", p.LessonID)
	}
	title := util.SanitizeFilename(firstNonEmpty(pickTitle(payload), fallbackTitle, "eoffcn_"+p.LessonID))
	playback = hydrateEoffcnWhiteboardPlayback(c, headers, title, playback)
	return mediaInfoWithExtra(title, playback.URL, eoffcnStreamHeaders(headers, playback.URL, playback.Extra), playback.Extra), nil
}

type watchIDs struct{ VideoID, RoomID, Account, K, LiveURL string }

type eoffcnPlayback struct {
	URL        string
	Extra      map[string]any
	Whiteboard eoffcnWhiteboardInfo
}

type eoffcnWhiteboardInfo struct {
	Whiteboard bool
	APIURL     string
	Params     map[string]string
	Source     string
}

func resolveEoffcnPlayback(c *util.Client, headers map[string]string, payload any) eoffcnPlayback {
	whiteboard := detectEoffcnWhiteboard(payload)

	// findMediaURL tries both plain URLs and AES-CBC decryption of live_url/video_url
	// using the static key from source (Eoffcn_Course._get_m3u8_info line 330).
	playback := eoffcnPlayback{URL: findMediaURL(payload), Whiteboard: whiteboard}
	ids := collectWatchIDs(payload)
	if playback.URL == "" || (whiteboard.Whiteboard && whiteboard.APIURL == "" && (ids.LiveURL != "" || ids.VideoID != "" || ids.RoomID != "")) {
		// Source: Eoffcn_Course._get_m3u8_info for video_type 6 first extracts
		// k= and account= from live_url, then calls _decrypt_video_key(k, account)
		// which uses the RSA flow. _request_watch_demand_data does the same for
		// watch_demand content types with a live_url containing query params.
		if ids.LiveURL != "" || ids.VideoID != "" || ids.RoomID != "" {
			watched := requestWatchDemandPlayback(c, headers, ids)
			whiteboard = mergeEoffcnWhiteboardInfo(whiteboard, watched.Whiteboard)
			if playback.URL == "" && watched.URL != "" {
				playback.URL = watched.URL
			}
			playback.Extra = mergeEoffcnExtra(playback.Extra, watched.Extra)
		}
	}

	playback.Whiteboard = whiteboard
	if playback.URL == "" && whiteboard.Whiteboard && whiteboard.APIURL != "" {
		playback.URL = whiteboard.APIURL
	}
	playback.Extra = mergeEoffcnExtra(playback.Extra, eoffcnWhiteboardExtra(whiteboard))
	return playback
}

// requestWatchDemand keeps the original string-returning helper contract.
func requestWatchDemand(c *util.Client, headers map[string]string, ids watchIDs) string {
	return requestWatchDemandPlayback(c, headers, ids).URL
}

// requestWatchDemandPlayback implements the full RSA-encrypted watch_demand flow.
//
// Source: Eoffcn_Course._request_watch_demand_data (line 496) and
// _decrypt_video_key (line 292). The flow is:
//  1. GET pub_key_url -> JSON data field is the RSA public key, AES-encrypted
//     with static key/IV "wwwoffcncloudcom".
//  2. AES-decrypt to get the raw RSA public key body (base64 PEM body without
//     headers).
//  3. Generate a 16-char random alphanumeric key.
//  4. RSA-encrypt "offcn|||" + randomKey with the public key (PKCS1v15).
//  5. POST to encrypt_url with {account, k, encry_key: <rsa_encrypted>}.
//  6. Server returns AES-encrypted JSON; decrypt with key=randomKey, iv=randomKey.
//  7. Parse the decrypted JSON for media URLs and whiteboard metadata.
func requestWatchDemandPlayback(c *util.Client, headers map[string]string, ids watchIDs) eoffcnPlayback {
	if c == nil {
		return eoffcnPlayback{}
	}
	// Step 1: Fetch the encrypted public key.
	pubKeyEncrypted := getPubKey(c, headers)
	if pubKeyEncrypted == "" {
		return eoffcnPlayback{}
	}

	// Step 2: AES-decrypt the public key.
	decryptedPubKey := aesDecryptWithStatic(pubKeyEncrypted, pubKeyAESKey, pubKeyAESIV)
	if decryptedPubKey == "" {
		return eoffcnPlayback{}
	}

	// Step 3: Generate 16-char random key.
	randomKey := util.RandomAlphanumeric(16)

	// Step 4: RSA-encrypt "offcn|||" + randomKey.
	// The Python source wraps the key body in "-----BEGIN RSA PUBLIC KEY-----\n"
	// + body + "\n-----END RSA PUBLIC KEY-----\n".
	pemKey := "-----BEGIN RSA PUBLIC KEY-----\n" + decryptedPubKey + "\n-----END RSA PUBLIC KEY-----\n"
	encrypted, err := util.RSAEncryptPKCS1([]byte(encryptPrefix+randomKey), pemKey)
	if err != nil {
		return eoffcnPlayback{}
	}

	// Step 5: Build POST data and send.
	// For _request_watch_demand_data: {account: <account>, k: <k>, encry_key: <encrypted>}
	// For _decrypt_video_key: same structure with str_k and account.
	account := ids.Account
	k := ids.K
	if k == "" && ids.LiveURL != "" {
		// Parse k and account from live_url query params.
		k, account = parseLiveURLParams(ids.LiveURL)
	}

	form := map[string]string{
		"account":   account,
		"k":         k,
		"encry_key": encrypted,
	}

	body, err := c.PostForm(encrypt_url, form, headers)
	if err != nil {
		return eoffcnPlayback{}
	}

	// Step 6: Parse and AES-decrypt the response.
	var payload map[string]any
	if json.Unmarshal([]byte(body), &payload) != nil {
		return eoffcnPlayback{}
	}
	respData, _ := payload["data"].(string)
	if respData == "" {
		return eoffcnPlayback{}
	}

	// AES-decrypt with randomKey as both key and IV.
	decrypted := aesDecryptWithStatic(respData, randomKey, randomKey)
	if decrypted == "" {
		return eoffcnPlayback{}
	}

	// Step 7: Parse the decrypted data for media URLs and whiteboard metadata.
	return extractWatchDemandPlayback(decrypted)
}

// getPubKey fetches the RSA public key from the server.
// Source: Eoffcn_Course._get_pub_key (line 279).
func getPubKey(c *util.Client, headers map[string]string) string {
	body, err := c.GetString(pub_key_url, headers)
	if err != nil {
		return ""
	}
	var payload map[string]any
	if json.Unmarshal([]byte(body), &payload) != nil {
		return ""
	}
	data, _ := payload["data"].(string)
	return data
}

// aesDecryptWithStatic decrypts base64-encoded AES-CBC data using the given
// key and IV strings. Returns the decrypted plaintext as a string.
// Mirrors Python: AESEncrypt(key, iv).aes_decrypt(data).
func aesDecryptWithStatic(encrypted, key, iv string) string {
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		// Try URL-safe base64.
		ciphertext, err = base64.URLEncoding.DecodeString(encrypted)
		if err != nil {
			return ""
		}
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return ""
	}
	if len(iv) != block.BlockSize() {
		return ""
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return ""
	}
	mode := cipher.NewCBCDecrypter(block, []byte(iv))
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)
	plaintext = pkcs7Unpad(plaintext)
	if len(plaintext) == 0 {
		return ""
	}
	return string(plaintext)
}

// parseLiveURLParams extracts the "k" and "account" parameters from a live_url.
// Source: Eoffcn_Course._parse_live_url_params (line 496).
func parseLiveURLParams(liveURL string) (k, account string) {
	if u, err := url.Parse(liveURL); err == nil {
		q := u.Query()
		k = q.Get("k")
		account = q.Get("account")
	}
	if k == "" {
		if m := regexp.MustCompile(`[?&]k=([^&]+)`).FindStringSubmatch(liveURL); len(m) > 1 {
			k = m[1]
		}
	}
	if account == "" {
		if m := regexp.MustCompile(`[?&]account=([^&]+)`).FindStringSubmatch(liveURL); len(m) > 1 {
			account = m[1]
		}
	}
	return
}

// extractWatchDemandURL parses the decrypted watch_demand response for media URLs.
// Source: Eoffcn_Course._normalize_watch_demand_data (line 528) extracts URLs
// from many possible field names.
func extractWatchDemandURL(decrypted string) string {
	return extractWatchDemandPlayback(decrypted).URL
}

func extractWatchDemandPlayback(decrypted string) eoffcnPlayback {
	// Try parsing as JSON.
	var data any
	if json.Unmarshal([]byte(decrypted), &data) == nil {
		whiteboard := detectEoffcnWhiteboard(data)
		if u := findMediaURL(data); u != "" {
			return eoffcnPlayback{URL: u, Whiteboard: whiteboard, Extra: eoffcnWhiteboardExtra(whiteboard)}
		}
		// Also check for watch_demand-specific fields.
		if u := findWatchDemandFields(data); u != "" {
			return eoffcnPlayback{URL: u, Whiteboard: whiteboard, Extra: eoffcnWhiteboardExtra(whiteboard)}
		}
		if whiteboard.Whiteboard && whiteboard.APIURL != "" {
			return eoffcnPlayback{URL: whiteboard.APIURL, Whiteboard: whiteboard, Extra: eoffcnWhiteboardExtra(whiteboard)}
		}
	}
	whiteboard := detectEoffcnWhiteboard(decrypted)
	// Try extracting "vod":"..." pattern (used by _decrypt_video_key).
	if m := vodURLRe.FindStringSubmatch(decrypted); len(m) > 1 {
		u := strings.ReplaceAll(m[1], `\`, "")
		if isMediaURL(u) {
			return eoffcnPlayback{URL: u, Whiteboard: whiteboard, Extra: eoffcnWhiteboardExtra(whiteboard)}
		}
	}
	if whiteboard.Whiteboard && whiteboard.APIURL != "" {
		return eoffcnPlayback{URL: whiteboard.APIURL, Whiteboard: whiteboard, Extra: eoffcnWhiteboardExtra(whiteboard)}
	}
	return eoffcnPlayback{}
}

var vodURLRe = regexp.MustCompile(`"vod"\s*:\s*"(.*?)"`)

// vodKeyRe can extract the AES decryption key for the vod URL if needed.
// Source: _decrypt_video_key regex: "vod_key":"(.*?)"
// Currently the watch_demand flow returns ready-to-use URLs so this is reserved.
// var vodKeyRe = regexp.MustCompile(`"vod_key"\s*:\s*"(.*?)"`)

// findWatchDemandFields looks for eoffcn-specific watch_demand response fields.
// Source: Eoffcn_Course._normalize_watch_demand_data checks many whiteboard/
// audio URL field variants.
func findWatchDemandFields(v any) string {
	watchDemandKeys := []string{
		// Whiteboard play URLs
		"white_board_play_url", "whiteBoardPlayUrl", "whiteBoardPlayURL",
		"whiteboard_play_url", "whiteboardPlayUrl", "wbx_url", "wbxUrl",
		// Whiteboard resource URLs
		"white_board_resource_url", "whiteBoardResourceUrl", "whiteBoardResourceURL",
		"whiteboard_resource_url", "whiteboardResourceUrl", "wbr_url", "wbrUrl",
		// Board font
		"board_font", "boardFont", "font_url", "fontUrl",
		// Audio
		"audio_url", "audioUrl",
		// Standard media
		"live_url", "video_url", "m3u8", "m3u8Url", "playUrl", "play_url", "url",
	}
	switch x := v.(type) {
	case map[string]any:
		// If data field is present and is a dict/string, recurse into it.
		if dataVal, ok := x["data"]; ok {
			if u := findWatchDemandFields(dataVal); u != "" {
				return u
			}
		}
		for _, k := range watchDemandKeys {
			if s := normalizeURL(valueString(x, k)); isMediaURL(s) {
				return s
			}
		}
		for _, k := range watchDemandKeys {
			s := normalizeURL(valueString(x, k))
			if isEoffcnWhiteboardKey(k) && isHTTPURL(s) {
				return s
			}
		}
		// Check "items" sub-array.
		if items, ok := x["items"]; ok {
			if u := findWatchDemandFields(items); u != "" {
				return u
			}
		}
		for _, child := range x {
			if u := findWatchDemandFields(child); u != "" {
				return u
			}
		}
	case []any:
		for _, child := range x {
			if u := findWatchDemandFields(child); u != "" {
				return u
			}
		}
	case string:
		if s := normalizeURL(x); isMediaURL(s) {
			return s
		}
		// May be a nested JSON string.
		var inner any
		if json.Unmarshal([]byte(x), &inner) == nil {
			return findWatchDemandFields(inner)
		}
	}
	return ""
}

func parseParams(raw string) eoffcnParams {
	out := eoffcnParams{ModuleType: "0"}
	if u, err := url.Parse(raw); err == nil {
		q := u.Query()
		out.SystemOrder = firstNonEmpty(q.Get("system_order"), q.Get("systemSn"), q.Get("system_order_num"), q.Get("order"))
		out.PackageID = firstNonEmpty(q.Get("package_id"), q.Get("packageId"), q.Get("cid"))
		out.LessonID = firstNonEmpty(q.Get("lesson_id"), q.Get("lessonId"), q.Get("lid"))
		out.ModuleType = firstNonEmpty(q.Get("module_type"), q.Get("moduleType"), q.Get("m_type"), "0")
		out.Coding = firstNonEmpty(q.Get("coding"), q.Get("code"))
		out.SpuID = firstNonEmpty(q.Get("spuId"), q.Get("spu_id"), q.Get("spu"))
	}
	out.SystemOrder = firstNonEmpty(out.SystemOrder, rx(courseIDRe, raw))
	out.PackageID = firstNonEmpty(out.PackageID, rx(packageIDRe, raw))
	out.LessonID = firstNonEmpty(out.LessonID, rx(lessonIDRe, raw))
	out.Coding = firstNonEmpty(out.Coding, rx(codingRe, raw))
	out.SpuID = firstNonEmpty(out.SpuID, rx(spuIDRe, raw))
	return out
}

func fetchSpuIDFromPage(c *util.Client, rawURL string, headers map[string]string) string {
	body, err := c.GetString(rawURL, headers)
	if err != nil {
		return ""
	}
	return firstNonEmpty(rx(spuIDRe, body), rx(spuIDRe, rawURL))
}

func selectOrder(c *util.Client, headers map[string]string, p eoffcnParams) (eoffcnOrder, error) {
	orders, err := fetchOldOrders(c, headers)
	if err != nil {
		return eoffcnOrder{}, err
	}
	if len(orders) == 0 {
		return eoffcnOrder{}, fmt.Errorf("eoffcn: no old orders found")
	}
	if selected, ok := matchOldOrder(orders, p); ok {
		return selected, nil
	}
	return eoffcnOrder{}, fmt.Errorf("eoffcn: old order not found for spuId=%s system_order=%s", p.SpuID, p.SystemOrder)
}

func mergeOrderParams(p eoffcnParams, order eoffcnOrder) eoffcnParams {
	if order.SystemOrder != "" {
		p.SystemOrder = order.SystemOrder
	}
	if order.SpuID != "" {
		p.SpuID = order.SpuID
	}
	if order.Title != "" {
		p.Title = order.Title
	}
	if order.PayMoney != "" {
		p.PayMoney = order.PayMoney
	}
	return p
}

func fetchOldOrders(c *util.Client, headers map[string]string) ([]eoffcnOrder, error) {
	body, err := c.GetString(order_url, headers)
	if err != nil {
		return nil, fmt.Errorf("fetch eoffcn old order list: %w", err)
	}
	var payload any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil, fmt.Errorf("parse eoffcn old order list: %w", err)
	}
	return collectOldOrders(payload), nil
}

func matchOldOrder(orders []eoffcnOrder, p eoffcnParams) (eoffcnOrder, bool) {
	for _, order := range orders {
		if p.SystemOrder != "" && order.SystemOrder != p.SystemOrder {
			continue
		}
		if p.SpuID != "" && order.SpuID != p.SpuID {
			continue
		}
		return order, true
	}
	if p.SystemOrder != "" {
		for _, order := range orders {
			if order.SystemOrder == p.SystemOrder {
				return order, true
			}
		}
	}
	if p.SpuID != "" {
		for _, order := range orders {
			if order.SpuID == p.SpuID {
				return order, true
			}
		}
	}
	return eoffcnOrder{}, false
}

func collectOldOrders(v any) []eoffcnOrder {
	var out []eoffcnOrder
	var walk func(any)
	walk = func(x any) {
		switch vv := x.(type) {
		case map[string]any:
			systemOrder := firstNonEmpty(valueString(vv, "systemSn", "systemSnNum", "system_order_num", "system_order"), valueString(mapValue(vv, "orderInfoExpand"), "systemSn", "system_order_num"))
			spuID := firstNonEmpty(valueString(vv, "spuId", "spu_id", "spuID"), valueString(mapValue(vv, "orderInfoExpand"), "spuId"))
			title := firstNonEmpty(valueString(vv, "goodsName", "cargoName", "name", "title"), valueString(mapValue(vv, "orderInfoExpand"), "goodsName", "cargoName"))
			payMoney := firstNonEmpty(valueString(vv, "payMoney", "pay_money", "money"), valueString(mapValue(vv, "orderInfoExpand"), "payMoney", "money"))
			if systemOrder != "" && (spuID != "" || title != "" || payMoney != "") {
				out = append(out, eoffcnOrder{SystemOrder: systemOrder, SpuID: spuID, Title: title, PayMoney: payMoney})
			}
			for _, child := range vv {
				walk(child)
			}
		case []any:
			for _, child := range vv {
				walk(child)
			}
		}
	}
	walk(v)
	return dedupeOrders(out)
}

func dedupeOrders(orders []eoffcnOrder) []eoffcnOrder {
	seen := map[string]bool{}
	out := make([]eoffcnOrder, 0, len(orders))
	for _, order := range orders {
		key := order.SystemOrder + "\x00" + order.SpuID + "\x00" + order.Title
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, order)
	}
	return out
}

func mapValue(m map[string]any, key string) map[string]any {
	if v, ok := m[key]; ok {
		if mv, ok := v.(map[string]any); ok {
			return mv
		}
	}
	return map[string]any{}
}

func collectLessonNodes(v any, p eoffcnParams) []lessonNode {
	var out []lessonNode
	var walk func(any, string)
	walk = func(x any, prefix string) {
		switch vv := x.(type) {
		case map[string]any:
			id := valueString(vv, "id", "lesson_id", "lessonId", "video_id", "videoId")
			moduleType := valueString(vv, "module_type", "moduleType", "type", "video_type")
			if id != "" && (hasAny(vv, "level_name", "room_id", "roomId", "file_id", "fileId", "video_id", "videoId") || strings.Contains(strings.ToLower(prefix), "lesson")) {
				out = append(out, lessonNode{ID: id, Title: firstNonEmpty(valueString(vv, "level_name", "name", "title"), prefix), PackageID: firstNonEmpty(valueString(vv, "package_id", "packageId"), p.PackageID), ModuleType: firstNonEmpty(moduleType, p.ModuleType), RoomID: valueString(vv, "room_id", "roomId"), FileID: valueString(vv, "file_id", "fileId")})
			}
			nextPrefix := firstNonEmpty(valueString(vv, "level_name", "name", "title"), prefix)
			for _, k := range []string{"child", "children", "outline_info", "list", "data", "items"} {
				if child, ok := vv[k]; ok {
					walk(child, nextPrefix)
				}
			}
		case []any:
			for _, item := range vv {
				walk(item, prefix)
			}
		}
	}
	walk(v, "")
	return out
}

func collectWatchIDs(v any) watchIDs {
	ids := watchIDs{}
	var walk func(any)
	walk = func(x any) {
		switch vv := x.(type) {
		case map[string]any:
			ids.VideoID = firstNonEmpty(ids.VideoID, valueString(vv, "video_id", "videoId", "vid"))
			ids.RoomID = firstNonEmpty(ids.RoomID, valueString(vv, "room_id", "roomId"))
			ids.Account = firstNonEmpty(ids.Account, valueString(vv, "account", "uid", "user_id"))
			ids.K = firstNonEmpty(ids.K, valueString(vv, "k"))
			// Capture live_url / video_url for parsing k and account.
			ids.LiveURL = firstNonEmpty(ids.LiveURL, valueString(vv, "live_url", "video_url"))
			for _, child := range vv {
				walk(child)
			}
		case []any:
			for _, child := range vv {
				walk(child)
			}
		}
	}
	walk(v)
	return ids
}

func detectEoffcnWhiteboard(v any) eoffcnWhiteboardInfo {
	info := eoffcnWhiteboardInfo{Params: map[string]string{}}
	var walk func(any, string)
	walk = func(x any, key string) {
		if isEoffcnWhiteboardKey(key) {
			info.Whiteboard = true
			if info.Source == "" {
				info.Source = key
			}
		}
		switch t := x.(type) {
		case map[string]any:
			for k, child := range t {
				if strings.EqualFold(k, "video_type") && strings.TrimSpace(fmt.Sprint(child)) == "6" {
					info.Whiteboard = true
					if info.Source == "" {
						info.Source = "video_type=6"
					}
				}
				if isEoffcnBoardParamKey(k) {
					addEoffcnWhiteboardParam(info.Params, k, fmt.Sprint(child))
				}
				if s, ok := child.(string); ok {
					considerEoffcnWhiteboardURL(&info, s, k, isEoffcnWhiteboardKey(k))
				}
				walk(child, k)
			}
		case []any:
			for _, child := range t {
				walk(child, key)
			}
		case string:
			s := normalizeURL(t)
			if looksLikeEoffcnWhiteboardString(s) {
				info.Whiteboard = true
				if info.Source == "" {
					info.Source = key
				}
			}
			considerEoffcnWhiteboardURL(&info, s, key, isEoffcnWhiteboardKey(key))
			if nested, ok := parseNestedJSON(s); ok {
				walk(nested, key)
			}
		case float64, int, int64, json.Number:
			if strings.EqualFold(key, "video_type") && strings.TrimSpace(fmt.Sprint(t)) == "6" {
				info.Whiteboard = true
				if info.Source == "" {
					info.Source = "video_type=6"
				}
			}
		}
	}
	walk(v, "")
	if len(info.Params) == 0 {
		info.Params = nil
	}
	return info
}

func mergeEoffcnWhiteboardInfo(a, b eoffcnWhiteboardInfo) eoffcnWhiteboardInfo {
	out := a
	if b.Whiteboard {
		out.Whiteboard = true
	}
	if out.APIURL == "" || eoffcnWhiteboardURLScore(b.APIURL) > eoffcnWhiteboardURLScore(out.APIURL) {
		out.APIURL = b.APIURL
	}
	if out.Source == "" {
		out.Source = b.Source
	}
	if len(b.Params) > 0 {
		if out.Params == nil {
			out.Params = map[string]string{}
		}
		for k, v := range b.Params {
			if _, ok := out.Params[k]; !ok && strings.TrimSpace(v) != "" {
				out.Params[k] = v
			}
		}
	}
	return out
}

func eoffcnWhiteboardExtra(info eoffcnWhiteboardInfo) map[string]any {
	if !info.Whiteboard {
		return nil
	}
	extra := map[string]any{
		"whiteboard":          true,
		"whiteboard_renderer": "eoffcn_wboffcn",
		"whiteboard_referer":  eoffcnBoardReferer,
		"wboffcn_js_url":      eoffcnWbOffcnJSURL,
		"wboffcn_mem_url":     eoffcnWbOffcnMemURL,
		"render_required":     true,
	}
	if info.APIURL != "" {
		extra["whiteboard_api_url"] = info.APIURL
	}
	if len(info.Params) > 0 {
		extra["whiteboard_params"] = info.Params
	}
	if info.Source != "" {
		extra["whiteboard_source"] = info.Source
	}
	return extra
}

func mergeEoffcnExtra(dst, src map[string]any) map[string]any {
	if len(dst) == 0 && len(src) == 0 {
		return nil
	}
	out := map[string]any{}
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range src {
		out[k] = v
	}
	return out
}

func considerEoffcnWhiteboardURL(info *eoffcnWhiteboardInfo, raw, source string, fromBoardKey bool) {
	s := normalizeURL(raw)
	if s == "" {
		return
	}
	collectEoffcnWhiteboardParams(info.Params, s)
	if !isHTTPURL(s) {
		return
	}
	if fromBoardKey || isEoffcnWhiteboardURL(s) {
		info.Whiteboard = true
		if info.Source == "" {
			info.Source = source
		}
		if info.APIURL == "" || eoffcnWhiteboardURLScore(s) > eoffcnWhiteboardURLScore(info.APIURL) {
			info.APIURL = s
		}
	}
}

func collectEoffcnWhiteboardParams(params map[string]string, raw string) {
	if params == nil {
		return
	}
	u, err := url.Parse(normalizeURL(raw))
	if err == nil {
		for key, values := range u.Query() {
			if len(values) > 0 && isEoffcnBoardParamKey(key) {
				addEoffcnWhiteboardParam(params, key, values[0])
			}
		}
	}
	for _, key := range []string{"room_num", "roomNum", "room_id", "roomId", "account", "k", "video_id", "videoId", "vid", "live_id", "liveId", "package_id", "packageId"} {
		re := regexp.MustCompile(`(?i)(?:^|[?&#])` + regexp.QuoteMeta(key) + `=([^&#]+)`)
		if m := re.FindStringSubmatch(raw); len(m) > 1 {
			if v, err := url.QueryUnescape(m[1]); err == nil {
				addEoffcnWhiteboardParam(params, key, v)
			} else {
				addEoffcnWhiteboardParam(params, key, m[1])
			}
		}
	}
}

func addEoffcnWhiteboardParam(params map[string]string, key, value string) {
	if params == nil {
		return
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" || value == "<nil>" {
		return
	}
	if len(value) > 2048 {
		return
	}
	if _, ok := params[key]; !ok {
		params[key] = value
	}
}

func isEoffcnBoardParamKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "room_num", "roomnum", "room_id", "roomid", "account", "k", "video_id", "videoid", "vid", "live_id", "liveid", "package_id", "packageid":
		return true
	default:
		return false
	}
}

func isEoffcnWhiteboardKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return false
	}
	for _, marker := range []string{
		"whiteboard", "white_board", "board_context", "boardcontext", "board_url", "boardurl",
		"board_play", "boardplay", "board_resource", "boardresource", "board_font", "boardfont",
		"wboffcn", "wbx", "wbr",
	} {
		if strings.Contains(k, marker) {
			return true
		}
	}
	return false
}

func looksLikeEoffcnWhiteboardString(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	if low == "" {
		return false
	}
	for _, marker := range []string{
		"pcvod.offcncloud.com", "wboffcn", "wb1004", "/base/wb",
		"whiteboard", "white_board", "board_context", ".wbx", ".wbr", "板书",
	} {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

func isEoffcnWhiteboardURL(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	if !isHTTPURL(low) {
		return false
	}
	if strings.Contains(low, "pcvod.offcncloud.com") ||
		strings.Contains(low, "wboffcn") ||
		strings.Contains(low, "wb1004") ||
		strings.Contains(low, "/base/wb") ||
		strings.Contains(low, "whiteboard") ||
		strings.Contains(low, "white_board") ||
		strings.Contains(low, "board") ||
		strings.Contains(low, ".wbx") ||
		strings.Contains(low, ".wbr") {
		return true
	}
	if strings.Contains(low, "offcncloud.com") && (strings.Contains(low, "room_id=") || strings.Contains(low, "room_num=") || strings.Contains(low, "account=") || strings.Contains(low, "k=")) {
		return true
	}
	return false
}

func eoffcnWhiteboardURLScore(s string) int {
	low := strings.ToLower(strings.TrimSpace(s))
	if low == "" {
		return 0
	}
	score := 1
	if strings.Contains(low, "pcvod.offcncloud.com") {
		score += 100
	}
	if strings.Contains(low, "offcncloud.com") {
		score += 50
	}
	if strings.Contains(low, "whiteboard") || strings.Contains(low, "white_board") || strings.Contains(low, "board") {
		score += 30
	}
	if strings.Contains(low, "wboffcn") || strings.Contains(low, "wb1004") || strings.Contains(low, ".wbx") || strings.Contains(low, ".wbr") {
		score += 20
	}
	if isMediaURL(low) {
		score -= 10
	}
	return score
}

func parseNestedJSON(s string) (any, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") && !strings.HasPrefix(s, "[") {
		return nil, false
	}
	var out any
	if json.Unmarshal([]byte(s), &out) != nil {
		return nil, false
	}
	return out, true
}

func findMediaURL(v any) string {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range []string{"live_url", "video_url", "m3u8", "m3u8Url", "playUrl", "play_url", "url"} {
			if s := normalizeURL(valueString(x, k)); isMediaURL(s) {
				return s
			}
		}
		// Source: Eoffcn_Course._get_m3u8_info decrypts live_url / video_url with static AES key.
		// Try AES-CBC decryption on opaque values that did not pass isMediaURL above.
		for _, k := range []string{"live_url", "video_url"} {
			raw := valueString(x, k)
			if raw == "" {
				continue
			}
			if dec := aesDecryptLiveURL(raw); isMediaURL(dec) {
				return dec
			}
		}
		for _, child := range x {
			if s := findMediaURL(child); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range x {
			if s := findMediaURL(child); s != "" {
				return s
			}
		}
	case string:
		if s := normalizeURL(x); isMediaURL(s) {
			return s
		}
	}
	return ""
}

// aesDecryptLiveURL attempts AES-CBC decryption of an encrypted URL string using the
// static key/IV pair from the source (Eoffcn_Course._get_m3u8_info line 330).
// The source uses AESEncrypt.aes_decrypt with key "1234567898882222" and IV "8NONwyJtHesysWpM".
func aesDecryptLiveURL(encrypted string) string {
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		// Try raw hex or URL-safe base64 as fallback.
		ciphertext, err = base64.URLEncoding.DecodeString(encrypted)
		if err != nil {
			return ""
		}
	}
	block, err := aes.NewCipher([]byte(aesKey))
	if err != nil {
		return ""
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return ""
	}
	mode := cipher.NewCBCDecrypter(block, []byte(aesIV))
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	// Remove PKCS7 padding.
	plaintext = pkcs7Unpad(plaintext)
	if len(plaintext) == 0 {
		return ""
	}
	return normalizeURL(string(plaintext))
}

// pkcs7Unpad removes PKCS#7 padding from decrypted data.
func pkcs7Unpad(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > aes.BlockSize || padLen > len(data) {
		return data // not padded or invalid, return as-is
	}
	for i := len(data) - padLen; i < len(data); i++ {
		if data[i] != byte(padLen) {
			return data // invalid padding, return as-is
		}
	}
	return data[:len(data)-padLen]
}

func mediaInfo(title, u string, h map[string]string) *extractor.MediaInfo {
	return mediaInfoWithExtra(title, u, h, nil)
}

func mediaInfoWithExtra(title, u string, h map[string]string, extra map[string]any) *extractor.MediaInfo {
	format := eoffcnFormat(u, extra)
	streams := map[string]extractor.Stream{"best": {
		Quality:   "best",
		URLs:      []string{u},
		Format:    format,
		NeedMerge: format == "m3u8",
		Headers:   h,
		Extra:     cloneExtra(extra),
	}}
	if htmlURL := extraString(extra, "whiteboard_html_url"); htmlURL != "" && htmlURL != u {
		streams["whiteboard"] = extractor.Stream{
			Quality: "whiteboard",
			URLs:    []string{htmlURL},
			Format:  "html",
			Headers: map[string]string{"Referer": eoffcnBoardReferer},
			Extra:   cloneExtra(extra),
		}
	}
	return &extractor.MediaInfo{
		Site:    "eoffcn",
		Title:   title,
		Streams: streams,
		Extra:   cloneExtra(extra),
	}
}

func eoffcnFormat(u string, extra map[string]any) string {
	low := strings.ToLower(u)
	switch {
	case strings.HasPrefix(low, "data:text/html"):
		return "html"
	case strings.Contains(low, ".m3u8"):
		return "m3u8"
	case strings.Contains(low, ".flv"):
		return "flv"
	case strings.Contains(low, ".mp3"):
		return "mp3"
	case strings.Contains(low, ".mp4"):
		return "mp4"
	case extra != nil && extra["whiteboard"] == true:
		return "html"
	default:
		return "mp4"
	}
}

func eoffcnStreamHeaders(headers map[string]string, mediaURL string, extra map[string]any) map[string]string {
	out := cloneStringMap(headers)
	if len(extra) > 0 {
		if whiteboard, ok := extra["whiteboard"].(bool); ok && whiteboard {
			if apiURL, _ := extra["whiteboard_api_url"].(string); mediaURL == apiURL || needsEoffcnBoardReferer(mediaURL) {
				out["Referer"] = eoffcnBoardReferer
			}
		}
	}
	return out
}

func needsEoffcnBoardReferer(mediaURL string) bool {
	low := strings.ToLower(strings.TrimSpace(mediaURL))
	if low == "" {
		return false
	}
	if strings.Contains(low, "pcvod.offcncloud.com") ||
		strings.Contains(low, "wboffcn") ||
		strings.Contains(low, "wb1004") ||
		strings.Contains(low, "/base/wb") ||
		strings.Contains(low, ".wbx") ||
		strings.Contains(low, ".wbr") {
		return true
	}
	return !isMediaURL(mediaURL) && isEoffcnWhiteboardURL(mediaURL)
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneExtra(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func rx(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	for i := 1; i < len(m); i++ {
		if m[i] != "" {
			return m[i]
		}
	}
	return ""
}

func pickTitle(v any) string {
	if m, ok := v.(map[string]any); ok {
		if s := valueString(m, "goodsName", "cargoName", "name", "title", "level_name"); s != "" {
			return s
		}
		for _, child := range m {
			if s := pickTitle(child); s != "" {
				return s
			}
		}
	} else if a, ok := v.([]any); ok {
		for _, child := range a {
			if s := pickTitle(child); s != "" {
				return s
			}
		}
	}
	return ""
}

func valueString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func hasAny(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func normalizeURL(s string) string {
	s = strings.TrimSpace(strings.Trim(s, `"'`))
	s = strings.ReplaceAll(s, `\/`, `/`)
	if strings.HasPrefix(s, "//") {
		return "https:" + s
	}
	return s
}

func isMediaURL(s string) bool {
	low := strings.ToLower(s)
	return strings.HasPrefix(low, "http") && (strings.Contains(low, ".m3u8") || strings.Contains(low, ".mp4") || strings.Contains(low, ".flv") || strings.Contains(low, ".mp3"))
}

func isHTTPURL(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
