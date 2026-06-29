package baijiayunxiao

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

func TestExtractMock(t *testing.T) {
	fixture := readGoldenFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()
	assertFixtureServed(t, srv.URL, fixture)

	ext, err := extractor.Match("https://www.baijiayun.com/course/1001")
	if err != nil {
		t.Fatalf("extractor pattern should match fixture URL: %v", err)
	}
	info, err := ext.Extract("https://www.baijiayun.com/course/1001", nil)
	if err == nil {
		t.Fatalf("expected login-cookie error, got info: %#v", info)
	}
	if info != nil {
		t.Fatalf("expected nil MediaInfo on auth error, got %#v", info)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "requires login cookies") {
		t.Fatalf("expected explicit auth error, got %v", err)
	}
}

func TestParseCourseURLCustomBjyxDomain(t *testing.T) {
	cu, ok := parseCourseURL("https://huayan.fashuoschool.com/course/476?type=bjyx")
	if !ok {
		t.Fatal("expected custom bjyx course URL to be accepted")
	}
	if cu.domain != "huayan.fashuoschool.com" || cu.cid != "476" {
		t.Fatalf("courseURL = %#v", cu)
	}
	if _, ok := parseCourseURL("https://example.com/course/476"); ok {
		t.Fatal("generic non-bjyx course URL should remain rejected")
	}
}

func TestBaijiayunxiaoM3U8RequiresMerge(t *testing.T) {
	info := mediaInfo("baijiayunxiao", "lesson", "https://cdn.example.com/a.m3u8", "m3u8", nil)
	stream, ok := info.Streams["best"]
	if !ok {
		t.Fatal("missing best stream")
	}
	if !stream.NeedMerge {
		t.Fatal("m3u8 stream should require merge")
	}
}

func TestYunduanEntryCoursePlayback(t *testing.T) {
	counts := map[string]int{}
	handler := yunduanMockHandler(t, counts, false)
	installBaijiayunxiaoMockTransport(t, handler)

	jar := yunduanTestJar(t)
	setYunduanCookie(t, jar, "https://demo.at.baijiayun.com/", "ORGSUPERSESSID", "org-session")

	info, err := (&Baijiayunxiao{}).Extract("https://www.baijiayun.com/entry", &extractor.ExtractOpts{Cookies: jar})
	if err != nil {
		t.Fatalf("extract yunduan entry: %v", err)
	}
	if info.Title != "云端入口课程" {
		t.Fatalf("title = %q", info.Title)
	}
	if len(info.Entries) != 2 {
		t.Fatalf("entries = %d, want 2: %#v counts=%#v", len(info.Entries), info.Entries, counts)
	}
	assertYunduanEntryURL(t, info.Entries, "https://cdn.example.com/room-course-1.m3u8")
	assertYunduanEntryURL(t, info.Entries, "https://cdn.example.com/room-course-2.m3u8")
	for _, key := range []string{
		"www.baijiayun.com/entry",
		"demo.at.baijiayun.com/org/account/getUserInfo",
		"demo.at.baijiayun.com/org/course_playback/getCourseList",
		"demo.at.baijiayun.com/org/course_playback/getLessonList",
		"demo.at.baijiayun.com/org/course_playback/getRecentList",
		"api.baijiayun.com/web/playback/getPlayInfo",
	} {
		if counts[key] == 0 {
			t.Fatalf("route %s was not called; counts=%#v", key, counts)
		}
	}
}

func TestYunduanClassPlaybackChain(t *testing.T) {
	counts := map[string]int{}
	handler := yunduanMockHandler(t, counts, true)
	installBaijiayunxiaoMockTransport(t, handler)

	jar := yunduanTestJar(t)
	setYunduanCookie(t, jar, "https://demo.at.baijiayun.com/", "ORGSUPERSESSID", "org-session")

	rawURL := "https://demo.at.baijiayun.com/org/class_playback/getLongTermList?room_id=room-long"
	info, err := (&Baijiayunxiao{}).Extract(rawURL, &extractor.ExtractOpts{Cookies: jar})
	if err != nil {
		t.Fatalf("extract yunduan class playback: %v", err)
	}
	if info.Title != "长期班课" {
		t.Fatalf("title = %q", info.Title)
	}
	if len(info.Entries) != 2 {
		t.Fatalf("entries = %d, want 2: %#v", len(info.Entries), info.Entries)
	}
	assertYunduanEntryURL(t, info.Entries, "https://cdn.example.com/room-long-1.m3u8")
	assertYunduanEntryURL(t, info.Entries, "https://cdn.example.com/room-long-2.m3u8")
	for _, key := range []string{
		"demo.at.baijiayun.com/org/class_playback/getLongTermRoomList",
		"demo.at.baijiayun.com/org/class_playback/getLongTermList",
		"demo.at.baijiayun.com/org/class_playback/getShortTermList",
		"api.baijiayun.com/web/playback/getPlayInfo",
	} {
		if counts[key] == 0 {
			t.Fatalf("route %s was not called; counts=%#v", key, counts)
		}
	}
}

func readGoldenFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/sample.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if !json.Valid(b) {
		t.Fatalf("fixture is not valid JSON: %s", b)
	}
	return b
}

func assertFixtureServed(t *testing.T, baseURL string, want []byte) {
	t.Helper()
	resp, err := http.Get(baseURL + "/fixture")
	if err != nil {
		t.Fatalf("fetch fixture from mock server: %v", err)
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read fixture response: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("mock fixture mismatch: got %s want %s", got, want)
	}
}

func installBaijiayunxiaoMockTransport(t *testing.T, handler http.Handler) {
	t.Helper()
	plain := httptest.NewServer(handler)
	tlsSrv := httptest.NewTLSServer(handler)

	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatal("unexpected default transport type")
	}
	oldTransport := baseTransport.Clone()
	oldProxy := util.DefaultProxy()
	if err := util.SetDefaultProxy(""); err != nil {
		t.Fatal(err)
	}

	plainURL, err := url.Parse(plain.URL)
	if err != nil {
		t.Fatal(err)
	}
	tlsURL, err := url.Parse(tlsSrv.URL)
	if err != nil {
		t.Fatal(err)
	}

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
			_ = raw.Close()
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

func yunduanMockHandler(t *testing.T, counts map[string]int, includeClass bool) http.Handler {
	t.Helper()
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Host + r.URL.Path
		mu.Lock()
		counts[key]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Host == "www.baijiayun.com" && r.URL.Path == "/entry":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, `<html>demo.at.baijiayun.com</html>`)
		case r.Host == "demo.at.baijiayun.com" && r.URL.Path == "/org/account/getUserInfo":
			_, _ = io.WriteString(w, `{"code":0,"data":{"company":"Demo"}}`)
		case r.Host == "demo.at.baijiayun.com" && r.URL.Path == "/org/course_playback/getCourseList":
			_, _ = io.WriteString(w, `{"code":0,"data":{"list":[{"course_id":"course-1","title":"云端入口课程","lesson_count":2}],"total":1}}`)
		case r.Host == "demo.at.baijiayun.com" && r.URL.Path == "/org/course_playback/getLessonList":
			if r.URL.Query().Get("course_id") != "course-1" {
				t.Fatalf("course_id = %q", r.URL.Query().Get("course_id"))
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"list":[{"lesson_id":"lesson-1","course_id":"course-1","title":"课程回放一","room_id":"room-course-1","token":"token-course-1","duration":65}],"total":1}}`)
		case r.Host == "demo.at.baijiayun.com" && r.URL.Path == "/org/course_playback/getApiLessonList":
			_, _ = io.WriteString(w, `{"code":0,"data":{"list":[],"total":0}}`)
		case r.Host == "demo.at.baijiayun.com" && r.URL.Path == "/org/course_playback/getRecentList":
			_, _ = io.WriteString(w, `{"code":0,"data":{"list":[{"lesson_id":"lesson-2","course_id":"course-1","title":"课程回放二","play_url":"https://www.baijiayun.com/web/playback/index?room_id=room-course-2&token=token-course-2","duration":125}],"total":1}}`)
		case r.Host == "demo.at.baijiayun.com" && r.URL.Path == "/org/class_playback/getLongTermRoomList":
			if includeClass {
				_, _ = io.WriteString(w, `{"code":0,"data":{"list":[{"room_id":"room-long","title":"长期班课","playback_count":2}],"total":1}}`)
			} else {
				_, _ = io.WriteString(w, `{"code":0,"data":{"list":[],"total":0}}`)
			}
		case r.Host == "demo.at.baijiayun.com" && r.URL.Path == "/org/class_playback/getLongTermList":
			if r.URL.Query().Get("room_id") != "room-long" {
				t.Fatalf("room_id = %q", r.URL.Query().Get("room_id"))
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"list":[{"record_id":"record-long-1","title":"长期回放一","room_id":"room-long-1","player_token":"token-long-1"}],"total":1}}`)
		case r.Host == "demo.at.baijiayun.com" && r.URL.Path == "/org/class_playback/getShortTermList":
			_, _ = io.WriteString(w, `{"code":0,"data":{"list":[{"record_id":"record-long-2","title":"短期回放二","play_url":"https://www.baijiayun.com/web/playback/index?classid=room-long-2&token=token-long-2"}],"total":1}}`)
		case r.Host == "demo.at.baijiayun.com" && r.URL.Path == "/org/class_playback/getRecentList":
			_, _ = io.WriteString(w, `{"code":0,"data":{"list":[],"total":0}}`)
		case r.Host == "api.baijiayun.com" && r.URL.Path == "/web/playback/getPlayInfo":
			roomID := r.URL.Query().Get("room_id")
			_, _ = io.WriteString(w, `{"code":0,"data":{"playback_url":"https://cdn.example.com/`+roomID+`.m3u8","playback_title":"`+roomID+`"}}`)
		default:
			t.Fatalf("unexpected request: host=%s path=%s raw=%s", r.Host, r.URL.Path, r.URL.RawQuery)
		}
	})
}

func yunduanTestJar(t *testing.T) http.CookieJar {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return jar
}

func setYunduanCookie(t *testing.T, jar http.CookieJar, rawURL, name, value string) {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(u, []*http.Cookie{{Name: name, Value: value, Path: "/"}})
}

func assertYunduanEntryURL(t *testing.T, entries []*extractor.MediaInfo, want string) {
	t.Helper()
	for _, entry := range entries {
		for _, stream := range entry.Streams {
			for _, got := range stream.URLs {
				if got == want {
					return
				}
			}
		}
	}
	t.Fatalf("entry URL %s not found in %#v", want, entries)
}
