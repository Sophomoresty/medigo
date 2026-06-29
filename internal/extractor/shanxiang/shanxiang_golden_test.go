package shanxiang

import (
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
	"testing"
	"time"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

type mockTransport struct{}

func installMockTransport(t *testing.T, handler http.Handler) {
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

func loadFixtures(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("testdata/sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures map[string]json.RawMessage
	if err := json.Unmarshal(b, &fixtures); err != nil {
		t.Fatal(err)
	}
	return fixtures
}

func writeFixture(t *testing.T, w http.ResponseWriter, fixtures map[string]json.RawMessage, key string) {
	t.Helper()
	raw, ok := fixtures[key]
	if !ok {
		t.Fatalf("missing fixture %s", key)
	}
	if len(raw) > 0 && raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(s))
		return
	}
	_, _ = w.Write(raw)
}

func readJSONBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Body.Close()
	if strings.TrimSpace(string(b)) == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func pageNo(t *testing.T, r *http.Request) int {
	t.Helper()
	body := readJSONBody(t, r)
	if body == nil {
		return 1
	}
	if page, ok := body["page"].(map[string]any); ok {
		if v, ok := page["page"].(float64); ok && v > 0 {
			return int(v)
		}
	}
	return 1
}

func newJar() http.CookieJar {
	jar, _ := cookiejar.New(nil)
	return jar
}

func setCookies(t *testing.T, jar http.CookieJar, raw string, cookies ...*http.Cookie) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(u, cookies)
}

func firstPlayableURL(mi *extractor.MediaInfo) string {
	if mi == nil {
		return ""
	}
	for _, stream := range mi.Streams {
		if len(stream.URLs) > 0 && strings.TrimSpace(stream.URLs[0]) != "" {
			return stream.URLs[0]
		}
	}
	for _, entry := range mi.Entries {
		if u := firstPlayableURL(entry); u != "" {
			return u
		}
	}
	return ""
}

func TestExtractMock(t *testing.T) {
	fixtures := loadFixtures(t)
	installMockTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Host == "www.sx1211.com" && r.Method == http.MethodGet && r.URL.Path == "/user/course.html":
			_, _ = w.Write([]byte(`<html><body><span class="js-user-name">mock-user</span><a href="/user/course.html">我的课程</a></body></html>`))
		case r.Host == "www.sx1211.com" && r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/course/study.html"):
			writeFixture(t, w, fixtures, "study_page")
		case r.Host == "www.sx1211.com" && r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/course/playbackView"):
			writeFixture(t, w, fixtures, "playback_page")
		case r.Host == "view.csslcloud.net" && r.Method == http.MethodPost && r.URL.Path == "/replay/user/login":
			writeFixture(t, w, fixtures, "shanxiang_replay_login")
		case r.Host == "view.csslcloud.net" && r.Method == http.MethodGet && r.URL.Path == "/replay/video/play":
			if r.Header.Get("X-HD-Token") == "" {
				t.Errorf("missing X-HD-Token on replay/video/play")
			}
			writeFixture(t, w, fixtures, "shanxiang_replay_play")
		case r.Host == "view.csslcloud.net" && r.Method == http.MethodPost && r.URL.Path == "/api/room/replay/login":
			writeFixture(t, w, fixtures, "cssl_login")
		case r.Host == "view.csslcloud.net" && r.Method == http.MethodGet && r.URL.Path == "/api/record/vod":
			writeFixture(t, w, fixtures, "cssl_vod")
		case r.Host == "cdn.example.com" && r.Method == http.MethodGet && r.URL.Path == "/shanxiang.m3u8":
			writeFixture(t, w, fixtures, "m3u8")
		default:
			t.Errorf("unexpected request: %s %s%s", r.Method, r.Host, r.URL.String())
			http.NotFound(w, r)
		}
	}))

	jar := newJar()

	ext, err := extractor.Match("https://www.sx1211.com/course/study.html?id=1001&skuId=2001")
	if err != nil {
		t.Fatal(err)
	}
	info, err := ext.Extract("https://www.sx1211.com/course/study.html?id=1001&skuId=2001", &extractor.ExtractOpts{Cookies: jar})
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("nil MediaInfo")
	}
	if info.Site != "shanxiang" {
		t.Fatalf("site=%q", info.Site)
	}
	got := firstPlayableURL(info)
	if !strings.Contains(got, "cdn.example.com/shanxiang.m3u8") && !strings.HasPrefix(got, "data:application/vnd.apple.mpegurl") {
		t.Fatalf("playable URL %q does not contain expected fixture URL", got)
	}
}

func TestCourseListPaginationAndPrice(t *testing.T) {
	installMockTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "www.sx1211.com" || r.URL.Path != "/User/getAjaxCourseList" {
			t.Errorf("unexpected request: %s %s%s", r.Method, r.Host, r.URL.String())
			http.NotFound(w, r)
			return
		}
		page := r.URL.Query().Get("p")
		switch page {
		case "1":
			_, _ = w.Write([]byte(`{"success":"1","data":{"rows":[{"productid":1001,"skuid":2001,"productname":"Course 1","price":"199"}],"totalPages":2,"nextPageIndex":2}}`))
		case "2":
			_, _ = w.Write([]byte(`{"success":"1","data":{"rows":[{"productid":1002,"skuId":2002,"name":"Course 2","price":"0"}],"totalPages":2,"nextPageIndex":0}}`))
		default:
			t.Fatalf("unexpected page %q", page)
		}
	}))
	c := util.NewClient()
	courses, err := fetchCourseList(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(courses) != 2 {
		t.Fatalf("course count=%d, want 2", len(courses))
	}
	if courses[0].CourseID != "1001" || courses[0].SKUId != "2001" || courses[0].Price != "199" || courses[0].Purchased {
		t.Fatalf("course[0]=%#v", courses[0])
	}
	if courses[1].CourseID != "1002" || !courses[1].Purchased {
		t.Fatalf("course[1]=%#v", courses[1])
	}
}
