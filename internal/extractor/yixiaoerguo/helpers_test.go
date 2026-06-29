package yixiaoerguo

import "testing"

func TestQXJunkRoundTrip(t *testing.T) {
	plain := "eyJhZGRyZXNzIjoiaHR0cHM6Ly9leGFtcGxlLmNvbS9hLm0zdTgifQ=="
	encoded := qxJunkEncode(plain, 3, 1)
	if encoded == plain {
		t.Fatal("encoded text was not changed")
	}
	if got := qxJunkDecode(encoded, 3, 1); got != plain {
		t.Fatalf("decode mismatch: got %q want %q", got, plain)
	}
}

func TestBuildQXMediaInfoSegments(t *testing.T) {
	items := []map[string]any{
		{"cdn_url": "https://cdn.example.com/part2.mp4", "duration": 10, "size": 2000, "startTime": 10},
		{"cdn_url": "https://cdn.example.com/part1.mp4", "duration": 10, "size": 1000, "startTime": 0},
	}
	info := buildQXMediaInfo(items, "20")
	if len(info.URLs) != 2 || info.URLs[0] != "https://cdn.example.com/part1.mp4" || info.URLs[1] != "https://cdn.example.com/part2.mp4" {
		t.Fatalf("unexpected ordered URLs: %#v", info.URLs)
	}
	if len(info.Segments) != 2 {
		t.Fatalf("segments=%d, want 2", len(info.Segments))
	}
}

func TestExtractItemsNilReturnsEmpty(t *testing.T) {
	if got := extractItems(nil); len(got) != 0 {
		t.Fatalf("extractItems(nil) len=%d, want 0", len(got))
	}
}
