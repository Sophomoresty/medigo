// Package gaodun implements source-aligned Gaodun course and glive2-vod extraction.
package gaodun

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/nichuanfang/medigo/internal/extractor"
	"github.com/nichuanfang/medigo/internal/util"
)

const (
	course_url              = "https://apigateway.gaodun.com/ep-course/api/v2/front/space/vcourse/pc"
	info_url                = "https://apigateway.gaodun.com/ep-study/front/course/%s/syllabus"
	info_gradation_url      = "https://apigateway.gaodun.com/g-study/api/v1/front/gl/course/gradation/%s"
	info_glive_url          = "https://apigateway.gaodun.com/g-study/api/v1/front/course/%s/syllabus/glive/%s"
	info_syllabus_url       = "https://apigateway.gaodun.com/ep-study/front/course/%s/syllabus/%s"
	video_play_url          = "https://apigateway.gaodun.com/glive2-vod/api/v1/live/resource?code=%s&res=%s"
	live_token_url          = "https://apigateway.gaodun.com/glive2-vod/api/v1/vod/check?token=%s"
	live_play_url           = "https://apigateway.gaodun.com/glive2-vod/api/v1/live/resource?code=%s&res=%s"
	live_old_url            = "https://apigateway.gaodun.com/glive2-cloud-gateway/api/v1/live/record/info/%s"
	source_category_url     = "https://apigateway.gaodun.com/ep-course/api/v1/course/%s/handout/category"
	source_gradation_url    = "https://apigateway.gaodun.com/ep-course/api/v1/course/%s/gradation/handout"
	file_url                = "https://apigateway.gaodun.com/hermes/front/v1/download/resource?resource_id=%s&filename=%s"
	price_url               = "https://apigateway.gaodun.com/goodscenter/api/v3/vcourse/detailByIds?ids=%s"
	pc_token_url            = "https://apigateway.gaodun.com/glive2-cloud-gateway/api/v1/live/getPc"
	pe_token_url            = "https://apigateway.gaodun.com/glive2-cloud-gateway/api/v1/live/getPe"
	passport_glive_user_url = "https://apigateway.gaodun.com/passport/api/v3/get/glive-user-info"
)

var patterns = []string{`(?:[\w-]+\.)?gaodun\.com/|apigateway\.gaodun\.com/`}

func init() {
	extractor.Register(&Gaodun{}, extractor.SiteInfo{Name: "Gaodun", URL: "gaodun.com", NeedAuth: true})
}

type Gaodun struct{}

func (g *Gaodun) Patterns() []string { return patterns }

var (
	cidRe        = regexp.MustCompile(`(?i)(?:courseId|course_id|cid|ids?)=([A-Za-z0-9_-]+)|/(?:course|vcourse)/(\w+)`)
	videoIDRe    = regexp.MustCompile(`(?i)(?:videoId|video_id|vid|code|did)=([A-Za-z0-9_-]+)`)
	recordIDRe   = regexp.MustCompile(`(?i)(?:record_id|recordId)=([A-Za-z0-9_-]+)|/record/info/(\w+)`)
	syllabusIDRe = regexp.MustCompile(`(?i)(?:syllabus_id|syllabusId)=([A-Za-z0-9_-]+)`)
	tokenRe      = regexp.MustCompile(`(?i)(?:token)=([A-Za-z0-9._-]+)`)
)

type requestIDs struct {
	CourseID   string
	SyllabusID string
	VideoID    string
	RecordID   string
	Token      string
}

type videoNode struct {
	ID    string
	Title string
	Kind  string
}

func (g *Gaodun) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("gaodun requires login cookies")
	}
	ids := parseIDs(rawURL)
	if ids.CourseID == "" && ids.VideoID == "" && ids.RecordID == "" && ids.Token == "" {
		return nil, fmt.Errorf("cannot parse gaodun cid/video id from URL: %s", rawURL)
	}

	c := util.NewClient()
	c.SetCookieJar(opts.Cookies)
	headers := map[string]string{
		"Accept":  "application/json, text/plain, */*",
		"Referer": "https://www.gaodun.com",
		"Origin":  "https://www.gaodun.com",
	}

	if ids.VideoID != "" || ids.RecordID != "" || ids.Token != "" {
		entry, err := resolveDirect(c, headers, ids, "gaodun_"+firstNonEmpty(ids.VideoID, ids.RecordID, ids.Token))
		if err != nil {
			return nil, err
		}
		return entry, nil
	}

	entries, title, err := resolveCourse(c, headers, ids)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("gaodun: no playable resource found for course %s", ids.CourseID)
	}
	return &extractor.MediaInfo{Site: "gaodun", Title: util.SanitizeFilename(firstNonEmpty(title, "gaodun_"+ids.CourseID)), Entries: entries}, nil
}

func resolveCourse(c *util.Client, headers map[string]string, ids requestIDs) ([]*extractor.MediaInfo, string, error) {
	apis := []string{
		fmt.Sprintf(info_url, url.PathEscape(ids.CourseID)),
		fmt.Sprintf(info_gradation_url, url.PathEscape(ids.CourseID)),
		fmt.Sprintf(source_category_url, url.PathEscape(ids.CourseID)),
		fmt.Sprintf(source_gradation_url, url.PathEscape(ids.CourseID)),
	}
	if ids.SyllabusID != "" {
		apis = append([]string{
			fmt.Sprintf(info_glive_url, url.PathEscape(ids.CourseID), url.PathEscape(ids.SyllabusID)),
			fmt.Sprintf(info_syllabus_url, url.PathEscape(ids.CourseID), url.PathEscape(ids.SyllabusID)),
		}, apis...)
	}

	var nodes []videoNode
	var direct []*extractor.MediaInfo
	var title string
	for _, api := range apis {
		body, err := c.GetString(api, headers)
		if err != nil {
			continue
		}
		var payload any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			continue
		}
		title = firstNonEmpty(title, pickTitle(payload))
		if u := findMediaURL(payload); u != "" {
			direct = append(direct, mediaInfo(firstNonEmpty(pickTitle(payload), ids.CourseID), u, headers))
		}
		nodes = append(nodes, collectVideoNodes(payload)...)
	}
	if len(direct) > 0 {
		return direct, title, nil
	}
	if len(nodes) == 0 {
		return nil, title, nil
	}

	seen := map[string]bool{}
	entries := make([]*extractor.MediaInfo, 0, len(nodes))
	for _, n := range nodes {
		if n.ID == "" || seen[n.ID] {
			continue
		}
		seen[n.ID] = true
		entry, err := resolveDirect(c, headers, requestIDs{VideoID: n.ID}, n.Title)
		if err == nil {
			entries = append(entries, entry)
		}
	}
	return entries, title, nil
}

func resolveDirect(c *util.Client, headers map[string]string, ids requestIDs, fallbackTitle string) (*extractor.MediaInfo, error) {
	var payloads []any
	fetchJSON := func(api string) {
		body, err := c.GetString(api, headers)
		if err != nil {
			return
		}
		var payload any
		if json.Unmarshal([]byte(body), &payload) == nil {
			payloads = append(payloads, payload)
		}
	}

	if ids.RecordID != "" {
		fetchJSON(fmt.Sprintf(live_old_url, url.PathEscape(ids.RecordID)))
	}
	if ids.Token != "" {
		fetchJSON(fmt.Sprintf(live_token_url, url.QueryEscape(ids.Token)))
	}
	if ids.VideoID != "" {
		for _, mode := range []string{"FHD", "HD", "SD"} {
			fetchJSON(fmt.Sprintf(video_play_url, url.QueryEscape(ids.VideoID), mode))
			fetchJSON(fmt.Sprintf(live_play_url, url.QueryEscape(ids.VideoID), mode))
		}
	}

	for _, payload := range payloads {
		if u := findMediaURL(payload); u != "" {
			title := util.SanitizeFilename(firstNonEmpty(pickTitle(payload), fallbackTitle, "gaodun_"+firstNonEmpty(ids.VideoID, ids.RecordID, ids.Token)))
			return mediaInfo(title, u, headers), nil
		}
	}
	return nil, fmt.Errorf("gaodun: no path/playUrl from glive2-vod for %s", firstNonEmpty(ids.VideoID, ids.RecordID, ids.Token))
}

func parseIDs(raw string) requestIDs {
	out := requestIDs{}
	if u, err := url.Parse(raw); err == nil {
		q := u.Query()
		out.CourseID = firstNonEmpty(q.Get("courseId"), q.Get("course_id"), q.Get("cid"), q.Get("ids"), q.Get("id"))
		out.SyllabusID = firstNonEmpty(q.Get("syllabus_id"), q.Get("syllabusId"))
		out.VideoID = firstNonEmpty(q.Get("videoId"), q.Get("video_id"), q.Get("vid"), q.Get("code"), q.Get("did"))
		out.RecordID = firstNonEmpty(q.Get("record_id"), q.Get("recordId"))
		out.Token = q.Get("token")
	}
	out.CourseID = firstNonEmpty(out.CourseID, rx(cidRe, raw))
	out.SyllabusID = firstNonEmpty(out.SyllabusID, rx(syllabusIDRe, raw))
	out.VideoID = firstNonEmpty(out.VideoID, rx(videoIDRe, raw))
	out.RecordID = firstNonEmpty(out.RecordID, rx(recordIDRe, raw))
	out.Token = firstNonEmpty(out.Token, rx(tokenRe, raw))
	return out
}

func collectVideoNodes(v any) []videoNode {
	var out []videoNode
	var walk func(any, string)
	walk = func(x any, prefix string) {
		switch vv := x.(type) {
		case map[string]any:
			title := firstNonEmpty(valueString(vv, "name", "title", "courseName", "syllabusName"), prefix)
			id := valueString(vv, "videoId", "video_id", "did", "code", "resourceId")
			kind := valueString(vv, "type", "resourceType", "videoType")
			if id != "" && (kind == "" || strings.Contains(strings.ToLower(kind), "video") || hasAny(vv, "resource", "path")) {
				out = append(out, videoNode{ID: id, Title: title, Kind: kind})
			}
			if res, ok := vv["resource"]; ok {
				walk(res, title)
			}
			for _, k := range []string{"children", "items", "list", "syllabus", "courseList", "result", "data"} {
				if child, ok := vv[k]; ok {
					walk(child, title)
				}
			}
		case []any:
			for _, child := range vv {
				walk(child, prefix)
			}
		}
	}
	walk(v, "")
	return out
}

func findMediaURL(v any) string {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range []string{"path", "playUrl", "play_url", "url", "m3u8", "m3u8Url", "file_url", "fileUrl"} {
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

func mediaInfo(title, u string, headers map[string]string) *extractor.MediaInfo {
	format := "mp4"
	if strings.Contains(strings.ToLower(u), ".m3u8") {
		format = "m3u8"
	}
	return &extractor.MediaInfo{Site: "gaodun", Title: util.SanitizeFilename(title), Streams: map[string]extractor.Stream{"best": {Quality: "best", URLs: []string{u}, Format: format, Headers: headers}}}
}

func pickTitle(v any) string {
	switch x := v.(type) {
	case map[string]any:
		if s := valueString(x, "courseName", "name", "title", "syllabusName"); s != "" {
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

func rx(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	for i := 1; i < len(m); i++ {
		if m[i] != "" {
			return m[i]
		}
	}
	return ""
}

func normalizeURL(s string) string {
	s = strings.TrimSpace(strings.Trim(s, `"'`))
	s = strings.ReplaceAll(s, `\/`, `/`)
	if strings.HasPrefix(s, "//") {
		return "https:" + s
	}
	return s
}

func isMediaURL(s string) bool {
	low := strings.ToLower(s)
	return strings.HasPrefix(low, "http") && (strings.Contains(low, ".m3u8") || strings.Contains(low, ".mp4") || strings.Contains(low, ".flv") || strings.Contains(low, ".mp3"))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
