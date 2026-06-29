package cto51

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"mime"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

type fileDownloadMeta struct {
	URL    string
	Title  string
	Format string
	Size   int64
}

func parseRoute(raw string) route {
	var r route
	r.CID = extractFirst(courseRe, raw)
	r.LID = extractFirst(lessonRe, raw)
	r.TrainID = extractFirst(trainRe, raw)
	r.TrainCourseID = extractFirst(trainCourseRe, raw)
	if u, err := url.Parse(raw); err == nil {
		q := u.Query()
		if id := q.Get("id"); strings.Contains(id, "_") {
			parts := strings.SplitN(id, "_", 2)
			r.TrainCourseID = firstNonEmpty(r.TrainCourseID, parts[0])
			r.LID = firstNonEmpty(parts[1], r.LID)
		}
		r.TrainID = firstNonEmpty(r.TrainID, q.Get("train_id"), q.Get("trainId"))
		r.TrainCourseID = firstNonEmpty(r.TrainCourseID, q.Get("train_course_id"), q.Get("trainCourseId"))
		r.LID = firstNonEmpty(q.Get("lesson_id"), q.Get("lessonId"), r.LID)
	}
	return r
}

type lessonRef struct {
	ID            string
	Title         string
	URL           string
	CourseID      string
	TrainID       string
	TrainCourseID string
	ChapterTitle  string
	SourceKind    string
	LiveID        string
	Preview       bool
	Size          int64
	Raw           map[string]any
}

func parseLessonLinks(body string) []lessonRef {
	seen := map[string]bool{}
	var out []lessonRef
	for _, m := range lessonLinkRe.FindAllStringSubmatch(body, -1) {
		if m[2] == "" || seen[m[2]] {
			continue
		}
		seen[m[2]] = true
		out = append(out, lessonRef{ID: m[2], Title: cleanText(m[3]), URL: normalizeURL(m[1], "https://edu.51cto.com/")})
	}
	return out
}

func headers(raw string) map[string]string {
	return map[string]string{"Accept": "application/json, text/plain, */*", "Origin": "https://edu.51cto.com", "Referer": firstNonEmpty(raw, "https://edu.51cto.com/")}
}
func cloneHeaders(h map[string]string) map[string]string {
	out := make(map[string]string, len(h)+1)
	for k, v := range h {
		out[k] = v
	}
	return out
}
func addQuery(base string, params map[string]string) string {
	if len(params) == 0 {
		return base
	}
	q := url.Values{}
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	if strings.Contains(base, "?") {
		return base + "&" + q.Encode()
	}
	return base + "?" + q.Encode()
}
func mediaInfo(title, u, format string, h map[string]string) *extractor.MediaInfo {
	stream := extractor.Stream{Quality: "best", URLs: []string{u}, Format: format, Headers: h}
	if format == "m3u8" || strings.HasPrefix(strings.ToLower(u), "data:application/vnd.apple.mpegurl") {
		stream.NeedMerge = true
	}
	return &extractor.MediaInfo{Site: "cto51", Title: util.SanitizeFilename(title), Streams: map[string]extractor.Stream{"best": stream}}
}
func firstMedia(list []media) media {
	if len(list) > 0 {
		return list[0]
	}
	return media{}
}
func mediaInfoFromMedia(m media, h map[string]string) *extractor.MediaInfo {
	info := mediaInfo(firstNonEmpty(m.Title, "51cto"), m.URL, m.Format, h)
	stream := info.Streams["best"]
	stream.Size = m.Size
	if len(m.Extra) > 0 {
		stream.Extra = m.Extra
		info.Extra = m.Extra
	}
	info.Streams["best"] = stream
	return info
}
func normalizeText(s string) string {
	s = html.UnescapeString(strings.TrimSpace(strings.Trim(s, `"'`)))
	s = strings.ReplaceAll(s, `\/`, "/")
	s = strings.ReplaceAll(s, `\\/`, "/")
	s = strings.ReplaceAll(s, `\u002F`, "/")
	return strings.TrimRight(s, `"' )],;`)
}
func normalizeURL(raw, base string) string {
	raw = normalizeText(raw)
	if raw == "" || strings.HasPrefix(strings.ToLower(raw), "javascript:") || strings.HasPrefix(raw, "#") {
		return ""
	}
	raw = strings.ReplaceAll(raw, " ", "%20")
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.IsAbs() {
		return u.String()
	}
	if base == "" {
		base = "https://edu.51cto.com/"
	}
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return b.ResolveReference(u).String()
}
func playPageURL(raw string) string {
	raw = normalizeText(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	lower := strings.ToLower(raw)
	if !strings.HasPrefix(lower, "http") {
		return ""
	}
	if strings.Contains(lower, "/center/wejob/play/lived") ||
		strings.Contains(lower, "/center/wejob/live/view") ||
		strings.Contains(lower, "/center/course/lesson/index") {
		return raw
	}
	return ""
}
func isVideoFormat(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "m3u8", "mp4", "flv", "mp3", "m4a", "aac":
		return true
	default:
		return false
	}
}
func isFileFormat(format string) bool {
	switch strings.ToLower(strings.Trim(strings.TrimSpace(format), ".")) {
	case "pdf", "ppt", "pptx", "doc", "docx", "xls", "xlsx", "zip", "rar", "7z", "tar", "gz", "txt", "md":
		return true
	default:
		return false
	}
}
func streamKey(info *extractor.MediaInfo) string {
	if info == nil {
		return ""
	}
	for _, st := range info.Streams {
		if len(st.URLs) > 0 {
			return strings.Join(st.URLs, "\x00")
		}
	}
	if info.Extra != nil {
		for _, k := range []string{"course_id", "train_id", "lesson_id", "file_url"} {
			if v := strings.TrimSpace(fmt.Sprint(info.Extra[k])); v != "" && v != "<nil>" {
				return k + ":" + v
			}
		}
	}
	return info.Title
}
func appendEntry(entries *[]*extractor.MediaInfo, seen map[string]bool, entry *extractor.MediaInfo) {
	if entry == nil {
		return
	}
	key := streamKey(entry)
	if key == "" || seen[key] {
		return
	}
	seen[key] = true
	*entries = append(*entries, entry)
}
func mediaFormat(raw string) string {
	u := raw
	if parsed, err := url.Parse(raw); err == nil {
		u = parsed.Path
	}
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(u)), ".")
	switch ext {
	case "m3u8", "mp4", "flv", "mp3", "m4a", "aac", "pdf", "ppt", "pptx", "doc", "docx", "xls", "xlsx", "zip", "rar", "7z", "tar", "gz", "txt", "md":
		return ext
	default:
		if strings.Contains(strings.ToLower(raw), ".m3u8") {
			return "m3u8"
		}
		if strings.Contains(strings.ToLower(raw), ".mp4") {
			return "mp4"
		}
		return ""
	}
}
func extractTitle(body string) string {
	m := titleRe.FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	return firstNonEmpty(cleanText(m[1]), cleanText(m[2]))
}
func normalizePriceValue(raw string) string {
	s := strings.TrimSpace(strings.Trim(raw, `"'`))
	if s == "" {
		return ""
	}
	s = strings.TrimPrefix(strings.TrimSpace(s), "￥")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return ""
	}
	if f >= 1000 && f == float64(int64(f)) {
		f = f / 100
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", f), "0"), ".")
}
func normalizePriceOrDefault(raw string) string {
	if price := normalizePriceValue(raw); price != "" {
		if price != "0" {
			return price
		}
	}
	return defaultCoursePrice
}
func extractPriceFromHTML(text string) string {
	if text == "" {
		return ""
	}
	if m := priceVarRe.FindStringSubmatch(text); len(m) > 1 {
		return normalizePriceValue(m[1])
	}
	if m := rmbPriceRe.FindStringSubmatch(text); len(m) > 1 {
		return normalizePriceValue(m[1])
	}
	if strings.Contains(text, "免费") {
		return "0"
	}
	return ""
}
func priceFromPayloads(payloads []any) string {
	for _, m := range walkMaps(payloads) {
		if textValue(m, "lesson_id", "lessonId", "file_id", "fileId") != "" || looksLikeFileMap(m) {
			continue
		}
		if price := pickNormalizedPrice(m, "price", "course_price", "coursePrice", "pay_price", "payPrice", "sale_price", "salePrice", "discount_price", "discountPrice", "learnCoinPrice", "original_price", "originalPrice"); price != "" {
			return price
		}
	}
	return ""
}
func trainingPriceFromPayloads(payloads []any, trainID string) string {
	prices := extractTrainingOrderPrices(payloads)
	if price := normalizePriceValue(prices[strings.TrimSpace(trainID)]); price != "" {
		return price
	}
	return ""
}
func extractTrainingOrderPrices(v any) map[string]string {
	out := map[string]string{}
	var walk func(any, bool, string)
	walk = func(node any, trainingContext bool, inheritedPrice string) {
		switch x := node.(type) {
		case []any:
			for _, item := range x {
				walk(item, trainingContext, inheritedPrice)
			}
		case map[string]any:
			isTraining := trainingContext || looksLikeTrainingOrderItem(x)
			price := pickNormalizedPrice(x, "original_price", "originalPrice", "trade_original_price", "tradeOriginalPrice", "total_price", "totalPrice", "total_price_tp", "totalPriceTp", "pay_price", "payPrice", "trade_pay_price", "tradePayPrice", "price", "course_price", "coursePrice", "train_price", "trainPrice")
			if price == "" {
				price = inheritedPrice
			}
			if isTraining && price != "" {
				if id := firstNonEmpty(trainingOrderTrainID(x), textValue(x, "train_id", "trainId", "training_id", "trainingId")); id != "" {
					out[id] = price
				}
			}
			for _, child := range x {
				walk(child, isTraining, price)
			}
		}
	}
	walk(v, false, "")
	return out
}
func pickNormalizedPrice(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if price := normalizePriceValue(textValue(m, key)); price != "" && price != "0" {
			return price
		}
	}
	return ""
}
func looksLikeTrainingOrderItem(m map[string]any) bool {
	var parts []string
	for _, key := range []string{"good_type_title", "goodTypeTitle", "good_type_name", "goodTypeName", "good_title", "goodTitle", "title", "name", "good_url", "goodUrl", "url", "course_url", "courseUrl", "jump_url", "jumpUrl"} {
		if s := textValue(m, key); s != "" {
			parts = append(parts, s)
		}
	}
	text := strings.ToLower(strings.ReplaceAll(strings.Join(parts, " "), `\/`, "/"))
	return strings.Contains(text, "精品班") || strings.Contains(text, "/px/train/") || strings.Contains(text, "/training_") || strings.Contains(text, "/center/wejob/") || strings.Contains(text, "train_id=")
}
func trainingOrderTrainID(m map[string]any) string {
	for _, key := range []string{"good_url", "goodUrl", "url", "course_url", "courseUrl", "jump_url", "jumpUrl"} {
		if id := trainIDFromURL(textValue(m, key)); id != "" {
			return id
		}
	}
	return textValue(m, "train_id", "trainId", "training_id", "trainingId", "good_id", "goodId")
}
func nestedFileURL(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	return textValue(m, "url", "fileUrl", "file_url", "downloadUrl", "download_url")
}
func resolveFileDownloadMeta(c *util.Client, raw string, h map[string]string) fileDownloadMeta {
	if c == nil || strings.TrimSpace(raw) == "" {
		return fileDownloadMeta{}
	}
	resp, err := c.Get(raw, h)
	if err != nil {
		return fileDownloadMeta{}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fileDownloadMeta{}
	}
	out := fileDownloadMeta{URL: raw, Size: resp.ContentLength}
	if resp.Request != nil && resp.Request.URL != nil {
		out.URL = resp.Request.URL.String()
	}
	out.Title = contentDispositionFilename(resp.Header.Get("Content-Disposition"))
	out.Format = firstNonEmpty(mediaFormat(out.URL), mediaFormat(out.Title), extFromContentType(resp.Header.Get("Content-Type")))
	return out
}
func contentDispositionFilename(header string) string {
	if strings.TrimSpace(header) == "" {
		return ""
	}
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		lower := strings.ToLower(part)
		switch {
		case strings.HasPrefix(lower, "filename*="):
			value := strings.TrimSpace(part[len("filename*="):])
			value = strings.Trim(value, `"`)
			if i := strings.Index(value, "''"); i >= 0 {
				value = value[i+2:]
			}
			if decoded, err := url.QueryUnescape(value); err == nil {
				value = decoded
			}
			return strings.TrimSpace(value)
		case strings.HasPrefix(lower, "filename="):
			value := strings.Trim(strings.TrimSpace(part[len("filename="):]), `"`)
			if decoded, err := url.QueryUnescape(value); err == nil {
				value = decoded
			}
			return strings.TrimSpace(value)
		}
	}
	return ""
}
func extFromContentType(contentType string) string {
	contentType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	if contentType == "" {
		return ""
	}
	switch contentType {
	case "application/vnd.apple.mpegurl", "application/x-mpegURL":
		return "m3u8"
	case "application/pdf":
		return "pdf"
	case "application/zip":
		return "zip"
	}
	exts, err := mime.ExtensionsByType(contentType)
	if err != nil || len(exts) == 0 {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(exts[0]), ".")
}
func cleanText(s string) string {
	return strings.Join(strings.Fields(regexp.MustCompile(`(?s)<[^>]+>`).ReplaceAllString(html.UnescapeString(s), " ")), " ")
}
func textValue(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s := anyString(v); s != "" {
				return s
			}
		}
	}
	return ""
}
func anyString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case jsonNumber:
		return strings.TrimSpace(t.String())
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" || s == "<nil>" {
			return ""
		}
		return s
	}
}
func boolValue(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case bool:
				return t
			case float64:
				return t != 0
			case int:
				return t != 0
			default:
				s := strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
				return s == "1" || s == "true" || s == "yes" || s == "y"
			}
		}
	}
	return false
}
func int64Value(m map[string]any, keys ...string) int64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case int:
				return int64(t)
			case int64:
				return t
			case float64:
				return int64(t)
			case jsonNumber:
				n, _ := strconv.ParseInt(t.String(), 10, 64)
				return n
			default:
				s := strings.TrimSpace(fmt.Sprint(v))
				if s == "" || s == "<nil>" {
					continue
				}
				if n, err := strconv.ParseInt(s, 10, 64); err == nil {
					return n
				}
				if f, err := strconv.ParseFloat(s, 64); err == nil {
					return int64(f)
				}
			}
		}
	}
	return 0
}

type jsonNumber interface{ String() string }

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
func extractFirst(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	for _, g := range m[1:] {
		if g != "" {
			return g
		}
	}
	return ""
}

func decodeJSON(text string, out any) error {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	return dec.Decode(out)
}

func deepFindText(v any, keys ...string) string {
	for _, m := range walkMaps(v) {
		if s := textValue(m, keys...); s != "" {
			return s
		}
	}
	return ""
}

func randomHex(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, (n+1)/2)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	s := hex.EncodeToString(b)
	if len(s) > n {
		return s[:n]
	}
	return s
}
