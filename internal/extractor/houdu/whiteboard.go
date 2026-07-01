package houdu

import (
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/extractor/shared"
)

func (x *hdCtx) renderHouduWhiteboard(title string, src hdSource) (extractor.Stream, map[string]any, bool) {
	if src.Extra == nil || src.Extra["whiteboard"] != true {
		return extractor.Stream{}, nil, false
	}
	apiURL := firstNonEmpty(anyString(src.Extra["whiteboard_api_url"]), src.URL)
	if apiURL == "" {
		return extractor.Stream{}, nil, false
	}
	headers := map[string]string{"Referer": referer, "User-Agent": USER_AGENT}
	if x.cookie != "" {
		headers["Cookie"] = x.cookie
	}
	if x.token != "" {
		headers["Authorization"] = x.token
	}
	opts := shared.WhiteboardRenderOptions{Title: houduBoardTitle(title), Site: "houdu", BaseURL: apiURL, Headers: headers, IncludePNG: true}
	export, ok := shared.RenderWhiteboardFromURL(x.c, apiURL, opts)
	if !ok {
		export, ok = shared.RenderWhiteboardFromPayload(x.c, apiURL, opts)
	}
	if !ok {
		return extractor.Stream{}, nil, false
	}
	extra := shared.WhiteboardExportExtra(export)
	extra["whiteboard_type"] = firstNonEmpty(anyString(src.Extra["whiteboard_type"]), "houdu")
	extra["whiteboard_renderer"] = "medigo_html_whiteboard"
	extra["whiteboard_referer"] = referer
	extra["whiteboard_html_url"] = export.URL
	extra["whiteboard_data_url"] = apiURL
	if src.Extra["whiteboard_params"] != nil {
		extra["whiteboard_params"] = src.Extra["whiteboard_params"]
	}
	if src.Extra["whiteboard_source"] != nil {
		extra["whiteboard_source"] = src.Extra["whiteboard_source"]
	}
	stream := extractor.Stream{Quality: "whiteboard", URLs: []string{export.URL}, Format: "html", Headers: headers, Extra: cloneAnyMap(extra)}
	return stream, extra, true
}

func houduBoardTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "houdu_board_板书"
	}
	if strings.HasSuffix(title, "_板书") {
		return title
	}
	return title + "_板书"
}

func anyString(v any) string {
	s := strings.TrimSpace(str(v))
	if s == "" || s == "<nil>" {
		return ""
	}
	return s
}
