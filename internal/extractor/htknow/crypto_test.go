package htknow

import "testing"

func TestPKCS7UnpadRejectsInvalidPaddingBytes(t *testing.T) {
	input := []byte("hello\x02\x03")
	got := pkcs7Unpad(append([]byte(nil), input...))
	if string(got) != string(input) {
		t.Fatalf("pkcs7Unpad stripped invalid padding: %q", got)
	}
}

func TestPKCS7UnpadAcceptsValidPadding(t *testing.T) {
	got := pkcs7Unpad([]byte("hello\x03\x03\x03"))
	if string(got) != "hello" {
		t.Fatalf("pkcs7Unpad = %q, want hello", got)
	}
}
