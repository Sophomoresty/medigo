package nmkjxy

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Sophomoresty/mediago/internal/extractor"
)

func TestExtractMock(t *testing.T) {
	routes := goldenLoadRoutes(t)
	goldenInstallTransport(t, routes)
	jar := goldenNewJar(t)
	got, err := (&Nmkjxy{}).Extract("https://www.nmkjxy.com/course?courseId=1001", &extractor.ExtractOpts{Cookies: jar})
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	goldenAssertMedia(t, "nmkjxy", got)
	if len(got.Entries) != 2 {
		t.Fatalf("entries = %d, want 2: %#v", len(got.Entries), got.Entries)
	}
	assertEntryStream(t, got.Entries, "best", "https://media.example.com/nmkjxy/lesson.mp4", "mp4")
	assertEntryStream(t, got.Entries, "file", "https://media.example.com/nmkjxy/lecture.pdf", "pdf")
}

func goldenLoadRoutes(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile("testdata/sample.json")
	if err != nil {
		t.Fatalf("read sample fixture: %v", err)
	}
	routes := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &routes); err != nil {
		t.Fatalf("parse sample fixture: %v", err)
	}
	return routes
}

func goldenInstallTransport(t *testing.T, routes map[string]json.RawMessage) {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if raw, ok := goldenExactRoute(routes, r); ok {
			goldenWriteResponse(w, raw)
			return
		}
		if strings.HasSuffix(strings.ToLower(r.URL.Path), ".m3u8") {
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:4\n#EXTINF:4,\nsegment.ts\n#EXT-X-ENDLIST\n"))
			return
		}
		if raw, ok := routes["__default"]; ok {
			goldenWriteResponse(w, raw)
			return
		}
		http.Error(w, `{"code":404,"data":{}}`, http.StatusNotFound)
	})
	httpSrv := httptest.NewServer(handler)
	httpsSrv := httptest.NewTLSServer(handler)
	oldDefault := http.DefaultTransport
	base, ok := oldDefault.(*http.Transport)
	if !ok {
		t.Fatalf("http.DefaultTransport is %T, want *http.Transport", oldDefault)
	}
	tr := base.Clone()
	tr.Proxy = nil
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	httpAddr := httpSrv.Listener.Addr().String()
	httpsAddr := httpsSrv.Listener.Addr().String()
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		target := httpAddr
		if strings.HasSuffix(addr, ":443") {
			target = httpsAddr
		}
		return dialer.DialContext(ctx, network, target)
	}
	http.DefaultTransport = tr
	t.Cleanup(func() {
		http.DefaultTransport = oldDefault
		httpSrv.Close()
		httpsSrv.Close()
	})
}

func goldenExactRoute(routes map[string]json.RawMessage, r *http.Request) (json.RawMessage, bool) {
	for _, key := range []string{
		r.Method + " " + r.Host + r.URL.Path,
		r.Method + " " + r.URL.Path,
		r.Host + r.URL.Path,
		r.URL.Path,
	} {
		if raw, ok := routes[key]; ok {
			return raw, true
		}
	}
	return nil, false
}

func goldenWriteResponse(w http.ResponseWriter, raw json.RawMessage) {
	var body string
	if len(raw) > 0 && raw[0] == '"' && json.Unmarshal(raw, &body) == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(body))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(raw)
}

func goldenNewJar(t *testing.T) http.CookieJar {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	return jar
}

func goldenSetCookie(t *testing.T, jar http.CookieJar, rawURL, name, value string) {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse cookie URL %q: %v", rawURL, err)
	}
	jar.SetCookies(u, []*http.Cookie{{Name: name, Value: value, Path: "/"}})
}

func goldenAssertMedia(t *testing.T, site string, got *extractor.MediaInfo) {
	t.Helper()
	if got == nil {
		t.Fatalf("Extract returned nil MediaInfo")
	}
	if got.Site != site {
		t.Fatalf("Site = %q, want %q", got.Site, site)
	}
	if len(got.Streams) == 0 && len(got.Entries) == 0 {
		t.Fatalf("MediaInfo has no streams or entries: %#v", got)
	}
	for i, entry := range got.Entries {
		if entry == nil {
			t.Fatalf("Entries[%d] is nil", i)
		}
		if entry.Site != site {
			t.Fatalf("Entries[%d].Site = %q, want %q", i, entry.Site, site)
		}
		if len(entry.Streams) == 0 && len(entry.Entries) == 0 {
			t.Fatalf("Entries[%d] has no streams or child entries: %#v", i, entry)
		}
	}
}

func TestParseCoursewareGroups(t *testing.T) {
	got := parseCoursewareGroups(map[string]any{
		"groups": []any{
			map[string]any{
				"title": "讲义A",
				"files": []any{
					map[string]any{
						"coursewareUrl":  "/courseware/a.pdf",
						"coursewareName": "讲义A.pdf",
						"fileSize":       "2048",
					},
				},
			},
		},
	}, 1)
	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1: %#v", len(got), got)
	}
	assertEntryStream(t, got, "file", "https://www.nmkjxy.com/courseware/a.pdf", "pdf")
	if got[0].Title != "(1.1.1)--讲义A" {
		t.Fatalf("Title = %q, want %q", got[0].Title, "(1.1.1)--讲义A")
	}
}

func TestParseLegacyCoursewareFiles(t *testing.T) {
	got := parseLegacyCoursewareFiles(map[string]any{
		"data": []any{
			map[string]any{
				"chapterSn": "2",
				"fileUrl":   "https://cdn.example.com/legacy.docx",
				"fileName":  "Legacy.docx",
				"size":      json.Number("4096"),
			},
		},
	}, 1)
	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1: %#v", len(got), got)
	}
	assertEntryStream(t, got, "file", "https://cdn.example.com/legacy.docx", "docx")
	if got[0].Title != "(1.2.1)--Legacy" {
		t.Fatalf("Title = %q, want %q", got[0].Title, "(1.2.1)--Legacy")
	}
}

func assertEntryStream(t *testing.T, entries []*extractor.MediaInfo, key, wantURL, wantFormat string) {
	t.Helper()
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		stream, ok := entry.Streams[key]
		if !ok || len(stream.URLs) == 0 {
			continue
		}
		if stream.URLs[0] != wantURL {
			continue
		}
		if stream.Format != wantFormat {
			t.Fatalf("stream format = %q, want %q: %#v", stream.Format, wantFormat, stream)
		}
		if stream.Quality != "source" && key == "file" {
			t.Fatalf("file stream quality = %q, want source", stream.Quality)
		}
		return
	}
	t.Fatalf("entry with stream %q url %q not found in %#v", key, wantURL, entries)
}
