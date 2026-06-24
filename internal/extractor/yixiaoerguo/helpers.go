package yixiaoerguo

import (
	"encoding/json"
	"fmt"
	"strings"
)

func parseJSON(body string) (map[string]any, error) {
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func successFalse(m map[string]any) bool {
	if v, ok := m["success"].(bool); ok && !v {
		return true
	}
	code := firstString(m, "code")
	return code == "401" || code == "1001"
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := strings.TrimSpace(fmt.Sprint(m[k])); s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstNonNil(vals ...any) any {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

func dig(m map[string]any, keys ...string) any { return digAny(m, keys...) }

func digAny(v any, keys ...string) any {
	cur := v
	for _, k := range keys {
		m := asMap(cur)
		if len(m) == 0 {
			return nil
		}
		cur = m[k]
	}
	return cur
}

func extractItems(v any) []map[string]any {
	if arr, ok := v.([]any); ok {
		out := make([]map[string]any, 0, len(arr))
		for _, it := range arr {
			if m := asMap(it); len(m) > 0 {
				out = append(out, m)
			}
		}
		return out
	}
	m := asMap(v)
	for _, k := range []string{"list", "records", "items", "rows", "content", "courseList", "courses", "chapters", "sections", "children", "data"} {
		if out := extractItems(m[k]); len(out) > 0 {
			return out
		}
	}
	return nil
}

func boolValue(v any) bool {
	s := strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
	return s == "true" || s == "1" || s == "yes"
}

func joinIdx(idx []int) string {
	parts := make([]string, len(idx))
	for i, v := range idx {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, ".")
}

func findURLs(v any, keyNames ...string) []string {
	keySet := map[string]bool{}
	for _, k := range keyNames {
		keySet[k] = true
	}
	seen := map[string]bool{}
	var out []string
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case []any:
			for _, it := range t {
				walk(it)
			}
		case map[string]any:
			for k, v := range t {
				if keySet[k] {
					if s := strings.TrimSpace(fmt.Sprint(v)); strings.HasPrefix(s, "http") && !seen[s] {
						seen[s] = true
						out = append(out, s)
					}
				}
				walk(v)
			}
		}
	}
	walk(v)
	return out
}

func bestMedia(items []map[string]any) map[string]any {
	var best map[string]any
	var bestSize float64
	for _, it := range items {
		u := firstString(it, "cdn_url", "url")
		if u == "" {
			continue
		}
		size := floatValue(it["size"])
		if best == nil || size >= bestSize {
			best = it
			bestSize = size
			best["url"] = u
		}
	}
	return best
}

func floatValue(v any) float64 {
	var f float64
	_, _ = fmt.Sscan(fmt.Sprint(v), &f)
	return f
}

func cleanTitle(s string) string { return titleCleanRe.ReplaceAllString(strings.TrimSpace(s), "_") }

func pickFormat(u string) string {
	if strings.Contains(strings.ToLower(u), ".m3u8") {
		return "m3u8"
	}
	return "mp4"
}
