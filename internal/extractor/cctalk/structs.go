package cctalk

import (
	"encoding/json"
	"fmt"
	"strings"
)

func normalizeSeriesStructs(data any, seriesID string) any {
	if data == nil {
		return nil
	}
	seriesID = strings.TrimSpace(seriesID)
	var normalizeNode func(any) any
	normalizeNode = func(value any) any {
		switch x := value.(type) {
		case []any:
			out := make([]any, 0, len(x))
			for _, item := range x {
				if normalized := normalizeNode(item); normalized != nil {
					out = append(out, normalized)
				}
			}
			if len(out) == 0 {
				return nil
			}
			return out
		case []map[string]any:
			out := make([]any, 0, len(x))
			for _, item := range x {
				if normalized := normalizeNode(item); normalized != nil {
					out = append(out, normalized)
				}
			}
			if len(out) == 0 {
				return nil
			}
			return out
		case map[string]any:
			m := mergeMaps(x, asMap(x["videoInfo"]))
			if seriesID != "" {
				if _, ok := m["seriesId"]; !ok {
					m["seriesId"] = seriesID
				}
			}
			contentID := firstNonEmpty(textValue(m, "contentId", "content_id"), textValue(m, "videoId", "video_id"), textValue(m, "lessonId", "lesson_id"), textValue(m, "bizId"))
			ct := firstNonEmpty(textValue(m, "contentType", "content_type"), textValue(m, "sourceType", "source_type"), textValue(m, "type"))
			if contentID == "" && isVideoContentType(ct) {
				contentID = textValue(m, "id")
			}
			if contentID != "" {
				if _, ok := m["contentId"]; !ok {
					m["contentId"] = contentID
				}
				if _, ok := m["lessonId"]; !ok {
					m["lessonId"] = contentID
				}
				if isVideoContentType(ct) || ct == "" || asMap(x["videoInfo"]) != nil {
					if _, ok := m["videoId"]; !ok {
						m["videoId"] = contentID
					}
				}
			}
			for _, key := range childKeys() {
				if child, ok := m[key]; ok {
					if normalized := normalizeNode(child); normalized != nil {
						m[key] = normalized
					}
				}
			}
			return m
		default:
			return value
		}
	}
	if normalized := normalizeNode(data); normalized != nil {
		return normalized
	}
	return data
}

func (a *apiClient) queryLessonList(courseID string, lessonIDs []string) []map[string]any {
	courseID = strings.TrimSpace(courseID)
	lessonIDs = uniqueNonEmpty(lessonIDs)
	if courseID == "" || len(lessonIDs) == 0 {
		return nil
	}
	idsJSON, _ := json.Marshal(lessonIDs)
	idsCSV := strings.Join(lessonIDs, ",")
	for _, body := range []string{string(idsJSON), idsCSV} {
		for _, version := range []string{"v1.1", "v1.2"} {
			data := extractData(a.requestAPI(fmt.Sprintf("/course/%s/query_lesson_list", courseID), map[string]string{"lessonIds": body}, "post", version))
			if out := mapsFromAny(data); len(out) > 0 {
				return out
			}
		}
	}
	return nil
}

func (a *apiClient) batchQueryLesson(courseID string, structs any) []map[string]any {
	courseID = strings.TrimSpace(courseID)
	if courseID == "" || structs == nil || a == nil || a.c == nil {
		return nil
	}
	unitList := extractStructUnitList(structs)
	if len(unitList) == 0 {
		var ids []string
		for _, node := range walkMaps(structs) {
			ids = append(ids, firstNonEmpty(textValue(node, "lessonId", "lesson_id"), textValue(node, "contentId", "content_id"), textValue(node, "videoId", "video_id")))
		}
		return a.queryLessonList(courseID, ids)
	}
	payload, _ := json.Marshal(unitList)
	for _, version := range []string{"v1.1", "v1.2"} {
		data := extractData(a.requestAPI(fmt.Sprintf("/course/%s/batch_query_lesson", courseID), map[string]string{"courseId": courseID, "unitList": string(payload)}, "post", version))
		if out := mapsFromAny(data); len(out) > 0 {
			return out
		}
	}
	return nil
}

func extractStructUnitList(structs any) []map[string]any {
	var out []map[string]any
	for _, node := range walkMaps(structs) {
		unitID := firstNonEmpty(textValue(node, "unitId", "unit_id"), textValue(node, "id"))
		lessonID := firstNonEmpty(textValue(node, "lessonId", "lesson_id"), textValue(node, "contentId", "content_id"), textValue(node, "videoId", "video_id"), textValue(node, "bizId"))
		if unitID == "" && lessonID == "" {
			continue
		}
		item := map[string]any{}
		if unitID != "" {
			item["unitId"] = unitID
		}
		if lessonID != "" {
			item["lessonId"] = lessonID
			item["contentId"] = lessonID
		}
		out = append(out, item)
	}
	return out
}

func mergeStructsWithDetails(structs any, details []map[string]any) any {
	if len(details) == 0 {
		return structs
	}
	byID := map[string]map[string]any{}
	for _, detail := range details {
		key := firstNonEmpty(textValue(detail, "lessonId", "lesson_id"), textValue(detail, "contentId", "content_id"), textValue(detail, "videoId", "video_id"), textValue(detail, "id", "bizId"))
		if key != "" {
			byID[key] = detail
		}
	}
	var merge func(any) any
	merge = func(value any) any {
		switch x := value.(type) {
		case []any:
			out := make([]any, 0, len(x))
			for _, item := range x {
				out = append(out, merge(item))
			}
			return out
		case map[string]any:
			m := mergeMaps(nil, x)
			key := firstNonEmpty(textValue(m, "lessonId", "lesson_id"), textValue(m, "contentId", "content_id"), textValue(m, "videoId", "video_id"), textValue(m, "id", "bizId"))
			if detail := byID[key]; len(detail) > 0 {
				m = mergeMaps(m, detail)
			}
			for _, child := range childKeys() {
				if nested, ok := m[child]; ok {
					m[child] = merge(nested)
				}
			}
			return m
		default:
			return value
		}
	}
	return merge(structs)
}

func mapsFromAny(value any) []map[string]any {
	var out []map[string]any
	for _, item := range extractList(value) {
		if m := asMap(item); len(m) > 0 {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		if m := asMap(value); len(m) > 0 {
			out = append(out, m)
		}
	}
	return out
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
