package cctalk

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

const (
	cctalkMyGroupListURL = "https://m.cctalk.com/webapi/content/v1.1/user/my_group_list?start=%d&limit=%d&sortType=1&keyword=%s"
	cctalkMyCourseURL    = "https://m.cctalk.com/mycourse"
	cctalkMobileURL      = "https://m.cctalk.com"
)

func (a *apiClient) getCourseDetail(courseID string) map[string]any {
	if strings.TrimSpace(courseID) == "" {
		return nil
	}
	endpoints := [][2]string{
		{fmt.Sprintf("/course/%s/course_detail", courseID), "v1.1"},
		{fmt.Sprintf("/course/%s/course_detail", courseID), "v1.2"},
		{fmt.Sprintf("/course/%s/detail", courseID), "v1.1"},
	}
	for _, ep := range endpoints {
		data := asMap(extractData(a.requestAPI(ep[0], nil, "", ep[1])))
		if len(data) > 0 {
			return data
		}
	}
	data := asMap(extractData(a.requestAPI("/course/detail", map[string]string{"courseId": courseID}, "", "v1.1")))
	if len(data) > 0 {
		return data
	}
	return nil
}

func (a *apiClient) getGroupInfo(groupID string) map[string]any {
	if strings.TrimSpace(groupID) == "" {
		return nil
	}
	for _, path := range []string{
		fmt.Sprintf("/webapi/im/v1.3/group/%s/info?isRichInfo=true", groupID),
		fmt.Sprintf("/webapi/im/v1.1/group/%s/baseinfo", groupID),
	} {
		data := asMap(extractData(a.requestJSON(CCTALK_BASE_URL+path, nil, "")))
		if len(data) > 0 {
			data["groupId"] = groupID
			if _, ok := data["courseId"]; !ok {
				data["courseId"] = groupID
			}
			if gn := textValue(data, "groupName"); gn != "" {
				if _, ok := data["courseName"]; !ok {
					data["courseName"] = gn
				}
			}
			return data
		}
	}
	return nil
}

func (a *apiClient) getSeriesInfo(seriesID string) map[string]any {
	if strings.TrimSpace(seriesID) == "" {
		return nil
	}
	for _, ep := range [][2]string{
		{fmt.Sprintf("/series/%s/get_series_info", seriesID), "v1.1"},
		{fmt.Sprintf("/series/%s/get_series_info", seriesID), "v1.2"},
	} {
		data := asMap(extractData(a.requestAPI(ep[0], nil, "", ep[1])))
		if len(data) > 0 {
			data["seriesId"] = seriesID
			return data
		}
	}
	return nil
}

func (a *apiClient) getGroupSeries(groupID string) []map[string]any {
	if strings.TrimSpace(groupID) == "" {
		return nil
	}
	var result []map[string]any
	seen := map[string]bool{}
	offset := 0
	for i := 0; i < 20; i++ {
		var page []any
		for _, version := range []string{"v1.2", "v1.1"} {
			data := extractData(a.requestAPI(
				fmt.Sprintf("/series/group/%s/series", groupID),
				map[string]string{"limit": "50", "start": fmt.Sprint(offset)},
				"", version,
			))
			if list := extractList(data); len(list) > 0 {
				page = list
				break
			}
		}
		for _, item := range page {
			m := asMap(item)
			if m == nil {
				continue
			}
			m["groupId"] = groupID
			if _, ok := m["courseId"]; !ok {
				m["courseId"] = firstNonEmpty(textValue(m, "seriesId"), textValue(m, "id"))
			}
			if _, ok := m["courseName"]; !ok {
				m["courseName"] = firstNonEmpty(textValue(m, "seriesName"), textValue(m, "name"), textValue(m, "title"))
			}
			m["_source"] = "direct_series"
			key := firstNonEmpty(textValue(m, "seriesId"), textValue(m, "courseId"), textValue(m, "id"))
			if key != "" && !seen[key] {
				seen[key] = true
				result = append(result, m)
			}
		}
		if len(page) == 0 || len(page) < 50 {
			break
		}
		offset += len(page)
	}
	return result
}

func (a *apiClient) getGroupSeriesStructs(groupID string, groupSeries []map[string]any) any {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" || len(groupSeries) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(groupSeries))
	seen := map[string]bool{}
	for idx, series := range groupSeries {
		seriesID := firstNonEmpty(
			textValue(series, "seriesId", "series_id"),
			textValue(series, "courseId", "course_id"),
			textValue(series, "id"),
		)
		if seriesID == "" || seen[seriesID] {
			continue
		}
		seen[seriesID] = true
		structs := a.getSeriesStructs(seriesID)
		if len(walkMaps(structs)) == 0 {
			continue
		}
		unitName := firstNonEmpty(textValue(series, "seriesName", "courseName", "name", "title"), fmt.Sprintf("系列%d", idx+1))
		out = append(out, map[string]any{
			"children":  structs,
			"groupId":   groupID,
			"seriesId":  seriesID,
			"showIndex": idx + 1,
			"unitName":  unitName,
			"unitId":    "series_" + seriesID,
		})
	}
	return out
}

func (a *apiClient) getLessonInfo(lessonID string) map[string]any {
	if strings.TrimSpace(lessonID) == "" {
		return nil
	}
	for _, source := range []string{"0", "1", "2", ""} {
		data := asMap(extractData(a.requestAPI("/course/get_lesson_info",
			map[string]string{"lessonId": lessonID, "source": source, "withCourse": "true"},
			"", "v1.1")))
		if len(data) > 0 {
			return data
		}
	}
	return nil
}

func (a *apiClient) getSubscribeCourses() []map[string]any {
	var result []map[string]any
	seen := map[string]bool{}

	offset := 0
	for i := 0; i < 20; i++ {
		body, err := a.c.GetString(
			fmt.Sprintf(cctalkMyGroupListURL, offset, 20, ""),
			mergeStringMaps(a.headers, map[string]string{"Referer": cctalkMyCourseURL, "Origin": cctalkMobileURL}),
		)
		if err != nil {
			break
		}
		var raw map[string]any
		if json.Unmarshal([]byte(body), &raw) != nil {
			break
		}
		data := asMap(extractData(raw))
		page := extractList(data)
		appendUniqueCourses(&result, seen, page, "my_group_list")

		nextOffset := offset
		hasMore := false
		if data != nil {
			hasMore = truthyAny(data["moreData"]) || truthyAny(data["hasNext"])
			if np := intFromAny(data["nextPage"]); np > offset {
				nextOffset = np
				hasMore = true
			}
		}
		if nextOffset <= offset {
			nextOffset = offset + maxInt(len(page), 20)
		}
		if len(page) == 0 || !hasMore {
			break
		}
		offset = nextOffset
	}

	uid := a.currentUserID()
	if uid == "" {
		return result
	}
	for _, courseType := range []string{"1", "2", "0"} {
		timeline := ""
		for i := 0; i < 20; i++ {
			params := map[string]string{"limit": "50", "timeline": timeline, "courseType": courseType}
			data := asMap(extractData(a.requestAPI(fmt.Sprintf("/user/%s/course_subscribe_list", uid), params, "", "v1.1")))
			page := extractList(data)
			appendUniqueCourses(&result, seen, page, "course_subscribe_list")
			nextTimeline := ""
			if data != nil {
				nextTimeline = firstNonEmpty(textValue(data, "nextTimeline"), textValue(data, "next_timeline"), textValue(data, "timeline"))
			}
			if len(page) == 0 || nextTimeline == "" || nextTimeline == timeline {
				break
			}
			timeline = nextTimeline
		}
	}
	return result
}

func (a *apiClient) currentUserID() string {
	if a == nil || a.c == nil {
		return ""
	}
	if header := firstNonEmpty(a.cookieHeader, cookieHeaderFromJar(a.jar)); header != "" {
		cookies := parseCookieHeader(header)
		for name, value := range cookies {
			switch strings.ToLower(name) {
			case "clubauth":
				if uid := decodeClubAuthUID(value); uid != "" {
					return uid
				}
			case "access_token", "accesstoken":
				if uid := accessTokenUID(value); uid != "" {
					return uid
				}
			case "hjuid", "uid", "userid", "user_id":
				if onlyDigits(value) {
					return value
				}
			}
		}
	}
	for _, host := range []string{CCTALK_BASE_URL + "/", cctalkMobileURL + "/"} {
		if parsed, err := url.Parse(host); err == nil && a.jar != nil {
			for _, cookie := range a.jar.Cookies(parsed) {
				switch strings.ToLower(cookie.Name) {
				case "clubauth":
					if uid := decodeClubAuthUID(cookie.Value); uid != "" {
						return uid
					}
				case "access_token":
					if uid := accessTokenUID(cookie.Value); uid != "" {
						return uid
					}
				}
			}
		}
	}
	return ""
}

func appendUniqueCourses(out *[]map[string]any, seen map[string]bool, page []any, source string) {
	for _, item := range page {
		m := asMap(item)
		if m == nil || shouldHideSubscribeCourse(m) {
			continue
		}
		if _, ok := m["_source"]; !ok {
			m["_source"] = source
		}
		key := courseMapKey(m)
		if key != "" && !seen[key] {
			seen[key] = true
			*out = append(*out, m)
		}
	}
}

func shouldHideSubscribeCourse(item map[string]any) bool {
	if _, ok := item["allVideoCount"]; !ok {
		return false
	}
	return intFromAny(item["allVideoCount"]) <= 0
}

func courseMapKey(item map[string]any) string {
	return firstNonEmpty(
		textValue(item, "courseId", "course_id"),
		textValue(item, "seriesId", "series_id"),
		textValue(item, "groupId", "group_id"),
		textValue(item, "contentId", "content_id"),
		textValue(item, "id"),
		textValue(item, "title", "courseName", "seriesName", "groupName"),
	)
}

func courseListMedia(siteTitle string, courses []map[string]any) *extractor.MediaInfo {
	entries := make([]*extractor.MediaInfo, 0, len(courses))
	seen := map[string]bool{}
	for _, course := range courses {
		link := courseURL(course)
		key := firstNonEmpty(link, courseMapKey(course))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		title := firstNonEmpty(textValue(course, "courseName", "seriesName", "groupName", "title", "name", "contentTitle"), key)
		extra := map[string]any{"url": link, "course": course}
		if price := priceInfo(course); len(price) > 0 {
			extra["price"] = price
		}
		entries = append(entries, &extractor.MediaInfo{
			Site:  "cctalk",
			Title: util.SanitizeFilename(title),
			Extra: extra,
		})
	}
	return &extractor.MediaInfo{Site: "cctalk", Title: util.SanitizeFilename(siteTitle), Entries: entries}
}

func courseURL(course map[string]any) string {
	if u := firstNonEmpty(textValue(course, "url", "courseUrl", "courseURL", "shareUrl", "shareURL")); u != "" {
		return normalizeMediaURL(u)
	}
	groupID := firstNonEmpty(textValue(course, "groupId", "group_id"), textValue(course, "gid"))
	seriesID := firstNonEmpty(textValue(course, "seriesId", "series_id"), textValue(course, "sid"))
	courseID := firstNonEmpty(textValue(course, "courseId", "course_id"), textValue(course, "id", "contentId", "content_id"))
	if groupID != "" && seriesID != "" {
		return CCTALK_BASE_URL + "/m/group/" + url.PathEscape(groupID) + "/series/" + url.PathEscape(seriesID)
	}
	if seriesID != "" {
		return CCTALK_BASE_URL + "/m/series/" + url.PathEscape(seriesID)
	}
	if groupID != "" {
		return CCTALK_BASE_URL + "/m/group/" + url.PathEscape(groupID)
	}
	if courseID != "" {
		return CCTALK_BASE_URL + "/m/course/" + url.PathEscape(courseID)
	}
	return ""
}

func mergeStringMaps(base, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func decodeClubAuthUID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value)%2 != 0 {
		return ""
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return ""
	}
	uid, _, _ := strings.Cut(string(decoded), ".")
	if onlyDigits(uid) {
		return uid
	}
	return ""
}

func accessTokenUID(value string) string {
	prefix, _, ok := strings.Cut(strings.TrimSpace(value), ".")
	if !ok || prefix == "" {
		return ""
	}
	var n uint64
	if _, err := fmt.Sscanf(prefix, "%x", &n); err != nil || n == 0 {
		return ""
	}
	return fmt.Sprint(n)
}

func onlyDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func intFromAny(v any) int {
	var out int
	_, _ = fmt.Sscanf(strings.TrimSpace(fmt.Sprint(v)), "%d", &out)
	return out
}

func truthyAny(v any) bool {
	s := strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
	return s != "" && s != "0" && s != "false" && s != "<nil>"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
