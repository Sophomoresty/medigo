package speiyou

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

func authFromJar(j http.CookieJar) authInfo {
	var a authInfo
	var parts []string
	seenParts := map[string]bool{}
	addCookiePart := func(part string) {
		part = strings.TrimSpace(part)
		if part == "" || !strings.Contains(part, "=") || seenParts[part] {
			return
		}
		seenParts[part] = true
		parts = append(parts, part)
		name, value, _ := strings.Cut(part, "=")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if tokenName(name) {
			a.Token = first(a.Token, cookieValue(value))
		}
		if strings.EqualFold(name, "stuId") || strings.EqualFold(name, "stu_id") || strings.EqualFold(name, "pu_uid") {
			a.StuID = first(a.StuID, cookieValue(value))
		}
	}
	for _, host := range []string{referer, "https://speiyou.com/", "https://www.speiyou.com/", "https://course-api-online.speiyou.com/", "https://classroom-api-online.speiyou.com/"} {
		u, _ := url.Parse(host)
		for _, ck := range j.Cookies(u) {
			addCookiePart(ck.Name + "=" + ck.Value)
			decodedValue := cookieValue(ck.Value)
			if tokenName(ck.Name) {
				a.Token = first(a.Token, decodedValue)
			}
			if strings.EqualFold(ck.Name, "stuId") || strings.EqualFold(ck.Name, "stu_id") || strings.EqualFold(ck.Name, "pu_uid") {
				a.StuID = first(a.StuID, decodedValue)
			}
			if strings.HasPrefix(strings.TrimSpace(decodedValue), "{") {
				var m map[string]any
				if json.Unmarshal([]byte(decodedValue), &m) == nil {
					a.Token = first(a.Token, findText(m, "token", "hb_token", "passport_token", "signToken"))
					a.StuID = first(a.StuID, findText(m, "stuId", "stu_id", "pu_uid", "puUid", "studentId", "student_id"))
					for _, part := range cookiePartsFromPayload(m) {
						addCookiePart(part)
					}
				}
			}
		}
	}
	a.Cookie = strings.Join(parts, "; ")
	return a
}
func baseHeaders(a authInfo) map[string]string {
	return map[string]string{"User-Agent": USER_AGENT, "referer": referer, "resVer": "1.0.6", "version": "3.60.0.2368", "terminal": "pc", "lang": "ch", "appClientType": "xes", "Referer": referer, "Origin": "owcr://classroom", "Accept": "application/json, text/plain, */*", "token": a.Token, "authorization": a.Token, "cookie": a.Cookie, "Cookie": a.Cookie, "stuId": a.StuID}
}
func tokenName(n string) bool {
	n = strings.ToLower(n)
	return n == "token" || n == "hb_token" || n == "passport_token" || n == "signtoken" || n == "authorization"
}
func cookiePartsFromPayload(m map[string]any) []string {
	var out []string
	addString := func(s string) {
		for _, part := range strings.Split(s, ";") {
			part = strings.TrimSpace(part)
			if strings.Contains(part, "=") {
				out = append(out, part)
			}
		}
	}
	for _, key := range []string{"cookie", "cookieValue"} {
		if s := textAt(m, key); s != "" {
			addString(s)
		}
	}
	for _, key := range []string{"cookies", "cookieList"} {
		switch v := m[key].(type) {
		case string:
			addString(v)
		case []any:
			for _, item := range v {
				cm := unwrapMap(item)
				name := textAt(cm, "name")
				value := first(textAt(cm, "value"), textAt(cm, "cookieValue"))
				if name != "" && value != "" {
					out = append(out, name+"="+value)
				}
			}
		}
	}
	return out
}
func courseKey(m map[string]any) string {
	course, live := unwrapMap(m["courseInfo"]), unwrapMap(m["liveInfo"])
	return first(textAt(m, "stdCourseId", "std_course_id", "course_id", "courseId"), textAt(course, "stdCourseId", "std_course_id", "course_id", "courseId"), textAt(live, "stdCourseId", "std_course_id", "course_id", "courseId"))
}
func lessonKey(m map[string]any) string {
	return first(textAt(m, "liveId", "live_id"), textAt(unwrapMap(m["liveInfo"]), "liveId", "live_id", "id"))
}
func jsonToMaps(v any) []map[string]any {
	if l, ok := v.([]any); ok {
		return maps(l)
	}
	m := unwrapMap(v)
	for _, k := range []string{"data", "list", "result", "records"} {
		if l, ok := m[k].([]any); ok {
			return maps(l)
		}
		if mm, ok := m[k].(map[string]any); ok {
			if out := jsonToMaps(mm); len(out) > 0 {
				return out
			}
		}
	}
	return nil
}
func maps(in []any) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, v := range in {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
func unwrapMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}
func valueStrings(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			out = append(out, fmt.Sprint(e))
		}
		return out
	case []string:
		return x
	}
	return nil
}
func listValues(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case []map[string]any:
		out := make([]any, 0, len(x))
		for _, m := range x {
			out = append(out, m)
		}
		return out
	}
	return nil
}
func findURL(v any) string {
	switch t := v.(type) {
	case map[string]any:
		for _, k := range []string{"videoUrl", "url", "playUrl"} {
			if u := textAt(t, k); strings.HasPrefix(u, "http") {
				return u
			}
		}
		for _, x := range t {
			if u := findURL(x); u != "" {
				return u
			}
		}
	case []any:
		for _, x := range t {
			if u := findURL(x); u != "" {
				return u
			}
		}
	}
	return ""
}
func findText(v any, keys ...string) string {
	switch t := v.(type) {
	case map[string]any:
		if s := textAt(t, keys...); s != "" {
			return s
		}
		for _, x := range t {
			if s := findText(x, keys...); s != "" {
				return s
			}
		}
	case []any:
		for _, x := range t {
			if s := findText(x, keys...); s != "" {
				return s
			}
		}
	}
	return ""
}
func textAt(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && s != "<nil>" && !strings.EqualFold(s, "null") && !strings.EqualFold(s, "undefined") {
				return s
			}
		}
	}
	return ""
}
func intAt(m map[string]any, key string) int64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch x := v.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	case json.Number:
		n, _ := x.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n
	}
	return 0
}
func clone(h map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range h {
		out[k] = v
	}
	return out
}
func match1(s, pat string) string {
	if m := regexp.MustCompile(pat).FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(html.UnescapeString(m[1]))
	}
	return ""
}
func first(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" && strings.TrimSpace(v) != "<nil>" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
func cookieValue(v string) string {
	if decoded, err := url.PathUnescape(v); err == nil && decoded != "" {
		return decoded
	}
	return v
}
func sanitize(s string) string {
	s = html.UnescapeString(strings.TrimSpace(s))
	return regexp.MustCompile(`[\\/:*?"<>|\r\n\t]+`).ReplaceAllString(s, "_")
}
func pickFormat(u string) string {
	p := strings.ToLower(strings.SplitN(strings.SplitN(u, "?", 2)[0], "#", 2)[0])
	if i := strings.LastIndex(p, "."); i >= 0 && i < len(p)-1 {
		return p[i+1:]
	}
	return "mp4"
}

func validSubjectResponse(v any) bool {
	switch x := v.(type) {
	case []any:
		return true
	case []map[string]any:
		return true
	case map[string]any:
		for _, key := range []string{"data", "list", "result", "records"} {
			child, ok := x[key]
			if !ok {
				continue
			}
			switch child.(type) {
			case []any, []map[string]any:
				return true
			default:
				if validSubjectResponse(child) {
					return true
				}
			}
		}
	}
	return false
}

var (
	priceNumberRe       = regexp.MustCompile(`-?\d+(?:\.\d+)?`)
	lessonCountSuffixRe = regexp.MustCompile(`（\d+讲）$`)
)

func extractPrice(payloads ...any) any {
	priceKeys := []string{"price", "salePrice", "sellPrice", "coursePrice", "activityPrice", "actualPrice", "originPrice", "originalPrice", "amount", "money"}
	for _, payload := range payloads {
		for _, m := range walkDicts(payload, 6) {
			for _, key := range priceKeys {
				if price, ok := normalizePriceValue(m[key]); ok {
					return price
				}
			}
		}
	}
	return nil
}

func walkDicts(payload any, maxDepth int) []map[string]any {
	if maxDepth < 0 || payload == nil {
		return nil
	}
	switch x := payload.(type) {
	case map[string]any:
		out := []map[string]any{x}
		for _, child := range x {
			out = append(out, walkDicts(child, maxDepth-1)...)
		}
		return out
	case []any:
		out := []map[string]any{}
		for _, child := range x {
			out = append(out, walkDicts(child, maxDepth-1)...)
		}
		return out
	case []map[string]any:
		out := []map[string]any{}
		for _, child := range x {
			out = append(out, walkDicts(child, maxDepth-1)...)
		}
		return out
	}
	return nil
}

func normalizePriceValue(v any) (any, bool) {
	if v == nil {
		return nil, false
	}
	if _, ok := v.(bool); ok {
		return nil, false
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" || s == "<nil>" || strings.EqualFold(s, "none") || strings.EqualFold(s, "null") || strings.EqualFold(s, "undefined") || strings.EqualFold(s, "nan") {
		return nil, false
	}
	if strings.Contains(s, "免费") {
		return 0, true
	}
	s = strings.NewReplacer(",", "", "￥", "", "¥", "", "元", "", " ", "").Replace(s)
	num := priceNumberRe.FindString(s)
	if num == "" {
		return nil, false
	}
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return nil, false
	}
	if f < 0 {
		f = 0
	}
	if f == float64(int64(f)) {
		return int64(f), true
	}
	return f, true
}

func maxCourseTime(lessons []map[string]any, seed int64) int64 {
	out := seed
	for _, lesson := range lessons {
		out = maxInt64(out, intAt(lesson, "liveStarttime"))
	}
	return out
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func stripLessonCount(title string) string {
	return strings.TrimSpace(lessonCountSuffixRe.ReplaceAllString(title, ""))
}

func formatCourseTitle(title string, lessonCount int) string {
	title = stripLessonCount(first(title, "未命名课程"))
	if lessonCount <= 0 {
		return title
	}
	return fmt.Sprintf("%s（%d讲）", title, lessonCount)
}
