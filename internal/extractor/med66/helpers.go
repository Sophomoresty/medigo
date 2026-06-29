package med66

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

func collectMaps(v any) []anyMap {
	var out []anyMap
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case map[string]any:
			out = append(out, anyMap(t))
			for _, child := range t {
				walk(child)
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	walk(v)
	return out
}

func firstString(m anyMap, keys ...string) string {
	for _, k := range keys {
		if s := strings.TrimSpace(strAny(m[k])); s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

func strAny(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}

func normalizeURL(s, base string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "//") {
		return "https:" + s
	}
	if strings.HasPrefix(s, "/") {
		b, _ := url.Parse(base)
		u, _ := url.Parse(s)
		return b.ResolveReference(u).String()
	}
	return s
}

func pickFormat(u string) string {
	if strings.Contains(u, ".m3u8") {
		return "m3u8"
	}
	return "mp4"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func uniqueNonEmpty(values ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func med66HeadersFromJar(jar http.CookieJar, base map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	if cookie := med66CookieHeader(jar); cookie != "" {
		out["Cookie"] = cookie
		out["cookie"] = cookie
	}
	return out
}

func med66CookieHeader(jar http.CookieJar) string {
	if jar == nil {
		return ""
	}
	seen := map[string]bool{}
	var parts []string
	for _, raw := range []string{"https://member.med66.com/", "https://www.med66.com/", "https://elearning.med66.com/", "https://live.cdeledu.com/"} {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		for _, ck := range jar.Cookies(u) {
			key := ck.Name + "=" + ck.Value
			if key == "=" || seen[ck.Name] {
				continue
			}
			seen[ck.Name] = true
			parts = append(parts, key)
		}
	}
	return strings.Join(parts, "; ")
}

func cookieValue(jar http.CookieJar, bases []string, name string) string {
	for _, raw := range bases {
		u, _ := url.Parse(raw)
		for _, c := range jar.Cookies(u) {
			if c.Name == name {
				return c.Value
			}
		}
	}
	return ""
}

func collectPriceCandidates(v any) []float64 {
	var out []float64
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case map[string]any:
			for key, value := range t {
				low := strings.ToLower(key)
				if strings.Contains(low, "price") || strings.Contains(low, "needpay") || strings.Contains(low, "money") || strings.Contains(low, "amount") {
					if f := toFloat(value); f > 0 {
						out = append(out, f)
					}
				}
				walk(value)
			}
		case anyMap:
			walk(map[string]any(t))
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	walk(v)
	return out
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case float64:
		if x > 5000 && x == float64(int64(x)) {
			return x / 100
		}
		return x
	case json.Number:
		f, _ := strconv.ParseFloat(x.String(), 64)
		if f > 5000 && f == float64(int64(f)) {
			return f / 100
		}
		return f
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" || s == "<nil>" {
		return 0
	}
	m := regexp.MustCompile(`\d+(?:\.\d+)?`).FindString(s)
	if m == "" {
		return 0
	}
	f, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return 0
	}
	if f > 5000 && !strings.Contains(m, ".") {
		f /= 100
	}
	return f
}

func normalizeCoursePrice(v float64) float64 {
	if v <= 0 {
		return 0
	}
	if v > 5000 && v == float64(int64(v)) {
		return v / 100
	}
	return v
}
