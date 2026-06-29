// Package ledu implements an extractor for ledupeiyou.com courses.
//
// API endpoints from decompiled Mooc/Courses/Ledu/:
//
//	https://passport.vdyoo.com
//	https://app.ledupeiyou.com
//	https://classroom-api.ledupeiyou.com
//	https://classroom-api-online.saasp.vdyoo.com
//	https://course-api-online.saasp.vdyoo.com
//	https://cloudlearn.ledupeiyou.com
package ledu

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

const (
	talHost               = "https://passport.vdyoo.com"
	appHost               = "https://app.ledupeiyou.com"
	apiHost               = "https://classroom-api.ledupeiyou.com"
	onlineAPIHost         = "https://classroom-api-online.saasp.vdyoo.com"
	courseAPIHost         = "https://course-api-online.saasp.vdyoo.com"
	cloudlearnHost        = "https://cloudlearn.ledupeiyou.com"
	h5StudyHost           = "https://app.ledupeiyou.com"
	userInfoPath          = "/backstage/user/tallogin/code"
	h5GetClassListPath    = "/backend-service/m/backend/study/getClassList"
	h5CurriculumListPath  = "/wx-aggregation/cs/backend-service/m/backend/study/getCurriculumList"
	h5LessonDetailPath    = "/wx-aggregation/cs/backend-service/m/backend/study/lessonDetail"
	h5CourseMaterialsPath = "/wx-aggregation/cs/backend-service/m/backend/study/queryCourseMaterials"
	getClassListPath      = "/backstage/xes/study/v1/classroom/getClassList"
	queryLessonsPath      = "/homepage/lessonDetailV0812/queryLessons"
	lessonDetailPath      = "/homepage/lessonDetailV0812/queryLessonDetail"
	courseMaterialsPath   = "/homepage/lessonDetail/queryCourseMaterialListV0303"
	handoutPDFPath        = "/homepage/lessonDetail/share/handout"
	homepageListPath      = "/home-pages/stu/curriculum/index/curriculumListDetails"
	homepageOnePath       = "/home-pages/stu/curriculum/index/curriculumOneDetails"
	homepageHomeworksPath = "/home-pages/stu/aggregation/getHomeworks"
	videoInfoPath         = "/playback/v4/video/init?from=YUNXUEXI"
	legacyRecordPath      = "/classroom-ai/record/v1/resources"
	recordResourcesPath   = "/classroom-ai/record/v3/resources"
	realRecordInitPath    = "/classroom/basic/v1/real-record/init/auth"
	previewSourcePath     = "/newtask/student/task/video/getM3U8videoUrl"
	previewBehaviorPath   = "/service-taskcenter/outside/task/getItemBehaviorData"
	courseSubjectListPath = "/course/v1/student/course/subject-list"
	courseListPath        = "/course/v1/student/course/list"
	courseDetailListPath  = "/course/v1/student/course/user-live-list"
	courseNotePath        = "/course/v1/note/%s"
	coursePaperLinkPath   = "/futureboard/prepare/futureboard/course/paper/download"
	classroomInitAuthPath = "/classroom/basic/v2/init/auth"
	classroomInitStuPath  = "/classroom/basic/v2/init/student"
	browserUA             = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36"
	leduReferer           = "https://app.ledupeiyou.com/"
)

var patterns = []string{`(?:[\w-]+\.)?ledupeiyou\.com/`, `classroom-api(?:-online)?\.(?:ledupeiyou|saasp\.vdyoo)\.com/`}

func init() {
	extractor.Register(&Ledu{}, extractor.SiteInfo{Name: "Ledu", URL: "ledupeiyou.com", NeedAuth: true})
}

type Ledu struct{}

func (s *Ledu) Patterns() []string { return patterns }

var classIDRe = regexp.MustCompile(`(?i)(?:classId|class_id|id)=([A-Za-z0-9_-]+)|/class(?:room)?/([A-Za-z0-9_-]+)`)

func (s *Ledu) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("ledu requires login cookies")
	}
	c := util.NewClient()
	c.SetCookieJar(opts.Cookies)
	cookie := leduCookieString(opts.Cookies)
	studentID := firstText(cookieValue(cookie, "stuId"), cookieValue(cookie, "stuIdStr"), cookieValue(cookie, "LEDU_STUID_PROD"), cookieValue(cookie, "studentId"), cookieValue(cookie, "student_id"), cookieValue(cookie, "user_id"), cookieValue(cookie, "uid"), cookieValue(cookie, "puid"), cookieValue(cookie, "pu_uid"))
	if studentID == "" {
		return nil, fmt.Errorf("ledu requires stuId/user_id cookie")
	}
	headers := leduHeaders(cookie, studentID, "", "", "", "", "")
	_, validateErr := leduGetJSON(c, courseAPIHost, courseSubjectListPath, map[string]string{"stuId": studentID}, headers)

	cid := parseClassID(rawURL)
	classes := fetchClasses(c, headers, studentID)
	if len(classes) == 0 && validateErr != nil {
		return nil, fmt.Errorf("ledu validate pc cookie: %w", validateErr)
	}
	classInfo := chooseClass(classes, cid)
	if classInfo == nil && len(classes) > 0 {
		classInfo = classes[0]
	}
	if classInfo == nil {
		return nil, fmt.Errorf("ledu: no class found for %s", rawURL)
	}
	cid = firstText(classInfo["classId"], classInfo["id"], classInfo["class_id"], cid)
	title := firstText(classInfo["clientCourseName"], classInfo["clientClassName"], classInfo["className"], classInfo["courseName"], classInfo["name"], "ledu_"+cid)
	courseID := firstText(classInfo["pcStdCourseId"], classInfo["stdCourseId"], classInfo["stdCourseIdForDetail"], classInfo["courseId"], cid)
	grade := firstText(classInfo["gradeId"], classInfo["stdGrade"])
	price := leduResolveCoursePrice(classInfo, title)
	purchased := leduPurchased(classInfo)

	details := fetchCourseDetails(c, headers, studentID, courseID)
	if clientPayload := fetchClientCoursePayload(c, headers, classInfo); clientPayload != nil {
		if clientTitle := leduExtractClientCourseName(clientPayload); clientTitle != "" {
			title = clientTitle
			classInfo["clientCourseName"] = clientTitle
		}
		details = mergeLeduDetails(details, detailItemsFromClientPayload(c, headers, classInfo, clientPayload))
	}
	if len(details) == 0 {
		details = detailItemsFromClassInfo(classInfo)
		if len(details) == 0 {
			details = append(details, classInfo)
		}
	}
	entries := buildEntries(c, details, leduHeaders(cookie, studentID, cid, courseID, grade, "", ""))
	if len(entries) == 0 {
		return nil, fmt.Errorf("ledu: no playable video/material entries for classId=%s", cid)
	}
	return &extractor.MediaInfo{Site: "ledu", Title: title, Entries: entries, Extra: map[string]any{"classId": cid, "stdCourseId": courseID, "stuId": studentID, "price": price, "purchased": purchased}}, nil
}

func leduHeaders(cookie, stuID, classID, courseID, grade, liveID, tutorID string) map[string]string {
	h := map[string]string{"Accept": "application/json, text/plain, */*", "User-Agent": browserUA, "Referer": leduReferer, "Origin": strings.TrimRight(leduReferer, "/"), "terminal": "pc", "version": "7.76.91", "branchId": "1111", "stuId": stuID, "stdClassId": classID, "stdCourseId": courseID, "stdGrade": grade, "liveId": liveID, "tutorId": tutorID, "reqTime": strconv.FormatInt(time.Now().UnixMilli(), 10), "lang": "ch", "businessType": "saasp"}
	if cookie != "" {
		h["Cookie"] = cookie
	}
	if tok := firstText(cookieValue(cookie, "token"), cookieValue(cookie, "hb_token"), cookieValue(cookie, "classroom_token")); tok != "" {
		h["token"] = tok
	}
	return h
}

func fetchClasses(c *util.Client, headers map[string]string, stuID string) []map[string]any {
	var out []map[string]any
	seen := map[string]bool{}
	for _, status := range []string{"1", "2", "3"} {
		for page := 1; page <= 50; page++ {
			payload, err := leduGetJSON(c, courseAPIHost, courseListPath, map[string]string{"order": "asc", "perPage": "100", "page": strconv.Itoa(page), "stdSubject": "", "courseStatus": status, "stuId": stuID}, headers)
			if err != nil {
				break
			}
			recs := extractRecords(extractPayload(payload))
			if len(recs) == 0 {
				break
			}
			added := 0
			for _, rec := range recs {
				id := firstText(rec["classId"], rec["id"], rec["class_id"], rec["stdClassId"])
				if id == "" || seen[id] {
					continue
				}
				seen[id] = true
				out = append(out, rec)
				added++
			}
			if added == 0 || len(recs) < 100 {
				break
			}
		}
	}
	if len(out) == 0 {
		out = fetchH5Classes(c, headers)
	}
	return out
}

func fetchCourseDetails(c *util.Client, headers map[string]string, stuID, courseID string) []map[string]any {
	var out []map[string]any
	seen := map[string]bool{}
	for _, typ := range []string{"1", "2", "3", "4"} {
		for page := 1; page <= 50; page++ {
			payload, err := leduGetJSON(c, courseAPIHost, courseDetailListPath, map[string]string{"order": orderForType(typ), "version": "", "perPage": "100", "page": strconv.Itoa(page), "needPage": "1", "type": typ, "stdCourseId": courseID, "stuId": stuID}, headers)
			if err != nil {
				break
			}
			recs := extractRecords(extractPayload(payload))
			if len(recs) == 0 {
				break
			}
			added := 0
			for _, rec := range recs {
				key := firstText(rec["liveId"], rec["taskId"], rec["noteId"], rec["paperId"], rec["coursewareId"], rec["liveName"]) + ":" + typ
				if key == ":" || seen[key] {
					continue
				}
				seen[key] = true
				rec["detailType"] = typ
				out = append(out, rec)
				added++
			}
			if added == 0 || len(recs) < 100 {
				break
			}
		}
	}
	return out
}

func fetchH5Classes(c *util.Client, headers map[string]string) []map[string]any {
	var out []map[string]any
	seen := map[string]bool{}
	for _, status := range []string{"1", "2", "3"} {
		for page := 1; page <= 50; page++ {
			payloads := []map[string]any{
				{"classStatus": status, "pageSize": 100, "page": page},
				{"pageSize": 100, "pageNo": page, "classStatus": status},
				{"pageSize": 100, "pageNum": page, "classStatus": status},
			}
			var recs []map[string]any
			for _, body := range payloads {
				payload, err := leduPostJSON(c, appHost, h5GetClassListPath, body, headers)
				if err != nil {
					continue
				}
				recs = extractRecords(extractPayload(payload))
				if len(recs) > 0 {
					break
				}
			}
			if len(recs) == 0 {
				break
			}
			added := 0
			for _, rec := range recs {
				id := firstText(rec["classId"], rec["id"], rec["class_id"], rec["stdClassId"])
				if id == "" || seen[id] {
					continue
				}
				seen[id] = true
				out = append(out, rec)
				added++
			}
			if added == 0 || len(recs) < 100 {
				break
			}
		}
	}
	if len(out) == 0 {
		for _, body := range []map[string]any{
			{"pageSize": 100, "pageNo": 1},
			{"pageSize": 100, "page": 1},
			{},
		} {
			payload, err := leduPostEncryptedAppJSON(c, getClassListPath, body, headers)
			if err != nil {
				continue
			}
			for _, rec := range extractRecords(extractPayload(payload)) {
				id := firstText(rec["classId"], rec["id"], rec["class_id"], rec["stdClassId"])
				if id == "" || seen[id] {
					continue
				}
				seen[id] = true
				out = append(out, rec)
			}
			if len(out) > 0 {
				break
			}
		}
	}
	return out
}

func detailItemsFromClassInfo(classInfo map[string]any) []map[string]any {
	if classInfo == nil {
		return nil
	}
	var curricula []map[string]any
	for _, key := range []string{"curriculumList", "curriculumInfos", "curriculums", "curriculumDTOList", "stuCurriculumNumberListVos", "stuCurriculumNumberListVOs"} {
		if items := extractRecords(classInfo[key]); len(items) > 0 {
			curricula = items
			break
		}
	}
	if len(curricula) == 0 {
		return nil
	}
	registIDs := uniqueLeduTexts(valuesFromAny(firstNonNil(classInfo["registIdList"], classInfo["registIds"], classInfo["registerIds"], classInfo["registId"])))
	out := make([]map[string]any, 0, len(curricula))
	for i, curriculum := range curricula {
		detail := cloneAnyMap(classInfo)
		delete(detail, "curriculumList")
		delete(detail, "curriculumInfos")
		delete(detail, "curriculums")
		delete(detail, "curriculumDTOList")
		for k, v := range curriculum {
			detail[k] = v
		}
		if detail["classId"] == nil {
			detail["classId"] = firstText(classInfo["classId"], classInfo["id"], classInfo["class_id"], classInfo["stdClassId"])
		}
		if detail["curriculumId"] == nil {
			detail["curriculumId"] = firstText(curriculum["curriculum_id"], curriculum["coursewareCurriculumId"], curriculum["courseId"], curriculum["id"])
		}
		if detail["curriculumNo"] == nil {
			detail["curriculumNo"] = firstText(curriculum["curriculum_no"], curriculum["curriculumNum"], curriculum["number"], strconv.Itoa(i+1))
		}
		if detail["registId"] == nil {
			detail["registId"] = firstText(curriculum["regist_id"], curriculum["registerId"], pickIndex(registIDs, i))
		}
		if detail["liveName"] == nil {
			detail["liveName"] = firstText(curriculum["curriculumName"], curriculum["curriculumDisplayName"], curriculum["cName"], curriculum["name"], curriculum["title"], fmt.Sprintf("lesson_%d", i+1))
		}
		out = append(out, detail)
	}
	return out
}

func fetchClientCoursePayload(c *util.Client, headers map[string]string, classInfo map[string]any) any {
	if classInfo == nil {
		return nil
	}
	classID := firstText(classInfo["classId"], classInfo["id"], classInfo["class_id"], classInfo["stdClassId"])
	studentID := firstText(headers["stuId"], headers["studentId"])
	if classID == "" || studentID == "" {
		return nil
	}
	courseID := firstText(classInfo["stdCourseId"], classInfo["stdCourseIdForDetail"], classInfo["pcStdCourseId"], classInfo["courseId"], classID)
	body := map[string]any{
		"stdCourseId": courseID,
		"classId":     classID,
		"stuIdStr":    studentID,
		"stuId":       leduMaybeInt(studentID),
	}
	payload, err := leduPostJSON(c, apiHost, homepageListPath, body, headers)
	if err != nil {
		return nil
	}
	data := extractPayload(payload)
	if !hasUsefulMap(data) && len(leduClientCurriculumItems(data)) == 0 {
		if m, ok := data.(map[string]any); ok && len(m) == 0 {
			return nil
		}
	}
	return data
}

func detailItemsFromClientPayload(c *util.Client, headers map[string]string, classInfo map[string]any, payload any) []map[string]any {
	items := leduClientCurriculumItems(payload)
	if len(items) == 0 {
		return nil
	}
	registIDs := uniqueLeduTexts(valuesFromAny(firstNonNil(classInfo["registIdList"], classInfo["registIds"], classInfo["registerIds"], classInfo["registId"])))
	out := make([]map[string]any, 0, len(items))
	for i, item := range items {
		detail := cloneAnyMap(classInfo)
		delete(detail, "curriculumList")
		delete(detail, "curriculumInfos")
		delete(detail, "curriculums")
		delete(detail, "curriculumDTOList")
		for k, v := range item {
			detail[k] = v
		}
		if detail["classId"] == nil {
			detail["classId"] = firstText(item["classId"], classInfo["classId"], classInfo["id"], classInfo["class_id"], classInfo["stdClassId"])
		}
		if detail["curriculumId"] == nil {
			detail["curriculumId"] = firstText(item["curriculum_id"], item["coursewareCurriculumId"], item["courseId"], item["id"])
		}
		if detail["curriculumNo"] == nil {
			detail["curriculumNo"] = firstText(item["curriculum_no"], item["curriculumNum"], item["number"], strconv.Itoa(i+1))
		}
		if detail["registId"] == nil {
			detail["registId"] = firstText(item["regist_id"], item["registerId"], pickIndex(registIDs, i))
		}
		if detail["liveName"] == nil {
			detail["liveName"] = firstText(item["mainPrompt"], item["title"], item["curriculumName"], item["curriculumDisplayName"], item["cName"], item["name"], fmt.Sprintf("lesson_%d", i+1))
		}
		detail["clientListItem"] = item
		if one := fetchClientCurriculumOneDetails(c, headers, classInfo, detail); one != nil {
			detail["clientOneDetail"] = one
			if oneMap, ok := extractPayload(one).(map[string]any); ok {
				for k, v := range oneMap {
					if detail[k] == nil {
						detail[k] = v
					}
				}
			}
		}
		if homework := fetchClientHomeworks(c, headers, classInfo, detail); homework != nil {
			detail["clientHomeworks"] = homework
		}
		out = append(out, detail)
	}
	return out
}

func fetchClientCurriculumOneDetails(c *util.Client, headers map[string]string, classInfo, curriculum map[string]any) any {
	body := leduClientCurriculumBody(headers, classInfo, curriculum)
	if body == nil {
		return nil
	}
	payload, err := leduPostJSON(c, apiHost, homepageOnePath, body, headers)
	if err != nil {
		return nil
	}
	return extractPayload(payload)
}

func fetchClientHomeworks(c *util.Client, headers map[string]string, classInfo, curriculum map[string]any) any {
	body := leduClientCurriculumBody(headers, classInfo, curriculum)
	if body == nil {
		return nil
	}
	body["chapterId"] = firstText(curriculum["chapterId"], curriculum["chapter_id"], body["studySelfRoomChapterId"])
	body["type"] = firstNonNil(curriculum["type"], 1)
	payload, err := leduPostJSON(c, apiHost, homepageHomeworksPath, body, headers)
	if err != nil {
		return nil
	}
	return extractPayload(payload)
}

func leduClientCurriculumBody(headers map[string]string, classInfo, curriculum map[string]any) map[string]any {
	if classInfo == nil {
		classInfo = map[string]any{}
	}
	if curriculum == nil {
		curriculum = map[string]any{}
	}
	studentID := firstText(headers["stuId"], headers["studentId"])
	classID := firstText(curriculum["classId"], curriculum["class_id"], classInfo["classId"], classInfo["id"], classInfo["class_id"], classInfo["stdClassId"], headers["stdClassId"])
	curriculumID := firstText(curriculum["curriculumId"], curriculum["curriculum_id"], curriculum["coursewareCurriculumId"], curriculum["courseId"], curriculum["id"])
	curriculumNo := firstText(curriculum["curriculumNo"], curriculum["curriculum_no"], curriculum["curriculumNum"], curriculum["number"])
	registID := firstText(curriculum["registId"], curriculum["regist_id"], curriculum["registerId"], classInfo["registId"])
	if studentID == "" || classID == "" || curriculumID == "" {
		return nil
	}
	studySelfRoomInfo := anyMap(curriculum["studySelfRoomInfo"])
	recommend := anyMap(curriculum["recommendJumpsSceneVO"])
	courseID := firstText(curriculum["stdCourseId"], curriculum["newCourseId"], recommend["courseId"], classInfo["stdCourseId"], classInfo["stdCourseIdForDetail"], classInfo["pcStdCourseId"], classInfo["courseId"], headers["stdCourseId"])
	return map[string]any{
		"stuIdStr":               studentID,
		"stuId":                  leduMaybeInt(studentID),
		"studySelfRoomChapterId": firstText(curriculum["studySelfRoomChapterId"], studySelfRoomInfo["chapterId"]),
		"stdCourseId":            courseID,
		"curriculumNo":           leduMaybeInt(curriculumNo),
		"curriculumId":           curriculumID,
		"classId":                classID,
		"registId":               registID,
	}
}

func leduClientCurriculumItems(payload any) []map[string]any {
	for _, node := range nestedMaps(payload) {
		for _, key := range []string{"stuCurriculumNumberListVos", "stuCurriculumNumberListVOs", "curriculumList", "curriculumInfos"} {
			if items := recordsFromImmediate(node[key]); len(items) > 0 {
				return items
			}
		}
	}
	return nil
}

func leduExtractClientCourseName(payload any) string {
	preferred := firstTextFromMaps(payload, "clientCourseName")
	if preferred != "" {
		return preferred
	}
	for _, node := range nestedMaps(payload) {
		if node["stuCurriculumCourseVo"] != nil {
			if m := anyMap(node["stuCurriculumCourseVo"]); m != nil {
				if name := firstText(m["courseName"], m["className"], m["name"], m["title"]); name != "" {
					return name
				}
			}
		}
	}
	for _, node := range nestedMaps(payload) {
		if name := firstText(node["courseName"], node["className"], node["name"], node["title"]); name != "" {
			return name
		}
	}
	return ""
}

func mergeLeduDetails(base, extra []map[string]any) []map[string]any {
	if len(extra) == 0 {
		return base
	}
	out := append([]map[string]any{}, base...)
	index := map[string]int{}
	for i, item := range out {
		if key := leduDetailKey(item); key != "" {
			index[key] = i
		}
	}
	for _, item := range extra {
		key := leduDetailKey(item)
		if key != "" {
			if i, ok := index[key]; ok {
				for k, v := range item {
					if out[i][k] == nil {
						out[i][k] = v
					}
				}
				continue
			}
			index[key] = len(out)
		}
		out = append(out, item)
	}
	return out
}

func leduDetailKey(m map[string]any) string {
	parts := []string{
		firstText(m["liveId"], m["live_id"], m["chapterId"], m["chapter_id"]),
		firstText(m["curriculumId"], m["curriculum_id"]),
		firstText(m["curriculumNo"], m["curriculum_no"]),
		firstText(m["registId"], m["regist_id"]),
	}
	key := strings.Join(parts, "|")
	if key == "|||" {
		return ""
	}
	return key
}

func recordsFromImmediate(v any) []map[string]any {
	switch x := v.(type) {
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, it := range x {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case []map[string]any:
		return x
	default:
		return nil
	}
}

func leduMaybeInt(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		return n
	}
	return value
}

func buildEntries(c *util.Client, details []map[string]any, headers map[string]string) []*extractor.MediaInfo {
	var entries []*extractor.MediaInfo
	seen := map[string]bool{}
	classID := headers["stdClassId"]
	stuID := headers["stuId"]
	puid := firstText(cookieValue(headers["Cookie"], "puid"), cookieValue(headers["Cookie"], "pu_uid"), cookieValue(headers["Cookie"], "uid"))

	for i, detail := range details {
		roots := []map[string]any{detail}
		liveID := firstText(detail["liveId"], detail["live_id"])
		playbackHeaders := cloneHeaders(headers)

		// 1. classroomInitAuth -- critical precondition for video playback.
		// The Python flow updates the PC classroom context with initData before
		// calling init/student and playback/video/init; keep that context on the
		// per-lesson headers rather than discarding it after parsing roots.
		if liveID != "" {
			playbackHeaders["liveId"] = liveID
			playbackHeaders["tutorId"] = firstText(detail["tutorId"], detail["tutor_id"])
			if authPayload, err := classroomInitAuth(c, playbackHeaders, liveID); err == nil {
				authToken, initData := initAuthTokens(authPayload)
				if authToken != "" {
					playbackHeaders["token"] = authToken
					playbackHeaders["authorization"] = authToken
					playbackHeaders["Authorization"] = "Bearer " + authToken
					playbackHeaders["login-token"] = authToken
				}
				if initData != nil {
					applyLeduInitContext(playbackHeaders, initData)
					roots = append(roots, nestedMaps(initData)...)
				}
				_, _ = leduGetJSON(c, onlineAPIHost, classroomInitStuPath, nil, playbackHeaders)
			}
		}

		// 2. Video init (existing path)
		if liveID != "" {
			if payload, err := leduGetJSON(c, onlineAPIHost, videoInfoPath, nil, playbackHeaders); err == nil {
				roots = append(roots, nestedMaps(extractPayload(payload))...)
			}
		}

		// 3. Handout PDF (existing path)
		if paperID := firstText(detail["paperId"], detail["paper_id"]); paperID != "" {
			if payload, err := leduGetJSON(c, cloudlearnHost, handoutPDFPath, map[string]string{"paperId": paperID}, headers); err == nil {
				roots = append(roots, nestedMaps(extractPayload(payload))...)
			}
		}

		// 4. queryLessons + queryLessonDetail -- structured lesson info with video IDs
		curriculumID := firstText(detail["curriculumId"], detail["curriculum_id"])
		curriculumNo := firstText(detail["curriculumNo"], detail["curriculum_no"])
		registID := firstText(detail["registId"], detail["regist_id"])
		if classID != "" && (curriculumID != "" || liveID != "") {
			lessons := fetchQueryLessons(c, headers, classID, curriculumID, curriculumNo, registID, stuID, puid)
			for _, lesson := range lessons {
				roots = append(roots, lesson)
				// Extract scene objects from each lesson
				if scene, ok := lesson["sceneObject"].(map[string]any); ok {
					roots = append(roots, nestedMaps(scene)...)
				}
			}
			// Also fetch detailed lesson info
			if curriculumID != "" {
				if detailPayload := fetchLessonDetail(c, headers, classID, curriculumID, curriculumNo, registID, stuID, puid); detailPayload != nil {
					roots = append(roots, nestedMaps(detailPayload)...)
				}
			}
		}

		// 5. recordResources -- for recorded video URLs (encUrl/m3u8Url with encKey/encIv)
		resourceID := firstText(detail["resourceId"], detail["resource_id"], detail["cloudLearnVideoResourceId"])
		seenResourceIDs := map[string]bool{}
		if resourceID != "" {
			seenResourceIDs[resourceID] = true
			if recPayload := fetchRecordResources(c, playbackHeaders, resourceID); recPayload != nil {
				roots = append(roots, nestedMaps(recPayload)...)
			}
		}

		// 5a. Lesson-detail scene nodes can contain their own playback or
		// record identifiers. Python normalizes these into video_info before
		// dispatching download_playback/download_record; hydrate them here so
		// later media collection sees actual source URLs.
		seenPlaybackLiveIDs := map[string]bool{}
		if liveID != "" {
			seenPlaybackLiveIDs[liveID] = true
		}
		for _, node := range nestedMaps(roots) {
			nodeLiveID := firstText(node["liveId"], node["live_id"])
			if nodeLiveID != "" && !seenPlaybackLiveIDs[nodeLiveID] {
				seenPlaybackLiveIDs[nodeLiveID] = true
				ctx := cloneHeaders(playbackHeaders)
				applyLeduNodeContext(ctx, node)
				ctx["liveId"] = nodeLiveID
				if authPayload, err := classroomInitAuth(c, ctx, nodeLiveID); err == nil {
					authToken, initData := initAuthTokens(authPayload)
					if authToken != "" {
						ctx["token"] = authToken
						ctx["authorization"] = authToken
						ctx["Authorization"] = "Bearer " + authToken
						ctx["login-token"] = authToken
					}
					if initData != nil {
						applyLeduInitContext(ctx, initData)
						roots = append(roots, nestedMaps(initData)...)
					}
					_, _ = leduGetJSON(c, onlineAPIHost, classroomInitStuPath, nil, ctx)
				}
				if payload, err := leduGetJSON(c, onlineAPIHost, videoInfoPath, nil, ctx); err == nil {
					roots = append(roots, nestedMaps(extractPayload(payload))...)
				}
			}
			for _, rid := range []string{firstText(node["resourceId"], node["resource_id"], node["cloudLearnVideoResourceId"], node["realRecordId"], node["real_record_id"])} {
				if rid == "" || seenResourceIDs[rid] {
					continue
				}
				seenResourceIDs[rid] = true
				if recPayload := fetchRecordResources(c, playbackHeaders, rid); recPayload != nil {
					roots = append(roots, nestedMaps(recPayload)...)
				}
			}
		}

		// 5b. real-record and preview tasks. The restored Python source probes
		// real-record init first, then falls back to record resources and preview
		// task video APIs when task/courseware identifiers are present.
		for _, node := range nestedMaps(roots) {
			if recPayload := fetchRealRecordInit(c, playbackHeaders, node); recPayload != nil {
				roots = append(roots, nestedMaps(recPayload)...)
				if rrid := firstTextFromMaps(recPayload, "realRecordId", "real_record_id", "cloudLearnVideoResourceId", "resourceId"); rrid != "" {
					if rr := fetchRecordResources(c, playbackHeaders, rrid); rr != nil {
						roots = append(roots, nestedMaps(rr)...)
					}
				}
			}
			if preview := fetchPreviewMedia(c, playbackHeaders, node); preview != nil {
				roots = append(roots, nestedMaps(preview)...)
			}
		}

		// 6. courseMaterials -- downloadable files (PDFs, docs, etc.)
		if classID != "" && (curriculumID != "" || liveID != "") {
			materials := fetchCourseMaterials(c, playbackHeaders, classID, curriculumID, curriculumNo, registID, stuID, puid)
			for _, mat := range materials {
				murl := resolveLeduMaterialURL(c, playbackHeaders, mat)
				if murl == "" || seen[murl] {
					continue
				}
				murl = normalizeLeduURL(murl)
				if murl == "" {
					continue
				}
				seen[murl] = true
				name := firstText(mat["itemName"], mat["name"], mat["title"], mat["fileName"], fmt.Sprintf("material_%03d", len(entries)+1))
				format := mediaFormat(murl, mat)
				stream := extractor.Stream{Quality: "best", URLs: []string{murl}, Format: format, Headers: leduMediaHeaders(playbackHeaders)}
				extra := map[string]any{"type": "material"}
				if pid := firstText(mat["paperId"], mat["paper_id"]); pid != "" {
					extra["paperId"] = pid
				}
				if noteID := firstText(mat["noteId"], mat["note_id"]); noteID != "" {
					extra["noteId"] = noteID
				}
				entries = append(entries, &extractor.MediaInfo{Site: "ledu", Title: fmt.Sprintf("(%d.%d)--%s", i+1, len(entries)+1, name), Streams: map[string]extractor.Stream{"best": stream}, Extra: extra})
			}
		}

		// Collect video entries from all roots
		for _, node := range nestedMaps(roots) {
			for _, murl := range mediaURLs(node) {
				if murl == "" || seen[murl] {
					continue
				}
				seen[murl] = true
				name := firstText(node["video_name"], node["videoTitle"], node["video_title"], node["liveName"], node["taskName"], node["itemName"], node["title"], node["name"], fmt.Sprintf("item_%03d", len(entries)+1))
				format := mediaFormat(murl, node)
				stream := extractor.Stream{Quality: "best", URLs: []string{murl}, Format: format, Headers: leduMediaHeaders(playbackHeaders)}
				if format == "m3u8" {
					stream.NeedMerge = true
				}
				extra := map[string]any{"source": firstText(node["liveId"], node["taskId"], node["paperId"], node["noteId"])}
				// Propagate encryption info if present
				if encKey := firstText(node["encKey"]); encKey != "" {
					extra["encKey"] = encKey
				}
				if encIv := firstText(node["encIv"]); encIv != "" {
					extra["encIv"] = encIv
				}
				if format == "m3u8" {
					if dataURL, text, ok := prepareLeduM3U8(c, murl, node, playbackHeaders); ok {
						stream.URLs = []string{dataURL}
						extra["m3u8_text"] = text
						extra["source_type"] = "m3u8_text"
					}
				}
				entries = append(entries, &extractor.MediaInfo{Site: "ledu", Title: fmt.Sprintf("[%d.%d]--%s", i+1, len(entries)+1, name), Streams: map[string]extractor.Stream{"best": stream}, Extra: extra})
			}
		}
	}
	return entries
}

func leduGetJSON(c *util.Client, host, path string, params map[string]string, headers map[string]string) (any, error) {
	u, err := url.Parse(host + path)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	reqHeaders := decorateLeduHeaders(host, path, "GET", params, nil, headers)
	resp, err := c.Get(u.String(), reqHeaders)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	applyLeduResponseHeaders(headers, reqHeaders, resp.Header)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ledu GET %s: HTTP %d", u.String(), resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	payload, err := leduParseJSON(body)
	if err != nil {
		return nil, fmt.Errorf("ledu parse %s: %w", u.String(), err)
	}
	return payload, nil
}

// leduPostJSON sends a JSON POST request to host+path with the given body map.
func leduPostJSON(c *util.Client, host, path string, body map[string]any, headers map[string]string) (any, error) {
	u := host + path
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	h := decorateLeduHeaders(host, path, "POST", nil, body, headers)
	h["Content-Type"] = "application/json; charset=UTF-8"
	resp, err := c.Post(u, bytes.NewReader(raw), h)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	applyLeduResponseHeaders(headers, h, resp.Header)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ledu POST %s: HTTP %d", u, resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	payload, err := leduParseJSON(respBody)
	if err != nil {
		return nil, fmt.Errorf("ledu parse POST %s: %w", u, err)
	}
	return payload, nil
}

func leduPostEncryptedAppJSON(c *util.Client, path string, body map[string]any, headers map[string]string) (any, error) {
	u := appHost + path
	plain, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	enc, err := leduAppAESEncrypt(string(plain))
	if err != nil {
		return nil, err
	}
	wire, err := json.Marshal(map[string]any{"encContent": enc})
	if err != nil {
		return nil, err
	}
	h := decorateLeduHeaders(appHost, path, "POST", nil, body, headers)
	h["Content-Type"] = "application/json; charset=UTF-8"
	resp, err := c.Post(u, bytes.NewReader(wire), h)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	applyLeduResponseHeaders(headers, h, resp.Header)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ledu encrypted POST %s: HTTP %d", u, resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	payload, err := leduParseJSON(respBody)
	if err != nil {
		return nil, fmt.Errorf("ledu parse encrypted POST %s: %w", u, err)
	}
	return payload, nil
}

// ---------- structured API calls ----------

// classroomInitAuth is the critical precondition for video playback. It sends
// GET onlineAPIHost/classroom/basic/v2/init/auth?classroomMode=playback&resVer=1.1
// and returns the initData payload containing auth tokens, course/live context.
func classroomInitAuth(c *util.Client, headers map[string]string, liveID string) (any, error) {
	ctx := cloneHeaders(headers)
	if liveID != "" {
		ctx["liveId"] = liveID
	}
	return leduGetJSON(c, onlineAPIHost, classroomInitAuthPath, map[string]string{
		"classroomMode": "playback",
		"resVer":        "1.1",
	}, ctx)
}

// initAuthTokens extracts the token from classroomInitAuth response and returns
// updated headers with it. Also returns the initData map for context extraction.
func initAuthTokens(payload any) (token string, initData map[string]any) {
	m, ok := payload.(map[string]any)
	if !ok {
		return "", nil
	}
	// The response may have {"data": {"initData": {...}}} or {"initData": {...}}
	root := m
	if d, ok := m["data"].(map[string]any); ok {
		root = d
	}
	initData, _ = root["initData"].(map[string]any)
	if initData == nil {
		initData = root
	}
	// Extract token from response headers field or initData
	token = firstText(m["token"], root["token"], initData["token"], initData["authToken"], initData["loginToken"], firstTextFromMaps(initData, "token", "authToken", "loginToken", "authorization"))
	return token, initData
}

func applyLeduInitContext(headers map[string]string, initData map[string]any) {
	if initData == nil {
		return
	}
	course := anyMap(initData["course"])
	live := anyMap(initData["live"])
	classInfo := anyMap(initData["classInfo"])
	teacher := anyMap(initData["teacher"])
	task := anyMap(initData["task"])
	set := func(k string, vals ...any) {
		if v := firstText(vals...); v != "" {
			headers[k] = v
		}
	}
	set("stdSubject", live["stdSubject"], course["stdSubject"])
	set("stdGrade", course["stdGrade"], live["stdGrade"])
	set("stdCourseId", course["stdCourseId"], task["stdCourseId"])
	set("stdClassId", classInfo["stdClassId"], classInfo["classId"])
	set("branchId", course["branchId"], live["areaId"])
	set("liveId", live["liveId"], task["liveId"])
	set("liveType", live["liveTypeString"], live["liveType"], task["liveTypeString"])
	set("lecturerId", teacher["lecturerId"])
	set("tutorId", teacher["tutorId"])
}

// fetchCourseMaterials calls POST cloudlearnHost/homepage/lessonDetail/queryCourseMaterialListV0303.
// Returns material items with itemUrl/fileUrl + itemName + paperId.
func fetchCourseMaterials(c *util.Client, headers map[string]string, classID, curriculumID, curriculumNo, registID, studentID, studentUID string) []map[string]any {
	body := map[string]any{
		"classId":      classID,
		"curriculumId": curriculumID,
		"curriculumNo": curriculumNo,
		"registId":     registID,
		"studentId":    studentID,
		"studentUid":   studentUID,
	}
	h5Params := map[string]string{"classId": classID, "curriculumId": curriculumID, "registId": registID}
	if curriculumNo != "" {
		h5Params["curriculumNo"] = curriculumNo
	}
	if payload, err := leduGetJSON(c, appHost, h5CourseMaterialsPath, h5Params, headers); err == nil {
		if items := extractMaterialItems(extractPayload(payload)); len(items) > 0 {
			return items
		}
	}
	payload, err := leduPostJSON(c, cloudlearnHost, courseMaterialsPath, body, headers)
	if err != nil {
		return nil
	}
	return extractMaterialItems(extractPayload(payload))
}

// extractMaterialItems walks the response tree to find material entries that have
// a downloadable URL (itemUrl/fileUrl/url) and a name (itemName/paperId).
func extractMaterialItems(v any) []map[string]any {
	var out []map[string]any
	seen := map[string]bool{}
	for _, node := range nestedMaps(v) {
		murl := firstText(node["itemUrl"], node["fileUrl"], node["url"], node["downloadUrl"], node["resourceUrl"], node["attachmentUrl"], node["highLightItemUrl"])
		pid := firstText(node["paperId"], node["paper_id"])
		noteID := firstText(node["noteId"], node["note_id"])
		if murl == "" && pid == "" && noteID == "" {
			continue
		}
		if murl != "" && !(strings.HasPrefix(murl, "http") || strings.HasPrefix(murl, "//")) && pid == "" && noteID == "" {
			continue
		}
		name := firstText(node["itemName"], node["name"], node["title"], node["fileName"])
		if name == "" && pid == "" && noteID == "" {
			continue
		}
		key := firstText(murl, pid, noteID) + "|" + name
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, node)
	}
	return out
}

// fetchQueryLessons calls POST cloudlearnHost/homepage/lessonDetailV0812/queryLessons.
// Returns structured lesson list with video IDs and scene objects.
func fetchQueryLessons(c *util.Client, headers map[string]string, classID, curriculumID, curriculumNo, registID, studentID, studentUID string) []map[string]any {
	body := map[string]any{
		"classId":      classID,
		"curriculumId": curriculumID,
		"curriculumNo": curriculumNo,
		"registId":     registID,
		"registType":   "1",
		"lessonType":   "",
		"studentId":    studentID,
		"studentUid":   studentUID,
	}
	for _, h5Body := range []map[string]any{
		{"classId": classID, "curriculumId": curriculumID, "curriculumNo": curriculumNo, "registId": registID, "stuId": studentID, "studentId": studentID, "studentUid": studentUID},
		{"classId": classID, "curriculumId": curriculumID, "registId": registID},
	} {
		payload, err := leduPostJSON(c, appHost, h5CurriculumListPath, h5Body, headers)
		if err != nil {
			continue
		}
		if lessons := extractLessonList(extractPayload(payload)); len(lessons) > 0 {
			return lessons
		}
	}
	payload, err := leduPostJSON(c, cloudlearnHost, queryLessonsPath, body, headers)
	if err != nil {
		return nil
	}
	return extractLessonList(extractPayload(payload))
}

// fetchLessonDetail calls POST cloudlearnHost/homepage/lessonDetailV0812/queryLessonDetail.
// Returns detailed lesson info with scene objects containing video resources.
func fetchLessonDetail(c *util.Client, headers map[string]string, classID, curriculumID, curriculumNo, registID, studentID, studentUID string) any {
	body := map[string]any{
		"classId":       classID,
		"registClassId": classID,
		"curriculumId":  curriculumID,
		"curriculumNo":  curriculumNo,
		"registId":      registID,
		"studentId":     studentID,
		"studentUid":    studentUID,
	}
	for _, h5Body := range []map[string]any{
		{"classId": classID, "registClassId": classID, "curriculumId": curriculumID, "curriculumNo": curriculumNo, "registId": registID, "stuId": studentID, "studentId": studentID, "studentUid": studentUID},
		{"classId": classID, "curriculumId": curriculumID, "registId": registID},
	} {
		payload, err := leduPostJSON(c, appHost, h5LessonDetailPath, h5Body, headers)
		if err != nil {
			continue
		}
		data := extractPayload(payload)
		if hasUsefulMap(data) {
			return data
		}
	}
	payload, err := leduPostJSON(c, cloudlearnHost, lessonDetailPath, body, headers)
	if err != nil {
		return nil
	}
	return extractPayload(payload)
}

// extractLessonList pulls lesson dicts from the queryLessons response.
func extractLessonList(v any) []map[string]any {
	if arr, ok := v.([]any); ok {
		out := make([]map[string]any, 0, len(arr))
		for _, it := range arr {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	if m, ok := v.(map[string]any); ok {
		for _, k := range []string{"lessonList", "lessons", "list", "curriculumList"} {
			if r := extractLessonList(m[k]); len(r) > 0 {
				return r
			}
		}
		// Single lesson object with sceneObject
		if m["sceneObject"] != nil || m["chapterId"] != nil || m["liveType"] != nil {
			return []map[string]any{m}
		}
	}
	return nil
}

// fetchRecordResources calls GET onlineAPIHost/classroom-ai/record/v3/resources
// to get recorded video URLs (encUrl/m3u8Url with encKey/encIv).
func fetchRecordResources(c *util.Client, headers map[string]string, resourceID string) any {
	params := map[string]string{}
	if resourceID != "" {
		params["cloudLearnVideoResourceId"] = resourceID
	}
	payload, err := leduGetJSON(c, onlineAPIHost, recordResourcesPath, params, headers)
	if err == nil {
		data := extractPayload(payload)
		if hasMediaCandidates(data) {
			return data
		}
		if u := firstTextFromMaps(data, "resourcesUrl", "resourceUrl"); u != "" && strings.HasPrefix(u, "http") {
			if remote, e := leduGetJSONURL(c, u, headers); e == nil && hasMediaCandidates(remote) {
				return extractPayload(remote)
			}
		}
	}
	if legacy, err := leduGetJSON(c, onlineAPIHost, legacyRecordPath, params, headers); err == nil {
		return extractPayload(legacy)
	}
	return nil
}

func fetchRealRecordInit(c *util.Client, headers map[string]string, task map[string]any) any {
	taskID := firstText(task["taskId"], task["task_id"])
	coursewareID := firstText(task["coursewareId"], task["courseware_id"])
	curriculumID := firstText(task["curriculumId"], task["curriculum_id"])
	taskType := firstText(task["taskTypeString"], task["task_type_string"])
	if taskID == "" && coursewareID == "" {
		return nil
	}
	payload, err := leduGetJSON(c, onlineAPIHost, realRecordInitPath, map[string]string{
		"versionUpgradeMark": "",
		"coursewareId":       coursewareID,
		"taskTypeString":     taskType,
		"taskId":             taskID,
		"curriculumId":       curriculumID,
	}, headers)
	if err != nil {
		return nil
	}
	return extractPayload(payload)
}

func fetchPreviewMedia(c *util.Client, headers map[string]string, task map[string]any) any {
	taskID := firstText(task["taskId"], task["task_id"], task["id"])
	contentID := firstText(task["contentDetailId"], task["content_detail_id"], task["contentId"], task["content_id"])
	if taskID == "" {
		return nil
	}
	body := map[string]any{"taskId": taskID, "task_id": taskID, "studentId": headers["stuId"], "studentUid": firstText(cookieValue(headers["Cookie"], "puid"), cookieValue(headers["Cookie"], "pu_uid"))}
	if payload, err := leduPostJSON(c, cloudlearnHost, previewBehaviorPath, body, headers); err == nil && hasMediaCandidates(extractPayload(payload)) {
		return extractPayload(payload)
	}
	if contentID == "" {
		return nil
	}
	body["contentDetailId"] = contentID
	body["content_detail_id"] = contentID
	if payload, err := leduPostJSON(c, cloudlearnHost, previewSourcePath, body, headers); err == nil {
		return extractPayload(payload)
	}
	return nil
}

func hasMediaCandidates(v any) bool {
	for _, node := range nestedMaps(v) {
		if len(mediaURLs(node)) > 0 {
			return true
		}
	}
	return false
}

func hasUsefulMap(v any) bool {
	for _, node := range nestedMaps(v) {
		if len(node) == 0 {
			continue
		}
		for _, key := range []string{"sceneObject", "sceneList", "chapterId", "liveId", "taskId", "paperId", "materialList", "lessonList"} {
			if node[key] != nil {
				return true
			}
		}
	}
	return false
}

func chooseClass(classes []map[string]any, cid string) map[string]any {
	if cid == "" {
		return nil
	}
	for _, c := range classes {
		if firstText(c["classId"], c["id"], c["class_id"], c["stdClassId"]) == cid {
			return c
		}
	}
	return nil
}

func parseClassID(raw string) string {
	if m := classIDRe.FindStringSubmatch(raw); len(m) > 0 {
		return firstText(m[1], m[2])
	}
	return ""
}

func orderForType(t string) string {
	if t == "2" || t == "4" {
		return "desc"
	}
	return "asc"
}

func mediaURL(m map[string]any) string {
	if urls := mediaURLs(m); len(urls) > 0 {
		return urls[0]
	}
	return ""
}

func mediaURLs(m map[string]any) []string {
	if m == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(u string) {
		u = normalizeLeduURL(u)
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		out = append(out, u)
	}
	keys := []string{"m3u8Url", "videoM3u8Url", "m3u8", "m3u8_url", "mp4", "mp4Url", "trVideoUrl", "videoUrl", "videoUrlList", "encUrl", "encUrls", "fileUrl", "itemUrl", "downloadUrl", "resourceUrl", "attachmentUrl", "pdfUrl", "src", "url"}
	for _, k := range keys {
		collectMediaURLs(m[k], k, m, add)
	}
	for k, v := range m {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "line") || strings.Contains(lk, "definition") || strings.Contains(lk, "quality") {
			collectMediaURLs(v, k, m, add)
		}
	}
	return out
}

func collectMediaURLs(v any, key string, ctx map[string]any, add func(string)) {
	switch x := v.(type) {
	case string:
		u := normalizeLeduURL(x)
		if u != "" && (looksMedia(u) || isMaterial(ctx) || mediaKeyAllowsURL(key)) {
			add(u)
		}
	case []any:
		for _, it := range x {
			collectMediaURLs(it, key, ctx, add)
		}
	case []string:
		for _, it := range x {
			collectMediaURLs(it, key, ctx, add)
		}
	case map[string]any:
		for _, k := range []string{"m3u8Url", "videoM3u8Url", "m3u8", "m3u8_url", "mp4", "mp4Url", "trVideoUrl", "videoUrl", "videoUrlList", "encUrl", "encUrls", "url", "fileUrl", "itemUrl", "downloadUrl", "resourceUrl", "attachmentUrl", "pdfUrl", "src"} {
			collectMediaURLs(x[k], k, x, add)
		}
		for k, it := range x {
			lk := strings.ToLower(k)
			if strings.HasPrefix(lk, "line") || strings.Contains(lk, "definition") || strings.Contains(lk, "quality") {
				collectMediaURLs(it, k, x, add)
			}
		}
	}
}

func firstMediaURL(v any, key string, ctx map[string]any) string {
	switch x := v.(type) {
	case string:
		u := normalizeLeduURL(x)
		if u == "" {
			return ""
		}
		if looksMedia(u) || isMaterial(ctx) || mediaKeyAllowsURL(key) {
			return u
		}
	case []any:
		for _, it := range x {
			if u := firstMediaURL(it, key, ctx); u != "" {
				return u
			}
		}
	case []string:
		for _, it := range x {
			if u := firstMediaURL(it, key, ctx); u != "" {
				return u
			}
		}
	case map[string]any:
		for _, k := range []string{"m3u8Url", "videoM3u8Url", "m3u8", "mp4Url", "trVideoUrl", "videoUrl", "encUrl", "url", "fileUrl", "downloadUrl"} {
			if u := firstMediaURL(x[k], k, x); u != "" {
				return u
			}
		}
	}
	return ""
}

func mediaKeyAllowsURL(key string) bool {
	key = strings.ToLower(key)
	for _, part := range []string{"m3u8", "mp4", "videourl", "trvideourl", "encurl", "fileurl", "itemurl", "downloadurl", "resourceurl", "attachmenturl", "pdfurl"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

func looksMedia(s string) bool {
	ls := strings.ToLower(s)
	return strings.Contains(ls, ".m3u8") || strings.Contains(ls, ".mp4") || strings.Contains(ls, ".pdf") || strings.Contains(ls, ".ppt") || strings.Contains(ls, ".doc") || strings.Contains(ls, ".xls") || strings.Contains(ls, ".zip") || strings.Contains(ls, ".rar") || strings.Contains(ls, ".7z")
}

func isMaterial(m map[string]any) bool {
	return firstText(m["paperId"], m["paper_id"], m["noteId"], m["itemName"], m["fileName"]) != ""
}

func mediaFormat(s string, m map[string]any) string {
	ls := strings.ToLower(strings.SplitN(strings.SplitN(s, "?", 2)[0], "#", 2)[0])
	for _, ext := range []string{"m3u8", "mp4", "pdf", "pptx", "ppt", "docx", "doc", "xlsx", "xls", "zip", "rar", "7z", "txt"} {
		if strings.HasSuffix(ls, "."+ext) {
			return ext
		}
	}
	for _, key := range []string{"m3u8Url", "videoM3u8Url", "m3u8", "m3u8_url", "encUrl", "encUrls"} {
		if firstMediaURL(m[key], key, m) == s || firstText(m[key]) == s {
			return "m3u8"
		}
	}
	for _, key := range []string{"mp4", "mp4Url", "trVideoUrl", "videoUrl", "videoUrlList"} {
		if firstMediaURL(m[key], key, m) == s || firstText(m[key]) == s {
			return "mp4"
		}
	}
	if ft := strings.TrimPrefix(strings.ToLower(firstText(m["fileType"], m["type"], m["contentType"])), "."); ft != "" {
		return ft
	}
	return "bin"
}

func extractPayload(v any) any {
	for {
		m, ok := v.(map[string]any)
		if !ok {
			return v
		}
		advanced := false
		for _, k := range []string{"data", "result", "content", "payload"} {
			if x, ok := m[k]; ok && x != nil {
				v, advanced = x, true
				break
			}
		}
		if !advanced {
			return m
		}
	}
}

func extractRecords(v any) []map[string]any {
	switch x := v.(type) {
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, it := range x {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case map[string]any:
		for _, k := range []string{"classInfo", "classInfos", "classList", "list", "rows", "records", "lessonList", "lessons", "curriculumList", "items"} {
			if r := extractRecords(x[k]); len(r) > 0 {
				return r
			}
		}
	}
	return nil
}

func nestedMaps(v any) []map[string]any {
	var out []map[string]any
	var walk func(any)
	walk = func(x any) {
		switch y := x.(type) {
		case []map[string]any:
			for _, m := range y {
				walk(m)
			}
		case []any:
			for _, it := range y {
				walk(it)
			}
		case map[string]any:
			out = append(out, y)
			for _, it := range y {
				walk(it)
			}
		}
	}
	walk(v)
	return out
}

func cloneHeaders(h map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range h {
		out[k] = v
	}
	return out
}

func leduCookieString(jar http.CookieJar) string {
	if jar == nil {
		return ""
	}
	seen, parts := map[string]bool{}, []string{}
	for _, raw := range []string{appHost, apiHost, onlineAPIHost, courseAPIHost, cloudlearnHost, talHost, "https://stu.ledupeiyou.com"} {
		u, _ := url.Parse(raw)
		for _, ck := range jar.Cookies(u) {
			if !seen[ck.Name] {
				seen[ck.Name] = true
				parts = append(parts, ck.Name+"="+ck.Value)
			}
		}
	}
	return strings.Join(parts, "; ")
}

func cookieValue(cookie, name string) string {
	for _, p := range strings.Split(cookie, ";") {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) == 2 && strings.EqualFold(kv[0], name) {
			return kv[1]
		}
	}
	return ""
}

func firstText(vals ...any) string {
	for _, v := range vals {
		if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}
