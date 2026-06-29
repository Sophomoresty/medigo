package gaotu

import (
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
)

func TestPatternsMatchGaotuAPIDomains(t *testing.T) {
	compiled := make([]*regexp.Regexp, 0, len((&Gaotu{}).Patterns()))
	for _, pattern := range (&Gaotu{}).Patterns() {
		compiled = append(compiled, regexp.MustCompile(pattern))
	}
	for _, rawURL := range []string{
		"https://api.gaotu.cn/studyPlatform/v1/unit/clazz/list?clazzNumber=G001",
		"https://interactive.gaotu.cn/live/api/studyCenter/v1/user/pc/clazz/detail?clazzNumber=G001",
		"https://api.gaotu100.com/live/zplan/login/videoLive?clazzLessonNumber=T001",
		"https://interactive.gaotu100.com/live/api/live/zplan/playbackWeb?clazzNumber=T001",
		"https://api.gtgz.cn/web/order/pay/shape/list",
		"https://interactive.gtgz.cn/live/api/pan/listDir",
		"https://api.naiyouxuexi.com/studyPlatform/v1/unit/clazz/list?clazzNumber=S001",
		"https://interactive.naiyouxuexi.com/live/api/pan/file",
	} {
		t.Run(rawURL, func(t *testing.T) {
			for _, re := range compiled {
				if re.MatchString(rawURL) {
					return
				}
			}
			t.Fatalf("gaotu pattern did not match %q", rawURL)
		})
	}
}

func TestEndpointsForBrandDomains(t *testing.T) {
	tests := []struct {
		name      string
		rawURL    string
		courseURL string
		infoURL   string
		videoURL  string
		liveURL   string
		sourceURL string
		fileURL   string
		priceURL  string
		orderURL  string
		referer   string
		pClient   string
		userAgent string
	}{
		{
			name:      "gaotu",
			rawURL:    "https://www.gaotu.cn/course?clazzNumber=G001",
			courseURL: "https://api.gaotu.cn/studyPlatform/v1/unit/clazz/list?isDebounce=true&os=h5-pc&p_client=1",
			infoURL:   "https://interactive.gaotu.cn/live/api/studyCenter/v1/user/pc/clazz/detail",
			videoURL:  "https://api.gaotu.cn/live/zplan/login/videoLive",
			liveURL:   "https://interactive.gaotu.cn/live/api/live/zplan/playbackWeb",
			sourceURL: "https://interactive.gaotu.cn/live/api/pan/listDir",
			fileURL:   "https://interactive.gaotu.cn/live/api/pan/file",
			priceURL:  "https://api.gaotu.cn/cs/api/product/course/detailButton?productSpuNumber=%s",
			orderURL:  "https://api.gaotu.cn/web/order/pay/shape/list",
			referer:   "https://www.gaotu.cn",
			pClient:   "1",
			userAgent: "WenZaiZhiBoClient-Windows7-gaotu-9.0.5.49",
		},
		{
			name:      "tutu",
			rawURL:    "https://gaotu100.com/course?clazzNumber=T001",
			courseURL: "https://api.gaotu100.com/studyPlatform/v1/unit/clazz/list?isDebounce=true&os=h5-pc&p_client=2",
			infoURL:   "https://interactive.gaotu100.com/live/api/studyCenter/v1/user/pc/clazz/detail",
			videoURL:  "https://api.gaotu100.com/live/zplan/login/videoLive",
			liveURL:   "https://interactive.gaotu100.com/live/api/live/zplan/playbackWeb",
			sourceURL: "https://interactive.gaotu100.com/live/api/pan/listDir",
			fileURL:   "https://interactive.gaotu100.com/live/api/pan/file",
			priceURL:  "https://api.gaotu100.com/cs/api/product/course/detailButton?productSpuNumber=%s",
			orderURL:  "https://api.gaotu100.com/web/order/pay/shape/list",
			referer:   "https://gaotu100.com",
			pClient:   "2",
			userAgent: "WenZaiZhiBoClient-Windows7-tutuketang-10.0.0.89",
		},
		{
			name:      "gaozhong",
			rawURL:    "https://www.gtgz.cn/course?clazzNumber=H001",
			courseURL: "https://api.gtgz.cn/studyPlatform/v1/unit/clazz/list?isDebounce=true&os=h5-pc&p_client=8",
			infoURL:   "https://interactive.gtgz.cn/live/api/studyCenter/v1/user/pc/clazz/detail",
			videoURL:  "https://api.gtgz.cn/live/zplan/login/videoLive",
			liveURL:   "https://interactive.gtgz.cn/live/api/live/zplan/playbackWeb",
			sourceURL: "https://interactive.gtgz.cn/live/api/pan/listDir",
			fileURL:   "https://interactive.gtgz.cn/live/api/pan/file",
			priceURL:  "https://api.gtgz.cn/cs/api/product/course/detailButton?productSpuNumber=%s",
			orderURL:  "https://api.gtgz.cn/web/order/pay/shape/list",
			referer:   "https://www.gtgz.cn",
			pClient:   "8",
			userAgent: "WenZaiZhiBoClient-Windows7-gtugzgh-10.0.0.89",
		},
		{
			name:      "suyang",
			rawURL:    "https://www.naiyouxuexi.com/course?clazzNumber=S001",
			courseURL: "https://api.naiyouxuexi.com/studyPlatform/v1/unit/clazz/list?isDebounce=true&os=h5-pc&p_client=18",
			infoURL:   "https://interactive.naiyouxuexi.com/live/api/studyCenter/v1/user/pc/clazz/detail",
			videoURL:  "https://api.naiyouxuexi.com/live/zplan/login/videoLive",
			liveURL:   "https://interactive.naiyouxuexi.com/live/api/live/zplan/playbackWeb",
			sourceURL: "https://interactive.naiyouxuexi.com/live/api/pan/listDir",
			fileURL:   "https://interactive.naiyouxuexi.com/live/api/pan/file",
			priceURL:  "https://api.naiyouxuexi.com/cs/api/product/course/detailButton?productSpuNumber=%s",
			orderURL:  "https://api.naiyouxuexi.com/web/order/pay/shape/list",
			referer:   "https://www.naiyouxuexi.com",
			pClient:   "18",
			userAgent: "WenZaiZhiBoClient-Windows7-gaotusuyang-10.0.20.2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := endpointsFor(tt.rawURL)
			if got.referer != tt.referer {
				t.Fatalf("referer = %q, want %q", got.referer, tt.referer)
			}
			if got.pClient != tt.pClient {
				t.Fatalf("pClient = %q, want %q", got.pClient, tt.pClient)
			}
			if got.courseURL() != tt.courseURL {
				t.Fatalf("courseURL = %q, want %q", got.courseURL(), tt.courseURL)
			}
			if got.infoURL() != tt.infoURL {
				t.Fatalf("infoURL = %q, want %q", got.infoURL(), tt.infoURL)
			}
			if got.videoURL() != tt.videoURL {
				t.Fatalf("videoURL = %q, want %q", got.videoURL(), tt.videoURL)
			}
			if got.liveURL() != tt.liveURL {
				t.Fatalf("liveURL = %q, want %q", got.liveURL(), tt.liveURL)
			}
			if got.sourceURL() != tt.sourceURL {
				t.Fatalf("sourceURL = %q, want %q", got.sourceURL(), tt.sourceURL)
			}
			if got.fileURL() != tt.fileURL {
				t.Fatalf("fileURL = %q, want %q", got.fileURL(), tt.fileURL)
			}
			if got.priceURL() != tt.priceURL {
				t.Fatalf("priceURL = %q, want %q", got.priceURL(), tt.priceURL)
			}
			if got.orderURL() != tt.orderURL {
				t.Fatalf("orderURL = %q, want %q", got.orderURL(), tt.orderURL)
			}
			if !strings.Contains(got.userAgent, tt.userAgent) {
				t.Fatalf("userAgent = %q, want to contain %q", got.userAgent, tt.userAgent)
			}
		})
	}
}

func TestEndpointsForAPIDomains(t *testing.T) {
	tests := []struct {
		name            string
		rawURL          string
		apiHost         string
		interactiveHost string
		pClient         string
	}{
		{
			name:            "api_gaotu",
			rawURL:          "https://api.gaotu.cn/studyPlatform/v1/unit/clazz/list?clazzNumber=G001",
			apiHost:         "api.gaotu.cn",
			interactiveHost: "interactive.gaotu.cn",
			pClient:         "1",
		},
		{
			name:            "interactive_gaotu",
			rawURL:          "https://interactive.gaotu.cn/live/api/studyCenter/v1/user/pc/clazz/detail?clazzNumber=G001",
			apiHost:         "api.gaotu.cn",
			interactiveHost: "interactive.gaotu.cn",
			pClient:         "1",
		},
		{
			name:            "api_gaotu100",
			rawURL:          "https://api.gaotu100.com/live/zplan/login/videoLive?clazzLessonNumber=T001",
			apiHost:         "api.gaotu100.com",
			interactiveHost: "interactive.gaotu100.com",
			pClient:         "2",
		},
		{
			name:            "interactive_gaotu100",
			rawURL:          "https://interactive.gaotu100.com/live/api/live/zplan/playbackWeb?clazzNumber=T001",
			apiHost:         "api.gaotu100.com",
			interactiveHost: "interactive.gaotu100.com",
			pClient:         "2",
		},
		{
			name:            "api_gtgz",
			rawURL:          "https://api.gtgz.cn/web/order/pay/shape/list",
			apiHost:         "api.gtgz.cn",
			interactiveHost: "interactive.gtgz.cn",
			pClient:         "8",
		},
		{
			name:            "interactive_gtgz",
			rawURL:          "https://interactive.gtgz.cn/live/api/pan/listDir",
			apiHost:         "api.gtgz.cn",
			interactiveHost: "interactive.gtgz.cn",
			pClient:         "8",
		},
		{
			name:            "api_naiyouxuexi",
			rawURL:          "https://api.naiyouxuexi.com/studyPlatform/v1/unit/clazz/list?clazzNumber=S001",
			apiHost:         "api.naiyouxuexi.com",
			interactiveHost: "interactive.naiyouxuexi.com",
			pClient:         "18",
		},
		{
			name:            "interactive_naiyouxuexi",
			rawURL:          "https://interactive.naiyouxuexi.com/live/api/pan/file",
			apiHost:         "api.naiyouxuexi.com",
			interactiveHost: "interactive.naiyouxuexi.com",
			pClient:         "18",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := endpointsFor(tt.rawURL)
			if got.apiHost != tt.apiHost {
				t.Fatalf("apiHost = %q, want %q", got.apiHost, tt.apiHost)
			}
			if got.interactiveHost != tt.interactiveHost {
				t.Fatalf("interactiveHost = %q, want %q", got.interactiveHost, tt.interactiveHost)
			}
			if got.pClient != tt.pClient {
				t.Fatalf("pClient = %q, want %q", got.pClient, tt.pClient)
			}
		})
	}
}

func TestGaotuPriceFromPayload(t *testing.T) {
	price, ok := gaotuPriceFromPayload(map[string]any{
		"data": map[string]any{
			"coreButton": map[string]any{
				"price": "12345",
			},
		},
	})
	if !ok {
		t.Fatal("price not found")
	}
	if price != 123.45 {
		t.Fatalf("price = %v, want 123.45", price)
	}
}

func TestGaotuOrderPriceFromPayload(t *testing.T) {
	price, ok := gaotuOrderPriceFromPayload(map[string]any{
		"data": map[string]any{
			"payOrderList": []any{
				map[string]any{
					"orderBaseVO": map[string]any{
						"course": map[string]any{"courseId": "C001"},
					},
					"paymentInfo": map[string]any{"originalPrice": "45600"},
				},
			},
		},
	}, "C001")
	if !ok {
		t.Fatal("order price not found")
	}
	if price != 456 {
		t.Fatalf("price = %v, want 456", price)
	}
}

func TestGaotuLessonRequestPayloads(t *testing.T) {
	id := ids{Live: "L001", SID: "session-token"}
	video := gaotuVideoRequestPayload(id)
	if video["liveId"] != "L001" || video["sid"] != "session-token" || video["roleType"] != 0 {
		t.Fatalf("video payload mismatch: %#v", video)
	}
	live := gaotuLiveRequestPayload(id)
	if live["liveId"] != "L001" || live["sessionId"] != "session-token" || live["roleType"] != 0 {
		t.Fatalf("live payload mismatch: %#v", live)
	}
}

func TestGaotuMediaURLFromPayloadPrefersLargestCDNAndDecodesEncURL(t *testing.T) {
	encoded := encodeTestBjcloudvod("https://cdn.example.com/decoded.mp4")
	got := gaotuMediaURLFromPayload(map[string]any{
		"data": map[string]any{
			"play_info": map[string]any{
				"low": map[string]any{
					"size": float64(10),
					"cdn_list": []any{
						map[string]any{"url": "https://cdn.example.com/low.mp4"},
					},
				},
				"high": map[string]any{
					"size": float64(20),
					"cdn_list": []any{
						map[string]any{"enc_url": encoded},
					},
				},
			},
		},
	})
	if got != "https://cdn.example.com/decoded.mp4" {
		t.Fatalf("media url = %q, want decoded high url", got)
	}
}

func TestGaotuMediaURLFromNestedStringPayload(t *testing.T) {
	got := gaotuMediaURLFromPayload(map[string]any{
		"data": map[string]any{
			"signinLivePlayback": `{"play_info":{"source":{"size":1,"cdn_list":[{"url":"https://cdn.example.com/live.m3u8"}]}}}`,
		},
	})
	if got != "https://cdn.example.com/live.m3u8" {
		t.Fatalf("media url = %q, want nested m3u8", got)
	}
}

func encodeTestBjcloudvod(raw string) string {
	shift := byte(3)
	encoded := make([]byte, len(raw)+1)
	encoded[0] = shift
	for i, b := range []byte(raw) {
		encoded[i+1] = b ^ byte((int(shift)+i)%8)
	}
	return "bjcloudvod://" + strings.NewReplacer("+", "-", "/", "_", "=", "").Replace(base64.StdEncoding.EncodeToString(encoded))
}

func TestCollectGaotuPanNodes(t *testing.T) {
	nodes := collectGaotuPanNodes(map[string]any{
		"data": map[string]any{
			"dirList": []any{
				map[string]any{
					"entityType":   float64(1),
					"entityNumber": "DIR1",
					"name":         "资料目录",
					"rootNumber":   "ROOT1",
				},
				map[string]any{
					"entityType":   float64(2),
					"entityNumber": "DOC1",
					"url":          "https://cdn.example.com/handout.pdf?token=x",
					"name":         "讲义.pdf",
					"rootNumber":   "ROOT1",
				},
				map[string]any{
					"entityType":   float64(100),
					"entityNumber": "VID1",
					"url":          "https://interactive.gaotu.cn/play?vid=abc",
					"name":         "课堂回放",
					"rootNumber":   "ROOT1",
				},
			},
		},
	})
	if len(nodes) != 3 {
		t.Fatalf("len(nodes) = %d, want 3: %#v", len(nodes), nodes)
	}
	if !isGaotuDir(nodes[0]) {
		t.Fatalf("first node should be directory: %#v", nodes[0])
	}
	if nodes[1].ID != "DOC1" || nodes[1].Format != "pdf" || nodes[1].Root != "ROOT1" {
		t.Fatalf("doc node parsed incorrectly: %#v", nodes[1])
	}
	if nodes[2].Type != "100" || nodes[2].ID != "VID1" {
		t.Fatalf("video node parsed incorrectly: %#v", nodes[2])
	}
}
