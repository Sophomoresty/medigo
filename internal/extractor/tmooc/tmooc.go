// Package tmooc implements an extractor for tmooc.cn (达内TMOOC) courses.
package tmooc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/extractor/shared"
	"github.com/Sophomoresty/mediago/internal/util"
)

const (
	USER_AGENT            = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	referer               = "https://www.tmooc.cn/"
	tts_referer           = "https://tts10.tmooc.cn/#/"
	home_url              = "https://www.tmooc.cn/"
	tts_home_url          = "https://tts10.tmooc.cn/"
	legacy_course_api     = "https://uc.tmooc.cn/studentCenter/toMyttsPage"
	base_course_login_api = "https://ttsservice.tmooc.cn/tedu-student/v1/sso-tmooc"
	user_info_api         = "https://uc.tmooc.cn/userValidate/getUserInfo"
	course_outline_api    = "https://ttsservice.tmooc.cn/tedu-student/v1/study-center/formal"
	my_course_api         = "https://ttsservice.tmooc.cn/tedu-student/v1/study-center/get-my-course"
	valid_version_api     = "https://ttsservice.tmooc.cn/tedu-student/v1/study-center/all-vailid-version"
	change_version_api    = "https://ttsservice.tmooc.cn/tedu-student/v1/study-center/change-version/%s"
	course_login_api      = "https://ttsservice.tmooc.cn/tedu-student/v1/login/toLogin"
	video_play_api        = "https://ttsservice.tmooc.cn/tedu-student/v1/video/find-playback-msg/%s"
	user_course_api       = "https://uc.tmooc.cn/userCenter/findShowUserCourse"
	web_check_video_api   = "https://uc.tmooc.cn/video/checkVideo"
	web_course_detail_url = "https://www.tmooc.cn/course/%s.shtml"
	web_player_url        = "https://www.tmooc.cn/player/index.shtml?videoId=%s&courseId=%s"
	bokecc_site_id        = "0DD1F081022C163E"
	bokecc_video_api      = "https://p.bokecc.com/servlet/getvideofile?vid=%s&siteid=%s"
)

var patterns = []string{`(?:[\w-]+\.)?tmooc\.cn/`}

func init() {
	extractor.Register(&Tmooc{}, extractor.SiteInfo{Name: "Tmooc", URL: "tmooc.cn", NeedAuth: true})
}

type Tmooc struct{}

func (t *Tmooc) Patterns() []string { return patterns }

func (t *Tmooc) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("tmooc requires login cookies")
	}
	c := util.NewClient()
	c.SetCookieJar(opts.Cookies)
	h := headersFromJar(opts.Cookies)
	if strings.Contains(rawURL, "www.tmooc.cn/course/") || strings.Contains(rawURL, ".shtml") || strings.Contains(rawURL, "player/index") {
		if mi, err := extractWebCourse(c, h, rawURL); err == nil {
			return mi, nil
		}
	}
	return extractTTSCourse(c, h, rawURL)
}

type webVideo struct{ VideoID, StageID, CourseID, Title, DirectURL string }
type ttsVideo struct{ VideoID, Title string }

func extractWebCourse(c *util.Client, h map[string]string, rawURL string) (*extractor.MediaInfo, error) {
	courseID := first(match1(rawURL, `/course/(\d+)\.shtml`), match1(rawURL, `[?&]courseId=(\d+)`))
	if courseID == "" {
		return nil, fmt.Errorf("cannot parse tmooc web course id")
	}
	body, err := c.GetString(fmt.Sprintf(web_course_detail_url, url.PathEscape(courseID)), h)
	if err != nil {
		return nil, fmt.Errorf("tmooc web detail: %w", err)
	}
	if hid := match1(body, `id=['"]courseId['"][^>]*value=['"]([^'"]+)['"]`); hid != "" {
		courseID = hid
	}
	title := first(cleanText(match1(body, `<h3[^>]+id=['"]class_title['"][^>]*>(.*?)</h3>`)), cleanText(match1(body, `<title>(.*?)</title>`)), "tmooc_"+courseID)
	videos := collectWebVideos(body, courseID)
	var entries []*extractor.MediaInfo
	for _, v := range videos {
		play := v.DirectURL
		if play == "" {
			play = resolveWebVideo(c, h, v)
		}
		if play == "" {
			continue
		}
		entries = append(entries, media("tmooc", v.Title, play, map[string]any{"video_id": v.VideoID, "stage_id": v.StageID, "course_id": courseID}))
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("tmooc web: no playable videos found")
	}
	return &extractor.MediaInfo{Site: "tmooc", Title: sanitize(title), Entries: entries, Extra: map[string]any{"course_id": courseID, "mode": "web", "price": extractWebCoursePrice(body), "purchased": true}}, nil
}
func extractTTSCourse(c *util.Client, h map[string]string, rawURL string) (*extractor.MediaInfo, error) {
	courseID := first(match1(rawURL, `[?&](?:courseId|stuClassId|studentClassId|id)=(\w+)`), match1(rawURL, `/course/(\w+)`))
	courses, lastPayload := requestCourseList(c, h)
	selected := map[string]any{}
	for _, it := range courses {
		if courseID == "" || containsID(it, courseID) {
			selected = it
			courseID = firstID(it)
			break
		}
	}
	if courseID == "" && len(courses) == 0 {
		courseID = firstID(unwrapMap(lastPayload))
	}
	if courseID == "" {
		return nil, fmt.Errorf("cannot parse tmooc course id from URL or course list")
	}
	activation := activateCourse(c, h, selected)
	outline, err := requestJSON(c, course_outline_api, nil, h)
	if err != nil {
		return nil, fmt.Errorf("tmooc course outline: %w", err)
	}
	videos := collectTTSVideos(unwrapMap(outline))
	if len(videos) == 0 {
		videos = collectTTSVideos(selected)
	}
	var entries []*extractor.MediaInfo
	for _, v := range videos {
		play := resolveTTSVideo(c, h, v.VideoID)
		if play == "" {
			continue
		}
		entries = append(entries, media("tmooc", v.Title, play, map[string]any{"video_id": v.VideoID, "course_id": courseID, "mode": "tts"}))
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("tmooc tts: no playable videos found")
	}
	title := first(extractCourseTitle(selected), "tmooc_"+courseID)
	return &extractor.MediaInfo{Site: "tmooc", Title: sanitize(title), Entries: entries, Extra: map[string]any{"course_id": courseID, "mode": "tts", "price": extractCoursePrice(selected), "purchased": extractCoursePurchased(selected), "activation": activation}}, nil
}
func requestCourseList(c *util.Client, h map[string]string) ([]map[string]any, any) {
	apis := []struct{ url, ref string }{{legacy_course_api, tts_referer + "studentCenter/toMyttsPage"}, {my_course_api, tts_referer}, {valid_version_api, tts_referer}, {user_course_api, referer}}
	var last any
	for _, it := range apis {
		hh := clone(h)
		hh["Referer"] = it.ref
		resp, err := requestJSON(c, it.url, nil, hh)
		if err != nil {
			continue
		}
		last = resp
		list := extractList(resp)
		if len(list) > 0 {
			return list, resp
		}
	}
	return nil, last
}
func activateCourse(c *util.Client, h map[string]string, selected map[string]any) any {
	candidates := collectIDs(selected)
	seen := map[string]bool{}
	var last any
	for _, id := range candidates {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if resp := loginWithStuClassID(c, h, id); resp != nil {
			last = resp
			if token := extractToken(resp); token != "" {
				setTokenHeaders(h, token)
				setStuClassHeaders(h, id)
				return resp
			}
		}
	}
	for _, id := range candidates {
		if id == "" {
			continue
		}
		resp, err := requestJSON(c, fmt.Sprintf(change_version_api, url.PathEscape(id)), nil, mergeHeaders(h, map[string]string{"Origin": "https://tts10.tmooc.cn", "Referer": tts_referer}))
		if err == nil {
			last = resp
			if token := extractToken(resp); token != "" {
				setTokenHeaders(h, token)
				return resp
			}
		}
	}
	for _, id := range candidates {
		if id == "" {
			continue
		}
		resp, err := requestJSON(c, base_course_login_api, map[string]string{"stuClassId": id}, mergeHeaders(h, map[string]string{"Origin": "https://tts10.tmooc.cn", "Referer": tts_referer + "sso-tmooc?ttsSsoRedirect=true&stuClassId=" + url.QueryEscape(id)}))
		if err == nil {
			last = resp
			if token := extractToken(resp); token != "" {
				setTokenHeaders(h, token)
				setStuClassHeaders(h, id)
				return resp
			}
		}
	}
	return last
}
func collectWebVideos(page string, courseID string) []webVideo {
	var out []webVideo
	liRe := regexp.MustCompile(`(?is)<li\b[^>]*class=['"][^'"]*xtype2[^'"]*['"][^>]*>.*?</li>`)
	for i, block := range liRe.FindAllString(page, -1) {
		attrs := parseAttrs(block)
		stageID, videoID := first(attrs["data-stageid"], attrs["stageid"]), first(attrs["data-videoid"], attrs["videoid"])
		if stageID == "" || videoID == "" {
			continue
		}
		name := cleanText(first(match1(block, `<span class=['"]textx text-overflow['"][^>]*>(.*?)</span>`), block))
		out = append(out, webVideo{VideoID: videoID, StageID: stageID, CourseID: courseID, Title: sanitize(fmt.Sprintf("[%d]--%s", i+1, first(name, "未知视频"))), DirectURL: attrs["data-turl"]})
	}
	return out
}
func resolveWebVideo(c *util.Client, h map[string]string, v webVideo) string {
	player := fmt.Sprintf(web_player_url, url.QueryEscape(v.VideoID), url.QueryEscape(v.CourseID))
	resp, err := requestJSON(c, web_check_video_api, map[string]string{"courseId": v.CourseID, "stageId": v.StageID, "videoId": v.VideoID}, mergeHeaders(h, map[string]string{"X-Requested-With": "XMLHttpRequest", "Referer": player}))
	if err != nil {
		return ""
	}
	obj := unwrapMap(unwrapMap(resp)["obj"])
	guid := first(textAt(obj, "guid", "ccGuid"), findText(resp, "guid", "ccGuid"))
	if guid == "" {
		return findURL(resp)
	}
	play, err := shared.BokeCCResolve(c, guid, bokecc_site_id, mergeHeaders(h, map[string]string{"Referer": player}))
	if err != nil {
		return ""
	}
	return play
}
func collectTTSVideos(v any) []ttsVideo {
	var out []ttsVideo
	var walk func(any, []int)
	walk = func(x any, prefix []int) {
		switch t := x.(type) {
		case map[string]any:
			vid := textAt(t, "videoId", "video_id", "id")
			name := first(textAt(t, "videoName", "name", "title", "knowledgeName"), "未知视频")
			if vid != "" && (t["videoName"] != nil || t["knowledgeName"] != nil || t["duration"] != nil || t["playback"] != nil) {
				out = append(out, ttsVideo{VideoID: vid, Title: sanitize(fmt.Sprintf("[%s]--%s", joinInts(prefix, len(out)+1), name))})
			}
			for _, k := range []string{"bigStageList", "smallStageList", "knowledgeList", "videoList", "children", "list", "data"} {
				walk(t[k], append(prefix, len(out)+1))
			}
		case []any:
			for i, e := range t {
				walk(e, append(prefix, i+1))
			}
		}
	}
	walk(v, nil)
	return out
}
func resolveTTSVideo(c *util.Client, h map[string]string, vid string) string {
	resp, err := requestJSON(c, fmt.Sprintf(video_play_api, url.PathEscape(vid)), nil, mergeHeaders(h, map[string]string{"Referer": tts_referer}))
	if err != nil {
		return ""
	}
	return first(findURL(resp), textAt(unwrapMap(resp), "playUrl", "videoUrl", "url"))
}
func requestJSON(c *util.Client, api string, params map[string]string, h map[string]string) (any, error) {
	return requestJSONWithMethod(c, api, params, nil, nil, h, httpMethodGet)
}

const (
	httpMethodGet  = "get"
	httpMethodPost = "post"
)

func loginWithStuClassID(c *util.Client, h map[string]string, stuClassID string) any {
	stuClassID = strings.TrimSpace(stuClassID)
	if stuClassID == "" {
		return nil
	}
	setStuClassHeaders(h, stuClassID)
	headers := mergeHeaders(h, map[string]string{"Origin": "https://tts10.tmooc.cn", "Referer": tts_referer})
	attempts := []struct {
		method string
		params map[string]string
		form   map[string]string
		json   map[string]any
	}{
		{method: httpMethodPost, json: map[string]any{"stuClassId": stuClassID}},
		{method: httpMethodPost, form: map[string]string{"stuClassId": stuClassID}},
		{method: httpMethodGet, params: map[string]string{"stuClassId": stuClassID}},
	}
	var last any
	for _, attempt := range attempts {
		resp, err := requestJSONWithMethod(c, course_login_api, attempt.params, attempt.form, attempt.json, headers, attempt.method)
		if err != nil {
			continue
		}
		last = resp
		if token := extractToken(resp); token != "" {
			setTokenHeaders(h, token)
			return resp
		}
	}
	return last
}

func requestJSONWithMethod(c *util.Client, api string, params map[string]string, form map[string]string, jsonData map[string]any, h map[string]string, method string) (any, error) {
	u, err := url.Parse(api)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	var body []byte
	switch strings.ToLower(first(method, httpMethodGet)) {
	case httpMethodPost:
		headers := clone(h)
		var reader io.Reader
		if jsonData != nil {
			raw, err := json.Marshal(jsonData)
			if err != nil {
				return nil, err
			}
			reader = bytes.NewReader(raw)
			headers["Content-Type"] = "application/json;charset=UTF-8"
		} else {
			values := url.Values{}
			for k, v := range form {
				values.Set(k, v)
			}
			reader = strings.NewReader(values.Encode())
			headers["Content-Type"] = "application/x-www-form-urlencoded"
		}
		resp, err := c.Post(u.String(), reader, headers)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, u.String())
		}
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
	default:
		text, err := c.GetString(u.String(), h)
		if err != nil {
			return nil, err
		}
		body = []byte(text)
	}
	var out any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}
