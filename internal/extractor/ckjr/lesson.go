package ckjr

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Sophomoresty/mediago/internal/util"
)

func collectLessonNodes(payload any) []ckjrLessonNode {
	roots := extractPageRows(payload)
	if len(roots) == 0 {
		switch v := payloadData(payload).(type) {
		case map[string]any:
			roots = []map[string]any{v}
		case []any:
			roots = mapsFromAny(v)
		}
	}
	var out []ckjrLessonNode
	seen := map[string]bool{}
	var walk func(map[string]any, []int, []string)
	walk = func(node map[string]any, prefix []int, chapters []string) {
		if len(node) == 0 {
			return
		}
		children := nodeChildren(node)
		if len(children) > 0 {
			if nodeHasDirectMedia(node) {
				appendLessonNode(&out, seen, node, prefix, chapters)
			}
			nextChapters := chapters
			if title := nodeTitle(node); title != "" && !nodeHasDirectMedia(node) {
				nextChapters = append(append([]string{}, chapters...), title)
			}
			for i, child := range children {
				walk(child, append(append([]int{}, prefix...), i+1), nextChapters)
			}
			return
		}
		if looksLikeLessonNode(node) {
			appendLessonNode(&out, seen, node, prefix, chapters)
		}
	}
	for i, root := range roots {
		walk(root, []int{i + 1}, nil)
	}
	return out
}

func appendLessonNode(out *[]ckjrLessonNode, seen map[string]bool, node map[string]any, prefix []int, chapters []string) {
	key := strings.Join([]string{nodeResourceID(resolveKindFromNode(node, ""), node, ""), nodeTitle(node), fmt.Sprint(prefix)}, "|")
	if key != "" && seen[key] {
		return
	}
	seen[key] = true
	*out = append(*out, ckjrLessonNode{Node: node, Prefix: append([]int{}, prefix...), Chapter: strings.Join(cleanStrings(chapters), "--"), Index: len(*out) + 1})
}

func nodeChildren(node map[string]any) []map[string]any {
	for _, key := range []string{"children", "childList", "dirs", "subDirs", "list", "items", "records", "nodes", "sections", "sectionList", "chapters", "chapterList", "lessons", "lessonList", "courseList", "productList"} {
		if rows := listMapsFromAny(node[key]); len(rows) > 0 {
			return rows
		}
	}
	return nil
}

func looksLikeLessonNode(node map[string]any) bool {
	if nodeHasDirectMedia(node) {
		return true
	}
	for _, key := range []string{"prodId", "productId", "extId", "lessonId", "courseId", "detailId", "detail_id", "videoUrlEncode", "audioUrlEncode", "videoName", "audioName", "duration", "isPreview", "allowWholeWatch", "watchPermission", "videoType", "type", "liveId", "datumId", "testId", "courseType", "prodType", "qiniuObject", "fileName", "filename"} {
		if textValue(node, key) != "" {
			return true
		}
	}
	return false
}

func nodeHasDirectMedia(node map[string]any) bool {
	if directMediaURL(node) != "" || qcloudAuth(node) != nil || datumAccessURL(node) != "" || directHTMLContent(node) != "" {
		return true
	}
	for key, value := range node {
		s, ok := value.(string)
		if !ok || strings.TrimSpace(s) == "" {
			continue
		}
		mediaKind := inferMediaTypeFromKey(key)
		if mediaKind != "" {
			if u := normalizeMediaText(s); strings.HasPrefix(strings.ToLower(u), "http") || strings.HasPrefix(strings.TrimSpace(strings.ToLower(u)), "#extm3u") {
				return true
			}
			if maybeDecryptMediaValue(key, s) != "" {
				return true
			}
		}
	}
	return false
}

func directHTMLContent(node map[string]any) string {
	plainTextKeys := map[string]bool{
		"content": true, "detailContent": true, "graphicDetail": true, "graphicContent": true,
		"articleContent": true, "editorValue": true, "txtContent": true,
	}
	for _, key := range []string{"content", "detailContent", "graphicDetail", "graphicContent", "articleContent", "editorValue", "description", "intro", "introduce", "txtContent"} {
		s := strings.TrimSpace(fmt.Sprint(node[key]))
		if s == "" || s == "<nil>" {
			continue
		}
		lower := strings.ToLower(s)
		for _, hint := range []string{"<p", "<div", "<img", "<br", "<span", "<table", "<html", "&nbsp;"} {
			if strings.Contains(lower, hint) {
				return s
			}
		}
		if plainTextKeys[key] {
			return s
		}
	}
	return ""
}

func nodeTitle(node map[string]any) string {
	return util.SanitizeFilename(firstNonEmpty(textValue(node, "title", "name", "dirName", "chapterName", "sectionName", "courseTitle", "prodTitle", "videoName", "audioName", "detailName", "lessonName", "fileName", "filename"), ""))
}

func lessonTitle(lesson ckjrLessonNode, cand mediaCandidate, fallback string) string {
	title := firstNonEmpty(cand.Title, nodeTitle(lesson.Node), fallback, "ckjr")
	title = strings.TrimSuffix(title, "."+strings.TrimPrefix(strings.ToLower(cand.Format), "."))
	if len(lesson.Prefix) == 0 {
		return title
	}
	return fmt.Sprintf("[%s]--%s", joinInts(lesson.Prefix, "."), title)
}

func resolveKindFromNode(node map[string]any, fallback string) string {
	if kind := resolveCourseListKind(node, ""); kind != "" {
		return kind
	}
	if textValue(node, "liveId") != "" {
		return "live"
	}
	if textValue(node, "datumId", "qiniuObject") != "" {
		return "datum"
	}
	if textValue(node, "testId") != "" {
		return "testPaper"
	}
	if _, ok := routeCfg[fallback]; ok {
		return fallback
	}
	return "video"
}

func nodeResourceID(kind string, node map[string]any, fallback string) string {
	keys := map[string][]string{
		"testPaper":    {"testId", "prodId", "productId", "id"},
		"livePersonal": {"liveId", "activityId", "prodId", "productId", "id"},
		"live":         {"liveId", "activityId", "prodId", "productId", "id"},
		"datum":        {"datumId", "prodId", "productId", "id"},
		"package":      {"combosId", "comboId", "prodId", "productId", "id"},
		"column":       {"extId", "columnId", "prodId", "productId", "courseId", "course_id", "id"},
		"imgText":      {"courseId", "prodId", "productId", "course_id", "id"},
		"voice":        {"courseId", "prodId", "productId", "course_id", "id"},
		"video":        {"courseId", "prodId", "productId", "course_id", "id"},
	}
	for _, key := range keys[firstNonEmpty(kind, "video")] {
		if v := textValue(node, key); v != "" {
			return v
		}
	}
	return fallback
}

func courseListResourceID(kind string, node map[string]any, fallback string) string {
	keys := map[string][]string{
		"testPaper":    {"testId", "prodId", "productId", "id"},
		"livePersonal": {"liveId", "activityId", "prodId", "productId", "id"},
		"live":         {"liveId", "activityId", "prodId", "productId", "id"},
		"datum":        {"datumId", "prodId", "productId", "id"},
		"package":      {"combosId", "comboId", "prodId", "productId", "id"},
		"column":       {"prodId", "productId", "extId", "courseId", "course_id", "id"},
		"imgText":      {"prodId", "productId", "courseId", "course_id", "id"},
		"voice":        {"prodId", "productId", "courseId", "course_id", "id"},
		"video":        {"prodId", "productId", "courseId", "course_id", "id"},
	}
	for _, key := range keys[firstNonEmpty(kind, "video")] {
		if v := textValue(node, key); v != "" {
			return v
		}
	}
	return fallback
}

func extractPageRows(payload any) []map[string]any {
	container := extractDirContainer(payload)
	if rows := listMapsFromAny(container); len(rows) > 0 {
		return rows
	}
	m := asMap(container)
	for _, key := range []string{"list", "rows", "items", "records", "dirs", "children", "data", "sections", "sectionList", "chapterList", "lessonList", "courseList", "productList"} {
		if rows := listMapsFromAny(m[key]); len(rows) > 0 {
			return rows
		}
	}
	return nil
}

func extractDirContainer(payload any) any {
	if rows := listMapsFromAny(payload); len(rows) > 0 {
		return rows
	}
	root := asMap(payload)
	if len(root) == 0 {
		return map[string]any{}
	}
	candidates := []map[string]any{root}
	for _, key := range []string{"data", "result", "response"} {
		v := root[key]
		if rows := listMapsFromAny(v); len(rows) > 0 {
			return rows
		}
		if m := asMap(v); len(m) > 0 {
			candidates = append(candidates, m)
		}
	}
	for _, m := range candidates {
		for _, key := range []string{"list", "rows", "items", "records", "dirs", "children", "data", "sections", "sectionList", "chapterList", "lessonList", "courseList", "productList"} {
			if rows := listMapsFromAny(m[key]); len(rows) > 0 {
				return m
			}
		}
	}
	data := payloadData(payload)
	if rows := listMapsFromAny(data); len(rows) > 0 {
		return rows
	}
	if m := asMap(data); len(m) > 0 {
		return m
	}
	return root
}

func payloadData(payload any) any {
	for depth := 0; depth < 16; depth++ {
		m := asMap(payload)
		if len(m) == 0 {
			return payload
		}
		advanced := false
		for _, key := range []string{"data", "result", "response"} {
			if v, ok := m[key]; ok {
				switch v.(type) {
				case map[string]any, []any:
					payload = v
					advanced = true
				}
				if advanced {
					break
				}
			}
		}
		if !advanced {
			return payload
		}
	}
	return payload
}

func listMapsFromAny(v any) []map[string]any {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		if m := asMap(item); len(m) > 0 {
			out = append(out, m)
		}
	}
	return out
}

func pageHasMore(payload any, page, pageSize, rowCount int) bool {
	m := asMap(extractDirContainer(payload))
	for _, key := range []string{"hasMore", "has_next", "nextPage", "hasNext"} {
		if v, ok := m[key]; ok {
			return coerceBool(fmt.Sprint(v), false)
		}
	}
	total := intLike(firstNonEmpty(textValue(m, "total"), textValue(m, "totalCount"), textValue(m, "count")), 0)
	if total > 0 {
		return page*pageSize < total
	}
	totalPage := intLike(firstNonEmpty(textValue(m, "totalPage"), textValue(m, "totalPages"), textValue(m, "pages"), textValue(m, "lastPage")), 0)
	if totalPage > 0 {
		current := intLike(firstNonEmpty(textValue(m, "page"), textValue(m, "pageNum"), textValue(m, "current_page"), textValue(m, "currentPage")), page)
		return current < totalPage
	}
	return rowCount >= pageSize
}

func selectLessonCandidates(cands []mediaCandidate) []mediaCandidate {
	cands = dedupeCandidates(cands)
	sortMediaCandidates(cands)
	var out []mediaCandidate
	addedMedia := map[string]bool{}
	for _, cand := range cands {
		cand = normalizeCandidate(cand)
		kind := firstNonEmpty(cand.Kind, mediaKindFromFormat(cand.Format))
		if kind == "video" || kind == "audio" {
			if addedMedia[kind] {
				continue
			}
			addedMedia[kind] = true
			out = append(out, cand)
			continue
		}
		out = append(out, cand)
	}
	return out
}

func dedupeCandidates(in []mediaCandidate) []mediaCandidate {
	seen := map[string]bool{}
	out := make([]mediaCandidate, 0, len(in))
	for _, cand := range in {
		cand = normalizeCandidate(cand)
		key := strings.ToLower(firstNonEmpty(cand.URL, cand.Title))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, cand)
	}
	return out
}

func sortMediaCandidates(cands []mediaCandidate) {
	sort.SliceStable(cands, func(i, j int) bool {
		ri, rj := candidateRank(cands[i]), candidateRank(cands[j])
		if ri != rj {
			return ri < rj
		}
		return cands[i].URL < cands[j].URL
	})
}

func candidateRank(cand mediaCandidate) int {
	cand = normalizeCandidate(cand)
	kind := firstNonEmpty(cand.Kind, mediaKindFromFormat(cand.Format))
	format := strings.ToLower(cand.Format)
	switch {
	case kind == "video" && format == "mp4":
		return 0
	case kind == "video":
		return 1
	case kind == "audio":
		return 2
	case kind == "file" && format == "pdf":
		return 3
	case kind == "file":
		return 4
	default:
		return 5
	}
}

func routeTitle(payloads []any, r routeInfo) string {
	for _, payload := range payloads {
		if title := firstNonEmpty(textValue(asMap(payloadData(payload)), "title", "courseTitle", "courseName", "name", "prodTitle", "productTitle", "prodName", "datumTitle", "liveTitle", "paperTitle", "testTitle", "columnTitle", "combosTitle", "combosName", "comboTitle", "comboName"), firstTextFromNodes(payload, "title", "courseTitle", "courseName", "prodTitle", "prodName", "name")); title != "" {
			return title
		}
	}
	if r.Kind == "package" {
		return "套餐_" + r.ID
	}
	return "ckjr_" + r.ID
}

func firstTextFromNodes(payload any, keys ...string) string {
	for _, node := range walkMaps(payload) {
		if text := textValue(node, keys...); text != "" {
			return text
		}
	}
	return ""
}

func routeIsSingle(kind string) bool {
	switch kind {
	case "video", "voice", "column", "package":
		return false
	default:
		return true
	}
}

func apiResponseOK(payload any) bool {
	m := asMap(payload)
	if len(m) == 0 {
		return false
	}
	code := responseCode(payload)
	if code != "" && code != "0" && code != "200" {
		return false
	}
	if v, ok := m["success"]; ok && !coerceBool(fmt.Sprint(v), true) {
		return false
	}
	return true
}

func apiLoginResponseOK(payload any) bool {
	m := asMap(payload)
	if len(m) == 0 {
		return false
	}
	code := responseCode(payload)
	if code != "200" {
		return false
	}
	if v, ok := m["success"]; ok && !coerceBool(fmt.Sprint(v), true) {
		return false
	}
	return true
}

func responseCode(payload any) string {
	m := asMap(payload)
	return firstNonEmpty(textValue(m, "statusCode"), textValue(m, "code"))
}

func routeDisplayName(kind string) string {
	switch kind {
	case "testPaper":
		return "试卷"
	case "livePersonal", "live":
		return "直播"
	case "datum":
		return "资料"
	case "package":
		return "套餐"
	case "column":
		return "专栏"
	case "imgText":
		return "图文"
	case "voice":
		return "音频"
	case "video":
		return "视频"
	default:
		return "课程"
	}
}

func routeSortRank(kind string) int {
	switch kind {
	case "video":
		return 0
	case "voice":
		return 1
	case "imgText":
		return 2
	case "column":
		return 3
	case "package":
		return 4
	case "datum":
		return 5
	case "live", "livePersonal":
		return 6
	case "testPaper":
		return 7
	default:
		return 9
	}
}

func cleanStrings(vals []string) []string {
	out := vals[:0]
	for _, val := range vals {
		if strings.TrimSpace(val) != "" {
			out = append(out, strings.TrimSpace(val))
		}
	}
	return out
}

func joinInts(vals []int, sep string) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, sep)
}

func firstBoolText(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return strings.TrimSpace(fmt.Sprint(v))
		}
	}
	return ""
}

func coerceBool(raw string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return def
	case "1", "true", "yes", "y":
		return true
	case "0", "false", "no", "n":
		return false
	default:
		return def
	}
}

func intLike(v any, def int) int {
	switch t := v.(type) {
	case string:
		if t == "" {
			return def
		}
		if i, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
			return int(f)
		}
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		if !math.IsNaN(t) && !math.IsInf(t, 0) {
			return int(t)
		}
	default:
		if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
			return intLike(s, def)
		}
	}
	return def
}

func normalizePrice(raw string) float64 {
	if raw == "" {
		return 0
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(raw, ",", ""), 64)
	if err != nil || f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return math.Round(f*100) / 100
}
