package gaotu

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/Sophomoresty/mediago/internal/util"
)

func decodeWenzaiPCURL(c *util.Client, headers map[string]string, pc string) string {
	u, err := url.Parse(strings.TrimSpace(pc))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	path := strings.ToLower(u.Path)
	switch {
	case strings.Contains(path, "/web/video/getplayurl"):
		return getMediaJSON(c, headers, u.String())
	case strings.Contains(path, "/web/playback/getplaybackinfo"):
		return getFirstMediaJSON(c, headers, playbackURLVariants(u.String())...)
	default:
		return ""
	}
}

func playbackURLVariants(raw string) []string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	addEndType := func(in *url.URL) string {
		out := *in
		qv := out.Query()
		if qv.Get("end_type") == "" {
			qv.Set("end_type", "4")
		}
		out.RawQuery = qv.Encode()
		return out.String()
	}
	replacePlaybackInfoName := func(in *url.URL, to string) string {
		out := *in
		lowPath := strings.ToLower(out.Path)
		for _, from := range []string{"getplaybackinfov4", "getplaybackinfov3", "getplaybackinfo"} {
			if idx := strings.LastIndex(lowPath, from); idx >= 0 {
				out.Path = out.Path[:idx] + to + out.Path[idx+len(from):]
				break
			}
		}
		return out.String()
	}
	path := u.Path
	lowPath := strings.ToLower(path)
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	add(u.String())
	switch {
	case strings.Contains(lowPath, "getplaybackinfov4"):
		add(replacePlaybackInfoName(u, "getPlaybackInfoV3"))
		legacy, err := url.Parse(replacePlaybackInfoName(u, "getPlaybackInfo"))
		if err == nil {
			add(addEndType(legacy))
		}
	case strings.Contains(lowPath, "getplaybackinfov3"):
		add(replacePlaybackInfoName(u, "getPlaybackInfoV4"))
		legacy, err := url.Parse(replacePlaybackInfoName(u, "getPlaybackInfo"))
		if err == nil {
			add(addEndType(legacy))
		}
	case strings.Contains(lowPath, "getplaybackinfo"):
		add(addEndType(u))
	}
	return dedupeStrings(out)
}

func gaotuMediaURLFromBody(body []byte) string {
	payload, ok := decodeGaotuJSON(body)
	if !ok {
		return ""
	}
	return gaotuMediaURLFromPayload(payload)
}

func gaotuMediaURLFromPayload(payload any) string {
	if media := pickGaotuPlaybackURL(collectGaotuPlaybackCandidates(payload)); media != "" {
		return media
	}
	if media := findMediaURL(payload); media != "" {
		return media
	}
	if s, ok := payload.(string); ok {
		return gaotuMediaURLFromString(s)
	}
	return ""
}

func gaotuMediaURLFromString(raw string) string {
	raw = normalizeURL(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "bjcloudvod://") {
		return decodeBjcloudvod(raw)
	}
	if isHTTPURL(raw) && looksPlayableGaotuURL(raw) {
		return raw
	}
	if payload, ok := decodeGaotuJSON([]byte(raw)); ok {
		return gaotuMediaURLFromPayload(payload)
	}
	return ""
}

func decodeGaotuJSON(body []byte) (any, bool) {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return nil, false
	}
	var payload any
	if err := json.Unmarshal([]byte(text), &payload); err == nil {
		return payload, true
	}
	start := strings.IndexAny(text, "{[")
	end := strings.LastIndexAny(text, "}]")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(text[start:end+1]), &payload); err == nil {
			return payload, true
		}
	}
	return nil, false
}

func collectGaotuPlaybackCandidates(payload any) []map[string]any {
	var out []map[string]any
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if isGaotuPlaybackCandidate(x) {
				out = append(out, x)
			}
			for _, key := range []string{"play_info", "playInfo", "signinLivePlayback", "videoLiveDTO", "data", "cdn_list", "cdnList", "video", "list"} {
				if child, ok := x[key]; ok {
					walk(child)
				}
			}
			for _, child := range x {
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		case string:
			if child, ok := decodeGaotuJSON([]byte(x)); ok {
				walk(child)
			}
		}
	}
	walk(payload)
	return out
}

func isGaotuPlaybackCandidate(m map[string]any) bool {
	return hasAny(m,
		"cdn_list", "cdnList",
		"url", "enc_url", "encUrl",
		"play_url", "playUrl",
		"video_url", "videoUrl",
		"m3u8",
	)
}

func pickGaotuPlaybackURL(candidates []map[string]any) string {
	if len(candidates) == 0 {
		return ""
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return gaotuNumericValue(candidates[i]["size"]) > gaotuNumericValue(candidates[j]["size"])
	})
	for _, candidate := range candidates {
		cdnList := gaotuAnyList(candidate["cdn_list"])
		if len(cdnList) == 0 {
			cdnList = gaotuAnyList(candidate["cdnList"])
		}
		if len(cdnList) == 0 {
			cdnList = []any{candidate}
		}
		for _, raw := range cdnList {
			cdn, ok := raw.(map[string]any)
			if !ok {
				if s := gaotuMediaURLFromString(fmt.Sprint(raw)); s != "" {
					return s
				}
				continue
			}
			u := normalizeURL(firstNonEmpty(
				valueString(cdn, "url", "play_url", "playUrl", "video_url", "videoUrl", "m3u8"),
				valueString(cdn, "enc_url", "encUrl"),
			))
			if strings.HasPrefix(strings.ToLower(u), "bjcloudvod://") {
				u = normalizeURL(decodeBjcloudvod(u))
			}
			if isHTTPURL(u) && looksPlayableGaotuURL(u) {
				return u
			}
		}
	}
	return ""
}

func gaotuAnyList(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case []map[string]any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, item)
		}
		return out
	case map[string]any:
		return []any{x}
	default:
		return nil
	}
}

func gaotuNumericValue(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		n, _ := x.Float64()
		return n
	case string:
		n, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return n
	default:
		n, _ := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(v)), 64)
		return n
	}
}

func looksPlayableGaotuURL(raw string) bool {
	low := strings.ToLower(strings.TrimSpace(raw))
	if low == "" || !isHTTPURL(low) {
		return false
	}
	for _, ext := range []string{".pdf", ".ppt", ".pptx", ".doc", ".docx", ".xls", ".xlsx", ".zip", ".rar", ".7z"} {
		if strings.Contains(low, ext) {
			return false
		}
	}
	return true
}
