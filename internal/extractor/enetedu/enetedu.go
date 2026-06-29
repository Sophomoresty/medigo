// Package enetedu implements the Enetedu admin-api course extractor.
package enetedu

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

const (
	origin             = "https://www.enetedu.com"
	referer            = origin + "/"
	login_url          = origin + "/site/login"
	api_base           = origin + "/admin-api"
	token_key          = "eneteduToken"
	user_agent         = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	detail_path        = "/course/broadcast/glanceAndGet"
	task_tree_path     = "/course/broadcast/task/homeView"
	task_node_path     = "/course/broadcast/task/node/get"
	course_file_path   = "/media/course-resources/courseFileList"
	learning_tree_path = "/media/course-learning-info/learningCourseTreeList"
	transcode_path     = "/media/course-info/getMediaTranscodeInfo"
	playback_deal_path = "/media/broadcast/dealBackUrl"
	url0               = "https://www.enetedu.com/site/course/liveCourseDetails?id=2033384670799990785"
)

var patterns = []string{`(?:[\w-]+\.)?enetedu\.com/`}

func init() {
	extractor.Register(&Enetedu{}, extractor.SiteInfo{Name: "Enetedu", URL: "enetedu.com", NeedAuth: true})
}

type Enetedu struct{}

func (s *Enetedu) Patterns() []string { return patterns }

var idRe = regexp.MustCompile(`(?i)(?:\?|&)(?:id|courseId)=([A-Za-z0-9_-]+)|/(?:liveCourseDetails|course)/(\w+)`)

type videoInfo struct {
	Title     string
	NodeID    string
	VideoID   string
	ChapterID string
	FileName  string
	URL       string
	Raw       map[string]any
}

func (s *Enetedu) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("enetedu requires login cookies")
	}
	detailID := parseID(rawURL)
	if detailID == "" {
		return nil, fmt.Errorf("cannot parse enetedu course id from URL: %s", rawURL)
	}

	c := util.NewClient()
	c.SetCookieJar(opts.Cookies)
	headers := requestHeaders(opts.Cookies, rawURL)
	if err := validateLogin(c, headers); err != nil {
		return nil, err
	}

	detail, err := requestJSON(c, detail_path, map[string]string{"id": detailID}, nil, "GET", headers)
	if err != nil {
		return nil, fmt.Errorf("fetch enetedu detail: %w", err)
	}
	data := dataOf(detail)
	courseID := firstNonEmpty(valueString(data, "courseId", "id"), detailID)
	title := firstNonEmpty(valueString(data, "courseName", "name", "title"), "高校教师网培课程"+detailID)

	entries := []*extractor.MediaInfo{}
	for _, v := range parseLiveTasks(c, headers, courseID) {
		if entry := resolveVideo(c, headers, courseID, v); entry != nil {
			entries = append(entries, entry)
		}
	}
	for _, v := range parseLearningTree(c, headers, courseID) {
		if entry := resolveVideo(c, headers, courseID, v); entry != nil {
			entries = append(entries, entry)
		}
	}
	if media := findMediaURL(detail); media != "" {
		entries = append(entries, mediaInfo(title, media, headers))
	}
	entries = append(entries, fileEntries(parseNoticeFiles(data), headers)...)
	entries = append(entries, fileEntries(parseCourseFiles(c, headers, courseID), headers)...)
	if len(entries) == 0 {
		return nil, fmt.Errorf("enetedu: no playable video URL or course file for course %s", courseID)
	}
	return &extractor.MediaInfo{Site: "enetedu", Title: util.SanitizeFilename(title), Entries: dedupe(entries)}, nil
}

func parseLiveTasks(c *util.Client, headers map[string]string, courseID string) []videoInfo {
	payload, err := requestJSON(c, task_tree_path, map[string]string{"id": courseID}, nil, "GET", headers)
	if err != nil {
		return nil
	}
	var out []videoInfo
	walkLivePayload(dataOfAny(payload), &out)
	return out
}

func parseLearningTree(c *util.Client, headers map[string]string, courseID string) []videoInfo {
	payload, err := requestJSON(c, learning_tree_path, map[string]string{"courseId": courseID, "type": "1"}, nil, "GET", headers)
	if err != nil {
		return nil
	}
	var out []videoInfo
	walkLearningPayload(dataOfAny(payload), &out)
	return out
}

func parseCourseFiles(c *util.Client, headers map[string]string, courseID string) []fileInfo {
	if courseID == "" {
		return nil
	}
	payload, err := requestJSON(c, course_file_path, map[string]string{"courseId": courseID}, nil, "GET", headers)
	if err != nil {
		return nil
	}
	var out []fileInfo
	walkFilePayload(dataOfAny(payload), &out)
	return out
}

func resolveVideo(c *util.Client, headers map[string]string, courseID string, v videoInfo) *extractor.MediaInfo {
	media := v.URL
	if !isMediaURL(media) && v.VideoID != "" {
		media = resolveLearningURL(c, headers, courseID, v)
	}
	if !isMediaURL(media) && v.NodeID != "" {
		media = resolveNodeURL(c, headers, v.NodeID)
	}
	if !isMediaURL(media) {
		return nil
	}
	title := firstNonEmpty(v.Title, v.FileName, v.VideoID, v.NodeID, courseID)
	return mediaInfo(title, media, headers)
}

func resolveNodeURL(c *util.Client, headers map[string]string, nodeID string) string {
	payload, err := requestJSON(c, task_node_path, map[string]string{"id": nodeID}, nil, "GET", headers)
	if err != nil {
		return ""
	}
	data := dataOfAny(payload)
	media := findMediaURL(data)
	if media == "" {
		media = valueString(data, "sourceAddress", "playbackUrl", "url")
	}
	if isMediaURL(media) {
		return media
	}
	return dealPlaybackURL(c, headers, media)
}

func resolveLearningURL(c *util.Client, headers map[string]string, courseID string, v videoInfo) string {
	body := map[string]any{"fileName": v.FileName, "chapterId": v.ChapterID, "videoId": v.VideoID, "courseId": courseID}
	payload, err := requestJSON(c, transcode_path, nil, body, "POST", headers)
	if err != nil {
		return ""
	}
	data := dataOfAny(payload)
	if media := findMediaURL(data); media != "" {
		return media
	}
	return ""
}

func dealPlaybackURL(c *util.Client, headers map[string]string, raw string) string {
	if raw == "" {
		return ""
	}
	payload, err := requestJSON(c, playback_deal_path, nil, map[string]any{"url": raw}, "POST", headers)
	if err != nil {
		return ""
	}
	return findMediaURL(payload)
}

func requestJSON(c *util.Client, path string, params map[string]string, body map[string]any, method string, headers map[string]string) (any, error) {
	api := apiURL(path)
	if len(params) > 0 {
		q := url.Values{}
		for k, v := range params {
			q.Set(k, v)
		}
		sep := "?"
		if strings.Contains(api, "?") {
			sep = "&"
		}
		api += sep + q.Encode()
	}
	if strings.EqualFold(method, "POST") {
		buf, _ := json.Marshal(body)
		h := cloneHeaders(headers)
		h["Content-Type"] = "application/json;charset=UTF-8"
		resp, err := c.Post(api, strings.NewReader(string(buf)), h)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		var payload any
		if err := json.Unmarshal(b, &payload); err != nil {
			return nil, err
		}
		return payload, nil
	}
	text, err := c.GetString(api, headers)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func parseID(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		q := u.Query()
		if v := firstNonEmpty(q.Get("id"), q.Get("courseId")); v != "" {
			return v
		}
	}
	m := idRe.FindStringSubmatch(raw)
	for i := 1; i < len(m); i++ {
		if m[i] != "" {
			return m[i]
		}
	}
	return ""
}

func requestHeaders(jar http.CookieJar, raw string) map[string]string {
	headers := map[string]string{
		"Accept":     "application/json, text/plain, */*",
		"Referer":    firstNonEmpty(raw, referer),
		"Origin":     origin,
		"User-Agent": user_agent,
	}
	cookieParts := []string{}
	seenCookies := map[string]bool{}
	for _, host := range []string{"www.enetedu.com", "enetedu.com"} {
		u := &url.URL{Scheme: "https", Host: host, Path: "/"}
		for _, cookie := range jar.Cookies(u) {
			key := cookie.Name + "=" + cookie.Value
			if !seenCookies[key] {
				seenCookies[key] = true
				cookieParts = append(cookieParts, key)
			}
			if cookie.Name == token_key || strings.EqualFold(cookie.Name, "token") {
				headers[token_key] = cookie.Value
				headers["Authorization"] = cookie.Value
			}
			mergeLoginPayloadHeaders(headers, cookie.Value)
		}
	}
	if len(cookieParts) > 0 {
		cookieHeader := strings.Join(cookieParts, "; ")
		headers["Cookie"] = cookieHeader
		headers["cookie"] = cookieHeader
	}
	return headers
}

func mergeLoginPayloadHeaders(headers map[string]string, raw string) {
	payload := normalizeLoginPayload(raw)
	if len(payload) == 0 {
		return
	}
	storage := storageDict(payload)
	token := firstNonEmpty(valueString(payload, token_key, "token"), valueString(storage, token_key, "token"))
	if token != "" {
		headers[token_key] = token
		headers["Authorization"] = token
	}
	if cookieText := firstNonEmpty(valueString(payload, "cookie", "cookieValue", "cookie_string"), cookiesString(payload)); cookieText != "" {
		headers["Cookie"] = cookieText
		headers["cookie"] = cookieText
	}
}

func normalizeLoginPayload(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if unescaped, err := url.QueryUnescape(raw); err == nil {
		raw = strings.TrimSpace(unescaped)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		return payload
	}
	if vals, err := url.ParseQuery(raw); err == nil && len(vals) > 0 {
		payload = map[string]any{}
		for k, v := range vals {
			if len(v) > 0 {
				payload[k] = v[0]
			}
		}
		return payload
	}
	return nil
}

func storageDict(payload map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"localStorage", "local_storage", "sessionStorage", "session_storage", "storage"} {
		if m, ok := payload[key].(map[string]any); ok {
			for k, v := range m {
				out[k] = v
			}
		}
	}
	return out
}

func cookiesString(payload map[string]any) string {
	if items, ok := payload["cookies"].([]any); ok {
		parts := make([]string, 0, len(items))
		for _, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			name := valueString(m, "name")
			value := valueString(m, "value")
			if name != "" && value != "" {
				parts = append(parts, name+"="+value)
			}
		}
		return strings.Join(parts, "; ")
	}
	if m, ok := payload["cookies"].(map[string]any); ok {
		parts := make([]string, 0, len(m))
		for k, v := range m {
			value := strings.TrimSpace(fmt.Sprint(v))
			if k != "" && value != "" && value != "<nil>" {
				parts = append(parts, k+"="+value)
			}
		}
		return strings.Join(parts, "; ")
	}
	return ""
}

func validateLogin(c *util.Client, headers map[string]string) error {
	if firstNonEmpty(headers[token_key], headers["Authorization"]) == "" {
		return fmt.Errorf("enetedu requires %s token", token_key)
	}
	payload, err := requestJSON(c, "/site/user/baseinfo", nil, nil, "GET", headers)
	if err != nil {
		return fmt.Errorf("enetedu login check: %w", err)
	}
	data := dataOfAny(payload)
	if valueString(data, "id", "userId", "name", "nickname") == "" {
		return fmt.Errorf("enetedu login check returned no user")
	}
	return nil
}
