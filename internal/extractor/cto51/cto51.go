// Package cto51 implements 51CTO course and training extraction.
//
// Source alignment:
//
//	Mooc/Courses/Cto51/Cto51_Base.pyc.1shot.cdc.py
//	Mooc/Courses/Cto51/Cto51_Course.pyc.1shot.cdc.py
package cto51

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/extractor/shared"
	"github.com/Sophomoresty/mediago/internal/util"
)

const (
	urlStudyCourse        = "https://edu.51cto.com/center/course/user/get-study-course"
	urlWejobIndex         = "https://edu.51cto.com/center/wejob/user/index?train_id=%s"
	urlWejobCourse        = "https://edu.51cto.com/center/wejob/user/course?train_id=%s"
	urlEStudy             = "https://e.51cto.com/study"
	urlCourse             = "https://edu.51cto.com/course/%s.html"
	urlLesson             = "https://edu.51cto.com/lesson/%s.html"
	urlTrainLessonPlay    = "https://edu.51cto.com/center/course/lesson/index?id=%s"
	urlTrainLiveView      = "https://edu.51cto.com/center/wejob/live/view?id=%s"
	urlTrainLivePlay      = "https://edu.51cto.com/center/wejob/play/lived?live_id=%s"
	urlCourseTypeAPI      = "https://edu.51cto.com/center/course/user-june/search-list"
	urlCourseListAPI      = "https://edu.51cto.com/center/course/user-june/list"
	urlLessonListAPI      = "https://edu.51cto.com/center/course/user/get-lesson-list"
	urlLessonFileListAPI  = "https://edu.51cto.com/center/course/index/lesson-file-list"
	urlMaterialListAPI    = "https://edu.51cto.com/center/course/user-june/file-list"
	urlCourseIndexAPI     = "https://edu.51cto.com/center/course/index/index-api"
	urlCourseFileListAPI  = "https://edu.51cto.com/center/course/index/file-list"
	urlVodPlayAuthAPI     = "https://edu.51cto.com/center/player/play/vod-play-auth"
	urlQCloudPlayAPI      = "https://playvideo.qcloud.com/getplayinfo/v4/%s/%s"
	urlTrainingAPI        = "https://apie.51cto.com/api"
	urlTrainStageAPI      = "https://edu.51cto.com/center/wejob/user/train-course-stage-ajax"
	urlTrainCourseAPI     = "https://edu.51cto.com/center/wejob/user/train-course-ajax"
	urlTrainInfoAPI       = "https://edu.51cto.com/center/wejob/user/course-info-ajax"
	urlTrainLiveAPI       = "https://edu.51cto.com/center/wejob/user/train-live-ajax"
	urlTrainFileAPI       = "https://edu.51cto.com/center/wejob/user/train-file-list-ajax"
	urlTrainNextAPI       = "https://edu.51cto.com/center/wejob/center/next-info"
	urlOrderListAPI       = "https://edu.51cto.com/center/orders/order/ajax-get-order-list"
	urlOrderListLegacyAPI = "https://edu.51cto.com/center/orders/order/get-order-list"

	defaultCoursePrice = "999"
)

var patterns = []string{`(?:[\w-]+\.)?51cto\.com/`}

func init() {
	extractor.Register(&Cto51{}, extractor.SiteInfo{Name: "Cto51", URL: "51cto.com", NeedAuth: true})
}

type Cto51 struct{}

func (c *Cto51) Patterns() []string { return patterns }

type route struct{ CID, LID, TrainID, TrainCourseID string }
type media struct {
	URL    string
	Title  string
	Format string
	Size   int64
	Extra  map[string]any
}

var (
	courseRe      = regexp.MustCompile(`(?i)/course/(\d+)\.html|(?:course_id|courseId|cid)=([0-9]+)`)
	lessonRe      = regexp.MustCompile(`(?i)/lesson/(\d+)\.html|(?:lesson_id|lessonId|lid|id)=([0-9]+)`) // line 75 + train lesson line 78
	trainRe       = regexp.MustCompile(`(?i)(?:train_id|trainId)=([0-9]+)|/(?:px/train/(\d+)\.html|training_(\d+)\.html)`)
	trainCourseRe = regexp.MustCompile(`(?i)(?:train_course_id|trainCourseId)=([0-9]+)|(\d+)_\d+`)
	lessonLinkRe  = regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']*/lesson/(\d+)\.html[^"']*)["'][^>]*>(.*?)</a>`)
	titleRe       = regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>|<title[^>]*>(.*?)</title>`)
	aliParamRe    = regexp.MustCompile(`(?is)var\s+aliplayparam\s*=\s*\{(?P<body>[\s\S]*?)\}\s*;`)
	jsPairRe      = regexp.MustCompile(`(?is)["']?([A-Za-z0-9_]+)["']?\s*:\s*(?:"([^"]*)"|'([^']*)'|([^,\n}]+))`)
	mediaURLRe    = regexp.MustCompile(`(?i)https?:\\?/\\?/[^"'<>\s]+?\.(?:m3u8|mp4|flv|mp3|m4a|aac|pdf|pptx?|docx?|xlsx?|zip|rar|7z|tar|gz|txt|md)(?:\?[^"'<>\s]*)?`)
	fileAnchorRe  = regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	priceVarRe    = regexp.MustCompile(`(?i)\bvar\s+(?:course_price|price)\s*=\s*["']?(\d+(?:\.\d+)?)["']?`)
	rmbPriceRe    = regexp.MustCompile(`￥\s*(\d+(?:\.\d+)?)`)
)

func (x *Cto51) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("51cto requires login cookies")
	}
	c := util.NewClient()
	c.SetCookieJar(opts.Cookies)
	h := headers(rawURL)
	r := parseRoute(rawURL)
	listOnly := opts.ListOnly

	if r.LID != "" {
		if r.TrainCourseID != "" {
			return resolveTrainingLesson(c, r, h, listOnly)
		}
		return resolveLesson(c, r.LID, h, listOnly)
	}
	if r.TrainID != "" {
		return resolveTraining(c, r, h, listOnly)
	}
	if r.CID != "" {
		return resolveCourse(c, r.CID, h, listOnly)
	}
	return resolveMyCourses(c, h)
}

func resolveCourse(c *util.Client, cid string, h map[string]string, listOnly bool) (*extractor.MediaInfo, error) {
	payloads := fetchCoursePayloads(c, cid, h)
	title := firstNonEmpty(courseTitleFromPayloads(payloads), "51cto_"+cid)
	price := priceFromPayloads(payloads)
	lessons := lessonsFromPayloads(payloads, lessonContext{CourseID: cid})
	files := filesFromPayloads(payloads, "material")

	pageBody := ""
	if len(lessons) == 0 || title == "51cto_"+cid {
		page := fmt.Sprintf(urlCourse, cid)
		if body, err := c.GetString(page, h); err == nil {
			pageBody = body
			title = firstNonEmpty(extractTitle(body), title)
			if price == "" {
				price = extractPriceFromHTML(body)
			}
			if len(lessons) == 0 {
				lessons = append(lessons, parseLessonLinks(body)...)
			}
			files = append(files, filesFromHTML(body, "", "", "material")...)
		}
	}

	entries := make([]*extractor.MediaInfo, 0, len(lessons)+len(files))
	seen := map[string]bool{}
	for i, item := range dedupeLessons(lessons) {
		if item.ID == "" && item.URL == "" {
			continue
		}
		entry, err := lessonEntry(c, item, h, listOnly, i+1)
		if err != nil {
			continue
		}
		appendEntry(&entries, seen, entry)
	}
	for i, f := range dedupeFiles(files) {
		entry, err := fileEntry(c, f, h, i+1)
		if err != nil {
			continue
		}
		appendEntry(&entries, seen, entry)
	}
	if len(entries) == 0 && pageBody != "" {
		entries = entriesFromPayloads(c, []any{pageBody}, h)
	}
	if len(entries) == 0 {
		entries = entriesFromPayloads(c, payloads, h)
	}
	if len(entries) == 1 && !listOnly {
		return entries[0], nil
	}
	if len(entries) > 0 {
		extra := map[string]any{"course_id": cid, "paid_course_id": cid}
		extra["price"] = normalizePriceOrDefault(price)
		return &extractor.MediaInfo{Site: "cto51", Title: util.SanitizeFilename(title), Entries: entries, Extra: extra}, nil
	}
	return nil, fmt.Errorf("51cto course %s: no playable lesson/file media", cid)
}

func resolveLesson(c *util.Client, lid string, h map[string]string, listOnly bool) (*extractor.MediaInfo, error) {
	if listOnly {
		return lessonListEntry(lessonRef{ID: lid, Title: "lesson_" + lid}, 1), nil
	}
	pageURL := fmt.Sprintf(urlLesson, lid)
	body, err := c.GetString(pageURL, h)
	if err != nil {
		return nil, fmt.Errorf("51cto fetch lesson page: %w", err)
	}
	if m := videoFromText(body); m.URL != "" {
		return mediaInfo(firstNonEmpty(extractTitle(body), "lesson_"+lid), m.URL, m.Format, h), nil
	}
	if auth := parseAliPlayParam(body); len(auth) > 0 {
		auth["lesson_id"] = firstNonEmpty(auth["lesson_id"], lid)
		if apiAuth, err := requestVodPlayAuth(c, auth, h); err == nil && len(apiAuth) > 0 {
			auth = mergeAuth(auth, apiAuth)
		}
		if m, err := resolveAuth(c, auth, h); err == nil && m.URL != "" {
			return mediaInfo(firstNonEmpty(extractTitle(body), "lesson_"+lid), m.URL, m.Format, h), nil
		}
	}
	if auth, err := requestVodPlayAuth(c, map[string]string{"lesson_id": lid, "type": "course"}, h); err == nil && len(auth) > 0 {
		if m, err := resolveAuth(c, auth, h); err == nil && m.URL != "" {
			return mediaInfo(firstNonEmpty(extractTitle(body), "lesson_"+lid), m.URL, m.Format, h), nil
		}
	}
	payloads := fetchJSONPayloads(c, h, []apiReq{{urlVodPlayAuthAPI, map[string]string{"type": "course", "lesson_id": lid, "id": lid}}})
	entries := entriesFromPayloads(c, payloads, h)
	if len(entries) > 0 {
		return entries[0], nil
	}
	return nil, fmt.Errorf("51cto lesson %s: no media URL/playAuth found", lid)
}

func resolveTraining(c *util.Client, r route, h map[string]string, listOnly bool) (*extractor.MediaInfo, error) {
	if r.TrainCourseID != "" && r.LID != "" {
		return resolveTrainingLesson(c, r, h, listOnly)
	}
	payloads := fetchTrainingPayloads(c, r, h)
	title := firstNonEmpty(courseTitleFromPayloads(payloads), "train_"+r.TrainID)
	price := firstNonEmpty(trainingPriceFromPayloads(payloads, r.TrainID), priceFromPayloads(payloads))
	lessons := lessonsFromPayloads(payloads, lessonContext{TrainID: r.TrainID, TrainCourseID: r.TrainCourseID, SourceKind: "training"})
	files := filesFromPayloads(payloads, "material")
	entries := make([]*extractor.MediaInfo, 0, len(lessons)+len(files))
	seen := map[string]bool{}
	for i, item := range dedupeLessons(lessons) {
		entry, err := lessonEntry(c, item, h, listOnly, i+1)
		if err == nil {
			appendEntry(&entries, seen, entry)
		}
	}
	for i, f := range dedupeFiles(files) {
		entry, err := fileEntry(c, f, h, i+1)
		if err == nil {
			appendEntry(&entries, seen, entry)
		}
	}
	if len(entries) == 0 {
		entries = entriesFromPayloads(c, payloads, h)
	}
	if len(entries) == 1 && !listOnly {
		return entries[0], nil
	}
	if len(entries) > 0 {
		extra := map[string]any{"train_id": r.TrainID, "paid_course_id": "train_" + r.TrainID}
		extra["price"] = normalizePriceOrDefault(price)
		return &extractor.MediaInfo{Site: "cto51", Title: util.SanitizeFilename(title), Entries: entries, Extra: extra}, nil
	}
	return nil, fmt.Errorf("51cto train %s: no playable media", r.TrainID)
}

func resolveMyCourses(c *util.Client, h map[string]string) (*extractor.MediaInfo, error) {
	payloads := fetchMyCoursePayloads(c, h)
	courses := courseRefsFromPayloads(payloads)
	entries := courseRefEntries(courses)
	if len(entries) == 0 {
		htmlHeaders := cloneHeaders(h)
		htmlHeaders["Referer"] = firstNonEmpty(h["Referer"], "https://edu.51cto.com/")
		if body, err := c.GetString(urlEStudy, htmlHeaders); err == nil {
			entries = courseRefEntries(courseRefsFromHTML(body))
		}
	}
	if len(entries) == 0 {
		entries = entriesFromPayloads(c, payloads, h)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("51cto: no course/media entries in account APIs")
	}
	return &extractor.MediaInfo{Site: "cto51", Title: "51cto", Entries: entries}, nil
}

func resolveTrainingLesson(c *util.Client, r route, h map[string]string, listOnly bool) (*extractor.MediaInfo, error) {
	ref := lessonRef{ID: r.LID, TrainID: r.TrainID, TrainCourseID: r.TrainCourseID, SourceKind: "training", URL: trainingLessonURL(r.TrainID, r.TrainCourseID, r.LID)}
	return lessonEntry(c, ref, h, listOnly, 1)
}

type apiReq struct {
	URL    string
	Params map[string]string
}

func fetchJSONPayloads(c *util.Client, h map[string]string, reqs []apiReq) []any {
	var out []any
	for _, req := range reqs {
		body, err := c.GetString(addQuery(req.URL, req.Params), h)
		if err != nil {
			continue
		}
		var payload any
		if json.Unmarshal([]byte(body), &payload) == nil {
			out = append(out, payload)
		} else {
			out = append(out, body)
		}
	}
	return out
}

func entriesFromPayloads(c *util.Client, payloads []any, h map[string]string) []*extractor.MediaInfo {
	var entries []*extractor.MediaInfo
	seen := map[string]bool{}
	for _, p := range payloads {
		for _, m := range collectMedia(c, p, h) {
			if m.URL == "" || seen[m.URL] {
				continue
			}
			seen[m.URL] = true
			entries = append(entries, mediaInfoFromMedia(m, h))
		}
	}
	return entries
}

func collectMedia(c *util.Client, v any, h map[string]string) []media {
	var out []media
	switch x := v.(type) {
	case string:
		if m := mediaFromText(x); m.URL != "" {
			out = append(out, m)
		}
		if auth := parseAliPlayParam(x); len(auth) > 0 {
			if m, err := resolveAuth(c, auth, h); err == nil && m.URL != "" {
				out = append(out, m)
			}
		}
	case map[string]any:
		title := textValue(x, "lesson_name", "lessonName", "course_name", "courseName", "title", "name", "file_name", "fileName", "attach_name", "attachName", "video_name", "videoName", "live_name", "liveName")
		if auth := authFromMap(x); len(auth) > 0 {
			if m, err := resolveAuth(c, auth, h); err == nil && m.URL != "" {
				m.Title = title
				m.Size = int64Value(x, "size", "fileSize", "file_size", "video_size", "videoSize")
				out = append(out, m)
			}
		}
		for _, key := range []string{"play_url", "playUrl", "url", "lesson_url", "lessonUrl", "video_url", "videoUrl", "m3u8", "m3u8_url", "m3u8Url", "master_m3u8_url", "masterM3u8Url", "mp4_url", "mp4Url", "file_url", "fileUrl", "download_url", "downloadUrl", "downUrl", "attach_url", "attachUrl", "replay_url", "replayUrl", "playback_url", "playbackUrl", "live_url", "liveUrl", "path", "href", "link"} {
			raw := textValue(x, key)
			if m := mediaFromText(raw); m.URL != "" {
				m.Title = title
				m.Size = int64Value(x, "size", "fileSize", "file_size", "video_size", "videoSize")
				out = append(out, m)
				continue
			}
			if pageURL := playPageURL(normalizeURL(raw, "https://edu.51cto.com/")); pageURL != "" {
				if m, err := resolvePlayPage(c, pageURL, h); err == nil && m.URL != "" {
					m.Title = title
					out = append(out, m)
				}
			}
		}
		if liveID := textValue(x, "live_id", "liveId", "liveID", "liveid"); liveID != "" {
			for _, pageURL := range []string{fmt.Sprintf(urlTrainLiveView, liveID), fmt.Sprintf(urlTrainLivePlay, liveID)} {
				if m, err := resolvePlayPage(c, pageURL, h); err == nil && m.URL != "" {
					m.Title = title
					out = append(out, m)
					break
				}
			}
		}
		if lessonID := textValue(x, "lesson_id", "lessonId", "lessonID", "lid"); lessonID != "" {
			if m, err := resolvePlayPage(c, fmt.Sprintf(urlTrainLessonPlay, lessonID), h); err == nil && m.URL != "" {
				m.Title = title
				out = append(out, m)
			}
			if courseID := textValue(x, "course_id", "courseId", "courseID", "cid"); courseID != "" {
				if m, err := resolvePlayPage(c, fmt.Sprintf(urlTrainLessonPlay, courseID+"_"+lessonID), h); err == nil && m.URL != "" {
					m.Title = title
					out = append(out, m)
				}
			}
		}
		for _, vv := range x {
			out = append(out, collectMedia(c, vv, h)...)
		}
	case []any:
		for _, vv := range x {
			out = append(out, collectMedia(c, vv, h)...)
		}
	}
	return out
}

func resolvePlayPage(c *util.Client, pageURL string, h map[string]string) (media, error) {
	if pageURL == "" {
		return media{}, fmt.Errorf("empty 51cto play page")
	}
	reqHeaders := cloneHeaders(h)
	reqHeaders["Referer"] = firstNonEmpty(h["Referer"], "https://edu.51cto.com/")
	body, err := c.GetString(pageURL, reqHeaders)
	if err != nil {
		return media{}, err
	}
	if m := videoFromText(body); m.URL != "" {
		return m, nil
	}
	if auth := parseAliPlayParam(body); len(auth) > 0 {
		if apiAuth, err := requestVodPlayAuth(c, auth, reqHeaders); err == nil && len(apiAuth) > 0 {
			auth = mergeAuth(auth, apiAuth)
		}
		return resolveAuth(c, auth, reqHeaders)
	}
	var payload any
	if json.Unmarshal([]byte(body), &payload) == nil {
		if m := firstMedia(collectMedia(c, payload, reqHeaders)); m.URL != "" {
			return m, nil
		}
	}
	return media{}, fmt.Errorf("51cto play page has no media")
}

func resolveAuth(c *util.Client, auth map[string]string, h map[string]string) (media, error) {
	if auth["app_id"] != "" && auth["file_id"] != "" && auth["psign"] != "" {
		api := fmt.Sprintf(urlQCloudPlayAPI, url.PathEscape(auth["app_id"]), url.PathEscape(auth["file_id"]))
		body, err := c.GetString(addQuery(api, qcloudPlayParams(auth["psign"])), h)
		if err != nil {
			return media{}, err
		}
		var payload any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			return media{}, err
		}
		if m := firstMedia(collectMedia(c, payload, h)); m.URL != "" {
			return m, nil
		}
	}

	// Aliyun VOD flow: if playAuth decoded to AccessKeyId/Secret/Token, sign
	// a GetPlayInfo request via shared.AliyunResolvePlayInfo (source _request_aliyun_play_info_by_rand).
	if auth["vod_video_id"] != "" && auth["access_key_id"] != "" {
		payload := shared.AliyunPlayPayload{
			AccessKeyID:     auth["access_key_id"],
			AccessKeySecret: auth["access_key_secret"],
			SecurityToken:   auth["sts_token"],
			Region:          firstNonEmpty(auth["region"], "cn-shanghai"),
			AuthInfo:        auth["auth_info"],
			AuthTimeout:     auth["auth_timeout"],
		}
		opts := shared.AliyunPlayOptions{
			Headers:         h,
			Referer:         firstNonEmpty(h["Referer"], "https://edu.51cto.com/"),
			Origin:          firstNonEmpty(h["Origin"], "https://edu.51cto.com"),
			Formats:         auth["formats"],
			AuthTimeout:     auth["auth_timeout"],
			FetchM3U8:       true,
			RewriteM3U8Keys: true,
		}
		opts.ExtraParams = map[string]string{
			"Channel":       "HTML5",
			"Format":        "JSON",
			"PlayConfig":    "{}",
			"PlayerVersion": "2.32.0",
			"ReAuthInfo":    "{}",
			"StreamType":    "video",
		}
		if auth["rand"] != "" {
			opts.ExtraParams["Rand"] = auth["rand"]
		}
		if info, err := shared.AliyunResolvePlayInfo(c, payload, auth["vod_video_id"], opts); err == nil && info.URL != "" {
			mediaURL := info.URL
			format := firstNonEmpty(info.Format, mediaFormat(info.URL))
			if info.M3U8Text != "" {
				mediaURL = "data:application/vnd.apple.mpegurl;base64," + base64.StdEncoding.EncodeToString([]byte(info.M3U8Text))
				format = "m3u8"
			}
			extra := map[string]any{
				"aliyun_api":   info.APIURL,
				"source_type":  firstNonEmpty(info.SourceType, map[bool]string{true: "m3u8_text", false: "video_url"}[info.NeedMerge]),
				"encrypt_type": info.EncryptType,
			}
			if auth["rand"] != "" {
				extra["rand"] = auth["rand"]
			}
			if secData := cto51SecDataFromAuth(auth, info.PlayResponse); len(secData) > 0 {
				extra["m3u8_meta"] = map[string]any{"cryptor": map[string]any{
					"_lel_cryptor_type": "51cto_pes_aes_ecb",
					"key":               hex.EncodeToString(secData),
				}}
			}
			return media{URL: mediaURL, Format: format, Size: info.Size, Extra: extra}, nil
		}
	}

	if auth["play_url"] != "" {
		return mediaFromText(auth["play_url"]), nil
	}
	return media{}, fmt.Errorf("51cto auth has no supported media")
}

func requestVodPlayAuth(c *util.Client, auth map[string]string, h map[string]string) (map[string]string, error) {
	reqHeaders := cloneHeaders(h)
	reqHeaders["Accept"] = "application/json, text/plain, */*"
	reqHeaders["Referer"] = firstNonEmpty(h["Referer"], "https://edu.51cto.com/")

	playID := firstNonEmpty(auth["type"], auth["playid"], "course")
	if playID == "courseindex" {
		playID = "course"
	}
	lessonID := auth["lesson_id"]
	lessonType := auth["lesson_type"]
	if strings.HasPrefix(playID, "wejob") || strings.HasPrefix(lessonType, "wejob") {
		reqHeaders["Referer"] = firstNonEmpty(trainingLessonURL("", auth["train_course_id"], lessonID), reqHeaders["Referer"])
	}
	candidates := []map[string]string{
		{"type": playID, "lesson_id": lessonID, "id": auth["vod_video_id"], "sign": auth["psign"], "lesson_type": lessonType},
		{"type": playID, "lesson_id": lessonID, "id": auth["vod_video_id"], "sign": auth["psign"]},
		{"playid": playID, "lesson_id": lessonID, "id": auth["vod_video_id"], "sign": auth["psign"], "lesson_type": lessonType},
		{"playid": playID, "vod_video_id": auth["vod_video_id"], "vod_video_id_auth": auth["psign"]},
		{"play_id": playID, "vod_video_id": auth["vod_video_id"], "vod_video_id_auth": auth["psign"]},
		{"playid": playID, "vid": auth["vod_video_id"], "auth": auth["psign"]},
		{"playid": playID, "fileid": auth["vod_video_id"], "sign": auth["psign"]},
	}
	var lastErr error
	for _, params := range candidates {
		params = compactParams(params)
		if len(params) == 0 {
			continue
		}
		body, err := c.GetString(addQuery(urlVodPlayAuthAPI, params), reqHeaders)
		if err != nil {
			lastErr = err
			continue
		}
		var payload any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			lastErr = err
			continue
		}
		if out := authFromVodPayload(payload, auth); len(out) > 0 && (out["app_id"] != "" || out["psign"] != "" || out["vod_video_id"] != "") {
			return out, nil
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("51cto vod-play-auth returned no playable auth")
}

func authFromVodPayload(payload any, fallback map[string]string) map[string]string {
	merged := map[string]any{}
	for k, v := range fallback {
		if v != "" {
			merged[k] = v
		}
	}
	for _, m := range walkMaps(payload) {
		for k, v := range m {
			if textValue(merged, k) == "" {
				merged[k] = v
			}
		}
	}
	out := authFromMap(merged)
	if s := deepFindText(payload, "appID", "appId", "app_id", "appid"); s != "" {
		out["app_id"] = s
	}
	if s := deepFindText(payload, "playAuth", "play_auth", "psign", "p_sign", "pSign", "sign"); s != "" {
		out["psign"] = s
	}
	if s := deepFindText(payload, "vodVideoId", "vod_video_id", "fileId", "fileid", "vid", "videoId", "video_id"); s != "" {
		out["vod_video_id"] = s
		out["file_id"] = s
	}
	if out["file_id"] == "" {
		out["file_id"] = out["vod_video_id"]
	}
	if decoded := decodePlayAuth(out["psign"]); len(decoded) > 0 {
		for k, v := range decoded {
			if v != "" {
				out[k] = v
			}
		}
	}
	return out
}

func qcloudPlayParams(psign string) map[string]string {
	params := map[string]string{"keyId": "1", "psign": psign}
	if overlayKey := randomHex(32); overlayKey != "" {
		if ciphered := rsaEncryptOverlay(overlayKey); ciphered != "" {
			params["cipheredOverlayKey"] = ciphered
		} else {
			params["overlayKey"] = overlayKey
		}
	}
	if overlayIV := randomHex(32); overlayIV != "" {
		if ciphered := rsaEncryptOverlay(overlayIV); ciphered != "" {
			params["cipheredOverlayIv"] = ciphered
		} else {
			params["overlayIv"] = overlayIV
		}
	}
	return params
}

func mergeAuth(base, overlay map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		if strings.TrimSpace(v) != "" {
			out[k] = v
		}
	}
	return out
}

func compactParams(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		if strings.TrimSpace(v) != "" {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out
}

func authFromMap(m map[string]any) map[string]string {
	out := map[string]string{
		"app_id":            textValue(m, "app_id", "appId", "appID", "appid", "appIdStr"),
		"file_id":           textValue(m, "file_id", "fileId", "fileID", "fileid", "qcloud_file_id", "qcloudFileId", "fileIdStr", "vid", "video_id", "videoId"),
		"psign":             textValue(m, "psign", "pSign", "p_sign", "playSign", "play_sign", "playAuth", "play_auth", "vod_video_id_auth", "vodVideoIdAuth", "auth", "sign", "token"),
		"play_url":          textValue(m, "play_url", "playUrl", "PlayURL", "url", "m3u8", "m3u8_url"),
		"vod_video_id":      textValue(m, "vod_video_id", "vodVideoId", "vodVideoID", "VideoId", "videoId", "videoID", "video_id", "vid", "media_id", "mediaId", "mediaID", "aliyun_vid", "aliyunVid"),
		"access_key_id":     textValue(m, "AccessKeyId", "AccessKeyID", "accessKeyId", "access_key_id", "access_id", "ky"),
		"access_key_secret": textValue(m, "AccessKeySecret", "accessKeySecret", "access_key_secret", "access_secret", "sc"),
		"sts_token":         textValue(m, "SecurityToken", "securityToken", "sts_token", "stsToken", "tk"),
		"region":            textValue(m, "Region", "region", "regionId", "domain_region"),
		"auth_info":         textValue(m, "AuthInfo", "authInfo", "auth_info"),
		"auth_timeout":      textValue(m, "AuthTimeout", "authTimeout", "auth_timeout"),
		"formats":           textValue(m, "Formats", "formats"),
		"rand":              textValue(m, "Rand", "rand"),
		"seed":              textValue(m, "seed", "Seed"),
		"plaintext":         textValue(m, "Plaintext", "plaintext", "plainText"),
		"sec_data":          textValue(m, "secData", "sec_data", "SecData", "securityData", "SecurityData"),
		"play_domain":       textValue(m, "PlayDomain", "playDomain", "play_domain"),
		"terminal":          textValue(m, "Terminal", "terminal"),
		"customer_id":       textValue(m, "CustomerId", "customerId", "customer_id"),
		"type":              textValue(m, "type", "playid", "play_id"),
		"lesson_id":         textValue(m, "lesson_id", "lessonId", "lessonID", "lid"),
		"lesson_type":       textValue(m, "lesson_type", "lessonType", "lessonTypeStr"),
		"playerid":          textValue(m, "playerid", "playerId"),
	}
	if decoded := decodePlayAuth(out["psign"]); len(decoded) > 0 {
		for k, v := range decoded {
			if out[k] == "" {
				out[k] = v
			}
		}
	}
	return out
}

func parseAliPlayParam(text string) map[string]string {
	m := aliParamRe.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	raw := map[string]any{}
	for _, pair := range jsPairRe.FindAllStringSubmatch(m[1], -1) {
		value := firstNonEmpty(pair[2:]...)
		raw[pair[1]] = value
		raw[strings.ToLower(pair[1])] = value
	}
	return authFromMap(raw)
}

func decodePlayAuth(s string) map[string]string {
	if s == "" || strings.HasPrefix(s, "http") {
		return nil
	}
	var m map[string]any
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		if json.Unmarshal([]byte(trimmed), &m) != nil {
			return nil
		}
		return authFromMap(m)
	}
	padded := trimmed + strings.Repeat("=", (4-len(trimmed)%4)%4)
	b, err := base64.StdEncoding.DecodeString(padded)
	if err != nil {
		b, err = base64.URLEncoding.DecodeString(padded)
	}
	if err != nil || json.Unmarshal(b, &m) != nil {
		return nil
	}
	return authFromMap(m)
}

func mediaFromText(text string) media {
	text = normalizeText(text)
	if m := mediaURLRe.FindStringSubmatch(text); m != nil {
		text = normalizeText(m[0])
	}
	text = normalizeURL(text, "https://edu.51cto.com/")
	lower := strings.ToLower(text)
	if !strings.HasPrefix(lower, "http") {
		return media{}
	}
	format := mediaFormat(text)
	if format == "" {
		return media{}
	}
	return media{URL: text, Format: format}
}

func videoFromText(text string) media {
	m := mediaFromText(text)
	switch m.Format {
	case "m3u8", "mp4", "flv", "mp3", "m4a", "aac":
		return m
	default:
		return media{}
	}
}
