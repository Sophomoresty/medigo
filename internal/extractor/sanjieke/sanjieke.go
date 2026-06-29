// Package sanjieke implements an extractor for sanjieke.cn (三节课) courses.
package sanjieke

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

// Endpoints from decompiled Mooc/Courses/Sanjieke/:
const (
	urlReferer         = "https://study.sanjieke.cn/"
	urlOrigin          = "https://study.sanjieke.cn"
	urlClassroom       = "https://classroom.sanjieke.cn/my_course"
	urlClassroomOrigin = "https://classroom.sanjieke.cn"
	urlCourseList      = "https://service.sanjieke.cn/classroom/not_expired"
	urlCourseCatalog   = "https://web-api.sanjieke.cn/b-side/api/web/course/list"
	urlUserInfo        = "https://service.sanjieke.cn/user/info"
	urlStudyAPIRoot    = "https://web-api.sanjieke.cn/b-side/api/web/study/%s/%s"
	urlStudyInfo       = urlStudyAPIRoot + "/info"
	urlTree            = urlStudyAPIRoot + "/content/tree"
	urlSection         = urlStudyAPIRoot + "/content/%s"
	urlAttachmentList  = urlStudyAPIRoot + "/attachment/list"
	urlStudyPage       = "https://study.sanjieke.cn/course/%s/%s"
	urlVideoAuth       = "https://service.sanjieke.cn/video/master/auth"
	urlPublicProduct   = "https://www.sanjieke.cn/course/detail/sjk/%s"
	apiKey             = "cDpJh7SuWGFZCFfSjvByc34PNSBrNVrB"
	domainPrefix       = "cos"
	browserUA          = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36"
)

var patterns = []string{`(?:[\w-]+\.)?sanjieke\.cn/`}

func init() {
	extractor.Register(&Sanjieke{}, extractor.SiteInfo{Name: "Sanjieke", URL: "sanjieke.cn", NeedAuth: true})
}

type Sanjieke struct{}

func (s *Sanjieke) Patterns() []string { return patterns }

type courseKey struct {
	classID, courseID, projectID, productID, title string
	price                                          float64
	purchased                                      bool
}

type courseListResp struct {
	Code int `json:"code"`
	Data struct {
		List       []courseItem `json:"list"`
		IsLastPage bool         `json:"is_last_page"`
		LastPage   any          `json:"last_page"`
	} `json:"data"`
}

type courseItem struct {
	ClassID      any    `json:"class_id"`
	CourseID     any    `json:"course_id"`
	StudyCourse  any    `json:"study_course_id"`
	ProjectID    any    `json:"project_id"`
	ProjectID2   any    `json:"projectId"`
	ProductID    any    `json:"product_id"`
	ProductID2   any    `json:"productId"`
	Title        string `json:"title"`
	Name         string `json:"name"`
	Subtitle     string `json:"subtitle"`
	Price        any    `json:"price"`
	CoursePrice  any    `json:"course_price"`
	OriginPrice  any    `json:"origin_price"`
	CurrentPrice any    `json:"current_price"`
	StudyingURL  string `json:"studying_url"`
}

type infoResp struct {
	Code int `json:"code"`
	Data struct {
		Title string `json:"title"`
		Name  string `json:"name"`
	} `json:"data"`
}

type treeResp struct {
	Code int `json:"code"`
	Data struct {
		Tree     []node `json:"tree"`
		Nodes    []node `json:"nodes"`
		Children []node `json:"children"`
	} `json:"data"`
}

type contentResp struct {
	Code int `json:"code"`
	Data struct {
		Nodes        []node       `json:"nodes"`
		Children     []node       `json:"children"`
		VideoContent videoContent `json:"videoContent"`
	} `json:"data"`
}

type node struct {
	NodeID   any    `json:"nodeId"`
	ID       any    `json:"id"`
	Name     string `json:"name"`
	Title    string `json:"title"`
	Children []node `json:"children"`
}

type videoContent struct {
	ContentID any        `json:"contentId"`
	URL       string     `json:"url"`
	Items     []videoURL `json:"items"`
	Ratios    []videoURL `json:"resolutionRatioObjList"`
}

type videoURL struct {
	URL string `json:"url"`
}

func (s *Sanjieke) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("sanjieke requires login cookies")
	}
	key := parseCourseKey(rawURL)
	c := util.NewClient()
	c.SetCookieJar(opts.Cookies)
	loginChecked := false
	if hasSJKCookie(opts.Cookies) {
		if !checkSJKLogin(c, opts.Cookies) {
			return nil, fmt.Errorf("sanjieke: cookie validation failed; login required")
		}
		loginChecked = true
	}
	if key.courseID == "" {
		found, err := fetchCourseList(c, opts.Cookies, key)
		if err == nil {
			key = found
		}
	} else if hasSJKCookie(opts.Cookies) {
		if found, err := fetchCourseList(c, opts.Cookies, key); err == nil {
			key = mergeCourseKey(key, found)
		}
	}
	if key.courseID == "" {
		return nil, fmt.Errorf("cannot parse sanjieke course_id from URL: %s", rawURL)
	}
	if key.projectID == "" {
		key.projectID = "0"
	}

	h := studyHeaders(opts.Cookies, fmt.Sprintf(urlStudyPage, key.projectID, key.courseID))
	title := firstNonEmpty(key.title, "sanjieke_"+key.courseID)
	if body, err := c.GetString(fmt.Sprintf(urlStudyInfo, key.projectID, key.courseID), h); err == nil {
		var info infoResp
		if json.Unmarshal([]byte(body), &info) == nil && info.Code == 200 {
			title = firstNonEmpty(info.Data.Title, info.Data.Name, title)
		}
	}
	if key.price == 0 && key.productID != "" {
		key.price = fetchPublicProductPrice(c, key.productID)
	}
	body, err := c.GetString(fmt.Sprintf(urlTree, key.projectID, key.courseID), h)
	if err != nil {
		return nil, fmt.Errorf("sanjieke content/tree: %w", err)
	}
	var tree treeResp
	if err := json.Unmarshal([]byte(body), &tree); err != nil {
		return nil, fmt.Errorf("sanjieke parse content/tree: %w", err)
	}
	if tree.Code != 200 {
		return nil, fmt.Errorf("sanjieke content/tree code=%d", tree.Code)
	}
	entries := collectEntries(c, opts.Cookies, key, append(append([]node{}, tree.Data.Tree...), append(tree.Data.Nodes, tree.Data.Children...)...), nil)
	if len(entries) == 0 {
		entries = append(entries, attachmentEntries(c, opts.Cookies, key)...)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("sanjieke: no playable video or file nodes in course tree")
	}
	extra := compactSJKExtra(map[string]any{
		"class_id":      key.classID,
		"course_id":     key.courseID,
		"project_id":    key.projectID,
		"product_id":    key.productID,
		"price":         key.price,
		"purchased":     key.purchased,
		"login_checked": loginChecked,
	})
	return &extractor.MediaInfo{Site: "sanjieke", Title: title, Entries: entries, Extra: extra}, nil
}

func fetchCourseList(c *util.Client, jar http.CookieJar, want courseKey) (courseKey, error) {
	const limit = 12
	var first courseKey
	for page := 1; page <= 200; page++ {
		apiURL := urlCourseList + "?teacherId=&keyword=&sortDirection=&sortField=lastStudyAt&tab=all&page=" + strconv.Itoa(page) + "&limit=" + strconv.Itoa(limit)
		body, err := c.GetString(apiURL, classroomHeaders(jar))
		if err != nil {
			return courseKey{}, err
		}
		var resp courseListResp
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			return courseKey{}, err
		}
		if resp.Code != 200 {
			return courseKey{}, fmt.Errorf("course list code=%d", resp.Code)
		}
		for _, item := range resp.Data.List {
			normalized := normalizeCourseItem(item)
			classID := normalized.classID
			courseID := normalized.courseID
			if courseID != "" && first.courseID == "" {
				first = normalized
			}
			if want.classID != "" && classID != want.classID {
				continue
			}
			if want.courseID != "" && courseID != want.courseID {
				continue
			}
			if courseID != "" {
				return normalized, nil
			}
		}
		if resp.Data.IsLastPage || len(resp.Data.List) < limit || page >= toInt(resp.Data.LastPage, 200) {
			break
		}
	}
	if want.classID == "" && want.courseID == "" && first.courseID != "" {
		return first, nil
	}
	return courseKey{}, fmt.Errorf("course list has no matching sanjieke course")
}

func collectEntries(c *util.Client, jar http.CookieJar, key courseKey, nodes []node, prefix []string) []*extractor.MediaInfo {
	var entries []*extractor.MediaInfo
	for _, n := range nodes {
		name := firstNonEmpty(n.Name, n.Title)
		nextPrefix := append(append([]string{}, prefix...), name)
		nodeID := firstNonEmpty(anyString(n.NodeID), anyString(n.ID))
		if nodeID != "" {
			entries = append(entries, entriesFromContent(c, jar, key, nodeID, nextPrefix)...)
		}
		if len(n.Children) > 0 {
			entries = append(entries, collectEntries(c, jar, key, n.Children, nextPrefix)...)
		}
	}
	return entries
}

func entriesFromContent(c *util.Client, jar http.CookieJar, key courseKey, nodeID string, prefix []string) []*extractor.MediaInfo {
	referer := buildPageReferer(key, nodeID)
	body, err := c.GetString(fmt.Sprintf(urlSection, key.projectID, key.courseID, nodeID), studyHeaders(jar, referer))
	if err != nil {
		return nil
	}
	var resp contentResp
	if json.Unmarshal([]byte(body), &resp) != nil || resp.Code != 200 {
		return nil
	}
	var raw map[string]any
	_ = json.Unmarshal([]byte(body), &raw)
	var entries []*extractor.MediaInfo
	vc := resp.Data.VideoContent
	videoID := anyString(vc.ContentID)
	videoURL := pickVideoURL(vc)
	if videoURL == "" && videoID != "" {
		videoURL = authVideoURL(c, jar, videoID, referer)
	}
	if videoURL != "" {
		title := firstNonEmpty(strings.Join(nonEmpty(prefix), " / "), "sanjieke_"+nodeID)
		extra := map[string]any{"node_id": nodeID, "video_id": videoID, "referer": referer}
		streamURL := videoURL
		format := pickFormat(videoURL)
		if m3u8, keyPairs := fetchMediaM3U8(c, jar, videoURL, referer); m3u8 != "" {
			m3u8 = prepareSanjiekeM3U8(m3u8, keyPairs, videoURL)
			extra["m3u8_text"] = m3u8
			extra["m3u8_url"] = videoURL
			if len(keyPairs) > 0 {
				extra["key_pairs"] = keyPairs
			}
			streamURL = dataM3U8URL(m3u8)
			format = "m3u8"
		}
		stream := extractor.Stream{Quality: "best", URLs: []string{streamURL}, Format: format, Headers: mediaHeaders(jar, referer)}
		if format == "m3u8" {
			stream.NeedMerge = true
		}
		entries = append(entries, &extractor.MediaInfo{Site: "sanjieke", Title: title, Streams: map[string]extractor.Stream{"default": stream}, Extra: extra})
	}
	entries = append(entries, fileEntriesFromValue(jar, referer, raw, prefix)...)
	children := append(append([]node{}, resp.Data.Nodes...), resp.Data.Children...)
	entries = append(entries, collectEntries(c, jar, key, children, prefix)...)
	return entries
}

func authVideoURL(c *util.Client, jar http.CookieJar, videoID, referer string) string {
	apiURL := urlVideoAuth + "?cid=" + url.QueryEscape(videoID)
	body, err := c.GetString(apiURL, studyHeaders(jar, referer))
	if err != nil {
		return ""
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(body), &resp) != nil || resp.Code != 200 {
		return ""
	}
	return strings.TrimSpace(resp.Data.URL)
}

func fetchMediaM3U8(c *util.Client, jar http.CookieJar, mediaURL, referer string) (string, map[string]string) {
	if !strings.HasPrefix(mediaURL, "https://service.sanjieke.cn/video/media/") {
		return "", nil
	}
	body, err := c.GetString(mediaURL, mediaHeaders(jar, referer))
	if err != nil {
		return "", nil
	}
	text := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n"))
	if strings.HasPrefix(text, "#EXTM3U") {
		return text, fetchM3U8Keys(c, jar, text, mediaURL, referer)
	}
	var resp struct {
		Status   any               `json:"status"`
		M3U8Text string            `json:"m3u8Text"`
		KeyPairs map[string]string `json:"keyPairs"`
	}
	if json.Unmarshal([]byte(body), &resp) != nil {
		return "", nil
	}
	text = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(resp.M3U8Text, "\r\n", "\n"), "\r", "\n"))
	if text == "" {
		return "", nil
	}
	if len(resp.KeyPairs) == 0 {
		resp.KeyPairs = fetchM3U8Keys(c, jar, text, mediaURL, referer)
	}
	return text, resp.KeyPairs
}

func pickVideoURL(vc videoContent) string {
	for _, v := range append(vc.Ratios, vc.Items...) {
		if strings.TrimSpace(v.URL) != "" {
			return strings.TrimSpace(v.URL)
		}
	}
	return strings.TrimSpace(vc.URL)
}

var (
	classViewRe = regexp.MustCompile(`/course/view/cid/(\d+)/course_id/(\d+)`)
	studyRe     = regexp.MustCompile(`study\.sanjieke\.cn/(?:course|study)/(\d+)/(\d+)`)
)

func parseCourseKey(raw string) courseKey {
	var out courseKey
	if m := classViewRe.FindStringSubmatch(raw); len(m) > 2 {
		out.classID, out.courseID = m[1], m[2]
	}
	if m := studyRe.FindStringSubmatch(raw); len(m) > 2 {
		out.projectID, out.courseID = m[1], m[2]
	}
	if u, err := url.Parse(raw); err == nil {
		q := u.Query()
		out.classID = firstNonEmpty(out.classID, q.Get("cid"))
		out.courseID = firstNonEmpty(out.courseID, q.Get("course_id"), q.Get("courseId"))
		out.projectID = firstNonEmpty(out.projectID, q.Get("project_id"), q.Get("projectId"))
	}
	if out.projectID == "" {
		out.projectID = "0"
	}
	return out
}

func buildPageReferer(key courseKey, nodeID string) string {
	base := fmt.Sprintf(urlStudyPage, key.projectID, key.courseID)
	if nodeID != "" {
		return base + "/" + nodeID
	}
	return base
}
func studyHeaders(jar http.CookieJar, referer string) map[string]string {
	return authHeaders(jar, referer, urlOrigin)
}
func classroomHeaders(jar http.CookieJar) map[string]string {
	return authHeaders(jar, urlClassroom, urlClassroomOrigin)
}
func mediaHeaders(jar http.CookieJar, referer string) map[string]string {
	h := authHeaders(jar, referer, urlOrigin)
	h["Accept"] = "*/*"
	h["Sec-Fetch-Dest"] = "empty"
	h["Sec-Fetch-Mode"] = "cors"
	h["Sec-Fetch-Site"] = "same-site"
	return h
}

func authHeaders(jar http.CookieJar, referer, origin string) map[string]string {
	cookie := cookieHeader(jar, urlReferer, urlClassroom, urlUserInfo)
	h := map[string]string{"x-domain-prefix": domainPrefix, "sjk-apikey": apiKey, "User-Agent": browserUA, "X-Requested-With": "XMLHttpRequest", "Accept": "application/json, text/plain, */*", "Origin": origin, "Referer": referer, "cookie": cookie}
	if tok := sjkToken(cookie); tok != "" {
		h["Authorization"] = "Bearer " + tok
	}
	return h
}

func cookieHeader(jar http.CookieJar, rawURLs ...string) string {
	seen, vals := map[string]bool{}, []string{}
	for _, raw := range rawURLs {
		if u, err := url.Parse(raw); err == nil {
			for _, c := range jar.Cookies(u) {
				if !seen[c.Name] {
					seen[c.Name] = true
					vals = append(vals, c.Name+"="+c.Value)
				}
			}
		}
	}
	return strings.Join(vals, "; ")
}

func sjkToken(cookie string) string {
	for _, part := range strings.Split(cookie, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 && (kv[0] == "sjk_token" || kv[0] == "_sjk_jwt") {
			return strings.TrimSpace(kv[1])
		}
	}
	return ""
}
func extractProjectID(raw string) string {
	if m := regexp.MustCompile(`study\.sanjieke\.cn/(?:course|study)/(\d+)/\d+`).FindStringSubmatch(raw); len(m) > 1 {
		return m[1]
	}
	return ""
}
func anyString(v any) string {
	if v == nil {
		return ""
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
func nonEmpty(vals []string) []string {
	out := vals[:0]
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}
func pickFormat(u string) string {
	lower := strings.ToLower(u)
	if strings.Contains(lower, ".m3u8") || strings.HasPrefix(lower, "data:application/vnd.apple.mpegurl") {
		return "m3u8"
	}
	return "mp4"
}

type sjkFile struct {
	Title  string
	URL    string
	Format string
}

func attachmentEntries(c *util.Client, jar http.CookieJar, key courseKey) []*extractor.MediaInfo {
	body, err := c.GetString(fmt.Sprintf(urlAttachmentList, key.projectID, key.courseID), studyHeaders(jar, buildPageReferer(key, "")))
	if err != nil {
		return nil
	}
	var raw map[string]any
	if json.Unmarshal([]byte(body), &raw) != nil {
		return nil
	}
	if fmt.Sprint(raw["code"]) != "200" {
		return nil
	}
	return fileEntriesFromValue(jar, buildPageReferer(key, ""), raw["data"], nil)
}

func fileEntriesFromValue(jar http.CookieJar, referer string, value any, prefix []string) []*extractor.MediaInfo {
	files := collectSanjiekeFiles(value, prefix)
	entries := make([]*extractor.MediaInfo, 0, len(files))
	seen := map[string]bool{}
	for i, file := range files {
		if file.URL == "" || seen[file.URL] {
			continue
		}
		seen[file.URL] = true
		title := firstNonEmpty(file.Title, fmt.Sprintf("(%d)--资料", i+1))
		format := firstNonEmpty(file.Format, fileExt(file.URL), "bin")
		stream := extractor.Stream{Quality: format, URLs: []string{file.URL}, Format: format, Headers: authHeaders(jar, referer, urlOrigin)}
		entries = append(entries, &extractor.MediaInfo{Site: "sanjieke", Title: title, Streams: map[string]extractor.Stream{"default": stream}, Extra: map[string]any{"type": "file", "file_url": file.URL}})
	}
	return entries
}

func collectSanjiekeFiles(value any, prefix []string) []sjkFile {
	var files []sjkFile
	var walk func(any, []int)
	walk = func(v any, index []int) {
		switch x := v.(type) {
		case map[string]any:
			if f := parseSanjiekeFile(x, prefix, index); f.URL != "" {
				files = append(files, f)
			}
			for _, child := range x {
				walk(child, index)
			}
		case []any:
			for i, child := range x {
				walk(child, append(append([]int{}, index...), i+1))
			}
		}
	}
	walk(value, nil)
	return files
}

func parseSanjiekeFile(m map[string]any, prefix []string, index []int) sjkFile {
	rawURL := ""
	for _, key := range []string{"download_url", "downloadUrl", "file_url", "fileUrl", "attachment_url", "attachmentUrl", "url", "path", "src"} {
		if s := anyString(m[key]); strings.TrimSpace(s) != "" {
			rawURL = normalizeFileURL(s)
			break
		}
	}
	if rawURL == "" || isVideoURL(rawURL) || !strings.HasPrefix(strings.ToLower(rawURL), "http") {
		return sjkFile{}
	}
	format := strings.Trim(strings.ToLower(firstNonEmpty(anyString(m["file_fmt"]), anyString(m["format"]), anyString(m["ext"]), anyString(m["suffix"]), fileExt(rawURL))), ". ")
	if format == "" {
		format = "bin"
	}
	name := firstNonEmpty(anyString(m["file_name"]), anyString(m["fileName"]), anyString(m["name"]), anyString(m["title"]), urlBaseName(rawURL), "资料")
	if strings.HasSuffix(strings.ToLower(name), "."+format) {
		name = name[:len(name)-len(format)-1]
	}
	if joined := strings.Join(nonEmpty(prefix), " / "); joined != "" {
		name = joined + " / " + name
	}
	return sjkFile{Title: fmt.Sprintf("(%s)--%s", indexString(index), name), URL: rawURL, Format: format}
}

func fetchM3U8Keys(c *util.Client, jar http.CookieJar, m3u8Text, sourceURL, referer string) map[string]string {
	keys := map[string]string{}
	seen := map[string]bool{}
	for _, match := range regexp.MustCompile(`URI="(.*?)"`).FindAllStringSubmatch(m3u8Text, -1) {
		if len(match) != 2 || strings.TrimSpace(match[1]) == "" {
			continue
		}
		uri := strings.TrimSpace(match[1])
		keyURL := resolveAgainst(uri, sourceURL)
		if keyURL == "" || seen[keyURL] || strings.HasPrefix(strings.ToLower(keyURL), "data:") {
			continue
		}
		seen[keyURL] = true
		data, err := c.GetBytes(keyURL, mediaHeaders(jar, referer))
		if err != nil || len(data) == 0 {
			continue
		}
		hexKey := strings.ToLower(fmt.Sprintf("%x", data))
		keys[uri] = hexKey
		keys[keyURL] = hexKey
	}
	return keys
}

func prepareSanjiekeM3U8(text string, keyPairs map[string]string, sourceURL string) string {
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"), "\n")
	uriRe := regexp.MustCompile(`URI="(.*?)"`)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#EXT-X-KEY:") {
			lines[i] = uriRe.ReplaceAllStringFunc(line, func(match string) string {
				parts := uriRe.FindStringSubmatch(match)
				if len(parts) != 2 {
					return match
				}
				uri := strings.TrimSpace(parts[1])
				resolved := resolveAgainst(uri, sourceURL)
				if keyHex := firstNonEmpty(keyPairs[uri], keyPairs[resolved]); keyHex != "" {
					if b, err := hexBytes(keyHex); err == nil && len(b) > 0 {
						return `URI="data:application/octet-stream;base64,` + base64.StdEncoding.EncodeToString(b) + `"`
					}
				}
				return `URI="` + resolved + `"`
			})
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		lines[i] = resolveAgainst(trimmed, sourceURL)
	}
	return strings.Join(lines, "\n")
}

func resolveAgainst(raw, source string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "data:") {
		return raw
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	base, err := url.Parse(source)
	if err != nil {
		return raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return base.ResolveReference(ref).String()
}

func hexBytes(raw string) ([]byte, error) {
	raw = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(raw), "0x"), "0X")
	if len(raw)%2 == 1 {
		raw = "0" + raw
	}
	out := make([]byte, len(raw)/2)
	for i := range out {
		n, err := strconv.ParseUint(raw[i*2:i*2+2], 16, 8)
		if err != nil {
			return nil, err
		}
		out[i] = byte(n)
	}
	return out, nil
}

func dataM3U8URL(text string) string {
	return "data:application/vnd.apple.mpegurl;base64," + base64.StdEncoding.EncodeToString([]byte(text))
}

func normalizeFileURL(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\/`, `/`))
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	if strings.HasPrefix(raw, "/") {
		return urlOrigin + raw
	}
	return strings.ReplaceAll(raw, " ", "%20")
}

func isVideoURL(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.Contains(lower, ".m3u8") || strings.Contains(lower, ".mp4") || strings.Contains(lower, ".flv") || strings.Contains(lower, ".m4v") || strings.Contains(lower, ".mov")
}

func fileExt(raw string) string {
	u, err := url.Parse(raw)
	path := raw
	if err == nil {
		path = u.Path
	}
	if idx := strings.LastIndex(path, "."); idx >= 0 && idx+1 < len(path) {
		return strings.ToLower(path[idx+1:])
	}
	return ""
}

func urlBaseName(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	path := strings.Trim(u.Path, "/")
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		path = path[idx+1:]
	}
	return path
}

func indexString(index []int) string {
	if len(index) == 0 {
		return "1"
	}
	parts := make([]string, len(index))
	for i, n := range index {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ".")
}

func toInt(v any, fallback int) int {
	n, err := strconv.Atoi(anyString(v))
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
