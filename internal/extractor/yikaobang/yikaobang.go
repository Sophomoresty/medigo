// Package yikaobang implements the 医考帮 course extractor.
package yikaobang

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

const (
	ykbSite          = "yikaobang"
	ykbRefererURL    = "https://www.yikaobang.com.cn/"
	ykbHomeURL       = "https://www.yikaobang.com.cn/"
	ykbLegacyAPIBase = "https://api.yikaobang.com.cn/index.php/"
	ykbNewAPIBase    = "https://new-ykb.yikaobang.com.cn/"
	ykbH5Base        = "https://ykb-h5-web.yikaobang.com.cn/"
	ykbFileBase      = "https://file.medmeta.com/"
	ykbAppVersion    = "2.8.5.4"
)

const (
	ykbEndpointCourseList      = "course/main/courseList"
	ykbEndpointCourseSearch    = "course/main/search"
	ykbEndpointLearnCenterList = "course/center/list"
	ykbEndpointCourseDetail    = "course/main/detail"
	ykbEndpointCoursePackage   = "course/main/coursePackage"
	ykbEndpointCourseUnlock    = "course/main/listAndUserPermission"
	ykbEndpointCatalogue       = "course/center/catalogue"
	ykbEndpointHandout         = "course/center/handout"
	ykbEndpointCourseAK        = "Course/CourseV2/getCourseAk"
	ykbEndpointLegacyCourse    = "course/courseV2/getCourse"
	ykbEndpointLegacyChapter   = "vidteaching/main/chapter"
	ykbEndpointLegacyVideo     = "vidteaching/main/video"
)

var (
	patterns          = []string{`(?:[\w-]+\.)?yikaobang\.com\.cn/`, `医考帮`, `yikaobang`}
	ykbPathCourseRe   = regexp.MustCompile(`(?i)/(?:course|courses|courseDetail|detail|center|learn|package|goods)/(\d{2,}|[A-Za-z0-9_-]{6,})`)
	ykbPathVideoRe    = regexp.MustCompile(`(?i)/(?:video|play|lesson|chapterVideo)/(\d{2,}|[A-Za-z0-9_-]{6,})`)
	ykbPathChapterRe  = regexp.MustCompile(`(?i)/(?:chapter|catalogue|catalog)/(\d{2,}|[A-Za-z0-9_-]{6,})`)
	ykbPathGoodsRe    = regexp.MustCompile(`(?i)/(?:goods|shop|item)/(\d{2,}|[A-Za-z0-9_-]{6,})`)
	ykbLooseNumericRe = regexp.MustCompile(`^\d{2,}$`)
)

func init() {
	extractor.Register(&Yikaobang{}, extractor.SiteInfo{Name: "Yikaobang", URL: "yikaobang.com.cn", NeedAuth: true})
}

type Yikaobang struct{}

func (y *Yikaobang) Patterns() []string { return patterns }

func (y *Yikaobang) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil {
		opts = &extractor.ExtractOpts{}
	}
	sess, err := newYikaobangSession(opts)
	if err != nil {
		return nil, err
	}
	target := parseYikaobangTarget(rawURL)
	payloads, fetchErrs := sess.fetchPayloads(rawURL, target)
	result := parseYikaobangPayloads(payloads, target)
	media, buildErrs := sess.buildMedia(rawURL, target, result)
	if media != nil {
		return media, nil
	}
	allErrs := append(fetchErrs, buildErrs...)
	if len(allErrs) > 0 {
		return nil, fmt.Errorf("yikaobang: no playable course/video/file data resolved: %s", joinErrors(allErrs))
	}
	return nil, fmt.Errorf("yikaobang: no course/video/file data resolved from %s", strings.TrimSpace(rawURL))
}

type ykbSession struct {
	client   *util.Client
	headers  map[string]string
	listOnly bool
	quality  string
}

func newYikaobangSession(opts *extractor.ExtractOpts) (*ykbSession, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("yikaobang requires login cookies with token")
	}
	token := yikaobangAuthToken(opts.Cookies)
	if token == "" {
		return nil, fmt.Errorf("yikaobang requires login cookies with token")
	}
	client := util.NewClient()
	client.SetCookieJar(opts.Cookies)
	client.SetTimeout(12 * time.Second)
	headers := yikaobangHeaders(token)
	if cookie := yikaobangCookieHeader(opts.Cookies); cookie != "" {
		headers["Cookie"] = cookie
	}
	return &ykbSession{client: client, headers: headers, listOnly: opts.ListOnly, quality: opts.Quality}, nil
}

type ykbTarget struct {
	Raw        string
	CourseID   string
	CategoryID string
	ChapterID  string
	VideoID    string
	GoodsID    string
	ActivityID string
	AppID      string
	Keyword    string
}

func parseYikaobangTarget(raw string) ykbTarget {
	t := ykbTarget{Raw: strings.TrimSpace(raw)}
	if t.Raw == "" {
		return t
	}
	if ykbLooseNumericRe.MatchString(t.Raw) {
		t.CourseID = t.Raw
		return t
	}
	parsed, err := url.Parse(t.Raw)
	if err != nil || parsed.Host == "" {
		parsed, _ = url.Parse("https://" + strings.TrimPrefix(t.Raw, "//"))
	}
	if parsed == nil {
		return t
	}
	q := parsed.Query()
	t.CourseID = firstNonEmpty(q.Get("course_id"), q.Get("courseId"), q.Get("courseID"), q.Get("c_id"), q.Get("cid"), q.Get("course"))
	t.CategoryID = firstNonEmpty(q.Get("category_id"), q.Get("categoryId"), q.Get("cat_id"), q.Get("catalog_id"))
	t.ChapterID = firstNonEmpty(q.Get("chapter_id"), q.Get("chapterId"), q.Get("chapter"))
	t.VideoID = firstNonEmpty(q.Get("video_id"), q.Get("videoId"), q.Get("vid"), q.Get("video"), q.Get("lesson_id"), q.Get("lessonId"))
	t.GoodsID = firstNonEmpty(q.Get("goods_id"), q.Get("goodsId"), q.Get("verify_goods_id"), q.Get("item_id"))
	t.ActivityID = firstNonEmpty(q.Get("activity_id"), q.Get("activityId"), q.Get("xue_activity_id"))
	t.AppID = firstNonEmpty(q.Get("app_id"), q.Get("appId"), q.Get("appid"))
	t.Keyword = firstNonEmpty(q.Get("keyword"), q.Get("word"), q.Get("q"), q.Get("search"))
	pathValue := parsed.EscapedPath()
	if pathValue == "" {
		pathValue = parsed.Path
	}
	if t.VideoID == "" {
		t.VideoID = regexFirst(ykbPathVideoRe, pathValue)
	}
	if t.ChapterID == "" {
		t.ChapterID = regexFirst(ykbPathChapterRe, pathValue)
	}
	if t.GoodsID == "" {
		t.GoodsID = regexFirst(ykbPathGoodsRe, pathValue)
	}
	if t.CourseID == "" && t.VideoID == "" {
		t.CourseID = regexFirst(ykbPathCourseRe, pathValue)
	}
	if t.CourseID == "" {
		t.CourseID = firstNonEmpty(q.Get("id"), regexFirst(ykbPathCourseRe, pathValue))
	}
	if t.VideoID == "" && strings.Contains(strings.ToLower(pathValue), "video") {
		t.VideoID = q.Get("id")
	}
	return t
}

func (t ykbTarget) hasCourseLikeID() bool {
	return t.CourseID != "" || t.CategoryID != "" || t.GoodsID != "" || t.ActivityID != "" || t.AppID != ""
}

func (t ykbTarget) hasAnyID() bool {
	return t.hasCourseLikeID() || t.VideoID != "" || t.ChapterID != ""
}

type ykbAPIRequest struct {
	Method string
	URL    string
	Params map[string]string
	Name   string
}

type ykbPayload struct {
	Source string
	Root   any
	Body   string
}

func (s *ykbSession) fetchPayloads(raw string, target ykbTarget) ([]ykbPayload, []error) {
	requests := yikaobangRequests(raw, target)
	payloads := make([]ykbPayload, 0, len(requests))
	var errs []error
	seen := map[string]bool{}
	for _, req := range requests {
		key := req.Method + " " + yikaobangURLWithParams(req.URL, req.Params)
		if seen[key] {
			continue
		}
		seen[key] = true
		payload, err := s.fetchPayload(req)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", req.Name, err))
			continue
		}
		payloads = append(payloads, payload)
	}
	return payloads, errs
}

func yikaobangRequests(raw string, target ykbTarget) []ykbAPIRequest {
	var out []ykbAPIRequest
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
		out = append(out, ykbAPIRequest{Method: http.MethodGet, URL: raw, Name: "input"})
	}

	if !target.hasAnyID() {
		params := yikaobangListParams(target)
		for _, endpoint := range []string{ykbEndpointCourseList, ykbEndpointLearnCenterList, ykbEndpointCourseSearch} {
			out = appendAPIRequests(out, ykbNewAPI(endpoint), params, endpoint)
		}
		out = appendAPIRequests(out, ykbLegacyAPI("vidteaching/main/list"), params, "vidteaching/main/list")
		return out
	}

	if target.hasCourseLikeID() || target.ChapterID != "" {
		params := yikaobangCourseParams(target)
		for _, endpoint := range []string{
			ykbEndpointCourseDetail,
			ykbEndpointCoursePackage,
			ykbEndpointCourseUnlock,
			ykbEndpointCatalogue,
			ykbEndpointHandout,
		} {
			out = appendAPIRequests(out, ykbNewAPI(endpoint), params, endpoint)
		}
		for _, endpoint := range []string{ykbEndpointLegacyCourse, ykbEndpointLegacyChapter} {
			out = appendAPIRequests(out, ykbLegacyAPI(endpoint), params, endpoint)
		}
	}

	if target.VideoID != "" {
		params := yikaobangVideoParams(target)
		for _, endpoint := range []string{ykbEndpointCourseAK, ykbEndpointLegacyVideo, ykbEndpointHandout} {
			api := ykbLegacyAPI(endpoint)
			if strings.HasPrefix(endpoint, "course/center/") {
				api = ykbNewAPI(endpoint)
			}
			out = appendAPIRequests(out, api, params, endpoint)
		}
	}
	return out
}

func appendAPIRequests(out []ykbAPIRequest, api string, params map[string]string, name string) []ykbAPIRequest {
	out = append(out, ykbAPIRequest{Method: http.MethodPost, URL: api, Params: cloneStringMap(params), Name: name + " POST"})
	out = append(out, ykbAPIRequest{Method: http.MethodGet, URL: api, Params: cloneStringMap(params), Name: name + " GET"})
	return out
}

func (s *ykbSession) fetchPayload(req ykbAPIRequest) (ykbPayload, error) {
	method := strings.ToUpper(firstNonEmpty(req.Method, http.MethodGet))
	headers := cloneHeaders(s.headers)
	headers["Accept"] = "application/json, text/plain, */*"
	headers["Origin"] = strings.TrimRight(ykbH5Base, "/")
	var body string
	var err error
	source := req.URL
	if method == http.MethodPost {
		body, err = s.client.PostForm(req.URL, req.Params, headers)
	} else {
		source = yikaobangURLWithParams(req.URL, req.Params)
		body, err = s.client.GetString(source, headers)
	}
	if err != nil {
		return ykbPayload{}, err
	}
	root, decodeErr := decodeYikaobangBody(body)
	if decodeErr != nil && !ykbTextHasDownloadURL(body) {
		return ykbPayload{}, decodeErr
	}
	return ykbPayload{Source: source, Root: root, Body: body}, nil
}

func decodeYikaobangBody(body string) (any, error) {
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return body, err
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		return body, err
	}
	return root, nil
}

func yikaobangListParams(target ykbTarget) map[string]string {
	params := map[string]string{
		"page":        "1",
		"pageSize":    "50",
		"page_size":   "50",
		"limit":       "50",
		"size":        "50",
		"app_version": ykbAppVersion,
	}
	if target.Keyword != "" {
		params["keyword"] = target.Keyword
		params["word"] = target.Keyword
	}
	return params
}

func yikaobangCourseParams(target ykbTarget) map[string]string {
	params := yikaobangListParams(target)
	setParamAliases(params, target.CourseID, "id", "course_id", "courseId", "c_id", "cid")
	setParamAliases(params, target.CategoryID, "category_id", "categoryId", "cat_id", "catalog_id")
	setParamAliases(params, target.ChapterID, "chapter_id", "chapterId")
	setParamAliases(params, target.GoodsID, "goods_id", "goodsId", "verify_goods_id")
	setParamAliases(params, target.ActivityID, "activity_id", "activityId", "xue_activity_id")
	setParamAliases(params, target.AppID, "app_id", "appId", "appid")
	return params
}

func yikaobangVideoParams(target ykbTarget) map[string]string {
	params := yikaobangCourseParams(target)
	setParamAliases(params, target.VideoID, "id", "vid", "video_id", "videoId", "video")
	return params
}

func setParamAliases(params map[string]string, value string, names ...string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, name := range names {
		params[name] = value
	}
}

func ykbLegacyAPI(path string) string { return joinYKBURL(ykbLegacyAPIBase, path) }
func ykbNewAPI(path string) string    { return joinYKBURL(ykbNewAPIBase, path) }

func joinYKBURL(baseURL, pathValue string) string {
	baseURL = strings.TrimRight(baseURL, "/") + "/"
	pathValue = strings.TrimLeft(pathValue, "/")
	return baseURL + pathValue
}

func yikaobangURLWithParams(raw string, params map[string]string) string {
	if len(params) == 0 {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.TrimSpace(params[k]) != "" {
			q.Set(k, params[k])
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func yikaobangHeaders(token string) map[string]string {
	headers := map[string]string{
		"User-Agent":       util.RandomUA(),
		"referer":          ykbRefererURL,
		"Referer":          ykbRefererURL,
		"X-Requested-With": "XMLHttpRequest",
		"app-version":      ykbAppVersion,
	}
	if token != "" {
		headers["token"] = token
		headers["authorization"] = token
		headers["Authorization"] = token
	}
	return headers
}

func yikaobangAuthToken(jar http.CookieJar) string {
	for _, name := range []string{"token", "authorization", "Authorization", "auth", "access_token", "accessToken", "ykb_token"} {
		if value := yikaobangCookieValue(jar, name); value != "" {
			return value
		}
	}
	return ""
}

func yikaobangCookieValue(jar http.CookieJar, name string) string {
	if jar == nil {
		return ""
	}
	for _, host := range []string{ykbHomeURL, "https://yikaobang.com.cn/", ykbLegacyAPIBase, ykbNewAPIBase, ykbH5Base} {
		u, err := url.Parse(host)
		if err != nil {
			continue
		}
		for _, cookie := range jar.Cookies(u) {
			if strings.EqualFold(cookie.Name, name) {
				return strings.TrimSpace(cookie.Value)
			}
		}
	}
	return ""
}

func yikaobangCookieHeader(jar http.CookieJar) string {
	if jar == nil {
		return ""
	}
	seen := map[string]bool{}
	var parts []string
	for _, host := range []string{ykbHomeURL, "https://yikaobang.com.cn/", ykbLegacyAPIBase, ykbNewAPIBase, ykbH5Base} {
		u, err := url.Parse(host)
		if err != nil {
			continue
		}
		for _, cookie := range jar.Cookies(u) {
			if cookie.Name == "" || seen[strings.ToLower(cookie.Name)] {
				continue
			}
			seen[strings.ToLower(cookie.Name)] = true
			parts = append(parts, cookie.Name+"="+cookie.Value)
		}
	}
	return strings.Join(parts, "; ")
}
