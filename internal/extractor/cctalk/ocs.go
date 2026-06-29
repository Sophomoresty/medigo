package cctalk

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/Sophomoresty/mediago/internal/extractor"
)

var envNameUnsafeRe = regexp.MustCompile(`[^A-Za-z0-9]+`)

func extractCoursewareInfo(item any) map[string]any {
	out := map[string]any{}
	collectCoursewareInfo(item, out, 0)
	if firstNonEmpty(textValue(out, "tenantId"), "") == "" {
		out["tenantId"] = CCTALK_TENANT_ID
	}
	return out
}

func collectCoursewareInfo(value any, out map[string]any, depth int) {
	if value == nil || depth > 7 {
		return
	}
	switch x := value.(type) {
	case map[string]any:
		for _, key := range []string{"coursewareInfo", "courseWareInfo", "courseware_info", "ocsInfo", "videoInfo", "mediaInfo", "contentInfo", "resourceInfo", "playInfo", "activityInfo", "lessonInfo", "detail", "raw"} {
			if nested, ok := x[key]; ok {
				collectCoursewareInfo(nested, out, depth+1)
			}
		}
		for _, pair := range [][2]string{
			{"coursewareId", "coursewareId"}, {"courseWareId", "coursewareId"}, {"courseware_id", "coursewareId"}, {"coursewareID", "coursewareId"}, {"courseId", "coursewareId"}, {"course_id", "coursewareId"}, {"ocsId", "coursewareId"}, {"ocs_id", "coursewareId"},
			{"videoId", "videoId"}, {"video_id", "videoId"}, {"contentId", "videoId"}, {"content_id", "videoId"},
			{"tenantId", "tenantId"}, {"tenantID", "tenantId"}, {"tenant_id", "tenantId"},
			{"sourceType", "sourceType"}, {"source_type", "sourceType"}, {"contentType", "contentType"}, {"content_type", "contentType"},
			{"userSign", "userSign"}, {"user_sign", "userSign"}, {"userSignKey", "userSign"}, {"user_sign_key", "userSign"}, {"xUserSign", "userSign"}, {"x_user_sign", "userSign"}, {"signature", "userSign"}, {"sign", "userSign"},
			{"videoUrl", "videoUrl"}, {"videoURL", "videoUrl"}, {"playUrl", "videoUrl"}, {"playURL", "videoUrl"}, {"m3u8Url", "videoUrl"}, {"m3u8URL", "videoUrl"}, {"hlsUrl", "videoUrl"}, {"hlsURL", "videoUrl"}, {"mediaUrl", "videoUrl"}, {"mediaURL", "videoUrl"}, {"mp4Url", "videoUrl"}, {"mp4URL", "videoUrl"}, {"downloadUrl", "videoUrl"}, {"downloadURL", "videoUrl"}, {"sourceUrl", "videoUrl"}, {"sourceURL", "videoUrl"}, {"resourceUrl", "videoUrl"}, {"resourceURL", "videoUrl"}, {"url", "videoUrl"},
			{"path", "videoPath"}, {"filePath", "videoPath"}, {"mediaPath", "videoPath"}, {"resourcePath", "videoPath"}, {"sourcePath", "videoPath"}, {"playPath", "videoPath"}, {"hlsPath", "videoPath"}, {"m3u8Path", "videoPath"}, {"mp4Path", "videoPath"}, {"objectKey", "videoPath"},
			{"fileUrl", "fileUrl"}, {"fileURL", "fileUrl"}, {"materialUrl", "fileUrl"}, {"attachUrl", "fileUrl"}, {"attachURL", "fileUrl"},
		} {
			if textValue(out, pair[1]) == "" {
				if value := textValue(x, pair[0]); value != "" {
					out[pair[1]] = value
				}
			}
		}
		for _, nested := range x {
			switch nested.(type) {
			case map[string]any, []any:
				collectCoursewareInfo(nested, out, depth+1)
			}
		}
	case []any:
		for _, item := range x {
			collectCoursewareInfo(item, out, depth+1)
		}
	}
}

func hasMeaningfulCoursewareInfo(info map[string]any) bool {
	if len(info) == 0 {
		return false
	}
	for _, key := range []string{"coursewareId", "videoId", "userSign", "sourceType", "contentType"} {
		if key == "tenantId" {
			continue
		}
		if textValue(info, key) != "" {
			return true
		}
	}
	return false
}

func ocsHeadersFor(coursewareInfo map[string]any) map[string]string {
	userSign := userSignForCourseware(coursewareInfo)
	headers := map[string]string{
		"Accept":          "application/json, text/plain, */*",
		"Referer":         CCTALK_BASE_URL + "/",
		"Origin":          CCTALK_BASE_URL,
		"User-Agent":      CCTALK_OCS_USER_AGENT,
		"X-Tenant-Id":     firstNonEmpty(textValue(coursewareInfo, "tenantId"), CCTALK_TENANT_ID),
		"X-Tenant-ID":     firstNonEmpty(textValue(coursewareInfo, "tenantId"), CCTALK_TENANT_ID),
		"X-User-Sign":     userSign,
		"Hujiang-App-Key": CCTALK_PCWEB_KEY,
	}
	if headers["X-User-Sign"] == "" {
		delete(headers, "X-User-Sign")
	}
	return headers
}

func userSignForCourseware(coursewareInfo map[string]any) string {
	if sign := firstNonEmpty(textValue(coursewareInfo, "userSign"), textValue(coursewareInfo, "xUserSign"), textValue(coursewareInfo, "signature"), textValue(coursewareInfo, "sign")); looksLikeUserSign(sign) {
		return sign
	}
	coursewareID := textValue(coursewareInfo, "coursewareId")
	for _, name := range []string{"CCTALK_USER_SIGN", "CCTALK_X_USER_SIGN", "CCTALK_USER_SIGN_KEY", "CCTALK_SIGNATURE"} {
		if sign := strings.TrimSpace(os.Getenv(name)); looksLikeUserSign(sign) {
			return sign
		}
	}
	if coursewareID != "" {
		envName := "CCTALK_USER_SIGN_" + strings.Trim(envNameUnsafeRe.ReplaceAllString(coursewareID, "_"), "_")
		if sign := strings.TrimSpace(os.Getenv(envName)); looksLikeUserSign(sign) {
			return sign
		}
	}
	if raw := strings.TrimSpace(os.Getenv("CCTALK_USER_SIGN_MAP")); raw != "" {
		var root any
		if json.Unmarshal([]byte(raw), &root) == nil {
			if sign := lookupUserSignMap(root, coursewareID, textValue(coursewareInfo, "videoId")); sign != "" {
				return sign
			}
		}
	}
	return ""
}

func lookupUserSignMap(root any, keys ...string) string {
	switch x := root.(type) {
	case map[string]any:
		for _, key := range keys {
			if key == "" {
				continue
			}
			if sign := textAny(x[key]); looksLikeUserSign(sign) {
				return sign
			}
			if m := asMap(x[key]); len(m) > 0 {
				if sign := firstNonEmpty(textValue(m, "userSign", "xUserSign", "signature", "sign", "value")); looksLikeUserSign(sign) {
					return sign
				}
			}
		}
		for _, field := range []string{"userSign", "xUserSign", "signature", "sign", "value"} {
			if sign := textAny(x[field]); looksLikeUserSign(sign) {
				return sign
			}
		}
	}
	return ""
}

func looksLikeUserSign(sign string) bool {
	sign = strings.TrimSpace(sign)
	if sign == "" || sign == "<nil>" {
		return false
	}
	lower := strings.ToLower(sign)
	return lower != "null" && lower != "undefined" && lower != "false" && lower != "0"
}

func (a *apiClient) resolveOCSStream(coursewareInfo map[string]any) (extractor.Stream, map[string]any, bool) {
	coursewareID := textValue(coursewareInfo, "coursewareId")
	if coursewareID == "" || a == nil || a.c == nil {
		return extractor.Stream{}, nil, false
	}
	headers := ocsHeadersFor(coursewareInfo)
	for _, endpoint := range ocsEndpoints(coursewareID, coursewareInfo) {
		body, err := a.c.GetString(endpoint, headers)
		if err != nil || strings.TrimSpace(body) == "" {
			continue
		}
		var payload any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			continue
		}
		if stream, extra, ok := buildOCSStreamFromPayload(payload, coursewareInfo, headers); ok {
			extra["ocs_url"] = endpoint
			return stream, extra, true
		}
	}
	return extractor.Stream{}, nil, false
}

func ocsQuery(coursewareInfo map[string]any) string {
	q := url.Values{}
	for _, key := range []string{"tenantId", "sourceType", "contentType"} {
		if value := textValue(coursewareInfo, key); value != "" {
			q.Set(key, value)
		}
	}
	return q.Encode()
}

func ocsEndpoints(coursewareID string, coursewareInfo map[string]any) []string {
	q := ocsQuery(coursewareInfo)
	seen := map[string]bool{}
	var out []string
	add := func(endpoint string) {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" || seen[endpoint] {
			return
		}
		seen[endpoint] = true
		out = append(out, endpoint)
	}
	for _, base := range cctalkOCSCurrentBases {
		base = strings.TrimRight(base, "/")
		current := base + "/courseware_contents/" + url.PathEscape(coursewareID)
		if q != "" {
			current += "?" + q
		}
		add(current)
		legacy := base + "/courseware_contents/" + url.PathEscape(coursewareID) + "?part=1"
		if q != "" {
			legacy += "&" + q
		}
		add(legacy)
	}
	for _, host := range []string{"https://courseware-ocs.hjapi.com", "https://courseware-ocs1.hjapi.com"} {
		add(strings.TrimRight(host, "/") + "/v5/courseware_contents/h5/" + url.PathEscape(coursewareID) + "?t=" + fmt.Sprint(time.Now().Unix()))
	}
	return out
}

func buildEmbeddedOCSStream(item map[string]any, coursewareInfo map[string]any) (extractor.Stream, map[string]any, bool) {
	return buildOCSStreamFromPayload(item, coursewareInfo, ocsHeadersFor(coursewareInfo))
}

func buildOCSStreamFromPayload(payload any, coursewareInfo map[string]any, headers map[string]string) (extractor.Stream, map[string]any, bool) {
	payload = normalizeOCSPayload(payload)
	if mediaURL := normalizeOCSResourceURL(firstNonEmpty(findMediaURL(payload), textValue(coursewareInfo, "videoPath", "videoUrl"))); mediaURL != "" {
		if strings.HasPrefix(strings.TrimSpace(mediaURL), "#EXTM3U") {
			mediaURL = dataURL("application/vnd.apple.mpegurl", mediaURL)
		} else {
			mediaURL = signOCSMediaURL(mediaURL, headers)
		}
		format := pickFormat(mediaURL)
		return extractor.Stream{Quality: "best", URLs: []string{mediaURL}, Format: format, Headers: headers, NeedMerge: format == "m3u8"}, map[string]any{"mode": "direct_ocs", "payload": payload}, true
	}
	if item, root, ok := findV55M3U8Item(payload); ok {
		content := textValue(item, "content", "m3u8", "text")
		if decoded := maybeDecodeText(content); decoded != "" {
			content = decoded
		}
		if !strings.HasPrefix(strings.TrimSpace(content), "#EXTM3U") {
			return extractor.Stream{}, nil, false
		}
		host := firstNonEmpty(candidateHosts(root)...)
		if host == "" {
			host = CCTALK_OCS_MATERIAL_HOST
		}
		playlist := rewriteV55M3U8Text(content, host, item)
		resourceID := firstNonEmpty(textValue(item, "resourceId"), textValue(item, "resourceID"), textValue(root, "resourceId"), textValue(root, "resourceID"))
		extra := map[string]any{
			"mode":              "v55",
			"m3u8_resource_id":  resourceID,
			"m3u8_text":         playlist,
			"payload":           payload,
			"decrypted_payload": root,
			"courseware_id":     firstNonEmpty(textValue(coursewareInfo, "coursewareId"), textValue(root, "coursewareId")),
		}
		stream := extractor.Stream{
			Quality:   firstNonEmpty(textValue(item, "quality"), textValue(item, "name"), textValue(item, "label"), "v55"),
			URLs:      []string{dataURL("application/vnd.apple.mpegurl", playlist)},
			Format:    "m3u8",
			Headers:   headers,
			NeedMerge: true,
		}
		return stream, extra, true
	}
	return extractor.Stream{}, nil, false
}

func normalizeOCSResourceURL(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(raw, `\/`, `/`), `\u0026`, "&"), "&amp;", "&"))
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "#EXTM3U") {
		return raw
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "/") {
		return normalizeMediaURL(raw)
	}
	if looksMediaURL(raw) || strings.Contains(raw, "/") {
		return strings.TrimRight(CCTALK_OCS_MATERIAL_HOST, "/") + "/" + strings.TrimLeft(raw, "/")
	}
	return normalizeMediaURL(raw)
}

func signOCSMediaURL(mediaURL string, headers map[string]string) string {
	lower := strings.ToLower(strings.TrimSpace(mediaURL))
	if mediaURL == "" || !(strings.Contains(lower, ".m3u8") || strings.Contains(lower, ".mp4")) {
		return mediaURL
	}
	userSign := firstNonEmpty(headers["X-User-Sign"], headers["X-User-sign"])
	if userSign == "" && !isOCSMediaURL(mediaURL) {
		return mediaURL
	}
	params := map[string]string{
		"X-User-Sign": userSign,
		"X-Tenant-ID": firstNonEmpty(headers["X-Tenant-ID"], headers["X-Tenant-Id"], CCTALK_TENANT_ID),
	}
	return appendQueryParams(mediaURL, params)
}

func isOCSMediaURL(raw string) bool {
	parsed, err := url.Parse(normalizeMediaURL(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Host)
	return strings.Contains(host, "hjfile.cn") || strings.Contains(host, "hjapi.com") || strings.Contains(host, "ocs")
}

func appendQueryParams(raw string, params map[string]string) string {
	if raw == "" || len(params) == 0 {
		return raw
	}
	sep := "?"
	if strings.Contains(raw, "?") {
		sep = "&"
	}
	var parts []string
	for key, value := range params {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		if strings.Contains(raw, url.QueryEscape(key)+"=") || strings.Contains(raw, key+"=") {
			continue
		}
		parts = append(parts, url.QueryEscape(key)+"="+url.QueryEscape(value))
	}
	if len(parts) == 0 {
		return raw
	}
	return raw + sep + strings.Join(parts, "&")
}

func normalizeOCSPayload(payload any) any {
	switch x := payload.(type) {
	case map[string]any:
		for _, key := range []string{"data", "Data", "result", "Result", "payload"} {
			if nested, ok := x[key]; ok && nested != nil {
				if normalized := normalizeOCSPayload(nested); normalized != nil {
					return normalized
				}
			}
		}
		return x
	case string:
		text := strings.TrimSpace(x)
		if decoded := maybeDecodeText(text); decoded != "" {
			text = decoded
		}
		if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
			var out any
			if json.Unmarshal([]byte(text), &out) == nil {
				return normalizeOCSPayload(out)
			}
		}
		return x
	default:
		return payload
	}
}

func findV55M3U8Item(payload any) (map[string]any, map[string]any, bool) {
	var foundItem map[string]any
	var foundRoot map[string]any
	var walk func(any, map[string]any)
	walk = func(value any, root map[string]any) {
		if foundItem != nil {
			return
		}
		switch x := value.(type) {
		case map[string]any:
			if root == nil {
				root = x
			}
			if content := textValue(x, "content", "m3u8", "text"); strings.HasPrefix(strings.TrimSpace(maybeDecodeFallback(content)), "#EXTM3U") {
				foundItem, foundRoot = x, root
				return
			}
			if list, ok := x["m3u8s"].([]any); ok {
				for _, item := range list {
					if m, ok := item.(map[string]any); ok {
						if content := textValue(m, "content", "m3u8", "text"); strings.HasPrefix(strings.TrimSpace(maybeDecodeFallback(content)), "#EXTM3U") {
							foundItem, foundRoot = m, x
							return
						}
					}
				}
			}
			for _, nested := range x {
				walk(nested, root)
			}
		case []any:
			for _, item := range x {
				walk(item, root)
			}
		}
	}
	walk(payload, nil)
	return foundItem, foundRoot, foundItem != nil
}

func maybeDecodeFallback(text string) string {
	if decoded := maybeDecodeText(text); decoded != "" {
		return decoded
	}
	return text
}

func maybeDecodeText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "#EXTM3U") || strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		return text
	}
	decoded, err := base64.StdEncoding.DecodeString(text)
	if err == nil {
		plain := strings.TrimSpace(string(decoded))
		if strings.HasPrefix(plain, "#EXTM3U") || strings.HasPrefix(plain, "{") || strings.HasPrefix(plain, "[") {
			return plain
		}
	}
	return ""
}

func candidateHosts(payload map[string]any) []string {
	var out []string
	seen := map[string]bool{}
	for _, key := range []string{"cdnHosts", "cdn_hosts", "hosts", "host", "cdnHost", "baseUrl", "baseURL", "materialHost"} {
		switch value := payload[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				host := strings.TrimRight(strings.TrimSpace(value), "/")
				if !seen[host] {
					seen[host] = true
					out = append(out, host)
				}
			}
		case []any:
			for _, item := range value {
				if s := strings.TrimSpace(textAny(item)); s != "" {
					host := strings.TrimRight(s, "/")
					if !seen[host] {
						seen[host] = true
						out = append(out, host)
					}
				}
			}
		}
	}
	if !seen[CCTALK_OCS_MATERIAL_HOST] {
		out = append(out, CCTALK_OCS_MATERIAL_HOST)
	}
	return out
}

func rewriteV55M3U8Text(content, host string, item map[string]any) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var out []string
	insertedKey := false
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if i == 0 && line != "#EXTM3U" {
			out = append(out, "#EXTM3U")
		}
		if line == "" {
			out = append(out, raw)
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-KEY") {
			insertedKey = true
			out = append(out, raw)
			continue
		}
		out = append(out, rewriteM3U8Line(raw, host))
		if i == 0 && !insertedKey {
			if keyLine := v55KeyLine(item); keyLine != "" {
				out = append(out, keyLine)
				insertedKey = true
			}
		}
	}
	return strings.Join(out, "\n")
}

func rewriteM3U8Line(raw, host string) string {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") || strings.HasPrefix(line, "data:") {
		return raw
	}
	if strings.HasPrefix(line, "//") {
		return "https:" + line
	}
	if host == "" {
		return raw
	}
	if strings.HasPrefix(line, "/") {
		return strings.TrimRight(host, "/") + line
	}
	return strings.TrimRight(host, "/") + "/" + strings.TrimLeft(path.Clean(line), "/")
}

func v55KeyLine(item map[string]any) string {
	keyText := firstNonEmpty(textValue(item, "key"), textValue(item, "cryptor"), textValue(item, "hlsKey"))
	keyBytes := decodeKeyBytes(keyText)
	if len(keyBytes) == 0 {
		return ""
	}
	iv := ivHex(firstNonEmpty(textValue(item, "iv"), textValue(item, "IV")))
	return `#EXT-X-KEY:METHOD=AES-128,URI="data:application/octet-stream;base64,` + base64.StdEncoding.EncodeToString(keyBytes) + `",IV=` + iv
}

func decodeKeyBytes(text string) []byte {
	text = strings.TrimSpace(strings.TrimPrefix(text, "0x"))
	if text == "" {
		return nil
	}
	if decoded, err := hex.DecodeString(text); err == nil && len(decoded) > 0 {
		return decoded
	}
	if decoded, err := base64.StdEncoding.DecodeString(text); err == nil && len(decoded) > 0 {
		return decoded
	}
	return []byte(text)
}

func ivHex(text string) string {
	text = strings.TrimSpace(strings.TrimPrefix(text, "0x"))
	if text == "" {
		return "0x00000000000000000000000000000000"
	}
	if _, err := hex.DecodeString(text); err == nil {
		return "0x" + strings.ToLower(text)
	}
	return "0x" + hex.EncodeToString([]byte(text))
}

func dataURL(mime, content string) string {
	return "data:" + mime + ";charset=utf-8," + url.PathEscape(content)
}

func playbackType(item map[string]any, extra map[string]any) string {
	for _, value := range []string{textValue(item, "playback_type"), textValue(item, "sourceType"), textValue(item, "contentType"), textAny(extra["mode"])} {
		lower := strings.ToLower(strings.TrimSpace(value))
		if lower == "board" || lower == "whiteboard" || strings.Contains(lower, "board") {
			return "board"
		}
	}
	if isBoardPayload(item) || textAny(extra["m3u8_resource_id"]) != "" && strings.Contains(strings.ToLower(textAny(extra["m3u8_resource_id"])), "board") {
		return "board"
	}
	return "video"
}

func isBoardPayload(value any) bool {
	switch x := value.(type) {
	case map[string]any:
		for _, key := range []string{"board", "whiteboard", "boards", "boardInfo", "boardResources"} {
			if x[key] != nil {
				return true
			}
		}
		for _, key := range []string{"sourceType", "contentType", "type", "playback_type"} {
			lower := strings.ToLower(textValue(x, key))
			if lower == "board" || lower == "whiteboard" || strings.Contains(lower, "board") {
				return true
			}
		}
		for _, nested := range x {
			if isBoardPayload(nested) {
				return true
			}
		}
	case []any:
		for _, item := range x {
			if isBoardPayload(item) {
				return true
			}
		}
	}
	return false
}

func textAny(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "<nil>" {
			return ""
		}
		return s
	}
}
