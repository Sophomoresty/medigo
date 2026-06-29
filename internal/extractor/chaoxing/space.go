package chaoxing

import (
	"fmt"
	htmlpkg "html"
	"net/url"
	"regexp"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

type chaoxingCourseLink struct {
	URL   string
	Title string
}

func isChaoxingSpaceIndexURL(rawURL string) bool {
	return strings.Contains(strings.ToLower(rawURL), "i.mooc.chaoxing.com/space/index")
}

func (x *chaoxingContext) resolveSpaceIndex(rawURL string) (*extractor.MediaInfo, error) {
	body, err := x.getString(rawURL)
	if err != nil {
		return nil, fmt.Errorf("chaoxing space index: %w", err)
	}
	if !hasChaoxingPersonalName(body) {
		return nil, fmt.Errorf("chaoxing space index: login marker personalName not found")
	}
	x.extractAccessFromText(body)
	links := collectChaoxingSpaceCourseLinks(body, rawURL)
	if len(links) == 0 {
		return nil, fmt.Errorf("chaoxing space index: no course links found")
	}

	seen := map[string]bool{}
	entries := make([]*extractor.MediaInfo, 0, len(links))
	for i, link := range links {
		child := x.courseChildContext(link.URL)
		course, _, err := child.resolveCourse(link.URL)
		if err != nil || course == nil {
			continue
		}
		courseTitle := firstNonEmpty(course.Title, link.Title, child.title, fmt.Sprintf("course_%d", i+1))
		for _, entry := range course.Entries {
			if entry == nil {
				continue
			}
			entry.Title = util.SanitizeFilename(fmt.Sprintf("[%d]--%s/%s", i+1, courseTitle, firstNonEmpty(entry.Title, "item")))
			if entry.Extra == nil {
				entry.Extra = map[string]any{}
			}
			entry.Extra["source"] = "i.mooc.space"
			entry.Extra["course_url"] = link.URL
			entries = appendUniqueEntry(entries, entry, seen)
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("chaoxing space index: no downloadable course resources found")
	}
	return &extractor.MediaInfo{
		Site:    "chaoxing",
		Title:   util.SanitizeFilename(firstNonEmpty(x.title, "chaoxing_space_courses")),
		Entries: entries,
		Extra: compactExtra(map[string]any{
			"source":       "i.mooc.space",
			"course_count": len(links),
		}),
	}, nil
}

func (x *chaoxingContext) courseChildContext(rawURL string) *chaoxingContext {
	child := *x
	child.pathPrefix = ""
	child.newCourse = false
	child.courseID = ""
	child.clazzID = ""
	child.enc = ""
	child.oldEnc = ""
	child.cpi = ""
	child.openc = ""
	child.portalEnc = ""
	child.portalCourseEnc = ""
	child.portalT = ""
	child.title = ""
	child.headers = map[string]string{}
	for k, v := range x.headers {
		child.headers[k] = v
	}
	child.applyURLContext(rawURL)
	child.extractAccessFromURL(rawURL)
	child.extractPortalParams(rawURL)
	return &child
}

func (x *chaoxingContext) applyURLContext(rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return
	}
	host := strings.ToLower(u.Host)
	x.sourceHost = host
	customHost := u.Scheme + "://" + u.Host
	path := strings.ToLower(u.Path)
	for _, marker := range []string{"/mycourse/stu", "/mycourse/studentcourse"} {
		if idx := strings.Index(u.Path, marker); idx >= 0 {
			x.pathPrefix = u.Path[:idx]
			break
		}
	}
	if x.pathPrefix != "" && strings.Contains(u.Path, "/course/") {
		x.pathPrefix = u.Path[:strings.Index(u.Path, "/course/")]
	}
	if strings.Contains(path, "/mooc2-ans/") || strings.Contains(path, "/mooc-ans/") || queryValue(rawURL, "mooc2") == "1" || queryValue(rawURL, "ismooc2") == "1" {
		x.newCourse = true
	}
	if strings.HasPrefix(host, "mooc") {
		x.courseURL = customHost
		x.headers["Referer"] = x.courseURL + "/"
		x.headers["Origin"] = x.courseURL
		if isChaoxingSchoolHost(host) || strings.Contains(host, "mooc2-ans.") {
			x.newCourseURL = customHost
		}
	}
}

func collectChaoxingSpaceCourseLinks(text, baseURL string) []chaoxingCourseLink {
	seen := map[string]bool{}
	var out []chaoxingCourseLink
	add := func(raw, title string) {
		u := normalizeSpaceURL(raw, baseURL)
		if u == "" || seen[u] || !isChaoxingCourseCandidateURL(u) {
			return
		}
		seen[u] = true
		out = append(out, chaoxingCourseLink{URL: u, Title: cleanText(title)})
	}
	for _, m := range regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']+)["'][^>]*>([\s\S]*?)</a>`).FindAllStringSubmatch(text, -1) {
		add(m[1], firstNonEmpty(titleFromChunk(m[0]), stripTags(m[2])))
	}
	for _, m := range regexp.MustCompile(`(?is)(?:href|data-url|data-href|url|linkUrl)\s*=\s*["']([^"']+)["']`).FindAllStringSubmatch(text, -1) {
		add(m[1], "")
	}
	for _, m := range regexp.MustCompile(`(?is)["']((?:https?:\\?/\\?/|/)(?:\\.|[^"'])+?)["']`).FindAllStringSubmatch(text, -1) {
		add(m[1], "")
	}
	return out
}

func normalizeSpaceURL(raw, baseURL string) string {
	raw = strings.TrimSpace(htmlpkg.UnescapeString(raw))
	if raw == "" || strings.HasPrefix(strings.ToLower(raw), "javascript:") {
		return ""
	}
	raw = strings.ReplaceAll(raw, `\/`, `/`)
	raw = strings.ReplaceAll(raw, `\u0026`, "&")
	raw = strings.ReplaceAll(raw, `\\u0026`, "&")
	raw = strings.Trim(raw, `"'`)
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	if !isHTTPURL(raw) && baseURL != "" {
		raw = resolveRelativeURL(baseURL, raw)
	}
	return raw
}

func isChaoxingCourseCandidateURL(rawURL string) bool {
	low := strings.ToLower(rawURL)
	if strings.Contains(low, "i.mooc.chaoxing.com/space/index") {
		return false
	}
	if strings.Contains(low, "/mycourse/stu") ||
		strings.Contains(low, "/mycourse/studentcourse") ||
		strings.Contains(low, "/visit/stucoursemiddle") ||
		strings.Contains(low, "/courseportal/portal/") ||
		strings.Contains(low, "/course-ans/courseportal/") ||
		strings.Contains(low, "xueyinonline.com/detail/") {
		return true
	}
	if regexp.MustCompile(`(?i)/(?:mooc-ans/)?course/\d+\.html`).FindString(rawURL) != "" {
		return true
	}
	return strings.Contains(low, "courseid=") && (strings.Contains(low, "clazzid=") || strings.Contains(low, "enc=") || strings.Contains(low, "chapterid="))
}

func hasChaoxingPersonalName(text string) bool {
	return regexp.MustCompile(`(?is)<p\s+[^>]*class=["'][^"']*\bpersonalName\b[^"']*["'][\s\S]*?>`).MatchString(text)
}
