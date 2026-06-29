// Package shanxiang implements an extractor for sx1211.com courses.
//
// API endpoints from decompiled Mooc/Courses/Shanxiang/:
//
//	https://www.sx1211.com/User/getAjaxCourseList
//	https://www.sx1211.com/course/study.html?id={cid}&skuId={sku_id}
//	https://www.sx1211.com/course/playbackView?id={playback_id}&skuId={sku_id}&scheduleId={schedule_id}
//	https://www.sx1211.com/course/docview.html?product_id={cid}&doc_id={doc_id}
//	https://view.csslcloud.net/replay/user/login
//	https://view.csslcloud.net/replay/video/play
//	https://view.csslcloud.net/replay/data/meta
package shanxiang

import (
	"encoding/json"
	"fmt"
	"html"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/extractor/shared"
	"github.com/Sophomoresty/mediago/internal/util"
)

const (
	urlCourseList     = "https://www.sx1211.com/User/getAjaxCourseList"
	urlStudy          = "https://www.sx1211.com/course/study.html?id=%s&skuId=%s"
	urlPlayback       = "https://www.sx1211.com/course/playbackView?id=%s&skuId=%s&scheduleId=%s"
	urlDocview        = "https://www.sx1211.com/course/docview.html?product_id=%s&doc_id=%s"
	urlCsslLogin      = "https://view.csslcloud.net/replay/user/login"
	urlCsslPlay       = "https://view.csslcloud.net/replay/video/play"
	urlCsslMeta       = "https://view.csslcloud.net/replay/data/meta"
	urlCsslOrigin     = "https://view.csslcloud.net"
	csslDeviceType    = "h5-pc"
	csslDeviceVersion = "3.11.0"
	csslTpl           = 20
	csslTerminal      = 3
	urlReferer        = "https://www.sx1211.com/"
	urlLoginCheck     = "https://www.sx1211.com/user/course.html"
	coursePageLimit   = 100
)

var patterns = []string{`(?:[\w-]+\.)?(?:sx1211|shanxiangjiaoyu)\.com/|(?:shanxiang|山香教育|山香|sx1211)`}

func init() {
	extractor.Register(&Shanxiang{}, extractor.SiteInfo{Name: "Shanxiang", URL: "sx1211.com", NeedAuth: true})
}

type Shanxiang struct{}

func (s *Shanxiang) Patterns() []string { return patterns }

type playbackInfo struct {
	CourseID    string
	SKUId       string
	PlaybackID  string
	ScheduleID  string
	PlaybackURL string
	Title       string
	Price       string
	Purchased   bool
	ChapterPath []string
	Raw         map[string]any
}

type fileInfo struct {
	URL         string
	Title       string
	Referer     string
	CourseID    string
	Format      string
	ChapterPath []string
}

func (s *Shanxiang) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("shanxiang requires login cookies")
	}
	c := util.NewClient()
	c.SetCookieJar(opts.Cookies)
	if err := ensureShanxiangLogin(c); err != nil {
		return nil, err
	}

	info := parseInputURL(rawURL)
	if info.PlaybackID != "" {
		entry, err := resolvePlayback(c, info)
		if err != nil {
			return nil, err
		}
		return entry, nil
	}
	if info.CourseID == "" || info.SKUId == "" {
		course, err := fetchCourseFromList(c, info.CourseID)
		if err == nil && course.CourseID != "" {
			info.CourseID, info.SKUId, info.Title = course.CourseID, course.SKUId, course.Title
			info.Price, info.Purchased = course.Price, course.Purchased
		}
	}
	if info.CourseID == "" || info.SKUId == "" {
		return nil, fmt.Errorf("cannot parse shanxiang course id and skuId from URL: %s", rawURL)
	}

	studyURL := fmt.Sprintf(urlStudy, info.CourseID, info.SKUId)
	body, err := c.GetString(studyURL, shanxiangHeaders(urlLoginCheck))
	if err != nil {
		return nil, fmt.Errorf("fetch shanxiang study page: %w", err)
	}
	title := firstNonEmpty(info.Title, extractStudyTitle(body), "shanxiang_"+info.CourseID)
	lessons := parseLessons(body, studyURL, info.CourseID, info.SKUId)
	if len(lessons) == 0 {
		return nil, fmt.Errorf("shanxiang: no playback lessons found in study page")
	}

	entries := make([]*extractor.MediaInfo, 0, len(lessons))
	for _, lesson := range lessons {
		entry, err := resolvePlayback(c, lesson)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	for _, file := range parseFiles(body, studyURL, info.CourseID) {
		if entry := resolveFileEntry(c, file); entry != nil {
			entries = append(entries, entry)
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("shanxiang: no CSSLcloud streams or courseware files resolved")
	}
	extra := map[string]any{"course_id": info.CourseID, "sku_id": info.SKUId}
	if info.Price != "" {
		extra["price"] = info.Price
		extra["purchased"] = info.Purchased
	}
	chapters := chaptersFromEntries(entries)
	return &extractor.MediaInfo{Site: "shanxiang", Title: title, Entries: entries, Chapters: chapters, Extra: extra}, nil
}

type sxCourseResp struct {
	Success any `json:"success"`
	Data    struct {
		Rows []struct {
			ProductID   any    `json:"productid"`
			ID          any    `json:"id"`
			SKUId       any    `json:"skuid"`
			SKUId2      any    `json:"skuId"`
			ProductName string `json:"productname"`
			Name        string `json:"name"`
			Price       any    `json:"price"`
			MinPrice    any    `json:"minprice"`
			MaxPrice    any    `json:"maxprice"`
		} `json:"rows"`
		TotalPages    int `json:"totalPages"`
		NextPageIndex any `json:"nextPageIndex"`
	} `json:"data"`
}

func fetchCourseFromList(c *util.Client, wantCID string) (playbackInfo, error) {
	courses, err := fetchCourseList(c)
	if err != nil {
		return playbackInfo{}, err
	}
	for _, course := range courses {
		if wantCID != "" && course.CourseID != wantCID {
			continue
		}
		return course, nil
	}
	return playbackInfo{}, fmt.Errorf("shanxiang course list has no matching course")
}

func fetchCourseList(c *util.Client) ([]playbackInfo, error) {
	seen := map[string]bool{}
	var out []playbackInfo
	nextPage := 1
	for page := 1; page <= coursePageLimit && nextPage > 0; page = nextPage {
		q := url.Values{}
		q.Set("productObjType", "1")
		q.Set("keywords", "")
		q.Set("p", strconv.Itoa(page))
		q.Set("isGift", "-1")
		q.Set("limit", strconv.Itoa(coursePageLimit))
		body, err := c.GetString(urlCourseList+"?"+q.Encode(), shanxiangHeaders(urlReferer))
		if err != nil {
			return nil, err
		}
		var resp sxCourseResp
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			return nil, err
		}
		if !successLike(resp.Success) {
			return nil, fmt.Errorf("shanxiang course list success=%v", resp.Success)
		}
		for _, row := range resp.Data.Rows {
			cid := firstNonEmpty(anyString(row.ProductID), anyString(row.ID))
			sku := firstNonEmpty(anyString(row.SKUId), anyString(row.SKUId2))
			if cid == "" || sku == "" {
				continue
			}
			key := cid + ":" + sku
			if seen[key] {
				continue
			}
			seen[key] = true
			price := firstNonEmpty(anyString(row.Price), anyString(row.MinPrice), anyString(row.MaxPrice))
			out = append(out, playbackInfo{
				CourseID:  cid,
				SKUId:     sku,
				Title:     firstNonEmpty(row.ProductName, row.Name),
				Price:     price,
				Purchased: parsePrice(price) <= 0,
			})
		}
		totalPages := resp.Data.TotalPages
		next := intValue(resp.Data.NextPageIndex)
		if next > page {
			nextPage = next
		} else {
			nextPage = page + 1
		}
		if totalPages > 0 && page >= totalPages {
			break
		}
		if len(resp.Data.Rows) == 0 && totalPages == 0 {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("shanxiang course list empty")
	}
	return out, nil
}

func resolvePlayback(c *util.Client, p playbackInfo) (*extractor.MediaInfo, error) {
	if p.PlaybackURL == "" {
		p.PlaybackURL = fmt.Sprintf(urlPlayback, p.PlaybackID, p.SKUId, p.ScheduleID)
	}
	h := shanxiangHeaders(p.PlaybackURL)
	body, err := c.GetString(p.PlaybackURL, h)
	if err != nil {
		return nil, fmt.Errorf("fetch shanxiang playback page: %w", err)
	}
	if p.CourseID == "" {
		p.CourseID = firstNonEmpty(extractInputValue(body, "product_id"), extractInputValue(body, "productId"))
	}
	cc := parseCCInfo(body)
	accessID := firstNonEmpty(cc["userId"], cc["groupId"])
	roomID := firstNonEmpty(cc["roomId"], cc["liveId"])
	recordID := firstNonEmpty(cc["recordId"], cc["videoId"], p.PlaybackID)
	viewerToken := cc["viewertoken"]
	if accessID == "" || roomID == "" || recordID == "" || viewerToken == "" {
		return nil, fmt.Errorf("shanxiang: missing CSSLcloud fields userId/roomId/recordId/viewertoken")
	}
	play, source, err := resolveShanxiangCsslPlayInfo(c, p, cc)
	if err != nil {
		return nil, err
	}
	title := firstNonEmpty(p.Title, "shanxiang_"+recordID)
	extra := map[string]any{
		"course_id": p.CourseID, "sku_id": p.SKUId, "playback_id": p.PlaybackID,
		"schedule_id": p.ScheduleID, "account_id": accessID, "room_id": roomID, "record_id": recordID,
		"cssl_session_id": play.SessionID, "cssl_meta_url": urlCsslMeta, "cssl_source": source,
	}
	if len(p.ChapterPath) > 0 {
		extra["chapter_path"] = append([]string(nil), p.ChapterPath...)
	}
	if p.Price != "" {
		extra["price"] = p.Price
		extra["purchased"] = p.Purchased
	}
	streams := csslStreams(play, p.PlaybackURL)
	if strings.Contains(strings.ToLower(play.VideoURL), ".m3u8") {
		if m3u8, err := c.GetString(play.VideoURL, map[string]string{"Referer": p.PlaybackURL}); err == nil {
			if rewritten, err := shared.CssLcloudRewriteM3U8Keys(c, m3u8, p.PlaybackURL); err == nil {
				extra["m3u8_text"] = rewritten
				extra["source_type"] = "m3u8_text"
				applyM3U8TextToStreams(streams, rewritten)
			} else {
				extra["m3u8_rewrite_error"] = err.Error()
			}
		}
	}
	return &extractor.MediaInfo{Site: "shanxiang", Title: title, Streams: streams, Extra: extra}, nil
}

func parseInputURL(raw string) playbackInfo {
	out := playbackInfo{}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return out
	}
	q := u.Query()
	if strings.Contains(u.Path, "/course/playbackView") {
		out.PlaybackID = q.Get("id")
		out.SKUId = firstNonEmpty(q.Get("skuId"), q.Get("skuid"))
		out.ScheduleID = firstNonEmpty(q.Get("scheduleId"), q.Get("scheduleid"))
		out.PlaybackURL = raw
		return out
	}
	if strings.Contains(u.Path, "/course/study.html") || strings.Contains(u.Path, "/course/detail.html") {
		out.CourseID = q.Get("id")
		out.SKUId = firstNonEmpty(q.Get("skuId"), q.Get("skuid"))
		return out
	}
	if m := regexp.MustCompile(`/course/(\d+)`).FindStringSubmatch(u.Path); len(m) > 1 {
		out.CourseID = m[1]
	}
	return out
}

var (
	lessonLinkRe        = regexp.MustCompile(`(?is)(?:href|data-url)\s*=\s*["']([^"']*/course/playbackView[^"']*)["']`)
	scheduleBlockRe     = regexp.MustCompile(`(?is)<li\b[^>]*class=["'][^"']*schedule-li[^"']*["'][^>]*>.*?</li>`)
	docBlockRe          = regexp.MustCompile(`(?is)<(?:li|div)\b[^>]*class=["'][^"']*doc-inside-item[^"']*["'][^>]*>.*?</(?:li|div)>`)
	fileLinkRe          = regexp.MustCompile(`(?is)(?:href|data-url|src|url)\s*=\s*["']([^"']+)["']`)
	titleAttrRe         = regexp.MustCompile(`(?is)(?:title|data-title|aria-label)\s*=\s*["']([^"']{2,160})["']`)
	blockTitleRe        = regexp.MustCompile(`(?is)<(?:h1|h2|h3|h4|h5|div|span|p)[^>]*(?:class=["'][^"']*(?:title|name)[^"']*["'])[^>]*>(.*?)</(?:h1|h2|h3|h4|h5|div|span|p)>`)
	studyTitleRe        = regexp.MustCompile(`(?is)<(?:h1|h2|h3|div)[^>]*(?:h-title|course-title|js-title-name)[^>]*>(.*?)</(?:h1|h2|h3|div)>|<title>(.*?)</title>`)
	ccPairRe            = regexp.MustCompile(`(?is)(userId|roomId|recordId|viewername|viewertoken|groupId)\s*:\s*["']([^"']*)["']`)
	directFileURLRe     = regexp.MustCompile(`(?is)(https?:)?//[^"'\s<>]+\.(?:pdf|pptx?|docx?|xlsx?|xls|zip|rar|7z|caj|txt|mp4)(?:\?[^"'\s<>]*)?`)
	inputValueFormatTpl = `(?is)(?:id|name)=["']%s["'][^>]*value=["']([^"']*)["']|value=["']([^"']*)["'][^>]*(?:id|name)=["']%s["']`
)

func parseLessons(body, base, cid, sku string) []playbackInfo {
	seen := map[string]bool{}
	var out []playbackInfo
	consume := func(block string, startPos int, chapterPath []string) {
		m := lessonLinkRe.FindStringSubmatchIndex(block)
		if len(m) < 4 {
			return
		}
		link := html.UnescapeString(block[m[2]:m[3]])
		abs := makeAbsolute(link, base)
		pi := parseInputURL(abs)
		pi.CourseID = firstNonEmpty(pi.CourseID, cid)
		pi.SKUId = firstNonEmpty(pi.SKUId, sku)
		key := pi.PlaybackID + ":" + pi.ScheduleID
		if pi.PlaybackID == "" || seen[key] {
			return
		}
		seen[key] = true
		pi.ChapterPath = dedupeChapterPath(chapterPath)
		pi.Title = firstNonEmpty(extractContextTitle(block), fmt.Sprintf("视频 %d", len(out)+1))
		out = append(out, pi)
	}
	for _, m := range scheduleBlockRe.FindAllStringIndex(body, -1) {
		block := body[m[0]:m[1]]
		consume(block, m[0], inferChapterPath(body, m[0]))
	}
	for _, m := range lessonLinkRe.FindAllStringSubmatchIndex(body, -1) {
		start, end := m[0]-500, m[1]+500
		if start < 0 {
			start = 0
		}
		if end > len(body) {
			end = len(body)
		}
		consume(body[start:end], start, inferChapterPath(body, m[0]))
	}
	return out
}

func parseCCInfo(text string) map[string]string {
	out := map[string]string{}
	for _, m := range ccPairRe.FindAllStringSubmatch(text, -1) {
		out[m[1]] = html.UnescapeString(m[2])
	}
	for _, pair := range [][2]string{{"userId", "userId"}, {"roomId", "roomId"}, {"recordId", "recordId"}, {"viewerName", "viewername"}, {"viewerId", "viewerId"}, {"liveId", "liveId"}, {"videoId", "videoId"}} {
		if out[pair[1]] != "" {
			continue
		}
		re := regexp.MustCompile(`(?is)id=["']` + regexp.QuoteMeta(pair[0]) + `["'][^>]*value=["']([^"']*)["']`)
		if m := re.FindStringSubmatch(text); len(m) > 1 {
			out[pair[1]] = html.UnescapeString(m[1])
		}
	}
	return out
}

func parseFiles(body, base, cid string) []fileInfo {
	seen := map[string]bool{}
	var out []fileInfo
	consume := func(block string, startPos int, chapterPath []string) {
		for _, m := range fileLinkRe.FindAllStringSubmatchIndex(block, -1) {
			link := html.UnescapeString(block[m[2]:m[3]])
			appendFileInfo(&out, seen, link, base, cid, block, chapterPath)
		}
		for _, m := range directFileURLRe.FindAllString(block, -1) {
			appendFileInfo(&out, seen, html.UnescapeString(m), base, cid, block, chapterPath)
		}
	}
	for _, m := range docBlockRe.FindAllStringIndex(body, -1) {
		consume(body[m[0]:m[1]], m[0], inferChapterPath(body, m[0]))
	}
	for _, m := range fileLinkRe.FindAllStringSubmatchIndex(body, -1) {
		start, end := m[0]-500, m[1]+500
		if start < 0 {
			start = 0
		}
		if end > len(body) {
			end = len(body)
		}
		consume(body[start:end], start, inferChapterPath(body, m[0]))
	}
	return out
}

func appendFileInfo(out *[]fileInfo, seen map[string]bool, link, base, cid, context string, chapterPath []string) {
	if strings.Contains(link, "/course/playbackView") {
		return
	}
	abs := makeAbsolute(link, base)
	if !isFileURL(abs) && !strings.Contains(abs, "/course/docview.html") {
		return
	}
	if seen[abs] {
		return
	}
	seen[abs] = true
	*out = append(*out, fileInfo{
		URL:         abs,
		Title:       firstNonEmpty(extractContextTitle(context), fmt.Sprintf("资料 %d", len(*out)+1)),
		Referer:     base,
		CourseID:    cid,
		Format:      fileFormat(abs),
		ChapterPath: dedupeChapterPath(chapterPath),
	})
}

func resolveFileEntry(c *util.Client, f fileInfo) *extractor.MediaInfo {
	fileURL := f.URL
	if strings.Contains(fileURL, "/course/docview.html") {
		body, err := c.GetString(fileURL, shanxiangHeaders(f.Referer))
		if err != nil {
			return nil
		}
		fileURL = parseDocviewFileURL(body, fileURL)
	}
	fileURL = unwrapFileURL(fileURL)
	if !isFileURL(fileURL) {
		return nil
	}
	fmtName := fileFormat(fileURL)
	title := strings.TrimSpace(f.Title)
	if title == "" {
		title = "资料"
	}
	return &extractor.MediaInfo{
		Site:  "shanxiang",
		Title: cleanName(title),
		Streams: map[string]extractor.Stream{
			"file": {Quality: "file", URLs: []string{fileURL}, Format: fmtName, Headers: map[string]string{"Referer": firstNonEmpty(f.Referer, urlReferer)}},
		},
		Extra: fileExtra(f, fileURL),
	}
}

func fileExtra(f fileInfo, fileURL string) map[string]any {
	extra := map[string]any{"type": "file", "course_id": f.CourseID, "file_url": fileURL, "file_fmt": fileFormat(fileURL)}
	if len(f.ChapterPath) > 0 {
		extra["chapter_path"] = append([]string(nil), f.ChapterPath...)
	}
	return extra
}

func parseDocviewFileURL(body, base string) string {
	if u := unwrapFileURL(base); isFileURL(u) {
		return u
	}
	for _, m := range fileLinkRe.FindAllStringSubmatch(body, -1) {
		if u := unwrapFileURL(makeAbsolute(html.UnescapeString(m[1]), base)); isFileURL(u) {
			return u
		}
	}
	if m := regexp.MustCompile(`(?is)(https?://[^"'\s<>]+\.(?:pdf|pptx?|docx?|xlsx?|xls|zip|rar|7z|caj|txt)(?:\?[^"'\s<>]*)?)`).FindStringSubmatch(body); len(m) > 1 {
		return html.UnescapeString(m[1])
	}
	return ""
}

func unwrapFileURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}
	for _, key := range []string{"file", "url", "downloadUrl"} {
		if v := u.Query().Get(key); v != "" {
			return makeAbsolute(html.UnescapeString(v), raw)
		}
	}
	return raw
}

func csslStreams(play *shared.CssLcloudPlayInfo, referer string) map[string]extractor.Stream {
	streams := map[string]extractor.Stream{}
	list := play.VideoList
	if len(list) == 0 {
		list = []shared.CssLcloudStreamInfo{{URL: play.VideoURL}}
	}
	for i, v := range list {
		if v.URL == "" {
			continue
		}
		key := fmt.Sprintf("definition_%d", v.Definition)
		if v.Definition == 0 {
			key = fmt.Sprintf("video_%d", i+1)
		}
		format := pickFormat(v.URL)
		streams[key] = extractor.Stream{Quality: key, URLs: []string{v.URL}, Format: format, NeedMerge: format == "m3u8", AudioURL: play.AudioURL, Headers: map[string]string{"Referer": referer}}
	}
	return streams
}

func shanxiangHeaders(referer string) map[string]string {
	return map[string]string{"X-Requested-With": "XMLHttpRequest", "Accept": "application/json, text/plain, */*", "Origin": "https://www.sx1211.com", "Referer": referer, "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"}
}
func ensureShanxiangLogin(c *util.Client) error {
	body, err := c.GetString(urlLoginCheck, shanxiangHeaders(urlReferer))
	if err != nil {
		return fmt.Errorf("shanxiang login check: %w", err)
	}
	lower := strings.ToLower(body)
	if (strings.Contains(body, "js-user-name") && strings.Contains(lower, "/user/course.html")) || strings.Contains(body, "我的课程") {
		return nil
	}
	return fmt.Errorf("shanxiang login check failed: cookie is not logged in")
}
func successLike(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x == 1
	case int:
		return x == 1
	case string:
		s := strings.TrimSpace(strings.ToLower(x))
		return s == "1" || s == "true" || s == "success" || s == "ok"
	default:
		s := strings.TrimSpace(strings.ToLower(anyString(v)))
		return s == "1" || s == "true" || s == "success" || s == "ok"
	}
}
func parsePrice(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return math.Inf(1)
	}
	if strings.Contains(raw, "免费") {
		return 0
	}
	cleaned := regexp.MustCompile(`[^0-9.\-]+`).ReplaceAllString(raw, "")
	if cleaned == "" {
		return math.Inf(1)
	}
	v, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return math.Inf(1)
	}
	return v
}
func applyM3U8TextToStreams(streams map[string]extractor.Stream, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	dataURL := shanxiangM3U8DataURL(text)
	for key, stream := range streams {
		if stream.Format != "m3u8" {
			continue
		}
		sourceURLs := append([]string(nil), stream.URLs...)
		stream.URLs = []string{dataURL}
		if stream.Extra == nil {
			stream.Extra = map[string]any{}
		}
		stream.Extra["source_urls"] = sourceURLs
		stream.Extra["source_type"] = "m3u8_text"
		streams[key] = stream
	}
}
func chaptersFromEntries(entries []*extractor.MediaInfo) []extractor.Chapter {
	seen := map[string]bool{}
	var chapters []extractor.Chapter
	for _, entry := range entries {
		if entry == nil || entry.Extra == nil {
			continue
		}
		path, ok := entry.Extra["chapter_path"].([]string)
		if !ok {
			if raw, ok := entry.Extra["chapter_path"].([]any); ok {
				for _, v := range raw {
					path = append(path, anyString(v))
				}
			}
		}
		for _, title := range path {
			title = cleanChapterTitle(title)
			if title == "" || seen[title] {
				continue
			}
			seen[title] = true
			chapters = append(chapters, extractor.Chapter{Title: title, Index: len(chapters) + 1})
		}
	}
	return chapters
}
func extractStudyTitle(body string) string {
	if m := studyTitleRe.FindStringSubmatch(body); len(m) > 0 {
		return stripTags(firstNonEmpty(m[1], m[2]))
	}
	return ""
}
func extractContextTitle(s string) string {
	if m := titleAttrRe.FindStringSubmatch(s); len(m) > 1 {
		return stripTags(m[1])
	}
	if m := blockTitleRe.FindStringSubmatch(s); len(m) > 1 {
		return stripLessonNoise(stripTags(m[1]))
	}
	return stripLessonNoise(stripTags(s))
}
func stripLessonNoise(s string) string {
	s = regexp.MustCompile(`(?is)(暂无评分|暂无讲义|开始学习|继续学习|未学习|学到\s*\S+|\d{1,2}:\d{2}:\d{2})`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`\s+`).ReplaceAllString(strings.TrimSpace(s), " ")
	if len([]rune(s)) > 160 {
		return ""
	}
	if m := regexp.MustCompile(`^(.+?)(?:\s+\d+分钟|\s*$)`).FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return s
}
func inferChapterPath(body string, pos int) []string {
	start := pos - 2500
	if start < 0 {
		start = 0
	}
	prefix := body[start:pos]
	var path []string
	headingRe := regexp.MustCompile(`(?is)<(?:h1|h2|h3|h4|h5|div|li|span)\b[^>]*(?:class=["'][^"']*(?:chapter|catalog|section|unit|stage|module|period|level|tree|dir|title|name)[^"']*["'])[^>]*>(.*?)</(?:h1|h2|h3|h4|h5|div|li|span)>`)
	for _, m := range headingRe.FindAllStringSubmatch(prefix, -1) {
		title := cleanChapterTitle(stripTags(m[1]))
		if title != "" {
			path = append(path, title)
		}
	}
	if len(path) > 3 {
		path = path[len(path)-3:]
	}
	return dedupeChapterPath(path)
}
func dedupeChapterPath(path []string) []string {
	var out []string
	lastKey := ""
	for _, item := range path {
		item = cleanChapterTitle(item)
		key := normalizeKey(item)
		if item == "" || key == "" || key == lastKey {
			continue
		}
		out = append(out, item)
		lastKey = key
	}
	return out
}
func cleanChapterTitle(title string) string {
	title = strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(html.UnescapeString(title), " "))
	title = regexp.MustCompile(`[（(]\s*\d+\s*课时\s*[）)]`).ReplaceAllString(title, "")
	title = regexp.MustCompile(`[（(]\s*\d+\s*[）)]$`).ReplaceAllString(title, "")
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	switch title {
	case "课程章节", "课程目录", "课程文档", "手机看课":
		return ""
	}
	if regexp.MustCompile(`(开始学习|继续学习|暂无讲义|未学习|学到\s*\S+|\d{1,2}:\d{2}:\d{2})`).MatchString(title) || len([]rune(title)) > 80 {
		return ""
	}
	return title
}
func normalizeKey(text string) string {
	return strings.ToLower(regexp.MustCompile(`\s+`).ReplaceAllString(text, ""))
}
func extractInputValue(body, name string) string {
	re := regexp.MustCompile(fmt.Sprintf(inputValueFormatTpl, regexp.QuoteMeta(name), regexp.QuoteMeta(name)))
	if m := re.FindStringSubmatch(body); len(m) > 0 {
		return firstNonEmpty(m[1], m[2])
	}
	return ""
}
func stripTags(s string) string {
	return strings.TrimSpace(regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(html.UnescapeString(s), " "))
}
func makeAbsolute(raw, base string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	b, err := url.Parse(base)
	if err != nil {
		return raw
	}
	return b.ResolveReference(u).String()
}
func pickFormat(u string) string {
	lower := strings.ToLower(u)
	if strings.Contains(lower, ".m3u8") || strings.HasPrefix(lower, "data:application/vnd.apple.mpegurl") {
		return "m3u8"
	}
	return "mp4"
}
func isFileURL(u string) bool {
	switch fileFormat(unwrapFileURL(u)) {
	case "mp4", "pdf", "ppt", "pptx", "doc", "docx", "xls", "xlsx", "zip", "rar", "7z", "caj", "txt":
		return true
	default:
		return false
	}
}
func fileFormat(u string) string {
	p := strings.ToLower(strings.Split(strings.Split(u, "?")[0], "#")[0])
	if i := strings.LastIndex(p, "."); i >= 0 && i+1 < len(p) {
		return p[i+1:]
	}
	return ""
}
func cleanName(s string) string {
	return regexp.MustCompile(`[\\/:*?"<>|\r\n\t]+`).ReplaceAllString(strings.TrimSpace(s), "_")
}
func anyString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		if math.Trunc(x) == x {
			return strconv.FormatInt(int64(x), 10)
		}
		return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(x, 'f', -1, 64), "0"), ".")
	case float32:
		f := float64(x)
		if math.Trunc(f) == f {
			return strconv.FormatInt(int64(f), 10)
		}
		return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(f, 'f', -1, 64), "0"), ".")
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
