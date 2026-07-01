package houdu

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/Sophomoresty/mediago/internal/util"
)

func TestHouduExtractPlaybackMarksWhiteboardMedia(t *testing.T) {
	x := &hdCtx{token: "mini-token"}
	pb := x.extractPlayback(map[string]any{
		"data": map[string]any{
			"whiteboard": true,
			"play_info": map[string]any{
				"720p": map[string]any{
					"url": "https://cdn.example.com/replay/index.m3u8",
				},
			},
			"board_url": "https://example.com/whiteboard?room_id=RID&token=board-token",
		},
	})

	if pb.URL != "https://cdn.example.com/replay/index.m3u8?miniToken=mini-token" {
		t.Fatalf("URL = %q, want playable m3u8 with miniToken", pb.URL)
	}
	if pb.Format != "m3u8" || !pb.NeedMerge {
		t.Fatalf("format/merge = %s/%v, want m3u8/true", pb.Format, pb.NeedMerge)
	}
	if pb.Extra["whiteboard"] != true {
		t.Fatalf("whiteboard extra = %#v, want true", pb.Extra["whiteboard"])
	}
}

func TestHouduBoardOnlyPlaybackFromBaijiayunParams(t *testing.T) {
	pb := houduBoardOnlyPlayback("playback", map[string]any{
		"room_id":    "ROOM-ID",
		"token":      "play-token",
		"whiteboard": true,
	}, detectHouduWhiteboard(map[string]any{"whiteboard": true}))

	want := "https://h5.houduweilai.com/live/play?room_id=ROOM-ID&token=play-token"
	if pb.URL != want {
		t.Fatalf("URL = %q, want %q", pb.URL, want)
	}
	if pb.Format != "html" {
		t.Fatalf("format = %q, want html", pb.Format)
	}
	if pb.Extra["whiteboard"] != true {
		t.Fatalf("whiteboard extra = %#v, want true", pb.Extra["whiteboard"])
	}
	params, ok := pb.Extra["whiteboard_params"].(map[string]string)
	if !ok {
		t.Fatalf("whiteboard_params = %#v, want map[string]string", pb.Extra["whiteboard_params"])
	}
	if params["room_id"] != "ROOM-ID" || params["token"] != "play-token" {
		t.Fatalf("whiteboard params = %#v", params)
	}
}

func TestHouduMediaFromSourcesCopiesStreamExtra(t *testing.T) {
	x := &hdCtx{cookie: "c=v", token: "auth"}
	info, err := x.mediaFromSources([]hdSource{{
		Name:   "board",
		URL:    "https://h5.houduweilai.com/live/play?room_id=ROOM-ID&token=play-token",
		Kind:   "video",
		Format: "html",
		Extra:  map[string]any{"whiteboard": true},
	}})
	if err != nil {
		t.Fatalf("mediaFromSources returned error: %v", err)
	}
	stream := info.Entries[0].Streams["best"]
	if stream.Extra["whiteboard"] != true {
		t.Fatalf("stream extra whiteboard = %#v, want true", stream.Extra["whiteboard"])
	}
	if info.Entries[0].Extra["whiteboard"] != true {
		t.Fatalf("media extra whiteboard = %#v, want true", info.Entries[0].Extra["whiteboard"])
	}
}

func TestHouduMediaFromSourcesRendersWhiteboardDataURL(t *testing.T) {
	oldProxy := util.DefaultProxy()
	t.Cleanup(func() { _ = util.SetDefaultProxy(oldProxy) })
	if err := util.SetDefaultProxy(""); err != nil {
		t.Fatal(err)
	}
	boardXML := `<courseware totalTime="1000"><style width="640" height="480"/><resources><res id="wb1" url="data:application/json;base64,eyJ3aGl0ZUJvYXJkUGVuIjpbeyJkcmF3dGltZSI6MTAwLCJwb2ludHMiOiIwLDAgMTAwLDEwMCIsImNvbG9yIjoiIzAwMDAwMCIsInBlbiI6Mn1dfQ=="/></resources><normalPage number="1" startTime="0" endTime="1000"><elements><whiteboard res="wb1" width="640" height="480"/></elements></normalPage></courseware>`
	x := &hdCtx{c: util.NewClient(), cookie: "c=v", token: "auth"}
	info, err := x.mediaFromSources([]hdSource{{Name: "board", URL: "data:text/xml;base64," + base64.StdEncoding.EncodeToString([]byte(boardXML)), Kind: "video", Format: "html", Extra: map[string]any{"whiteboard": true}}})
	if err != nil {
		t.Fatalf("mediaFromSources returned error: %v", err)
	}
	stream := info.Entries[0].Streams["best"]
	if stream.Format != "html" || len(stream.URLs) != 1 || !strings.HasPrefix(stream.URLs[0], "data:text/html") {
		t.Fatalf("best stream = %#v, want rendered html data URL", stream)
	}
	if info.Entries[0].Extra["rendered"] != true || info.Entries[0].Extra["event_count"] == nil {
		t.Fatalf("render metadata missing: %#v", info.Entries[0].Extra)
	}
}
