package zhihuishu

import (
	"fmt"
	stdhtml "html"
	"regexp"
	"strings"

	"github.com/nichuanfang/medigo/internal/extractor"
	"github.com/nichuanfang/medigo/internal/util"
	xhtml "golang.org/x/net/html"
)

type courseHomeVideo struct {
	Title   string
	VideoID string
}

func extractCourseHomeCourse(rawURL, courseID string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	c := util.NewClient()
	c.SetCookieJar(opts.Cookies)
	h := zhihuishuHeaders("https://coursehome.zhihuishu.com/")

	page, err := c.GetString(fmt.Sprintf("https://coursehome.zhihuishu.com/courseHome/%s?ft=map", courseID), h)
	if err != nil {
		return nil, fmt.Errorf("fetch courseHome page: %w", err)
	}

	title := courseHomeTitle(page, courseID)
	termID := firstNonEmpty(
		match1(page, `var\s+termId\s*=\s*(-?\d+);`),
		match1(page, `termId\s*:\s*(-?\d+)`),
		match1(page, `courseTermId\s*:\s*(-?\d+)`),
	)
	if termID == "" {
		return nil, fmt.Errorf("courseHome %s missing termId", courseID)
	}

	contentURL := fmt.Sprintf("https://coursehome.zhihuishu.com/home/communication/content/%s/%s", courseID, termID)
	body, err := c.PostForm(contentURL, map[string]string{}, h)
	if err != nil {
		return nil, fmt.Errorf("fetch courseHome content: %w", err)
	}
	videos, err := parseCourseHomeVideos(body)
	if err != nil {
		return nil, fmt.Errorf("parse courseHome html: %w", err)
	}
	if len(videos) == 0 {
		return nil, fmt.Errorf("courseHome %s returned no videos", courseID)
	}

	var entries []*extractor.MediaInfo
	var firstErr error
	for _, item := range videos {
		if item.VideoID == "" {
			continue
		}
		videoURL, err := getVideoURL(c, item.VideoID, h)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		subURL, _ := getSubtitleURL(c, item.VideoID, h)
		entries = append(entries, &extractor.MediaInfo{
			Site:  "zhihuishu",
			Title: item.Title,
			Streams: map[string]extractor.Stream{
				"default": {
					Quality: "best",
					URLs:    []string{videoURL},
					Format:  pickFormat(videoURL),
					Headers: h,
				},
			},
			Subtitles: subtitleFromURL(subURL),
		})
	}
	if len(entries) == 0 {
		if firstErr != nil {
			return nil, fmt.Errorf("courseHome %s returned no playable videos: %w", courseID, firstErr)
		}
		return nil, fmt.Errorf("courseHome %s returned no playable videos", courseID)
	}

	return &extractor.MediaInfo{
		Site:    "zhihuishu",
		Title:   title,
		Entries: entries,
		Extra: map[string]any{
			"course_id": courseID,
			"term_id":   termID,
			"source":    "courseHome html",
			"raw_url":   rawURL,
		},
	}, nil
}

func courseHomeTitle(page, fallback string) string {
	courseName := stdhtml.UnescapeString(match1(page, `var\s+courseName\s*=\s*"(.*?)";`))
	schoolName := stdhtml.UnescapeString(match1(page, `var\s+schoolName\s*=\s*"(.*?)";`))
	switch {
	case courseName != "" && schoolName != "":
		return sanitize(courseName + "_" + schoolName)
	case courseName != "":
		return sanitize(courseName)
	default:
		return "zhihuishu_" + fallback
	}
}

func parseCourseHomeVideos(body string) ([]courseHomeVideo, error) {
	doc, err := xhtml.Parse(strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	var out []courseHomeVideo
	wraps := findByClass(doc, "div", "online-sections-wrap")
	for ci, wrap := range wraps {
		sections := findByClass(wrap, "div", "sections-wrap")
		for si, sectionWrap := range sections {
			sectionItem := firstByClass(sectionWrap, "div", "section-item")
			if sectionItem != nil {
				if videoID := attr(sectionItem, "videoid"); videoID != "" {
					title := titleFromClass(sectionItem, "online-section-title-text-wrap")
					out = append(out, courseHomeVideo{
						Title:   fmt.Sprintf("[%d.%d]--%s", ci+1, si+1, sanitizeCourseHomeName(title)),
						VideoID: videoID,
					})
				}
			}
			childNodes := findByClass(sectionWrap, "div", "section-childnode-item")
			for di, child := range childNodes {
				if videoID := attr(child, "videoid"); videoID != "" {
					title := titleFromClass(child, "online-section-title-text-wrap")
					out = append(out, courseHomeVideo{
						Title:   fmt.Sprintf("[%d.%d.%d]--%s", ci+1, si+1, di+1, sanitizeCourseHomeName(title)),
						VideoID: videoID,
					})
				}
			}
		}
	}
	return out, nil
}

func sanitizeCourseHomeName(s string) string {
	s = stdhtml.UnescapeString(strings.TrimSpace(s))
	if s == "" {
		return "zhihuishu"
	}
	return sanitize(s)
}

func titleFromClass(n *xhtml.Node, className string) string {
	if n == nil {
		return ""
	}
	if child := firstByClass(n, "", className); child != nil {
		if title := attr(child, "title"); title != "" {
			return title
		}
	}
	return ""
}

func attr(n *xhtml.Node, name string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}
	return ""
}

func firstByClass(n *xhtml.Node, tag, className string) *xhtml.Node {
	nodes := findByClass(n, tag, className)
	if len(nodes) == 0 {
		return nil
	}
	return nodes[0]
}

func findByClass(n *xhtml.Node, tag, className string) []*xhtml.Node {
	var out []*xhtml.Node
	var walk func(*xhtml.Node)
	walk = func(cur *xhtml.Node) {
		if cur == nil {
			return
		}
		if nodeMatchesClass(cur, tag, className) {
			out = append(out, cur)
		}
		for child := cur.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return out
}

func nodeMatchesClass(n *xhtml.Node, tag, className string) bool {
	if n == nil || n.Type != xhtml.ElementNode {
		return false
	}
	if tag != "" && !strings.EqualFold(n.Data, tag) {
		return false
	}
	if className == "" {
		return true
	}
	for _, a := range n.Attr {
		if !strings.EqualFold(a.Key, "class") {
			continue
		}
		for _, item := range strings.Fields(a.Val) {
			if item == className {
				return true
			}
		}
	}
	return false
}

func getSubtitleURL(c *util.Client, videoID string, h map[string]string) (string, error) {
	body, err := c.GetString(fmt.Sprintf("https://newbase.zhihuishu.com/video/subtitleV1/?id=%s", videoID), h)
	if err != nil {
		return "", err
	}
	if m := regexp.MustCompile(`\\"src\\"\s*:\s*\\"(http[^\\"]+)\\"`).FindStringSubmatch(body); len(m) > 1 {
		return strings.ReplaceAll(m[1], `\/`, `/`), nil
	}
	if m := regexp.MustCompile(`"src"\s*:\s*"(http[^"]+)"`).FindStringSubmatch(body); len(m) > 1 {
		return strings.ReplaceAll(m[1], `\/`, `/`), nil
	}
	return "", nil
}

func subtitleFromURL(u string) []extractor.Subtitle {
	if u == "" {
		return nil
	}
	return []extractor.Subtitle{{Language: "zh", URL: u, Format: "srt"}}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func match1(s, pat string) string {
	if m := regexp.MustCompile(pat).FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	return ""
}

var sanitizeRe = regexp.MustCompile(`[\\/:*?"<>|\r\n\t]+`)

func sanitize(s string) string {
	return sanitizeRe.ReplaceAllString(strings.TrimSpace(s), "_")
}
