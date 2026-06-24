package chaoxing

import (
	"fmt"
	htmlpkg "html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

func queryValue(raw, key string) string {
	if u, err := url.Parse(raw); err == nil {
		for k, vals := range u.Query() {
			if strings.EqualFold(k, key) && len(vals) > 0 {
				return vals[0]
			}
		}
	}
	return ""
}

func regexpFirst(text, pattern string) string {
	m := regexp.MustCompile(pattern).FindStringSubmatch(text)
	if len(m) > 1 {
		return htmlpkg.UnescapeString(m[1])
	}
	return ""
}

func hiddenValue(text, id string) string {
	for _, pattern := range []string{
		`(?is)id=["']` + regexp.QuoteMeta(id) + `["'][^>]*value=["']([^"']*)["']`,
		`(?is)value=["']([^"']*)["'][^>]*id=["']` + regexp.QuoteMeta(id) + `["']`,
	} {
		if m := regexp.MustCompile(pattern).FindStringSubmatch(text); len(m) > 1 {
			return htmlpkg.UnescapeString(m[1])
		}
	}
	return ""
}

func titleFromChunk(chunk string) string {
	return firstNonEmpty(regexpFirst(chunk, `(?is)title=["']([^"']+)["']`), stripTags(regexpFirst(chunk, `(?is)<span[^>]*>([\s\S]*?)</span>`)), stripTags(chunk))
}

func stripTags(s string) string {
	s = regexp.MustCompile(`(?is)<script[\s\S]*?</script>`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`(?is)<style[\s\S]*?</style>`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(s, " ")
	return cleanText(s)
}

func cleanText(s string) string {
	s = htmlpkg.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func lastIndexFloor(s, marker string, before, limit int) int {
	start := before - limit
	if start < 0 {
		start = 0
	}
	idx := strings.LastIndex(s[start:before], marker)
	if idx < 0 {
		return start
	}
	return start + idx
}

func nextIndexCeil(s, marker string, after, limit int) int {
	end := after + limit
	if end > len(s) {
		end = len(s)
	}
	idx := strings.Index(s[after:end], marker)
	if idx < 0 {
		return end
	}
	return after + idx + len(marker)
}

func jsObjectsAfter(text, marker string) []string {
	var out []string
	lower := strings.ToLower(text)
	marker = strings.ToLower(marker)
	pos := 0
	for {
		idx := strings.Index(lower[pos:], marker)
		if idx < 0 {
			break
		}
		idx += pos
		brace := strings.Index(text[idx:], "{")
		if brace < 0 {
			break
		}
		brace += idx
		if obj, end := balancedJSONObject(text, brace); obj != "" {
			out = append(out, obj)
			pos = end
		} else {
			pos = brace + 1
		}
	}
	return out
}

func balancedJSONObject(text string, start int) (string, int) {
	depth := 0
	inString := byte(0)
	escaped := false
	for i := start; i < len(text); i++ {
		ch := text[i]
		if inString != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == inString {
				inString = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			inString = ch
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return htmlpkg.UnescapeString(text[start : i+1]), i + 1
			}
		}
	}
	return "", start
}

func firstFieldString(v any, keys ...string) string {
	for _, s := range fieldStrings(v, keys...) {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func fieldStrings(v any, keys ...string) []string {
	keyset := map[string]bool{}
	for _, k := range keys {
		keyset[strings.ToLower(k)] = true
	}
	var out []string
	var walk func(any)
	walk = func(x any) {
		switch vv := x.(type) {
		case map[string]any:
			for k, val := range vv {
				if keyset[strings.ToLower(k)] {
					if s := strings.TrimSpace(toString(val)); s != "" {
						out = append(out, s)
					}
				}
				walk(val)
			}
		case []any:
			for _, child := range vv {
				walk(child)
			}
		}
	}
	walk(v)
	return out
}

func firstURLMatching(v any, pred func(string) bool) string {
	var found string
	var walk func(any)
	walk = func(x any) {
		if found != "" {
			return
		}
		switch vv := x.(type) {
		case map[string]any:
			for _, child := range vv {
				walk(child)
			}
		case []any:
			for _, child := range vv {
				walk(child)
			}
		case string:
			s := normalizeURL(vv)
			if pred(s) {
				found = s
			}
		}
	}
	walk(v)
	return found
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case bool:
		return strconv.FormatBool(x)
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

func normalizeURL(s string) string {
	s = strings.TrimSpace(strings.Trim(s, `"'`))
	s = strings.ReplaceAll(s, `\/`, `/`)
	if strings.HasPrefix(s, "//") {
		return "https:" + s
	}
	return s
}

func isHTTPURL(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://")
}

func isPlayableURL(s string) bool {
	low := strings.ToLower(s)
	return isHTTPURL(s) && (strings.Contains(low, ".mp4") || strings.Contains(low, ".m3u8") || strings.Contains(low, ".flv") || strings.Contains(low, ".mp3") || strings.Contains(low, ".m4a"))
}

func mediaFormat(rawURL, fallback string) string {
	ext := strings.TrimPrefix(strings.ToLower(firstNonEmpty(fileExt(rawURL), fallback)), ".")
	switch ext {
	case "m3u8":
		return "m3u8"
	case "mp3", "m4a", "flv", "mp4", "pdf", "ppt", "pptx", "doc", "docx", "xls", "xlsx", "zip", "rar", "7z", "jpg", "jpeg", "png", "webp", "gif":
		return ext
	case "":
		if strings.Contains(strings.ToLower(rawURL), ".m3u8") {
			return "m3u8"
		}
		return "mp4"
	default:
		return ext
	}
}

func normalizeExt(ext, name, rawURL string) string {
	ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	return firstNonEmpty(ext, fileExt(name), fileExt(rawURL))
}

func fileExt(s string) string {
	if idx := strings.IndexAny(s, "?#"); idx >= 0 {
		s = s[:idx]
	}
	if dot := strings.LastIndex(s, "."); dot >= 0 && dot < len(s)-1 {
		ext := strings.ToLower(s[dot+1:])
		if len(ext) <= 8 {
			return ext
		}
	}
	return ""
}

func isDocumentExt(ext string) bool {
	switch strings.TrimPrefix(strings.ToLower(ext), ".") {
	case "pdf", "ppt", "pptx", "doc", "docx", "xls", "xlsx", "zip", "rar", "7z", "caj", "txt", "jpg", "jpeg", "png", "webp", "gif":
		return true
	default:
		return false
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func compactExtra(extra map[string]any) map[string]any {
	if len(extra) == 0 {
		return nil
	}
	for k, v := range extra {
		if strings.TrimSpace(toString(v)) == "" {
			delete(extra, k)
		}
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}
