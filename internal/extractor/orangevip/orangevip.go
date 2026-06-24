// Package orangevip implements an extractor for orangevip.com courses.
package orangevip

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/nichuanfang/medigo/internal/extractor"
	"github.com/nichuanfang/medigo/internal/extractor/shared"
	"github.com/nichuanfang/medigo/internal/util"
)

const (
	referer        = "https://www.orangevip.com"
	course_url     = "https://clapp.orangevip.com/otm/web/course/list"
	info_url       = "https://clapp.orangevip.com/otm/web/course/query/coursePeriod"
	video_play_url = "https://api.baijiayun.com/web/playback/getPlayInfo?room_id=%s&token=%s&use_encrypt=0&render=jsonp"
	live_play_url  = "https://www.baijiayun.com/vod/video/getPlayUrl?vid=%s&render=jsonp&token=%s&use_encrypt=0"
	file_url       = "https://clapp.orangevip.com/otm/web/student/myCourseModelFile"
	price_url      = "https://www.orangevip.com/coursedetail/%s.html"
	token_url      = "https://clapp.orangevip.com/otm/web/course/v2/reviewPlayInfo"
	order_url      = "https://clapp.orangevip.com/otm/web/order/orderList"
)

var patterns = []string{`(?:[\w-]+\.)?orangevip\.com/`}

func init() {
	extractor.Register(&Orangevip{}, extractor.SiteInfo{Name: "Orangevip", URL: "orangevip.com", NeedAuth: true})
}

type Orangevip struct{}

func (s *Orangevip) Patterns() []string { return patterns }

type lesson struct{ VideoID, RoomID, LiveID, Name string }
type course struct{ ID, Title string }

type apiResp struct {
	CourseList        []map[string]any `json:"courseList"`
	CourseChapterList []map[string]any `json:"courseChapterList"`
	ChapterClass      []map[string]any `json:"chapterClass"`
	Data              any              `json:"data"`
	Files             []map[string]any `json:"files"`
	Orders            []map[string]any `json:"orders"`
}

var cidRe = regexp.MustCompile(`orangevip\.com/(?:clock/(\d+)|(?:my)?[cC]ourse[dD]etail/(\d+)|playcheckbjy/[^?]+/\?[^#]*?[cC]ourse[Ii]d=(\d+))`)

func (s *Orangevip) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("orangevip requires login cookies")
	}
	c := util.NewClient()
	c.SetCookieJar(opts.Cookies)
	h := headers()
	cid := parseCID(rawURL)
	courses, _ := fetchCourses(c, h)
	if cid == "" && len(courses) > 0 {
		cid = courses[0].ID
	}
	if cid == "" {
		return nil, fmt.Errorf("orangevip: cannot parse courseModelId from URL and course list is empty")
	}
	title := courseTitle(courses, cid)
	if title == "" {
		title = pageTitle(c, h, cid)
	}
	if title == "" {
		title = "orangevip_" + cid
	}
	chapters, err := fetchCourseInfo(c, h, cid)
	if err != nil {
		return nil, fmt.Errorf("orangevip coursePeriod: %w", err)
	}
	lessons := parseLessons(chapters)
	if len(lessons) == 0 {
		return nil, fmt.Errorf("orangevip: no coursePeriodList lessons in courseChapterList")
	}
	var entries []*extractor.MediaInfo
	seen := map[string]bool{}
	for i, le := range lessons {
		token := fetchToken(c, h, cid, le.VideoID)
		if token == "" {
			continue
		}
		videoURL, audioURL := resolveBaijiayun(c, h, le, token)
		if videoURL == "" || seen[videoURL] {
			continue
		}
		seen[videoURL] = true
		st := extractor.Stream{Quality: "best", URLs: []string{videoURL}, Format: formatOf(videoURL), AudioURL: audioURL, Headers: h}
		entries = append(entries, &extractor.MediaInfo{Site: "orangevip", Title: clean(fmt.Sprintf("[%d]--%s", i+1, first(le.Name, le.VideoID, le.LiveID))), Streams: map[string]extractor.Stream{"best": st}, Extra: map[string]any{"period_id": le.VideoID, "room_id": le.RoomID, "live_id": le.LiveID}})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("orangevip: no playable Baijiayun videos found from reviewPlayInfo")
	}
	return &extractor.MediaInfo{Site: "orangevip", Title: clean(title), Entries: entries, Extra: map[string]any{"course_id": cid, "files": fetchFiles(c, h, cid, "")}}, nil
}

func fetchCourses(c *util.Client, h map[string]string) ([]course, error) {
	var out []course
	for p := 1; p < 10; p++ {
		body, err := c.PostForm(course_url, map[string]string{"showCount": "99", "currentPageForApp": fmt.Sprint(p)}, h)
		if err != nil {
			return out, err
		}
		var resp apiResp
		if json.Unmarshal([]byte(body), &resp) != nil || len(resp.CourseList) == 0 {
			break
		}
		for _, it := range resp.CourseList {
			if truthy(it["isExpire"]) || fmt.Sprint(it["totalCount"]) == "0" {
				continue
			}
			id := firstText(it, "guid", "courseModelId")
			if id != "" {
				out = append(out, course{ID: id, Title: firstText(it, "courseName", "title", "name")})
			}
		}
	}
	return out, nil
}

func fetchCourseInfo(c *util.Client, h map[string]string, cid string) ([]map[string]any, error) {
	body, err := c.PostForm(info_url, map[string]string{"courseModelId": cid}, h)
	if err != nil {
		return nil, err
	}
	var resp apiResp
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return nil, err
	}
	return resp.CourseChapterList, nil
}

func parseLessons(chapters []map[string]any) []lesson {
	var out []lesson
	for ci, ch := range chapters {
		list, _ := ch["coursePeriodList"].([]any)
		for li, raw := range list {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			periodID := firstText(m, "guid")
			if periodID == "" {
				continue
			}
			name := clean(fmt.Sprintf("[%d.%d]--%s", ci+1, li+1, firstText(m, "coursePeriodTitle", "title", "name")))
			out = append(out, lesson{VideoID: periodID, RoomID: firstText(m, "roomId", "room_id"), LiveID: firstText(m, "videoId", "live_id"), Name: name})
		}
	}
	return out
}

func fetchToken(c *util.Client, h map[string]string, cid, periodID string) string {
	body, err := c.PostForm(token_url, map[string]string{"clientType": "1", "periodId": periodID, "courseId": cid}, h)
	if err != nil {
		return ""
	}
	var v any
	if json.Unmarshal([]byte(body), &v) != nil {
		return ""
	}
	return findFirst(v, "token")
}

func resolveBaijiayun(c *util.Client, h map[string]string, le lesson, token string) (string, string) {
	var videoURL string
	if le.RoomID != "" {
		if u, err := shared.BaijiayunResolvePlayback(c, le.RoomID, token, h); err == nil {
			videoURL = u
		}
	}
	if le.LiveID != "" {
		if u, err := shared.BaijiayunResolveVOD(c, le.LiveID, token, h); err == nil && u != "" {
			videoURL = u
		}
	}
	if videoURL == "" && le.RoomID != "" {
		body, err := c.GetString(fmt.Sprintf(video_play_url, url.QueryEscape(le.RoomID), url.QueryEscape(token)), h)
		if err == nil {
			videoURL = findFirstJSONP(body, "video_url", "url", "playback_url")
		}
	}
	return normalizeURL(videoURL), ""
}

func fetchFiles(c *util.Client, h map[string]string, cid, parentID string) []map[string]string {
	body, err := c.PostForm(file_url, map[string]string{"courseModelId": cid, "pguid": parentID}, h)
	if err != nil {
		return nil
	}
	var resp apiResp
	if json.Unmarshal([]byte(body), &resp) != nil {
		return nil
	}
	var out []map[string]string
	for _, f := range resp.Files {
		if truthy(f["isFolder"]) {
			out = append(out, fetchFiles(c, h, cid, firstText(f, "guid"))...)
			continue
		}
		u := firstText(f, "netUrl", "file_url", "url")
		if u == "" {
			continue
		}
		name := firstText(f, "fileName", "name", "title")
		out = append(out, map[string]string{"file_url": normalizeURL(u), "file_name": name, "file_fmt": ext(name, u), "file_id": firstText(f, "guid")})
	}
	return out
}

func parseCID(rawURL string) string {
	if m := cidRe.FindStringSubmatch(rawURL); m != nil {
		return first(m[1], m[2], m[3])
	}
	u, err := url.Parse(rawURL)
	if err == nil {
		return first(u.Query().Get("courseId"), u.Query().Get("courseid"), u.Query().Get("cid"), u.Query().Get("id"))
	}
	return ""
}

func pageTitle(c *util.Client, h map[string]string, cid string) string {
	body, err := c.GetString(fmt.Sprintf(price_url, url.PathEscape(cid)), h)
	if err != nil {
		return ""
	}
	return first(regexGroup(body, `"courseName"\s*:\s*"([^"]+)"`), regexGroup(body, `<title>(.*?)</title>`))
}

func headers() map[string]string {
	return map[string]string{"Referer": referer, "Origin": referer, "Accept": "application/json, text/plain, */*"}
}
func courseTitle(list []course, cid string) string {
	for _, c := range list {
		if c.ID == cid {
			return c.Title
		}
	}
	return ""
}
func firstText(m map[string]any, keys ...string) string {
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
func findFirst(v any, keys ...string) string {
	out := ""
	walk(v, func(m map[string]any) {
		if out == "" {
			out = firstText(m, keys...)
		}
	})
	return out
}
func walk(v any, fn func(map[string]any)) {
	switch t := v.(type) {
	case map[string]any:
		fn(t)
		for _, x := range t {
			walk(x, fn)
		}
	case []any:
		for _, x := range t {
			walk(x, fn)
		}
	}
}
func findFirstJSONP(text string, keys ...string) string {
	var v any
	body := strings.TrimSpace(text)
	if i := strings.Index(body, "("); i >= 0 && strings.HasSuffix(strings.TrimSuffix(body, ";"), ")") {
		body = body[i+1 : strings.LastIndex(body, ")")]
	}
	if json.Unmarshal([]byte(body), &v) != nil {
		return ""
	}
	return findFirst(v, keys...)
}
func first(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
func clean(s string) string {
	return strings.Trim(strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, s), " .")
}
func normalizeURL(u string) string {
	u = strings.TrimSpace(u)
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	return u
}
func formatOf(u string) string {
	if strings.Contains(strings.ToLower(u), ".m3u8") {
		return "m3u8"
	}
	return "mp4"
}
func truthy(v any) bool {
	s := strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
	return s == "1" || s == "true" || s == "yes"
}
func ext(name, u string) string {
	p := strings.Split(strings.Split(first(name, u), "?")[0], ".")
	if len(p) > 1 {
		return strings.ToLower(p[len(p)-1])
	}
	return ""
}
func regexGroup(s, pat string) string {
	if m := regexp.MustCompile(pat).FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}
