package shared

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/Sophomoresty/mediago/internal/util"
)

// WhiteboardRenderOptions controls provider-neutral board fetching and export.
type WhiteboardRenderOptions struct {
	Title        string
	Site         string
	MaterialHost string
	BaseURL      string
	Headers      map[string]string
	MaxPages     int
	IncludePNG   bool
}

// WhiteboardExport is a downloadable rendered board artifact.
type WhiteboardExport struct {
	Title     string                   `json:"title"`
	Site      string                   `json:"site,omitempty"`
	URL       string                   `json:"url"`
	Format    string                   `json:"format"`
	HTML      string                   `json:"-"`
	Timeline  WhiteboardTimeline       `json:"timeline"`
	Pages     []WhiteboardRenderedPage `json:"pages"`
	SourceURL string                   `json:"source_url,omitempty"`
}

// WhiteboardRenderedPage contains per-page export metadata.
type WhiteboardRenderedPage struct {
	ID         string `json:"id"`
	Title      string `json:"title,omitempty"`
	StartMS    int64  `json:"start_ms,omitempty"`
	EndMS      int64  `json:"end_ms,omitempty"`
	EventCount int    `json:"event_count"`
	ImageURL   string `json:"image_url,omitempty"`
	BoardURL   string `json:"board_url,omitempty"`
	PNGDataURL string `json:"png_data_url,omitempty"`
}

// FindWhiteboardPayload returns the first nested value that looks like a board payload.
func FindWhiteboardPayload(values ...any) (any, bool) {
	for _, value := range values {
		if payload, ok := findWhiteboardPayload(value, 0); ok {
			return payload, true
		}
	}
	return nil, false
}

func findWhiteboardPayload(value any, depth int) (any, bool) {
	if value == nil || depth > 10 {
		return nil, false
	}
	switch v := value.(type) {
	case string:
		text := decodeWhiteboardText(v)
		if looksLikeCCTalkWhiteboardXML(text) {
			return text, true
		}
		if nested, ok := decodeNestedJSON(text); ok {
			return findWhiteboardPayload(nested, depth+1)
		}
	case map[string]any:
		if looksLikeWhiteboardPayloadMap(v) {
			return v, true
		}
		for _, key := range []string{"decrypted_payload", "data", "payload", "courseware", "contentInfo", "coursewareInfo", "courseWareInfo", "ocsInfo", "raw"} {
			if payload, ok := findWhiteboardPayload(v[key], depth+1); ok {
				return payload, true
			}
		}
		for _, child := range v {
			if payload, ok := findWhiteboardPayload(child, depth+1); ok {
				return payload, true
			}
		}
	case []any:
		for _, child := range v {
			if payload, ok := findWhiteboardPayload(child, depth+1); ok {
				return payload, true
			}
		}
	}
	return nil, false
}

func looksLikeWhiteboardPayloadMap(m map[string]any) bool {
	if text := decodeWhiteboardText(firstText(m, "content", "xml", "coursewareContent")); looksLikeCCTalkWhiteboardXML(text) {
		return true
	}
	for _, key := range []string{"whiteBoardPen", "whiteboardPen", "pen", "events", "eventList", "pages", "normalPages"} {
		if _, ok := m[key]; ok {
			return true
		}
	}
	mediaType := strings.TrimSpace(firstText(m, "mediaType", "coursewareType", "contentType", "sourceType"))
	return mediaType == "3" || strings.EqualFold(mediaType, "board") || strings.EqualFold(mediaType, "whiteboard")
}

func looksLikeCCTalkWhiteboardXML(text string) bool {
	low := strings.ToLower(strings.TrimSpace(text))
	return strings.Contains(low, "<normalpage") && strings.Contains(low, "<whiteboard")
}

// BuildWhiteboardTimeline normalizes Cctalk/Mddclass-like XML or generic JSON into a timeline.
func BuildWhiteboardTimeline(payload any, opts WhiteboardRenderOptions) (WhiteboardTimeline, bool) {
	payload, ok := FindWhiteboardPayload(payload)
	if !ok {
		return WhiteboardTimeline{}, false
	}
	provider := firstNonEmpty(opts.Site, "whiteboard")
	var timeline WhiteboardTimeline
	switch v := payload.(type) {
	case string:
		text := decodeWhiteboardText(v)
		if looksLikeCCTalkWhiteboardXML(text) {
			parsed, err := ParseCCTalkWhiteboardXML([]byte(text), opts.Title)
			if err != nil {
				return WhiteboardTimeline{}, false
			}
			timeline = parsed
		} else if nested, ok := decodeNestedJSON(text); ok {
			timeline = ParseGenericWhiteboard(nested, provider, opts.Title)
		}
	case map[string]any:
		if text := decodeWhiteboardText(firstText(v, "content", "xml", "coursewareContent")); looksLikeCCTalkWhiteboardXML(text) {
			parsed, err := ParseCCTalkWhiteboardXML([]byte(text), opts.Title)
			if err != nil {
				return WhiteboardTimeline{}, false
			}
			timeline = parsed
			generic := ParseGenericWhiteboard(v, provider, opts.Title)
			timeline.Events = append(timeline.Events, generic.Events...)
		} else {
			timeline = ParseGenericWhiteboard(v, provider, opts.Title)
		}
	}
	if timeline.Provider == "" {
		timeline.Provider = provider
	}
	if timeline.Title == "" {
		timeline.Title = opts.Title
	}
	NormalizeWhiteboardTimeline(&timeline)
	return timeline, whiteboardTimelineHasContent(timeline)
}

func whiteboardTimelineHasContent(timeline WhiteboardTimeline) bool {
	if len(timeline.Events) > 0 {
		return true
	}
	for _, page := range timeline.Pages {
		if page.ImageURL != "" || page.BoardURL != "" || page.ImageID != "" || page.BoardID != "" || len(page.Events) > 0 {
			return true
		}
	}
	return false
}

// RenderWhiteboardFromPayload fetches referenced board assets and returns an HTML data URL export.
func RenderWhiteboardFromPayload(c *util.Client, payload any, opts WhiteboardRenderOptions) (WhiteboardExport, bool) {
	timeline, ok := BuildWhiteboardTimeline(payload, opts)
	if !ok {
		return WhiteboardExport{}, false
	}
	return renderWhiteboardTimeline(c, timeline, opts, "")
}

// RenderWhiteboardTimeline exports an already-normalized timeline after resolving
// referenced resources with the same fetch/render path used by payload rendering.
func RenderWhiteboardTimeline(c *util.Client, timeline WhiteboardTimeline, opts WhiteboardRenderOptions) (WhiteboardExport, bool) {
	return renderWhiteboardTimeline(c, timeline, opts, "")
}

// RenderWhiteboardFromURL fetches a page/API URL and tries the body plus linked board assets.
func RenderWhiteboardFromURL(c *util.Client, raw string, opts WhiteboardRenderOptions) (WhiteboardExport, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return WhiteboardExport{}, false
	}
	body, finalURL, err := fetchWhiteboardBytes(c, raw, opts.Headers)
	if err != nil || len(body) == 0 {
		return WhiteboardExport{}, false
	}
	localOpts := opts
	if localOpts.BaseURL == "" {
		localOpts.BaseURL = finalURL
	}
	if exp, ok := RenderWhiteboardFromPayload(c, parseWhiteboardBody(body), localOpts); ok {
		exp.SourceURL = finalURL
		return exp, true
	}
	for _, candidate := range whiteboardCandidateURLs(string(body), finalURL) {
		candidateBody, candidateURL, err := fetchWhiteboardBytes(c, candidate, opts.Headers)
		if err != nil || len(candidateBody) == 0 {
			continue
		}
		candidateOpts := opts
		candidateOpts.BaseURL = candidateURL
		if exp, ok := RenderWhiteboardFromPayload(c, parseWhiteboardBody(candidateBody), candidateOpts); ok {
			exp.SourceURL = candidateURL
			return exp, true
		}
	}
	return WhiteboardExport{}, false
}

func renderWhiteboardTimeline(c *util.Client, timeline WhiteboardTimeline, opts WhiteboardRenderOptions, sourceURL string) (WhiteboardExport, bool) {
	NormalizeWhiteboardTimeline(&timeline)
	if opts.MaxPages > 0 && len(timeline.Pages) > opts.MaxPages {
		timeline.Pages = timeline.Pages[:opts.MaxPages]
	}
	resourceByID := map[string]WhiteboardResource{}
	for _, res := range timeline.Resources {
		if res.ID != "" {
			resourceByID[res.ID] = res
		}
	}
	for i := range timeline.Pages {
		page := &timeline.Pages[i]
		page.ImageURL = resolveTimelineResource(page.ImageURL, page.ImageID, resourceByID, opts.MaterialHost, opts.BaseURL)
		page.BoardURL = resolveTimelineResource(page.BoardURL, page.BoardID, resourceByID, opts.MaterialHost, opts.BaseURL)
		if page.ImageURL != "" {
			if b, _, err := fetchWhiteboardBytes(c, page.ImageURL, opts.Headers); err == nil && len(b) > 0 && strings.HasPrefix(http.DetectContentType(b), "image/") {
				page.ImageURL = binaryDataURL(http.DetectContentType(b), b)
			}
		}
		if page.BoardURL != "" {
			if b, finalURL, err := fetchWhiteboardBytes(c, page.BoardURL, opts.Headers); err == nil && len(b) > 0 {
				boardTimeline := ParseGenericWhiteboard(parseWhiteboardBody(b), timeline.Provider, timeline.Title)
				for _, ev := range boardTimeline.Events {
					if ev.Page == "" {
						ev.Page = page.ID
					}
					timeline.Events = append(timeline.Events, ev)
				}
				page.BoardURL = finalURL
			}
		}
	}
	NormalizeWhiteboardTimeline(&timeline)
	rendered := make([]WhiteboardRenderedPage, 0, len(timeline.Pages))
	for _, page := range timeline.Pages {
		pageEvents := eventsForPageAt(timeline, page, page.EndMS)
		pngURL := ""
		if opts.IncludePNG {
			pngURL = renderTimelinePagePNG(timeline, page, pageEvents)
		}
		rendered = append(rendered, WhiteboardRenderedPage{ID: page.ID, Title: page.Title, StartMS: page.StartMS, EndMS: page.EndMS, EventCount: len(pageEvents), ImageURL: page.ImageURL, BoardURL: page.BoardURL, PNGDataURL: pngURL})
	}
	title := firstNonEmpty(timeline.Title, opts.Title, "whiteboard")
	timeline.Title = title
	htmlDoc := WhiteboardPlayableHTML(timeline)
	return WhiteboardExport{Title: title, Site: opts.Site, URL: HTMLDataURL(htmlDoc), Format: "html", HTML: htmlDoc, Timeline: timeline, Pages: rendered, SourceURL: sourceURL}, true
}

// WhiteboardExportExtra returns compact metadata suitable for MediaInfo.Extra or Stream.Extra.
func WhiteboardExportExtra(exp WhiteboardExport) map[string]any {
	pages := make([]map[string]any, 0, len(exp.Pages))
	for _, page := range exp.Pages {
		item := map[string]any{"id": page.ID, "start_ms": page.StartMS, "end_ms": page.EndMS, "event_count": page.EventCount}
		if page.ImageURL != "" && !strings.HasPrefix(strings.ToLower(page.ImageURL), "data:") {
			item["image_url"] = page.ImageURL
		}
		if page.BoardURL != "" {
			item["board_url"] = page.BoardURL
		}
		if page.PNGDataURL != "" {
			item["png_data_url"] = page.PNGDataURL
		}
		pages = append(pages, item)
	}
	out := map[string]any{"whiteboard": true, "rendered": true, "render_format": "html", "page_count": len(exp.Timeline.Pages), "event_count": len(exp.Timeline.Events), "duration_ms": exp.Timeline.DurationMS, "width": exp.Timeline.Width, "height": exp.Timeline.Height, "pages": pages}
	if exp.SourceURL != "" {
		out["source_url"] = exp.SourceURL
	}
	return out
}

func resolveTimelineResource(raw, id string, resources map[string]WhiteboardResource, materialHost, baseURL string) string {
	if raw == "" && id != "" {
		raw = resources[id].URL
	}
	return normalizeWhiteboardResourceURL(raw, materialHost, baseURL)
}

// ResolveWhiteboardResourceURL is the exported form of the whiteboard resource
// URL normalizer for site-specific OCS manifest hydration.
func ResolveWhiteboardResourceURL(raw, materialHost, baseURL string) string {
	return normalizeWhiteboardResourceURL(raw, materialHost, baseURL)
}

func normalizeWhiteboardResourceURL(raw, materialHost, baseURL string) string {
	raw = strings.TrimSpace(strings.Trim(raw, `"'`))
	raw = strings.ReplaceAll(raw, `\/`, `/`)
	raw = strings.ReplaceAll(raw, `\u0026`, "&")
	raw = strings.ReplaceAll(raw, "&amp;", "&")
	if raw == "" {
		return ""
	}
	low := strings.ToLower(raw)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") || strings.HasPrefix(low, "data:") {
		return raw
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	if materialHost != "" && (strings.HasPrefix(raw, "/") || (!strings.HasPrefix(raw, "./") && !strings.HasPrefix(raw, "../"))) {
		return strings.TrimRight(materialHost, "/") + "/" + strings.TrimLeft(raw, "/")
	}
	if baseURL != "" {
		if base, err := url.Parse(baseURL); err == nil {
			if rel, err := url.Parse(raw); err == nil {
				return base.ResolveReference(rel).String()
			}
		}
	}
	if materialHost != "" {
		return strings.TrimRight(materialHost, "/") + "/" + strings.TrimLeft(raw, "/")
	}
	return raw
}

func eventsForPageAt(t WhiteboardTimeline, page WhiteboardPage, until int64) []WhiteboardEvent {
	var out []WhiteboardEvent
	for _, ev := range t.Events {
		if ev.Page != "" && page.ID != "" && ev.Page != page.ID {
			continue
		}
		if ev.TimeMS > until {
			continue
		}
		if ev.EndMS > 0 && ev.EndMS <= until {
			continue
		}
		out = append(out, ev)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TimeMS < out[j].TimeMS })
	return out
}

func renderTimelinePagePNG(t WhiteboardTimeline, page WhiteboardPage, events []WhiteboardEvent) string {
	width, height := t.Width, t.Height
	if width <= 0 {
		width = 1280
	}
	if height <= 0 {
		height = 720
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	if page.ImageURL != "" {
		if b, ok := decodeDataURLBytes(page.ImageURL); ok {
			if bg, _, err := image.Decode(bytes.NewReader(b)); err == nil {
				drawScaledImage(img, bg, pageRect(page, width, height))
			}
		}
	}
	profile := pageProfile(page, events, width, height)
	for _, ev := range events {
		if ev.Type == "clear" {
			draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
			continue
		}
		points := scaleTimelinePoints(ev.Points, profile, width, height)
		if len(points) == 0 {
			continue
		}
		col := parseTimelineColor(ev.Color)
		thickness := int(math.Round(math.Max(1, ev.Width*profile.scale)))
		if ev.Type == "eraser" {
			col = color.RGBA{R: 255, G: 255, B: 255, A: 255}
			thickness = maxLocalInt(thickness*4, 12)
		}
		drawTimelinePolyline(img, points, col, thickness)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}
	return binaryDataURL("image/png", buf.Bytes())
}

type timelineProfile struct{ x, y, sx, sy, scale float64 }

func pageProfile(page WhiteboardPage, events []WhiteboardEvent, width, height int) timelineProfile {
	rect := pageRect(page, width, height)
	maxX, maxY := 0.0, 0.0
	for _, ev := range events {
		for _, p := range ev.Points {
			if p.X > maxX {
				maxX = p.X
			}
			if p.Y > maxY {
				maxY = p.Y
			}
		}
	}
	logicalW, logicalH := maxX, maxY
	if maxX > 0 && maxX <= 650 && maxY > 0 && maxY <= 510 {
		logicalW, logicalH = 630, 495
	}
	if logicalW <= 0 {
		logicalW = rect.Width
	}
	if logicalH <= 0 {
		logicalH = rect.Height
	}
	sx, sy := rect.Width/math.Max(1, logicalW), rect.Height/math.Max(1, logicalH)
	return timelineProfile{x: rect.X, y: rect.Y, sx: sx, sy: sy, scale: math.Min(sx, sy)}
}

func pageRect(page WhiteboardPage, width, height int) WhiteboardPage {
	if page.Width <= 0 {
		page.Width = float64(width)
	}
	if page.Height <= 0 {
		page.Height = float64(height)
	}
	return page
}

func scaleTimelinePoints(points []WhiteboardPoint, profile timelineProfile, width, height int) []WhiteboardPoint {
	out := make([]WhiteboardPoint, 0, len(points))
	for _, p := range points {
		x := profile.x + p.X*profile.sx
		y := profile.y + p.Y*profile.sy
		out = append(out, WhiteboardPoint{X: math.Max(float64(-width), math.Min(float64(width*2), x)), Y: math.Max(float64(-height), math.Min(float64(height*2), y))})
	}
	return out
}

func drawScaledImage(dst *image.RGBA, src image.Image, rect WhiteboardPage) {
	r := image.Rect(int(math.Round(rect.X)), int(math.Round(rect.Y)), int(math.Round(rect.X+rect.Width)), int(math.Round(rect.Y+rect.Height))).Intersect(dst.Bounds())
	if r.Empty() || src == nil {
		return
	}
	sb := src.Bounds()
	if sb.Empty() {
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			sx := sb.Min.X + int(float64(x-int(math.Round(rect.X)))*float64(sb.Dx())/math.Max(1, rect.Width))
			sy := sb.Min.Y + int(float64(y-int(math.Round(rect.Y)))*float64(sb.Dy())/math.Max(1, rect.Height))
			dst.Set(x, y, src.At(clampLocalInt(sx, sb.Min.X, sb.Max.X-1), clampLocalInt(sy, sb.Min.Y, sb.Max.Y-1)))
		}
	}
}

func drawTimelinePolyline(img *image.RGBA, points []WhiteboardPoint, col color.RGBA, thickness int) {
	if len(points) == 1 {
		drawCircle(img, int(math.Round(points[0].X)), int(math.Round(points[0].Y)), maxLocalInt(1, thickness/2), col)
		return
	}
	for i := 1; i < len(points); i++ {
		drawLine(img, points[i-1], points[i], col, thickness)
	}
}

func drawLine(img *image.RGBA, a, b WhiteboardPoint, col color.RGBA, thickness int) {
	x0, y0 := int(math.Round(a.X)), int(math.Round(a.Y))
	x1, y1 := int(math.Round(b.X)), int(math.Round(b.Y))
	steps := int(math.Max(math.Abs(float64(x1-x0)), math.Abs(float64(y1-y0))))
	if steps == 0 {
		drawCircle(img, x0, y0, maxLocalInt(1, thickness/2), col)
		return
	}
	r := maxLocalInt(1, thickness/2)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(x0) + (float64(x1)-float64(x0))*t))
		y := int(math.Round(float64(y0) + (float64(y1)-float64(y0))*t))
		drawCircle(img, x, y, r, col)
	}
}

func drawCircle(img *image.RGBA, cx, cy, radius int, col color.RGBA) {
	if radius < 1 {
		radius = 1
	}
	r2 := radius * radius
	for y := cy - radius; y <= cy+radius; y++ {
		for x := cx - radius; x <= cx+radius; x++ {
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r2 && image.Pt(x, y).In(img.Bounds()) {
				img.SetRGBA(x, y, col)
			}
		}
	}
}

func parseTimelineColor(raw string) color.RGBA {
	s := strings.TrimSpace(raw)
	if s == "" {
		return color.RGBA{A: 255}
	}
	if strings.HasPrefix(s, "#") {
		hex := strings.TrimPrefix(s, "#")
		if len(hex) == 3 {
			hex = strings.Repeat(hex[0:1], 2) + strings.Repeat(hex[1:2], 2) + strings.Repeat(hex[2:3], 2)
		}
		if len(hex) >= 6 {
			if n, err := strconvParseHex(hex[:6]); err == nil {
				return color.RGBA{R: byte(n >> 16), G: byte(n >> 8), B: byte(n), A: 255}
			}
		}
	}
	return color.RGBA{A: 255}
}

func strconvParseHex(s string) (uint64, error) {
	var n uint64
	for _, r := range s {
		n <<= 4
		switch {
		case r >= '0' && r <= '9':
			n += uint64(r - '0')
		case r >= 'a' && r <= 'f':
			n += uint64(r-'a') + 10
		case r >= 'A' && r <= 'F':
			n += uint64(r-'A') + 10
		default:
			return 0, fmt.Errorf("bad hex")
		}
	}
	return n, nil
}

func parseWhiteboardBody(body []byte) any {
	text := strings.TrimSpace(string(body))
	if nested, ok := decodeNestedJSON(decodeWhiteboardText(text)); ok {
		return nested
	}
	return decodeWhiteboardText(text)
}

func decodeWhiteboardText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, `\u003c`, "<")
	text = strings.ReplaceAll(text, `\u003e`, ">")
	text = strings.ReplaceAll(text, `\"`, `"`)
	if decoded, err := base64.StdEncoding.DecodeString(text); err == nil && len(decoded) > 0 {
		plain := strings.TrimSpace(string(decoded))
		if strings.HasPrefix(plain, "<") || strings.HasPrefix(plain, "{") || strings.HasPrefix(plain, "[") {
			return plain
		}
	}
	return text
}

func fetchWhiteboardBytes(c *util.Client, raw string, headers map[string]string) ([]byte, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", fmt.Errorf("empty URL")
	}
	if data, ok := decodeDataURLBytes(raw); ok {
		return data, raw, nil
	}
	if c == nil {
		c = util.NewClient()
	}
	resp, err := c.Get(raw, headers)
	if err != nil {
		return nil, raw, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, raw, fmt.Errorf("HTTP %d from %s", resp.StatusCode, raw)
	}
	finalURL := raw
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	b, err := io.ReadAll(resp.Body)
	return b, finalURL, err
}

func decodeDataURLBytes(raw string) ([]byte, bool) {
	if !strings.HasPrefix(strings.ToLower(raw), "data:") {
		return nil, false
	}
	idx := strings.Index(raw, ",")
	if idx < 0 {
		return nil, false
	}
	meta := strings.ToLower(raw[:idx])
	data := raw[idx+1:]
	if strings.Contains(meta, ";base64") {
		b, err := base64.StdEncoding.DecodeString(data)
		return b, err == nil
	}
	text, err := url.PathUnescape(data)
	return []byte(text), err == nil
}

func binaryDataURL(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

var whiteboardURLRe = regexp.MustCompile(`(?i)(https?:)?//[^\s"'<>]+|["']([^"']*(?:whiteboard|white_board|board|courseware_contents|\.json|\.cwr)[^"']*)["']`)

func whiteboardCandidateURLs(text, base string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range whiteboardURLRe.FindAllStringSubmatch(text, -1) {
		candidate := m[0]
		if len(m) > 2 && m[2] != "" {
			candidate = m[2]
		}
		candidate = strings.Trim(candidate, `"'`)
		low := strings.ToLower(candidate)
		if !(strings.Contains(low, "whiteboard") || strings.Contains(low, "white_board") || strings.Contains(low, "board") || strings.Contains(low, "courseware_contents") || strings.Contains(low, ".json") || strings.Contains(low, ".cwr")) {
			continue
		}
		candidate = normalizeWhiteboardResourceURL(candidate, "", base)
		if candidate == "" || seen[candidate] || strings.HasPrefix(strings.ToLower(candidate), "javascript:") {
			continue
		}
		seen[candidate] = true
		out = append(out, candidate)
		if len(out) >= 12 {
			break
		}
	}
	return out
}

func maxLocalInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampLocalInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}
