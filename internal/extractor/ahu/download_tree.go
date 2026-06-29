package ahu

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"

	"github.com/Sophomoresty/mediago/internal/util"
)

func (x *ahuCourse) parseCourseVideos(body, baseURL string) []*ahuTreeNode {
	// Source uses BeautifulSoup header walk. Go keeps the same output shape and
	// falls back to surrounding-header heuristics for regex-only HTML parsing.
	seen := map[string]bool{}
	var nodes []*ahuTreeNode
	var currentChapter, currentSection *ahuTreeNode

	tokenRe := regexp.MustCompile(`(?is)<div\b([^>]*)>(.*?)</div>|<a\b([^>]*)href=["']([^"']*/video/videoplay\.html\?[^"']*lessonId=[0-9][^"']*)["'][^>]*>(.*?)</a>`)
	for _, m := range tokenRe.FindAllStringSubmatchIndex(body, -1) {
		if m[2] >= 0 {
			attrs := htmlSlice(body, m[2], m[3])
			inner := htmlSlice(body, m[4], m[5])
			switch {
			case htmlAttrsHaveClass(attrs, "yxg-collapse-head-one", "card-header"):
				idx := len(nodes) + 1
				title := extractHeaderTitle(inner, fmt.Sprintf("章节%d", idx))
				currentChapter = treeNode(formatDirTitle(idx, title), title, []int{idx})
				nodes = append(nodes, currentChapter)
				currentSection = nil
			case htmlAttrsHaveClass(attrs, "yxg-collapse-head-two"):
				currentChapter = x.ensureDefaultChapter(&nodes)
				idx := len(currentChapter.Children) + 1
				title := extractHeaderTitle(inner, fmt.Sprintf("小节%d", idx))
				currentSection = treeNode(formatDirTitle(idx, title), title, append(append([]int(nil), currentChapter.IndexTuple...), idx))
				currentChapter.Children = append(currentChapter.Children, currentSection)
			}
			continue
		}

		href := htmlSlice(body, m[8], m[9])
		lessonID := extractFirst(lessonIDRe, href)
		if lessonID == "" || seen[lessonID] {
			continue
		}
		seen[lessonID] = true
		title := cleanLessonTitle(firstNonEmpty(textForSelector(htmlSlice(body, m[10], m[11]), `yxg-timeline-title-tow`), stripTags(htmlSlice(body, m[10], m[11]))))
		target := currentSection
		if target == nil {
			target = currentChapter
		}
		if target == nil {
			target = x.ensureDefaultChapter(&nodes)
		}
		if title == "" || title == "未命名课时" {
			title = fmt.Sprintf("课时%d", target.VideoCnt+1)
		}
		target.VideoCnt++
		idx := append(append([]int(nil), target.IndexTuple...), target.VideoCnt)
		videoName := formatVideoName(idx, title)
		duration := cleanText(textForSelector(htmlSlice(body, m[10], m[11]), `yxg-item-time`))
		target.Sources = append(target.Sources, ahuSource{Type: "video", Title: videoName, Name: title, VideoName: videoName, LessonID: lessonID, PlayURL: normalizeResourceURLWithBase(href, baseURL), Duration: duration, Index: idx})
	}

	if len(nodes) == 0 {
		x.outline = nil
		return nil
	}
	x.outline = nodes
	return x.outline
}

func (x *ahuCourse) ensureDefaultChapter(nodes *[]*ahuTreeNode) *ahuTreeNode {
	if len(*nodes) > 0 {
		return (*nodes)[len(*nodes)-1]
	}
	title := "默认章节"
	n := treeNode(formatDirTitle(1, title), title, []int{1})
	*nodes = append(*nodes, n)
	return n
}

func (x *ahuCourse) parseCourseFilesTree(body, baseURL string) []*ahuTreeNode {
	var nodes []*ahuTreeNode
	seen := map[string]bool{}
	for _, raw := range extractJSONArrayAssignments(body, "handoutsList") {
		var payload any
		if jsonUnmarshal(raw, &payload) == nil {
			for _, ref := range resourceRefsFromAny(payload, baseURL) {
				resourceURL := normalizeResourceURLWithBase(ref.URL, baseURL)
				x.appendFileToNodes(&nodes, firstNonEmpty(ref.Title, "课程讲义"), resourceURL, seen)
			}
		}
	}
	for _, m := range fileHrefRe.FindAllStringSubmatch(body, -1) {
		href := strings.TrimSpace(html.UnescapeString(m[1]))
		low := strings.ToLower(href)
		if strings.Contains(low, "/pay/buyclass.html") || strings.Contains(low, "/video/videoplay.html") {
			continue
		}
		resourceURL := normalizeResourceURLWithBase(href, baseURL)
		if !isFileURL(resourceURL) {
			continue
		}
		title := firstNonEmpty(cleanText(stripTags(m[2])), fileTitleFromURL(resourceURL), "课程讲义")
		x.appendFileToNodes(&nodes, title, resourceURL, seen)
	}
	return nodes
}

func treeNode(dirTitle, rawTitle string, index []int) *ahuTreeNode {
	return &ahuTreeNode{DirTitle: dirTitle, RawTitle: rawTitle, IndexTuple: append([]int(nil), index...)}
}

func treeMapToDownloadInfo(nodes []*ahuTreeNode) ahuDownloadInfo {
	out := ahuDownloadInfo{}
	for _, node := range sortTreeNodes(nodes) {
		if nodeHasSources(node) {
			out[node.DirTitle] = treeToDownloadInfo(node)
		}
	}
	return out
}

func treeToDownloadInfo(node *ahuTreeNode) any {
	children := map[string]any{}
	for _, child := range sortTreeNodes(node.Children) {
		if nodeHasSources(child) {
			children[child.DirTitle] = treeToDownloadInfo(child)
		}
	}
	if len(node.Sources) > 0 && len(children) > 0 {
		return []any{append([]ahuSource(nil), node.Sources...), children}
	}
	if len(children) > 0 {
		return children
	}
	return append([]ahuSource(nil), node.Sources...)
}

func nodeHasSources(node *ahuTreeNode) bool {
	if node == nil {
		return false
	}
	if len(node.Sources) > 0 {
		return true
	}
	for _, child := range node.Children {
		if nodeHasSources(child) {
			return true
		}
	}
	return false
}

func sortTreeNodes(nodes []*ahuTreeNode) []*ahuTreeNode {
	out := append([]*ahuTreeNode(nil), nodes...)
	sort.SliceStable(out, func(i, j int) bool { return compareIndex(out[i].IndexTuple, out[j].IndexTuple) < 0 })
	return out
}

func compareIndex(a, b []int) int {
	max := len(a)
	if len(b) > max {
		max = len(b)
	}
	for i := 0; i < max; i++ {
		av, bv := 9999, 9999
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

func (x *ahuCourse) findFileTargetOutlineNode(fileTitle string) *ahuTreeNode {
	needle := matchTextVariants(fileTitle)
	var best *ahuTreeNode
	bestDepth, bestLen := 0, 0
	var visit func(*ahuTreeNode)
	visit = func(n *ahuTreeNode) {
		if n == nil {
			return
		}
		matchLen := bestVariantMatchLen(matchTextVariants(n.RawTitle), needle)
		if matchLen > 0 {
			depth := len(n.IndexTuple)
			if depth > bestDepth || (depth == bestDepth && matchLen > bestLen) {
				best, bestDepth, bestLen = n, depth, matchLen
			}
		}
		for _, child := range n.Children {
			visit(child)
		}
	}
	for _, n := range x.outline {
		visit(n)
	}
	return best
}

func (x *ahuCourse) ensureFilePathNode(nodes *[]*ahuTreeNode, outline *ahuTreeNode) *ahuTreeNode {
	if outline == nil || len(outline.IndexTuple) == 0 {
		return x.ensureNamedFileNode(nodes, formatDirTitle(len(*nodes)+1, "课程讲义"), "课程讲义", []int{len(*nodes) + 1})
	}
	path := x.outlinePathToNode(outline)
	if len(path) == 0 {
		path = []*ahuTreeNode{outline}
	}
	current := x.ensureNamedFileNode(nodes, path[0].DirTitle, path[0].RawTitle, path[0].IndexTuple)
	for _, p := range path[1:] {
		current = x.ensureNamedFileNode(&current.Children, p.DirTitle, p.RawTitle, p.IndexTuple)
	}
	if len(current.Children) == 0 && len(outline.Children) > 0 {
		return x.ensureNamedFileNode(&current.Children, formatDirTitle(1, "综合讲义"), "综合讲义", append(append([]int(nil), current.IndexTuple...), 1))
	}
	return current
}

func (x *ahuCourse) outlinePathToNode(target *ahuTreeNode) []*ahuTreeNode {
	var path []*ahuTreeNode
	var visit func(*ahuTreeNode, []*ahuTreeNode) bool
	visit = func(n *ahuTreeNode, parents []*ahuTreeNode) bool {
		if n == nil {
			return false
		}
		next := append(parents, n)
		if n == target || sameIndexTuple(n.IndexTuple, target.IndexTuple) {
			path = append([]*ahuTreeNode(nil), next...)
			return true
		}
		for _, child := range n.Children {
			if visit(child, next) {
				return true
			}
		}
		return false
	}
	for _, root := range x.outline {
		if visit(root, nil) {
			break
		}
	}
	return path
}

func (x *ahuCourse) ensureNamedFileNode(nodes *[]*ahuTreeNode, dirTitle, rawTitle string, index []int) *ahuTreeNode {
	for _, n := range *nodes {
		if n.DirTitle == dirTitle {
			return n
		}
	}
	n := treeNode(dirTitle, rawTitle, index)
	*nodes = append(*nodes, n)
	return n
}

func (x *ahuCourse) appendFileToNodes(nodes *[]*ahuTreeNode, fileTitle, fileURL string, seen map[string]bool) {
	fileURL = normalizeResourceURL(fileURL)
	if fileURL == "" || seen[fileURL] {
		return
	}
	seen[fileURL] = true
	outline := x.findFileTargetOutlineNode(fileTitle)
	node := x.ensureFilePathNode(nodes, outline)
	node.FileCnt++
	idx := append(append([]int(nil), node.IndexTuple...), node.FileCnt)
	node.Sources = append(node.Sources, buildFileInfo(fileTitle, fileURL, idx))
}

func buildFileInfo(fileTitle, fileURL string, index []int) ahuSource {
	fileURL = quoteResourceURL(fileURL)
	fmtName := firstNonEmpty(pickFormat(fileURL), "pdf")
	name := formatFileName(index, firstNonEmpty(fileTitle, fileTitleFromURL(fileURL), "课程资料"))
	if !strings.HasSuffix(strings.ToLower(name), "."+strings.ToLower(fmtName)) {
		name += "." + fmtName
	}
	return ahuSource{Type: "file", Title: name, FileName: name, FileURL: fileURL, FileFmt: fmtName, Index: append([]int(nil), index...)}
}

func normalizeMatchText(text string) string {
	text = regexp.MustCompile(`^第\s*[\d一二三四五六七八九十百千万]+\s*(?:部分|章|节|篇|单元)\s*`).ReplaceAllString(strings.TrimSpace(text), "")
	text = regexp.MustCompile(`[\s\-_（）()【】\[\]《》<>:：、，,。.!！?？/\\]+`).ReplaceAllString(text, "")
	return strings.ToLower(text)
}

func matchTextVariants(text string) []string {
	base := normalizeMatchText(text)
	if base == "" {
		return nil
	}
	set := map[string]bool{base: true, regexp.MustCompile(`[与和及]`).ReplaceAllString(base, ""): true}
	for v := range set {
		if len([]rune(v)) > 4 && (strings.HasPrefix(v, "上") || strings.HasPrefix(v, "下")) {
			set[string([]rune(v)[1:])] = true
		}
		if strings.HasSuffix(v, "外科学") && len([]rune(v)) > 3 {
			r := []rune(v)
			set[string(r[:len(r)-1])] = true
		}
		for _, suffix := range []string{"疾病", "外科", "讲义", "课件", "资料", "笔记", "习题", "试题", "答案"} {
			if strings.HasSuffix(v, suffix) && len([]rune(v)) > len([]rune(suffix))+1 {
				r := []rune(v)
				set[string(r[:len(r)-len([]rune(suffix))])] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		if v != "" {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return len([]rune(out[i])) > len([]rune(out[j])) })
	return out
}

func bestVariantMatchLen(needle, haystack []string) int {
	best := 0
	for _, n := range needle {
		for _, h := range haystack {
			if n != "" && h != "" && strings.Contains(h, n) && len([]rune(n)) > best {
				best = len([]rune(n))
			}
			if n != "" && h != "" && len([]rune(h)) >= 4 && strings.Contains(n, h) && len([]rune(h)) > best {
				best = len([]rune(h))
			}
		}
	}
	return best
}

func formatDirTitle(index int, title string) string {
	return util.SanitizeFilename(fmt.Sprintf("{%d}--%s", index, firstNonEmpty(title, "未命名章节")))
}

func formatVideoName(index []int, title string) string {
	return util.SanitizeFilename(fmt.Sprintf("[%s]--%s", joinInts(index, "."), firstNonEmpty(title, "未命名课时")))
}

func formatFileName(index []int, title string) string {
	return util.SanitizeFilename(fmt.Sprintf("(%s)--%s", joinInts(index, "."), firstNonEmpty(title, "课程资料")))
}

func joinInts(values []int, sep string) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, fmt.Sprint(v))
	}
	return strings.Join(parts, sep)
}

func cleanLessonTitle(text string) string {
	text = cleanText(text)
	text = regexp.MustCompile(`^\d{1,3}:\d{2}(?::\d{2})?\s*`).ReplaceAllString(text, "")
	text = regexp.MustCompile(`^课时\s*\d+\s*`).ReplaceAllString(text, "")
	text = regexp.MustCompile(`(去学习|免费试听)`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`\b\d{1,3}:\d{2}(?::\d{2})?\b`).ReplaceAllString(text, " ")
	text = cleanText(text)
	if text == "" {
		return "未命名课时"
	}
	return text
}

func htmlAttrsHaveClass(attrs string, classes ...string) bool {
	m := regexp.MustCompile(`(?is)\bclass\s*=\s*["']([^"']+)["']`).FindStringSubmatch(attrs)
	if len(m) < 2 {
		return false
	}
	have := map[string]bool{}
	for _, c := range strings.Fields(m[1]) {
		have[c] = true
	}
	for _, c := range classes {
		if have[c] {
			return true
		}
	}
	return false
}

func extractHeaderTitle(inner, def string) string {
	text := firstNonEmpty(firstTagText(inner, "p"), firstTagText(inner, "h1"), firstTagText(inner, "h2"), firstTagText(inner, "h3"), firstTagText(inner, "h4"), firstTagText(inner, "h5"), firstTagText(inner, "span"), stripTags(inner))
	text = regexp.MustCompile(`^第\s*[\d一二三四五六七八九十百千万]+\s*(?:部分|章|节|篇|单元)\s*`).ReplaceAllString(cleanText(text), "")
	if text == "" {
		text = def
	}
	return text
}

func firstTagText(htmlText, tag string) string {
	re := regexp.MustCompile(`(?is)<` + tag + `\b[^>]*>(.*?)</` + tag + `>`)
	if m := re.FindStringSubmatch(htmlText); len(m) > 1 {
		return cleanText(stripTags(m[1]))
	}
	return ""
}

func textForSelector(htmlText, className string) string {
	re := regexp.MustCompile(`(?is)<[^>]*class=["'][^"']*` + regexp.QuoteMeta(className) + `[^"']*["'][^>]*>(.*?)</[^>]+>`)
	if m := re.FindStringSubmatch(htmlText); len(m) > 1 {
		return cleanText(stripTags(m[1]))
	}
	return ""
}

func sameIndexTuple(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func htmlSlice(s string, start, end int) string {
	if start < 0 || end < start || end > len(s) {
		return ""
	}
	return html.UnescapeString(s[start:end])
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func jsonUnmarshal(raw string, v any) error { return json.Unmarshal([]byte(raw), v) }
