package enetedu

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
	"github.com/Sophomoresty/mediago/internal/util"
)

func TestExtractMock(t *testing.T) {
	fixture := mustReadFixture(t, "testdata/sample.json")
	installEneteduMockTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/admin-api/site/user/baseinfo":
			_, _ = w.Write([]byte(`{"code":200,"data":{"id":"u1","name":"tester"}}`))
		case "/admin-api/course/broadcast/glanceAndGet":
			_, _ = w.Write(fixture)
		case "/admin-api/course/broadcast/task/homeView":
			_, _ = w.Write([]byte(`{"code":200,"data":[{"name":"Live 1","realId":"node-1","sourceAddress":"https://cdn.example/enetedu-live.m3u8"}]}`))
		case "/admin-api/media/course-learning-info/learningCourseTreeList":
			_, _ = w.Write([]byte(`{"code":200,"data":[{"fileName":"Lesson 2","videoId":"v2","chapterId":"c2"}]}`))
		case "/admin-api/media/course-info/getMediaTranscodeInfo":
			_, _ = w.Write([]byte(`{"code":200,"data":{"transcodeOutputs":{"list":[{"playUrl":"https://cdn.example/enetedu-v2.mp4"}]}}}`))
		case "/admin-api/media/course-resources/courseFileList":
			_, _ = w.Write([]byte(`{"code":200,"data":[{"fileName":"Slides","filePath":"https://cdn.example/slides.pdf"}]}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))

	jar, _ := cookiejar.New(nil)
	setCookie(t, jar, "https://www.enetedu.com/", &http.Cookie{Name: token_key, Value: "en-token"})
	testURL := "https://www.enetedu.com/site/course/liveCourseDetails?id=course-1"
	if _, err := extractor.Match(testURL); err != nil {
		t.Fatalf("test URL should match extractor pattern: %v", err)
	}

	info, err := (&Enetedu{}).Extract(testURL, &extractor.ExtractOpts{Cookies: jar})
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if info.Site != "enetedu" || len(info.Entries) < 3 {
		t.Fatalf("unexpected media info: %#v", info)
	}
	got := firstEneteduPlayableURL(info)
	if !strings.Contains(got, "cdn.example") {
		t.Fatalf("playable URL = %q, want fixture CDN", got)
	}
}

func TestExtractRequiresAuth(t *testing.T) {
	_, err := (&Enetedu{}).Extract("https://www.enetedu.com/site/course/liveCourseDetails?id=course-1", &extractor.ExtractOpts{})
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "requires") {
		t.Fatalf("error = %v, want auth error", err)
	}
}

func mustReadFixture(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if !json.Valid(b) {
		t.Fatalf("fixture %s is not valid json", path)
	}
	return b
}

func installEneteduMockTransport(t *testing.T, handler http.Handler) {
	t.Helper()
	plain := httptest.NewServer(handler)
	tlsSrv := httptest.NewTLSServer(handler)
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected default transport type %T", http.DefaultTransport)
	}
	oldTransport := baseTransport.Clone()
	oldProxy := util.DefaultProxy()
	if err := util.SetDefaultProxy(""); err != nil {
		t.Fatal(err)
	}
	plainURL, _ := url.Parse(plain.URL)
	tlsURL, _ := url.Parse(tlsSrv.URL)
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	tr := oldTransport.Clone()
	tr.Proxy = nil
	tr.ForceAttemptHTTP2 = false
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, plainURL.Host)
	}
	tr.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		raw, err := dialer.DialContext(ctx, network, tlsURL.Host)
		if err != nil {
			return nil, err
		}
		conn := tls.Client(raw, &tls.Config{InsecureSkipVerify: true})
		if err := conn.HandshakeContext(ctx); err != nil {
			raw.Close()
			return nil, err
		}
		return conn, nil
	}
	http.DefaultTransport = tr
	t.Cleanup(func() {
		http.DefaultTransport = oldTransport
		_ = util.SetDefaultProxy(oldProxy)
		plain.Close()
		tlsSrv.Close()
	})
}

func setCookie(t *testing.T, jar http.CookieJar, raw string, cookies ...*http.Cookie) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(u, cookies)
}

func firstEneteduPlayableURL(mi *extractor.MediaInfo) string {
	if mi == nil {
		return ""
	}
	for _, stream := range mi.Streams {
		if len(stream.URLs) > 0 && strings.TrimSpace(stream.URLs[0]) != "" {
			return stream.URLs[0]
		}
	}
	for _, entry := range mi.Entries {
		if u := firstEneteduPlayableURL(entry); u != "" {
			return u
		}
	}
	return ""
}
