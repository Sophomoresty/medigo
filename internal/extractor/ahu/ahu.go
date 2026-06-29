package ahu

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/extractor/shared"
	"github.com/Sophomoresty/mediago/internal/util"
)

const (
	course_list_url = "https://www.ahuyikao.com/center/mycourse.html"
	course_info_url = "https://www.ahuyikao.com/course/courseinfo.html?courseId=%s"
	video_play_url  = "https://www.ahuyikao.com/video/videoplay.html?courseId=%s&lessonId=%s#%s"
)

var patterns = []string{`(?:[\w-]+\.)*(?:ahuyikao|ahumooc)\.com/`}

func init() {
	extractor.Register(&Ahu{}, extractor.SiteInfo{Name: "Ahu", URL: "ahuyikao.com", NeedAuth: true})
}

type Ahu struct{}

func (a *Ahu) Patterns() []string { return patterns }

var (
	cidRe        = regexp.MustCompile(`(?i)(?:courseId|course_id)=([0-9]+)|/course/(?:courseinfo\.html)?/?([0-9]+)`)
	lessonIDRe   = regexp.MustCompile(`(?i)(?:lessonId|lesson_id)=([0-9]+)`)
	titleRe      = regexp.MustCompile(`(?is)<(?:h4|h1)[^>]*>(.*?)</(?:h4|h1)>|<title[^>]*>(.*?)</title>`)
	courseLinkRe = regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']*courseinfo\.html\?[^"']*courseId=[0-9][^"']*)["'][^>]*>(.*?)</a>`)
	lessonLinkRe = regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']*/video/videoplay\.html\?[^"']*lessonId=[0-9][^"']*)["'][^>]*>(.*?)</a>`)
	jsVarRe      = regexp.MustCompile(`(?is)var\s+%s\s*=\s*["']([^"']+)["']`)
	jsonFieldRe  = regexp.MustCompile(`(?is)["']%s["']\s*:\s*["']([^"']+)["']`)
	directURLRe  = regexp.MustCompile(`(?is)(?:var\s+)?(?:videoSrc|m3u8_url|playUrl|sourceAddress|videoUrl|url)\s*[:=]\s*["']([^"']+)["']`)
	fileHrefRe   = regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
)

func (a *Ahu) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("ahu requires login cookies")
	}
	dlOpts := AhuDownloadOptions{Cookies: opts.Cookies, Quality: opts.Quality, ListOnly: opts.ListOnly}
	ctx, err := newAhuCourse(dlOpts)
	if err != nil {
		return nil, err
	}
	if err := ctx.prepare(rawURL, true); err != nil {
		return nil, err
	}
	if lessonID := extractFirst(lessonIDRe, rawURL); lessonID != "" {
		play, err := ctx.getPlayInfo(lessonID)
		if err != nil {
			return nil, err
		}
		stream, extra, err := ctx.streamFromPlay(play)
		if err != nil {
			return nil, err
		}
		if extra == nil {
			extra = map[string]any{}
		}
		extra["course_id"] = ctx.cid
		extra["lesson_id"] = lessonID
		return &extractor.MediaInfo{Site: "ahu", Title: util.SanitizeFilename("ahu_" + ctx.cid + "_" + lessonID), Streams: map[string]extractor.Stream{"best": stream}, Extra: extra}, nil
	}
	return ctx.mediaInfo()
}

func validateAhuLogin(c *util.Client, jar http.CookieJar, headers map[string]string) error {
	if !hasAhuLoginCookie(jar) {
		return fmt.Errorf("ahu requires valid login cookies (PHPSSESSID or laravel_session)")
	}
	checkURL := fmt.Sprintf("%s?_=%d", course_list_url, time.Now().UnixMilli())
	resp, err := c.Get(checkURL, headers)
	if err != nil {
		return fmt.Errorf("ahu login check: %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ahu login check read: %w", err)
	}
	body := string(bodyBytes)
	finalURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	if !strings.Contains(finalURL, "/center/mycourse.html") {
		return fmt.Errorf("ahu login check failed: redirected to %s", firstNonEmpty(finalURL, resp.Status))
	}
	if (strings.Contains(body, "退出登录") && strings.Contains(body, "yxg-mc-student")) || strings.Contains(body, "/login/loginout.html") {
		return nil
	}
	return fmt.Errorf("ahu login check failed: mycourse page did not contain logged-in markers")
}

func hasAhuLoginCookie(jar http.CookieJar) bool {
	if jar == nil {
		return false
	}
	for _, host := range []string{"www.ahuyikao.com", "ahuyikao.com", "www.ahumooc.com", "ahumooc.com"} {
		for _, ck := range jar.Cookies(&url.URL{Scheme: "https", Host: host, Path: "/"}) {
			name := strings.ToLower(strings.TrimSpace(ck.Name))
			if ck.Value != "" && (name == "phpsessid" || name == "laravel_session") {
				return true
			}
		}
	}
	return false
}

type lessonRef struct {
	ID    string
	Title string
}

type courseRef struct {
	ID       string
	Title    string
	LearnURL string
}

type resourceRef struct {
	Title  string
	URL    string
	Format string
}

func fetchCourseList(c *util.Client, headers map[string]string) []courseRef {
	seen := map[string]bool{}
	var out []courseRef
	for page := 1; page <= 50; page++ {
		listURL := fmt.Sprintf("%s?page=%d", course_list_url, page)
		body, err := c.GetString(listURL, headers)
		if err != nil || strings.TrimSpace(body) == "" {
			break
		}
		added := 0
		for _, m := range courseLinkRe.FindAllStringSubmatchIndex(body, -1) {
			href := html.UnescapeString(body[m[2]:m[3]])
			id := extractFirst(cidRe, href)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ctx := surrounding(body, m[0], m[1], 900)
			title := firstNonEmpty(cleanText(stripTags(body[m[4]:m[5]])), firstParagraphText(ctx), "阿虎课程"+id)
			learnURL := ""
			if lm := lessonLinkRe.FindStringSubmatch(ctx); len(lm) > 1 {
				learnURL = normalizeResourceURLWithBase(lm[1], course_list_url)
			}
			out = append(out, courseRef{ID: id, Title: title, LearnURL: learnURL})
			added++
		}
		if added == 0 || !strings.Contains(body, fmt.Sprintf("page=%d", page+1)) {
			break
		}
	}
	return out
}

func parseLessons(body string) []lessonRef {
	seen := map[string]bool{}
	var lessons []lessonRef
	for _, m := range lessonLinkRe.FindAllStringSubmatch(body, -1) {
		id := extractFirst(lessonIDRe, html.UnescapeString(m[1]))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		lessons = append(lessons, lessonRef{
			ID:    id,
			Title: cleanText(stripTags(m[2])),
		})
	}
	return lessons
}

func parseCourseFiles(body, baseURL string) []resourceRef {
	seen := map[string]bool{}
	var out []resourceRef
	add := func(title, rawURL string) {
		resourceURL := normalizeResourceURLWithBase(rawURL, baseURL)
		if resourceURL == "" || !isFileURL(resourceURL) || seen[resourceURL] {
			return
		}
		seen[resourceURL] = true
		out = append(out, resourceRef{
			Title:  firstNonEmpty(cleanText(stripTags(title)), fileTitleFromURL(resourceURL), "课程讲义"),
			URL:    resourceURL,
			Format: pickFormat(resourceURL),
		})
	}
	for _, raw := range extractJSONArrayAssignments(body, "handoutsList") {
		var payload any
		if json.Unmarshal([]byte(raw), &payload) == nil {
			for _, ref := range resourceRefsFromAny(payload, baseURL) {
				add(ref.Title, ref.URL)
			}
		}
	}
	for _, m := range fileHrefRe.FindAllStringSubmatch(body, -1) {
		href := strings.TrimSpace(html.UnescapeString(m[1]))
		low := strings.ToLower(href)
		if strings.Contains(low, "/pay/buyclass.html") || strings.Contains(low, "/video/videoplay.html") {
			continue
		}
		add(m[2], href)
	}
	return out
}

func resourceRefsFromAny(v any, baseURL string) []resourceRef {
	var refs []resourceRef
	var walk func(any, string)
	walk = func(x any, title string) {
		switch vv := x.(type) {
		case map[string]any:
			nextTitle := firstNonEmpty(anyString(vv["title"]), anyString(vv["fileName"]), anyString(vv["name"]), anyString(vv["resourceName"]), title)
			for _, key := range []string{"url", "fileUrl", "filePath", "path", "downloadUrl", "resourceUrl"} {
				if raw := anyString(vv[key]); raw != "" {
					resourceURL := normalizeResourceURLWithBase(raw, baseURL)
					if isFileURL(resourceURL) {
						refs = append(refs, resourceRef{Title: nextTitle, URL: resourceURL, Format: pickFormat(resourceURL)})
					}
				}
			}
			for _, child := range vv {
				walk(child, nextTitle)
			}
		case []any:
			for _, child := range vv {
				walk(child, title)
			}
		case string:
			resourceURL := normalizeResourceURLWithBase(vv, baseURL)
			if isFileURL(resourceURL) {
				refs = append(refs, resourceRef{Title: title, URL: resourceURL, Format: pickFormat(resourceURL)})
			}
		}
	}
	walk(v, "")
	return refs
}

func resourceEntries(refs []resourceRef, headers map[string]string) []*extractor.MediaInfo {
	entries := make([]*extractor.MediaInfo, 0, len(refs))
	for _, ref := range refs {
		format := firstNonEmpty(ref.Format, pickFormat(ref.URL), "bin")
		title := strings.TrimSuffix(util.SanitizeFilename(firstNonEmpty(ref.Title, fileTitleFromURL(ref.URL), "课程讲义")), "."+format)
		entries = append(entries, &extractor.MediaInfo{
			Site:  "ahu",
			Title: title,
			Streams: map[string]extractor.Stream{"best": {
				Quality: "best",
				URLs:    []string{ref.URL},
				Format:  format,
				Headers: cloneHeaders(headers),
			}},
			Extra: map[string]any{"type": "file", "source_url": ref.URL},
		})
	}
	return entries
}

func resolveLesson(c *util.Client, headers map[string]string, cid, lessonID string) (extractor.Stream, error) {
	playURL := fmt.Sprintf(video_play_url, cid, lessonID, lessonID)
	playHeaders := cloneHeaders(headers)
	playHeaders["Referer"] = fmt.Sprintf(course_info_url, cid)
	playHeaders["referer"] = fmt.Sprintf(course_info_url, cid)
	body, err := c.GetString(playURL, playHeaders)
	if err != nil {
		return extractor.Stream{}, fmt.Errorf("fetch ahu play page: %w", err)
	}

	if direct := normalizeResourceURLWithBase(extractFirst(directURLRe, body), playURL); direct != "" {
		return mediaStream(direct, playHeaders), nil
	}

	videoID := firstNonEmpty(
		jsVar(body, "aliyunVideoId"),
		jsVar(body, "vodVideoId"),
		jsVar(body, "aliyunVid"),
		jsonField(body, "VideoId"),
		jsonField(body, "videoId"),
		jsonField(body, "vid"),
	)
	playAuth := firstNonEmpty(
		jsVar(body, "playAuth"),
		jsVar(body, "PlayAuth"),
		jsonField(body, "PlayAuth"),
		jsonField(body, "playAuth"),
	)
	if videoID != "" && playAuth != "" {
		mediaURL, err := requestAliyunPlayInfo(c, videoID, playAuth, playHeaders)
		if err == nil && mediaURL != "" {
			return mediaStream(mediaURL, playHeaders), nil
		}
	}

	// Baijiayun playback flow (source _download_baijiayun_playback):
	// extract hlsToken/playId from page, call shared.BaijiayunResolvePlayback.
	hlsToken := firstNonEmpty(jsVar(body, "hlsToken"), jsonField(body, "hlsToken"))
	playID := firstNonEmpty(
		jsVar(body, "playId"),
		jsVar(body, "roomId"),
		jsVar(body, "room_id"),
		jsonField(body, "playId"),
		jsonField(body, "roomId"),
		jsonField(body, "room_id"),
	)
	if hlsToken != "" && playID != "" {
		playbackURL, err := shared.BaijiayunResolvePlayback(c, playID, hlsToken, playHeaders)
		if err == nil && playbackURL != "" {
			return mediaStream(playbackURL, playHeaders), nil
		}
	}

	// Source also records hlsToken/playId/baijiayun markers; if no direct or
	// Aliyun or Baijiayun media URL is present, there is no downloadable stream.
	return extractor.Stream{}, fmt.Errorf("ahu: no direct/aliyun/baijiayun media URL for lesson %s", lessonID)
}

func requestAliyunPlayInfo(c *util.Client, videoID, playAuth string, headers map[string]string) (string, error) {
	payload := shared.AliyunDecodePlayAuth(playAuth)
	payload.Region = firstNonEmpty(payload.Region, "cn-shanghai")
	payload.AuthTimeout = firstNonEmpty(payload.AuthTimeout, "7200")
	if payload.AccessKeyID == "" || payload.AccessKeySecret == "" || payload.AuthInfo == "" {
		return "", fmt.Errorf("ahu aliyun playAuth missing access/authInfo")
	}

	playCfg, _ := json.Marshal(map[string]string{"EncryptType": "AliyunVoDEncryption"})
	opts := shared.AliyunPlayOptions{
		Headers:     ahuAliyunHeaders(headers),
		Referer:     firstNonEmpty(headers["Referer"], headers["referer"], course_list_url),
		Origin:      "https://www.ahuyikao.com",
		Formats:     "m3u8,mp4",
		Definitions: "FD,LD,SD,HD,OD,2K,4K",
		FetchM3U8:   true,
	}
	if len(playCfg) > 0 {
		opts.ExtraParams = map[string]string{"PlayConfig": string(playCfg)}
	}

	info, err := shared.AliyunResolvePlayInfo(c, payload, videoID, opts)
	if err != nil {
		return "", fmt.Errorf("ahu aliyun GetPlayInfo: %w", err)
	}
	mediaURL := normalizeResourceURL(info.URL)
	if strings.EqualFold(info.EncryptType, "AliyunVoDEncryption") {
		if !info.NeedMerge {
			return "", fmt.Errorf("ahu aliyun encrypted response is not m3u8")
		}
		text := info.M3U8Text
		sourceURL := mediaURL
		if strings.Contains(text, "#EXT-X-STREAM-INF") && !strings.Contains(text, "#EXT-X-KEY") {
			if variantURL := ahuFirstVariantURL(text, mediaURL); variantURL != "" {
				if variantText, err := c.GetString(variantURL, ahuM3U8Headers(headers)); err == nil && strings.TrimSpace(variantText) != "" {
					text = variantText
					sourceURL = variantURL
				}
			}
		}
		if strings.TrimSpace(text) == "" {
			return "", fmt.Errorf("ahu aliyun encrypted m3u8 is empty")
		}
		rewritten, err := shared.AliyunRewriteM3U8Keys(c, text, payload, info.EncryptType, sourceURL, opts)
		if err != nil {
			return "", fmt.Errorf("ahu aliyun GetLicense: %w", err)
		}
		rewritten = ahuInlineHexKeysAsDataURLs(ahuAbsolutizeM3U8Text(rewritten, sourceURL))
		return ahuM3U8DataURL(rewritten), nil
	}
	return mediaURL, nil
}

func ahuAliyunHeaders(headers map[string]string) map[string]string {
	h := cloneHeaders(headers)
	if firstNonEmpty(h["Accept"], h["accept"]) == "" {
		h["Accept"] = "application/json, text/plain, */*"
	}
	if firstNonEmpty(h["Origin"], h["origin"]) == "" {
		h["Origin"] = "https://www.ahuyikao.com"
	}
	return h
}

func ahuM3U8Headers(headers map[string]string) map[string]string {
	h := cloneHeaders(headers)
	h["Accept"] = "application/vnd.apple.mpegurl, application/x-mpegURL, */*"
	if firstNonEmpty(h["Origin"], h["origin"]) == "" {
		h["Origin"] = "https://www.ahuyikao.com"
	}
	return h
}

func ahuFirstVariantURL(m3u8Text, sourceURL string) string {
	inStream := false
	for _, line := range strings.Split(strings.ReplaceAll(m3u8Text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#EXT-X-STREAM-INF") {
			inStream = true
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if inStream {
			return normalizeResourceURLWithBase(trimmed, sourceURL)
		}
	}
	return ""
}

func ahuAbsolutizeM3U8Text(m3u8Text, sourceURL string) string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(m3u8Text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "data:") {
			out = append(out, line)
			continue
		}
		out = append(out, normalizeResourceURLWithBase(trimmed, sourceURL))
	}
	return strings.Join(out, "\n")
}

func ahuInlineHexKeysAsDataURLs(m3u8Text string) string {
	quoted := regexp.MustCompile(`URI=(["'])(0x[0-9a-fA-F]+)(["'])`)
	m3u8Text = quoted.ReplaceAllStringFunc(m3u8Text, func(line string) string {
		m := quoted.FindStringSubmatch(line)
		if len(m) != 4 {
			return line
		}
		return `URI=` + m[1] + ahuHexKeyDataURL(m[2]) + m[3]
	})
	unquoted := regexp.MustCompile(`URI=(0x[0-9a-fA-F]+)`)
	return unquoted.ReplaceAllStringFunc(m3u8Text, func(line string) string {
		m := unquoted.FindStringSubmatch(line)
		if len(m) != 2 {
			return line
		}
		return `URI="` + ahuHexKeyDataURL(m[1]) + `"`
	})
}

func ahuHexKeyDataURL(hexKey string) string {
	raw := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(hexKey), "0x"), "0X")
	key, err := hex.DecodeString(raw)
	if err != nil || len(key) == 0 {
		return hexKey
	}
	return "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString(key)
}

func ahuM3U8DataURL(text string) string {
	return "data:application/vnd.apple.mpegurl;base64," + base64.StdEncoding.EncodeToString([]byte(text))
}

func jsVar(body, name string) string {
	re := regexp.MustCompile(fmt.Sprintf(jsVarRe.String(), regexp.QuoteMeta(name)))
	return html.UnescapeString(extractFirst(re, body))
}

func jsonField(body, name string) string {
	re := regexp.MustCompile(fmt.Sprintf(jsonFieldRe.String(), regexp.QuoteMeta(name)))
	return html.UnescapeString(extractFirst(re, body))
}

func mediaStream(u string, headers map[string]string) extractor.Stream {
	format := pickFormat(u)
	return extractor.Stream{
		Quality:   "best",
		URLs:      []string{u},
		Format:    format,
		NeedMerge: format == "m3u8",
		Headers:   cloneHeaders(headers),
	}
}

func extractTitle(body string) string {
	for _, m := range titleRe.FindAllStringSubmatch(body, -1) {
		for _, g := range m[1:] {
			if s := cleanText(stripTags(g)); s != "" {
				return strings.TrimSuffix(s, "_阿虎医考")
			}
		}
	}
	return ""
}

func extractFirst(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	for _, g := range m[1:] {
		if g != "" {
			return g
		}
	}
	return ""
}

func cleanText(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(s)), " ")
}

func stripTags(s string) string {
	return regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(s, " ")
}

func normalizeResourceURL(s string) string {
	s = strings.TrimSpace(html.UnescapeString(strings.ReplaceAll(s, `\/`, `/`)))
	if strings.HasPrefix(s, "//") {
		return "https:" + s
	}
	return s
}

func normalizeResourceURLWithBase(s, baseURL string) string {
	s = normalizeResourceURL(s)
	if s == "" || strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}
	if strings.HasPrefix(s, "//") {
		return "https:" + s
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return s
	}
	ref, err := url.Parse(s)
	if err != nil {
		return s
	}
	return base.ResolveReference(ref).String()
}

func quoteResourceURL(s string) string {
	s = normalizeResourceURL(s)
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" {
		return s
	}
	u.RawPath = ""
	u.ForceQuery = false
	return u.String()
}

func pickFormat(u string) string {
	low := strings.ToLower(u)
	if strings.Contains(low, ".m3u8") || strings.HasPrefix(low, "data:application/vnd.apple.mpegurl") {
		return "m3u8"
	}
	if parsed, err := url.Parse(u); err == nil {
		path := strings.ToLower(parsed.Path)
		if idx := strings.LastIndex(path, "."); idx >= 0 && idx < len(path)-1 {
			return path[idx+1:]
		}
	}
	return "mp4"
}

func isFileURL(raw string) bool {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "http") {
		return false
	}
	switch pickFormat(raw) {
	case "pdf", "ppt", "pptx", "doc", "docx", "xls", "xlsx", "zip", "rar", "7z":
		return true
	default:
		return false
	}
}

func fileTitleFromURL(raw string) string {
	u, err := url.Parse(raw)
	path := raw
	if err == nil {
		path = u.Path
	}
	if i := strings.LastIndex(path, "/"); i >= 0 && i < len(path)-1 {
		name, _ := url.PathUnescape(path[i+1:])
		if ext := pickFormat(name); ext != "" {
			return strings.TrimSuffix(name, "."+ext)
		}
		return name
	}
	return ""
}

func extractJSONArrayAssignments(text, varName string) []string {
	re := regexp.MustCompile(`(?is)(?:var|let|const)\s+` + regexp.QuoteMeta(varName) + `\s*=`)
	var out []string
	for _, loc := range re.FindAllStringIndex(text, -1) {
		start := strings.Index(text[loc[1]:], "[")
		if start < 0 {
			continue
		}
		start += loc[1]
		if arr := balancedArray(text[start:]); arr != "" {
			out = append(out, arr)
		}
	}
	return out
}

func balancedArray(s string) string {
	depth := 0
	inStr := rune(0)
	escaped := false
	for i, r := range s {
		if inStr != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == inStr {
				inStr = 0
			}
			continue
		}
		if r == '"' || r == '\'' {
			inStr = r
			continue
		}
		switch r {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return ""
}

func surrounding(s string, start, end, width int) string {
	left := start - width
	if left < 0 {
		left = 0
	}
	right := end + width
	if right > len(s) {
		right = len(s)
	}
	return s[left:right]
}

func firstParagraphText(s string) string {
	re := regexp.MustCompile(`(?is)<p\b[^>]*>(.*?)</p>`)
	if m := re.FindStringSubmatch(s); len(m) > 1 {
		return cleanText(stripTags(m[1]))
	}
	return ""
}

func dedupeEntries(in []*extractor.MediaInfo) []*extractor.MediaInfo {
	seen := map[string]bool{}
	out := make([]*extractor.MediaInfo, 0, len(in))
	for _, entry := range in {
		if entry == nil {
			continue
		}
		key := entry.Title
		for _, stream := range entry.Streams {
			if len(stream.URLs) > 0 {
				key += "|" + stream.URLs[0]
				break
			}
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, entry)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func anyString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}

func cloneHeaders(h map[string]string) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = v
	}
	return out
}
