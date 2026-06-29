// Package speiyou implements an extractor for speiyou.com (学而思培优 / S-培优).
package speiyou

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

const (
	USER_AGENT       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	referer          = "https://speiyou.cn/"
	subject_api      = "https://course-api-online.speiyou.com/course/v1/student/course/subject-list?stuId=%s"
	course_list_api  = "https://course-api-online.speiyou.com/course/v1/student/course/list?businessBelong=1,3,5,10&courseStatus=0&stdSubject=%s&page=%d&perPage=20&order=asc&stuId=%s"
	chapter_list_api = "https://course-api-online.speiyou.com/course/v1/student/course/user-live-list?stdCourseId=%s&type=1&needPage=1&page=%d&perPage=50&order=asc&stuId=%s"
	live_list_api    = "https://course-api-online.speiyou.com/course/v1/student/live/list?businessBelong=1,3,5,10&stuId=%s&liveStatus=%s&nowTime=%d&stdSubject=%s&order=%s&needCourseInfo=1&needPage=1&page=%d&perPage=%d"
	auth_api         = "https://classroom-api-online.speiyou.com/classroom/basic/v2/init/auth?resVer=1.1&classroomMode=playback"
	video_api        = "https://classroom-api-online.speiyou.com/playback/v1/video/init"
)

var patterns = []string{`(?:[\w-]+\.)?speiyou\.com/`}

func init() {
	extractor.Register(&Speiyou{}, extractor.SiteInfo{Name: "Speiyou", URL: "speiyou.com", NeedAuth: true})
}

type Speiyou struct{}

func (s *Speiyou) Patterns() []string { return patterns }

func (s *Speiyou) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("speiyou requires login cookies")
	}
	auth := authFromJar(opts.Cookies)
	parsedURL, _ := url.Parse(rawURL)
	query := url.Values{}
	if parsedURL != nil {
		query = parsedURL.Query()
	}
	if auth.StuID == "" {
		auth.StuID = first(query.Get("stuId"), query.Get("pu_uid"), match1(rawURL, `[?&]stuId=(\d+)`), match1(rawURL, `[?&]pu_uid=(\d+)`))
	}
	if auth.StuID == "" {
		return nil, fmt.Errorf("speiyou requires stuId in login cookies or URL")
	}
	courseID := first(query.Get("stdCourseId"), query.Get("courseId"), query.Get("cid"), match1(rawURL, `[?&](?:stdCourseId|courseId|cid)=(\d+)`), match1(rawURL, `/course/(\d+)`))
	stdSubject := first(query.Get("stdSubject"), match1(rawURL, `[?&]stdSubject=([^&#]+)`))
	c := util.NewClient()
	c.SetCookieJar(opts.Cookies)
	h := baseHeaders(auth)
	subjects, err := validateSpeiyouLogin(c, h, auth.StuID)
	if err != nil {
		return nil, err
	}
	courses, lessons := fetchCourseAndLessons(c, h, auth.StuID, courseID, speiyouSubjectCandidates(stdSubject, subjects))
	var price any
	if courseID == "" && len(courses) > 0 {
		courseID = courses[0].ID
		stdSubject = first(stdSubject, courses[0].Subject)
		lessons = courses[0].Lessons
		price = courses[0].Price
	}
	if courseID == "" {
		return nil, fmt.Errorf("cannot parse speiyou stdCourseId/courseId from URL")
	}
	if len(lessons) == 0 {
		lessons = fetchLegacyLessons(c, h, auth.StuID, courseID)
	}
	if len(lessons) == 0 {
		lessons = filterLessons(fetchLiveList(c, h, auth.StuID, stdSubject, "1", "desc", 50), courseID)
	}
	title := "speiyou_" + courseID
	for _, cr := range courses {
		if cr.ID == courseID && cr.Title != "" {
			title = stripLessonCount(cr.Title)
			stdSubject = first(stdSubject, cr.Subject)
			if price == nil {
				price = cr.Price
			}
			break
		}
	}
	var entries []*extractor.MediaInfo
	for i, lesson := range lessons {
		info := normalizeLesson(lesson, i+1, courseID, stdSubject)
		if info.LiveID == "" {
			continue
		}
		playURL, entryPrice := resolveVideo(c, h, auth, info)
		if playURL == "" {
			continue
		}
		if price == nil {
			price = entryPrice
		}
		format := pickFormat(playURL)
		extra := map[string]any{"live_id": info.LiveID, "std_course_id": info.StdCourseID, "std_subject": info.StdSubject}
		if entryPrice != nil {
			extra["price"] = entryPrice
		}
		entries = append(entries, &extractor.MediaInfo{Site: "speiyou", Title: info.Title, Streams: map[string]extractor.Stream{"best": {Quality: "best", URLs: []string{playURL}, Format: format, NeedMerge: format == "m3u8", Headers: map[string]string{"Referer": referer, "Origin": "owcr://classroom", "User-Agent": USER_AGENT}}}, Extra: extra})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("speiyou: no playback videoUrls returned from classroom API")
	}
	extra := map[string]any{"std_course_id": courseID, "stu_id": auth.StuID, "purchased": true}
	if stdSubject != "" {
		extra["std_subject"] = stdSubject
	}
	if price != nil {
		extra["price"] = price
	}
	return &extractor.MediaInfo{Site: "speiyou", Title: sanitize(title), Entries: entries, Extra: extra}, nil
}

type authInfo struct{ Token, StuID, Cookie string }
type courseRef struct {
	ID, Title, Subject string
	Lessons            []map[string]any
	Price              any
	LastTime           int64
}
type lessonInfo struct {
	Raw                                                                                                           map[string]any
	LiveID, Title, StdCourseID, StdSubject, StdGrade, StdClassID, BranchID, AreaID, LecturerID, TutorID, LiveType string
	Price                                                                                                         any
}

func validateSpeiyouLogin(c *util.Client, h map[string]string, stuID string) ([]map[string]any, error) {
	resp, err := requestJSON(c, fmt.Sprintf(subject_api, url.QueryEscape(stuID)), h)
	if err != nil {
		return nil, fmt.Errorf("speiyou subject-list login check: %w", err)
	}
	if !validSubjectResponse(resp) {
		return nil, fmt.Errorf("speiyou login check failed: subject-list response has no list payload")
	}
	return jsonToMaps(resp), nil
}

func speiyouSubjectCandidates(seed string, subjects []map[string]any) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	add("")
	add(seed)
	for _, subject := range subjects {
		add(first(textAt(subject, "stdSubject"), textAt(subject, "subject"), textAt(subject, "code")))
	}
	return out
}

func fetchCourseAndLessons(c *util.Client, h map[string]string, stuID, courseID string, subjects []string) ([]courseRef, []map[string]any) {
	if len(subjects) == 0 {
		subjects = []string{""}
	}
	allLive := []map[string]any{}
	grouped := map[string]*courseRef{}
	for _, subject := range subjects {
		live := fetchLiveList(c, h, stuID, subject, "1", "desc", 50)
		allLive = append(allLive, live...)
		mergeCourseGroups(grouped, groupLiveCourses(live))
		if len(live) > 0 && subject == "" {
			break
		}
	}

	for _, subject := range subjects {
		found := false
		for page := 1; page <= 200; page++ {
			resp, err := requestJSON(c, fmt.Sprintf(course_list_api, url.QueryEscape(subject), page, url.QueryEscape(stuID)), h)
			if err != nil {
				break
			}
			items := jsonToMaps(resp)
			if len(items) > 0 {
				found = true
			}
			for _, it := range items {
				id := courseKey(it)
				if id == "" {
					continue
				}
				cr := grouped[id]
				if cr == nil {
					cr = &courseRef{ID: id, Title: sanitize(first(textAt(it, "courseName", "name", "title"), "未命名课程")), Subject: first(textAt(it, "stdSubject"), subject), Price: extractPrice(it)}
					grouped[id] = cr
				} else {
					cr.Title = first(cr.Title, sanitize(first(textAt(it, "courseName", "name", "title"), "未命名课程")))
					cr.Subject = first(cr.Subject, textAt(it, "stdSubject"), subject)
					if cr.Price == nil {
						cr.Price = extractPrice(it)
					}
				}
			}
			if len(items) < 20 {
				break
			}
		}
		if found && subject == "" {
			break
		}
	}
	var out []courseRef
	for _, cr := range grouped {
		sort.SliceStable(cr.Lessons, func(i, j int) bool {
			return intAt(cr.Lessons[i], "liveStarttime") < intAt(cr.Lessons[j], "liveStarttime")
		})
		cr.LastTime = maxCourseTime(cr.Lessons, cr.LastTime)
		if len(cr.Lessons) > 0 {
			cr.Title = formatCourseTitle(cr.Title, len(cr.Lessons))
		}
		out = append(out, *cr)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].LastTime != out[j].LastTime {
			return out[i].LastTime > out[j].LastTime
		}
		return out[i].Title < out[j].Title
	})
	if courseID == "" {
		return out, nil
	}
	if cr := grouped[courseID]; cr != nil && len(cr.Lessons) > 0 {
		return out, cr.Lessons
	}
	return out, filterLessons(allLive, courseID)
}

func groupLiveCourses(live []map[string]any) map[string]*courseRef {
	grouped := map[string]*courseRef{}
	for _, lesson := range live {
		id := courseKey(lesson)
		if id == "" {
			continue
		}
		cr := grouped[id]
		if cr == nil {
			cr = &courseRef{
				ID:       id,
				Title:    sanitize(first(textAt(lesson, "courseName", "name", "title"), "未命名课程")),
				Subject:  first(textAt(lesson, "stdSubject"), textAt(lesson, "subjectName")),
				Price:    extractPrice(lesson),
				LastTime: intAt(lesson, "liveStarttime"),
			}
			grouped[id] = cr
		}
		if cr.Price == nil {
			cr.Price = extractPrice(lesson)
		}
		cr.LastTime = maxInt64(cr.LastTime, intAt(lesson, "liveStarttime"))
		cr.Lessons = append(cr.Lessons, lesson)
	}
	return grouped
}

func mergeCourseGroups(dst, src map[string]*courseRef) {
	for id, incoming := range src {
		cur := dst[id]
		if cur == nil {
			dst[id] = incoming
			continue
		}
		cur.Title = first(stripLessonCount(cur.Title), stripLessonCount(incoming.Title), "未命名课程")
		cur.Subject = first(cur.Subject, incoming.Subject)
		if cur.Price == nil {
			cur.Price = incoming.Price
		}
		cur.LastTime = maxInt64(cur.LastTime, incoming.LastTime)
		seenLessons := map[string]bool{}
		for _, lesson := range cur.Lessons {
			if key := lessonKey(lesson); key != "" {
				seenLessons[key] = true
			}
		}
		for _, lesson := range incoming.Lessons {
			key := lessonKey(lesson)
			if key == "" || seenLessons[key] {
				continue
			}
			seenLessons[key] = true
			cur.Lessons = append(cur.Lessons, lesson)
		}
	}
}

func fetchLiveList(c *util.Client, h map[string]string, stuID, subject, status, order string, perPage int) []map[string]any {
	var out []map[string]any
	seen := map[string]bool{}
	for page := 1; page <= 200; page++ {
		api := fmt.Sprintf(live_list_api, url.QueryEscape(stuID), url.QueryEscape(status), time.Now().UnixMilli(), url.QueryEscape(subject), url.QueryEscape(order), page, perPage)
		resp, err := requestJSON(c, api, h)
		if err != nil {
			break
		}
		items := jsonToMaps(resp)
		for _, it := range items {
			k := courseKey(it) + ":" + lessonKey(it)
			if lessonKey(it) != "" && !seen[k] {
				seen[k] = true
				out = append(out, it)
			}
		}
		if len(items) < perPage {
			break
		}
	}
	return out
}
func fetchLegacyLessons(c *util.Client, h map[string]string, stuID, courseID string) []map[string]any {
	var out []map[string]any
	for page := 1; page <= 200; page++ {
		resp, err := requestJSON(c, fmt.Sprintf(chapter_list_api, url.QueryEscape(courseID), page, url.QueryEscape(stuID)), h)
		if err != nil {
			break
		}
		items := jsonToMaps(resp)
		out = append(out, items...)
		if len(items) < 50 {
			break
		}
	}
	return out
}
func filterLessons(list []map[string]any, courseID string) []map[string]any {
	var out []map[string]any
	for _, it := range list {
		if courseKey(it) == courseID || courseID == "" {
			out = append(out, it)
		}
	}
	return out
}
func normalizeLesson(m map[string]any, index int, courseID, subject string) lessonInfo {
	live, course, classInfo, teacher := unwrapMap(m["liveInfo"]), unwrapMap(m["courseInfo"]), unwrapMap(m["classInfo"]), unwrapMap(m["teacher"])
	title := first(textAt(m, "liveName", "title", "name"), textAt(live, "liveName", "title", "name"), "未命名课时")
	return lessonInfo{
		Raw:         m,
		LiveID:      lessonKey(m),
		Title:       sanitize(fmt.Sprintf("[%d]--%s", index, title)),
		StdCourseID: first(textAt(m, "stdCourseId", "course_id", "courseId"), textAt(course, "stdCourseId", "course_id", "courseId"), courseID),
		StdSubject:  first(textAt(m, "stdSubject"), textAt(course, "stdSubject"), textAt(live, "stdSubject"), subject),
		StdGrade:    first(textAt(m, "stdGrade"), textAt(course, "stdGrade")),
		StdClassID:  first(textAt(m, "stdClassId", "classId"), textAt(classInfo, "stdClassId", "classId")),
		BranchID:    first(textAt(m, "branchId"), textAt(course, "branchId"), textAt(m, "areaId"), textAt(live, "areaId")),
		AreaID:      first(textAt(m, "areaId"), textAt(live, "areaId")),
		LecturerID:  first(textAt(m, "lecturerId"), textAt(teacher, "lecturerId")),
		TutorID:     first(textAt(m, "tutorId"), textAt(teacher, "tutorId")),
		LiveType:    first(textAt(m, "liveTypeString", "liveType"), textAt(live, "liveTypeString", "liveType"), "SMALL_GROUPS_V2_MODE"),
		Price:       extractPrice(m),
	}
}
func resolveVideo(c *util.Client, base map[string]string, auth authInfo, info lessonInfo) (string, any) {
	h := playbackHeaders(base, auth, info)
	if resp, err := requestJSON(c, auth_api, h); err == nil {
		mergeAuthInfo(&info, unwrapMap(resp))
		h = playbackHeaders(base, auth, info)
	}
	resp, err := requestJSON(c, video_api, h)
	if err != nil {
		return "", info.Price
	}
	m := unwrapMap(resp)
	urls := valueStrings(m["videoUrls"])
	if len(urls) == 0 {
		urls = valueStrings(m["videoUrl"])
	}
	if len(urls) == 0 {
		data := unwrapMap(m["data"])
		urls = valueStrings(data["videoUrls"])
		if len(urls) == 0 {
			urls = valueStrings(data["videoUrl"])
		}
	}
	for _, u := range urls {
		if strings.HasPrefix(strings.TrimSpace(u), "http") {
			return strings.TrimSpace(u), info.Price
		}
	}
	return findURL(m), info.Price
}
func mergeAuthInfo(info *lessonInfo, m map[string]any) {
	init := unwrapMap(m["initData"])
	if len(init) == 0 {
		init = unwrapMap(unwrapMap(m["data"])["initData"])
	}
	live, course, classInfo, teacher := unwrapMap(init["live"]), unwrapMap(init["course"]), unwrapMap(init["classInfo"]), unwrapMap(init["teacher"])
	info.StdSubject = first(info.StdSubject, textAt(live, "stdSubject"), textAt(course, "stdSubject"))
	info.StdGrade = first(info.StdGrade, textAt(course, "stdGrade"))
	info.BranchID = first(info.BranchID, textAt(course, "branchId"), textAt(live, "areaId"))
	info.AreaID = first(info.AreaID, textAt(live, "areaId"), info.BranchID)
	info.StdClassID = first(info.StdClassID, textAt(classInfo, "stdClassId", "classId"))
	info.LiveType = first(info.LiveType, textAt(live, "liveTypeString"), "SMALL_GROUPS_V2_MODE")
	info.LecturerID = first(info.LecturerID, textAt(teacher, "lecturerId"))
	info.TutorID = first(info.TutorID, textAt(teacher, "tutorId"))
	if info.Price == nil {
		info.Price = extractPrice(init)
	}
	for _, item := range listValues(teacher["teacherList"]) {
		tm := unwrapMap(item)
		teacherID := first(textAt(tm, "teacherId", "id"))
		if teacherID == "" {
			continue
		}
		role := strings.ToLower(first(textAt(tm, "teacherType", "role")))
		switch {
		case info.LecturerID == "" && (role == "" || role == "1" || role == "lecturer" || role == "主讲"):
			info.LecturerID = teacherID
		case info.TutorID == "":
			info.TutorID = teacherID
		}
	}
}
func playbackHeaders(base map[string]string, auth authInfo, i lessonInfo) map[string]string {
	h := clone(base)
	for k, v := range map[string]string{"liveType": i.LiveType, "tutorId": i.TutorID, "lecturerId": i.LecturerID, "stdClassId": i.StdClassID, "branchId": first(i.AreaID, i.BranchID), "stdGrade": i.StdGrade, "stdSubject": i.StdSubject, "stdCourseId": i.StdCourseID, "liveId": i.LiveID, "stuId": auth.StuID, "authorization": auth.Token, "token": auth.Token, "Host": "classroom-api-online.speiyou.com"} {
		if v != "" {
			h[k] = v
		}
	}
	h["Origin"] = "owcr://classroom"
	h["Referer"] = referer
	return h
}
func requestJSON(c *util.Client, api string, h map[string]string) (any, error) {
	body, err := c.GetString(api, h)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		return nil, err
	}
	return out, nil
}
