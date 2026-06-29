package fenbi

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

func collectEpisodes(v any) []episodeNode {
	seen := map[string]bool{}
	var out []episodeNode
	var walk func(any, string)
	walk = func(x any, title string) {
		x = unwrapData(x)
		switch vv := x.(type) {
		case map[string]any:
			nextTitle := firstNonEmpty(valueString(vv, "title", "name", "episodeTitle", "lessonTitle", "episodeName", "videoName", "video_name", "coursewareName"), title)
			id := valueString(vv, "id", "episodeId", "episode_id", "episode_id_str", "videoId", "video_id", "contentId")
			if id != "" && !seen[id] && isEpisodeMap(vv) {
				seen[id] = true
				out = append(out, episodeNode{ID: id, Title: nextTitle, Raw: vv})
			}
			for _, k := range treeChildKeys() {
				if child, ok := vv[k]; ok {
					walk(child, nextTitle)
				}
			}
		case []any:
			for _, child := range vv {
				walk(child, title)
			}
		}
	}
	walk(v, "")
	return out
}

func isEpisodeMap(m map[string]any) bool {
	if hasAny(m, "episodeId", "episode_id", "episode_id_str", "videoId", "video_id") {
		return true
	}
	if hasAny(m, "mediafile", "mediaFile", "duration", "mediaDuration", "bizType", "biz_type") {
		return true
	}
	kind := strings.ToLower(firstNonEmpty(valueString(m, "content_kind", "kind"), valueString(m, "nodeType", "node_type", "type", "contentType", "content_type")))
	return strings.Contains(kind, "episode") || strings.Contains(kind, "video") || strings.Contains(kind, "live") || strings.Contains(kind, "review")
}

func treeChildKeys() []string {
	return []string{
		"syllabus", "children", "subContent", "subContents", "contents", "contentList",
		"groups", "groupList", "tabs", "tabList", "columns", "columnList",
		"subjects", "subjectList", "lectureSets", "lectureSetList",
		"episodeSets", "episodeSetList", "sets", "setList", "chapters", "chapterList",
		"units", "unitList", "modules", "moduleList", "sections", "sectionList",
		"catalogs", "catalogList", "lessons", "lessonList", "tasks", "taskList",
		"episodes", "episodeList", "episodeNodes", "nodes", "coursewares",
		"coursewareList", "items", "list", "data",
	}
}

func findMediaURL(v any) string {
	u, _, _ := pickVideoURLFromMeta(v)
	return u
}

func pickVideoURLFromMeta(v any) (string, int64, map[string]any) {
	candidates := collectVideoCandidates(v, "")
	if len(candidates) == 0 {
		return "", 0, nil
	}
	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.Rank > best.Rank ||
			(candidate.Rank == best.Rank && candidate.Size > best.Size) ||
			(candidate.Rank == best.Rank && candidate.Size == best.Size && strings.Contains(strings.ToLower(candidate.URL), "vod.fbstatic.cn")) {
			best = candidate
		}
	}
	return best.URL, best.Size, best.Raw
}

func collectVideoCandidates(v any, source string) []fenbiVideoCandidate {
	v = unwrapData(v)
	switch x := v.(type) {
	case string:
		if s := normalizeURL(x); isMediaURL(s) {
			return []fenbiVideoCandidate{{URL: s, Rank: videoQualityRank(map[string]any{"url": s}), Source: source}}
		}
	case []any:
		var out []fenbiVideoCandidate
		for _, child := range x {
			out = append(out, collectVideoCandidates(child, source)...)
		}
		return dedupeVideoCandidates(out)
	case map[string]any:
		var out []fenbiVideoCandidate
		for _, k := range []string{"url", "mediaUrl", "media_url", "downloadUrl", "download_url", "path", "playUrl", "m3u8"} {
			if s := normalizeURL(anyString(x[k])); isMediaURL(s) {
				raw := cloneMapForExtra(x)
				raw["url"] = s
				out = append(out, fenbiVideoCandidate{
					URL:    s,
					Size:   normalizeSizeBytes(firstAny(raw, "size", "fileSize", "mediaSize")),
					Rank:   videoQualityRank(raw),
					Raw:    raw,
					Source: firstNonEmpty(source, k),
				})
			}
		}
		for _, k := range []string{"mediaFiles", "qualities", "mediaList", "mediaSizes", "streamList", "videoList", "definitions", "urls", "files", "list", "streams", "data"} {
			if child, ok := x[k]; ok {
				out = append(out, collectVideoCandidates(child, k)...)
			}
		}
		if len(out) == 0 {
			for k, child := range x {
				out = append(out, collectVideoCandidates(child, k)...)
			}
		}
		return dedupeVideoCandidates(out)
	}
	return nil
}

func dedupeVideoCandidates(values []fenbiVideoCandidate) []fenbiVideoCandidate {
	seen := map[string]bool{}
	var out []fenbiVideoCandidate
	for _, item := range values {
		if item.URL == "" || seen[item.URL] {
			continue
		}
		seen[item.URL] = true
		out = append(out, item)
	}
	return out
}

func videoQualityRank(m map[string]any) int {
	if m == nil {
		return 0
	}
	var parts []string
	for _, key := range []string{"quality", "qualityType", "qualityCode", "qualityDesc", "qualityName", "definition", "definitionId", "definitionType", "definitionName", "clarity", "format", "label", "name", "desc", "type", "typeName", "streamType", "url"} {
		if value := anyString(m[key]); value != "" {
			parts = append(parts, value)
		}
	}
	text := strings.Join(parts, " ")
	low := strings.ToLower(text)
	rank := 0
	switch {
	case strings.Contains(low, "4k") || strings.Contains(low, "2160") || strings.Contains(low, "uhd") || strings.Contains(text, "原画") || strings.Contains(text, "蓝光"):
		rank = 2160
	case strings.Contains(low, "1080") || strings.Contains(low, "fhd") || strings.Contains(low, "fullhd") || strings.Contains(text, "超清"):
		rank = 1080
	case regexp.MustCompile(`(?i)(^|[^a-z])hd([^a-z]|$)`).MatchString(low) || strings.Contains(low, "720") || strings.Contains(text, "高清"):
		rank = 720
	case regexp.MustCompile(`(?i)(^|[^a-z])sd([^a-z]|$)`).MatchString(low) || strings.Contains(low, "480") || strings.Contains(low, "360") || strings.Contains(text, "标清") || strings.Contains(text, "流畅"):
		rank = 480
	}
	if m2 := regexp.MustCompile(`(\d{3,4})\s*[xX*]\s*(\d{3,4})`).FindStringSubmatch(text); len(m2) > 2 {
		rank = maxInt(rank, toInt(m2[1], 0), toInt(m2[2], 0))
	}
	for _, m2 := range regexp.MustCompile(`(?i)(?<!\d)([1-4]\d{2,3})(?:p)?(?!\d)`).FindAllStringSubmatch(text, -1) {
		if len(m2) > 1 {
			rank = maxInt(rank, toInt(m2[1], 0))
		}
	}
	rank = maxInt(rank, toInt(firstAny(m, "height", "videoHeight", "h"), 0))
	rank = rank*1_000_000 + toInt(firstAny(m, "width", "videoWidth", "w"), 0)*1_000 + toInt(firstAny(m, "bitrate", "bitRate", "videoBitrate", "kbps"), 0)
	return rank
}

func maxInt(values ...int) int {
	maxValue := 0
	for _, value := range values {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

func normalizeSizeBytes(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	}
	s := anyString(v)
	if s == "" {
		return 0
	}
	re := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(gb|g|mb|m|kb|k|bytes?|b)?`)
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return 0
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil || f <= 0 {
		return 0
	}
	unit := ""
	if len(m) > 2 {
		unit = strings.ToLower(m[2])
	}
	switch unit {
	case "gb", "g":
		f *= 1024 * 1024 * 1024
	case "mb", "m":
		f *= 1024 * 1024
	case "kb", "k":
		f *= 1024
	default:
		if f > 0 && f < 1024*1024 {
			// Source normalizes ambiguous small numeric values as MB.
			f *= 1024 * 1024
		}
	}
	return int64(f)
}

func cloneMapForExtra(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func pickTitle(v any) string {
	v = unwrapData(v)
	switch x := v.(type) {
	case map[string]any:
		if s := valueString(x, "courseTitle", "lectureTitle", "lectureSetTitle", "title", "name", "episodeTitle", "episodeName", "videoName"); s != "" {
			return s
		}
		for _, child := range x {
			if s := pickTitle(child); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range x {
			if s := pickTitle(child); s != "" {
				return s
			}
		}
	}
	return ""
}

func mediaInfo(title, mediaURL string, size int64, headers map[string]string) *extractor.MediaInfo {
	format := "mp4"
	if strings.Contains(strings.ToLower(mediaURL), ".m3u8") || strings.HasPrefix(strings.ToLower(mediaURL), "data:application/vnd.apple.mpegurl") {
		format = "m3u8"
	}
	stream := extractor.Stream{Quality: "best", URLs: []string{mediaURL}, Format: format, Size: size, Headers: headers}
	if format == "m3u8" {
		stream.NeedMerge = true
	}
	return &extractor.MediaInfo{Site: "fenbi", Title: util.SanitizeFilename(title), Streams: map[string]extractor.Stream{"best": stream}}
}

func withFenbiCookieHeader(jar http.CookieJar, base map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	cookie := fenbiCookieHeader(jar,
		"https://pc.fenbi.com/",
		"https://ke.fenbi.com/",
		"https://live.fenbi.com/",
		"https://login.fenbi.com/",
		referer,
	)
	if cookie != "" {
		out["Cookie"] = cookie
		out["cookie"] = cookie
	}
	return out
}

func fenbiCookieHeader(jar http.CookieJar, rawURLs ...string) string {
	if jar == nil {
		return ""
	}
	seen := map[string]bool{}
	var parts []string
	for _, raw := range rawURLs {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		for _, ck := range jar.Cookies(u) {
			key := ck.Name + "=" + ck.Value
			if key == "=" || seen[key] {
				continue
			}
			seen[key] = true
			parts = append(parts, key)
		}
	}
	return strings.Join(parts, "; ")
}

func valueString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := valueStringAny(m[k]); s != "" {
			return s
		}
	}
	return ""
}

func hasAny(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func normalizeURL(s string) string {
	s = strings.TrimSpace(strings.Trim(s, `"'`))
	s = strings.ReplaceAll(s, `\/`, `/`)
	if strings.HasPrefix(s, "//") {
		return "https:" + s
	}
	return s
}

func unwrapData(v any) any {
	for {
		m, ok := v.(map[string]any)
		if !ok {
			return v
		}
		code := anyString(m["code"])
		if code != "" && code != "0" && code != "1" && !strings.EqualFold(code, "true") {
			return map[string]any{}
		}
		child, ok := m["data"]
		if !ok {
			return v
		}
		switch child.(type) {
		case map[string]any, []any, string:
			v = child
		default:
			return v
		}
	}
}

func listMaps(v any, keys ...string) []map[string]any {
	v = unwrapData(v)
	switch x := v.(type) {
	case []any:
		return mapsFromList(x)
	case map[string]any:
		for _, key := range keys {
			if rows, ok := x[key]; ok {
				if out := listMaps(rows); len(out) > 0 {
					return out
				}
			}
		}
		for _, key := range []string{"list", "items", "data", "datas", "lectures", "lectureList", "materials", "materialList"} {
			if rows, ok := x[key]; ok {
				if out := listMaps(rows); len(out) > 0 {
					return out
				}
			}
		}
	}
	return nil
}

func mapsFromList(rows []any) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if m, ok := unwrapData(row).(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func firstAny(v any, keys ...string) any {
	if m, ok := unwrapData(v).(map[string]any); ok {
		for _, key := range keys {
			if value, ok := m[key]; ok && anyString(value) != "" {
				return value
			}
		}
	}
	return nil
}

func toInt(v any, fallback int) int {
	switch x := v.(type) {
	case nil:
		return fallback
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case jsonNumber:
		i, err := strconv.Atoi(x.String())
		if err == nil {
			return i
		}
	}
	s := anyString(v)
	if s == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fallback
	}
	return int(f)
}

type jsonNumber interface{ String() string }

func mergeVideoInfo(dst map[string]any, src any) {
	switch x := unwrapData(src).(type) {
	case map[string]any:
		for _, key := range []string{"prefix", "episode_id", "episodeId", "lecture_id", "lectureId", "content_id", "contentId", "biz_type", "bizType", "biz_id", "bizId", "material_id", "materialId", "note_material_id", "noteMaterialId"} {
			if _, exists := dst[key]; exists {
				continue
			}
			if value, ok := x[key]; ok && anyString(value) != "" {
				dst[key] = value
			}
		}
		for _, key := range []string{"title", "name", "episodeTitle", "lectureTitle", "courseTitle", "supportReplay", "support_replay", "replayDataVersion", "replay_data_version", "hasVideo", "playStatus", "play_status"} {
			if _, exists := dst[key]; exists {
				continue
			}
			if value, ok := x[key]; ok && anyString(value) != "" {
				dst[key] = value
			}
		}
		for _, key := range []string{"episode", "episodeInfo", "mediafile", "mediaFile", "video", "data", "detail"} {
			if child, ok := x[key]; ok {
				mergeVideoInfo(dst, child)
			}
		}
	case []any:
		for _, child := range x {
			mergeVideoInfo(dst, child)
		}
	}
}

func infoString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			if s := anyString(value); s != "" {
				return s
			}
		}
	}
	return ""
}

func collectMaterialCandidates(values ...any) []map[string]any {
	var out []map[string]any
	seen := map[string]bool{}
	var walk func(any, bool)
	walk = func(v any, inMaterialList bool) {
		v = unwrapData(v)
		switch x := v.(type) {
		case map[string]any:
			if isMaterialMap(x, inMaterialList) {
				addMaterialCandidate(&out, seen, x)
			}
			for _, key := range []string{"materials", "materialList", "material_list", "courseMaterials", "courseMaterialList", "handouts", "handoutList", "attachments", "attachmentList", "coursewareList", "datas", "data", "list", "items"} {
				if child, ok := x[key]; ok {
					lowKey := strings.ToLower(key)
					walk(child, strings.Contains(lowKey, "material") || strings.Contains(lowKey, "handout") || strings.Contains(lowKey, "attachment") || strings.Contains(lowKey, "courseware"))
				}
			}
			for _, key := range []string{"material_id", "materialId", "note_material_id", "noteMaterialId"} {
				if valueString(x, key) != "" {
					addMaterialCandidate(&out, seen, map[string]any{key: valueString(x, key), "name": materialDefaultName(key)})
				}
			}
		case []any:
			for _, child := range x {
				walk(child, inMaterialList)
			}
		}
	}
	for _, value := range values {
		walk(value, false)
	}
	return out
}

func isMaterialMap(m map[string]any, inMaterialList bool) bool {
	if hasAny(m, "materialId", "material_id", "noteMaterialId", "note_material_id", "fileId", "file_id") {
		return true
	}
	if pickURLFromResponse(m) != "" && (inMaterialList || hasAny(m, "fileName", "filename", "materialName", "coursewareName", "typeName", "fileType", "ext")) {
		return true
	}
	return false
}

func addMaterialCandidate(out *[]map[string]any, seen map[string]bool, m map[string]any) {
	key := firstNonEmpty(valueString(m, "materialId", "id", "material_id", "fileId", "file_id", "noteMaterialId", "note_material_id"), pickURLFromResponse(m))
	if key == "" {
		key = fmt.Sprintf("%p", m)
	}
	if seen[key] {
		return
	}
	seen[key] = true
	copyMap := map[string]any{}
	for k, v := range m {
		copyMap[k] = v
	}
	*out = append(*out, copyMap)
}

func materialDefaultName(key string) string {
	if strings.Contains(strings.ToLower(key), "note") {
		return "笔记解析"
	}
	return "讲义"
}

func pickURLFromResponse(v any) string {
	v = unwrapData(v)
	switch x := v.(type) {
	case string:
		return normalizeURL(x)
	case []any:
		candidates := make([]string, 0, len(x))
		for _, child := range x {
			if u := pickURLFromResponse(child); u != "" {
				candidates = append(candidates, u)
			}
		}
		return bestMaterialURL(candidates)
	case map[string]any:
		var candidates []string
		for _, key := range []string{"url", "path", "downloadUrl", "download_url", "fileUrl", "file_url", "sourceUrl", "source_url"} {
			if s := normalizeURL(anyString(x[key])); s != "" {
				candidates = append(candidates, s)
			}
		}
		for _, key := range []string{"urls", "urlList", "paths", "files", "list", "data"} {
			if child, ok := x[key]; ok {
				if u := pickURLFromResponse(child); u != "" {
					candidates = append(candidates, u)
				}
			}
		}
		return bestMaterialURL(candidates)
	default:
		return ""
	}
}

func bestMaterialURL(candidates []string) string {
	var first, firstNonImage string
	for _, candidate := range candidates {
		candidate = normalizeURL(candidate)
		if candidate == "" {
			continue
		}
		if first == "" {
			first = candidate
		}
		lower := strings.ToLower(candidate)
		if strings.Contains(lower, ".pdf") {
			return candidate
		}
		if firstNonImage == "" && !isImageExt(fileExt(candidate)) {
			firstNonImage = candidate
		}
	}
	return firstNonEmpty(firstNonImage, first)
}

func materialName(m map[string]any) string {
	for _, candidate := range []string{
		valueString(m, "name", "title", "materialName", "material_name", "fileName", "filename", "file_name", "coursewareName", "typeName"),
		valueString(m, "materialId", "id", "material_id", "fileId", "file_id", "keynoteId", "keynote_id", "noteMaterialId", "note_material_id"),
		pickURLFromResponse(m),
	} {
		base, _ := filenameParts(candidate)
		if base != "" && !isGenericMaterialName(base) && !isMaterialIDName(base) {
			return base
		}
		cleaned := cleanFileTitle(candidate)
		if cleaned != "" && !isGenericMaterialName(cleaned) && !isMaterialIDName(cleaned) {
			return cleaned
		}
	}
	return "课件"
}

func fileExt(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "bin"
	}
	if strings.Contains(raw, "/") || strings.Contains(raw, ".") {
		if u, err := url.Parse(raw); err == nil {
			if ext := strings.TrimPrefix(strings.ToLower(path.Ext(u.Path)), "."); ext != "" {
				return ext
			}
		}
		if ext := strings.TrimPrefix(strings.ToLower(path.Ext(raw)), "."); ext != "" {
			return ext
		}
	}
	raw = strings.TrimPrefix(strings.ToLower(raw), ".")
	if raw == "" {
		return "bin"
	}
	return raw
}

func filenameParts(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	if parsed, err := url.Parse(raw); err == nil && (parsed.Scheme != "" || parsed.Host != "") {
		q := parsed.Query()
		for _, key := range []string{"filename", "fileName", "downloadFileName", "attname", "name"} {
			if values, ok := q[key]; ok {
				for _, value := range values {
					if base, ext := filenameParts(value); base != "" {
						return base, ext
					}
				}
			}
		}
		raw = parsed.Path
	}
	raw, _ = url.QueryUnescape(raw)
	raw = strings.ReplaceAll(raw, "\\", "/")
	raw = strings.TrimSpace(path.Base(strings.Split(strings.Split(raw, "?")[0], "#")[0]))
	if raw == "" || raw == "." || raw == ".." {
		return "", ""
	}
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(raw)), ".")
	base := raw
	if ext != "" {
		base = strings.TrimSuffix(raw, "."+ext)
	}
	return cleanFileTitle(base), ext
}

func cleanFileTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	return strings.TrimSpace(regexp.MustCompile(`[?*|<>:"/⁄\\\s]`).ReplaceAllString(title, ""))
}

func isGenericMaterialName(name string) bool {
	name = strings.ToLower(cleanFileTitle(name))
	name = strings.Trim(name, "()[]{}（）【】「」《》")
	name = regexp.MustCompile(`[\s_\-—–]+`).ReplaceAllString(name, "")
	if name == "" || regexp.MustCompile(`^\d+$`).MatchString(name) || regexp.MustCompile(`^[0-9a-f]{16,}$`).MatchString(name) {
		return true
	}
	switch name {
	case "资料", "讲义", "笔记", "课件", "附件", "文件", "教材", "文档", "pdf", "ppt", "pptx", "doc", "docx", "document", "file", "material":
		return true
	}
	return regexp.MustCompile(`^(资料|讲义|笔记|课件|附件|文件|教材|文档)\d*$`).MatchString(name)
}

func isMaterialIDName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	name = regexp.MustCompile(`\.[a-zA-Z0-9]{1,8}$`).ReplaceAllString(name, "")
	return regexp.MustCompile(`^[0-9a-f]{12,}$`).MatchString(name) || regexp.MustCompile(`^\d{12,}$`).MatchString(name)
}

func isImageExt(ext string) bool {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "jpg", "jpeg", "png", "gif", "webp", "bmp", "svg", "ico", "heic", "heif":
		return true
	default:
		return false
	}
}

func isMediaURL(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(low, "http") && (strings.Contains(low, ".m3u8") || strings.Contains(low, ".mp4") || strings.Contains(low, ".flv") || strings.Contains(low, ".mov") || strings.Contains(low, ".m4v") || strings.Contains(low, ".mp3") || strings.Contains(low, ".m4a") || strings.Contains(low, ".aac") || strings.Contains(low, ".wav"))
}

func anyString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case fmt.Stringer:
		return strings.TrimSpace(x.String())
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		s := strings.TrimSpace(fmt.Sprint(x))
		if s == "<nil>" {
			return ""
		}
		return s
	}
}

func valueStringAny(v any) string {
	s := anyString(v)
	if s == "" || s == "0" || s == "<nil>" {
		return ""
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func extractPrice(info any) float64 {
	switch m := unwrapData(info).(type) {
	case map[string]any:
		for _, key := range []string{"payPrice", "promotionPrice", "price", "floorPrice", "topPrice", "originPrice", "salePrice"} {
			if price := normalizePriceValue(m[key]); price > 0 {
				return price
			}
		}
	}
	return 0
}

func extractPurchased(info any, fallback bool) bool {
	if m, ok := unwrapData(info).(map[string]any); ok {
		for _, key := range []string{"paid", "purchased", "hasBought", "hasPurchased", "bought", "joined"} {
			if value, exists := m[key]; exists {
				return truthy(value)
			}
		}
	}
	return fallback
}

func normalizePriceValue(value any) float64 {
	switch x := value.(type) {
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case float64:
		if x > 5000 && x == float64(int64(x)) {
			return x / 100
		}
		return x
	}
	s := strings.TrimSpace(anyString(value))
	if s == "" {
		return 0
	}
	m := regexp.MustCompile(`\d+(?:\.\d+)?`).FindString(s)
	if m == "" {
		return 0
	}
	f, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return 0
	}
	if f > 5000 && !strings.Contains(m, ".") {
		f /= 100
	}
	return f
}

func truthy(value any) bool {
	switch x := value.(type) {
	case bool:
		return x
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	}
	switch strings.ToLower(strings.TrimSpace(anyString(value))) {
	case "", "0", "false", "no", "null", "nil", "未购买", "未开通":
		return false
	default:
		return true
	}
}

func applyPaymentExtra(extra map[string]any, info any) {
	if extra == nil {
		return
	}
	if price := extractPrice(info); price > 0 {
		extra["price"] = price
	}
	if m, ok := unwrapData(info).(map[string]any); ok {
		for _, key := range []string{"paid", "purchased", "hasBought", "hasPurchased", "bought", "joined"} {
			if _, exists := m[key]; exists {
				extra["purchased"] = extractPurchased(m, true)
				return
			}
		}
	}
}

// collectEpisodeSetIDs walks a payload tree and collects all episode set IDs
// into the seen map, so we can avoid re-fetching sets we already have.
func collectEpisodeSetIDs(v any, seen map[string]bool) {
	v = unwrapData(v)
	switch x := v.(type) {
	case map[string]any:
		if id := valueString(x, "episodeSetId", "episode_set_id"); id != "" {
			seen[id] = true
		}
		for _, child := range x {
			collectEpisodeSetIDs(child, seen)
		}
	case []any:
		for _, child := range x {
			collectEpisodeSetIDs(child, seen)
		}
	}
}

// extractSummaryEpisodeSetIDs extracts root-level episode set IDs from
// the lecture summary's "episodeSets" array. Only returns sets that are
// root sets (not child sets). Mirrors source _summary_episode_set_entries
// (Fenbi_Course line 1135).
func extractSummaryEpisodeSetIDs(v any) []string {
	v = unwrapData(v)
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	// Look in the summary payload for episodeSets.
	setsRaw, ok := m["episodeSets"]
	if !ok {
		return nil
	}
	setsList, ok := setsRaw.([]any)
	if !ok {
		return nil
	}
	var ids []string
	for _, item := range setsList {
		setMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		// Skip non-root sets: source checks for "root" key or parentEpisodeSetId.
		if rootVal, hasRoot := setMap["root"]; hasRoot {
			if rootVal == false || anyString(rootVal) == "false" || anyString(rootVal) == "0" {
				continue
			}
		}
		parentID := firstNonEmpty(anyString(setMap["parentEpisodeSetId"]), anyString(setMap["parent_episode_set_id"]))
		if parentID != "" && parentID != "0" {
			continue
		}
		setID := firstNonEmpty(
			valueString(setMap, "episodeSetId", "episode_set_id", "setId", "set_id", "id"),
		)
		if setID != "" {
			ids = append(ids, setID)
		}
	}
	return ids
}
