// Package plaso implements an extractor for plaso.cn courses.
package plaso

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

const (
	plasoDefaultBase = "https://www.plaso.cn"
	polyVideoURL     = "https://api.polyv.net/v2/video/5153980715/get-video-info"
	plasoPlayerURL   = "https://wwwr.plaso.cn/static/yxt/?appType=player&noUser=1&fileId="
)

const (
	checkCookiePath  = "/gt/servlet/group/getTeacherWillExpireGroupNum"
	coursePath       = "/gt/servlet/group/getGroupsByActive"
	courseListPath   = "/course/api/v1/m/package/student/list"
	packageListPath  = "/course/api/v1/m/package/list"
	historyListPath  = "/liveclassgo/api/v1/history/listRecord"
	homeworkListPath = "/homework/student/studentHomeworks"
	sharePath        = "/sc/nc/newGetShareInfo"
	filePath         = "/yxt/servlet/file/preview/getfileinfo"
	fileInfoPath     = "/yxt/servlet/file/getfileinfo"
	infoPath         = "/cs/xfilegroup/getXFileGroupInfo"
	packagePath      = "/course/api/v1/nct/m/package/task/list"
	dirInfoPath      = "/yxt/servlet/bigDir/getXfgTask"
	m3u8Path         = "/yxt/servlet/ali/getPlayInfo"
	polySignPath     = "/yxt/servlet/file/preview/getPolyvVidInfoV2"
	m3u8SignPath     = "/yxt/servlet/org/nc/polyvViewSign"
	stsPath          = "/yxt/servlet/stsHelper/stsInfo"
	stsPreviewPath   = "/yxt/servlet/stsHelper/preview/stsInfo"
)

var patterns = []string{
	`(?:[\w-]+\.)?plaso\.cn/`,
	`(?:[\w-]+\.)?plaso\.com/`,
	`(?:[\w-]+\.)?aiwenyun\.cn/`,
}

func init() {
	extractor.Register(&Plaso{}, extractor.SiteInfo{Name: "Plaso", URL: "plaso.cn", NeedAuth: true})
}

type Plaso struct{}

func (s *Plaso) Patterns() []string { return patterns }

type plasoEndpoints struct {
	base     string
	platform string
}

type plasoSession struct {
	client  *util.Client
	eps     plasoEndpoints
	headers map[string]string
	quality string
}

type fileItem struct {
	ID           string
	MyID         string
	Location     string
	LocationPath string
	Name         string
	Type         string
	URL          string
	Vid          string
	VideoID      string
	StorageID    string
	Chapter      string
	Index        []int
	Size         int64
	Raw          map[string]any
}

type courseInfo struct {
	ID        string
	Title     string
	Source    string
	History   bool
	Homework  bool
	Class     bool
	Origin    bool
	Price     string
	Purchased bool
	Raw       map[string]any
}

type plasoSource struct {
	URL        string
	Format     string
	Quality    string
	SourceType string
	M3U8Text   string
	AudioURL   string
	NeedMerge  bool
	Size       int64
	Extra      map[string]any
}

type plasoStaticResource struct {
	URL      string
	Path     string
	Host     string
	Entry    bool
	Required bool
}

var (
	cidRe   = regexp.MustCompile(`[?&](?:sfId|sfid|shareKey|fileId|fid|id|packageId|courseId|groupId|fileGroupId|dirId)=([\w.-]+)`)
	mediaRe = regexp.MustCompile(`https?://[^"'\s<>]+\.(?:m3u8|mp4|mp3)(?:\?[^"'\s<>]*)?`)
)

func (s *Plaso) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	sess := newPlasoSession(rawURL, safePlasoExtractOpts(opts))
	if staticMI := sess.resolveNativeStaticMedia(rawURL); staticMI != nil {
		return staticMI, nil
	}
	if directMI := sess.resolveDirectResourceMedia(rawURL); directMI != nil {
		return directMI, nil
	}
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("plaso requires login cookies")
	}
	if err := sess.checkCookie(); err != nil {
		return nil, err
	}
	cid := parseCID(rawURL)
	cidKind, resolvedCID := splitPlasoCourseID(cid)
	title := "plaso_" + firstNonEmpty(cid, "course")

	var files []fileItem
	if resolvedCID != "" {
		shareFiles, shareTitle := sess.fetchShareOrFile(resolvedCID)
		if len(shareFiles) > 0 {
			files = append(files, shareFiles...)
			title = firstNonEmpty(shareTitle, files[0].Name, title)
		}
	}

	var selected courseInfo
	if len(files) == 0 {
		courses := sess.fetchCourseList()
		if cid == "" {
			if len(courses) > 0 {
				return buildCourseListMedia(courses, sess.eps.platform), nil
			}
			return nil, fmt.Errorf("plaso: no course id in URL and no courses found")
		}
		for _, co := range courses {
			if sameID(co.ID, cid) {
				selected = co
				title = firstNonEmpty(co.Title, title)
				cidKind, resolvedCID = splitPlasoCourseID(co.ID)
				break
			}
		}
		files = append(files, sess.fetchFilesForCourse(cidKind, resolvedCID)...)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("plaso: no file/task records found from share/package APIs")
	}

	entries := make([]*extractor.MediaInfo, 0, len(files))
	seen := map[string]bool{}
	unresolved := 0
	for i, f := range files {
		mi := sess.resolveFile(f, i+1)
		if mi == nil {
			unresolved++
			continue
		}
		u := firstStreamURL(mi)
		key := firstNonEmpty(u, mi.Title)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		entries = append(entries, mi)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("plaso: no playable video/material URLs resolved from %d file records", unresolved)
	}

	extra := map[string]any{"course_id": cid, "resolved_id": resolvedCID, "platform": sess.eps.platform}
	if cidKind != "" {
		extra["course_kind"] = cidKind
	}
	if selected.Price != "" {
		extra["price"] = selected.Price
	}
	if selected.Purchased {
		extra["purchased"] = true
	}
	if unresolved > 0 {
		extra["unresolved_count"] = unresolved
	}
	return &extractor.MediaInfo{Site: "plaso", Title: clean(title), Entries: entries, Extra: extra}, nil
}

func safePlasoExtractOpts(opts *extractor.ExtractOpts) *extractor.ExtractOpts {
	if opts != nil {
		return opts
	}
	return &extractor.ExtractOpts{}
}

func newPlasoSession(rawURL string, opts *extractor.ExtractOpts) *plasoSession {
	c := util.NewClient()
	c.SetCookieJar(opts.Cookies)
	eps := newPlasoEndpoints(rawURL)
	return &plasoSession{client: c, eps: eps, headers: eps.headers(opts.Cookies), quality: opts.Quality}
}

func newPlasoEndpoints(rawURL string) plasoEndpoints {
	host := ""
	if u, err := url.Parse(rawURL); err == nil {
		host = strings.ToLower(u.Hostname())
	}
	switch {
	case strings.Contains(host, "aiwenyun.cn"):
		return plasoEndpoints{base: "https://www.aiwenyun.cn", platform: "aiwenyun"}
	case strings.Contains(host, "jhpy.plaso.cn"):
		return plasoEndpoints{base: "https://jhpy.plaso.cn", platform: "jhpy"}
	default:
		return plasoEndpoints{base: plasoDefaultBase, platform: "plaso"}
	}
}

func (e plasoEndpoints) url(path string) string { return e.base + path }

func (e plasoEndpoints) headers(jar http.CookieJar) map[string]string {
	h := map[string]string{
		"Accept":     "application/json, text/plain, */*",
		"Origin":     e.base,
		"Referer":    e.base,
		"referer":    e.base,
		"User-Agent": plasoUserAgent(),
	}
	if jar == nil {
		return h
	}
	baseURL, err := url.Parse(e.base)
	if err != nil {
		return h
	}
	cookies := jar.Cookies(baseURL)
	parts := make([]string, 0, len(cookies))
	for _, ck := range cookies {
		if ck == nil || ck.Name == "" {
			continue
		}
		parts = append(parts, ck.Name+"="+ck.Value)
		if strings.EqualFold(ck.Name, "access_token") && ck.Value != "" {
			h["access-token"] = ck.Value
		}
	}
	if len(parts) > 0 {
		h["Cookie"] = strings.Join(parts, "; ")
	}
	return h
}

func (s *plasoSession) checkCookie() error {
	v, err := s.postJSON(s.eps.url(checkCookiePath), map[string]string{})
	if err != nil {
		return fmt.Errorf("plaso cookie validation failed: %w", err)
	}
	code := findFirst(v, "code")
	if code != "" && code != "0" {
		return fmt.Errorf("plaso cookie validation failed: code=%s", code)
	}
	return nil
}

func (s *plasoSession) fetchCourseList() []courseInfo {
	apis := []struct {
		name  string
		url   string
		data  map[string]string
		paged bool
	}{
		{"course", s.eps.url(coursePath), map[string]string{"groupId": "", "search": ""}, false},
		{"student_package", s.eps.url(courseListPath), map[string]string{"pageSize": "200", "pageNum": "1", "search": ""}, true},
		{"package", s.eps.url(packageListPath), map[string]string{"pageSize": "200", "pageNum": "1", "search": ""}, true},
		{"history", s.eps.url(historyListPath), map[string]string{"dateFrom": "0", "dateTo": "2000000000000", "pageSize": "999", "pageNum": "1"}, false},
		{"homework", s.eps.url(homeworkListPath), map[string]string{"pageSize": "999", "pageNum": "1", "status": "5", "search": ""}, false},
	}
	seen := map[string]bool{}
	var out []courseInfo
	for _, api := range apis {
		for page := 1; page <= 5; page++ {
			data := cloneStringMap(api.data)
			if api.paged {
				data["pageNum"] = fmt.Sprint(page)
			}
			v, err := s.postJSON(api.url, data)
			if err != nil {
				break
			}
			added := 0
			if api.name == "history" {
				for _, f := range s.historyItemsFromPayload(v) {
					id := firstNonEmpty(f.ID, f.MyID)
					if id == "" || f.Name == "" || seen[api.name+":"+id] {
						continue
					}
					seen[api.name+":"+id] = true
					out = append(out, courseInfo{ID: "history_" + strings.TrimPrefix(id, "history_"), Title: prefixTitle("历史课堂_", f.Name), Source: api.name, History: true, Purchased: true, Raw: f.Raw})
					added++
				}
			}
			if api.name == "homework" {
				for _, co := range homeworkCoursesFromPayload(v) {
					if co.ID == "" || co.Title == "" || seen[api.name+":"+co.ID] {
						continue
					}
					seen[api.name+":"+co.ID] = true
					out = append(out, co)
					added++
				}
			}
			if api.name == "history" || api.name == "homework" {
				if !api.paged || added == 0 {
					break
				}
				continue
			}
			walk(v, func(m map[string]any) {
				co := courseInfoFromMap(api.name, m)
				if co.ID == "" || co.Title == "" || seen[api.name+":"+co.ID] {
					return
				}
				seen[api.name+":"+co.ID] = true
				out = append(out, co)
				added++
			})
			if !api.paged || added == 0 {
				break
			}
		}
	}
	return out
}

func courseInfoFromMap(source string, m map[string]any) courseInfo {
	id := firstText(m,
		"fileGroupId", "file_group_id", "xFileGroupId", "x_file_group_id",
		"packageId", "package_id", "originId", "origin_id", "courseId", "course_id",
		"groupId", "group_id", "id", "fileId", "file_id")
	title := firstText(m,
		"packageName", "package_name", "groupName", "group_name", "courseName", "course_name",
		"className", "class_name", "subjectName", "subject_name", "homeworkName", "homework_name",
		"title", "name")
	if id == "" || title == "" || hasAnyKey(m, "url", "URL", "playUrl", "PlayURL", "m3u8Url", "vid", "polyvVid", "videoId") {
		return courseInfo{}
	}
	co := courseInfo{ID: id, Title: title, Source: source}
	switch source {
	case "history":
		co.ID = "history_" + strings.TrimPrefix(id, "history_")
		co.History = true
		co.Title = prefixTitle("历史课堂_", title)
	case "homework":
		co.ID = "homework_" + strings.TrimPrefix(id, "homework_")
		co.Homework = true
		co.Title = prefixTitle("课后巩固_", title)
	case "course":
		co.Class = true
	default:
		co.Origin = truthy(m["is_origin"]) || truthy(m["isOrigin"]) || truthy(m["origin"])
	}
	co.Price = firstText(m, "price", "salePrice", "payPrice", "orderPrice", "amount", "cost", "goodsPrice", "discountPrice", "unitPrice")
	co.Purchased = truthy(m["purchased"]) || truthy(m["isBuy"]) || truthy(m["isBought"]) || truthy(m["hasBuy"]) || truthy(m["isPurchased"]) ||
		truthy(m["canStudy"]) || truthy(m["permission"]) || truthy(m["buy"]) || strings.EqualFold(firstText(m, "status"), "purchased")
	co.Raw = m
	return co
}

func buildCourseListMedia(courses []courseInfo, platform string) *extractor.MediaInfo {
	entries := make([]*extractor.MediaInfo, 0, len(courses))
	for i, co := range courses {
		extra := map[string]any{
			"course_id": co.ID,
			"source":    co.Source,
			"platform":  platform,
			"index":     i + 1,
		}
		if co.History {
			extra["is_history"] = true
		}
		if co.Homework {
			extra["is_homework"] = true
		}
		if co.Class {
			extra["is_class"] = true
		}
		if co.Origin {
			extra["is_origin"] = true
		}
		if co.Price != "" {
			extra["price"] = co.Price
		}
		if co.Purchased {
			extra["purchased"] = true
		}
		entries = append(entries, &extractor.MediaInfo{Site: "plaso", Title: clean(co.Title), Extra: extra})
	}
	return &extractor.MediaInfo{Site: "plaso", Title: "plaso_courses", Entries: entries, Extra: map[string]any{"platform": platform, "course_count": len(entries)}}
}

func (s *plasoSession) fetchFilesForCourse(kind, cid string) []fileItem {
	switch kind {
	case "history":
		return s.fetchHistoryFiles(cid)
	case "homework":
		return s.fetchHomeworkFiles(cid)
	default:
		return s.fetchPackageFiles(cid)
	}
}

func (s *plasoSession) fetchPackageFiles(cid string) []fileItem {
	if strings.TrimSpace(cid) == "" {
		return nil
	}
	requests := []struct {
		url  string
		data map[string]string
	}{
		{s.eps.url(packagePath), map[string]string{"packageId": cid, "id": cid, "fileGroupId": cid, "taskNum": "0"}},
		{s.eps.url(infoPath), map[string]string{"fileGroupId": cid, "id": cid, "packageId": cid}},
		{s.eps.url(dirInfoPath), map[string]string{"fileGroupId": cid, "id": cid, "dirId": cid, "hiddenTask": "false", "sourceWay": "course"}},
		{s.eps.url(dirInfoPath), map[string]string{"id": cid, "xFileId": cid, "cid": cid, "hiddenTask": "1", "sourceWay": "course", "needMyFav": "1", "needProgress": "1"}},
		{s.eps.url(filePath), map[string]string{"fileId": cid, "id": cid}},
		{s.eps.url(fileInfoPath), map[string]string{"fileId": cid, "id": cid}},
	}
	var out []fileItem
	for _, req := range requests {
		v, err := s.postJSON(req.url, req.data)
		if err != nil {
			continue
		}
		out = append(out, collectFileItems(v)...)
	}
	return s.expandFileDetails(dedupeFiles(out))
}

func (s *plasoSession) fetchHistoryFiles(cid string) []fileItem {
	var out []fileItem
	pageSize := 999
	for start := 0; start < 20000; {
		v, err := s.postJSON(s.eps.url(historyListPath), map[string]string{
			"dateFrom": "0", "dateTo": "2000000000000", "indexStart": fmt.Sprint(start), "pageSize": fmt.Sprint(pageSize),
		})
		if err != nil {
			break
		}
		items := s.historyItemsFromPayload(v)
		if len(items) == 0 {
			break
		}
		for _, f := range items {
			if cid == "" || sameFileIdentity(f, cid) {
				out = append(out, f)
			}
		}
		if len(items) < pageSize {
			break
		}
		start += len(items)
	}
	return s.expandFileDetails(dedupeFiles(out))
}

func (s *plasoSession) historyItemsFromPayload(v any) []fileItem {
	var out []fileItem
	for _, item := range extractNamedList(v, "list", "rows", "records") {
		m := asAnyMap(item)
		if len(m) == 0 {
			continue
		}
		if f := normalizeHistoryFile(m); itemHasSignal(f) {
			out = append(out, f)
		} else {
			out = append(out, collectFileItems(m)...)
		}
	}
	if len(out) == 0 {
		out = collectFileItems(v)
	}
	return dedupeFiles(out)
}

func normalizeHistoryFile(m map[string]any) fileItem {
	fileCommon := asAnyMap(m["fileCommon"])
	if recs := asAnyList(m["records"]); len(recs) > 0 {
		rec := asAnyMap(recs[0])
		if len(rec) > 0 {
			if fc := asAnyMap(rec["fileCommon"]); len(fc) > 0 && firstText(fileCommon, "location") == "" {
				fileCommon = fc
			}
		}
	}
	recordFile := map[string]any{}
	if rfs := asAnyList(m["recordFiles"]); len(rfs) > 0 {
		recordFile = asAnyMap(rfs[0])
	}
	merged := map[string]any{}
	for k, v := range fileCommon {
		merged[k] = v
	}
	for k, v := range recordFile {
		if _, ok := merged[k]; !ok || valueText(merged[k]) == "" {
			merged[k] = v
		}
	}
	for k, v := range m {
		if _, ok := merged[k]; !ok || valueText(merged[k]) == "" {
			merged[k] = v
		}
	}
	item := buildFileItem(merged)
	item.ID = firstNonEmpty(firstText(m, "fileId", "file_id"), firstText(recordFile, "_id", "originId", "fileId"), firstText(fileCommon, "_id", "originId", "fileId"), item.ID)
	item.MyID = firstNonEmpty(firstText(m, "myid", "myId"), firstText(fileCommon, "myid", "myId", "_id"), firstText(recordFile, "_id"), item.MyID)
	item.Location = firstNonEmpty(firstText(recordFile, "location"), firstText(fileCommon, "location"), item.Location)
	item.Name = firstNonEmpty(firstText(m, "title", "name"), item.Name)
	item.Raw = m
	return item
}

func (s *plasoSession) fetchHomeworkFiles(cid string) []fileItem {
	var out []fileItem
	v, err := s.postJSON(s.eps.url(homeworkListPath), map[string]string{"pageStart": "0", "pageSize": "999", "status": "5", "timeRange": "5"})
	if err == nil {
		for _, item := range extractNamedList(v, "zuoyes", "homeworks", "list", "rows") {
			m := asAnyMap(item)
			if len(m) == 0 {
				continue
			}
			id := firstText(m, "homework_id", "homeworkId", "fileId", "id", "_id")
			if cid != "" && !sameID(id, cid) {
				continue
			}
			out = append(out, collectFileItems(m)...)
			if id != "" {
				if detail, ok := s.fetchFileDetail(fileItem{ID: id, MyID: id, Raw: m}); ok {
					out = append(out, detail)
				}
				if fileList, ok := s.fetchFileIDList(id); ok {
					out = append(out, fileList...)
				}
			}
		}
	}
	if cid != "" && len(out) == 0 {
		if detail, ok := s.fetchFileDetail(fileItem{ID: cid, MyID: cid}); ok {
			out = append(out, detail)
		}
		if fileList, ok := s.fetchFileIDList(cid); ok {
			out = append(out, fileList...)
		}
	}
	return s.expandFileDetails(dedupeFiles(out))
}

func homeworkCoursesFromPayload(v any) []courseInfo {
	var out []courseInfo
	for _, item := range extractNamedList(v, "zuoyes", "homeworks", "list", "rows") {
		m := asAnyMap(item)
		if len(m) == 0 {
			continue
		}
		id := firstText(m, "homework_id", "homeworkId", "fileId", "id", "_id")
		title := firstText(m, "homework_name", "homeworkName", "title", "name")
		if id == "" || title == "" {
			continue
		}
		out = append(out, courseInfo{ID: "homework_" + strings.TrimPrefix(id, "homework_"), Title: prefixTitle("课后巩固_", title), Source: "homework", Homework: true, Purchased: true, Raw: m})
	}
	return out
}

func (s *plasoSession) fetchFileIDList(fileID string) ([]fileItem, bool) {
	v, err := s.postJSON(s.eps.url(fileInfoPath), map[string]string{"fileId": fileID, "id": fileID})
	if err != nil {
		return nil, false
	}
	files := collectFileItems(v)
	return files, len(files) > 0
}

func (s *plasoSession) fetchShareOrFile(id string) ([]fileItem, string) {
	requests := []struct {
		url  string
		data map[string]string
	}{
		{s.eps.url(sharePath), map[string]string{"sfId": id, "shareKey": id, "fileId": id, "id": id}},
		{s.eps.url(filePath), map[string]string{"fileId": id, "id": id}},
		{s.eps.url(fileInfoPath), map[string]string{"fileId": id, "id": id}},
	}
	for _, req := range requests {
		v, err := s.postJSON(req.url, req.data)
		if err != nil {
			continue
		}
		files := collectFileItems(v)
		if len(files) == 0 {
			continue
		}
		files = s.expandFileDetails(dedupeFiles(files))
		title := firstNonEmpty(findFirst(v, "shareName", "courseName", "packageName", "name", "title"), files[0].Name)
		return files, title
	}
	return nil, ""
}

func (s *plasoSession) expandFileDetails(files []fileItem) []fileItem {
	if len(files) == 0 {
		return nil
	}
	out := make([]fileItem, 0, len(files))
	cache := map[string]fileItem{}
	for _, f := range files {
		id := firstNonEmpty(f.ID, f.MyID)
		if needsFileDetail(f) && id != "" {
			if detail, ok := cache[id]; ok {
				f = mergeFileItem(f, detail)
			} else if detail, ok := s.fetchFileDetail(f); ok {
				cache[id] = detail
				f = mergeFileItem(f, detail)
			}
		}
		out = append(out, f)
	}
	return dedupeFiles(out)
}

func (s *plasoSession) fetchFileDetail(f fileItem) (fileItem, bool) {
	for _, api := range []string{s.eps.url(filePath), s.eps.url(fileInfoPath)} {
		v, err := s.postJSON(api, s.playRequestData(f))
		if err != nil {
			continue
		}
		for _, detail := range collectFileItems(v) {
			if sameID(detail.ID, f.ID) || sameID(detail.MyID, f.MyID) || firstNonEmpty(detail.URL, detail.Location, detail.LocationPath, detail.Vid, detail.VideoID) != "" {
				return detail, true
			}
		}
	}
	return fileItem{}, false
}

func (s *plasoSession) resolveFile(f fileItem, idx int) *extractor.MediaInfo {
	name := clean(firstNonEmpty(f.Name, fmt.Sprintf("[%02d]--plaso", idx)))
	if f.Chapter != "" && !strings.Contains(name, f.Chapter) {
		name = clean(f.Chapter + "--" + name)
	}
	for _, src := range s.resolveSources(f) {
		if src.URL == "" {
			continue
		}
		return s.sourceMediaInfo(name, f, src)
	}
	return nil
}

func (s *plasoSession) resolveSources(f fileItem) []plasoSource {
	var out []plasoSource
	for _, raw := range []string{f.URL, f.Location, f.LocationPath} {
		if src := s.directSource(f, raw); src.URL != "" {
			out = append(out, src)
		}
	}
	if src := s.fetchAliPlaySource(f); src.URL != "" {
		out = append(out, src)
	}
	if src := s.fetchPolyvSource(f); src.URL != "" {
		out = append(out, src)
	}
	if src := s.fetchPlistSource(f); src.URL != "" {
		out = append(out, src)
	}
	if src := s.buildDirectDocumentSource(f); src.URL != "" {
		out = append(out, src)
	}
	if src := s.buildPlayerSource(f); src.URL != "" {
		out = append(out, src)
	}
	return out
}

func (s *plasoSession) directSource(f fileItem, raw string) plasoSource {
	u := s.normalizeMediaURL(raw, "")
	if u == "" || !strings.HasPrefix(u, "http") {
		return plasoSource{}
	}
	lu := strings.ToLower(u)
	if isLikelyPlistURL(u) && !strings.Contains(lu, ".m3u8") && !strings.Contains(lu, ".mp4") && !strings.Contains(lu, ".mp3") && !strings.Contains(lu, "format=m3u8") {
		return plasoSource{}
	}
	fmtv := formatOf(u, f.Type)
	if !looksDownloadable(u) && f.Type == "" {
		return plasoSource{}
	}
	return plasoSource{URL: u, Format: fmtv, Quality: "best", SourceType: "direct", NeedMerge: fmtv == "m3u8", Size: f.Size}
}

func (s *plasoSession) sourceMediaInfo(title string, f fileItem, src plasoSource) *extractor.MediaInfo {
	u := s.normalizeMediaURL(src.URL, "")
	if u == "" {
		return nil
	}
	fmtv := firstNonEmpty(src.Format, formatOf(u, f.Type))
	extra := map[string]any{
		"file_id":       f.ID,
		"my_id":         f.MyID,
		"location":      f.Location,
		"location_path": f.LocationPath,
		"storage_id":    f.StorageID,
		"chapter":       f.Chapter,
		"index":         f.Index,
		"file_type":     f.Type,
		"source_type":   firstNonEmpty(src.SourceType, "direct"),
		"platform":      s.eps.platform,
	}
	for k, v := range src.Extra {
		extra[k] = v
	}
	if src.M3U8Text != "" {
		extra["m3u8_url"] = u
		extra["m3u8_text"] = src.M3U8Text
		extra["source_type"] = "m3u8_text"
		u = m3u8DataURL(src.M3U8Text)
		fmtv = "m3u8"
	}
	stream := extractor.Stream{
		Quality:   firstNonEmpty(src.Quality, "best"),
		URLs:      []string{u},
		Format:    fmtv,
		Size:      firstPositive(src.Size, f.Size),
		NeedMerge: src.NeedMerge || fmtv == "m3u8",
		AudioURL:  src.AudioURL,
		Headers:   streamHeaders(s.headers),
		Extra:     cloneAnyMap(extra),
	}
	return &extractor.MediaInfo{Site: "plaso", Title: title, Streams: map[string]extractor.Stream{"best": stream}, Extra: extra}
}

func m3u8DataURL(text string) string {
	return "data:application/vnd.apple.mpegurl;base64," + base64.StdEncoding.EncodeToString([]byte(text))
}

func (s *plasoSession) postJSON(api string, data map[string]string) (any, error) {
	body, err := s.client.PostForm(api, data, s.headers)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return nil, err
	}
	return v, nil
}
