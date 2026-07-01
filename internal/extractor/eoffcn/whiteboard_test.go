package eoffcn

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sophomoresty/mediago/internal/util"
)

func TestHydrateEoffcnWhiteboardFetchesPCVodAndBuildsHTMLStream(t *testing.T) {
	var gotReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReferer = r.Header.Get("Referer")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"items":[{"Url":"/board/page1.wbx","Tm":1200,"Idx":2}],"imageUrl":"/images/page1.png","fontUrl":"/font.ttf","audioUrl":"/audio/index.m3u8"}}`))
	}))
	defer srv.Close()

	c := util.NewClient()
	apiURL := srv.URL + "/pcvod?room_num=RN&room_id=RID&account=acct&k=key"
	playback := eoffcnPlayback{
		URL: apiURL,
		Whiteboard: eoffcnWhiteboardInfo{
			Whiteboard: true,
			APIURL:     apiURL,
			Params:     map[string]string{"room_num": "RN", "room_id": "RID"},
			Source:     "test",
		},
	}

	playback = hydrateEoffcnWhiteboardPlayback(c, map[string]string{"Referer": "https://www.eoffcn.com"}, "白板课", playback)
	if gotReferer != eoffcnBoardReferer {
		t.Fatalf("pcvod Referer = %q, want %q", gotReferer, eoffcnBoardReferer)
	}
	if !strings.HasPrefix(playback.URL, "data:text/html") {
		t.Fatalf("playback URL = %.40q, want html data URL", playback.URL)
	}
	if playback.Extra["whiteboard_fetched"] != true {
		t.Fatalf("whiteboard_fetched = %#v, want true", playback.Extra["whiteboard_fetched"])
	}
	manifest, ok := playback.Extra["whiteboard_manifest"].(map[string]any)
	if !ok {
		t.Fatalf("whiteboard_manifest = %#v", playback.Extra["whiteboard_manifest"])
	}
	board, ok := manifest["eoffcn"].(map[string]any)
	if !ok {
		t.Fatalf("manifest eoffcn = %#v", manifest["eoffcn"])
	}
	wbx, ok := board["wbx"].([]map[string]any)
	if !ok || len(wbx) != 1 {
		t.Fatalf("wbx = %#v", board["wbx"])
	}
	if wbx[0]["src"] != srv.URL+"/board/page1.wbx" || wbx[0]["atMs"] != 1200 || wbx[0]["idx"] != 2 {
		t.Fatalf("wbx[0] = %#v", wbx[0])
	}
	if htmlURL, _ := playback.Extra["whiteboard_html_url"].(string); !strings.HasPrefix(htmlURL, "data:text/html") {
		t.Fatalf("whiteboard_html_url = %.40q", htmlURL)
	}
	htmlDoc := buildEoffcnWhiteboardHTML("白板课", manifest)
	if !strings.Contains(htmlDoc, "__MEDIGO_WHITEBOARD_PLAYER__") || !strings.Contains(htmlDoc, "saveFrameImage") {
		t.Fatalf("whiteboard html should expose frame export API")
	}
}

func TestHydrateEoffcnWhiteboardExtractsRelativeAssetsFromHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><script>window.assets=["/board/page2.wbx?token=1","/images/bg.jpg","/font/board.woff2"]</script></body></html>`))
	}))
	defer srv.Close()

	apiURL := srv.URL + "/pcvod.html"
	playback := hydrateEoffcnWhiteboardPlayback(util.NewClient(), nil, "白板课", eoffcnPlayback{
		URL: apiURL,
		Whiteboard: eoffcnWhiteboardInfo{
			Whiteboard: true,
			APIURL:     apiURL,
		},
	})
	manifest, ok := playback.Extra["whiteboard_manifest"].(map[string]any)
	if !ok {
		t.Fatalf("whiteboard_manifest = %#v", playback.Extra["whiteboard_manifest"])
	}
	board, ok := manifest["eoffcn"].(map[string]any)
	if !ok {
		t.Fatalf("manifest eoffcn = %#v", manifest["eoffcn"])
	}
	wbx, ok := board["wbx"].([]map[string]any)
	if !ok || len(wbx) != 1 {
		t.Fatalf("wbx = %#v", board["wbx"])
	}
	if wbx[0]["src"] != srv.URL+"/board/page2.wbx?token=1" {
		t.Fatalf("relative wbx src = %#v", wbx[0]["src"])
	}
	if font, ok := board["font"].(map[string]any); !ok || font["src"] != srv.URL+"/font/board.woff2" {
		t.Fatalf("font = %#v", board["font"])
	}
}

func TestEoffcnMediaInfoAddsWhiteboardHTMLSideStream(t *testing.T) {
	info := mediaInfoWithExtra("白板+视频", "https://cdn.example.com/replay.m3u8", map[string]string{"Referer": "https://www.eoffcn.com"}, map[string]any{
		"whiteboard":          true,
		"whiteboard_api_url":  "https://pcvod.offcncloud.com/replay",
		"whiteboard_html_url": "data:text/html;charset=utf-8,%3Chtml%3E%3C/html%3E",
	})
	if _, ok := info.Streams["best"]; !ok {
		t.Fatalf("best stream missing: %#v", info.Streams)
	}
	wb, ok := info.Streams["whiteboard"]
	if !ok {
		t.Fatalf("whiteboard stream missing: %#v", info.Streams)
	}
	if wb.Format != "html" || !strings.HasPrefix(wb.URLs[0], "data:text/html") {
		t.Fatalf("whiteboard stream = %#v", wb)
	}
}
