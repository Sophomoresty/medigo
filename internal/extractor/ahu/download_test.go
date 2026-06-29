package ahu

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadTreeFileMatchingAndSplit(t *testing.T) {
	x := &ahuCourse{headers: map[string]string{}}
	chapter := treeNode(formatDirTitle(1, "上消化道疾病"), "上消化道疾病", []int{1})
	section := treeNode(formatDirTitle(1, "胃外科学"), "胃外科学", []int{1, 1})
	chapter.Children = append(chapter.Children, section)
	x.outline = []*ahuTreeNode{chapter}
	var nodes []*ahuTreeNode
	x.appendFileToNodes(&nodes, "胃外科讲义", "https://www.ahuyikao.com/files/stomach.pdf", map[string]bool{})
	info := treeMapToDownloadInfo(nodes)
	_, files := splitDownloadInfo(info)
	if len(files) != 1 {
		t.Fatalf("files = %d, want 1; info=%#v", len(files), info)
	}
	if files[0].FileName != "(1.1.1)--胃外科讲义.pdf" {
		t.Fatalf("file name = %q", files[0].FileName)
	}
	entries := x.entriesFromInfo(info, false, true)
	if len(entries) != 1 || entries[0].Extra["type"] != "file" {
		t.Fatalf("file entries mismatch: %#v", entries)
	}
}

func TestAhuAliyunCompatAPIs(t *testing.T) {
	key := []byte("0123456789abcdef")
	mediaID := "12345678-1234-1234-1234-123456789abc"
	challenge := "challenge-token"
	keyToken := base64.StdEncoding.EncodeToString([]byte(mediaID + challenge))
	installAhuMockTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.Host, "vod.") && r.URL.Query().Get("Action") == "GetPlayInfo":
			_ = json.NewEncoder(w).Encode(map[string]any{"PlayInfoList": map[string]any{"PlayInfo": []map[string]any{{"PlayURL": "https://cdn.example.com/ahu.m3u8", "Definition": "HD", "Format": "m3u8", "Encrypt": "1", "EncryptType": "AliyunVoDEncryption"}}}})
		case r.Host == "cdn.example.com" && r.URL.Path == "/ahu.m3u8":
			_, _ = w.Write([]byte(`#EXTM3U
#EXT-X-KEY:METHOD=AES-128,URI="/key"
#EXTINF:1,
seg.ts
#EXT-X-ENDLIST
`))
		case r.Host == "cdn.example.com" && r.URL.Path == "/key":
			_, _ = w.Write([]byte(keyToken))
		case strings.HasPrefix(r.Host, "mts.") && r.Method == http.MethodPost:
			_ = r.ParseForm()
			if r.PostForm.Get("MediaId") != mediaID || r.PostForm.Get("data") != challenge {
				t.Fatalf("unexpected license form: %v", r.PostForm)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"License": base64.StdEncoding.EncodeToString(key)})
		default:
			t.Fatalf("unexpected request: %s %s%s", r.Method, r.Host, r.URL.String())
		}
	}))
	x, err := newAhuCourse(AhuDownloadOptions{Cookies: ahuTestJar(t)})
	if err != nil {
		t.Fatal(err)
	}
	x.cid = "1001"
	playAuth := ahuTestPlayAuth(t)
	info, err := x._request_aliyun_play_info_legacy(playAuth, "vid1")
	if err != nil {
		t.Fatalf("legacy play info: %v", err)
	}
	if info.M3U8Text == "" || !strings.Contains(info.M3U8Text, "data:application/octet-stream;base64,") {
		t.Fatalf("legacy did not rewrite encrypted m3u8: %#v", info)
	}
	if _, err := x._request_aliyun_play_info_by_rand(playAuth, "vid1", "rand-value"); err != nil {
		t.Fatalf("by-rand play info: %v", err)
	}
	payload := x._decode_aliyun_play_auth(playAuth)
	gotKey, err := x._request_aliyun_license(payload, mediaID, challenge, "AliyunVoDEncryption")
	if err != nil {
		t.Fatalf("license: %v", err)
	}
	if string(gotKey) != string(key) {
		t.Fatalf("license key = %x", gotKey)
	}
	km, kc := x._extract_aliyun_key_material(keyToken)
	if km != mediaID || kc != challenge {
		t.Fatalf("key material = %q %q", km, kc)
	}
}

func TestDownloadAhuWritesVideoAndFiles(t *testing.T) {
	installAhuMockTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Host == "www.ahuyikao.com" && r.URL.Path == "/center/mycourse.html":
			_, _ = w.Write([]byte(`<html><body><a href="/login/loginout.html">退出登录</a><div class="yxg-mc-student"></div></body></html>`))
		case r.Host == "www.ahuyikao.com" && r.URL.Path == "/course/courseinfo.html":
			_, _ = w.Write([]byte(`<html><head><title>AHU Fixture Course_阿虎医考</title></head><body>
				<div class="yxg-collapse-head-one"><p>第一章 基础课</p></div>
				<a href="/video/videoplay.html?courseId=1001&lessonId=2002#2002"><span class="yxg-timeline-title-tow"><p>课时 1 Lesson One 12:34</p></span><span class="yxg-item-time">12:34</span></a>
				<script>var handoutsList = [{"title":"Lesson One 讲义","url":"/files/lesson-one.pdf"}];</script>
			</body></html>`))
		case r.Host == "www.ahuyikao.com" && r.URL.Path == "/video/videoplay.html":
			_, _ = w.Write([]byte(`<script>var videoSrc = "https://cdn.example.com/video.mp4";</script>`))
		case r.Host == "cdn.example.com" && r.URL.Path == "/video.mp4":
			_, _ = w.Write([]byte("mp4-data"))
		case r.Host == "www.ahuyikao.com" && r.URL.Path == "/files/lesson-one.pdf":
			_, _ = w.Write([]byte("%PDF-test"))
		default:
			t.Fatalf("unexpected request: %s %s%s", r.Method, r.Host, r.URL.String())
		}
	}))

	outDir := t.TempDir()
	paths, err := DownloadAhu("https://www.ahuyikao.com/course/courseinfo.html?courseId=1001", AhuDownloadOptions{Cookies: ahuTestJar(t), OutputDir: outDir, NoProgress: true})
	if err != nil {
		t.Fatalf("DownloadAhu returned error: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("downloaded paths = %d, want 2: %#v", len(paths), paths)
	}
	for _, p := range paths {
		if !strings.HasPrefix(p, filepath.Join(outDir, "AHU Fixture Course")) {
			t.Fatalf("path outside course dir: %s", p)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read downloaded file %s: %v", p, err)
		}
		if len(b) == 0 {
			t.Fatalf("downloaded file is empty: %s", p)
		}
	}
}
