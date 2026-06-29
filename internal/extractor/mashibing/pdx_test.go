package mashibing

import (
	"crypto/aes"
	"crypto/cipher"
	"strings"
	"testing"
)

func TestPolyvPDXSecretIsValidAES256Key(t *testing.T) {
	if polyvPDXSecretErr != nil {
		t.Fatalf("polyvPDXSecretErr = %v", polyvPDXSecretErr)
	}
	if len(polyvPDXSecret) != 32 {
		t.Fatalf("len(polyvPDXSecret) = %d, want 32", len(polyvPDXSecret))
	}
	if _, err := aes.NewCipher(polyvPDXSecret); err != nil {
		t.Fatalf("polyvPDXSecret should be a valid AES key: %v", err)
	}
}

func TestDecodePolyvPDXKeyReturnsErrorInsteadOfPanic(t *testing.T) {
	if key, err := decodePolyvPDXKey("not valid base64 !!!"); err == nil {
		t.Fatalf("decodePolyvPDXKey returned key %x without error", key)
	}
}

func TestDecryptPolyvPDXTextRejectsInvalidCiphertext(t *testing.T) {
	for _, ciphertext := range []string{"", "abcd"} {
		_, err := decryptPolyvPDXText(ciphertext)
		if err == nil {
			t.Fatalf("decryptPolyvPDXText(%q) should reject non block-aligned ciphertext", ciphertext)
		}
		if !strings.Contains(err.Error(), "block-aligned") {
			t.Fatalf("decryptPolyvPDXText(%q) error = %v, want block-aligned error", ciphertext, err)
		}
	}
}

func TestDecryptPolyvKeyRejectsEmptyCiphertext(t *testing.T) {
	_, err := decryptPolyvKey(nil)
	if err == nil {
		t.Fatal("decryptPolyvKey should reject empty ciphertext")
	}
	if !strings.Contains(err.Error(), "block-aligned") {
		t.Fatalf("error = %v, want block-aligned error", err)
	}
}

func TestDecryptPolyvKeyDoesNotStripInvalidPadding(t *testing.T) {
	if polyvPDXSecretErr != nil {
		t.Fatalf("polyvPDXSecretErr = %v", polyvPDXSecretErr)
	}
	plain := append([]byte("invalid-pad-"), 'A', 'B', 0x02, 0x03)
	if len(plain) != aes.BlockSize {
		t.Fatalf("test plaintext length = %d, want AES block size", len(plain))
	}
	block, err := aes.NewCipher(polyvPDXSecret)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	ciphertext := append([]byte(nil), plain...)
	cipher.NewCBCEncrypter(block, polyvPDXIV).CryptBlocks(ciphertext, ciphertext)
	got, err := decryptPolyvKey(ciphertext)
	if err != nil {
		t.Fatalf("decryptPolyvKey: %v", err)
	}
	if string(got) != string(plain) {
		t.Fatalf("plaintext = %q, want %q", got, plain)
	}
}
