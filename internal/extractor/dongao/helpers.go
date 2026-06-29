package dongao

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

var (
	directMediaRe = regexp.MustCompile(`(?i)https?:\\?/\\?/[^"'<>\s]+\.(?:m3u8|mp4|flv|mov|m4v|mp3|m4a)(?:\?[^"'<>\s]*)?`)
	kvMediaRe     = regexp.MustCompile(`(?is)(?:cifMainSource|sdMainSource|hdMainSource|mainSource|videoSource|source|url|path|playUrl|playbackUrl|video_url)\s*[:=]\s*["']([^"']+)["']`)
	anchorRe      = regexp.MustCompile(`(?is)<a\b[^>]*href\s*=\s*["']([^"']+)["'][^>]*>([\s\S]*?)</a>`)
	directFileRe  = regexp.MustCompile(`(?i)(?:https?:\\?/\\?/|/)[^"'<>\s]+\.(?:pdf|pptx?|docx?|xlsx?|xls|zip|rar|7z|caj)(?:\?[^"'<>\s]*)?`)
)

const (
	dongaoAESKey = "j7hbgt2Jwn#7&86G"
	dongaoAESIV  = "5268&yu34Nh&ka#x"
)

type resourceRef struct {
	Title  string `json:"title"`
	URL    string `json:"url"`
	Format string `json:"format"`
}

func findMediaInText(text string) string {
	for _, m := range directMediaRe.FindAllString(text, -1) {
		if s := normalizeURL(m); isMediaURL(s) {
			return s
		}
	}
	for _, m := range kvMediaRe.FindAllStringSubmatch(text, -1) {
		if s := normalizeURL(m[1]); isMediaURL(s) {
			return s
		}
	}
	if payload := parseJSONText(text); payload != nil {
		if s := findMediaURL(payload); s != "" {
			return s
		}
	}
	return ""
}

func collectLectureNodes(v any, fallbackTitle string) []lectureNode {
	seen := map[string]bool{}
	var out []lectureNode
	var walk func(any, string)
	walk = func(x any, title string) {
		switch vv := x.(type) {
		case map[string]any:
			nextTitle := firstNonEmpty(valueString(vv, "lectureName", "lectureTitle", "title", "name", "videoName", "courseName"), title, fallbackTitle)
			id := valueString(vv, "lectureId", "lectureID", "listenLectureId", "liveNumberId", "liveLectureId", "id")
			if id != "" && !seen[id] && (hasAny(vv, "lectureId", "lectureID", "listenLectureId", "liveNumberId", "liveLectureId") || strings.Contains(strings.ToLower(nextTitle), "讲")) {
				seen[id] = true
				out = append(out, lectureNode{ID: id, Title: nextTitle})
			}
			for _, child := range vv {
				walk(child, nextTitle)
			}
		case []any:
			for _, child := range vv {
				walk(child, title)
			}
		}
	}
	walk(v, fallbackTitle)
	return out
}

func findMediaURL(v any) string {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range []string{"hdMainSource", "sdMainSource", "cifMainSource", "source", "mainSource", "videoSource", "url", "path", "playUrl", "playbackUrl", "video_url", "m3u8"} {
			if s := normalizeURL(valueString(x, k)); isMediaURL(s) {
				return s
			}
		}
		for _, child := range x {
			if s := findMediaURL(child); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range x {
			if s := findMediaURL(child); s != "" {
				return s
			}
		}
	case string:
		if s := normalizeURL(x); isMediaURL(s) {
			return s
		}
	}
	return ""
}

func pickTitle(v any) string {
	switch x := v.(type) {
	case map[string]any:
		if s := valueString(x, "courseName", "lectureName", "lectureTitle", "name", "title", "videoName"); s != "" {
			return s
		}
		for _, child := range x {
			if s := pickTitle(child); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range x {
			if s := pickTitle(child); s != "" {
				return s
			}
		}
	}
	return ""
}

func extractJSONObjects(text string) []string {
	var out []string
	for _, marker := range []string{"courseCatalog", "liveAndCourseMap", "lectureList", "listenParam"} {
		idx := strings.Index(text, marker)
		if idx < 0 {
			continue
		}
		start := strings.LastIndex(text[:idx], "{")
		if start < 0 {
			continue
		}
		if obj := balancedJSON(text[start:]); obj != "" {
			out = append(out, obj)
		}
	}
	return out
}

func balancedJSON(s string) string {
	depth := 0
	inStr := byte(0)
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inStr != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == inStr {
				inStr = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inStr = ch
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return ""
}

func mediaInfo(title, mediaURL string, headers map[string]string) *extractor.MediaInfo {
	return mediaInfoWithQuality(title, mediaURL, headers, "best")
}

func mediaInfoWithQuality(title, mediaURL string, headers map[string]string, quality string) *extractor.MediaInfo {
	format := "mp4"
	if strings.Contains(strings.ToLower(mediaURL), ".m3u8") {
		format = "m3u8"
	}
	return &extractor.MediaInfo{Site: "dongao", Title: util.SanitizeFilename(title), Streams: map[string]extractor.Stream{"best": {Quality: firstNonEmpty(quality, "best"), URLs: []string{mediaURL}, Format: format, NeedMerge: format == "m3u8", Headers: cloneHeaders(headers)}}}
}

func collectResourceRefsFromText(text, baseURL string) []resourceRef {
	refs := make([]resourceRef, 0)
	seen := map[string]bool{}
	add := func(title, rawURL string) {
		resourceURL := normalizeURLWithBase(rawURL, baseURL)
		if resourceURL == "" || !isFileURL(resourceURL) || seen[resourceURL] {
			return
		}
		seen[resourceURL] = true
		refs = append(refs, resourceRef{
			Title:  firstNonEmpty(cleanText(stripTags(title)), fileTitleFromURL(resourceURL), "课程资料"),
			URL:    resourceURL,
			Format: fileExtension(resourceURL),
		})
	}
	for _, m := range anchorRe.FindAllStringSubmatch(text, -1) {
		href := html.UnescapeString(m[1])
		if strings.Contains(strings.ToLower(href), "/lecture/") {
			continue
		}
		add(m[2], href)
	}
	for _, raw := range directFileRe.FindAllString(text, -1) {
		add("", raw)
	}
	if payload := parseJSONText(text); payload != nil {
		for _, ref := range collectResourceRefsFromAny(payload, baseURL) {
			if !seen[ref.URL] {
				seen[ref.URL] = true
				refs = append(refs, ref)
			}
		}
	}
	return refs
}

func collectResourceRefsFromAny(v any, baseURL string) []resourceRef {
	var refs []resourceRef
	var walk func(any, string)
	walk = func(x any, title string) {
		switch vv := x.(type) {
		case map[string]any:
			nextTitle := firstNonEmpty(valueString(vv, "fileName", "fileTitle", "resourceName", "title", "name", "lectureName", "courseName"), title)
			for _, key := range []string{"handoutUrl", "handoutURL", "handout", "pdfUrl", "pptUrl", "docUrl", "paperUrl", "fileUrl", "fileURL", "downloadUrl", "resourceUrl", "attachmentUrl", "coursewareUrl", "url", "path"} {
				if raw := valueString(vv, key); raw != "" {
					resourceURL := normalizeURLWithBase(raw, baseURL)
					if isFileURL(resourceURL) {
						refs = append(refs, resourceRef{
							Title:  firstNonEmpty(nextTitle, fileTitleFromURL(resourceURL), "课程资料"),
							URL:    resourceURL,
							Format: fileExtension(resourceURL),
						})
					}
				}
			}
			for _, child := range vv {
				walk(child, nextTitle)
			}
		case []any:
			for _, child := range vv {
				walk(child, title)
			}
		case string:
			resourceURL := normalizeURLWithBase(vv, baseURL)
			if isFileURL(resourceURL) {
				refs = append(refs, resourceRef{
					Title:  firstNonEmpty(title, fileTitleFromURL(resourceURL), "课程资料"),
					URL:    resourceURL,
					Format: fileExtension(resourceURL),
				})
			}
		}
	}
	walk(v, "")
	return dedupeResourceRefs(refs)
}

func resourceEntriesFromRefs(refs []resourceRef, headers map[string]string) []*extractor.MediaInfo {
	refs = dedupeResourceRefs(refs)
	entries := make([]*extractor.MediaInfo, 0, len(refs))
	for _, ref := range refs {
		entries = append(entries, resourceMediaInfo(ref, headers))
	}
	return entries
}

func resourceMediaInfo(ref resourceRef, headers map[string]string) *extractor.MediaInfo {
	format := firstNonEmpty(ref.Format, fileExtension(ref.URL), "bin")
	title := util.SanitizeFilename(firstNonEmpty(ref.Title, fileTitleFromURL(ref.URL), "课程资料"))
	return &extractor.MediaInfo{
		Site:  "dongao",
		Title: strings.TrimSuffix(title, "."+format),
		Streams: map[string]extractor.Stream{"best": {
			Quality: "best",
			URLs:    []string{ref.URL},
			Format:  format,
			Headers: cloneHeaders(headers),
		}},
		Extra: map[string]any{"type": "file", "source_url": ref.URL},
	}
}

func addResourceExtra(entry *extractor.MediaInfo, refs []resourceRef) {
	if entry == nil || len(refs) == 0 {
		return
	}
	if entry.Extra == nil {
		entry.Extra = map[string]any{}
	}
	entry.Extra["resources"] = dedupeResourceRefs(refs)
}

func resourceRefsFromExtra(entry *extractor.MediaInfo) []resourceRef {
	if entry == nil || entry.Extra == nil {
		return nil
	}
	switch refs := entry.Extra["resources"].(type) {
	case []resourceRef:
		return refs
	case []any:
		out := make([]resourceRef, 0, len(refs))
		for _, item := range refs {
			if m, ok := item.(map[string]any); ok {
				out = append(out, resourceRef{Title: valueString(m, "title", "Title"), URL: valueString(m, "url", "URL"), Format: valueString(m, "format", "Format")})
			}
		}
		return out
	default:
		return nil
	}
}

func dedupeResourceRefs(refs []resourceRef) []resourceRef {
	seen := map[string]bool{}
	out := make([]resourceRef, 0, len(refs))
	for _, ref := range refs {
		ref.URL = normalizeURLWithBase(ref.URL, referer)
		if ref.URL == "" || !isFileURL(ref.URL) || seen[ref.URL] {
			continue
		}
		seen[ref.URL] = true
		ref.Title = firstNonEmpty(ref.Title, fileTitleFromURL(ref.URL), "课程资料")
		ref.Format = firstNonEmpty(ref.Format, fileExtension(ref.URL), "bin")
		out = append(out, ref)
	}
	return out
}

func dedupeMediaEntries(entries []*extractor.MediaInfo) []*extractor.MediaInfo {
	seen := map[string]bool{}
	out := make([]*extractor.MediaInfo, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		key := entry.Title
		for _, stream := range entry.Streams {
			if len(stream.URLs) > 0 {
				key += "|" + stream.URLs[0]
				break
			}
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, entry)
	}
	return out
}

func valueString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func hasAny(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func normalizeURL(s string) string {
	s = strings.TrimSpace(strings.Trim(s, `"'`))
	s = strings.ReplaceAll(s, `\\/`, `/`)
	s = strings.ReplaceAll(s, `\/`, `/`)
	if strings.HasPrefix(s, "//") {
		return "https:" + s
	}
	if strings.HasPrefix(s, "/") {
		return origin + s
	}
	return s
}

func normalizeURLWithBase(s, baseURL string) string {
	s = strings.TrimSpace(html.UnescapeString(strings.Trim(s, `"'`)))
	s = strings.ReplaceAll(s, `\\/`, `/`)
	s = strings.ReplaceAll(s, `\/`, `/`)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "//") {
		return "https:" + s
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}
	if baseURL == "" {
		return normalizeURL(s)
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return normalizeURL(s)
	}
	ref, err := url.Parse(s)
	if err != nil {
		return normalizeURL(s)
	}
	return base.ResolveReference(ref).String()
}

func isMediaURL(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(low, "http") && (strings.Contains(low, ".m3u8") || strings.Contains(low, ".mp4") || strings.Contains(low, ".flv") || strings.Contains(low, ".mov") || strings.Contains(low, ".m4v") || strings.Contains(low, ".mp3") || strings.Contains(low, ".m4a"))
}

func isFileURL(s string) bool {
	switch fileExtension(s) {
	case "pdf", "ppt", "pptx", "doc", "docx", "xls", "xlsx", "zip", "rar", "7z", "caj":
		return strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), "http")
	default:
		return false
	}
}

func fileExtension(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	path := raw
	if err == nil {
		path = u.Path
	}
	if i := strings.LastIndex(path, "."); i >= 0 && i < len(path)-1 {
		return strings.ToLower(strings.TrimSpace(path[i+1:]))
	}
	return ""
}

func fileTitleFromURL(raw string) string {
	u, err := url.Parse(raw)
	path := raw
	if err == nil {
		path = u.Path
	}
	if i := strings.LastIndex(path, "/"); i >= 0 && i < len(path)-1 {
		name, _ := url.PathUnescape(path[i+1:])
		ext := fileExtension(name)
		return strings.TrimSuffix(name, "."+ext)
	}
	return ""
}

func stripTags(s string) string {
	return regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(s, " ")
}

func cloneHeaders(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func appendQuery(raw string, query url.Values) string {
	if len(query) == 0 {
		return raw
	}
	sep := "?"
	if strings.Contains(raw, "?") {
		sep = "&"
	}
	return raw + sep + query.Encode()
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func parseListenParam(lectureHTML string) map[string]any {
	for _, raw := range listenParamCandidates(lectureHTML) {
		if parsed := parseLooseJSONObject(raw); len(parsed) > 0 {
			return parsed
		}
	}
	return nil
}

func listenParamCandidates(text string) []string {
	var out []string
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?is)listenParam\s*=\s*'([\s\S]*?)'\s*;`),
		regexp.MustCompile(`(?is)listenParam\s*=\s*"([\s\S]*?)"\s*;`),
	} {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			out = append(out, m[1])
		}
	}
	if idx := strings.Index(text, "listenParam"); idx >= 0 {
		if start := strings.Index(text[idx:], "{"); start >= 0 {
			if obj := balancedJSON(text[idx+start:]); obj != "" {
				out = append(out, obj)
			}
		}
	}
	return out
}

func parseLooseJSONObject(raw string) map[string]any {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	raw = strings.ReplaceAll(raw, `\/`, "/")
	raw = strings.ReplaceAll(raw, `\u002F`, "/")
	raw = strings.ReplaceAll(raw, `\u002f`, "/")
	raw = strings.ReplaceAll(raw, `\u003A`, ":")
	raw = strings.ReplaceAll(raw, `\u003a`, ":")
	raw = strings.ReplaceAll(raw, `\u0026`, "&")
	if unq, err := strconv.Unquote(raw); err == nil {
		raw = unq
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) == nil {
		return m
	}
	normalized := regexp.MustCompile(`(?m)([{,]\s*)([A-Za-z_][A-Za-z0-9_]*)\s*:`).ReplaceAllString(raw, `${1}"${2}":`)
	normalized = regexp.MustCompile(`'([^'\\]*(?:\\.[^'\\]*)*)'`).ReplaceAllString(normalized, `"$1"`)
	if json.Unmarshal([]byte(normalized), &m) == nil {
		return m
	}
	return nil
}

func pickDongaoVideoSource(listen map[string]any, quality string) (string, string) {
	for _, key := range dongaoQualityOrder(quality) {
		if u := dongaoSourceValue(listen[key]); u != "" {
			return u, key
		}
	}
	for _, key := range []string{"source", "url", "path", "playUrl", "mainSource", "videoSource", "hdMainSource", "sdMainSource", "cifMainSource"} {
		if u := dongaoSourceValue(listen[key]); u != "" {
			return u, key
		}
	}
	return "", ""
}

func dongaoQualityOrder(quality string) []string {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "sd", "ld", "cif":
		return []string{"cifMainSource", "sdMainSource", "hdMainSource"}
	case "hd":
		return []string{"sdMainSource", "hdMainSource", "cifMainSource"}
	default:
		return []string{"hdMainSource", "sdMainSource", "cifMainSource"}
	}
}

func dongaoSourceValue(v any) string {
	switch x := v.(type) {
	case string:
		raw := normalizeURL(x)
		if isMediaURL(raw) {
			return raw
		}
		if dec := dongaoDecryptSource(x); isMediaURL(dec) {
			return dec
		}
	case map[string]any:
		if u := findMediaURL(x); u != "" {
			return u
		}
	case []any:
		for _, item := range x {
			if u := dongaoSourceValue(item); u != "" {
				return u
			}
		}
	}
	return ""
}

func dongaoDecryptSource(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || strings.HasPrefix(strings.ToLower(s), "http") {
		return ""
	}
	s = strings.ReplaceAll(s, " ", "+")
	s += strings.Repeat("=", (4-len(s)%4)%4)
	ct, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(ct) == 0 {
		return ""
	}
	pt, err := util.AESDecryptCBC(ct, []byte(dongaoAESKey), []byte(dongaoAESIV))
	if err != nil {
		return ""
	}
	return normalizeURL(string(pt))
}

type dongaoCourseRef struct {
	Title       string
	URL         string
	ExamID      string
	ExamName    string
	SubjectID   string
	SubjectName string
	SeasonID    string
	SeasonName  string
	Year        string
	Price       float64
	Purchased   bool
	Raw         map[string]any
}

func fetchDongaoCourseList(c *util.Client, headers map[string]string) ([]dongaoCourseRef, error) {
	payloads, err := fetchDongaoMemberPayloads(c, headers)
	if err != nil {
		return nil, err
	}
	var courses []dongaoCourseRef
	for _, payload := range payloads {
		courses = append(courses, buildDongaoCourses(payload)...)
	}
	courses = dedupeDongaoCourses(courses)
	sort.SliceStable(courses, func(i, j int) bool {
		if courses[i].Year != courses[j].Year {
			return courses[i].Year > courses[j].Year
		}
		return courses[i].Title < courses[j].Title
	})
	return courses, nil
}

func fetchDongaoMemberPayloads(c *util.Client, headers map[string]string) ([]any, error) {
	var out []any
	var lastErr error
	for _, endpoint := range []string{login_check_url, member_service_url} {
		for _, method := range []string{"GET", "POST"} {
			var body string
			var err error
			if method == "POST" {
				body, err = c.PostForm(endpoint, map[string]string{}, headers)
			} else {
				body, err = c.GetString(endpoint, headers)
			}
			if err != nil {
				lastErr = err
				continue
			}
			if isDongaoGuestPayload(body) {
				continue
			}
			payload := parseJSONText(body)
			if payload == nil {
				continue
			}
			out = append(out, payload)
		}
	}
	if len(out) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return out, nil
}

func buildDongaoCourses(root any) []dongaoCourseRef {
	var out []dongaoCourseRef
	seen := map[string]bool{}
	var walk func(any, dongaoCourseRef)
	walk = func(v any, ctx dongaoCourseRef) {
		switch x := v.(type) {
		case map[string]any:
			next := ctx
			next.Raw = x
			if id := valueString(x, "examId", "ed"); id != "" {
				next.ExamID = id
			}
			if name := valueString(x, "examName", "en"); name != "" {
				next.ExamName = name
			}
			if id := valueString(x, "subjectId", "sd", "sid"); id != "" {
				next.SubjectID = id
			}
			if name := valueString(x, "subjectName", "sn"); name != "" {
				next.SubjectName = name
			}
			if id := valueString(x, "sSubjectId", "ssd", "ssid", "seasonId"); id != "" {
				next.SeasonID = id
			}
			if name := valueString(x, "sSubjectName", "ssn", "seasonName"); name != "" {
				next.SeasonName = name
			}
			if year := firstNonEmpty(valueString(x, "sSubjectYear", "yr"), yearFromText(next.SeasonName)); year != "" {
				next.Year = year
			}
			if price := normalizeDongaoPrice(valueString(x, "price", "st", "amount")); price > 0 {
				next.Price = price
			}
			if next.SubjectID != "" && next.SeasonID != "" {
				key := next.SubjectID + "|" + next.SeasonID
				if !seen[key] {
					seen[key] = true
					next.Purchased = true
					next.Title = firstNonEmpty(valueString(x, "title", "name"), dongaoCourseTitle(next))
					next.URL = fmt.Sprintf("https://course.dongao.com/progress?sid=%s&ssid=%s", url.QueryEscape(next.SubjectID), url.QueryEscape(next.SeasonID))
					out = append(out, next)
				}
			}
			for _, child := range x {
				walk(child, next)
			}
		case []any:
			for _, child := range x {
				walk(child, ctx)
			}
		}
	}
	walk(root, dongaoCourseRef{})
	return out
}

func normalizeDongaoPrice(raw string) float64 {
	n, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || n <= 0 {
		return 0
	}
	if n >= 1000 {
		return n / 10
	}
	return n * 100
}

func yearFromText(text string) string {
	if m := regexp.MustCompile(`20\d{2}`).FindString(text); m != "" {
		return m
	}
	return ""
}

func dongaoCourseTitle(c dongaoCourseRef) string {
	subject := firstNonEmpty(c.SubjectName, c.SubjectID)
	season := firstNonEmpty(c.SeasonName, c.SeasonID)
	if season != "" {
		return fmt.Sprintf("东奥课程-%s[%s]", subject, season)
	}
	return "东奥课程-" + subject
}

func dedupeDongaoCourses(in []dongaoCourseRef) []dongaoCourseRef {
	seen := map[string]int{}
	var out []dongaoCourseRef
	for _, c := range in {
		if c.SubjectID == "" || c.SeasonID == "" {
			continue
		}
		key := c.SubjectID + "|" + c.SeasonID
		if idx, ok := seen[key]; ok {
			if out[idx].Title == "" {
				out[idx].Title = c.Title
			}
			if out[idx].Price == 0 {
				out[idx].Price = c.Price
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, c)
	}
	return out
}

func dongaoCourseListMedia(courses []dongaoCourseRef) *extractor.MediaInfo {
	entries := make([]*extractor.MediaInfo, 0, len(courses))
	for _, course := range courses {
		entries = append(entries, &extractor.MediaInfo{
			Site:  "dongao",
			Title: util.SanitizeFilename(firstNonEmpty(course.Title, course.SubjectID+"_"+course.SeasonID)),
			Extra: map[string]any{
				"url":          course.URL,
				"course_id":    course.SubjectID + "_" + course.SeasonID,
				"subject_id":   course.SubjectID,
				"subject_name": course.SubjectName,
				"season_id":    course.SeasonID,
				"season_name":  course.SeasonName,
				"exam_id":      course.ExamID,
				"exam_name":    course.ExamName,
				"year":         course.Year,
				"price":        course.Price,
				"purchased":    course.Purchased,
				"course":       course.Raw,
			},
		})
	}
	return &extractor.MediaInfo{Site: "dongao", Title: "dongao_courses", Entries: entries}
}

func dongaoCookieHeader(jar interface{ Cookies(*url.URL) []*http.Cookie }) string {
	if jar == nil {
		return ""
	}
	hosts := []string{"course.dongao.com", "my.dongao.com", "serveapi.dongao.com", "www.dongao.com", "dongao.com"}
	seen := map[string]bool{}
	var parts []string
	for _, host := range hosts {
		for _, ck := range jar.Cookies(&url.URL{Scheme: "https", Host: host, Path: "/"}) {
			if ck.Name == "" || seen[ck.Name] {
				continue
			}
			seen[ck.Name] = true
			parts = append(parts, ck.Name+"="+ck.Value)
		}
	}
	return strings.Join(parts, "; ")
}

func hasDongaoLoginCookie(jar interface{ Cookies(*url.URL) []*http.Cookie }) bool {
	cookies := cookieMapFromHeader(dongaoCookieHeader(jar))
	return cookies["dongaoLogin"] != "" && cookies["memberinfo"] != ""
}

func validateDongaoLogin(c *util.Client, jar interface{ Cookies(*url.URL) []*http.Cookie }, headers map[string]string) error {
	if !hasDongaoLoginCookie(jar) {
		return fmt.Errorf("dongao requires valid login cookies (dongaoLogin and memberinfo)")
	}
	payloads, err := fetchDongaoMemberPayloads(c, headers)
	if err != nil {
		return fmt.Errorf("dongao login check: %w", err)
	}
	if len(payloads) == 0 {
		return fmt.Errorf("dongao login check failed: member course APIs returned no logged-in payload")
	}
	apiHeaders := cloneHeaders(headers)
	apiHeaders["Referer"] = device_verify_referer
	apiHeaders["Origin"] = device_verify_origin
	for _, payload := range payloads {
		courses := buildDongaoCourses(payload)
		if len(courses) == 0 {
			continue
		}
		form := map[string]string{"lecturerId": "", "sid": courses[0].SubjectID, "ssid": courses[0].SeasonID}
		if body, err := c.PostForm(stage_probe_url, form, apiHeaders); err == nil && !isDongaoGuestPayload(body) {
			if parsed := parseJSONText(body); parsed != nil {
				return nil
			}
		}
		return nil
	}
	return nil
}

func isDongaoGuestPayload(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return true
	}
	if strings.HasPrefix(lower, "<") && (strings.Contains(lower, "login") || strings.Contains(lower, "passport") || strings.Contains(lower, "登录")) {
		return true
	}
	return strings.Contains(lower, "loginbgimgpath") || strings.Contains(lower, "loginpageimagelink") || strings.Contains(lower, "deviceverify")
}

func cookieMapFromHeader(header string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(header, ";") {
		pieces := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pieces) == 2 {
			out[pieces[0]] = pieces[1]
		}
	}
	return out
}
