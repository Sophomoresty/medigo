package mddclass

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestMddclassBuildOCSStreamRendersBoardPayload(t *testing.T) {
	boardJSON := base64.StdEncoding.EncodeToString([]byte(`{"whiteBoardPen":[{"drawtime":100,"points":"0,0 100,100","color":"#000000","pen":2}]}`))
	boardXML := `<courseware name="板书" totalTime="1000"><style width="640" height="480"/><resources><res id="wb1" url="data:application/json;base64,` + boardJSON + `"/></resources><normalPage number="1" startTime="0" endTime="1000"><elements><whiteboard res="wb1" width="640" height="480"/></elements></normalPage></courseware>`
	stream, extra, ok := mddclassBuildOCSStream(map[string]any{"content": boardXML, "coursewareType": "3", "coursewareId": "cw-board"}, map[string]any{"coursewareId": "cw-board"}, map[string]string{"Referer": mddclassOCSReferer})
	if !ok {
		t.Fatal("mddclassBuildOCSStream did not recognize board payload")
	}
	if stream.Format != "html" || len(stream.URLs) != 1 || !strings.HasPrefix(stream.URLs[0], "data:text/html") {
		t.Fatalf("stream = %#v, want html data URL", stream)
	}
	if extra["mode"] != "board" || extra["media_type"] != "board" {
		t.Fatalf("extra = %#v, want board mode", extra)
	}
	wb, ok := extra["whiteboard"].(map[string]any)
	if !ok || wb["rendered"] != true || wb["event_count"] == nil {
		t.Fatalf("whiteboard metadata missing: %#v", extra["whiteboard"])
	}
}
