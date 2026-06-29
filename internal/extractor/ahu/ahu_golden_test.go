package ahu

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/Sophomoresty/mediago/internal/extractor"
)

func TestExtractMock(t *testing.T) {
	fixture := readGoldenFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()
	assertFixtureServed(t, srv.URL, fixture)

	ext, err := extractor.Match("https://www.ahuyikao.com/course/courseinfo.html?courseId=1001")
	if err != nil {
		t.Fatalf("extractor pattern should match fixture URL: %v", err)
	}
	info, err := ext.Extract("https://www.ahuyikao.com/course/courseinfo.html?courseId=1001", nil)
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

func TestExtractCourseBuildsVideoAndFileEntries(t *testing.T) {
	installAhuMockTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Host == "www.ahuyikao.com" && r.URL.Path == "/center/mycourse.html":
			_, _ = w.Write([]byte(`<html><body><a href="/login/loginout.html">退出登录</a><div class="yxg-mc-student"><a href="/course/courseinfo.html?courseId=1001"><p>Fixture Course</p></a></div></body></html>`))
		case r.Host == "www.ahuyikao.com" && r.URL.Path == "/course/courseinfo.html":
			_, _ = w.Write([]byte(`<html><head><title>AHU Fixture Course_阿虎医考</title></head><body>
				<div class="yxg-collapse-head-one"><p>第一章 基础课</p></div>
				<a href="/video/videoplay.html?courseId=1001&lessonId=2002#2002"><span class="yxg-timeline-title-tow"><p>课时 1 Lesson One 12:34</p></span><span class="yxg-item-time">12:34</span></a>
				<script>var handoutsList = [{"title":"Lesson One 讲义","url":"/files/lesson-one.pdf"}];</script>
			</body></html>`))
		case r.Host == "www.ahuyikao.com" && r.URL.Path == "/video/videoplay.html":
			_, _ = w.Write([]byte(`<script>var aliyunVideoId = "aliyun-video-1"; var playAuth = "` + ahuTestPlayAuth(t) + `";</script>`))
		case strings.HasPrefix(r.Host, "vod.") && r.URL.Query().Get("Action") == "GetPlayInfo":
			_ = json.NewEncoder(w).Encode(map[string]any{"PlayInfoList": map[string]any{"PlayInfo": []map[string]any{{"PlayURL": "https://cdn.example.com/ahu.mp4", "Definition": "HD", "Format": "mp4"}}}})
		default:
			t.Fatalf("unexpected request: %s %s%s", r.Method, r.Host, r.URL.String())
		}
	}))
	info, err := (&Ahu{}).Extract("https://www.ahuyikao.com/course/courseinfo.html?courseId=1001", &extractor.ExtractOpts{Cookies: ahuTestJar(t)})
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if info.Title != "AHU Fixture Course" {
		t.Fatalf("title = %q", info.Title)
	}
	if len(info.Entries) != 2 {
		t.Fatalf("entries = %d, want 2: %#v", len(info.Entries), info.Entries)
	}
	var haveVideo, haveFile bool
	for _, e := range info.Entries {
		if e.Extra["type"] == "video" {
			haveVideo = true
			if e.Streams["best"].URLs[0] != "https://cdn.example.com/ahu.mp4" {
				t.Fatalf("video URL = %q", e.Streams["best"].URLs[0])
			}
		}
		if e.Extra["type"] == "file" {
			haveFile = true
			if e.Streams["best"].URLs[0] != "https://www.ahuyikao.com/files/lesson-one.pdf" {
				t.Fatalf("file URL = %q", e.Streams["best"].URLs[0])
			}
		}
	}
	if !haveVideo || !haveFile {
		t.Fatalf("missing video/file entries: %#v", info.Entries)
	}
}

type ahuStaticJar struct{}

func (ahuStaticJar) SetCookies(*url.URL, []*http.Cookie) {}

func (ahuStaticJar) Cookies(*url.URL) []*http.Cookie {
	return []*http.Cookie{{Name: "PHPSESSID", Value: "test-session", Path: "/"}}
}

func ahuTestJar(t *testing.T) http.CookieJar {
	t.Helper()
	return ahuStaticJar{}
}

func ahuTestPlayAuth(t *testing.T) string {
	t.Helper()
	b, err := json.Marshal(map[string]string{"AccessKeyId": "ak-test", "AccessKeySecret": "secret-test", "SecurityToken": "token-test", "Region": "cn-shanghai", "AuthInfo": "auth-test"})
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
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
