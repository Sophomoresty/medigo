package eoffcn

import "testing"

func TestAESDecryptWithStaticRejectsInvalidIVLength(t *testing.T) {
	got := aesDecryptWithStatic("AAAAAAAAAAAAAAAAAAAAAA==", "wwwoffcncloudcom", "short")
	if got != "" {
		t.Fatalf("aesDecryptWithStatic = %q, want empty result for invalid IV", got)
	}
}
