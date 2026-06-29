package yikaobang

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

	"github.com/Sophomoresty/mediago/internal/extractor"
)

func loadGoldenFixture(t *testing.T) []byte {
	t.Helper()
	fixture, err := os.ReadFile("testdata/sample.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if !json.Valid(fixture) {
		t.Fatalf("fixture is not valid JSON: %s", fixture)
	}
	return fixture
}

func installMockTransport(t *testing.T, httpURL, httpsURL string) {
	t.Helper()
	httpTarget, err := url.Parse(httpURL)
	if err != nil {
		t.Fatalf("parse HTTP mock server URL: %v", err)
	}
	httpsTarget, err := url.Parse(httpsURL)
	if err != nil {
		t.Fatalf("parse HTTPS mock server URL: %v", err)
	}
	previous := http.DefaultTransport
	base, ok := previous.(*http.Transport)
	if !ok {
		t.Fatalf("default transport has unexpected type %T", previous)
	}
	tr := base.Clone()
	tr.Proxy = nil
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		d := &net.Dialer{}
		return d.DialContext(ctx, network, httpTarget.Host)
	}
	tr.DialTLSContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		d := &tls.Dialer{NetDialer: &net.Dialer{}, Config: &tls.Config{InsecureSkipVerify: true}}
		return d.DialContext(ctx, network, httpsTarget.Host)
	}
	http.DefaultTransport = tr
	t.Cleanup(func() { http.DefaultTransport = previous })
}

func containsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func assertGoldenOutcome(t *testing.T, media *extractor.MediaInfo, err error) {
	t.Helper()
	if err != nil {
		msg := strings.ToLower(err.Error())
		allowed := []string{"yikaobang", "login", "cookie", "auth", "blocked", "rejected", "cannot parse", "parse", "invalid character", "no playable", "no media", "empty", "failed", "requires", "required", "not found", "missing", "token"}
		if !containsAny(msg, allowed) {
			t.Fatalf("unexpected extractor error: %v", err)
		}
		return
	}
	if media == nil {
		t.Fatalf("Extract returned nil MediaInfo without error")
	}
	if media.Site != "yikaobang" {
		t.Fatalf("Site = %q, want yikaobang", media.Site)
	}
	if len(media.Streams) == 0 && len(media.Entries) == 0 {
		t.Fatalf("MediaInfo has no streams or entries: %#v", media)
	}
}

func TestExtractMock(t *testing.T) {
	fixture := loadGoldenFixture(t)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(fixture)
	})
	httpSrv := httptest.NewServer(handler)
	defer httpSrv.Close()
	httpsSrv := httptest.NewTLSServer(handler)
	defer httpsSrv.Close()
	installMockTransport(t, httpSrv.URL, httpsSrv.URL)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("new cookie jar: %v", err)
	}
	setYKBTestToken(t, jar)

	media, err := (&Yikaobang{}).Extract("https://www.yikaobang.com.cn/course/1001", &extractor.ExtractOpts{Cookies: jar})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	assertGoldenOutcome(t, media, err)
	if len(media.Entries) != 2 {
		t.Fatalf("Entries = %d, want 2 video/file entries: %#v", len(media.Entries), media)
	}
	if media.Entries[0].Streams["best"].Format != "m3u8" {
		t.Fatalf("first entry format = %q, want m3u8", media.Entries[0].Streams["best"].Format)
	}
	if media.Entries[1].Streams["file"].Format != "pdf" {
		t.Fatalf("second entry format = %q, want pdf", media.Entries[1].Streams["file"].Format)
	}
	if got := goldenFirstPlayableURL(media); got != "https://cdn.example.com/ykb/intro.m3u8" {
		t.Fatalf("first playable URL = %q, want %q", got, "https://cdn.example.com/ykb/intro.m3u8")
	}
}

func goldenFirstPlayableURL(media *extractor.MediaInfo) string {
	if media == nil {
		return ""
	}
	for _, stream := range media.Streams {
		if len(stream.URLs) > 0 && strings.TrimSpace(stream.URLs[0]) != "" {
			return strings.TrimSpace(stream.URLs[0])
		}
	}
	for _, entry := range media.Entries {
		if got := goldenFirstPlayableURL(entry); got != "" {
			return got
		}
	}
	return ""
}

func setYKBTestToken(t *testing.T, jar http.CookieJar) {
	t.Helper()
	for _, raw := range []string{ykbHomeURL, ykbLegacyAPIBase, ykbNewAPIBase, ykbH5Base} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse cookie origin %q: %v", raw, err)
		}
		jar.SetCookies(u, []*http.Cookie{{Name: "token", Value: "test-token"}})
	}
}

func TestParseFixture(t *testing.T) {
	fixture := loadGoldenFixture(t)
	root, err := decodeYikaobangBody(string(fixture))
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	result := parseYikaobangPayloads([]ykbPayload{{Source: "https://new-ykb.yikaobang.com.cn/course/center/catalogue", Root: root, Body: string(fixture)}}, ykbTarget{CourseID: "1001"})
	if len(result.Courses) != 1 {
		t.Fatalf("courses = %d, want 1: %#v", len(result.Courses), result.Courses)
	}
	if len(result.Videos) != 1 {
		t.Fatalf("videos = %d, want 1: %#v", len(result.Videos), result.Videos)
	}
	if len(result.Files) != 1 {
		t.Fatalf("files = %d, want 1: %#v", len(result.Files), result.Files)
	}
	if len(result.Chapters) == 0 {
		t.Fatalf("expected chapters from fixture")
	}
}
