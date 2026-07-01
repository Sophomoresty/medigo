package cctalk

import (
	"fmt"
	"html"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

func entriesFromMap(a *apiClient, item map[string]any, fallbackTitle string) []*extractor.MediaInfo {
	var out []*extractor.MediaInfo
	if hasArticleHint(item) {
		if entry := articleEntry(a, item, fallbackTitle); entry != nil {
			out = append(out, entry)
		}
	}
	for i, material := range iterMaterialCandidates(item) {
		if entry := fileEntry(a, material, i+1, fallbackTitle); entry != nil {
			out = append(out, entry)
		}
	}
	if hasVideoHint(item) || findMediaURL(item) != "" || textValue(extractCoursewareInfo(item), "coursewareId") != "" || hasProviderVideoHint(item) || isBoardPayload(item) {
		if entry, err := mediaFromMap(a, item, fallbackTitle); err == nil {
			out = append(out, entry)
		} else if b, ok := asBlocked(err); ok {
			out = append(out, blockedEntry(firstNonEmpty(textValue(item, "lessonName", "videoName", "contentName", "title", "name", "subject"), fallbackTitle), b))
		}
	}
	return out
}

func hasDownloadableResource(item map[string]any) bool {
	return hasArticleHint(item) || looksLikeFileInfo(item) || hasVideoHint(item) || hasProviderVideoHint(item) || textValue(extractCoursewareInfo(item), "coursewareId") != "" || isBoardPayload(item)
}

func hasVideoHint(item map[string]any) bool {
	if findMediaURL(item) != "" {
		return true
	}
	if textValue(extractCoursewareInfo(item), "coursewareId") != "" {
		return true
	}
	if hasProviderVideoHint(item) {
		return true
	}
	for _, key := range []string{"videoId", "video_id", "coursewareId", "courseWareId", "contentId", "lessonId", "lesson_id", "bizId"} {
		if textValue(item, key) != "" {
			ct := strings.ToLower(firstNonEmpty(textValue(item, "contentType"), textValue(item, "content_type"), textValue(item, "sourceType"), textValue(item, "source_type")))
			return ct == "" || isVideoContentType(ct)
		}
	}
	return false
}

func isVideoContentType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "2", "3", "video", "vod", "record", "recorded", "replay", "board", "whiteboard":
		return true
	default:
		return false
	}
}

func isMaterialContentType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "4", "file", "doc", "document", "material", "resource", "attachment":
		return true
	default:
		return false
	}
}

func hasArticleHint(item map[string]any) bool {
	if _, ok := item["articleInfo"].(map[string]any); ok {
		return true
	}
	if textValue(item, "articleId", "article_id") != "" {
		return true
	}
	ct := strings.ToLower(firstNonEmpty(textValue(item, "contentType"), textValue(item, "content_type"), textValue(item, "sourceType"), textValue(item, "source_type"), textValue(item, "type")))
	return ct == "article" || ct == "graphic" || ct == "图文"
}

func articleEntry(a *apiClient, item map[string]any, fallbackTitle string) *extractor.MediaInfo {
	article := mapFromAny(item["articleInfo"])
	if len(article) == 0 {
		article = item
	}
	articleID := firstNonEmpty(textValue(article, "articleId", "article_id", "id"), textValue(item, "articleId", "article_id", "contentId", "lessonId", "id"))
	if !articleHasBody(article) && articleID != "" && a != nil {
		if detail := a.getArticleDetail(articleID); len(detail) > 0 {
			article = mergeMaps(article, detail)
		}
	}
	title := firstNonEmpty(textValue(article, "articleName", "title", "name", "contentTitle"), textValue(item, "lessonName", "contentName", "title", "name"), fallbackTitle, "未命名图文")
	doc := buildArticleHTML(title, article)
	return &extractor.MediaInfo{
		Site:  "cctalk",
		Title: util.SanitizeFilename(stripExt(title)),
		Streams: map[string]extractor.Stream{
			"document": {Quality: "article", URLs: []string{dataURL("text/html", doc)}, Format: "html", Headers: baseHeaders()},
		},
		Extra: map[string]any{"type": "article", "article_id": articleID, "article_info": article},
	}
}

func (a *apiClient) getArticleDetail(articleID string) map[string]any {
	if a == nil || a.c == nil || strings.TrimSpace(articleID) == "" {
		return nil
	}
	data := extractData(a.requestAPI("/article/detail", map[string]string{"articleId": articleID, "contentId": articleID}, "", "v1.1"))
	return asMap(data)
}

func articleHasBody(article map[string]any) bool {
	for _, key := range []string{"content", "body", "intro", "detail", "richText", "html", "text"} {
		if strings.TrimSpace(textValue(article, key)) != "" {
			return true
		}
	}
	return false
}

func buildArticleHTML(title string, article map[string]any) string {
	var parts []string
	for _, pair := range [][2]string{{"标题", title}, {"发布时间", textValue(article, "publishTime", "publish_time", "createdAt")}, {"浏览数", textValue(article, "viewCount", "view_count")}} {
		if strings.TrimSpace(pair[1]) != "" {
			parts = append(parts, "<p><strong>"+html.EscapeString(pair[0])+"</strong>: "+html.EscapeString(pair[1])+"</p>")
		}
	}
	body := firstNonEmpty(textValue(article, "content"), textValue(article, "body"), textValue(article, "richText"), textValue(article, "html"), textValue(article, "intro"), textValue(article, "text"))
	if body == "" {
		if intro := htmlFromIntroList(article["introList"]); intro != "" {
			body = intro
		}
	}
	if body == "" {
		body = "<p>暂无图文内容</p>"
	} else if !strings.Contains(strings.ToLower(body), "<p") && !strings.Contains(strings.ToLower(body), "<div") && !strings.Contains(strings.ToLower(body), "<img") {
		body = "<p>" + html.EscapeString(body) + "</p>"
	}
	parts = append(parts, body)
	escapedTitle := html.EscapeString(firstNonEmpty(title, "未命名图文"))
	return "<!doctype html><html><head><meta charset=\"utf-8\"><title>" + escapedTitle + "</title></head><body><h1>" + escapedTitle + "</h1>" + strings.Join(parts, "\n") + "</body></html>"
}

func htmlFromIntroList(value any) string {
	var parts []string
	var walk func(any)
	walk = func(value any) {
		switch x := value.(type) {
		case string:
			if strings.TrimSpace(x) != "" {
				if strings.Contains(strings.ToLower(x), "<img") || strings.Contains(strings.ToLower(x), "<p") || strings.Contains(strings.ToLower(x), "<div") {
					parts = append(parts, x)
				} else if looksMediaURL(x) || isImageURL(x) {
					parts = append(parts, `<p><img src="`+html.EscapeString(normalizeMediaURL(x))+`"></p>`)
				} else {
					parts = append(parts, "<p>"+html.EscapeString(x)+"</p>")
				}
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		case map[string]any:
			if img := firstNonEmpty(textValue(x, "imgUrl", "imageUrl", "imageURL", "picUrl", "picURL", "url", "src")); img != "" && (isImageURL(img) || textValue(x, "type") == "image") {
				parts = append(parts, `<p><img src="`+html.EscapeString(normalizeMediaURL(img))+`"></p>`)
			} else if text := firstNonEmpty(textValue(x, "text", "content", "html", "value", "title")); text != "" {
				walk(text)
			}
		}
	}
	walk(value)
	return strings.Join(parts, "\n")
}

func iterMaterialCandidates(item map[string]any) []map[string]any {
	var out []map[string]any
	var walk func(any, int)
	walk = func(value any, depth int) {
		if value == nil || depth > 6 {
			return
		}
		switch x := value.(type) {
		case map[string]any:
			if looksLikeFileInfo(x) {
				out = append(out, x)
			}
			for _, key := range []string{"materials", "materialList", "coursewareList", "resourceList", "resources", "attachments", "attachmentList", "files", "fileList", "docs", "docList"} {
				if nested, ok := x[key]; ok {
					walk(nested, depth+1)
				}
			}
			for _, nested := range x {
				switch nested.(type) {
				case map[string]any, []any:
					walk(nested, depth+1)
				}
			}
		case []any:
			for _, it := range x {
				walk(it, depth+1)
			}
		}
	}
	walk(item, 0)
	return dedupeMapsByURL(out)
}

func looksLikeFileInfo(item map[string]any) bool {
	if item == nil {
		return false
	}
	if textValue(item, "fileUrl", "fileURL", "resourceUrl", "resourceURL", "materialUrl", "attachUrl") != "" {
		return true
	}
	if textValue(item, "fileName", "file_name", "resourceName", "materialName", "attachName") != "" {
		return true
	}
	if isMaterialContentType(firstNonEmpty(textValue(item, "contentType"), textValue(item, "content_type"), textValue(item, "sourceType"), textValue(item, "source_type"), textValue(item, "type"))) {
		return true
	}
	return isMaterialURL(textValue(item, "downloadUrl", "url"))
}

func isMaterialURL(fileURL string) bool {
	lower := strings.ToLower(strings.TrimSpace(fileURL))
	if lower == "" {
		return false
	}
	for _, ext := range []string{".pdf", ".ppt", ".pptx", ".doc", ".docx", ".xls", ".xlsx", ".zip", ".rar", ".7z", ".txt"} {
		if strings.Contains(lower, ext) {
			return true
		}
	}
	return strings.Contains(lower, "/file/") || strings.Contains(lower, "/files/") || strings.Contains(lower, "/resource/") || strings.Contains(lower, "/download/")
}

func isImageURL(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg"} {
		if strings.Contains(lower, ext) {
			return true
		}
	}
	return false
}

func normalizeFileURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "/") {
		return normalizeMediaURL(raw)
	}
	if strings.Contains(raw, "/") && !strings.Contains(raw, "://") {
		return strings.TrimRight(CCTALK_OCS_MATERIAL_HOST, "/") + "/" + strings.TrimLeft(raw, "/")
	}
	return normalizeMediaURL(raw)
}

func fileEntry(a *apiClient, item map[string]any, index int, fallbackTitle string) *extractor.MediaInfo {
	fileURL := normalizeFileURL(firstNonEmpty(textValue(item, "fileUrl", "fileURL", "resourceUrl", "resourceURL", "materialUrl", "attachUrl", "downloadUrl", "url", "path", "filePath", "resourcePath")))
	if fileURL == "" {
		return nil
	}
	headers := baseHeaders()
	if a != nil && a.headers != nil {
		headers = a.headers
	}
	rawTitle := firstNonEmpty(textValue(item, "fileName", "file_name", "resourceName", "materialName", "attachName", "title", "name", "contentTitle", "coursewareName"), fallbackTitle, "资料")
	ext := guessFileExt(rawTitle, fileURL)
	if ext == "" {
		ext = "dat"
	}
	title := util.SanitizeFilename(stripExt(fmt.Sprintf("[%d]--%s", index, rawTitle)))
	return &extractor.MediaInfo{
		Site:  "cctalk",
		Title: title,
		Streams: map[string]extractor.Stream{
			"file": {Quality: "file", URLs: []string{fileURL}, Format: ext, Size: int64Value(item["fileSize"], item["size"], item["totalSize"]), Headers: headers},
		},
		Extra: map[string]any{"type": "file", "file_url": fileURL, "file_name": rawTitle, "raw": item},
	}
}

func guessFileExt(name, rawURL string) string {
	for _, source := range []string{name, rawURL} {
		if source == "" {
			continue
		}
		if u, err := url.Parse(source); err == nil && u.Path != "" {
			source = u.Path
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(strings.Split(source, "?")[0])), ".")
		if ext != "" && len(ext) <= 8 {
			return ext
		}
	}
	return ""
}

func stripExt(name string) string {
	ext := filepath.Ext(name)
	if ext != "" && len(ext) <= 9 {
		return strings.TrimSuffix(name, ext)
	}
	return name
}

func mapFromAny(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}

func dedupeMapsByURL(items []map[string]any) []map[string]any {
	seen := map[string]bool{}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		key := firstNonEmpty(textValue(item, "fileUrl", "fileURL", "resourceUrl", "resourceURL", "materialUrl", "attachUrl", "downloadUrl", "url"), textValue(item, "fileName", "file_name", "resourceName", "materialName", "attachName"))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func entryKey(info *extractor.MediaInfo) string {
	if info == nil {
		return ""
	}
	for _, stream := range info.Streams {
		if len(stream.URLs) > 0 {
			return info.Title + "\x00" + stream.URLs[0]
		}
	}
	return info.Title
}

func int64Value(values ...any) int64 {
	for _, value := range values {
		text := textAny(value)
		if text == "" {
			continue
		}
		var n int64
		if _, err := fmt.Sscan(text, &n); err == nil && n > 0 {
			return n
		}
	}
	return 0
}
