package gaodun

import "testing"

func TestCollectGaodunCoursesFromCourseURLPayload(t *testing.T) {
	payload := map[string]any{
		"result": map[string]any{
			"courseList": map[string]any{
				"active": []any{
					map[string]any{
						"id":           "1001",
						"saasCourseId": "C1001",
						"name":         "CPA 基础课",
						"children": []any{
							map[string]any{
								"id":           "1002",
								"saasCourseId": "C1002",
								"name":         "CPA 子课",
							},
						},
					},
				},
			},
		},
	}

	got := collectGaodunCourses(payload)
	if len(got) != 2 {
		t.Fatalf("len(courses) = %d, want 2: %#v", len(got), got)
	}
	if got[0].ID != "C1001" || got[0].Title != "CPA 基础课" {
		t.Fatalf("course[0] = %#v", got[0])
	}
	if got[1].ID != "C1002" || got[1].Title != "CPA 子课" {
		t.Fatalf("course[1] = %#v", got[1])
	}
}

func TestCollectGaodunFilesFromHandoutPayload(t *testing.T) {
	payload := map[string]any{
		"result": []any{
			map[string]any{
				"type": "file",
				"name": "讲义",
				"resource": map[string]any{
					"id":        "R1",
					"path":      "//cdn.gaodun.example/handout.pdf",
					"extension": ".pdf",
				},
			},
			map[string]any{
				"type": "file",
				"name": "课件",
				"resource": map[string]any{
					"id":        "R2",
					"extension": "pptx",
				},
			},
		},
	}

	got := collectGaodunFiles(payload)
	if len(got) != 2 {
		t.Fatalf("len(files) = %d, want 2: %#v", len(got), got)
	}
	if got[0].ID != "R1" || got[0].URL != "https://cdn.gaodun.example/handout.pdf" || got[0].Format != "pdf" {
		t.Fatalf("file[0] = %#v", got[0])
	}
	if got[1].ID != "R2" || got[1].Name != "课件" || got[1].Format != "pptx" {
		t.Fatalf("file[1] = %#v", got[1])
	}
}

func TestFirstTokenText(t *testing.T) {
	tests := []struct {
		name    string
		payload any
		want    string
	}{
		{name: "pc result string", payload: map[string]any{"result": "pc-token", "message": "ok"}, want: "pc-token"},
		{name: "pe result key", payload: map[string]any{"result": map[string]any{"key": "pe-token"}, "message": "ok"}, want: "pe-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstTokenText(tt.payload); got != tt.want {
				t.Fatalf("firstTokenText() = %q, want %q", got, tt.want)
			}
		})
	}
}
