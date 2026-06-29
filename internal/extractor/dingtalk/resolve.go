package dingtalk

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Playback URL extraction from JSON payloads
// ---------------------------------------------------------------------------

var playbackURLKeys = []string{
	"playbackUrl", "preVideoPlayUrl", "preVideoUrl",
	"playUrl", "signedPlayUrl", "signedPlayUrlRts", "liveUrlHls",
}

var mediaURLKeys = append(playbackURLKeys,
	"mp4", "mp4Url", "videoUrl", "videoPlayUrl",
	"previewUrl", "fileUrl", "downloadUrl",
	"cdnPresignedUrl", "ossPresignedUrl", "signedUrl", "url",
)

var videoExtensions = []string{
	".m3u8", ".mp4", ".mov", ".m4v", ".flv", ".avi",
	".wmv", ".mkv", ".webm", ".ts",
}

var mediaURLRe = regexp.MustCompile(`https?://[^\s"'<>\\]+`)

var embeddedMediaQueryKeys = map[string]bool{
	"videoPlayUrl": true,
	"playbackUrl":  true,
	"playUrl":      true,
	"videoUrl":     true,
	"video_url":    true,
	"previewUrl":   true,
	"fileUrl":      true,
	"downloadUrl":  true,
	"signedUrl":    true,
	"url":          true,
	"src":          true,
}

// findPlaybackURLs walks a JSON value and collects M3U8 playback URLs.
func findPlaybackURLs(payload any) []string {
	var urls []string
	seen := map[string]bool{}

	// First pass: look for known keys with m3u8
	walkJSON(payload, func(key string, val any) {
		s, ok := val.(string)
		if !ok || s == "" {
			return
		}
		s = strings.TrimSpace(s)
		if !strings.HasPrefix(s, "http") {
			return
		}
		for _, k := range playbackURLKeys {
			if key == k && strings.Contains(strings.ToLower(s), ".m3u8") {
				if !seen[s] {
					seen[s] = true
					urls = append(urls, s)
				}
				return
			}
		}
	})

	// Second pass: any string value that looks like m3u8
	walkJSON(payload, func(key string, val any) {
		s, ok := val.(string)
		if !ok || s == "" {
			return
		}
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "http") && strings.Contains(strings.ToLower(s), ".m3u8") {
			if !seen[s] {
				seen[s] = true
				urls = append(urls, s)
			}
		}
	})

	return urls
}

// findMediaURLs walks a JSON value and collects any media URLs (m3u8, mp4, etc).
func findMediaURLs(payload any) []string {
	var urls []string
	seen := map[string]bool{}
	appendOne := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		urls = append(urls, s)
	}

	walkJSON(payload, func(key string, val any) {
		s, ok := val.(string)
		if !ok || s == "" {
			return
		}
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "http") && (isMediaURL(s) || isForceMediaKey(key)) {
			appendOne(s)
		}
		for _, embedded := range extractEmbeddedMediaURLs(s, 3) {
			appendOne(embedded)
		}
	})

	// Last resort: media URLs sometimes appear inside JSON-encoded strings or
	// escaped text values; mirror the Python source's regex scan over payload.
	if raw, err := json.Marshal(payload); err == nil {
		for _, match := range mediaURLRe.FindAllString(string(raw), -1) {
			for _, embedded := range extractEmbeddedMediaURLs(match, 2) {
				appendOne(embedded)
			}
		}
	}

	return urls
}

func isMediaURL(u string) bool {
	lower := strings.ToLower(u)
	for _, ext := range videoExtensions {
		if strings.Contains(lower, ext) {
			return true
		}
	}
	return false
}

func isDirectMediaURL(text string) bool {
	if !strings.HasPrefix(text, "http://") && !strings.HasPrefix(text, "https://") {
		return false
	}
	parsed, err := url.Parse(text)
	lower := strings.ToLower(text)
	if err == nil {
		lower = strings.ToLower(parsed.Path)
	}
	for _, ext := range videoExtensions {
		if strings.Contains(lower, ext) {
			return true
		}
	}
	return false
}

func extractEmbeddedMediaURLs(text string, maxDepth int) []string {
	if maxDepth <= 0 {
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	seenTexts := map[string]bool{}
	seenURLs := map[string]bool{}
	var out []string
	var walk func(string, int)
	walk = func(value string, depth int) {
		if depth <= 0 {
			return
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if value == "" || seenTexts[value] {
			return
		}
		seenTexts[value] = true
		if decoded, err := url.QueryUnescape(value); err == nil && decoded != value {
			walk(decoded, depth-1)
		}
		if isDirectMediaURL(value) && !seenURLs[value] {
			seenURLs[value] = true
			out = append(out, value)
		}
		if parsed, err := url.Parse(value); err == nil {
			for key, vals := range parsed.Query() {
				if !embeddedMediaQueryKeys[key] {
					continue
				}
				for _, v := range vals {
					walk(v, depth-1)
				}
			}
		}
		for _, match := range mediaURLRe.FindAllString(value, -1) {
			match = strings.TrimRight(match, `\.,;)]}`)
			if isDirectMediaURL(match) && !seenURLs[match] {
				seenURLs[match] = true
				out = append(out, match)
			}
			if match != value {
				walk(match, depth-1)
			}
		}
	}
	walk(text, maxDepth)
	return out
}

func isForceMediaKey(key string) bool {
	for _, k := range mediaURLKeys {
		if key == k {
			return true
		}
	}
	return false
}

// walkJSON recursively walks a JSON value, calling fn for each key-value pair.
func walkJSON(val any, fn func(key string, val any)) {
	switch v := val.(type) {
	case map[string]any:
		for k, child := range v {
			fn(k, child)
			walkJSON(child, fn)
		}
	case []any:
		for _, child := range v {
			walkJSON(child, fn)
		}
	}
}

// choosePreferredMediaURL picks the best URL from a list (prefer mp4 > media > first).
func choosePreferredMediaURL(urls []string) string {
	if len(urls) == 0 {
		return ""
	}
	// Prefer mp4
	for _, u := range urls {
		if strings.Contains(strings.ToLower(u), ".mp4") {
			return u
		}
	}
	// Then any media URL
	for _, u := range urls {
		if isMediaURL(u) {
			return u
		}
	}
	return urls[0]
}

// ---------------------------------------------------------------------------
// Live replay resolution (roomId + liveUuid)
// ---------------------------------------------------------------------------

// resolveLiveReplay connects via LWP and resolves the playback URL for a
// live replay given roomId and liveUuid.
func resolveLiveReplay(cookie, roomID, liveUUID string) (*liveReplayResult, error) {
	client, err := newLwpClient(cookie)
	if err != nil {
		return nil, err
	}
	if err := client.connect(); err != nil {
		return nil, err
	}
	defer client.close()

	result := &liveReplayResult{
		RoomID:   roomID,
		LiveUUID: liveUUID,
		Extra:    map[string]any{},
	}

	// Try download permission first (gives direct downloadable URLs)
	dlResult, dlErr := resolveDownloadPlaylist(client, liveUUID, roomID, "", cookie)
	if dlErr == nil && dlResult != nil {
		result.PlaybackURLs = dlResult.urls
		result.Title = dlResult.title
		result.M3U8Content = dlResult.m3u8Content
		result.Extra["root_playlist"] = dlResult.extra()
		return result, nil
	}

	// Fallback: getLiveRoomInfo
	roomInfo, err := resolveLiveRoom(client, roomID, liveUUID)
	if err != nil {
		// Last resort: try to get from record summary
		summary, summaryErr := resolveLiveRecordSummary(client, liveUUID)
		if summaryErr != nil {
			if dlErr != nil {
				return nil, fmt.Errorf("all resolution paths failed: download=%v, room=%v, summary=%v", dlErr, err, summaryErr)
			}
			return nil, fmt.Errorf("getLiveRoomInfo: %v, getLiveRecordSummary: %v", err, summaryErr)
		}
		result.Title = summary.title
		result.PlaybackURLs = summary.playbackURLs
		if summary.anchorID != "" {
			if dlResult, err := resolveDownloadPlaylist(client, liveUUID, roomID, summary.anchorID, cookie); err == nil && dlResult != nil {
				result.PlaybackURLs = dlResult.urls
				result.M3U8Content = dlResult.m3u8Content
				result.Extra["root_playlist"] = dlResult.extra()
			}
		}
		return result, nil
	}

	result.Title = roomInfo.title
	result.PlaybackURLs = roomInfo.playbackURLs
	if roomInfo.anchorID != "" {
		if dlResult, err := resolveDownloadPlaylist(client, liveUUID, roomID, roomInfo.anchorID, cookie); err == nil && dlResult != nil {
			result.PlaybackURLs = dlResult.urls
			result.M3U8Content = dlResult.m3u8Content
			result.Extra["root_playlist"] = dlResult.extra()
			return result, nil
		}
	}

	// Resolve H5 playlist to get M3U8 content
	if len(result.PlaybackURLs) > 0 {
		for _, pbURL := range result.PlaybackURLs {
			h5, err := resolveH5Playlist(client, liveUUID, pbURL)
			if err == nil && h5.content != "" {
				result.M3U8Content = prepareDingTalkM3U8Text(h5.content, pbURL, h5.playbackToken)
				result.PlaybackToken = h5.playbackToken
				result.Extra["root_playlist"] = map[string]any{
					"source":        "getH5PlayUrl",
					"playbackUrl":   pbURL,
					"playbackToken": h5.playbackToken,
				}
				break
			}
		}
	}

	return result, nil
}

type liveReplayResult struct {
	RoomID        string
	LiveUUID      string
	Title         string
	PlaybackURLs  []string
	PlaybackToken string
	M3U8Content   string
	Extra         map[string]any
}

// ---------------------------------------------------------------------------
// Public live share resolution (encCid + liveUuid)
// ---------------------------------------------------------------------------

func resolvePublicLiveShare(cookie, encCid, liveUUID, pcCode string) (*liveReplayResult, error) {
	client, err := newLwpClient(cookie)
	if err != nil {
		return nil, err
	}
	if err := client.connect(); err != nil {
		return nil, err
	}
	defer client.close()

	publicInfo, err := resolveLivePublicInfo(client, encCid, liveUUID, pcCode)
	if err != nil {
		return nil, err
	}

	result := &liveReplayResult{
		LiveUUID:     liveUUID,
		Title:        publicInfo.title,
		PlaybackURLs: publicInfo.playbackURLs,
		Extra:        map[string]any{"cid": publicInfo.cid},
	}

	// Fetch the M3U8 content directly (public shares often have accessible M3U8 URLs)
	if len(result.PlaybackURLs) > 0 {
		m3u8Content, err := fetchDirectPlaylist(result.PlaybackURLs[0], cookie)
		if err == nil && m3u8Content != "" {
			result.M3U8Content = prepareDingTalkM3U8Text(m3u8Content, result.PlaybackURLs[0], "")
			result.Extra["root_playlist"] = map[string]any{"source": "direct", "playbackUrl": result.PlaybackURLs[0]}
		}
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// AI transcribe resolution (/transcribes/<uuid>)
// ---------------------------------------------------------------------------

func resolveAITranscribe(cookie, minutesUUID string) (*liveReplayResult, error) {
	client, err := newLwpClient(cookie)
	if err != nil {
		return nil, err
	}
	if err := client.connect(); err != nil {
		return nil, err
	}
	defer client.close()

	resp, err := client.call("/r/Adaptor/PortalMinutesI/minutesDetailV2",
		[]any{map[string]string{"uuid": minutesUUID}}, 0)
	if err != nil {
		return nil, fmt.Errorf("minutesDetailV2: %w", err)
	}

	body := extractBody(resp)
	data := getMapField(body, "data")
	if data == nil {
		data = body
	}

	mediaURLs := findMediaURLs(data)
	if len(mediaURLs) == 0 {
		mediaURLs = findMediaURLs(body)
	}
	if len(mediaURLs) == 0 {
		return nil, fmt.Errorf("minutesDetailV2 returned no media URLs for uuid=%s", minutesUUID)
	}

	title := getStringField(data, "title")
	if title == "" {
		title = "dingtalk_transcribe_" + minutesUUID
	}
	extra := map[string]any{
		"minutes_uuid": minutesUUID,
		"media_urls":   append([]string(nil), mediaURLs...),
	}
	bestURL := choosePreferredMediaURL(mediaURLs)
	if strings.Contains(strings.ToLower(bestURL), ".m3u8") {
		if backupUUID := extractLiveUUIDFromMediaURL(bestURL); backupUUID != "" {
			extra["backup_live_uuid"] = backupUUID
			if dlResult, err := resolveDownloadPlaylist(client, backupUUID, "", "", cookie); err == nil && dlResult != nil {
				extra["root_playlist"] = dlResult.extra()
				return &liveReplayResult{
					LiveUUID:     minutesUUID,
					Title:        title,
					PlaybackURLs: dlResult.urls,
					M3U8Content:  dlResult.m3u8Content,
					Extra:        extra,
				}, nil
			}
		}
		if m3u8Content, err := fetchDirectPlaylist(bestURL, cookie); err == nil && m3u8Content != "" {
			extra["root_playlist"] = map[string]any{"source": "direct", "playbackUrl": bestURL}
			return &liveReplayResult{
				LiveUUID:     minutesUUID,
				Title:        title,
				PlaybackURLs: mediaURLs,
				M3U8Content:  prepareDingTalkM3U8Text(m3u8Content, bestURL, ""),
				Extra:        extra,
			}, nil
		}
	}

	return &liveReplayResult{
		LiveUUID:     minutesUUID,
		Title:        title,
		PlaybackURLs: mediaURLs,
		Extra:        extra,
	}, nil
}

// ---------------------------------------------------------------------------
// LWP RPC wrappers
// ---------------------------------------------------------------------------

type roomInfoResult struct {
	title        string
	playbackURLs []string
	anchorID     string
}

func resolveLiveRoom(client *lwpClient, roomID, liveUUID string) (*roomInfoResult, error) {
	body := []any{
		map[string]any{
			"mustReturnRoomInfo": true,
			"roomId":             roomID,
			"liveUuid":           liveUUID,
		},
	}

	resp, err := client.call("/r/Adaptor/LiveRoom/getLiveRoomInfo", body, 0)
	if err != nil {
		return nil, err
	}

	respBody := extractBody(resp)
	playbackURLs := findPlaybackURLs(respBody)

	// Extract title from roomInfoModel
	title := ""
	if roomModel, ok := respBody["roomInfoModel"].(map[string]any); ok {
		if t, ok := roomModel["title"].(string); ok {
			title = t
		}
	}

	// Also try liveDetails[0].openLiveDetailModel
	if details, ok := respBody["liveDetails"].([]any); ok && len(details) > 0 {
		if detail, ok := details[0].(map[string]any); ok {
			if model, ok := detail["openLiveDetailModel"].(map[string]any); ok {
				if len(playbackURLs) == 0 {
					playbackURLs = findPlaybackURLs(model)
				}
				if title == "" {
					if t, ok := model["title"].(string); ok {
						title = t
					}
				}
			}
		}
	}

	if len(playbackURLs) == 0 {
		playbackURLs = findPlaybackURLs(resp)
	}

	return &roomInfoResult{
		title:        title,
		playbackURLs: playbackURLs,
		anchorID:     findFirstString(respBody, "anchorId", "anchorUid", "anchor"),
	}, nil
}

type h5PlaylistResult struct {
	content       string
	playbackToken string
}

func resolveH5Playlist(client *lwpClient, liveUUID, playbackURL string) (*h5PlaylistResult, error) {
	body := []any{
		map[string]string{
			"playbackUrl": playbackURL,
			"uuid":        liveUUID,
		},
	}

	resp, err := client.call("/r/Adaptor/LiveStream/getH5PlayUrl", body, 0)
	if err != nil {
		return nil, err
	}

	respBody := extractBody(resp)
	content := findFirstString(respBody, "content", "m3u8Content")
	token := findFirstString(respBody, "playbackToken")

	if content == "" {
		return nil, fmt.Errorf("getH5PlayUrl returned no content")
	}

	return &h5PlaylistResult{
		content:       content,
		playbackToken: token,
	}, nil
}

type downloadResult struct {
	urls        []string
	title       string
	m3u8Content string
	sourceURL   string
	anchorID    string
}

func (d *downloadResult) extra() map[string]any {
	if d == nil {
		return nil
	}
	return map[string]any{
		"source":      "hasDownloadPermission",
		"playbackUrl": d.sourceURL,
		"anchorId":    d.anchorID,
	}
}

func resolveDownloadPlaylist(client *lwpClient, liveUUID, roomID, anchorID, cookie string) (*downloadResult, error) {
	if liveUUID == "" {
		return nil, fmt.Errorf("missing liveUuid")
	}

	// Try hasDownloadPermission with different body shapes
	var bodies [][]any
	addBody := func(m map[string]string) {
		key := m["cid"] + "|" + m["liveUuid"] + "|" + m["anchorId"]
		for _, body := range bodies {
			if existing, ok := body[0].(map[string]string); ok && existing["cid"]+"|"+existing["liveUuid"]+"|"+existing["anchorId"] == key {
				return
			}
		}
		bodies = append(bodies, []any{m})
	}
	addBody(map[string]string{"liveUuid": liveUUID})
	if anchorID != "" {
		addBody(map[string]string{"liveUuid": liveUUID, "anchorId": anchorID})
	}
	if roomID != "" {
		addBody(map[string]string{"cid": roomID, "liveUuid": liveUUID})
		if anchorID != "" {
			addBody(map[string]string{"cid": roomID, "liveUuid": liveUUID, "anchorId": anchorID})
		}
	}

	for _, body := range bodies {
		resp, err := client.call("/r/Adaptor/OnePunchLive/hasDownloadPermission", body, 20*time.Second)
		if err != nil {
			continue
		}

		respBody := extractBody(resp)
		var urls []string
		// Look for downloadUrl/playbackUrl directly
		for _, key := range []string{"downloadUrl", "playbackUrl", "url"} {
			if u, ok := respBody[key].(string); ok && u != "" && strings.HasPrefix(u, "http") {
				urls = append(urls, u)
			}
		}
		urls = append(urls, findPlaybackURLs(respBody)...)
		urls = uniqueStrings(urls)

		if len(urls) == 0 {
			continue
		}

		// Try to fetch the M3U8 content directly
		result := &downloadResult{urls: urls, anchorID: anchorID}
		for _, u := range urls {
			content, err := fetchDirectPlaylist(u, cookie)
			if err == nil && strings.Contains(content, "#EXTM3U") {
				result.m3u8Content = absolutizeM3U8(content, u)
				result.sourceURL = u
				break
			}
		}
		if result.sourceURL == "" {
			result.sourceURL = urls[0]
		}
		return result, nil
	}

	return nil, fmt.Errorf("hasDownloadPermission returned no downloadable URLs")
}

type publicInfoResult struct {
	title        string
	playbackURLs []string
	cid          string
}

func resolveLivePublicInfo(client *lwpClient, encCid, liveUUID, pcCode string) (*publicInfoResult, error) {
	bodies := [][]any{
		{encCid, liveUUID},
	}
	if pcCode != "" {
		bodies = append(bodies, []any{encCid, liveUUID, pcCode})
	}

	for _, body := range bodies {
		resp, err := client.call("/r/Adaptor/LiveRecord/getLivePublicInfoByEncCid", body, 0)
		if err != nil {
			continue
		}

		respBody := extractBody(resp)
		urls := findPlaybackURLs(respBody)
		if len(urls) == 0 {
			continue
		}

		title := ""
		if t, ok := respBody["title"].(string); ok {
			title = t
		}
		if title == "" {
			if t, ok := respBody["conversationName"].(string); ok {
				title = t
			}
		}

		cid := ""
		if c, ok := respBody["cid"].(string); ok {
			cid = c
		}

		return &publicInfoResult{
			title:        title,
			playbackURLs: urls,
			cid:          cid,
		}, nil
	}

	return nil, fmt.Errorf("getLivePublicInfoByEncCid returned no playback URLs")
}

type recordSummaryResult struct {
	title        string
	playbackURLs []string
	anchorID     string
}

func resolveLiveRecordSummary(client *lwpClient, liveUUID string) (*recordSummaryResult, error) {
	endpoints := []struct {
		uri  string
		body []any
	}{
		{"/r/Adaptor/LiveRecord/getRecentlyView", []any{map[string]int{"pageSize": 80}}},
		{"/r/Adaptor/LiveEntry/getRecommendLivePlayback", []any{map[string]int{"pageSize": 80, "index": 1}}},
	}

	for _, ep := range endpoints {
		resp, err := client.call(ep.uri, ep.body, 20*time.Second)
		if err != nil {
			continue
		}

		respBody := extractBody(resp)
		item := findLiveRecordItem(respBody, liveUUID)
		if item == nil {
			continue
		}

		title := ""
		if basic, ok := item["liveBasicInfo"].(map[string]any); ok {
			if t, ok := basic["title"].(string); ok {
				title = t
			}
		}

		urls := findPlaybackURLs(item)
		if len(urls) == 0 {
			urls = findPlaybackURLs(respBody)
		}

		return &recordSummaryResult{
			title:        title,
			playbackURLs: urls,
			anchorID:     findFirstString(item, "anchorId", "anchorUid", "anchor"),
		}, nil
	}

	return nil, fmt.Errorf("no live record found for liveUuid=%s", liveUUID)
}

// ---------------------------------------------------------------------------
// JSON body helpers
// ---------------------------------------------------------------------------

func extractBody(msg map[string]any) map[string]any {
	if msg == nil {
		return map[string]any{}
	}
	if body, ok := msg["body"].(map[string]any); ok {
		return body
	}
	if body, ok := msg["body"].([]any); ok && len(body) == 1 {
		if m, ok := body[0].(map[string]any); ok {
			return m
		}
	}
	return msg
}

func getMapField(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return nil
}

func findFirstString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		var found string
		walkJSON(payload, func(k string, v any) {
			if found != "" {
				return
			}
			if k == key {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					found = strings.TrimSpace(s)
				}
			}
		})
		if found != "" {
			return found
		}
	}
	return ""
}

func findLiveRecordItem(payload any, liveUUID string) map[string]any {
	if liveUUID == "" {
		return nil
	}
	var found map[string]any
	walkJSON(payload, func(key string, val any) {
		if found != nil {
			return
		}
		m, ok := val.(map[string]any)
		if !ok {
			return
		}
		basic := m
		if b, ok := m["liveBasicInfo"].(map[string]any); ok {
			basic = b
		}
		if uuid, ok := basic["liveUuid"].(string); ok && uuid == liveUUID {
			found = m
		}
	})
	return found
}

// ---------------------------------------------------------------------------
// M3U8 / URL helpers
// ---------------------------------------------------------------------------

var tsPathRe = regexp.MustCompile(`(/live.*?\.ts)(?:\?|$)`)
var uriLineRe = regexp.MustCompile(`URI="([^"]+)"`)

// absolutizeM3U8 makes relative URLs in M3U8 content absolute.
func absolutizeM3U8(content, sourceURL string) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			// Rewrite URI= in tags
			if strings.Contains(line, `URI="`) {
				line = uriLineRe.ReplaceAllStringFunc(line, func(match string) string {
					sub := uriLineRe.FindStringSubmatch(match)
					if len(sub) > 1 {
						abs := resolveURL(sourceURL, sub[1])
						return fmt.Sprintf(`URI="%s"`, abs)
					}
					return match
				})
			}
			lines = append(lines, line)
		} else {
			// Non-comment, non-empty line = segment URL
			lines = append(lines, resolveURL(sourceURL, trimmed))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func resolveURL(base, ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return ref
	}
	refURL, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return baseURL.ResolveReference(refURL).String()
}

// makeDingToken creates the authentication token for TS segments.
func makeDingToken(segmentURL, playbackToken string) string {
	if playbackToken == "" {
		return ""
	}
	m := tsPathRe.FindStringSubmatch(segmentURL)
	if len(m) < 2 {
		return ""
	}
	tsPath := m[1]
	nowTS := fmt.Sprintf("%d", time.Now().Unix())
	hash := md5.New()
	_, _ = io.WriteString(hash, fmt.Sprintf("%s%s%s", tsPath, nowTS, playbackToken))
	return fmt.Sprintf("%s-%x", nowTS, hash.Sum(nil))
}

// fetchDirectPlaylist fetches an M3U8 URL directly via HTTP.
func fetchDirectPlaylist(playlistURL, cookie string) (string, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest("GET", playlistURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Referer", referer)
	req.Header.Set("User-Agent", pcUA)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("playlist fetch status=%d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	content := string(body)
	if !strings.Contains(content, "#EXTM3U") {
		return "", fmt.Errorf("not a valid M3U8 playlist")
	}

	return content, nil
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

func uniqueStrings(ss []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		result = append(result, s)
	}
	return result
}
