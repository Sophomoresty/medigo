// Package houda implements an extractor for houdask.com courses that play via CSSLCloud.
//
// API endpoints from decompiled Mooc/Courses/Houda/Houda_Course.pyc:
//
//	http://www.houdask.com/api/center/online/myOnline/anon/getLearnFirstPage
//	http://www.houdask.com/api/center/online/myOnline/getXxStageAndLawList
//	http://www.houdask.com/api/center/myOnlineCourse/getLearnCourse
//	http://www.houdask.com/api/center/live/cc/anon/viewPlayback/{room_id}/{record_id}
//	https://view.csslcloud.net/replay/user/login
//	https://view.csslcloud.net/replay/video/play
//	https://view.csslcloud.net/replay/data/meta
package houda

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/nichuanfang/medigo/internal/extractor"
	"github.com/nichuanfang/medigo/internal/extractor/shared"
	"github.com/nichuanfang/medigo/internal/util"
)

const (
	urlOrigin          = "http://www.houdask.com"
	urlHome            = "http://www.houdask.com/"
	urlLoginCheck      = "http://www.houdask.com/api/center/sysUserPower/anon/ifLogin"
	urlCourseList      = "http://www.houdask.com/api/center/online/myOnline/anon/getLearnFirstPage"
	urlStageLaw        = "http://www.houdask.com/api/center/online/myOnline/getXxStageAndLawList"
	urlLearnCourse     = "http://www.houdask.com/api/center/myOnlineCourse/getLearnCourse"
	urlLiveDetail      = "http://www.houdask.com/api/center/myOnlineLive/anon/v202401/getById"
	urlMaterial        = "http://www.houdask.com/api/center/myLibraryMaterial/v2/getList"
	urlCCViewPlayback  = "http://www.houdask.com/api/center/live/cc/anon/viewPlayback/%s/%s"
	urlCsslLogin       = "https://view.csslcloud.net/replay/user/login"
	urlCsslPlay        = "https://view.csslcloud.net/replay/video/play"
	urlCsslMeta        = "https://view.csslcloud.net/replay/data/meta"
	urlCsslOrigin      = "https://view.csslcloud.net"
	csslDeviceType     = "h5-pc"
	csslDeviceVersion  = "3.21.0"
	csslTpl            = "20"
	csslTerminal       = "3"
	materialServiceTyp = "1"
)

var patterns = []string{
	`(?:[\w-]+\.)?houdask\.com/`,
	`(?:[\w-]+\.)?csslcloud\.net/`,
}

func init() {
	extractor.Register(&Houda{}, extractor.SiteInfo{Name: "Houda", URL: "houdask.com", NeedAuth: true})
}

type Houda struct{}

func (s *Houda) Patterns() []string { return patterns }

var classIDRe = regexp.MustCompile(`(?:classId|class_id)=([0-9]+)`)

func (s *Houda) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("houda requires login cookies")
	}
	c := util.NewClient()
	c.SetCookieJar(opts.Cookies)
	headers := houdaHeaders()

	if isCsslcloudURL(rawURL) {
		return s.extractCsslcloudURL(c, rawURL, headers)
	}
	if err := checkHoudaCookie(c, headers); err != nil {
		return nil, err
	}
	cid := parseClassID(rawURL)
	if cid == "" {
		return nil, fmt.Errorf("cannot parse houda classId from URL: %s", rawURL)
	}
	lessons, err := fetchHoudaLessons(c, cid, headers)
	if err != nil {
		return nil, err
	}
	if len(lessons) == 0 {
		return nil, fmt.Errorf("houda: no liveList lessons for classId=%s", cid)
	}

	entries := make([]*extractor.MediaInfo, 0, len(lessons))
	for i, lesson := range lessons {
		entry, err := buildHoudaEntry(c, cid, i+1, lesson, headers)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("houda: lessons found but no playable CSSLCloud streams for classId=%s", cid)
	}
	return &extractor.MediaInfo{Site: "houda", Title: "houda_" + cid, Entries: entries}, nil
}

func houdaHeaders() map[string]string {
	return map[string]string{
		"appType":          "WEB",
		"X-Requested-With": "XMLHttpRequest",
		"Accept":           "application/json, text/plain, */*",
		"Origin":           urlOrigin,
		"Referer":          urlHome,
	}
}

func checkHoudaCookie(c *util.Client, headers map[string]string) error {
	body, err := c.GetString(urlLoginCheck, headers)
	if err != nil {
		return fmt.Errorf("houda cookie check: %w", err)
	}
	var out struct {
		Code any `json:"code"`
		Data any `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		return fmt.Errorf("houda cookie check parse: %w", err)
	}
	if stringValue(out.Code) != "1" {
		return fmt.Errorf("houda requires valid logged-in cookie (code=%s)", stringValue(out.Code))
	}
	return nil
}

func fetchHoudaLessons(c *util.Client, cid string, headers map[string]string) ([]houdaLesson, error) {
	payload := map[string]string{"classId": cid}
	wrapped, _ := json.Marshal(map[string]string{"classId": cid})
	forms := []map[string]string{{"data": string(wrapped)}, payload}
	var lastErr error
	for _, form := range forms {
		body, err := c.PostForm(urlLearnCourse, form, headers)
		if err != nil {
			lastErr = err
			continue
		}
		var out struct {
			Code any `json:"code"`
			Data struct {
				LiveList []houdaLesson `json:"liveList"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			lastErr = err
			continue
		}
		if len(out.Data.LiveList) > 0 {
			return out.Data.LiveList, nil
		}
		lastErr = fmt.Errorf("empty liveList (code=%s)", stringValue(out.Code))
	}
	return nil, fmt.Errorf("houda fetch lessons: %w", lastErr)
}

type houdaLesson struct {
	ID          any `json:"id"`
	Title       any `json:"title"`
	Name        any `json:"name"`
	CourseName  any `json:"courseName"`
	CourseID    any `json:"courseId"`
	ClassID     any `json:"classId"`
	Type        any `json:"type"`
	CCLiveID    any `json:"ccLiveId"`
	RoomID      any `json:"roomId"`
	MainRoomID  any `json:"mainRoomId"`
	RecordID    any `json:"recordId"`
	LiveURL     any `json:"liveUrl"`
	PlaybackURL any `json:"playbackUrl"`
	PlaybackMP4 any `json:"playbackMp4"`
	PlaybackMP3 any `json:"playbackMp3"`
	StageID     any `json:"stageId"`
	StageName   any `json:"stageName"`
	LawID       any `json:"lawId"`
	LawName     any `json:"lawName"`
}

func buildHoudaEntry(c *util.Client, cid string, index int, lesson houdaLesson, headers map[string]string) (*extractor.MediaInfo, error) {
	title := firstText(lesson.Title, lesson.Name, lesson.CourseName, "未命名")
	lessonID := firstText(lesson.ID)
	roomID := firstText(lesson.RoomID, lesson.MainRoomID, lesson.CCLiveID)
	recordID := firstText(lesson.RecordID)
	direct := firstText(lesson.PlaybackMP4, lesson.PlaybackURL, lesson.LiveURL)

	streams := map[string]extractor.Stream{}
	extra := map[string]any{"course_id": cid, "lesson_id": lessonID, "record_id": recordID, "room_id": roomID}
	if roomID != "" && recordID != "" {
		info, err := resolveHoudaCSSL(c, roomID, recordID, title, headers)
		if err == nil {
			extra["csslcloud_session_id"] = info.SessionID
			for _, v := range info.VideoList {
				if v.URL == "" {
					continue
				}
				key := fmt.Sprintf("definition_%d", v.Definition)
				streams[key] = extractor.Stream{Quality: key, URLs: []string{v.URL}, Format: mediaExt(v.URL), AudioURL: info.AudioURL, Headers: map[string]string{"Referer": urlHome}}
			}
			if info.VideoURL != "" && mediaExt(info.VideoURL) == "m3u8" {
				if rewritten, err := rewriteHoudaM3U8(c, info.VideoURL, urlHome); err == nil {
					extra["m3u8_text"] = rewritten
				}
			}
		} else if direct == "" {
			return nil, err
		} else {
			extra["csslcloud_error"] = err.Error()
		}
	}
	if len(streams) == 0 && direct != "" {
		fmtName := mediaExt(direct)
		streams[fmtName] = extractor.Stream{Quality: "best", URLs: []string{direct}, Format: fmtName, Headers: map[string]string{"Referer": urlHome}}
		if fmtName == "m3u8" {
			if rewritten, err := rewriteHoudaM3U8(c, direct, urlHome); err == nil {
				extra["m3u8_text"] = rewritten
			}
		}
	}
	if len(streams) == 0 {
		return nil, fmt.Errorf("houda lesson %s has no stream", lessonID)
	}
	return &extractor.MediaInfo{Site: "houda", Title: fmt.Sprintf("[%d]--%s", index, title), Streams: streams, Extra: extra}, nil
}

func resolveHoudaCSSL(c *util.Client, roomID, recordID, title string, headers map[string]string) (*shared.CssLcloudPlayInfo, error) {
	cc, err := resolveHoudaCCCallback(c, roomID, recordID, headers)
	if err != nil {
		return nil, err
	}
	liveRoomID := firstNonEmpty(cc.RoomID, roomID)
	accessID := firstNonEmpty(cc.UserID, cc.AccountID)
	viewerToken := firstNonEmpty(cc.ViewerToken, accessID+":"+liveRoomID)
	return shared.CssLcloudResolvePlayInfo(c, shared.CssLcloudPayload{
		LiveRoomID:  liveRoomID,
		UserID:      accessID,
		AccessID:    accessID,
		RecordID:    firstNonEmpty(cc.RecordID, recordID),
		ViewerName:  firstNonEmpty(cc.ViewerName, title),
		ViewerToken: viewerToken,
		Referer:     urlHome,
	})
}

type houdaCCInfo struct {
	UserID      string
	AccountID   string
	RoomID      string
	RecordID    string
	ViewerName  string
	ViewerToken string
}

func resolveHoudaCCCallback(c *util.Client, roomID, recordID string, headers map[string]string) (houdaCCInfo, error) {
	callbackURL := fmt.Sprintf(urlCCViewPlayback, url.PathEscape(roomID), url.PathEscape(recordID))
	finalURL, err := fetchRedirectLocation(c, callbackURL, headers)
	if err != nil {
		return houdaCCInfo{}, err
	}
	u, err := url.Parse(finalURL)
	if err != nil {
		return houdaCCInfo{}, fmt.Errorf("houda parse CSSL callback URL: %w", err)
	}
	q := u.Query()
	info := houdaCCInfo{
		UserID:      firstNonEmpty(q.Get("userId"), q.Get("userid"), q.Get("uid")),
		AccountID:   firstNonEmpty(q.Get("accountId"), q.Get("accessid"), q.Get("accessId")),
		RoomID:      firstNonEmpty(q.Get("roomId"), q.Get("roomid"), q.Get("room_id"), roomID),
		RecordID:    firstNonEmpty(q.Get("recordId"), q.Get("recordid"), q.Get("record_id"), recordID),
		ViewerName:  firstNonEmpty(q.Get("viewername"), q.Get("viewerName"), q.Get("userName")),
		ViewerToken: firstNonEmpty(q.Get("viewertoken"), q.Get("viewerToken"), q.Get("userToken")),
	}
	if info.AccountID == "" {
		info.AccountID = info.UserID
	}
	if info.UserID == "" || info.RoomID == "" || info.RecordID == "" {
		return houdaCCInfo{}, fmt.Errorf("houda CSSL callback missing userId/roomId/recordId: %s", finalURL)
	}
	return info, nil
}

func fetchRedirectLocation(c *util.Client, raw string, headers map[string]string) (string, error) {
	resp, err := c.Get(raw, headers)
	if err != nil {
		return "", fmt.Errorf("houda CSSL callback: %w", err)
	}
	defer resp.Body.Close()
	if resp.Request != nil && resp.Request.URL != nil {
		return resp.Request.URL.String(), nil
	}
	return raw, nil
}

func rewriteHoudaM3U8(c *util.Client, m3u8URL, referer string) (string, error) {
	body, err := c.GetString(m3u8URL, map[string]string{"Referer": referer})
	if err != nil {
		return "", err
	}
	return shared.CssLcloudRewriteM3U8Keys(c, body, referer)
}

func (s *Houda) extractCsslcloudURL(c *util.Client, rawURL string, headers map[string]string) (*extractor.MediaInfo, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	roomID := firstNonEmpty(q.Get("roomId"), q.Get("roomid"), q.Get("liveRoomId"), q.Get("room_id"))
	recordID := firstNonEmpty(q.Get("recordId"), q.Get("recordid"), q.Get("record_id"))
	accessID := firstNonEmpty(q.Get("userId"), q.Get("userid"), q.Get("accountId"), q.Get("accessid"))
	viewerToken := firstNonEmpty(q.Get("viewertoken"), q.Get("viewerToken"), accessID+":"+roomID)
	info, err := shared.CssLcloudResolvePlayInfo(c, shared.CssLcloudPayload{LiveRoomID: roomID, UserID: accessID, AccessID: accessID, RecordID: recordID, ViewerName: "houda", ViewerToken: viewerToken, Referer: urlHome})
	if err != nil {
		return nil, err
	}
	stream := extractor.Stream{Quality: "best", URLs: []string{info.VideoURL}, Format: mediaExt(info.VideoURL), AudioURL: info.AudioURL, Headers: map[string]string{"Referer": urlHome}}
	return &extractor.MediaInfo{Site: "houda", Title: "houda_csslcloud", Streams: map[string]extractor.Stream{"best": stream}}, nil
}

func parseClassID(rawURL string) string {
	if m := classIDRe.FindStringSubmatch(rawURL); len(m) > 1 {
		return m[1]
	}
	return ""
}

func isCsslcloudURL(rawURL string) bool { return strings.Contains(rawURL, "csslcloud.net") }

func mediaExt(u string) string {
	lu := strings.ToLower(u)
	switch {
	case strings.Contains(lu, ".m3u8"):
		return "m3u8"
	case strings.Contains(lu, ".mp3"):
		return "mp3"
	default:
		return "mp4"
	}
}

func firstText(values ...any) string {
	for _, v := range values {
		if s := stringValue(v); s != "" {
			return s
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func stringValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}
