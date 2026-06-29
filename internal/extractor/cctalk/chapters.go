package cctalk

import (
	"net/url"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
)

func buildEntriesAndChapters(a *apiClient, structs any, fallback ids, courseMeta map[string]any) ([]*extractor.MediaInfo, []extractor.Chapter) {
	entries := buildEntriesFlat(a, structs, fallback, courseMeta)
	chapters := chaptersFromStructs(structs, fallback)
	if len(chapters) == 0 && len(entries) > 0 {
		chapters = make([]extractor.Chapter, 0, len(entries))
		for i, entry := range entries {
			if entry == nil {
				continue
			}
			chapters = append(chapters, extractor.Chapter{Title: entry.Title, Index: i + 1})
		}
	}
	return entries, chapters
}

func chaptersFromStructs(structs any, fallback ids) []extractor.Chapter {
	var out []extractor.Chapter
	seen := map[string]bool{}
	var walk func(any, ids)
	walk = func(value any, inherited ids) {
		switch x := value.(type) {
		case []any:
			for _, item := range x {
				walk(item, inherited)
			}
		case []map[string]any:
			for _, item := range x {
				walk(item, inherited)
			}
		case map[string]any:
			cur := mergeIDs(inherited, x)
			title := chapterTitle(x)
			if title != "" && isChapterNode(x) {
				chURL := chapterURL(x, cur)
				key := firstNonEmpty(chURL, textValue(x, "unitId", "id", "contentId", "videoId", "lessonId"), title)
				if key != "" && !seen[key] {
					seen[key] = true
					out = append(out, extractor.Chapter{Title: title, URL: chURL, Index: len(out) + 1})
				}
			}
			for _, key := range childKeys() {
				if child, ok := x[key]; ok {
					walk(child, cur)
				}
			}
		}
	}
	walk(structs, fallback)
	return out
}

func mergeIDs(base ids, item map[string]any) ids {
	return ids{
		CourseID: firstNonEmpty(textValue(item, "courseId", "course_id"), base.CourseID),
		GroupID:  firstNonEmpty(textValue(item, "groupId", "group_id"), base.GroupID),
		SeriesID: firstNonEmpty(textValue(item, "seriesId", "series_id"), base.SeriesID),
		VideoID:  firstNonEmpty(textValue(item, "videoId", "video_id", "contentId", "content_id", "lessonId", "lesson_id", "bizId"), base.VideoID),
	}
}

func childKeys() []string {
	return []string{"children", "childs", "nodes", "units", "unitList", "chapters", "chapterList", "lessons", "lessonList", "items", "list", "contents", "contentList", "videoList", "video_list", "videos", "recordList", "record_list", "records", "mediaList", "media_list", "playList", "play_list", "resources", "resourceList", "materials", "materialList"}
}

func chapterTitle(item map[string]any) string {
	return firstNonEmpty(textValue(item,
		"unitName", "unit_name", "chapterName", "chapter_name", "sectionName", "section_name",
		"lessonName", "lesson_name", "videoName", "video_name", "contentName", "content_name",
		"courseName", "seriesName", "groupName", "title", "name", "subject", "contentTitle"))
}

func isChapterNode(item map[string]any) bool {
	if hasAnyChild(item) || hasVideoHint(item) || hasArticleHint(item) || looksLikeFileInfo(item) {
		return true
	}
	for _, key := range []string{"unitId", "unit_id", "showIndex", "sort", "sortIndex", "orderIndex", "index", "seq", "sequence"} {
		if textValue(item, key) != "" {
			return true
		}
	}
	return false
}

func hasAnyChild(item map[string]any) bool {
	for _, key := range childKeys() {
		if len(extractList(item[key])) > 0 {
			return true
		}
		if m := asMap(item[key]); len(m) > 0 {
			return true
		}
	}
	return false
}

func chapterURL(item map[string]any, id ids) string {
	if u := firstNonEmpty(textValue(item, "url", "shareUrl", "shareURL", "courseUrl", "courseURL")); u != "" {
		return normalizeMediaURL(u)
	}
	if id.GroupID != "" && id.SeriesID != "" && id.VideoID != "" {
		return CCTALK_BASE_URL + "/m/group/" + url.PathEscape(id.GroupID) + "/series/" + url.PathEscape(id.SeriesID) + "/v/" + url.PathEscape(id.VideoID)
	}
	if id.SeriesID != "" && id.VideoID != "" {
		return CCTALK_BASE_URL + "/m/series/" + url.PathEscape(id.SeriesID) + "/v/" + url.PathEscape(id.VideoID)
	}
	if id.GroupID != "" && id.SeriesID != "" {
		return CCTALK_BASE_URL + "/m/group/" + url.PathEscape(id.GroupID) + "/series/" + url.PathEscape(id.SeriesID)
	}
	if id.SeriesID != "" {
		return CCTALK_BASE_URL + "/m/series/" + url.PathEscape(id.SeriesID)
	}
	if id.GroupID != "" {
		return CCTALK_BASE_URL + "/m/group/" + url.PathEscape(id.GroupID)
	}
	if id.CourseID != "" {
		return CCTALK_BASE_URL + "/m/course/" + url.PathEscape(id.CourseID)
	}
	return ""
}

func courseExtra(id ids, course map[string]any) map[string]any {
	extra := map[string]any{
		"course_id": id.CourseID,
		"group_id":  id.GroupID,
		"series_id": id.SeriesID,
		"course":    course,
	}
	if price := priceInfo(course); len(price) > 0 {
		extra["price"] = price
	}
	return extra
}

func priceInfo(item map[string]any) map[string]any {
	if len(item) == 0 {
		return nil
	}
	out := map[string]any{}
	if price := firstNonEmpty(textValue(item, "price", "sellPrice", "salePrice", "currentPrice", "coursePrice", "minPrice", "amount")); price != "" {
		out["price"] = price
	}
	if original := firstNonEmpty(textValue(item, "originPrice", "originalPrice", "marketPrice", "listPrice", "discountPrice")); original != "" {
		out["original_price"] = original
	}
	if free := triStateBool(item, "isFree", "free", "freeFlag"); free != nil {
		out["free"] = *free
	}
	if paid := triStateBool(item, "isBought", "isBuy", "isPurchased", "purchased", "hasBuy", "hasPaid", "paid", "isJoin", "joined", "isMember"); paid != nil {
		out["purchased"] = *paid
	} else if status := strings.ToLower(firstNonEmpty(textValue(item, "buyStatus", "purchaseStatus", "payStatus", "studyStatus", "joinStatus"))); status != "" {
		out["purchased"] = status == "1" || status == "2" || status == "true" || strings.Contains(status, "buy") || strings.Contains(status, "paid") || strings.Contains(status, "join")
	}
	if needPay := triStateBool(item, "needPay", "need_pay", "shouldPay"); needPay != nil {
		out["need_pay"] = *needPay
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func triStateBool(item map[string]any, keys ...string) *bool {
	for _, key := range keys {
		raw, ok := item[key]
		if !ok || raw == nil {
			continue
		}
		s := strings.ToLower(strings.TrimSpace(textAny(raw)))
		if s == "" || s == "<nil>" {
			continue
		}
		value := s == "1" || s == "true" || s == "yes" || s == "y" || s == "paid" || s == "bought" || s == "joined"
		if s == "0" || s == "false" || s == "no" || s == "n" || s == "unpaid" || s == "none" {
			value = false
		}
		return &value
	}
	return nil
}
