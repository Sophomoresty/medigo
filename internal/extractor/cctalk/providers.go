package cctalk

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/extractor/shared"
)

var polyvVIDRe = regexp.MustCompile(`(?i)^[0-9a-f]{32}(?:_[a-z0-9]+)?$`)

func (a *apiClient) resolveProviderStream(item map[string]any) (extractor.Stream, map[string]any, bool) {
	if a == nil || a.c == nil || len(item) == 0 {
		return extractor.Stream{}, nil, false
	}
	if stream, extra, ok := a.resolveAliyunStream(item); ok {
		return stream, extra, true
	}
	if stream, extra, ok := a.resolvePolyvStream(item); ok {
		return stream, extra, true
	}
	return extractor.Stream{}, nil, false
}

func hasProviderVideoHint(item map[string]any) bool {
	return hasAliyunHint(item) || hasPolyvHint(item)
}

func (a *apiClient) resolveAliyunStream(item map[string]any) (extractor.Stream, map[string]any, bool) {
	payload, videoID, raw, ok := findAliyunPayload(item)
	if !ok || videoID == "" {
		return extractor.Stream{}, nil, false
	}
	if payload.Region == "" {
		payload.Region = firstNonEmpty(textValue(item, "region", "regionId", "Region", "domain_region"), "cn-shanghai")
	}
	playCfg, _ := json.Marshal(map[string]string{"EncryptType": "AliyunVoDEncryption"})
	headers := providerHeaders(a)
	info, err := shared.AliyunResolvePlayInfo(a.c, payload, videoID, shared.AliyunPlayOptions{
		Referer:           CCTALK_BASE_URL + "/",
		Origin:            CCTALK_BASE_URL,
		Quality:           a.quality,
		PreferDefinitions: aliyunDefinitions(a.quality),
		Headers:           headers,
		FetchM3U8:         true,
		RewriteM3U8Keys:   true,
		ExtraParams:       map[string]string{"PlayConfig": string(playCfg)},
	})
	if err != nil || info == nil || info.URL == "" {
		return extractor.Stream{}, nil, false
	}
	streamURL := info.URL
	if strings.TrimSpace(info.M3U8Text) != "" {
		streamURL = dataURL("application/vnd.apple.mpegurl", info.M3U8Text)
	}
	format := firstNonEmpty(info.Format, pickFormat(info.URL))
	stream := extractor.Stream{Quality: firstNonEmpty(info.Definition, "best"), URLs: []string{streamURL}, Format: format, NeedMerge: info.NeedMerge || format == "m3u8", Size: info.Size, Headers: headers}
	extra := map[string]any{
		"mode":         "aliyun",
		"aliyun_vid":   videoID,
		"aliyun_api":   info.APIURL,
		"source_type":  info.SourceType,
		"encrypt_type": info.EncryptType,
		"play_auth":    raw,
	}
	if info.M3U8Text != "" {
		extra["m3u8_text"] = info.M3U8Text
	}
	return stream, extra, true
}

func (a *apiClient) resolvePolyvStream(item map[string]any) (extractor.Stream, map[string]any, bool) {
	vid := findPolyvVID(item)
	if vid == "" {
		return extractor.Stream{}, nil, false
	}
	headers := providerHeaders(a)
	sec, err := shared.PolyvResolveSecure(a.c, vid, headers)
	if err != nil {
		return extractor.Stream{}, nil, false
	}
	manifest, err := shared.PolyvPickBestManifest(sec)
	if err != nil || strings.TrimSpace(manifest) == "" {
		return extractor.Stream{}, nil, false
	}
	manifest = normalizePolyvManifest(manifest)
	streamURL := manifest
	extra := map[string]any{"mode": "polyv", "polyv_vid": vid, "polyv_token": sec.Data.Playsafe.Token, "polyv_secure_url": fmt.Sprintf(shared.PolyvSecureURLTmpl, url.PathEscape(vid))}
	if text, err := a.c.GetString(manifest, headers); err == nil && strings.HasPrefix(strings.TrimSpace(text), "#EXTM3U") {
		if rewritten, err := shared.PolyvRewriteM3U8Keys(a.c, text, sec.Data.Playsafe.Token, headers["Referer"]); err == nil && rewritten != "" {
			text = rewritten
		}
		streamURL = dataURL("application/vnd.apple.mpegurl", text)
		extra["m3u8_text"] = text
	}
	return extractor.Stream{Quality: "best", URLs: []string{streamURL}, Format: "m3u8", NeedMerge: true, Headers: headers}, extra, true
}

func findAliyunPayload(value any) (shared.AliyunPlayPayload, string, any, bool) {
	var found shared.AliyunPlayPayload
	var foundVID string
	var foundRaw any
	walkAllMaps(value, func(m map[string]any) bool {
		vid := firstNonEmpty(textValue(m, "aliyunVideoId", "aliyunVid", "aliyun_video_id", "aliyun_vid", "aliVideoId", "aliVid", "vodVideoId", "videoId", "VideoId", "vid"))
		for _, key := range []string{"playAuth", "play_auth", "playauth", "aliyunPlayAuth", "aliyun_play_auth", "vodPlayAuth", "authInfo", "AuthInfo"} {
			if raw, ok := m[key]; ok && textAny(raw) != "" {
				payload := shared.AliyunDecodePlayAuth(raw)
				if payload.AccessKeyID != "" || payload.AccessKeySecret != "" || payload.SecurityToken != "" || payload.AuthInfo != "" {
					found, foundVID, foundRaw = payload, vid, raw
					return false
				}
			}
		}
		payload := shared.AliyunPayloadFromMap(m, m)
		if payload.AccessKeyID != "" && payload.AccessKeySecret != "" {
			found, foundVID, foundRaw = payload, vid, m
			return false
		}
		return true
	})
	if foundVID == "" {
		foundVID = findFirstText(value, "aliyunVideoId", "aliyunVid", "aliyun_video_id", "aliyun_vid", "aliVideoId", "aliVid", "vodVideoId")
	}
	if found.AccessKeyID == "" || found.AccessKeySecret == "" || foundVID == "" {
		return shared.AliyunPlayPayload{}, "", nil, false
	}
	return found, foundVID, foundRaw, true
}

func findPolyvVID(value any) string {
	if u := findFirstPolyvURL(value); u != "" {
		if vid := polyvVIDFromURL(u); vid != "" {
			return vid
		}
	}
	for _, key := range []string{"polyvVid", "polyvVID", "polyv_vid", "polyVid", "poly_vid", "polyvVideoId", "polyv_video_id", "videoPoolId", "videoPoolID", "vid"} {
		if vid := findFirstText(value, key); looksPolyvVID(vid) {
			return normalizePolyvVID(vid)
		}
	}
	if vid := findFirstText(value, "videoId", "video_id"); looksPolyvVID(vid) {
		return normalizePolyvVID(vid)
	}
	return ""
}

func hasAliyunHint(value any) bool {
	_, _, _, ok := findAliyunPayload(value)
	return ok
}

func hasPolyvHint(value any) bool {
	return findPolyvVID(value) != ""
}

func providerHeaders(a *apiClient) map[string]string {
	headers := baseHeaders()
	if a != nil && a.headers != nil {
		headers = mergeStringMaps(a.headers, nil)
	}
	if headers["Referer"] == "" {
		headers["Referer"] = CCTALK_BASE_URL + "/"
	}
	if headers["Origin"] == "" {
		headers["Origin"] = CCTALK_BASE_URL
	}
	return headers
}

func aliyunDefinitions(quality string) []string {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "4k":
		return []string{"4K", "2K", "OD", "HD", "SD", "LD", "FD"}
	case "2k":
		return []string{"2K", "OD", "HD", "SD", "LD", "FD"}
	case "od", "1080p", "1080":
		return []string{"OD", "HD", "SD", "LD", "FD"}
	case "hd", "720p", "720":
		return []string{"HD", "SD", "LD", "FD"}
	case "sd", "480p", "480":
		return []string{"SD", "LD", "FD"}
	default:
		return []string{"FD", "LD", "SD", "HD", "OD", "2K", "4K"}
	}
}

func normalizePolyvManifest(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\/`, `/`))
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	return strings.TrimRight(shared.PolyvHLSPlayBase, "/") + "/" + strings.TrimLeft(raw, "/")
}

func looksPolyvVID(raw string) bool {
	vid := normalizePolyvVID(raw)
	return polyvVIDRe.MatchString(vid)
}

func normalizePolyvVID(raw string) string {
	return strings.Trim(strings.TrimSpace(raw), "'")
}

func polyvVIDFromURL(raw string) string {
	parsed, err := url.Parse(normalizeMediaURL(raw))
	if err != nil {
		return ""
	}
	if !strings.Contains(strings.ToLower(parsed.Host), "polyv") && !strings.Contains(strings.ToLower(parsed.Host), "videocc") {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.TrimSuffix(parts[i], ".json")
		candidate = strings.TrimSuffix(candidate, ".js")
		candidate = strings.TrimSuffix(candidate, ".m3u8")
		if idx := strings.LastIndex(candidate, "_"); idx > 0 && len(candidate[:idx]) >= 32 {
			candidate = candidate[:idx]
		}
		if looksPolyvVID(candidate) {
			return normalizePolyvVID(candidate)
		}
	}
	return ""
}

func findFirstPolyvURL(value any) string {
	switch x := value.(type) {
	case string:
		if strings.Contains(strings.ToLower(x), "polyv") || strings.Contains(strings.ToLower(x), "videocc") {
			return x
		}
	case []any:
		for _, item := range x {
			if out := findFirstPolyvURL(item); out != "" {
				return out
			}
		}
	case map[string]any:
		for _, nested := range x {
			if out := findFirstPolyvURL(nested); out != "" {
				return out
			}
		}
	}
	return ""
}

func findFirstText(value any, keys ...string) string {
	wanted := map[string]bool{}
	for _, key := range keys {
		wanted[strings.ToLower(key)] = true
	}
	var out string
	walkAllMaps(value, func(m map[string]any) bool {
		for key, value := range m {
			if wanted[strings.ToLower(key)] {
				if text := textAny(value); text != "" {
					out = text
					return false
				}
			}
		}
		return true
	})
	return out
}

func walkAllMaps(value any, visit func(map[string]any) bool) {
	var walk func(any, int) bool
	walk = func(value any, depth int) bool {
		if value == nil || depth > 9 {
			return true
		}
		switch x := value.(type) {
		case map[string]any:
			if !visit(x) {
				return false
			}
			for _, nested := range x {
				if !walk(nested, depth+1) {
					return false
				}
			}
		case []any:
			for _, nested := range x {
				if !walk(nested, depth+1) {
					return false
				}
			}
		}
		return true
	}
	walk(value, 0)
}
