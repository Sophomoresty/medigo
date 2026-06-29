package yixiaoerguo

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
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
				continue
			}
			out = append(out, extractItems(it)...)
		}
		return out
	}
	if arr, ok := v.([]map[string]any); ok {
		out := make([]map[string]any, 0, len(arr))
		for _, m := range arr {
			if len(m) > 0 {
				out = append(out, m)
			}
		}
		return out
	}
	m := asMap(v)
	if len(m) == 0 {
		return nil
	}
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

type qxMediaInfo struct {
	URL       string
	URLs      []string
	Duration  float64
	Size      float64
	SizeBytes int64
	Segments  []map[string]any
	Raw       map[string]any
}

func buildQXMediaInfo(items []map[string]any, expectedDuration string) qxMediaInfo {
	filtered := make([]map[string]any, 0, len(items))
	for _, it := range items {
		if firstString(it, "cdn_url", "url") != "" {
			filtered = append(filtered, it)
		}
	}
	if len(filtered) == 0 {
		return qxMediaInfo{}
	}
	ordered := append([]map[string]any(nil), filtered...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return qxSegmentOrder(ordered[i], i) < qxSegmentOrder(ordered[j], j)
	})
	best := filtered[0]
	for _, it := range filtered[1:] {
		if floatValue(it["size"]) >= floatValue(best["size"]) {
			best = it
		}
	}
	expected := toSeconds(expectedDuration)
	if fromPayload := durationFromPayload(items); fromPayload > expected {
		expected = fromPayload
	}
	info := qxMediaInfo{
		URL:       firstString(best, "cdn_url", "url"),
		URLs:      []string{firstString(best, "cdn_url", "url")},
		Duration:  maxFloat(toSeconds(best["duration"]), expected),
		Size:      bytesToMB(floatValue(best["size"])),
		SizeBytes: int64(floatValue(best["size"])),
		Raw:       best,
	}
	if isQXSegmentedMediaList(ordered, expected) {
		info.URLs = info.URLs[:0]
		info.Segments = make([]map[string]any, 0, len(ordered))
		var totalDuration, totalBytes float64
		for _, it := range ordered {
			u := firstString(it, "cdn_url", "url")
			if u == "" {
				continue
			}
			dur := toSeconds(it["duration"])
			sizeBytes := floatValue(it["size"])
			seg := map[string]any{"url": u, "duration": dur, "size": bytesToMB(sizeBytes), "raw": it}
			info.Segments = append(info.Segments, seg)
			info.URLs = append(info.URLs, u)
			totalDuration += dur
			totalBytes += sizeBytes
		}
		if len(info.URLs) > 0 {
			info.URL = info.URLs[0]
		}
		if totalDuration > info.Duration {
			info.Duration = totalDuration
		}
		if totalBytes > 0 {
			info.SizeBytes = int64(totalBytes)
			info.Size = bytesToMB(totalBytes)
		}
	}
	return info
}

func isQXSegmentedMediaList(items []map[string]any, expected float64) bool {
	if len(items) <= 1 {
		return false
	}
	hasQuality := false
	hasPartKey := false
	var totalDuration float64
	var durationCount int
	for _, it := range items {
		for _, k := range []string{"quality", "definition", "resolution", "bitrate", "clarity", "quality_desc"} {
			if firstString(it, k) != "" {
				hasQuality = true
			}
		}
		for _, k := range []string{"part", "part_no", "segment", "segmentIndex", "start", "startTime", "start_time", "begin"} {
			if firstString(it, k) != "" {
				hasPartKey = true
			}
		}
		if d := toSeconds(it["duration"]); d > 0 {
			totalDuration += d
			durationCount++
		}
	}
	if hasQuality && !hasPartKey {
		return false
	}
	if hasPartKey {
		return true
	}
	if durationCount == len(items) {
		if expected > 0 {
			return math.Abs(totalDuration-expected) <= math.Max(5, expected*0.25) || totalDuration >= expected*0.7
		}
		return true
	}
	return false
}

func qxSegmentOrder(m map[string]any, fallback int) float64 {
	for _, k := range []string{"startTime", "start_time", "start", "begin", "part", "part_no", "segmentIndex", "segment", "index", "idx"} {
		if v := floatValue(m[k]); v != 0 {
			return v
		}
	}
	return float64(fallback)
}

func floatValue(v any) float64 {
	var f float64
	_, _ = fmt.Sscan(fmt.Sprint(v), &f)
	return f
}

func toSeconds(v any) float64 {
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" || s == "<nil>" {
		return 0
	}
	if strings.Contains(s, ":") {
		parts := strings.Split(s, ":")
		total := 0.0
		for _, p := range parts {
			n, _ := strconv.ParseFloat(strings.TrimSpace(p), 64)
			total = total*60 + n
		}
		return total
	}
	repl := strings.NewReplacer("小时", "h", "时", "h", "分钟", "m", "分", "m", "秒钟", "s", "秒", "s")
	s = repl.Replace(strings.ToLower(s))
	if strings.ContainsAny(s, "hms") {
		var total float64
		num := strings.Builder{}
		for _, r := range s {
			if (r >= '0' && r <= '9') || r == '.' {
				num.WriteRune(r)
				continue
			}
			n, _ := strconv.ParseFloat(num.String(), 64)
			num.Reset()
			switch r {
			case 'h':
				total += n * 3600
			case 'm':
				total += n * 60
			case 's':
				total += n
			}
		}
		return total
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	if n > 10000 {
		return n / 1000
	}
	return n
}

func durationFromPayload(v any) float64 {
	var total float64
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case []map[string]any:
			for _, it := range t {
				walk(it)
			}
		case []any:
			for _, it := range t {
				walk(it)
			}
		case map[string]any:
			for k, vv := range t {
				if strings.EqualFold(k, "duration") {
					total += toSeconds(vv)
				}
				walk(vv)
			}
		}
	}
	walk(v)
	return total
}

func bytesToMB(v float64) float64 {
	if v <= 0 {
		return 0
	}
	if v > 1024*1024 {
		return v / 1024 / 1024
	}
	return v
}

var yxPriceNumberRe = regexp.MustCompile(`\d+(?:\.\d+)?`)

func normalizeYXPrice(v any) float64 {
	if v == nil {
		return 0
	}
	text := strings.TrimSpace(fmt.Sprint(v))
	if text == "" || text == "<nil>" {
		return 0
	}
	text = strings.NewReplacer(",", "", "¥", "", "￥", "", "元", "").Replace(text)
	if match := yxPriceNumberRe.FindString(text); match != "" {
		text = match
	}
	price, err := strconv.ParseFloat(text, 64)
	if err != nil || price < 0 {
		return 0
	}
	return price
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func qxJunkEncode(text string, step, junk int) string {
	if step <= 0 || junk <= 0 {
		return text
	}
	var b strings.Builder
	for i, r := range text {
		if i > 0 && i%step == 0 {
			for j := 0; j < junk; j++ {
				b.WriteByte('A')
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

func qxJunkDecode(text string, step, junk int) string {
	if step <= 0 || junk <= 0 {
		return text
	}
	group := step + junk
	var b strings.Builder
	for i, r := range text {
		if (i+1)%group == 0 {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func parseQXMaybeEncryptedJSON(body string) (map[string]any, error) {
	if m, err := parseJSON(body); err == nil {
		return m, nil
	}
	raw := strings.TrimSpace(body)
	candidates := []string{raw}
	if dec, err := base64.StdEncoding.DecodeString(padBase64(raw)); err == nil {
		candidates = append(candidates, string(dec))
		candidates = append(candidates, string(shiftBytes(dec, 81, "xor")))
		candidates = append(candidates, string(shiftBytes(dec, 81, "sub")))
		candidates = append(candidates, string(shiftBytes(dec, 81, "add")))
	}
	for _, cand := range candidates {
		if m, err := parseJSON(cand); err == nil {
			return m, nil
		}
	}
	return nil, fmt.Errorf("qianxuecloud: response is not JSON")
}

func decodeQXBase64JSON(text string) map[string]any {
	b, err := base64.StdEncoding.DecodeString(padBase64(strings.TrimSpace(text)))
	if err != nil {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(b, &out) != nil {
		return nil
	}
	return out
}

func padBase64(s string) string {
	s = strings.TrimSpace(s)
	if rem := len(s) % 4; rem != 0 {
		s += strings.Repeat("=", 4-rem)
	}
	return s
}

func shiftBytes(in []byte, key byte, mode string) []byte {
	out := make([]byte, len(in))
	for i, b := range in {
		switch mode {
		case "xor":
			out[i] = b ^ key
		case "sub":
			out[i] = b - key
		case "add":
			out[i] = b + key
		}
	}
	return out
}

func findFirstStringDeep(v any, keys ...string) string {
	keySet := map[string]bool{}
	for _, k := range keys {
		keySet[k] = true
	}
	var out string
	var walk func(any)
	walk = func(x any) {
		if out != "" {
			return
		}
		switch t := x.(type) {
		case []any:
			for _, it := range t {
				walk(it)
			}
		case map[string]any:
			for k, vv := range t {
				if keySet[k] {
					if s := strings.TrimSpace(fmt.Sprint(vv)); s != "" && s != "<nil>" {
						out = s
						return
					}
				}
				walk(vv)
			}
		}
	}
	walk(v)
	return out
}

func cleanTitle(s string) string { return titleCleanRe.ReplaceAllString(strings.TrimSpace(s), "_") }

func pickFormat(u string) string {
	if strings.Contains(strings.ToLower(u), ".m3u8") {
		return "m3u8"
	}
	return "mp4"
}
