package dongao

import (
	"crypto/aes"
	"crypto/cipher"
	"testing"
)

func TestUnpadPKCS7RejectsInvalidPaddingBytes(t *testing.T) {
	input := []byte("hello\x02\x03")
	got, err := unpadPKCS7(append([]byte(nil), input...))
	if err != nil {
		t.Fatalf("unpadPKCS7 returned error: %v", err)
	}
	if string(got) != string(input) {
		t.Fatalf("unpadPKCS7 stripped invalid padding: %q", got)
	}
}

func TestDecryptCBCSegmentRejectsInvalidIVLength(t *testing.T) {
	_, err := decryptCBCSegment(make([]byte, aes.BlockSize), []byte("0123456789abcdef"), []byte("short"))
	if err == nil {
		t.Fatal("decryptCBCSegment should reject invalid IV length")
	}
}

func TestDecryptCBCSegmentKeepsInvalidPaddingPlaintext(t *testing.T) {
	key := []byte("0123456789abcdef")
	iv := []byte("abcdef0123456789")
	plain := []byte("invalid-pad-AB\x02\x03")
	if len(plain) != aes.BlockSize {
		t.Fatalf("test plaintext length = %d, want block size", len(plain))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	ciphertext := append([]byte(nil), plain...)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, ciphertext)
	got, err := decryptCBCSegment(ciphertext, key, iv)
	if err != nil {
		t.Fatalf("decryptCBCSegment: %v", err)
	}
	if string(got) != string(plain) {
		t.Fatalf("plaintext = %q, want %q", got, plain)
	}
}
