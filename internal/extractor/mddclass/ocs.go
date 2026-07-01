package mddclass

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

var mddclassBareMediaPathRe = regexp.MustCompile(`(?i)(?:^|/)[^"'\s<>]+\.(?:m3u8|mp4|flv|ts)(?:[?#].*)?$`)

func mddclassResolveOCSMedia(c *util.Client, sess *mddclassSession, video mddclassVideo, coursewareInfo map[string]any, values ...any) (extractor.Stream, map[string]any, bool) {
	headers := sess.ocsMediaHeaders(video, coursewareInfo)
	if direct := mddclassResolveOCSMediaURL(sess, values...); direct != "" {
		format := mddclassStreamFormat(direct)
		return extractor.Stream{Quality: "best", URLs: []string{direct}, Format: format, NeedMerge: format == "m3u8", Headers: headers}, map[string]any{"mode": "direct_ocs", "courseware_info": coursewareInfo}, true
	}
	if stream, extra, ok := mddclassBuildOCSWhiteboardStream(coursewareInfo, coursewareInfo, headers); ok {
		extra["mode"] = mddclassFirstText(extra["mode"], "board")
		if coursewareID := mddclassFirstText(coursewareInfo["coursewareId"], coursewareInfo["courseware_id"], coursewareInfo["courseWareId"], coursewareInfo["ocsId"], coursewareInfo["ocs_id"]); coursewareID != "" {
			extra["courseware_id"] = coursewareID
		}
		return stream, extra, true
	}
	if c == nil || sess == nil {
		return extractor.Stream{}, nil, false
	}
	coursewareID := mddclassFirstText(coursewareInfo["coursewareId"], coursewareInfo["courseware_id"], coursewareInfo["courseWareId"], coursewareInfo["ocsId"], coursewareInfo["ocs_id"])
	if coursewareID == "" {
		return extractor.Stream{}, nil, false
	}
	if mddclassFirstText(coursewareInfo["tenantId"], coursewareInfo["tenant_id"], sess.Auth["tenantId"], sess.Auth["tenant_id"]) == "" {
		coursewareInfo["tenantId"] = mddclassTenantID
	}
	candidates := mddclassOCSEndpoints(coursewareID, coursewareInfo)
	for _, candidate := range candidates {
		body, err := mddclassFetchOCSBody(c, candidate, headers)
		if err != nil || strings.TrimSpace(body) == "" {
			continue
		}
		var payload any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			continue
		}
		if stream, extra, ok := mddclassBuildOCSStream(c, candidate, payload, coursewareInfo, headers); ok {
			extra["mode"] = mddclassFirstText(extra["mode"], "ocs_api")
			extra["ocs_endpoint"] = candidate
			extra["courseware_id"] = coursewareID
			return stream, extra, true
		}
	}
	return extractor.Stream{}, nil, false
}

func mddclassFetchOCSBody(c *util.Client, endpoint string, headers map[string]string) (string, error) {
	if strings.Contains(endpoint, "courseware-ocs.sksight.com") {
		endpoint = mddclassRewriteOCSCDNHost(endpoint, "r1-ndr.ykt.cbern.com.cn")
	}
	body, err := c.GetString(endpoint, headers)
	if err == nil && strings.TrimSpace(body) != "" {
		return body, nil
	}
	var lastErr error = err
	for _, host := range []string{"r2-ndr.ykt.cbern.com.cn", "courseware-ocs.sksight.com"} {
		candidate := mddclassRewriteOCSCDNHost(endpoint, host)
		if candidate == endpoint {
			continue
		}
		body, err = c.GetString(candidate, headers)
		if err == nil && strings.TrimSpace(body) != "" {
			return body, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return body, nil
}

func mddclassRewriteOCSCDNHost(raw, host string) string {
	if raw == "" || host == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	if !strings.Contains(strings.ToLower(u.Hostname()), "courseware-ocs.sksight.com") && !strings.Contains(strings.ToLower(u.Hostname()), "r1-ndr.ykt.cbern.com.cn") && !strings.Contains(strings.ToLower(u.Hostname()), "r2-ndr.ykt.cbern.com.cn") {
		return raw
	}
	u.Host = host
	if strings.EqualFold(u.Scheme, "http") {
		u.Scheme = "https"
	}
	return u.String()
}

func mddclassOCSEndpoints(coursewareID string, coursewareInfo map[string]any) []string {
	escaped := url.PathEscape(coursewareID)
	out := []string{}
	for _, base := range []string{
		"https://courseware-ocs.sksight.com/v5.5/",
		mddclassOCSBase,
		"https://courseware-ocs.sksight.com/v5/",
		"https://courseware-ocs.sksight.com/",
	} {
		endpoint := strings.TrimRight(base, "/") + "/courseware_contents/" + escaped
		if q := mddclassOCSQuery(coursewareInfo); q != "" {
			endpoint += "?" + q
		}
		out = append(out, endpoint)
		for part := 1; part <= 8; part++ {
			q := url.Values{}
			q.Set("key", "1")
			q.Set("part", fmt.Sprint(part))
			for key, value := range mddclassOCSQueryValues(coursewareInfo) {
				q.Set(key, value)
			}
			out = append(out, strings.TrimRight(base, "/")+"/courseware_contents/"+escaped+"?"+q.Encode())
		}
	}
	out = append(out, strings.TrimRight(mddclassOCSAPIHost, "/")+"/v5/courseware_contents/h5/"+escaped+"?t="+fmt.Sprint(time.Now().UnixMilli()))
	return mddclassUniqueRawStrings(out)
}

func mddclassUniqueRawStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func mddclassOCSQuery(coursewareInfo map[string]any) string {
	values := url.Values{}
	for k, v := range mddclassOCSQueryValues(coursewareInfo) {
		values.Set(k, v)
	}
	return values.Encode()
}

func mddclassOCSQueryValues(coursewareInfo map[string]any) map[string]string {
	out := map[string]string{}
	for _, key := range []string{"tenantId", "sourceType", "contentType"} {
		if value := mddclassFirstText(coursewareInfo[key], coursewareInfo[strings.ToLower(key)]); value != "" {
			out[key] = value
		}
	}
	if value := mddclassFirstText(coursewareInfo["userSign"], coursewareInfo["user_sign"], coursewareInfo["userSignKey"], coursewareInfo["user_sign_key"]); value != "" {
		out["userSign"] = value
	}
	return out
}

func mddclassBuildOCSStream(args ...any) (extractor.Stream, map[string]any, bool) {
	var c *util.Client
	var baseURL string
	var payload any
	var coursewareInfo map[string]any
	var headers map[string]string
	if len(args) == 3 {
		payload = args[0]
		coursewareInfo, _ = args[1].(map[string]any)
		headers, _ = args[2].(map[string]string)
	} else if len(args) >= 5 {
		c, _ = args[0].(*util.Client)
		baseURL, _ = args[1].(string)
		payload = args[2]
		coursewareInfo, _ = args[3].(map[string]any)
		headers, _ = args[4].(map[string]string)
		_ = c
		_ = baseURL
	}
	if coursewareInfo == nil {
		coursewareInfo = map[string]any{}
	}
	if headers == nil {
		headers = map[string]string{}
	}
	payload = mddclassNormalizeOCSPayload(payload)
	if stream, extra, ok := mddclassBuildOCSWhiteboardStream(payload, coursewareInfo, headers); ok {
		return stream, extra, true
	}
	if mediaURL := mddclassNormalizeMediaURL(mddclassFindMediaURL(payload)); mediaURL != "" && !mddclassIsPlaceholderURL(mediaURL) {
		mediaURL = mddclassSignOCSMediaURLWithHeaders(mediaURL, headers)
		format := mddclassStreamFormat(mediaURL)
		return extractor.Stream{Quality: "best", URLs: []string{mediaURL}, Format: format, NeedMerge: format == "m3u8", Headers: headers}, map[string]any{"mode": "direct_ocs", "payload": payload}, true
	}
	if item, root, ok := mddclassFindOCSM3U8Item(payload); ok {
		content := mddclassMaybeDecodeText(mddclassFirstText(item["content"], item["m3u8"], item["text"]))
		if !strings.HasPrefix(strings.TrimSpace(content), "#EXTM3U") {
			return extractor.Stream{}, nil, false
		}
		host := mddclassFirstText(mddclassCandidateHosts(root)...)
		if host == "" {
			host = mddclassOCSMaterialHost
		}
		playlist := mddclassRewriteM3U8(content, host, item)
		stream := extractor.Stream{
			Quality:   mddclassFirstText(item["quality"], item["name"], item["label"], "ocs"),
			URLs:      []string{mddclassDataURL("application/vnd.apple.mpegurl", playlist)},
			Format:    "m3u8",
			NeedMerge: true,
			Headers:   headers,
		}
		return stream, map[string]any{"mode": "v55", "m3u8_text": playlist, "payload": payload, "decrypted_payload": root, "courseware_info": coursewareInfo}, true
	}
	return extractor.Stream{}, nil, false
}

func mddclassNormalizeOCSPayload(payload any) any {
	switch x := payload.(type) {
	case map[string]any:
		for _, key := range []string{"data", "Data", "result", "Result", "payload"} {
			if nested, ok := x[key]; ok && nested != nil {
				if normalized := mddclassNormalizeOCSPayload(nested); normalized != nil {
					return normalized
				}
			}
		}
		return x
	case string:
		text := strings.TrimSpace(x)
		if decoded := mddclassMaybeDecodeText(text); decoded != "" {
			text = decoded
		}
		if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
			var out any
			if json.Unmarshal([]byte(text), &out) == nil {
				return mddclassNormalizeOCSPayload(out)
			}
		}
		return x
	default:
		return payload
	}
}

func mddclassFindOCSM3U8Item(payload any) (map[string]any, map[string]any, bool) {
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
			if content := mddclassMaybeDecodeText(mddclassFirstText(x["content"], x["m3u8"], x["text"])); strings.HasPrefix(strings.TrimSpace(content), "#EXTM3U") {
				foundItem, foundRoot = x, root
				return
			}
			if list, ok := x["m3u8s"].([]any); ok {
				for _, item := range list {
					if m, ok := item.(map[string]any); ok {
						if content := mddclassMaybeDecodeText(mddclassFirstText(m["content"], m["m3u8"], m["text"])); strings.HasPrefix(strings.TrimSpace(content), "#EXTM3U") {
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

func mddclassMaybeDecodeText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "#EXTM3U") || strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		return text
	}
	if decoded, err := base64.StdEncoding.DecodeString(text); err == nil {
		plain := strings.TrimSpace(string(decoded))
		if strings.HasPrefix(plain, "#EXTM3U") || strings.HasPrefix(plain, "{") || strings.HasPrefix(plain, "[") {
			return plain
		}
	}
	return text
}

func mddclassCandidateHosts(payload map[string]any) []any {
	out := []any{}
	for _, key := range []string{"cdnHosts", "cdn_hosts", "hosts", "host", "cdnHost", "baseUrl", "baseURL", "materialHost"} {
		switch value := payload[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				out = append(out, strings.TrimRight(strings.TrimSpace(value), "/"))
			}
		case []any:
			for _, item := range value {
				if s := mddclassFirstText(item); s != "" {
					out = append(out, strings.TrimRight(s, "/"))
				}
			}
		}
	}
	return out
}

func mddclassPreferredOCSMaterialHost(payload any) string {
	host := ""
	var walk func(any, int)
	walk = func(value any, depth int) {
		if host != "" || value == nil || depth > 6 {
			return
		}
		switch x := value.(type) {
		case map[string]any:
			if candidate := mddclassFirstText(mddclassCandidateHosts(x)...); candidate != "" {
				host = candidate
				return
			}
			for _, nested := range x {
				walk(nested, depth+1)
			}
		case []any:
			for _, item := range x {
				walk(item, depth+1)
			}
		}
	}
	walk(payload, 0)
	if host == "" {
		host = mddclassOCSMaterialHost
	}
	return strings.TrimRight(host, "/")
}

func mddclassRewriteM3U8(content, host string, item map[string]any) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines)+1)
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
		out = append(out, mddclassRewriteM3U8Line(raw, host))
		if i == 0 && !insertedKey {
			if keyLine := mddclassV55KeyLine(item); keyLine != "" {
				out = append(out, keyLine)
				insertedKey = true
			}
		}
	}
	return strings.Join(out, "\n")
}

func mddclassRewriteM3U8Line(raw, host string) string {
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

func mddclassV55KeyLine(item map[string]any) string {
	keyText := mddclassFirstText(item["key"], item["cryptor"], item["hlsKey"])
	keyBytes := mddclassDecodeKeyBytes(keyText)
	if len(keyBytes) == 0 {
		return ""
	}
	iv := mddclassIVHex(mddclassFirstText(item["iv"], item["IV"]))
	return `#EXT-X-KEY:METHOD=AES-128,URI="data:application/octet-stream;base64,` + base64.StdEncoding.EncodeToString(keyBytes) + `",IV=` + iv
}

func mddclassDecodeKeyBytes(text string) []byte {
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

func mddclassIVHex(text string) string {
	text = strings.TrimSpace(strings.TrimPrefix(text, "0x"))
	if text == "" {
		return "0x00000000000000000000000000000000"
	}
	if _, err := hex.DecodeString(text); err == nil {
		return "0x" + strings.ToLower(text)
	}
	return "0x" + hex.EncodeToString([]byte(text))
}

func mddclassDataURL(mime, content string) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString([]byte(content))
}

func mddclassResolveOCSMediaURL(sess *mddclassSession, values ...any) string {
	for _, candidate := range mddclassCollectOCSMediaCandidates(values...) {
		mediaURL := mddclassNormalizeOCSResourceURL(candidate)
		if mediaURL == "" || mddclassIsPlaceholderURL(mediaURL) || !mddclassLooksLikeMediaURL(mediaURL) {
			continue
		}
		return mddclassSignOCSMediaURL(sess, mediaURL)
	}
	return ""
}

func mddclassCollectOCSMediaCandidates(values ...any) []string {
	seen := map[string]bool{}
	var out []string
	var walk func(any, int)
	walk = func(value any, depth int) {
		if depth > 8 || value == nil {
			return
		}
		switch v := value.(type) {
		case map[string]any:
			for _, key := range []string{
				"media_url", "mediaUrl", "mediaURL", "video_url", "videoUrl",
				"play_url", "playUrl", "hls_url", "hlsUrl", "m3u8_url", "m3u8Url",
				"mp4_url", "mp4Url", "download_url", "downloadUrl", "source_url",
				"sourceUrl", "resource_url", "resourceUrl", "file_url", "fileUrl",
				"url", "href",
				"path", "filePath", "mediaPath", "resourcePath", "sourcePath",
				"playPath", "hlsPath", "m3u8Path", "mp4Path", "objectKey", "key",
			} {
				if raw := mddclassFirstText(v[key]); raw != "" {
					addOCSCandidate(raw, seen, &out)
				}
			}
			for _, key := range []string{"cdnHosts", "cdn_hosts", "hosts"} {
				if hosts, ok := v[key].([]any); ok {
					for _, host := range hosts {
						addOCSCandidate(mddclassFirstText(host), seen, &out)
					}
				}
			}
			for _, nested := range v {
				walk(nested, depth+1)
			}
		case []any:
			for _, item := range v {
				walk(item, depth+1)
			}
		case string:
			addOCSCandidate(v, seen, &out)
		}
	}
	for _, value := range values {
		walk(value, 0)
	}
	return out
}

func addOCSCandidate(raw string, seen map[string]bool, out *[]string) {
	raw = strings.TrimSpace(strings.Trim(raw, `"'`))
	raw = strings.ReplaceAll(raw, `\u0026`, "&")
	raw = strings.ReplaceAll(raw, `\/`, `/`)
	raw = strings.ReplaceAll(raw, "&amp;", "&")
	if raw == "" || seen[raw] {
		return
	}
	if strings.Contains(strings.ToLower(raw), "courseware-ocs") ||
		strings.Contains(strings.ToLower(raw), "p1-ocs") ||
		mddclassLooksLikeMediaURL(raw) ||
		mddclassBareMediaPathRe.MatchString(raw) {
		seen[raw] = true
		*out = append(*out, raw)
	}
}

func mddclassNormalizeOCSResourceURL(raw string) string {
	raw = mddclassNormalizeMediaURL(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	if strings.HasPrefix(raw, "/") {
		return strings.TrimRight(mddclassOCSMaterialHost, "/") + raw
	}
	if strings.Contains(raw, "/") || mddclassBareMediaPathRe.MatchString(raw) {
		return strings.TrimRight(mddclassOCSMaterialHost, "/") + "/" + strings.TrimLeft(raw, "/")
	}
	return ""
}

func mddclassNormalizeOCSResourceURLWithHost(raw, materialHost string) string {
	raw = mddclassNormalizeMediaURL(raw)
	raw = strings.TrimSpace(strings.Trim(raw, `"'`))
	raw = strings.ReplaceAll(raw, `\u0026`, "&")
	raw = strings.ReplaceAll(raw, `\/`, `/`)
	raw = strings.ReplaceAll(raw, "&amp;", "&")
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "data:") {
		return raw
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	if materialHost == "" {
		materialHost = mddclassOCSMaterialHost
	}
	return strings.TrimRight(materialHost, "/") + "/" + strings.TrimLeft(raw, "/")
}

func mddclassSignOCSMediaURL(sess *mddclassSession, raw string) string {
	if sess == nil || raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if strings.EqualFold(u.Scheme, "data") {
		return raw
	}
	q := u.Query()
	if token := mddclassFirstText(sess.Auth["ocsAccessToken"], sess.Auth["ocs_access_token"], sess.Auth["ocsPlayerAccessToken"], sess.Auth["playerAccessToken"]); token != "" {
		for _, key := range []string{"accessToken", "ocsAccessToken", "token"} {
			if q.Get(key) == "" {
				q.Set(key, token)
				break
			}
		}
	}
	if tenant := mddclassFirstText(sess.Auth["tenantId"], sess.Auth["tenant_id"]); tenant != "" && q.Get("tenantId") == "" {
		q.Set("tenantId", tenant)
	}
	if sign := mddclassFirstText(sess.Auth["userSign"], sess.Auth["user_sign"]); sign != "" && q.Get("userSign") == "" {
		q.Set("userSign", sign)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func mddclassSignOCSMediaURLWithHeaders(raw string, headers map[string]string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || strings.EqualFold(u.Scheme, "data") {
		return raw
	}
	q := u.Query()
	if sign := mddclassFirstText(headers["X-User-Sign"], headers["userSign"], headers["User-Sign"]); sign != "" && q.Get("userSign") == "" {
		q.Set("userSign", sign)
	}
	if tenant := mddclassFirstText(headers["X-Tenant-ID"], headers["X-Tenant-Id"], headers["tenantId"]); tenant != "" && q.Get("tenantId") == "" {
		q.Set("tenantId", tenant)
	}
	if token := mddclassFirstText(headers["Authorization"], headers["AccessToken"], headers["X-Access-Token"]); token != "" && q.Get("accessToken") == "" {
		q.Set("accessToken", token)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func mddclassIsOCSURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return strings.Contains(host, "courseware-ocs") ||
		strings.Contains(host, "p1-ocs") ||
		strings.Contains(host, "r1-ndr.ykt.cbern.com.cn") ||
		strings.Contains(host, "r2-ndr.ykt.cbern.com.cn") ||
		strings.Contains(host, "sksight.com") && strings.Contains(strings.ToLower(u.Path), "ocs")
}

func mddclassIsOCSEndpointURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return strings.Contains(host, "courseware-ocs.sksight.com") ||
		strings.Contains(host, "r1-ndr.ykt.cbern.com.cn") ||
		strings.Contains(host, "r2-ndr.ykt.cbern.com.cn") ||
		strings.Contains(host, "courseware-ocs-api.sksight.com")
}

func mddclassFetchOCSJSON(c *util.Client, raw string, headers map[string]string) (any, string, bool) {
	if c == nil || raw == "" || !mddclassIsOCSEndpointURL(raw) {
		return nil, "", false
	}
	body, err := mddclassFetchOCSBody(c, raw, headers)
	if err != nil || strings.TrimSpace(body) == "" {
		return nil, "", false
	}
	var payload any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil, "", false
	}
	return mddclassNormalizeOCSPayload(payload), raw, true
}

func mddclassHTTPGetBytes(raw string, headers map[string]string) ([]byte, bool) {
	if raw == "" || strings.HasPrefix(strings.ToLower(raw), "data:") {
		return nil, false
	}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return nil, false
	}
	for k, v := range headers {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, false
	}
	data, err := io.ReadAll(resp.Body)
	return data, err == nil && len(data) > 0
}

func mddclassOCSHint(sess *mddclassSession, coursewareInfo map[string]any) string {
	if sess == nil {
		return ""
	}
	hasOCSMarker := false
	for _, key := range []string{"coursewareId", "courseware_id", "courseWareId", "ocsId", "ocs_id", "tenantId", "tenant_id", "userSign", "user_sign", "userSignKey", "user_sign_key", "ocsAccessToken", "ocs_access_token"} {
		if mddclassFirstText(coursewareInfo[key]) != "" {
			hasOCSMarker = true
			break
		}
	}
	if !hasOCSMarker {
		return ""
	}
	missing := []string{}
	if mddclassFirstText(coursewareInfo["coursewareId"], coursewareInfo["courseware_id"], coursewareInfo["courseWareId"], coursewareInfo["ocsId"], coursewareInfo["ocs_id"]) == "" {
		missing = append(missing, "coursewareId")
	}
	if mddclassFirstText(coursewareInfo["tenantId"], coursewareInfo["tenant_id"], sess.Auth["tenantId"], sess.Auth["tenant_id"]) == "" {
		missing = append(missing, "tenantId")
	}
	if mddclassFirstText(coursewareInfo["userSign"], coursewareInfo["user_sign"], sess.Auth["userSign"], sess.Auth["user_sign"]) == "" {
		missing = append(missing, "userSign")
	}
	if len(missing) > 0 {
		return fmt.Sprintf("OCS courseware at %s requires %s", mddclassOCSReferer, strings.Join(missing, ", "))
	}
	return fmt.Sprintf("OCS courseware metadata is present; direct media URL is still absent after parsing %s", mddclassOCSReferer)
}
