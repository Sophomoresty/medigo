package yizhiknow

import "testing"

func TestExtractItemsNilReturnsEmpty(t *testing.T) {
	if got := extractItems(nil); len(got) != 0 {
		t.Fatalf("extractItems(nil) len=%d, want 0", len(got))
	}
}
