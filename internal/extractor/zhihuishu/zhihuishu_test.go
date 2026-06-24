package zhihuishu

import "testing"

func TestExtractCourseHomeID(t *testing.T) {
	tests := map[string]string{
		"https://coursehome.zhihuishu.com/courseHome/1000006263#teachTeam": "1000006263",
		"https://study.zhihuishu.com/path?courseId=20001":                  "20001",
		"https://hikeweb.zhihuishu.com/hike-tch/course/x?proCourseId=42":   "42",
	}
	for rawURL, want := range tests {
		if got := extractCourseHomeID(rawURL); got != want {
			t.Fatalf("extractCourseHomeID(%q) = %q, want %q", rawURL, got, want)
		}
	}
}

func TestCourseHomeTitle(t *testing.T) {
	page := `var courseName = "大学物理"; var schoolName = "测试大学"; var termId = 123;`
	if got := courseHomeTitle(page, "fallback"); got != "大学物理_测试大学" {
		t.Fatalf("courseHomeTitle = %q", got)
	}
}

func TestParseCourseHomeVideos(t *testing.T) {
	body := `
<div class="onlines-sections-list-container">
  <div class="online-sections-wrap">
    <div class="online-item"><div class="online-section-title-text-wrap" title="第一章"></div></div>
    <div class="sections-wrap">
      <div class="section-item" videoid="v100">
        <div class="online-section-title-text-wrap" title="第一节"></div>
      </div>
      <div class="section-childnode-item" videoid="v101">
        <div class="online-section-title-text-wrap" title="小节 A"></div>
      </div>
      <div class="section-childnode-item" videoid="v102">
        <div class="online-section-title-text-wrap" title="小节 B"></div>
      </div>
    </div>
  </div>
</div>`
	videos, err := parseCourseHomeVideos(body)
	if err != nil {
		t.Fatalf("parseCourseHomeVideos: %v", err)
	}
	if len(videos) != 3 {
		t.Fatalf("videos len = %d, want 3: %#v", len(videos), videos)
	}
	want := []courseHomeVideo{
		{Title: "[1.1]--第一节", VideoID: "v100"},
		{Title: "[1.1.1]--小节 A", VideoID: "v101"},
		{Title: "[1.1.2]--小节 B", VideoID: "v102"},
	}
	for i := range want {
		if videos[i] != want[i] {
			t.Fatalf("videos[%d] = %#v, want %#v", i, videos[i], want[i])
		}
	}
}
