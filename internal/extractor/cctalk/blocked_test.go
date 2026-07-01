package cctalk

import (
	"strings"
	"testing"
)

func TestBoardWithoutPayloadIsBlocked(t *testing.T) {
	item := map[string]any{
		"lessonName":   "白板课时",
		"coursewareId": "cw-board-1",
		"sourceType":   "board",
	}
	entries := entriesFromMap(&apiClient{headers: baseHeaders()}, item, "课时1")
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if len(e.Streams) != 0 {
		t.Fatalf("blocked board entry should have no streams, got %#v", e.Streams)
	}
	if e.Extra["blocked"] != true || e.Extra["playback_type"] != "board" {
		t.Fatalf("expected blocked board entry, got Extra=%#v", e.Extra)
	}
	if reason, _ := e.Extra["block_reason"].(string); !strings.Contains(reason, "白板") || !strings.Contains(reason, "HTML") {
		t.Fatalf("board block reason should mention missing whiteboard HTML export data, got %q", reason)
	}
}

func TestBoardPayloadExportsWhiteboardHTML(t *testing.T) {
	item := map[string]any{
		"lessonName": "白板课时(OCS)",
		"sourceType": "board",
		"content": `<courseware totalTime="3000">
			<style width="1024" height="768"/>
			<resources>
				<res id="img1" url="/images/page1.png" type="image"/>
				<res id="wb1" url="/boards/page1.json" type="whiteboard"/>
			</resources>
			<normalPage number="1" startTime="0" endTime="3000">
				<image res="img1"><style x="0" y="0" width="1024" height="768"/></image>
				<whiteboard res="wb1"><style x="0" y="0" width="1024" height="768"/></whiteboard>
			</normalPage>
		</courseware>`,
		"cdnHosts": []any{"https://cdn.example.com/material"},
	}
	entries := entriesFromMap(&apiClient{headers: baseHeaders()}, item, "课时1")
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Extra["blocked"] == true {
		t.Fatalf("board payload with whiteboard XML must not be blocked, got Extra=%#v", e.Extra)
	}
	wb, ok := e.Streams["whiteboard"]
	if !ok {
		t.Fatalf("whiteboard stream missing: %#v", e.Streams)
	}
	if wb.Format != "html" || len(wb.URLs) != 1 || !strings.HasPrefix(wb.URLs[0], "data:text/html") {
		t.Fatalf("whiteboard stream = %#v", wb)
	}
	if e.Extra["whiteboard_pages"] != 1 || e.Extra["whiteboard_playable_format"] != "html" {
		t.Fatalf("whiteboard extra = %#v", e.Extra)
	}
}

func TestBoardWithM3U8ResolvesStreamNotBlocked(t *testing.T) {
	item := map[string]any{
		"lessonName":   "白板课时(可流式)",
		"coursewareId": "cw-board-2",
		"sourceType":   "board",
		"m3u8s": []any{
			map[string]any{
				"resourceId": "board-2",
				"content":    "#EXTM3U\n#EXTINF:10,\nseg0.ts\n",
			},
		},
		"cdnHosts": []any{"https://cdn.example.com/root"},
	}
	entries := entriesFromMap(&apiClient{headers: baseHeaders()}, item, "课时1")
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Extra["blocked"] == true {
		t.Fatalf("board lesson with m3u8s must NOT be blocked, got Extra=%#v", e.Extra)
	}
	best := e.Streams["best"]
	if best.Format != "m3u8" || len(best.URLs) == 0 {
		t.Fatalf("expected resolvable m3u8 stream, got %#v", e.Streams)
	}
}

func TestUnavailableLiveReplayIsBlocked(t *testing.T) {
	item := map[string]any{
		"lessonName":        "直播课时",
		"contentId":         "12345678",
		"forecastStartDate": "2026-07-01 20:00:00",
		"reviewStatus":      "0",
	}
	entries := entriesFromMap(&apiClient{headers: baseHeaders()}, item, "课时1")
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if len(e.Streams) != 0 {
		t.Fatalf("blocked live entry should have no streams, got %#v", e.Streams)
	}
	if e.Extra["blocked"] != true || e.Extra["playback_type"] != "live" {
		t.Fatalf("expected blocked live entry, got Extra=%#v", e.Extra)
	}
	if reason, _ := e.Extra["block_reason"].(string); !strings.Contains(reason, "回放") {
		t.Fatalf("live block reason should mention replay, got %q", reason)
	}
}

func TestPlayableVideoNotBlocked(t *testing.T) {
	item := map[string]any{
		"lessonName": "普通视频",
		"videoUrl":   "https://cdn.example.com/v/play.mp4",
	}
	entries := entriesFromMap(&apiClient{headers: baseHeaders()}, item, "课时1")
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Extra["blocked"] == true {
		t.Fatalf("playable video must not be blocked, got Extra=%#v", e.Extra)
	}
	if best := e.Streams["best"]; len(best.URLs) == 0 || best.URLs[0] != "https://cdn.example.com/v/play.mp4" {
		t.Fatalf("expected mp4 stream, got %#v", e.Streams)
	}
}

func TestLiveReplayWithMediaIsNotBlocked(t *testing.T) {
	// A live lesson whose replay is published exposes a media URL and must
	// flow through the normal VOD path, not the unavailable-replay block.
	item := map[string]any{
		"lessonName":        "直播课时(已生成回放)",
		"contentId":         "87654321",
		"forecastStartDate": "2026-06-01 20:00:00",
		"reviewStatus":      "0",
		"videoUrl":          "https://cdn.example.com/replay/live.m3u8",
	}
	if isUnavailableReplay(item) {
		t.Fatal("replay with a media URL must not be classified unavailable")
	}
	entries := entriesFromMap(&apiClient{headers: baseHeaders()}, item, "课时1")
	if len(entries) != 1 || entries[0].Extra["blocked"] == true {
		t.Fatalf("published replay must not be blocked, got %#v", entries)
	}
}
