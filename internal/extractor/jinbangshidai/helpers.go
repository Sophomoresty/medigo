package jinbangshidai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
)

func normalizeCourseInfo(m map[string]any) courseInfo {
	if m == nil {
		return courseInfo{}
	}
	cid := pickText(m["cid"], m["courseId"])
	title := cleanTitle(pickText(m["name"], m["title"]))
	return courseInfo{CourseID: cid, Title: title, Price: pickNumber(m["price"], m["oldPrice"], m["realPrice"]), Raw: m}
}

func collectSyllabusResources(nodeList []map[string]any, prefix []int) []resourceInfo {
	var out []resourceInfo
	for i, node := range nodeList {
		if node == nil {
			continue
		}
		nextPrefix := append(append([]int{}, prefix...), i+1)
		if children := listAt(node, "list"); len(children) > 0 {
			out = append(out, collectSyllabusResources(children, nextPrefix)...)
			continue
		}
		if typeIn(node["type"], 3, 4, 5) {
			out = append(out, makeResourceInfo(node, nextPrefix))
		}
	}
	return out
}

func makeResourceInfo(node map[string]any, inxTuple []int) resourceInfo {
	name := pickText(node["name"], node["title"], node["docName"], "课程资源")
	if typ := intVal(node["type"]); typ == 3 || typ == 5 {
		base, ext := splitExt(name)
		if ext != "" && isMediaExt(ext) && base != "" {
			name = base
		}
	}
	return resourceInfo{
		Name:         cleanTitle(fmt.Sprintf("[%s]--%s", joinInts(inxTuple, "."), name)),
		VideoID:      pickText(node["url"], node["videoId"], node["roomString"]),
		ResourceType: intVal(node["type"]),
		Prefix:       joinInts(inxTuple, "."),
		Source:       node,
	}
}

func materialFiles(r resourceInfo) []fileInfo {
	if r.ResourceType != 4 {
		return nil
	}
	var out []fileInfo
	for _, key := range []string{"docUrl", "url", "fileUrl", "file_url"} {
		raw := pickText(r.Source[key])
		if u := normalizeMediaURL(raw); u != "" {
			out = append(out, fileInfo{Name: r.Name, URL: u, Fmt: guessMaterialExt(u, r.Source["name"], r.Source["docName"])})
		}
	}
	if len(out) == 0 {
		if u := normalizeMediaURL(pickText(r.Source["docUrl"], r.Source["url"])); u != "" {
			out = append(out, fileInfo{Name: r.Name, URL: u, Fmt: guessMaterialExt(u, r.Source["name"], r.Source["docName"])})
		}
	}
	return out
}

func dedupeFiles(files []fileInfo) []fileInfo {
	seen := map[string]bool{}
	var out []fileInfo
	for _, f := range files {
		key := f.URL + "|" + f.Name
		if f.URL != "" && !seen[key] {
			seen[key] = true
			out = append(out, f)
		}
	}
	return out
}

func parseCID(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	for _, k := range []string{"courseId", "id", "cid", "course_id"} {
		if v := strings.TrimSpace(u.Query().Get(k)); v != "" {
			return v
		}
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if isDigits(parts[i]) {
			return parts[i]
		}
	}
	return ""
}

func extractCookieToken(cookie string) string {
	consts := []string{"ToKen", "token", "Token", "TOKEN"}
	cookie = strings.Trim(strings.TrimSpace(cookie), `"`)
	if cookie == "" {
		return ""
	}
	if cookie[0] == '{' || cookie[0] == '[' {
		var anyv any
		if json.Unmarshal([]byte(cookie), &anyv) == nil {
			if v := extractTokenFromAny(anyv, consts); v != "" {
				return v
			}
		}
	}
	if strings.Count(cookie, ".") >= 2 && !strings.Contains(strings.SplitN(cookie, ";", 2)[0], "=") {
		return cookie
	}
	for _, part := range strings.Split(cookie, ";") {
		if !strings.Contains(part, "=") {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		for _, want := range consts {
			if k == want && v != "" {
				return v
			}
		}
	}
	return ""
}

func extractTokenFromAny(v any, wants []string) string {
	switch t := v.(type) {
	case map[string]any:
		for _, want := range wants {
			if s := pickText(t[want]); s != "" {
				return s
			}
		}
		for _, vv := range t {
			if s := extractTokenFromAny(vv, wants); s != "" {
				return s
			}
		}
	case []any:
		for _, vv := range t {
			if s := extractTokenFromAny(vv, wants); s != "" {
				return s
			}
		}
	}
	return ""
}

func decodeJWTPayload(token string) map[string]any {
	token = strings.TrimSpace(token)
	if token == "" {
		return map[string]any{}
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return map[string]any{}
	}
	payload := parts[1]
	payload += strings.Repeat("=", (4-len(payload)%4)%4)
	raw, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		raw, err = base64.StdEncoding.DecodeString(payload)
	}
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return map[string]any{}
	}
	return out
}

func pickText(values ...any) string {
	for _, v := range values {
		if s := str(v); s != "" {
			return s
		}
	}
	return ""
}

func pickNumber(values ...any) float64 {
	for _, v := range values {
		switch t := v.(type) {
		case float64:
			if t != 0 {
				return t
			}
		case json.Number:
			if n, err := t.Float64(); err == nil && n != 0 {
				return n
			}
		case string:
			if t == "" {
				continue
			}
			if n, err := strconvParseFloat(t); err == nil && n != 0 {
				return n
			}
		default:
			if s := str(t); s != "" {
				if n, err := strconvParseFloat(s); err == nil && n != 0 {
					return n
				}
			}
		}
	}
	return 0
}

func typeIn(v any, values ...int) bool {
	n := intVal(v)
	for _, want := range values {
		if n == want {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func listAt(m map[string]any, key string) []map[string]any {
	if m == nil {
		return nil
	}
	raw, ok := m[key]
	if !ok {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		if sub, ok := raw.([]map[string]any); ok {
			return sub
		}
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, x := range arr {
		if mm, ok := x.(map[string]any); ok {
			out = append(out, mm)
		}
	}
	return out
}

func mapAt(m map[string]any, key string) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	if sub, ok := m[key].(map[string]any); ok {
		return sub
	}
	return map[string]any{}
}

func strAt(m map[string]any, keys ...string) string {
	cur := map[string]any(m)
	for i, key := range keys {
		if i == len(keys)-1 {
			return str(cur[key])
		}
		cur = mapAt(cur, key)
	}
	return ""
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.0f", t), "0"), ".")
	case int:
		return fmt.Sprint(t)
	case int64:
		return fmt.Sprint(t)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func intVal(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	case string:
		n, _ := strconvParseFloat(t)
		return int(n)
	default:
		return 0
	}
}

func cleanTitle(s string) string {
	s = html.UnescapeString(s)
	s = regexp.MustCompile(`(?is)<.*?>`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func joinInts(xs []int, sep string) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = fmt.Sprint(x)
	}
	return strings.Join(parts, sep)
}

func splitExt(name string) (string, string) {
	ext := strings.ToLower(path.Ext(name))
	if ext == "" {
		return name, ""
	}
	return strings.TrimSuffix(name, ext), ext
}

func extFormat(raw string) string {
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(strings.Split(raw, "?")[0])), ".")
	if ext != "" {
		return ext
	}
	return ""
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isMediaExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".mp4", ".flv", ".m3u8", ".mp3", ".m4a", ".ev1", ".ev2":
		return true
	default:
		return false
	}
}

func normalizeMediaURL(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\/`, `/`))
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	return ""
}

func guessMaterialExt(raw any, nameVals ...any) string {
	if u := str(raw); u != "" {
		if ext := strings.ToLower(path.Ext(strings.Split(u, "?")[0])); ext != "" {
			return strings.TrimPrefix(ext, ".")
		}
	}
	for _, v := range nameVals {
		if s := str(v); s != "" {
			if ext := strings.ToLower(path.Ext(s)); ext != "" {
				return strings.TrimPrefix(ext, ".")
			}
		}
	}
	return "html"
}

func cloneHeaders(h map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range h {
		out[k] = v
	}
	return out
}

func cookieHeader(jar http.CookieJar, origins []string) string {
	seen := map[string]bool{}
	var parts []string
	for _, raw := range origins {
		u, _ := url.Parse(raw)
		for _, c := range jar.Cookies(u) {
			key := c.Name + "=" + c.Value
			if !seen[key] {
				seen[key] = true
				parts = append(parts, key)
			}
		}
	}
	return strings.Join(parts, "; ")
}

func dedupeResources(resources []resourceInfo) []resourceInfo {
	seen := map[string]bool{}
	var out []resourceInfo
	for _, r := range resources {
		key := fmt.Sprintf("%d|%s|%s", r.ResourceType, r.VideoID, r.Name)
		if r.VideoID != "" || r.ResourceType == 4 {
			if !seen[key] {
				seen[key] = true
				out = append(out, r)
			}
		}
	}
	return out
}

func strconvParseFloat(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	return json.Number(s).Float64()
}
