package huatu

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

func cookieHeader(jar http.CookieJar, origins []string) string {
	seen := map[string]bool{}
	var parts []string
	for _, origin := range origins {
		u, err := url.Parse(origin)
		if err != nil {
			continue
		}
		for _, c := range jar.Cookies(u) {
			if c.Name == "" || seen[c.Name] {
				continue
			}
			seen[c.Name] = true
			parts = append(parts, c.Name+"="+c.Value)
		}
	}
	return strings.Join(parts, "; ")
}

func parseCookieHeader(cookie string) map[string]string {
	out := map[string]string{}
	for _, p := range strings.Split(cookie, ";") {
		p = strings.TrimSpace(p)
		if p == "" || !strings.Contains(p, "=") {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		out[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}
	return out
}

func cookieToken(cookie string) string {
	m := parseCookieHeader(cookie)
	return firstNonEmpty(m["ht_token"], m["ht_token_preview"], m["Newuc-Token"], m["token"])
}

func normalizeCookieTokenAliases(cookie string) string {
	m := parseCookieHeader(cookie)
	token := firstNonEmpty(m["ht_token"], m["ht_token_preview"], m["Newuc-Token"], m["token"])
	if token == "" {
		return cookie
	}
	for _, k := range []string{"ht_token", "ht_token_preview", "Newuc-Token", "token"} {
		if m[k] == "" {
			m[k] = token
		}
	}
	var parts []string
	seen := map[string]bool{}
	for _, p := range strings.Split(cookie, ";") {
		p = strings.TrimSpace(p)
		if p == "" || !strings.Contains(p, "=") {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		k := strings.TrimSpace(kv[0])
		if m[k] != "" && !seen[k] {
			parts = append(parts, k+"="+m[k])
			seen[k] = true
		}
	}
	for _, k := range []string{"ht_token", "ht_token_preview", "Newuc-Token", "token"} {
		if !seen[k] && m[k] != "" {
			parts = append(parts, k+"="+m[k])
		}
	}
	return strings.Join(parts, "; ")
}

func applyTokenHeaders(headers map[string]string, token string) {
	if token == "" {
		return
	}
	headers["ht_token_preview"] = token
	headers["ht_token"] = token
	headers["token"] = token
	headers["Newuc-Token"] = token
}

func firstQuery(values url.Values, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(values.Get(k)); v != "" {
			return v
		}
	}
	return ""
}

func parseFragmentQuery(fragment string) url.Values {
	fragment = strings.TrimLeft(fragment, "?#/&")
	if i := strings.Index(fragment, "?"); i >= 0 {
		fragment = fragment[i+1:]
	}
	v, _ := url.ParseQuery(fragment)
	return v
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	case float64:
		if math.Trunc(t) == t {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func listMaps(v any) []map[string]any {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

var unsafeNameRe = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]+`)

func cleanName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(s)
	s = unsafeNameRe.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func mediaFormat(raw string) string {
	lower := strings.ToLower(raw)
	if strings.Contains(lower, ".m3u8") {
		return "m3u8"
	}
	u, err := url.Parse(raw)
	path := raw
	if err == nil {
		path = u.Path
	}
	if i := strings.LastIndex(path, "."); i >= 0 && i+1 < len(path) {
		ext := strings.ToLower(path[i+1:])
		switch ext {
		case "mp4", "flv", "pdf", "ppt", "pptx", "doc", "docx", "xls", "xlsx", "zip", "rar":
			return ext
		}
	}
	return "mp4"
}

func pageCount(meta map[string]any) int {
	for _, k := range []string{"pageCount", "last_page", "totalPage", "total_page", "pages"} {
		if n := atoi(str(meta[k])); n > 0 {
			return n
		}
	}
	return 0
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
