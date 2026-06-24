package lizhiweike

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func lizhiDefinitionRank(m map[string]any) int {
	ranks := map[string]int{"原画": 5, "超清": 4, "高清": 3, "标清": 2, "流畅": 1}
	return ranks[firstText(m["definition"])]
}

func lizhiCookieString(jar http.CookieJar) string {
	hosts := []string{"m.lizhiweike.com", "apiv1.lizhiweike.com", "open.lizhiweike.com", "lizhiweike.com", "tenexer.cn", "szbaimao.com", "shifangfm.com", "shifangwk.cn", "xrcox.cn", "ckkzk.cn", "tenclass.cn", "liveweike.com"}
	seen, parts := map[string]bool{}, []string{}
	for _, h := range hosts {
		for _, ck := range jar.Cookies(&url.URL{Scheme: "https", Host: h}) {
			if ck.Value != "" && !seen[ck.Name] {
				seen[ck.Name] = true
				parts = append(parts, ck.Name+"="+ck.Value)
			}
		}
	}
	return strings.Join(parts, "; ")
}

func cookieValue(cookie, name string) string {
	for _, p := range strings.Split(cookie, ";") {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) == 2 && strings.EqualFold(kv[0], name) {
			return kv[1]
		}
	}
	return ""
}

func records(v any) []map[string]any {
	switch x := v.(type) {
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, it := range x {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

func mapAny(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}
func nested(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		cur = mapAny(cur)[k]
	}
	return cur
}
func nestedText(m map[string]any, keys ...string) string { return firstText(nested(m, keys...)) }
func boolOf(v any) bool                                  { b, _ := v.(bool); return b }
func intOf(v any) int                                    { return int(numOf(v)) }
func numOf(v any) float64                                { f, _ := strconv.ParseFloat(firstText(v), 64); return f }
func firstText(vals ...any) string {
	for _, v := range vals {
		if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}
func mediaExt(u string) string {
	u = strings.ToLower(u)
	if strings.Contains(u, ".m3u8") {
		return "m3u8"
	}
	if strings.Contains(u, ".mp3") {
		return "mp3"
	}
	return "mp4"
}
