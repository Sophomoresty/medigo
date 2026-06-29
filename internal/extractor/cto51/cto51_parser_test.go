package cto51

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
)

func TestCourseRefsFromPayloadsAndHTML(t *testing.T) {
	var payload any
	if err := decodeJSON(`{
		"data": {
			"list": [
				{"course_id": 1001, "course_name": "Go 基础", "course_url": "/course/1001.html", "price": "199"},
				{"train_id": 3001, "train_name": "微职位", "url": "/center/wejob/user/course?train_id=3001"},
				{"lesson_id": 2002, "course_id": 1001, "title": "课时不应成为课程"}
			]
		}
	}`, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	refs := courseRefsFromPayloads([]any{payload})
	if len(refs) != 2 {
		t.Fatalf("courseRefsFromPayloads len = %d, want 2: %#v", len(refs), refs)
	}
	if refs[0].ID != "1001" || refs[0].Title != "Go 基础" || refs[0].IsTraining {
		t.Fatalf("first course ref mismatch: %#v", refs[0])
	}
	if refs[1].TrainID != "3001" || !refs[1].IsTraining {
		t.Fatalf("training ref mismatch: %#v", refs[1])
	}

	htmlRefs := courseRefsFromHTML(`<a href="/course/1001.html">Go 基础</a><a href="/training_3001.html">微职位</a><a href="/course/1001.html">重复</a>`)
	if len(htmlRefs) != 2 {
		t.Fatalf("courseRefsFromHTML len = %d, want 2: %#v", len(htmlRefs), htmlRefs)
	}
	if htmlRefs[0].ID != "1001" || htmlRefs[1].TrainID != "3001" {
		t.Fatalf("HTML refs mismatch: %#v", htmlRefs)
	}
}

func TestLessonsAndFilesFromPayloads(t *testing.T) {
	var lessonPayload any
	if err := decodeJSON(`{
		"data": {
			"lessonList": [
				{"type": "chapter", "title": "第一章", "children": [
					{"lesson_id": 2002, "lesson_name": "开篇", "is_preview": 1}
				]},
				{"lessonId": 2003, "lessonName": "进阶", "chapter_name": "第二章"},
				{"id": "900_2004", "title": "训练课", "url": "/center/course/lesson/index?id=900_2004"},
				{"live_id": 777, "live_name": "直播回放", "type": "live"}
			]
		}
	}`, &lessonPayload); err != nil {
		t.Fatalf("decode lesson payload: %v", err)
	}
	var filePayload any
	if err := decodeJSON(`{
		"data": {
			"fileList": [
				{"fileUrl": "/download/handout.pdf", "fileName": "讲义", "lesson_id": 2002, "fileSize": 123},
				{"packFileUrl": "//cdn.example.com/course.zip", "packFileName": "整课资料"},
				{"url": "https://cdn.example.com/video.mp4", "title": "视频不应成为文件"}
			]
		}
	}`, &filePayload); err != nil {
		t.Fatalf("decode file payload: %v", err)
	}

	lessons := dedupeLessons(lessonsFromPayloads([]any{lessonPayload}, lessonContext{CourseID: "1001"}))
	if len(lessons) != 4 {
		t.Fatalf("lessons len = %d, want 4: %#v", len(lessons), lessons)
	}
	if lessons[0].ID != "2002" || lessons[0].ChapterTitle != "第一章" || !lessons[0].Preview {
		t.Fatalf("chapter lesson mismatch: %#v", lessons[0])
	}
	if lessons[2].ID != "2004" || lessons[2].TrainCourseID != "900" || lessons[2].SourceKind != "training" {
		t.Fatalf("training lesson mismatch: %#v", lessons[2])
	}
	if lessons[3].LiveID != "777" || lessons[3].SourceKind != "live" {
		t.Fatalf("live lesson mismatch: %#v", lessons[3])
	}

	files := dedupeFiles(filesFromPayloads([]any{filePayload}, "material"))
	if len(files) != 2 {
		t.Fatalf("files len = %d, want 2: %#v", len(files), files)
	}
	if files[0].Format != "pdf" || files[0].LessonID != "2002" || files[0].Size != 123 {
		t.Fatalf("lesson file mismatch: %#v", files[0])
	}
	if files[1].Format != "zip" || files[1].Title != "整课资料" {
		t.Fatalf("pack file mismatch: %#v", files[1])
	}
}

func TestHTMLFilesAndPlayAuthParsing(t *testing.T) {
	files := filesFromHTML(`<a href="/docs/a.pdf">PDF 讲义</a> "https:\/\/cdn.example.com\/b.zip?x=1"`, "章节", "2002", "courseware")
	if len(files) != 2 {
		t.Fatalf("filesFromHTML len = %d, want 2: %#v", len(files), files)
	}
	if files[0].URL != "https://edu.51cto.com/docs/a.pdf" || files[0].Title != "PDF 讲义" || files[0].Scope != "courseware" {
		t.Fatalf("HTML anchor file mismatch: %#v", files[0])
	}
	if files[1].Format != "zip" || files[1].LessonID != "2002" {
		t.Fatalf("HTML raw URL file mismatch: %#v", files[1])
	}

	playAuthJSON, _ := json.Marshal(map[string]string{
		"AccessKeyId":     "ak",
		"AccessKeySecret": "sk",
		"SecurityToken":   "tk",
		"Region":          "cn-shanghai",
		"AuthInfo":        "auth-info",
		"AuthTimeout":     "7200",
	})
	playAuth := base64.StdEncoding.EncodeToString(playAuthJSON)
	auth := parseAliPlayParam(`var aliplayparam = {type: "course", lesson_id: "2002", vod_video_id: "abcdef0123456789abcdef0123456789", sign: "` + playAuth + `", Rand: "rand-value"};`)
	if auth["lesson_id"] != "2002" || auth["vod_video_id"] != "abcdef0123456789abcdef0123456789" {
		t.Fatalf("aliplayparam ids mismatch: %#v", auth)
	}
	if auth["access_key_id"] != "ak" || auth["access_key_secret"] != "sk" || auth["sts_token"] != "tk" || auth["rand"] != "rand-value" {
		t.Fatalf("decoded playAuth mismatch: %#v", auth)
	}
	auth2 := parseAliPlayParam(`var aliplayparam = {playid: "course", vod_video_id: "vid-1", vod_video_id_auth: "request-auth"};`)
	if auth2["type"] != "course" || auth2["vod_video_id"] != "vid-1" || auth2["psign"] != "request-auth" {
		t.Fatalf("vod_video_id_auth parsing mismatch: %#v", auth2)
	}

	vodAuth := authFromVodPayload(map[string]any{"data": map[string]any{"info": map[string]any{
		"playAuth": playAuth,
		"videoId":  "aliyun-video",
	}}}, map[string]string{"psign": "request-sign", "vod_video_id": "request-video"})
	if vodAuth["psign"] != playAuth || vodAuth["vod_video_id"] != "aliyun-video" {
		t.Fatalf("vod-play-auth response should override request sign/id: %#v", vodAuth)
	}
	if vodAuth["access_key_id"] != "ak" || vodAuth["access_key_secret"] != "sk" {
		t.Fatalf("vod-play-auth playAuth decode mismatch: %#v", vodAuth)
	}
}

func TestQCloudPlayParamsEncryptsOverlay(t *testing.T) {
	params := qcloudPlayParams("psign-value")
	if params["keyId"] != "1" || params["psign"] != "psign-value" {
		t.Fatalf("base qcloud params mismatch: %#v", params)
	}
	for _, key := range []string{"cipheredOverlayKey", "cipheredOverlayIv"} {
		value := params[key]
		if value == "" {
			t.Fatalf("%s missing in qcloud params: %#v", key, params)
		}
		if _, err := hex.DecodeString(value); err != nil {
			t.Fatalf("%s is not hex: %v", key, err)
		}
		if len(value) != 256 {
			t.Fatalf("%s hex len = %d, want 256", key, len(value))
		}
	}
	if params["overlayKey"] != "" || params["overlayIv"] != "" {
		t.Fatalf("clear overlay fallback should not be used with bundled RSA key: %#v", params)
	}
}

func TestPriceAnd51ctoSecDataHelpers(t *testing.T) {
	if got := normalizePriceValue("19900"); got != "199" {
		t.Fatalf("normalizePriceValue cents = %q, want 199", got)
	}
	if got := extractPriceFromHTML(`<script>var course_price = "29900";</script>`); got != "299" {
		t.Fatalf("extractPriceFromHTML var = %q, want 299", got)
	}
	if got := extractPriceFromHTML(`<span>免费</span>`); got != "0" {
		t.Fatalf("extractPriceFromHTML free = %q, want 0", got)
	}

	seed := "seed-value"
	randPlain := "rand-plain"
	secret := []byte("0123456789abcdef")
	key1 := []byte(testMD5Middle16(seed))
	randCipher := testCto51CBCEncrypt([]byte(randPlain), key1, key1)
	key2 := []byte(testMD5Middle16(seed + randPlain))
	plainCipher := testCto51CBCEncrypt([]byte(base64.StdEncoding.EncodeToString(secret)), key2, key1)

	got := derive51ctoSecData(seed, randCipher, plainCipher)
	if !bytes.Equal(got, secret) {
		t.Fatalf("derive51ctoSecData = %x, want %x", got, secret)
	}
}

func testCto51CBCEncrypt(plain, key, iv []byte) string {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	buf := append(append([]byte{}, plain...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	dst := make([]byte, len(buf))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(dst, buf)
	return base64.StdEncoding.EncodeToString(dst)
}

func testMD5Middle16(text string) string {
	sum := md5.Sum([]byte(text))
	return fmt.Sprintf("%x", sum)[8:24]
}
