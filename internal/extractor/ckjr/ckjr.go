// Package ckjr implements 创客匠人 course extraction.
//
// Source alignment:
//
//	Mooc/Courses/Ckjr/Ckjr_Base.pyc.1shot.cdc.py
//	Mooc/Courses/Ckjr/Ckjr_Course.pyc.1shot.cdc.py
package ckjr

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

const (
	url0 = "https://kpapiop.ckjr001.com"
	url1 = "https://playvideo.qcloud.com/getplayinfo/v4/%s/%s"

	ckjrFromApp    = "oa"
	ckjrWebVersion = "202508141135"
	ckjrUA         = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36 NetType/WIFI MicroMessenger/7.0.20.1781(0x6700143B) WindowsWechat(0x63090a13) UnifiedPCWindowsWechat(0xf254181c) XWEB/19339 Flue"
)

var patterns = []string{`(?:(?:[\w-]+\.)?ckjr001\.com|(?:[\w-]+\.)?nineteenj\.cn|(?:[\w-]+\.)?gmp-office\.com)/|/kpv2p/`}

func init() {
	extractor.Register(&Ckjr{}, extractor.SiteInfo{Name: "Ckjr", URL: "ckjr001.com", NeedAuth: true})
}

type Ckjr struct{}

func (s *Ckjr) Patterns() []string { return patterns }

type routeInfo struct {
	Kind      string
	ID        string
	IDKey     string
	ProdType  string
	CourseTyp string
	Company   string
	BaseURL   string
	RawURL    string
	Query     map[string]string
}

type mediaCandidate struct {
	URL       string
	Title     string
	Format    string
	Kind      string
	SourceKey string
	Size      int64
	NeedMerge bool
	Extra     map[string]any
}

var (
	routeRe = regexp.MustCompile(`(?i)/kpv2p/([\w-]+).*#/?homePage/(course/(video|voice|imgText)|column/columnDetail|datum/datumDetail|live/(?:liveDetail|livePersonalDetail|liveRoom)|package/packageDetail|testPaper/testDetail)\?([^#\s]+)`)
	idRe    = regexp.MustCompile(`(?i)(?:courseId|extId|datumId|liveId|combosId|testId|prodId|productId|id)=([0-9]+)`)
	mediaRe = regexp.MustCompile(`(?i)(?:https?:)?//[^\s<>"'\\]+?\.(?:m3u8|mp4|m4v|mov|flv|mp3|m4a|aac|wav|pdf|docx?|pptx?|xlsx?|zip|rar|7z|txt|csv|jpe?g|png|gif|webp)(?:[^\s<>"'\\]*)?`)
)

var routeCfg = map[string]routeInfo{
	"video":        {Kind: "video", IDKey: "courseId", ProdType: "5", CourseTyp: "0"},
	"voice":        {Kind: "voice", IDKey: "courseId", ProdType: "5", CourseTyp: "1"},
	"imgText":      {Kind: "imgText", IDKey: "courseId", ProdType: "5", CourseTyp: "2"},
	"column":       {Kind: "column", IDKey: "extId", ProdType: "9", CourseTyp: "9"},
	"datum":        {Kind: "datum", IDKey: "datumId", ProdType: "8", CourseTyp: "8"},
	"live":         {Kind: "live", IDKey: "liveId", ProdType: "51", CourseTyp: "51"},
	"livePersonal": {Kind: "livePersonal", IDKey: "liveId", ProdType: "180", CourseTyp: "180"},
	"package":      {Kind: "package", IDKey: "combosId", ProdType: "61", CourseTyp: "61"},
	"testPaper":    {Kind: "testPaper", IDKey: "testId", ProdType: "125", CourseTyp: "125"},
}

func (s *Ckjr) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("ckjr requires login cookies")
	}
	c := util.NewClient()
	c.SetCookieJar(opts.Cookies)
	route := parseRoute(rawURL)
	headers := ckjrHeaders(rawURL)
	if cookie := ckjrCookieHeader(opts.Cookies, rawURL); cookie != "" {
		headers["Cookie"] = cookie
	}
	auth := ckjrAuthFromCookies(opts.Cookies, rawURL, route)
	ckjrApplyAuth(headers, &route, auth)

	if err := checkLogin(c, headers); err != nil {
		return nil, err
	}
	if route.ID == "" {
		courses, err := fetchCourseList(c, headers, route)
		if err != nil {
			return nil, err
		}
		if len(courses) == 0 {
			return nil, fmt.Errorf("ckjr: empty course list and no route id")
		}
		return courseListMedia(route, courses), nil
	}

	payloads := fetchRoutePayloads(c, route, headers)
	entries, chapters := entriesFromPayloads(c, route, payloads, headers)
	entries = dedupeEntries(entries)
	if len(entries) == 0 {
		return nil, fmt.Errorf("ckjr: no playable media URL in API response for %s=%s", route.IDKey, route.ID)
	}
	if routeIsSingle(route.Kind) && len(entries) == 1 {
		return entries[0], nil
	}
	return &extractor.MediaInfo{
		Site:     "ckjr",
		Title:    util.SanitizeFilename(routeTitle(payloads, route)),
		Entries:  entries,
		Chapters: chapters,
		Extra: map[string]any{
			"course_id":   route.ID,
			"route_kind":  route.Kind,
			"prod_type":   route.ProdType,
			"course_type": route.CourseTyp,
		},
	}, nil
}

func checkLogin(c *util.Client, headers map[string]string) error {
	payload, err := requestAPI(c, "/api/marketingAward/getMarketingAwardList", map[string]string{"name": "", "page": "1", "limit": "1", "prodType": "0"}, headers)
	if err != nil {
		return fmt.Errorf("ckjr login check: %w", err)
	}
	if !apiLoginResponseOK(payload) {
		msg := firstNonEmpty(textValue(asMap(payload), "msg", "message", "error"), "login check failed")
		return fmt.Errorf("ckjr login check failed: %s", msg)
	}
	return nil
}

func fetchRoutePayloads(c *util.Client, r routeInfo, headers map[string]string) []any {
	params := resourceParams(r)
	detailPaths := []string{
		fmt.Sprintf("/api/column/detail/%s", url.PathEscape(r.ID)),
		fmt.Sprintf("/api/courses/%s", url.PathEscape(r.ID)),
		"/api/courses/detail",
		"/api/course/detail",
		fmt.Sprintf("/api/columns/%s", url.PathEscape(r.ID)),
		"/api/columns/detail",
		"/api/column/detail",
		"/api/datum/detail",
		fmt.Sprintf("/api/datum/detail/%s", url.PathEscape(r.ID)),
		fmt.Sprintf("/api/datum/datumsRelates/%s", url.PathEscape(r.ID)),
		"/api/live/detail",
		"/api/testPaper/detail",
		"/api/prod/detail/info",
		"/api/product/detail/info",
	}
	dirPaths := []string{
		fmt.Sprintf("/api/courses/%s/dirs", url.PathEscape(r.ID)),
		"/api/courses/dirs",
		"/api/course/dirs",
	}
	switch r.Kind {
	case "column":
		detailPaths = append([]string{fmt.Sprintf("/api/column/detail/%s", url.PathEscape(r.ID)), "/api/column/detail"}, detailPaths...)
		dirPaths = append([]string{fmt.Sprintf("/api/column/getCourses/%s", url.PathEscape(r.ID)), fmt.Sprintf("/api/columns/%s/dirs", url.PathEscape(r.ID)), "/api/columns/dirs", "/api/column/dirs"}, dirPaths...)
	case "datum":
		detailPaths = append([]string{fmt.Sprintf("/api/datum/detail/%s", url.PathEscape(r.ID)), fmt.Sprintf("/api/datum/datumsRelates/%s", url.PathEscape(r.ID)), "/api/datum/detail"}, detailPaths...)
	case "live", "livePersonal":
		detailPaths = append([]string{fmt.Sprintf("/api/live/livePersonal/show/%s", url.PathEscape(r.ID)), fmt.Sprintf("/api/liveFlow/getHLSPlayURL/%s", url.PathEscape(r.ID)), fmt.Sprintf("/api/live/getHLSPlayURL/%s", url.PathEscape(r.ID)), "/api/live/detail", "/api/live/playback/list"}, detailPaths...)
		dirPaths = append([]string{fmt.Sprintf("/api/live/livePersonal/getPlayBackList/%s", url.PathEscape(r.ID))}, dirPaths...)
	case "package":
		detailPaths = append([]string{fmt.Sprintf("/api/combos/%s", url.PathEscape(r.ID)), "/api/package/detail"}, detailPaths...)
		dirPaths = append([]string{fmt.Sprintf("/api/combos/%s/dirs", url.PathEscape(r.ID)), "/api/combos/dirs"}, dirPaths...)
	}
	var out []any
	seenPath := map[string]bool{}
	for _, path := range detailPaths {
		if seenPath[path] {
			continue
		}
		seenPath[path] = true
		payload, err := requestAPI(c, path, params, headers)
		if err == nil && responseHasPayload(payload) {
			out = append(out, payload)
		}
	}
	for _, path := range dirPaths {
		if seenPath[path] {
			continue
		}
		seenPath[path] = true
		for _, payload := range fetchPagedPayloads(c, path, params, headers, ckjrDirPageSize) {
			out = append(out, payload)
		}
	}
	return out
}

func fetchPagedPayloads(c *util.Client, path string, baseParams map[string]string, headers map[string]string, pageSize int) []any {
	if pageSize <= 0 {
		pageSize = 100
	}
	var out []any
	for page := 1; page <= 99; page++ {
		params := cloneStringMap(baseParams)
		params["page"] = fmt.Sprint(page)
		params["pageNum"] = fmt.Sprint(page)
		params["pageSize"] = fmt.Sprint(pageSize)
		params["limit"] = fmt.Sprint(pageSize)
		params["size"] = fmt.Sprint(pageSize)
		payload, err := requestAPI(c, path, params, headers)
		if err != nil || !responseHasPayload(payload) {
			break
		}
		out = append(out, payload)
		rows := extractPageRows(payload)
		if len(rows) == 0 || !pageHasMore(payload, page, pageSize, len(rows)) {
			break
		}
	}
	return out
}

func requestAPI(c *util.Client, path string, params map[string]string, headers map[string]string) (any, error) {
	q := url.Values{}
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	if q.Get("fromApp") == "" {
		q.Set("fromApp", ckjrFromApp)
	}
	if q.Get("webversion") == "" {
		q.Set("webversion", ckjrWebVersion)
	}
	apiURL := path
	if !strings.HasPrefix(strings.ToLower(apiURL), "http") {
		apiURL = url0 + path
	}
	if strings.Contains(path, "?") {
		apiURL += "&" + q.Encode()
	} else {
		apiURL += "?" + q.Encode()
	}
	body, err := c.GetString(apiURL, headers)
	if err != nil {
		return nil, fmt.Errorf("ckjr GET %s: %w", path, err)
	}
	var payload any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil, fmt.Errorf("ckjr parse %s: %w", path, err)
	}
	return payload, nil
}

func entriesFromPayloads(c *util.Client, r routeInfo, payloads []any, headers map[string]string) ([]*extractor.MediaInfo, []extractor.Chapter) {
	var entries []*extractor.MediaInfo
	chapterSeen := map[string]bool{}
	var chapters []extractor.Chapter
	for _, payload := range payloads {
		before := len(entries)
		lessons := collectLessonNodes(payload)
		if len(lessons) == 0 {
			entries = append(entries, entriesFromPayload(c, payload, headers, r.ID)...)
			continue
		}
		for _, lesson := range lessons {
			if lesson.Chapter != "" && !chapterSeen[lesson.Chapter] {
				chapterSeen[lesson.Chapter] = true
				chapters = append(chapters, extractor.Chapter{Title: lesson.Chapter, Index: len(chapters) + 1})
			}
			cands := resolveLessonCandidates(c, r, lesson.Node, headers)
			for _, cand := range selectLessonCandidates(cands) {
				title := lessonTitle(lesson, cand, r.ID)
				extra := map[string]any{
					"kind":       firstNonEmpty(cand.Kind, mediaKindFromFormat(cand.Format)),
					"route_kind": r.Kind,
				}
				if lesson.Chapter != "" {
					extra["chapter"] = lesson.Chapter
				}
				if id := nodeResourceID(resolveKindFromNode(lesson.Node, r.Kind), lesson.Node, ""); id != "" {
					extra["resource_id"] = id
				}
				if cand.SourceKey != "" {
					extra["source_key"] = cand.SourceKey
				}
				for k, v := range cand.Extra {
					extra[k] = v
				}
				entries = append(entries, entryFromCandidate(title, cand, headers, extra))
			}
		}
		if len(entries) == before {
			entries = append(entries, entriesFromPayload(c, payload, headers, r.ID)...)
		}
	}
	return entries, chapters
}

func entriesFromPayload(c *util.Client, payload any, headers map[string]string, fallbackTitle string) []*extractor.MediaInfo {
	var entries []*extractor.MediaInfo
	seen := map[string]bool{}
	for _, node := range walkMaps(payload) {
		cands := mediaFromNode(c, node, headers)
		for _, cand := range cands {
			if cand.URL == "" || seen[cand.URL] {
				continue
			}
			seen[cand.URL] = true
			title := firstNonEmpty(cand.Title, textValue(node, "lessonName", "dirName", "chapterName", "title", "name", "courseName", "courseTitle", "prodName", "prodTitle", "productTitle", "videoName", "audioName", "detailName", "datumName", "datumTitle", "liveName", "liveTitle", "paperName", "paperTitle", "testName", "testTitle", "columnName", "columnTitle", "combosName", "combosTitle", "fileName", "filename"), fallbackTitle)
			extra := map[string]any{"kind": firstNonEmpty(cand.Kind, mediaKindFromFormat(cand.Format))}
			if cand.SourceKey != "" {
				extra["source_key"] = cand.SourceKey
			}
			for k, v := range cand.Extra {
				extra[k] = v
			}
			entries = append(entries, entryFromCandidate(title, cand, headers, extra))
		}
	}
	return entries
}

func entryFromCandidate(title string, cand mediaCandidate, headers map[string]string, extra map[string]any) *extractor.MediaInfo {
	cand = normalizeCandidate(cand)
	kind := firstNonEmpty(cand.Kind, mediaKindFromFormat(cand.Format))
	streamKey := "best"
	quality := "best"
	if kind == "file" {
		streamKey = "file"
		quality = "file"
	} else if cand.Format == "html" {
		streamKey = "document"
		quality = "document"
	}
	if extra == nil {
		extra = map[string]any{}
	}
	extra["kind"] = kind
	return &extractor.MediaInfo{Site: "ckjr", Title: util.SanitizeFilename(firstNonEmpty(title, cand.Title, "ckjr")), Streams: map[string]extractor.Stream{
		streamKey: {Quality: quality, URLs: []string{cand.URL}, Format: cand.Format, Size: cand.Size, NeedMerge: cand.NeedMerge || cand.Format == "m3u8", Headers: headers, Extra: cloneAnyMap(extra)},
	}, Extra: extra}
}

func mediaFromNode(c *util.Client, node map[string]any, headers map[string]string) []mediaCandidate {
	return collectMediaCandidates(c, node, headers)
}

func requestQCloud(c *util.Client, auth map[string]string, headers map[string]string) (string, error) {
	candidates, err := requestQCloudCandidates(c, auth, headers)
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", nil
	}
	return candidates[0].URL, nil
}

func resolveLessonCandidates(c *util.Client, r routeInfo, node map[string]any, headers map[string]string) []mediaCandidate {
	cands := collectMediaCandidates(c, node, headers)
	if len(cands) > 0 {
		return cands
	}
	for _, payload := range fetchLessonDetailPayloads(c, r, node, headers) {
		cands = append(cands, collectMediaCandidates(c, payload, headers)...)
	}
	return dedupeCandidates(cands)
}

func fetchLessonDetailPayloads(c *util.Client, r routeInfo, node map[string]any, headers map[string]string) []any {
	kind := resolveKindFromNode(node, r.Kind)
	id := nodeResourceID(kind, node, r.ID)
	if id == "" {
		return nil
	}
	child := routeCfg[firstNonEmpty(kind, r.Kind, "video")]
	child.ID = id
	child.Company = r.Company
	child.BaseURL = r.BaseURL
	child.RawURL = r.RawURL
	child.Query = cloneStringMap(r.Query)
	params := resourceParams(child)
	for _, k := range []string{"prodId", "productId", "courseId", "detailId", "detail_id", "lessonId", "dirId", "liveId", "datumId", "testId", "extId", "columnId", "combosId"} {
		if v := textValue(node, k); v != "" {
			params[k] = v
		}
	}
	params[child.IDKey] = id
	now := fmt.Sprint(time.Now().UnixMilli())
	var paths []string
	switch child.Kind {
	case "live", "livePersonal":
		liveID := firstNonEmpty(textValue(node, "liveId"), id)
		detailID := textValue(node, "detailId", "detail_id", "lessonDetailId")
		relateID := textValue(node, "reviewRelateId", "recRelateId", "relateId")
		liveParams := cloneStringMap(params)
		liveParams["time"] = now
		if relateID != "" {
			liveParams["relateId"] = relateID
		}
		if child.Kind == "livePersonal" && detailID != "" {
			if payload, err := requestAPI(c, fmt.Sprintf("/api/live/livePersonal/getPlayUrl/%s/%s", url.PathEscape(liveID), url.PathEscape(detailID)), liveParams, headers); err == nil && responseHasPayload(payload) {
				return []any{payload}
			}
		}
		paths = []string{
			fmt.Sprintf("/api/liveFlow/getHLSPlayURL/%s", url.PathEscape(liveID)),
			fmt.Sprintf("/api/live/getHLSPlayURL/%s", url.PathEscape(liveID)),
			"/api/live/detail",
			"/api/live/playback/list",
		}
		params = liveParams
	case "datum":
		paths = []string{
			fmt.Sprintf("/api/datum/detail/%s", url.PathEscape(id)),
			fmt.Sprintf("/api/datum/datumsRelates/%s", url.PathEscape(id)),
			"/api/datum/detail",
			"/api/prod/detail/info",
			"/api/product/detail/info",
		}
	case "testPaper":
		paths = []string{"/api/testPaper/detail", "/api/prod/detail/info", "/api/product/detail/info"}
	default:
		paths = []string{
			"/api/prod/detail/info",
			"/api/product/detail/info",
			fmt.Sprintf("/api/courses/%s", url.PathEscape(id)),
			"/api/courses/detail",
			"/api/course/detail",
		}
	}
	var out []any
	seen := map[string]bool{}
	for _, path := range paths {
		if seen[path] {
			continue
		}
		seen[path] = true
		payload, err := requestAPI(c, path, params, headers)
		if err == nil && responseHasPayload(payload) {
			out = append(out, payload)
		}
	}
	return out
}
