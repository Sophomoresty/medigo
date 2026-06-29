package plaso

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
)

func plasoUserAgent() string {
	return "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) plaso_client/1.07.123 Chrome/91.0.4472.164 Electron/12.0.18 Safari/537.36"
}

func parseCID(rawURL string) string {
	if m := cidRe.FindStringSubmatch(rawURL); len(m) == 2 {
		return m[1]
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	q := u.Query()
	if id := firstNonEmpty(q.Get("sfId"), q.Get("sfid"), q.Get("shareKey"), q.Get("fileId"), q.Get("fid"), q.Get("id"), q.Get("packageId"), q.Get("courseId"), q.Get("groupId"), q.Get("fileGroupId"), q.Get("dirId")); id != "" {
		return id
	}
	parts := strings.FieldsFunc(u.EscapedPath(), func(r rune) bool { return r == '/' || r == '#' })
	for i := len(parts) - 1; i >= 0; i-- {
		p, _ := url.PathUnescape(parts[i])
		p = strings.TrimSpace(p)
		if p != "" && p != "course" && p != "detail" && p != "share" && p != "file" {
			return p
		}
	}
	return ""
}

func splitPlasoCourseID(id string) (string, string) {
	id = strings.TrimSpace(id)
	switch {
	case strings.HasPrefix(id, "history_"):
		return "history", strings.TrimPrefix(id, "history_")
	case strings.HasPrefix(id, "homework_"):
		return "homework", strings.TrimPrefix(id, "homework_")
	default:
		return "", id
	}
}

func collectFileItems(v any) []fileItem {
	var out []fileItem
	var visit func(any, []string, []int)
	visit = func(x any, chapters []string, index []int) {
		switch t := x.(type) {
		case map[string]any:
			item := buildFileItem(t)
			containerOnly := hasChildContainers(t) && !hasDirectMediaSignal(t)
			if isFileLikeMap(t) && itemHasSignal(item) && !containerOnly {
				item.Chapter = strings.Join(chapters, " / ")
				item.Index = append([]int(nil), index...)
				out = append(out, item)
			}
			nextChapters := chapters
			if containerOnly || (hasChildContainers(t) && !isFileLikeMap(t)) {
				if title := chapterTitle(t); title != "" {
					nextChapters = append(append([]string(nil), chapters...), title)
				}
			}
			seenKeys := map[string]bool{}
			childNo := 0
			for _, key := range plasoChildKeys() {
				children := asAnyList(t[key])
				if len(children) == 0 {
					continue
				}
				seenKeys[key] = true
				for _, child := range children {
					childNo++
					visit(child, nextChapters, appendIndex(index, childNo))
				}
			}
			for key, child := range t {
				if seenKeys[key] {
					continue
				}
				switch child.(type) {
				case map[string]any, []any, []map[string]any:
					visit(child, nextChapters, index)
				}
			}
		case []any:
			for i, child := range t {
				visit(child, chapters, appendIndex(index, i+1))
			}
		case []map[string]any:
			for i, child := range t {
				visit(child, chapters, appendIndex(index, i+1))
			}
		}
	}
	visit(v, nil, nil)
	return dedupeFiles(out)
}

func buildFileItem(m map[string]any) fileItem {
	item := fileItem{
		ID:           firstText(m, "fileId", "file_id", "xFileId", "x_file_id", "id", "originId", "origin_id", "_id", "fid", "sfId", "sfid", "shareKey"),
		MyID:         firstText(m, "myid", "myId", "my_id", "myID"),
		Location:     firstText(m, "location", "ossLocation", "objectKey", "key"),
		LocationPath: firstText(m, "locationPath", "location_path", "filePath", "pathName", "objectPath", "object_path"),
		Name:         firstText(m, "name", "title", "file_name", "fileName", "taskName", "resourceName", "materialName"),
		Type:         strings.ToLower(firstText(m, "type", "file_type", "fileType", "sourceType", "resourceType", "suffix", "fileSuffix", "format")),
		URL: firstText(m,
			"url", "URL", "file_url", "fileUrl", "downloadUrl", "download_url", "previewUrl", "preview_url",
			"playUrl", "playURL", "PlayURL", "HDPlayURL", "SDPlayURL", "LDPlayURL", "m3u8Url", "m3u8URL", "m3u8_url",
			"hdPlayUrl", "hdPlayURL", "sdPlayUrl", "sdPlayURL", "ldPlayUrl", "ldPlayURL",
			"sourceUrl", "sourceURL", "resourceUrl", "resourceURL", "mediaUrl", "mediaURL", "path", "downloadPath"),
		Vid:       firstText(m, "vid", "polyvVid", "polyv_vid", "polyVid", "videoPoolId", "video_pool_id"),
		VideoID:   firstText(m, "videoId", "videoID", "aliyunVid", "aliyunVideoId", "aliVideoId", "video_id", "videoIdAli"),
		StorageID: firstText(m, "storageId", "storageID", "storage_id", "storage"),
		Size:      parseSize(firstValue(m, "size", "fileSize", "file_size", "length", "contentLength")),
		Raw:       m,
	}
	if item.LocationPath == item.URL && !looksDownloadable(item.URL) {
		item.URL = ""
	}
	item.URL = strings.TrimSpace(item.URL)
	return item
}

func itemHasSignal(f fileItem) bool {
	return firstNonEmpty(f.URL, f.Location, f.LocationPath, f.Vid, f.VideoID) != "" ||
		(f.ID != "" && (f.MyID != "" || f.Type != "" || (f.Name != "" && isFileLikeMap(f.Raw))))
}

func hasChildContainers(m map[string]any) bool {
	for _, key := range plasoChildKeys() {
		if len(asAnyList(m[key])) > 0 {
			return true
		}
	}
	return false
}

func hasDirectMediaSignal(m map[string]any) bool {
	return hasAnyKey(m,
		"location", "locationPath", "location_path", "filePath", "pathName",
		"url", "URL", "file_url", "fileUrl", "downloadUrl", "previewUrl", "playUrl", "PlayURL",
		"m3u8Url", "m3u8URL", "mediaUrl", "resourceUrl", "path", "downloadPath", "objectKey", "key",
		"vid", "polyvVid", "videoId", "videoID", "aliyunVid", "storageId", "storage_id")
}

func plasoChildKeys() []string {
	return []string{
		"courseChapterList", "chapterList", "chapters", "chapter",
		"children", "child", "dirs", "dirList", "directories",
		"taskList", "tasks", "taskInfoList", "records", "recordList",
		"files", "fileList", "fileInfos", "resourceList", "resources", "materials",
		"zuoyes", "homeworks", "list", "rows", "data", "obj",
	}
}

func chapterTitle(m map[string]any) string {
	title := firstText(m,
		"chapterName", "chapter_name", "dirName", "dir_name", "directoryName",
		"sectionName", "sectionTitle", "catalogName", "catalogTitle", "groupName",
		"title", "name")
	if title == "" {
		return ""
	}
	return clean(title)
}

func isFileLikeMap(m map[string]any) bool {
	if len(m) == 0 {
		return false
	}
	if hasAnyKey(m,
		"fileId", "file_id", "xFileId", "x_file_id", "originId", "origin_id", "_id", "fid", "sfId", "sfid", "shareKey",
		"myid", "myId", "my_id", "location", "locationPath", "location_path", "filePath", "pathName",
		"url", "URL", "file_url", "fileUrl", "downloadUrl", "previewUrl", "playUrl", "PlayURL",
		"m3u8Url", "m3u8URL", "mediaUrl", "resourceUrl", "path", "downloadPath", "objectKey", "key",
		"vid", "polyvVid", "videoId", "videoID", "aliyunVid", "storageId", "storage_id") {
		return true
	}
	typ := strings.ToLower(firstText(m, "type", "file_type", "fileType", "sourceType", "resourceType", "suffix", "fileSuffix", "format"))
	switch typ {
	case "2", "3", "4", "7", "8", "20", "video", "audio", "live", "file", "material", "mp4", "m3u8", "mp3", "pdf", "ppt", "pptx", "doc", "docx", "xls", "xlsx":
		return firstText(m, "name", "title", "fileName", "taskName", "resourceName") != ""
	}
	return false
}

func hasAnyKey(m map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := m[key]; ok {
			return true
		}
	}
	return false
}

func appendIndex(index []int, n int) []int {
	out := append([]int(nil), index...)
	out = append(out, n)
	return out
}

func pickPlayURL(v any, quality string) (string, string) {
	order := plasoQualityOrder(quality)
	var generic string
	var genericQuality string
	var found string
	var foundQuality string
	walk(v, func(m map[string]any) {
		if found != "" {
			return
		}
		if playURLs := asAnyMap(m["playUrls"]); len(playURLs) > 0 {
			if u, q := pickFromQualityMap(playURLs, order); u != "" {
				found, foundQuality = u, q
				return
			}
		}
		for _, q := range order {
			keys := []string{q + "PlayUrl", q + "PlayURL", strings.ToUpper(q) + "PlayUrl", strings.ToUpper(q) + "PlayURL", q + "Url", q + "URL", q}
			if u := firstText(m, keys...); u != "" {
				found, foundQuality = u, q
				return
			}
		}
		if generic == "" {
			generic = firstText(m, "PlayURL", "PlayUrl", "playURL", "playUrl", "m3u8Url", "m3u8URL", "url", "URL", "sourceUrl", "mediaUrl", "path", "location")
			genericQuality = firstText(m, "Definition", "definition", "quality", "level")
		}
	})
	if found != "" {
		return found, foundQuality
	}
	return generic, genericQuality
}

func pickFromQualityMap(m map[string]any, order []string) (string, string) {
	for _, q := range order {
		for _, key := range []string{q, strings.ToUpper(q), strings.ToLower(q), q + "Url", q + "URL"} {
			if v, ok := m[key]; ok {
				if mm := asAnyMap(v); len(mm) > 0 {
					if u := firstText(mm, "url", "URL", "playUrl", "PlayURL", "m3u8Url"); u != "" {
						return u, q
					}
				}
				if s := valueText(v); s != "" {
					return s, q
				}
			}
		}
	}
	return "", ""
}

func plasoQualityOrder(quality string) []string {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "ld", "sd", "low":
		return []string{"ld", "sd", "hd", "od", "shd"}
	case "hd":
		return []string{"hd", "sd", "ld", "od", "shd"}
	case "od", "shd", "fhd", "best":
		return []string{"od", "shd", "hd", "sd", "ld"}
	default:
		return []string{"od", "hd", "sd", "ld", "shd"}
	}
}

func aliyunPreferDefinitions(quality string) []string {
	out := make([]string, 0, len(plasoQualityOrder(quality))+3)
	for _, q := range plasoQualityOrder(quality) {
		out = append(out, strings.ToUpper(q))
	}
	out = append(out, "FD", "2K", "4K")
	return out
}

func walk(v any, fn func(map[string]any)) {
	switch t := v.(type) {
	case map[string]any:
		fn(t)
		for _, x := range t {
			walk(x, fn)
		}
	case []any:
		for _, x := range t {
			walk(x, fn)
		}
	}
}

func extractNamedList(v any, keys ...string) []any {
	var out []any
	walk(v, func(m map[string]any) {
		if len(out) > 0 {
			return
		}
		for _, key := range keys {
			if list := asAnyList(m[key]); len(list) > 0 {
				out = list
				return
			}
		}
	})
	if len(out) > 0 {
		return out
	}
	if list := asAnyList(v); len(list) > 0 {
		return list
	}
	return nil
}

func asAnyMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func asAnyList(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case []map[string]any:
		out := make([]any, 0, len(t))
		for _, x := range t {
			out = append(out, x)
		}
		return out
	case map[string]any:
		return []any{t}
	default:
		return nil
	}
}

func firstText(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s := valueText(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func firstValue(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

func valueText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return strings.TrimSpace(t.String())
	case map[string]any, []any, []map[string]any:
		return ""
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return strings.TrimSpace(fmt.Sprint(t))
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func findFirst(v any, keys ...string) string {
	out := ""
	walk(v, func(m map[string]any) {
		if out == "" {
			out = firstText(m, keys...)
		}
	})
	return out
}

func findFirstValue(v any, keys ...string) any {
	var out any
	walk(v, func(m map[string]any) {
		if out == nil {
			out = firstValue(m, keys...)
		}
	})
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstPositive(vals ...int64) int64 {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}

func cloneStringMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func needsFileDetail(f fileItem) bool {
	if firstNonEmpty(f.URL, f.Location, f.LocationPath, f.Vid, f.VideoID) == "" {
		return true
	}
	return f.Name == "" || f.Type == "" || (isDocumentLike(f) && f.StorageID == "")
}

func mergeFileItem(base, detail fileItem) fileItem {
	out := base
	if out.ID == "" {
		out.ID = detail.ID
	}
	if out.MyID == "" {
		out.MyID = detail.MyID
	}
	if out.Location == "" {
		out.Location = detail.Location
	}
	if out.LocationPath == "" {
		out.LocationPath = detail.LocationPath
	}
	if out.Name == "" {
		out.Name = detail.Name
	}
	if out.Type == "" {
		out.Type = detail.Type
	}
	if out.URL == "" {
		out.URL = detail.URL
	}
	if out.Vid == "" {
		out.Vid = detail.Vid
	}
	if out.VideoID == "" {
		out.VideoID = detail.VideoID
	}
	if out.StorageID == "" {
		out.StorageID = detail.StorageID
	}
	if out.Size == 0 {
		out.Size = detail.Size
	}
	if out.Raw == nil {
		out.Raw = detail.Raw
	}
	return out
}

func sameID(a, b string) bool {
	return strings.TrimSpace(a) != "" && strings.TrimSpace(a) == strings.TrimSpace(b)
}

func sameFileIdentity(f fileItem, id string) bool {
	id = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(id, "history_"), "homework_"))
	return sameID(f.ID, id) || sameID(f.MyID, id) || sameID(f.VideoID, id) || sameID(f.Vid, id)
}

func prefixTitle(prefix, title string) string {
	if strings.HasPrefix(title, prefix) {
		return title
	}
	return prefix + title
}

func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "1" || s == "true" || s == "yes" || s == "y"
	case float64:
		return t != 0
	case int:
		return t != 0
	default:
		return strings.TrimSpace(fmt.Sprint(t)) == "1"
	}
}

func parseSize(v any) int64 {
	s := valueText(v)
	if s == "" || s == "<nil>" {
		return 0
	}
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f)
	}
	return 0
}

func clean(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "plaso"
	}
	return strings.Trim(strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '_'
		}
		return r
	}, s), " .")
}

func looksDownloadable(raw string) bool {
	l := strings.ToLower(strings.TrimSpace(raw))
	if strings.Contains(l, ".m3u8") || strings.Contains(l, ".mp4") || strings.Contains(l, ".mp3") {
		return true
	}
	if isLikelyPlistURL(l) {
		return false
	}
	fmtv := formatOf(raw, "")
	return fmtv != "bin" || strings.Contains(l, "download")
}

func formatOf(raw, typ string) string {
	l := strings.ToLower(strings.TrimSpace(raw))
	if strings.Contains(l, ".m3u8") || strings.Contains(l, "format=m3u8") {
		return "m3u8"
	}
	if strings.Contains(l, ".mp4") {
		return "mp4"
	}
	if strings.Contains(l, ".mp3") || strings.Contains(l, ".m4a") || strings.Contains(l, ".aac") {
		if strings.Contains(l, ".m4a") {
			return "m4a"
		}
		if strings.Contains(l, ".aac") {
			return "aac"
		}
		return "mp3"
	}
	if u, err := url.Parse(raw); err == nil {
		ext := strings.TrimPrefix(strings.ToLower(path.Ext(u.Path)), ".")
		if ext != "" {
			return ext
		}
	}
	t := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(typ), "."))
	switch {
	case strings.Contains(t, "video") || strings.Contains(t, "live"):
		return "mp4"
	case strings.Contains(t, "audio"):
		return "mp3"
	case strings.Contains(t, "pdf"):
		return "pdf"
	case strings.Contains(t, "ppt"):
		return "ppt"
	case strings.Contains(t, "doc"):
		return "doc"
	case strings.Contains(t, "xls"):
		return "xls"
	case t != "" && t != "file" && t != "material" && len(t) <= 8:
		return t
	default:
		return "bin"
	}
}

func isDocumentLike(f fileItem) bool {
	fmtv := formatOf(firstNonEmpty(f.URL, f.LocationPath, f.Location), f.Type)
	if fmtv == "m3u8" || fmtv == "mp4" || fmtv == "mp3" || fmtv == "m4a" || fmtv == "aac" {
		return false
	}
	t := strings.ToLower(f.Type + " " + f.Name + " " + f.Location + " " + f.LocationPath)
	if strings.Contains(t, "video") || strings.Contains(t, "audio") || strings.Contains(t, "live") {
		return false
	}
	return fmtv != "bin" ||
		strings.Contains(t, "file") || strings.Contains(t, "课件") ||
		strings.Contains(t, "讲义") || strings.Contains(t, "资料") || strings.Contains(t, "文档") ||
		strings.Contains(t, "作业") || strings.Contains(t, "试卷") ||
		strings.Contains(t, "pdf") || strings.Contains(t, "ppt") || strings.Contains(t, "doc")
}

func isVideoLike(f fileItem) bool {
	if firstNonEmpty(f.Vid, f.VideoID) != "" {
		return true
	}
	fmtv := formatOf(firstNonEmpty(f.URL, f.LocationPath, f.Location), f.Type)
	if fmtv == "m3u8" || fmtv == "mp4" || fmtv == "mp3" || fmtv == "m4a" || fmtv == "aac" {
		return true
	}
	t := strings.ToLower(f.Type + " " + f.Name + " " + f.Location + " " + f.LocationPath + " " + f.URL)
	return strings.Contains(t, "video") || strings.Contains(t, "live") || strings.Contains(t, "audio") || strings.Contains(t, "polyv") || strings.Contains(t, "aliyun")
}

func isLikelyPlistURL(raw string) bool {
	l := strings.ToLower(raw)
	return strings.Contains(l, ".json") || strings.Contains(l, ".plist") || strings.Contains(l, "plist") || strings.Contains(l, "manifest") || strings.Contains(l, "playback")
}

func isAudioMap(m map[string]any) bool {
	if truthy(m["is_audio"]) || truthy(m["isAudio"]) {
		return true
	}
	t := strings.ToLower(firstText(m, "type", "mediaType", "fileType", "kind"))
	return strings.Contains(t, "audio")
}

func buildPlasoEventAudioURL(mediaPath string) string {
	mediaPath = strings.TrimSpace(strings.ReplaceAll(mediaPath, `\/`, `/`))
	if mediaPath == "" {
		return ""
	}
	if strings.HasPrefix(mediaPath, "http://") || strings.HasPrefix(mediaPath, "https://") {
		return mediaPath
	}
	if strings.HasPrefix(mediaPath, "//") {
		return "https:" + mediaPath
	}
	return "https://file.plaso.cn/teaching/" + strings.TrimLeft(mediaPath, "/")
}

func looksPolyvVID(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.Contains(strings.ToLower(s), "polyv") {
		return true
	}
	return len(s) >= 24 && !strings.Contains(s, "-")
}

func streamHeaders(h map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range h {
		if strings.TrimSpace(v) != "" {
			out[k] = v
		}
	}
	return out
}

func firstStreamURL(mi *extractor.MediaInfo) string {
	if mi == nil {
		return ""
	}
	if st, ok := mi.Streams["best"]; ok && len(st.URLs) > 0 {
		return st.URLs[0]
	}
	for _, st := range mi.Streams {
		if len(st.URLs) > 0 {
			return st.URLs[0]
		}
	}
	return ""
}

func dedupeFiles(in []fileItem) []fileItem {
	seen := map[string]bool{}
	out := make([]fileItem, 0, len(in))
	for _, f := range in {
		key := firstNonEmpty(f.URL, f.Vid, f.VideoID, f.Location, f.LocationPath, f.ID+"|"+f.MyID)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	return out
}

func mergeExtra(a, b map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if u, err := url.Parse(host); err == nil && u.Host != "" {
		return u.Host
	}
	return strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
}

func buildOSSHost(bucket, endpoint, region string) string {
	endpoint = normalizeHost(endpoint)
	if bucket != "" && endpoint != "" {
		return bucket + "." + endpoint
	}
	if bucket != "" && region != "" {
		return bucket + ".oss-" + region + ".aliyuncs.com"
	}
	return ""
}

func bucketFromHost(host string) string {
	host = normalizeHost(host)
	if i := strings.Index(host, ".oss-"); i > 0 {
		return host[:i]
	}
	if i := strings.Index(host, ".oss."); i > 0 {
		return host[:i]
	}
	return ""
}

func regionFromEndpoint(host string) string {
	host = normalizeHost(host)
	if i := strings.Index(host, "oss-"); i >= 0 {
		rest := host[i+len("oss-"):]
		if j := strings.Index(rest, "."); j > 0 {
			return rest[:j]
		}
	}
	return ""
}

func normalizeOSSRegion(region string) string {
	region = strings.TrimSpace(region)
	if strings.HasPrefix(region, "oss-") {
		return strings.TrimPrefix(region, "oss-")
	}
	return region
}

func normalizePlasoStorageObjectKey(location, locationPath string) string {
	location = strings.TrimSpace(strings.ReplaceAll(location, `\/`, `/`))
	locationPath = strings.Trim(strings.TrimSpace(strings.ReplaceAll(locationPath, `\/`, `/`)), "/")
	if location == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(location), "http://") || strings.HasPrefix(strings.ToLower(location), "https://") {
		return location
	}
	objectKey := strings.TrimLeft(location, "/")
	if locationPath == "" {
		return objectKey
	}
	if strings.HasPrefix(strings.ToLower(objectKey), strings.ToLower(locationPath)+"/") {
		return objectKey
	}
	return locationPath + "/" + objectKey
}

func plasoPolyvSignInfo(v any) map[string]string {
	out := map[string]string{}
	walk(v, func(m map[string]any) {
		for _, key := range []string{"sign", "ts", "timestamp", "token"} {
			if out[key] == "" {
				out[key] = firstText(m, key)
			}
		}
	})
	return out
}

func (s *plasoSession) aliPlayRequestDataVariants(f fileItem) []map[string]string {
	base := s.playRequestData(f)
	var out []map[string]string
	add := func(m map[string]string) {
		keyParts := make([]string, 0, len(m))
		for k, v := range m {
			if strings.TrimSpace(v) != "" {
				keyParts = append(keyParts, k+"="+v)
			}
		}
		if len(keyParts) == 0 {
			return
		}
		key := strings.Join(keyParts, "&")
		for _, old := range out {
			oldParts := make([]string, 0, len(old))
			for k, v := range old {
				oldParts = append(oldParts, k+"="+v)
			}
			if strings.Join(oldParts, "&") == key {
				return
			}
		}
		out = append(out, m)
	}
	add(base)
	if f.ID != "" {
		add(map[string]string{"id": f.ID, "fileId": f.ID})
	}
	if f.VideoID != "" {
		add(map[string]string{"id": f.VideoID, "fileId": f.VideoID})
	}
	if f.Location != "" {
		m := cloneStringMap(base)
		m["location"] = f.Location
		add(m)
	}
	if len(out) == 0 {
		out = append(out, map[string]string{})
	}
	return out
}

func plasoPlayerURLEncrypt(raw string) string {
	key := []byte{180, 41, 188, 88, 149, 109, 70, 72}
	b := []byte(raw)
	out := make([]byte, len(b)*2)
	for i, c := range b {
		hex.Encode(out[i*2:i*2+2], []byte{c ^ key[i%len(key)]})
	}
	return string(out)
}
