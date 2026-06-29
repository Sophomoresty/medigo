package htknow

import (
	"crypto/aes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func basePayload(userID, customID, productID string) map[string]any {
	return map[string]any{"product_version": "v1", "user_id": userID, "custom_id": customID, "version": "v1", "app_name": "wx", "product_id": productID}
}
func parseCourseID(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	q := u.Query()
	for _, k := range []string{"id", "course_id", "courseId", "product_id", "productId", "cid"} {
		if v := strings.TrimSpace(q.Get(k)); v != "" {
			return v
		}
	}
	parts := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' || r == '-' || r == '_' })
	for i := len(parts) - 1; i >= 0; i-- {
		if isDigits(parts[i]) {
			return parts[i]
		}
	}
	return ""
}
func cookieMap(jar http.CookieJar, bases []string) map[string]string {
	out := map[string]string{}
	for _, raw := range bases {
		u, _ := url.Parse(raw)
		for _, c := range jar.Cookies(u) {
			if _, ok := out[c.Name]; !ok {
				out[c.Name] = decodeCookie(c.Value)
			}
		}
	}
	return out
}
func cookieHeader(m map[string]string) string {
	var parts []string
	for k, v := range m {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}
func decodeCookie(s string) string {
	if u, err := url.QueryUnescape(s); err == nil {
		return u
	}
	return s
}
func userIDFromCookie(s string) string {
	var m map[string]any
	if json.Unmarshal([]byte(s), &m) == nil {
		return str(m["id"])
	}
	return ""
}
func accountIDs(cookies map[string]string, fallback string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	add(fallback)
	for _, k := range []string{"wechatList", "appletList", "ksList"} {
		var arr []map[string]any
		if json.Unmarshal([]byte(cookies[k]), &arr) != nil {
			continue
		}
		for _, a := range arr {
			add(str(a["id"]))
			for _, child := range listAt(a, "child_list") {
				add(str(child["id"]))
			}
		}
	}
	return out
}
func decodeB64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty base64")
	}
	s += strings.Repeat("=", (4-len(s)%4)%4)
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.URLEncoding.DecodeString(s)
}
func pkcs7Unpad(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	n := int(b[len(b)-1])
	if n <= 0 || n > aes.BlockSize || n > len(b) {
		return b
	}
	for _, v := range b[len(b)-n:] {
		if int(v) != n {
			return b
		}
	}
	return b[:len(b)-n]
}
func mapAt(m map[string]any, keys ...string) map[string]any {
	cur := m
	for _, k := range keys {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return map[string]any{}
		}
		cur = next
	}
	return cur
}
func strAt(m map[string]any, keys ...string) string {
	if len(keys) == 0 {
		return ""
	}
	mm := mapAt(m, keys[:len(keys)-1]...)
	return str(mm[keys[len(keys)-1]])
}
func listAt(m map[string]any, keys ...string) []map[string]any {
	v := any(m)
	for _, k := range keys {
		mm, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = mm[k]
	}
	arr, ok := v.([]any)
	if !ok {
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
func str(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.0f", t), "0"), ".")
	default:
		if v == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
func intVal(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	case string:
		if t == "200" {
			return 200
		}
	}
	return 0
}
func trimMP4(s string) string { return strings.TrimSuffix(strings.TrimSpace(s), ".mp4") }
func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
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
