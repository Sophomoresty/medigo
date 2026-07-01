package cctalk

import (
	"encoding/json"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/extractor/shared"
	"github.com/Sophomoresty/mediago/internal/util"
)

func (a *apiClient) resolveOCSWhiteboard(coursewareInfo map[string]any, title string) (*extractor.MediaInfo, bool) {
	coursewareID := textValue(coursewareInfo, "coursewareId")
	if coursewareID == "" || a == nil || a.c == nil {
		return nil, false
	}
	headers := ocsHeadersFor(coursewareInfo)
	for _, endpoint := range ocsEndpoints(coursewareID, coursewareInfo) {
		body, err := a.c.GetString(endpoint, headers)
		if err != nil || strings.TrimSpace(body) == "" {
			continue
		}
		var payload any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			continue
		}
		if entry, ok := cctalkWhiteboardEntryFromPayload(payload, coursewareInfo, title, headers, endpoint); ok {
			if entry.Extra == nil {
				entry.Extra = map[string]any{}
			}
			entry.Extra["ocs_url"] = endpoint
			return entry, true
		}
	}
	return nil, false
}

func cctalkWhiteboardEntryFromPayload(payload any, coursewareInfo map[string]any, title string, headers map[string]string, baseURL string) (*extractor.MediaInfo, bool) {
	normalized := normalizeOCSPayload(payload)
	boardPayload, ok := shared.FindWhiteboardPayload(normalized, payload, coursewareInfo)
	if !ok {
		return nil, false
	}
	materialHost := CCTALK_OCS_MATERIAL_HOST
	if root := asMap(normalized); len(root) > 0 {
		if hosts := candidateHosts(root); len(hosts) > 0 {
			materialHost = hosts[0]
		}
	}
	timeline, ok := shared.BuildWhiteboardTimeline(boardPayload, shared.WhiteboardRenderOptions{Title: title, Site: "cctalk", MaterialHost: materialHost, BaseURL: baseURL})
	if !ok {
		return nil, false
	}
	resolveCCTalkWhiteboardTimelineURLs(&timeline, materialHost, baseURL)
	if !hasDrawableCCTalkWhiteboard(timeline) {
		return nil, false
	}
	streamExtra := map[string]any{
		"type":                       "whiteboard",
		"mode":                       "whiteboard_html",
		"playback_type":              "board",
		"courseware_id":              firstNonEmpty(textValue(coursewareInfo, "coursewareId"), textValue(asMap(normalized), "coursewareId")),
		"courseware_info":            coursewareInfo,
		"whiteboard_pages":           len(timeline.Pages),
		"whiteboard_events":          countWhiteboardTimelineEvents(timeline),
		"whiteboard_duration_ms":     timeline.DurationMS,
		"whiteboard_material_host":   materialHost,
		"whiteboard_playable_format": "html",
	}
	return &extractor.MediaInfo{
		Site:  "cctalk",
		Title: util.SanitizeFilename(firstNonEmpty(title, timeline.Title, "cctalk_whiteboard")),
		Streams: map[string]extractor.Stream{
			"whiteboard": {Quality: "whiteboard", URLs: []string{shared.HTMLDataURL(shared.WhiteboardPlayableHTML(timeline))}, Format: "html", Headers: headers, Extra: streamExtra},
		},
		Extra: streamExtra,
	}, true
}

func hasDrawableCCTalkWhiteboard(timeline shared.WhiteboardTimeline) bool {
	if len(timeline.Events) > 0 {
		return true
	}
	for _, page := range timeline.Pages {
		if len(page.Events) > 0 || strings.TrimSpace(page.BoardURL) != "" || strings.TrimSpace(page.ImageURL) != "" || strings.TrimSpace(page.BoardID) != "" || strings.TrimSpace(page.ImageID) != "" {
			return true
		}
	}
	return false
}

func resolveCCTalkWhiteboardTimelineURLs(timeline *shared.WhiteboardTimeline, materialHost, baseURL string) {
	if timeline == nil {
		return
	}
	resources := map[string]string{}
	for _, res := range timeline.Resources {
		if res.ID != "" && res.URL != "" {
			resources[res.ID] = res.URL
		}
	}
	for i := range timeline.Pages {
		if timeline.Pages[i].ImageURL == "" && timeline.Pages[i].ImageID != "" {
			timeline.Pages[i].ImageURL = resources[timeline.Pages[i].ImageID]
		}
		if timeline.Pages[i].BoardURL == "" && timeline.Pages[i].BoardID != "" {
			timeline.Pages[i].BoardURL = resources[timeline.Pages[i].BoardID]
		}
		timeline.Pages[i].ImageURL = normalizeCCTalkWhiteboardResourceURL(timeline.Pages[i].ImageURL, materialHost, baseURL)
		timeline.Pages[i].BoardURL = normalizeCCTalkWhiteboardResourceURL(timeline.Pages[i].BoardURL, materialHost, baseURL)
	}
}

func normalizeCCTalkWhiteboardResourceURL(raw, materialHost, baseURL string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "//") {
		return normalizeOCSResourceURL(raw)
	}
	if strings.HasPrefix(raw, "/") && strings.TrimSpace(materialHost) != "" {
		return strings.TrimRight(materialHost, "/") + raw
	}
	if baseURL != "" && strings.Contains(baseURL, "://") {
		return normalizeWhiteboardAgainstBase(raw, baseURL, materialHost)
	}
	if materialHost != "" {
		return strings.TrimRight(materialHost, "/") + "/" + strings.TrimLeft(raw, "/")
	}
	return normalizeOCSResourceURL(raw)
}

func normalizeWhiteboardAgainstBase(raw, baseURL, materialHost string) string {
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "data:") {
		return normalizeOCSResourceURL(raw)
	}
	// Keep CCTALK material host preference for bare resource paths; OCS API URLs
	// are not stable asset roots and often only identify the JSON endpoint.
	if materialHost != "" && !strings.HasPrefix(raw, "./") && !strings.HasPrefix(raw, "../") {
		return strings.TrimRight(materialHost, "/") + "/" + strings.TrimLeft(raw, "/")
	}
	return normalizeOCSResourceURL(raw)
}

func countWhiteboardTimelineEvents(timeline shared.WhiteboardTimeline) int {
	total := len(timeline.Events)
	for _, page := range timeline.Pages {
		total += len(page.Events)
	}
	return total
}
