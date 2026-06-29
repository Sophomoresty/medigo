// Package nmkjxy implements an extractor for nmkjxy.com (柠檬云课堂).
package nmkjxy

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

const (
	referer               = "https://www.nmkjxy.com/"
	origin                = "https://www.nmkjxy.com"
	check_login_url       = "https://api.nmkjxy.com/api/V520/RecentCourse?PageSize=1&PageIndex=1&RecentMonth=false&status=1"
	course_url            = "https://api.nmkjxy.com/api/V520/RecentCourse?PageSize=%s&PageIndex=%s&RecentMonth=false&status=1"
	product_url           = "https://api.nmkjxy.com/api/product/%s"
	video_list_url        = "https://api.nmkjxy.com/api/video/%s"
	courseware_url        = "https://api.nmkjxy.com/api/V310/Courseware/%s"
	legacy_courseware_url = "https://api.nmkjxy.com/api/Courseware/%s"
	recorded_video_url    = "https://apim.ningmengyun.com/api/MyOrder/RecordedVideoCourse?orderSn=%s&productId=%s"
	video_play_url        = "https://apim.ningmengyun.com/api/MyOrder/VideoPlayed?courseId=%s&videoSn=%s"
	video_played_url      = "https://apim.ningmengyun.com/api/MyOrder/VideoPlayed"
)

var patterns = []string{`(?:[\w-]+\.)?nmkjxy\.com/`}

var (
	materialFileExts = map[string]struct{}{
		"pdf": {}, "ppt": {}, "pptx": {}, "doc": {}, "docx": {}, "xls": {}, "xlsx": {},
		"zip": {}, "rar": {}, "7z": {}, "jpg": {}, "jpeg": {}, "png": {},
	}
	skipMaterialFileExts = map[string]struct{}{
		"vtt": {}, "srt": {}, "ass": {}, "ssa": {}, "json": {},
	}
	materialFileKeys = []string{
		"contentFilePath", "coursewarePath", "coursewareUrl", "handoutPath", "handoutUrl",
		"lecturePath", "lectureUrl", "materialPath", "materialUrl", "attachmentPath",
		"attachmentUrl", "filePath", "fileUrl", "downloadUrl", "path",
	}
	materialNameKeys = []string{
		"contentFileName", "coursewareName", "handoutName", "lectureName", "materialName",
		"attachmentName", "fileName", "name", "title",
	}
)

func init() {
	extractor.Register(&Nmkjxy{}, extractor.SiteInfo{Name: "Nmkjxy", URL: "nmkjxy.com", NeedAuth: true})
}

type Nmkjxy struct{}

func (n *Nmkjxy) Patterns() []string { return patterns }

func (n *Nmkjxy) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("nmkjxy requires login cookies")
	}
	c := util.NewClient()
	c.SetCookieJar(opts.Cookies)
	h := headers(opts.Cookies)
	downloadH := downloadHeaders(opts.Cookies, h)
	if err := checkCookie(c, h); err != nil {
		return nil, err
	}

	cid := parseCID(rawURL)
	orderSN := ""
	var courses []map[string]any
	if cid == "" {
		courses = fetchCourseList(c, h)
		if len(courses) == 0 {
			return nil, fmt.Errorf("cannot parse nmkjxy courseId/productId from URL and course list is empty")
		}
		cid = firstText(courses[0], "course_id", "productId", "prodId", "courseId", "id")
		orderSN = firstText(courses[0], "order_sn", "orderSn", "orderSN")
	}
	if cid == "" {
		return nil, fmt.Errorf("cannot parse nmkjxy courseId/productId from URL")
	}

	product, _ := requestJSON(c, fmt.Sprintf(product_url, cid), h)
	productData := dataMap(product)
	title := firstText(productData, "prodName", "name", "productName", "title")
	orderSN = first(orderSN, firstText(productData, "orderSn", "orderSN"))
	if orderSN == "" {
		if courses == nil {
			courses = fetchCourseList(c, h)
		}
		for _, course := range courses {
			if firstText(course, "course_id", "productId", "prodId", "courseId", "id") != cid {
				continue
			}
			orderSN = firstText(course, "order_sn", "orderSn", "orderSN")
			if title == "" {
				title = firstText(course, "title", "prodName", "productName", "courseName", "name")
			}
			break
		}
	}
	if title == "" {
		title = "nmkjxy_" + cid
	}

	var videoListErr error
	listJSON, err := requestJSON(c, fmt.Sprintf(video_list_url, cid), h)
	if err != nil {
		videoListErr = fmt.Errorf("nmkjxy video list: %w", err)
	}
	items := iterItems(listJSON)
	if len(items) == 0 && orderSN != "" {
		if recorded, err := requestJSON(c, fmt.Sprintf(recorded_video_url, url.QueryEscape(orderSN), url.QueryEscape(cid)), h); err == nil {
			items = iterItems(recorded)
		}
	}
	var entries []*extractor.MediaInfo
	seen := map[string]bool{}
	for i, item := range items {
		vi := parseVideo(item, cid, i+1)
		if vi.VideoSN == "" && vi.VideoID == "" {
			continue
		}
		if seen[vi.VideoSN+":"+vi.VideoID] {
			continue
		}
		seen[vi.VideoSN+":"+vi.VideoID] = true
		play, _ := requestJSON(c, fmt.Sprintf(video_play_url, cid, url.QueryEscape(first(vi.VideoSN, vi.VideoID))), h)
		playData := dataMap(play)
		picked := pickPlayInfo(playData["playInfoList"], qualityFromOpts(opts))
		playURL := normalizePlayURL(firstText(picked, "playURL", "playUrl", "url"))
		if playURL == "" {
			continue
		}
		format := pickFormat(playURL)
		stream := extractor.Stream{Quality: firstText(picked, "definition"), URLs: []string{playURL}, Format: format, Size: sizeBytes(picked["size"]), NeedMerge: format == "m3u8", Headers: cloneHeaders(downloadH)}
		if stream.Quality == "" {
			stream.Quality = "best"
		}
		extra := map[string]any{"video_id": vi.VideoID, "video_sn": vi.VideoSN, "video_num": cid, "video_nid": vi.VideoNID}
		if vi.VideoSN != "" {
			extra["mark_played_url"] = video_played_url
			extra["mark_played_payload"] = map[string]string{"videoNum": cid, "videoSn": vi.VideoSN, "videoNId": vi.VideoNID}
		}
		entries = append(entries, &extractor.MediaInfo{Site: "nmkjxy", Title: vi.Name, Streams: map[string]extractor.Stream{"best": stream}, Subtitles: subtitles(item, playData), Extra: extra})
	}
	courseware := fetchCourseware(c, h, cid, downloadH)
	for _, cw := range courseware {
		if cw == nil || len(cw.Streams) == 0 && len(cw.Entries) == 0 {
			continue
		}
		entries = append(entries, cw)
	}
	if len(entries) == 0 {
		if videoListErr != nil {
			return nil, videoListErr
		}
		return nil, fmt.Errorf("nmkjxy: no playable videos or courseware files found")
	}
	return &extractor.MediaInfo{Site: "nmkjxy", Title: sanitize(title), Entries: entries}, nil
}

type videoInfo struct{ VideoID, VideoSN, VideoNID, Name string }

func parseVideo(m map[string]any, cid string, fallback int) videoInfo {
	videoID := firstText(m, "videoId", "vodId")
	videoSN := firstText(m, "videoSn", "videoSN", "sectionSn", "id")
	parts := chapterIndexParts(m)
	base := firstText(m, "title", "videoTitle", "name", "videoName", "sectionName")
	if base == "" {
		base = first(videoSN, videoID, fmt.Sprint(fallback))
	}
	return videoInfo{VideoID: videoID, VideoSN: first(videoSN, videoID), VideoNID: firstText(m, "videoNId", "id"), Name: sanitize(fmt.Sprintf("[%s]--%s", formatIndex(parts, fallback), base))}
}

func requestJSON(c *util.Client, api string, h map[string]string) (any, error) {
	body, err := c.GetString(api, h)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return nil, err
	}
	return v, nil
}

func iterItems(v any) []map[string]any {
	var out []map[string]any
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			if looksVideo(t) {
				out = append(out, t)
			}
			for _, k := range []string{"data", "rows", "list", "items", "result"} {
				if y, ok := t[k]; ok {
					walk(y)
				}
			}
		}
	}
	walk(v)
	return out
}
func looksVideo(m map[string]any) bool {
	return firstText(m, "videoId", "vodId", "videoSn", "videoSN") != ""
}
func dataMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		if d, ok := m["data"].(map[string]any); ok {
			return d
		}
		return m
	}
	return map[string]any{}
}

func pickPlayInfo(v any, quality string) map[string]any {
	list, ok := v.([]any)
	if !ok {
		return map[string]any{}
	}
	defs := map[string]map[string]any{}
	var playable []map[string]any
	for _, e := range list {
		if m, ok := e.(map[string]any); ok {
			if firstText(m, "playURL", "playUrl", "url") != "" {
				playable = append(playable, m)
				defs[strings.ToUpper(firstText(m, "definition"))] = m
			}
		}
	}
	for _, d := range preferredDefinitions(quality) {
		if m := defs[d]; m != nil {
			return m
		}
	}
	var best map[string]any
	for _, m := range playable {
		if best == nil || sizeBytes(m["size"]) > sizeBytes(best["size"]) {
			best = m
		}
	}
	if best == nil {
		return map[string]any{}
	}
	return best
}

func preferredDefinitions(quality string) []string {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "sd", "ld", "fd":
		return []string{"LD", "FD", "SD", "HD", "OD"}
	case "hd":
		return []string{"SD", "HD", "LD", "FD", "OD"}
	case "fhd", "od", "4k", "2k":
		return []string{"HD", "OD", "SD", "LD", "FD"}
	default:
		return []string{"HD", "OD", "SD", "LD", "FD"}
	}
}

func checkCookie(c *util.Client, h map[string]string) error {
	body, err := c.GetString(check_login_url, h)
	if err != nil {
		return fmt.Errorf("nmkjxy login check: %w", err)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return fmt.Errorf("nmkjxy login check parse: %w", err)
	}
	code := firstText(resp, "code", "status")
	if boolValue(resp["success"]) || boolValue(resp["Success"]) || code == "0" || code == "200" || resp["data"] != nil {
		return nil
	}
	return fmt.Errorf("nmkjxy login check rejected token")
}

func fetchCourseList(c *util.Client, h map[string]string) []map[string]any {
	out := []map[string]any{}
	seen := map[string]bool{}
	for page := 0; page < 50; page++ {
		v, err := requestJSON(c, fmt.Sprintf(course_url, "20", strconv.Itoa(page)), h)
		if err != nil {
			break
		}
		items := iterCourseItems(v)
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			id := firstText(item, "prodId", "productId", "courseId", "id")
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			item["course_id"] = id
			item["order_sn"] = firstText(item, "orderSn", "orderSN")
			item["title"] = firstText(item, "prodName", "productName", "courseName", "title", "name")
			out = append(out, item)
		}
		if len(items) < 20 {
			break
		}
	}
	return out
}

func iterCourseItems(v any) []map[string]any {
	var out []map[string]any
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			if firstText(t, "prodId", "productId", "courseId", "id") != "" && firstText(t, "prodName", "productName", "courseName", "title", "name", "orderSn", "orderSN") != "" {
				out = append(out, t)
			}
			for _, k := range []string{"data", "rows", "list", "items", "result"} {
				if y, ok := t[k]; ok {
					walk(y)
				}
			}
		}
	}
	walk(v)
	return out
}

func fetchCourseware(c *util.Client, h map[string]string, cid string, downloadH map[string]string) []*extractor.MediaInfo {
	for _, api := range []string{fmt.Sprintf(courseware_url, cid), fmt.Sprintf(legacy_courseware_url, cid)} {
		v, err := requestJSON(c, api, h)
		if err != nil {
			continue
		}
		data := dataAny(v)
		if strings.Contains(api, "/V310/Courseware/") {
			if out := parseCoursewareGroupsWithHeaders(data, 1, downloadH); len(out) > 0 {
				return out
			}
		} else if out := parseLegacyCoursewareFilesWithHeaders(data, 1, downloadH); len(out) > 0 {
			return out
		}
	}
	return nil
}

func dataAny(v any) any {
	if m, ok := v.(map[string]any); ok {
		if d, ok := m["data"]; ok {
			return d
		}
	}
	return v
}

func parseCoursewareGroups(groups any, categoryIndex int) []*extractor.MediaInfo {
	return parseCoursewareGroupsWithHeaders(groups, categoryIndex, nil)
}

func parseCoursewareGroupsWithHeaders(groups any, categoryIndex int, downloadH map[string]string) []*extractor.MediaInfo {
	items := asList(groups)
	if len(items) == 0 {
		if m, ok := groups.(map[string]any); ok && isMaterialSource(m) {
			items = []any{m}
		}
	}
	if len(items) == 0 {
		return nil
	}
	var out []*extractor.MediaInfo
	seen := map[string]bool{}
	for i, item := range items {
		group, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, parseCoursewareGroup(group, categoryIndex, i+1, seen, downloadH)...)
	}
	return out
}

func parseCoursewareGroup(group map[string]any, categoryIndex, fallbackIndex int, seen map[string]bool, downloadH map[string]string) []*extractor.MediaInfo {
	var out []*extractor.MediaInfo
	files := asList(group["files"])
	if len(files) == 0 {
		return parseMaterialFiles(group, formatIndexPrefix(strconv.Itoa(categoryIndex), strconv.Itoa(fallbackIndex), "1"), seen, downloadH)
	}
	for fileIndex, item := range files {
		source, ok := item.(map[string]any)
		if !ok {
			continue
		}
		entry := parseFileInfoWithHeaders(source, formatIndexPrefix(strconv.Itoa(categoryIndex), strconv.Itoa(fallbackIndex), strconv.Itoa(fileIndex+1)), downloadH)
		if entry == nil {
			out = append(out, parseMaterialFiles(source, formatIndexPrefix(strconv.Itoa(categoryIndex), strconv.Itoa(fallbackIndex), strconv.Itoa(fileIndex+1)), seen, downloadH)...)
			continue
		}
		if url := firstStreamURL(entry); url != "" {
			if seen[url] {
				continue
			}
			seen[url] = true
		}
		out = append(out, entry)
	}
	return out
}

func parseMaterialFiles(source any, indexPrefix string, seen map[string]bool, downloadH map[string]string) []*extractor.MediaInfo {
	if seen == nil {
		seen = map[string]bool{}
	}
	var out []*extractor.MediaInfo
	for _, candidate := range iterMaterialFileSources(source) {
		entry := parseFileInfoWithHeaders(candidate, indexPrefix, downloadH)
		if entry == nil {
			continue
		}
		if url := firstStreamURL(entry); url != "" {
			if seen[url] {
				continue
			}
			seen[url] = true
		}
		out = append(out, entry)
	}
	return out
}

func parseLegacyCoursewareFiles(files any, categoryIndex int) []*extractor.MediaInfo {
	return parseLegacyCoursewareFilesWithHeaders(files, categoryIndex, nil)
}

func parseLegacyCoursewareFilesWithHeaders(files any, categoryIndex int, downloadH map[string]string) []*extractor.MediaInfo {
	items := asList(files)
	if len(items) == 0 {
		if m, ok := files.(map[string]any); ok && isMaterialSource(m) {
			items = []any{m}
		}
	}
	if len(items) == 0 {
		return nil
	}
	grouped := map[string][]map[string]any{}
	order := make([]string, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		chapter := firstText(m, "chapterSn", "chapterNum", "chapterIndex")
		if chapter == "" {
			chapter = "1"
		}
		if _, ok := grouped[chapter]; !ok {
			order = append(order, chapter)
		}
		grouped[chapter] = append(grouped[chapter], m)
	}

	var out []*extractor.MediaInfo
	seen := map[string]bool{}
	for _, chapter := range order {
		for fileIndex, item := range grouped[chapter] {
			entry := parseFileInfoWithHeaders(item, formatIndexPrefix(strconv.Itoa(categoryIndex), chapter, strconv.Itoa(fileIndex+1)), downloadH)
			if entry == nil {
				out = append(out, parseMaterialFiles(item, formatIndexPrefix(strconv.Itoa(categoryIndex), chapter, strconv.Itoa(fileIndex+1)), seen, downloadH)...)
				continue
			}
			if url := firstStreamURL(entry); url != "" {
				if seen[url] {
					continue
				}
				seen[url] = true
			}
			out = append(out, entry)
		}
	}
	if len(out) > 0 {
		return out
	}

	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		entry := parseFileInfoWithHeaders(m, formatIndexPrefix(strconv.Itoa(categoryIndex), strconv.Itoa(i+1)), downloadH)
		if entry == nil {
			out = append(out, parseMaterialFiles(m, formatIndexPrefix(strconv.Itoa(categoryIndex), strconv.Itoa(i+1)), seen, downloadH)...)
			continue
		}
		if url := firstStreamURL(entry); url != "" {
			if seen[url] {
				continue
			}
			seen[url] = true
		}
		out = append(out, entry)
	}
	return out
}

func iterMaterialFileSources(source any) []map[string]any {
	var out []map[string]any
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case []any:
			for _, item := range t {
				walk(item)
			}
		case map[string]any:
			for _, key := range materialFileKeys {
				raw := strings.TrimSpace(fmt.Sprint(t[key]))
				if raw == "" || raw == "<nil>" {
					continue
				}
				out = append(out, map[string]any{
					"fileUrl":  raw,
					"fileName": firstText(t, materialNameKeys...),
					"format":   fileExtFromURL(raw),
					"size":     firstAny(t, "fileSize", "size"),
				})
			}
			if isMaterialSource(t) {
				out = append(out, t)
			}
			for _, value := range t {
				switch value.(type) {
				case []any, map[string]any:
					walk(value)
				}
			}
		}
	}
	walk(source)
	return out
}

func isMaterialSource(source map[string]any) bool {
	if source == nil {
		return false
	}
	for _, key := range materialFileKeys {
		if strings.TrimSpace(fmt.Sprint(source[key])) != "" && fmt.Sprint(source[key]) != "<nil>" {
			return true
		}
	}
	for _, key := range materialNameKeys {
		if strings.TrimSpace(fmt.Sprint(source[key])) != "" && fmt.Sprint(source[key]) != "<nil>" {
			return true
		}
	}
	return false
}

func parseFileInfo(source map[string]any, indexPrefix string) *extractor.MediaInfo {
	return parseFileInfoWithHeaders(source, indexPrefix, nil)
}

func parseFileInfoWithHeaders(source map[string]any, indexPrefix string, downloadH map[string]string) *extractor.MediaInfo {
	if source == nil {
		return nil
	}
	rawURL := firstAnyText(source, "url", "fileUrl", "downloadUrl", "path", "filePath", "contentFilePath", "coursewarePath", "coursewareUrl", "handoutPath", "handoutUrl", "lecturePath", "lectureUrl", "materialPath", "materialUrl", "attachmentPath", "attachmentUrl")
	if rawURL == "" {
		return nil
	}
	fileURL := absURL(rawURL)
	name := firstAnyText(source, "fileName", "contentFileName", "coursewareName", "handoutName", "lectureName", "materialName", "attachmentName", "name", "title")
	format := firstAnyText(source, "format", "ext")
	if format == "" {
		format = fileExtFromURL(fileURL)
	}
	if format == "" && name != "" {
		format = fileExtFromURL(name)
	}
	format = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(format)), ".")
	if format == "" {
		format = "bin"
	}
	if !isMaterialFileExt(format) {
		return nil
	}
	if name == "" {
		name = fileNameFromURL(fileURL)
	}
	if name == "" {
		name = "课件"
	}
	name = stripFileExt(name, format)
	name = sanitize(fmt.Sprintf("(%s)--%s", indexPrefix, name))
	st := extractor.Stream{
		Quality: "source",
		URLs:    []string{fileURL},
		Format:  format,
		Size:    sizeBytes(firstAny(source, "fileSize", "size")),
		Headers: cloneHeaders(downloadH),
	}
	if len(st.Headers) == 0 {
		st.Headers = map[string]string{"Referer": referer, "Origin": origin}
	}
	return &extractor.MediaInfo{Site: "nmkjxy", Title: name, Streams: map[string]extractor.Stream{"file": st}, Extra: map[string]any{"kind": "file", "file_fmt": format, "file_url": fileURL}}
}

func asList(v any) []any {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		return t
	case map[string]any:
		for _, key := range []string{"groups", "files", "list", "items", "rows", "result", "data"} {
			if vv, ok := t[key]; ok {
				if list, ok := vv.([]any); ok {
					return list
				}
				if m, ok := vv.(map[string]any); ok {
					if list := asList(m); len(list) > 0 {
						return list
					}
					if isMaterialSource(m) {
						return []any{m}
					}
				}
			}
		}
		if isMaterialSource(t) {
			return []any{t}
		}
	}
	return nil
}

func firstAny(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
				return v
			}
		}
	}
	return nil
}

func firstAnyText(m map[string]any, keys ...string) string {
	if v := firstAny(m, keys...); v != nil {
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return ""
}

func firstStreamURL(info *extractor.MediaInfo) string {
	if info == nil {
		return ""
	}
	for _, st := range info.Streams {
		if len(st.URLs) > 0 && strings.TrimSpace(st.URLs[0]) != "" {
			return strings.TrimSpace(st.URLs[0])
		}
	}
	return ""
}

func stripFileExt(name, fileFmt string) string {
	name = strings.TrimSpace(name)
	fileFmt = strings.Trim(strings.ToLower(strings.TrimSpace(fileFmt)), ".")
	if name == "" || fileFmt == "" {
		return name
	}
	if strings.HasSuffix(strings.ToLower(name), "."+fileFmt) {
		return strings.TrimSpace(name[:len(name)-len(fileFmt)-1])
	}
	return name
}

func isMaterialFileExt(fileFmt string) bool {
	fileFmt = strings.Trim(strings.ToLower(strings.TrimSpace(fileFmt)), ".")
	if fileFmt == "" {
		return false
	}
	if _, skip := skipMaterialFileExts[fileFmt]; skip {
		return false
	}
	_, ok := materialFileExts[fileFmt]
	return ok
}

func fileExtFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	return strings.Trim(strings.ToLower(ext(rawURL, "")), ".")
}

func formatIndexPrefix(values ...string) string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, ".")
}

func subtitles(raw, play map[string]any) []extractor.Subtitle {
	seen := map[string]bool{}
	var out []extractor.Subtitle
	for _, m := range []map[string]any{raw, play} {
		for _, k := range []string{"subtitlePath", "subtitleUrl", "subTitlePath", "subTitleUrl", "srtPath", "vttPath"} {
			if u := absURL(firstText(m, k)); u != "" && !seen[u] {
				format := fileExtFromURL(u)
				if format == "" {
					format = "srt"
				}
				if format != "srt" && format != "vtt" {
					continue
				}
				seen[u] = true
				out = append(out, extractor.Subtitle{Language: "字幕", URL: u, Format: format})
			}
		}
	}
	return out
}

func headers(j http.CookieJar) map[string]string {
	h := map[string]string{"Referer": referer, "Origin": origin, "Accept": "application/json, text/plain, */*", "Author": "ningmengyun"}
	if tok := tokenFromJar(j); tok != "" {
		h["Authorization"] = "Bearer " + tok
	}
	return h
}

func downloadHeaders(j http.CookieJar, base map[string]string) map[string]string {
	h := cloneHeaders(base)
	if h == nil {
		h = map[string]string{}
	}
	if h["Referer"] == "" {
		h["Referer"] = referer
	}
	if h["Origin"] == "" {
		h["Origin"] = origin
	}
	if cookie := cookieHeader(j); cookie != "" {
		h["Cookie"] = cookie
	}
	return h
}

func cookieHeader(j http.CookieJar) string {
	if j == nil {
		return ""
	}
	seen := map[string]bool{}
	parts := []string{}
	for _, host := range []string{"https://www.nmkjxy.com/", "https://api.nmkjxy.com/", "https://apim.ningmengyun.com/"} {
		u, _ := url.Parse(host)
		for _, ck := range j.Cookies(u) {
			if ck == nil || ck.Name == "" || ck.Value == "" || seen[ck.Name] {
				continue
			}
			seen[ck.Name] = true
			parts = append(parts, ck.Name+"="+ck.Value)
		}
	}
	return strings.Join(parts, "; ")
}

func cloneHeaders(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		if strings.TrimSpace(v) != "" {
			out[k] = v
		}
	}
	return out
}

func tokenFromJar(j http.CookieJar) string {
	if j == nil {
		return ""
	}
	fallback := ""
	for _, host := range []string{"https://www.nmkjxy.com/", "https://api.nmkjxy.com/", "https://apim.ningmengyun.com/"} {
		u, _ := url.Parse(host)
		for _, ck := range j.Cookies(u) {
			if t := parseToken(ck.Name, ck.Value); t != "" {
				if isTokenCookieName(ck.Name) {
					return t
				}
				if fallback == "" {
					fallback = t
				}
			}
		}
	}
	return fallback
}
func parseToken(name, val string) string {
	tokenCookie := isTokenCookieName(name)
	v := strings.TrimSpace(val)
	if strings.EqualFold(name, "Authorization") && strings.HasPrefix(strings.ToLower(v), "bearer ") {
		return strings.TrimSpace(v[7:])
	}
	if strings.HasPrefix(strings.ToLower(v), "token:") {
		v = strings.TrimSpace(v[6:])
	}
	if u, err := url.QueryUnescape(v); err == nil {
		v = u
	}
	if strings.HasPrefix(strings.TrimSpace(v), "{") {
		var m map[string]any
		if json.Unmarshal([]byte(v), &m) == nil {
			return firstText(m, "access_token", "accessToken", "token")
		}
	}
	if strings.HasPrefix(strings.ToLower(v), "bearer ") {
		v = strings.TrimSpace(v[7:])
	}
	if strings.Contains(v, "=") && !tokenCookie {
		return ""
	}
	if !tokenCookie {
		return ""
	}
	return strings.TrimSpace(v)
}

func isTokenCookieName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || strings.Contains(name, "csrf") || strings.Contains(name, "xsrf") {
		return false
	}
	switch name {
	case "authorization", "access_token", "accesstoken", "access-token":
		return true
	default:
		return strings.Contains(name, "token")
	}
}

var cidRe = regexp.MustCompile(`(?i)[?&](?:courseId|course_id|productId|prodId|cid|id)=([0-9]+)|/(?:course|product|detail|video)/([0-9]+)`)

func parseCID(raw string) string {
	if m := cidRe.FindStringSubmatch(raw); len(m) > 1 {
		for i := 1; i < len(m); i++ {
			if m[i] != "" {
				return m[i]
			}
		}
	}
	return ""
}
func chapterIndexParts(m map[string]any) []string {
	p := firstText(m, "parentChapterSn", "parentChapterNum", "bigChapterSn", "bigChapterNum", "stageSn", "stageNum", "moduleSn", "moduleNum")
	c := firstText(m, "chapterSn", "chapterNum", "sectionSn", "sectionNum", "unitSn", "unitNum")
	if c == "" {
		c = "1"
	}
	if p != "" {
		return []string{p, c}
	}
	return []string{c}
}
func formatIndex(parts []string, fallback int) string {
	if len(parts) == 0 {
		return fmt.Sprint(fallback)
	}
	return strings.Join(parts, ".")
}
func firstText(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}
func first(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
func boolValue(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		return s == "true" || s == "1" || s == "ok" || s == "success"
	case float64:
		return x != 0
	case int:
		return x != 0
	}
	return false
}
func absURL(s string) string {
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err == nil && u.IsAbs() {
		return s
	}
	b, _ := url.Parse(referer)
	r, _ := url.Parse(s)
	return b.ResolveReference(r).String()
}

func normalizePlayURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "#extm3u") || strings.Contains(low, "\n#extm3u") {
		return "data:application/vnd.apple.mpegurl;base64," + base64.StdEncoding.EncodeToString([]byte(s))
	}
	return absURL(s)
}

func ext(u, def string) string {
	p := strings.ToLower(strings.Split(strings.Split(u, "?")[0], "#")[0])
	if i := strings.LastIndex(p, "."); i >= 0 && i+1 < len(p) {
		return p[i+1:]
	}
	return def
}
func sizeBytes(v any) int64 {
	switch x := v.(type) {
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return n
		}
		if f, err := x.Float64(); err == nil {
			return int64(f)
		}
	case float64:
		return int64(x)
	case float32:
		return int64(x)
	case int:
		return int64(x)
	case int64:
		return x
	case int32:
		return int64(x)
	case int16:
		return int64(x)
	case int8:
		return int64(x)
	case uint:
		return int64(x)
	case uint64:
		return int64(x)
	case uint32:
		return int64(x)
	case uint16:
		return int64(x)
	case uint8:
		return int64(x)
	case string:
		s := strings.TrimSpace(strings.ToUpper(x))
		if s == "" {
			return 0
		}
		unitMul := float64(1)
		switch {
		case strings.HasSuffix(s, "KB") || strings.HasSuffix(s, "K"):
			unitMul = 1024
			s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(s, "KB"), "K"))
		case strings.HasSuffix(s, "MB") || strings.HasSuffix(s, "M"):
			unitMul = 1024 * 1024
			s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(s, "MB"), "M"))
		case strings.HasSuffix(s, "GB") || strings.HasSuffix(s, "G"):
			unitMul = 1024 * 1024 * 1024
			s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(s, "GB"), "G"))
		case strings.HasSuffix(s, "TB") || strings.HasSuffix(s, "T"):
			unitMul = 1024 * 1024 * 1024 * 1024
			s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(s, "TB"), "T"))
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int64(f * unitMul)
		}
	}
	return 0
}
func pickFormat(u string) string {
	low := strings.ToLower(strings.TrimSpace(u))
	if strings.Contains(low, ".m3u8") || strings.HasPrefix(low, "#extm3u") || strings.HasPrefix(low, "data:application/vnd.apple.mpegurl") || strings.Contains(low, "mpegurl") {
		return "m3u8"
	}
	return "mp4"
}

func qualityFromOpts(opts *extractor.ExtractOpts) string {
	if opts == nil {
		return ""
	}
	return opts.Quality
}

var badName = regexp.MustCompile(`[\\/:*?"<>|\r\n\t]+`)

func fileNameFromURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	base := u.Path
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if decoded, err := url.QueryUnescape(base); err == nil {
		base = decoded
	}
	return strings.TrimSpace(base)
}

func sanitize(s string) string {
	s = badName.ReplaceAllString(strings.TrimSpace(s), "_")
	if s == "" {
		return "未命名视频"
	}
	return s
}
