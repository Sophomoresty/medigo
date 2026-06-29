package orangevip

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

type playbackResources struct {
	VideoURL string
	MP4URL   string
	AudioURL string
	DocURL   string
	Title    string
}

func fetchBaijiayunResources(c *util.Client, h map[string]string, apiURL string) playbackResources {
	body, err := c.GetString(apiURL, h)
	if err != nil {
		return playbackResources{}
	}
	var root any
	if json.Unmarshal([]byte(unwrapJSONP(body)), &root) != nil {
		return playbackResources{}
	}
	return extractPlaybackResources(root)
}

func extractPlaybackResources(root any) playbackResources {
	data := asMap(root)
	if d := asMap(data["data"]); len(d) > 0 {
		data = d
	}
	signal := asMap(data["signal"])
	doc := asMap(signal["doc"])
	playInfo := firstMap(asMap(data["play_info"]), asMap(data["playInfo"]), asMap(data["video_info"]))
	videoURL := normalizeURL(firstText(data, "video_url", "videoUrl", "playback_url", "playbackUrl", "play_url", "playUrl", "url"))
	if videoURL == "" {
		videoURL = extractOrangePlayURL(playInfo, false)
	}
	mp4URL := extractOrangePlayURL(playInfo, true)
	if videoURL == "" {
		videoURL = extractOrangePlayURLFromTree(data, false)
	}
	if mp4URL == "" {
		mp4URL = extractOrangePlayURLFromTree(data, true)
	}
	return playbackResources{
		VideoURL: videoURL,
		MP4URL:   normalizeURL(mp4URL),
		AudioURL: normalizeURL(firstText(data, "audio_url", "audioUrl")),
		DocURL:   normalizeURL(first(firstText(doc, "url"), firstText(data, "doc_url", "docUrl"))),
		Title:    firstText(asMap(data["video_info"]), "title"),
	}
}

func extractOrangePlayURL(playInfo map[string]any, mp4Only bool) string {
	if len(playInfo) == 0 {
		return ""
	}
	candidates := collectOrangeDefinitionMaps(playInfo)
	if len(candidates) == 0 {
		return ""
	}
	return pickOrangePlayURL(candidates, mp4Only)
}

func extractOrangePlayURLFromTree(root any, mp4Only bool) string {
	candidates := []map[string]any{}
	walkAny(root, func(m map[string]any) {
		if len(asList(m["cdn_list"])) > 0 || len(asList(m["cdnList"])) > 0 || firstText(m, "enc_url", "encUrl", "url") != "" {
			candidates = append(candidates, m)
		}
	})
	return pickOrangePlayURL(candidates, mp4Only)
}

func collectOrangeDefinitionMaps(playInfo map[string]any) []map[string]any {
	candidates := []map[string]any{}
	for _, v := range playInfo {
		if m := asMap(v); len(m) > 0 {
			candidates = append(candidates, m)
		}
		if list := asList(v); len(list) > 0 {
			for _, x := range list {
				if m := asMap(x); len(m) > 0 {
					candidates = append(candidates, m)
				}
			}
		}
	}
	if len(candidates) == 0 {
		candidates = append(candidates, playInfo)
	}
	return candidates
}

func pickOrangePlayURL(candidates []map[string]any, mp4Only bool) string {
	if len(candidates) == 0 {
		return ""
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return numericValue(candidates[i]["size"]) > numericValue(candidates[j]["size"])
	})
	var mp4URL, evURL, anyURL string
	for _, item := range candidates {
		cdnList := asList(item["cdn_list"])
		if len(cdnList) == 0 {
			cdnList = asList(item["cdnList"])
		}
		if len(cdnList) == 0 {
			cdnList = []any{item}
		}
		for _, raw := range cdnList {
			cdn := asMap(raw)
			u := normalizeURL(firstText(cdn, "url", "play_url", "playUrl"))
			if u == "" {
				enc := firstText(cdn, "enc_url", "encUrl")
				if strings.HasPrefix(enc, "bjcloudvod://") {
					u = normalizeURL(decodeBjcloudvod(enc))
				} else {
					u = normalizeURL(enc)
				}
			}
			if u == "" {
				continue
			}
			ext := videoExt(u)
			switch ext {
			case ".mp4":
				if mp4URL == "" {
					mp4URL = u
				}
			case ".ev1", ".ev2":
				if evURL == "" {
					evURL = u
				}
			default:
				if anyURL == "" {
					anyURL = u
				}
			}
		}
	}
	if mp4Only {
		return mp4URL
	}
	return first(mp4URL, evURL, anyURL)
}

func decodeBjcloudvod(encoded string) string {
	const prefix = "bjcloudvod://"
	if !strings.HasPrefix(encoded, prefix) {
		return ""
	}
	payload := strings.TrimPrefix(encoded, prefix)
	payload = strings.NewReplacer("-", "+", "_", "/").Replace(payload)
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(decoded) == 0 {
		return ""
	}
	shift := int(decoded[0] % 8)
	decoded = decoded[1:]
	out := make([]byte, len(decoded))
	for i, b := range decoded {
		out[i] = b ^ byte((shift+i)%8)
	}
	return string(out)
}

func docEntries(c *util.Client, h map[string]string, docURL, title string) []*extractor.MediaInfo {
	docURL = normalizeURL(docURL)
	if docURL == "" {
		return nil
	}
	var out []*extractor.MediaInfo
	if !strings.Contains(strings.ToLower(formatOf(docURL)), "mp4") {
		out = append(out, urlEntry("orangevip", clean(title), docURL, "file", h, map[string]any{"kind": "board_doc"}))
	}
	for i, u := range fetchPPTPageURLs(c, h, docURL) {
		out = append(out, urlEntry("orangevip", clean(fmt.Sprintf("%s_%03d", title, i+1)), u, "file", h, map[string]any{"kind": "board_page", "doc_url": docURL, "page_index": i + 1}))
	}
	return out
}

func fetchPPTPageURLs(c *util.Client, h map[string]string, docURL string) []string {
	body, err := c.GetString(docURL, h)
	if err != nil {
		return nil
	}
	var root any
	if json.Unmarshal([]byte(unwrapJSONP(body)), &root) != nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, item := range asList(root) {
		m := asMap(item)
		ext := strings.ToLower(firstText(m, "ext", "suffix", "format"))
		if ext != "" && ext != ".pptx" && ext != ".ppt" && ext != ".pdf" {
			continue
		}
		for _, page := range asList(m["page_list"]) {
			u := normalizeURL(firstText(asMap(page), "url"))
			if u != "" && !seen[u] {
				seen[u] = true
				out = append(out, u)
			}
		}
	}
	if len(out) == 0 {
		walkAny(root, func(m map[string]any) {
			u := normalizeURL(firstText(m, "url", "image_url", "imageUrl"))
			if u != "" && !seen[u] && looksLikeAssetURL(u) {
				seen[u] = true
				out = append(out, u)
			}
		})
	}
	return out
}

func fetchOrderPrice(c *util.Client, h map[string]string, cid string) (float64, bool) {
	for page := 1; page < 10; page++ {
		body, err := c.PostForm(order_url, map[string]string{"showCount": "99", "currentPageForApp": strconv.Itoa(page)}, h)
		if err != nil {
			return 0, false
		}
		var resp apiResp
		if json.Unmarshal([]byte(body), &resp) != nil || len(resp.Orders) == 0 {
			break
		}
		for _, order := range resp.Orders {
			if !matchesOrangeCourse(order, cid) {
				continue
			}
			price := orangePrice(firstText(order, "currentPrice", "orderPrice", "price"))
			return price, true
		}
	}
	return 0, false
}

func orderChaptersLikeWeb(chapters []map[string]any, chapterClass []map[string]any) []map[string]any {
	if len(chapters) == 0 || len(chapterClass) == 0 {
		return chapters
	}
	remaining := append([]map[string]any{}, chapters...)
	var ordered []map[string]any
	for _, cls := range chapterClass {
		guidSet := map[string]bool{}
		for _, guid := range chapterGuids(cls["chapterGuids"]) {
			guidSet[guid] = true
		}
		if len(guidSet) == 0 {
			continue
		}
		var nextRemaining []map[string]any
		for _, ch := range remaining {
			if guidSet[normalizeGuid(firstText(ch, "guid", "chapterGuid", "id"))] {
				ordered = append(ordered, ch)
			} else {
				nextRemaining = append(nextRemaining, ch)
			}
		}
		remaining = nextRemaining
	}
	return append(ordered, remaining...)
}

func chapterGuids(v any) []string {
	var out []string
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			if s := normalizeGuid(fmt.Sprint(x)); s != "" {
				out = append(out, s)
			}
		}
	case string:
		for _, part := range regexp.MustCompile(`[,;\s]+`).Split(t, -1) {
			if s := normalizeGuid(part); s != "" {
				out = append(out, s)
			}
		}
	default:
		if s := normalizeGuid(fmt.Sprint(v)); s != "" && s != "<nil>" {
			out = append(out, s)
		}
	}
	return out
}

func urlEntry(site, title, rawURL, quality string, h map[string]string, extra map[string]any) *extractor.MediaInfo {
	format := formatOf(rawURL)
	st := extractor.Stream{Quality: quality, URLs: []string{rawURL}, Format: format, Headers: h}
	if format == "m3u8" {
		st.NeedMerge = true
	}
	return &extractor.MediaInfo{Site: site, Title: title, Streams: map[string]extractor.Stream{quality: st}, Extra: extra}
}

func firstEntryURL(entry *extractor.MediaInfo) string {
	if entry == nil {
		return ""
	}
	for _, st := range entry.Streams {
		if len(st.URLs) > 0 {
			return st.URLs[0]
		}
	}
	return ""
}

func compactOrangeExtra(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		switch t := v.(type) {
		case nil:
			continue
		case string:
			if strings.TrimSpace(t) == "" {
				continue
			}
		case float64:
			if t == 0 {
				continue
			}
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func orangePrice(s string) float64 {
	s = strings.NewReplacer(",", "", "¥", "", "￥", "", "元", "").Replace(strings.TrimSpace(s))
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func matchesOrangeCourse(m map[string]any, cid string) bool {
	for _, key := range []string{"courseModelId", "courseModelID", "course_id", "courseId", "guid", "id"} {
		if firstText(m, key) == cid {
			return true
		}
	}
	return false
}

func unwrapJSONP(body string) string {
	body = strings.TrimSpace(body)
	if i := strings.Index(body, "("); i >= 0 && strings.HasSuffix(strings.TrimSuffix(body, ";"), ")") {
		return body[i+1 : strings.LastIndex(body, ")")]
	}
	return body
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func firstMap(maps ...map[string]any) map[string]any {
	for _, m := range maps {
		if len(m) > 0 {
			return m
		}
	}
	return nil
}

func asList(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case []map[string]any:
		out := make([]any, 0, len(t))
		for _, m := range t {
			out = append(out, m)
		}
		return out
	case map[string]any:
		for _, key := range []string{"list", "data", "page_list", "pageList"} {
			if out := asList(t[key]); len(out) > 0 {
				return out
			}
		}
		return []any{t}
	default:
		return nil
	}
}

func walkAny(v any, fn func(map[string]any)) {
	switch t := v.(type) {
	case map[string]any:
		fn(t)
		for _, child := range t {
			walkAny(child, fn)
		}
	case []any:
		for _, child := range t {
			walkAny(child, fn)
		}
	}
}

func numericValue(v any) float64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(v)), 64)
	return n
}

func videoExt(raw string) string {
	u, err := url.Parse(raw)
	path := raw
	if err == nil {
		path = u.Path
	}
	if i := strings.LastIndex(path, "."); i >= 0 {
		return strings.ToLower(path[i:])
	}
	return ""
}

func normalizeGuid(guid string) string {
	return strings.TrimSpace(guid)
}

func looksLikeAssetURL(u string) bool {
	lower := strings.ToLower(u)
	return strings.HasPrefix(lower, "http") && (strings.Contains(lower, ".jpg") || strings.Contains(lower, ".jpeg") || strings.Contains(lower, ".png") || strings.Contains(lower, ".webp") || strings.Contains(lower, ".pdf") || strings.Contains(lower, ".ppt"))
}
