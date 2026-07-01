package mddclass

import (
	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/extractor/shared"
)

func mddclassBuildOCSWhiteboardStream(payload any, coursewareInfo map[string]any, headers map[string]string) (extractor.Stream, map[string]any, bool) {
	export, ok := shared.RenderWhiteboardFromPayload(nil, payload, shared.WhiteboardRenderOptions{Title: mddclassFirstText(coursewareInfo["title"], coursewareInfo["coursewareName"], coursewareInfo["name"], "mddclass_board_板书"), Site: "mddclass", MaterialHost: mddclassOCSMaterialHost, Headers: headers, IncludePNG: true})
	if !ok {
		return extractor.Stream{}, nil, false
	}
	extra := shared.WhiteboardExportExtra(export)
	extra["mode"] = "board"
	extra["media_type"] = "board"
	extra["courseware_info"] = coursewareInfo
	stream := extractor.Stream{Quality: "whiteboard", URLs: []string{export.URL}, Format: "html", Headers: headers, Extra: extra}
	return stream, map[string]any{"mode": "board", "media_type": "board", "payload": payload, "decrypted_payload": payload, "courseware_info": coursewareInfo, "whiteboard": extra}, true
}
