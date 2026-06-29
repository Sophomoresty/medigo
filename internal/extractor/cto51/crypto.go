package cto51

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"regexp"
	"strings"
	"sync"
)

const cto51RSAPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQC3pDA7GTxOvNbXRGMi9QSIzQEI
+EMD1HcUPJSQSFuRkZkWo4VQECuPRg/xVjqwX1yUrHUvGQJsBwTS/6LIcQiSwYsO
qf+8TWxGQOJyW46gPPQVzTjNTiUoq435QB0v11lNxvKWBQIZLmacUZ2r1APta7i/
MY4Lx9XlZVMZNUdUywIDAQAB
-----END PUBLIC KEY-----`

var cto51RSAPublicKeyCache struct {
	once sync.Once
	key  *rsa.PublicKey
}

func rsaEncryptOverlay(overlay string) string {
	key := cto51PublicKey()
	if key == nil || overlay == "" {
		return ""
	}
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, key, []byte(overlay))
	if err != nil {
		return ""
	}
	return hex.EncodeToString(ciphertext)
}

func cto51PublicKey() *rsa.PublicKey {
	cto51RSAPublicKeyCache.once.Do(func() {
		block, _ := pem.Decode([]byte(cto51RSAPublicKeyPEM))
		if block == nil {
			return
		}
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return
		}
		if key, ok := pub.(*rsa.PublicKey); ok {
			cto51RSAPublicKeyCache.key = key
		}
	})
	return cto51RSAPublicKeyCache.key
}

func cto51SecDataFromAuth(auth map[string]string, playResponse map[string]any) []byte {
	if direct := firstNonEmpty(auth["sec_data"], deepFindText(playResponse, "secData", "sec_data", "SecData", "SecurityData", "securityData")); direct != "" {
		if key := cto51KeyBytesFromText(direct); len(key) > 0 {
			return key
		}
	}
	seed := firstNonEmpty(auth["seed"], deepFindText(playResponse, "seed", "Seed"))
	randValue := firstNonEmpty(deepFindText(playResponse, "Rand", "rand"), auth["rand"])
	plaintext := firstNonEmpty(deepFindText(playResponse, "Plaintext", "plaintext", "plainText"), auth["plaintext"])
	return derive51ctoSecData(seed, randValue, plaintext)
}

func derive51ctoSecData(seed, randValue, plaintext string) []byte {
	seed = strings.TrimSpace(seed)
	randValue = strings.TrimSpace(randValue)
	plaintext = strings.TrimSpace(plaintext)
	if seed == "" || randValue == "" || plaintext == "" {
		return nil
	}
	keyText := md5Middle16(seed)
	key1 := []byte(keyText)
	randPlain := decrypt51ctoCBCText(randValue, key1, key1)
	if randPlain == "" {
		return nil
	}
	key2 := []byte(md5Middle16(seed + randPlain))
	secretText := decrypt51ctoCBCText(plaintext, key2, key1)
	if secretText == "" {
		return nil
	}
	return cto51KeyBytesFromText(secretText)
}

func decrypt51ctoCBCText(ciphertext string, keyBytes, ivBytes []byte) string {
	if len(keyBytes) == 0 || len(ivBytes) != aes.BlockSize {
		return ""
	}
	encrypted := safeBase64Decode51cto(ciphertext)
	if len(encrypted) == 0 || len(encrypted)%aes.BlockSize != 0 {
		return ""
	}
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return ""
	}
	plain := make([]byte, len(encrypted))
	cipher.NewCBCDecrypter(block, ivBytes).CryptBlocks(plain, encrypted)
	plain = stripPKCS7(plain)
	plain = []byte(strings.TrimRight(string(plain), "\x00"))
	return strings.TrimSpace(string(plain))
}

func safeBase64Decode51cto(s string) []byte {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(strings.ReplaceAll(s, "-", "+"), "_", "/")
	padded := s + strings.Repeat("=", (4-len(s)%4)%4)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		if b, err := enc.DecodeString(padded); err == nil {
			return b
		}
		if b, err := enc.DecodeString(s); err == nil {
			return b
		}
	}
	return nil
}

func stripPKCS7(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	pad := int(data[len(data)-1])
	if pad <= 0 || pad > aes.BlockSize || pad > len(data) {
		return data
	}
	for _, b := range data[len(data)-pad:] {
		if int(b) != pad {
			return data
		}
	}
	return data[:len(data)-pad]
}

var cto51HexTextRe = regexp.MustCompile(`(?i)^(?:0x)?[0-9a-f]+$`)

func cto51KeyBytesFromText(text string) []byte {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if b := safeBase64Decode51cto(text); len(b) > 0 {
		if isAESKeyLen(len(b)) {
			return b
		}
		if key := cto51HexKeyBytes(string(b)); len(key) > 0 {
			return key
		}
	}
	return cto51HexKeyBytes(text)
}

func cto51HexKeyBytes(text string) []byte {
	compact := regexp.MustCompile(`\s+`).ReplaceAllString(strings.TrimSpace(text), "")
	if !cto51HexTextRe.MatchString(compact) {
		return nil
	}
	compact = strings.TrimPrefix(strings.TrimPrefix(compact, "0x"), "0X")
	if len(compact) != 32 && len(compact) != 48 && len(compact) != 64 {
		return nil
	}
	b, err := hex.DecodeString(compact)
	if err != nil || !isAESKeyLen(len(b)) {
		return nil
	}
	return b
}

func isAESKeyLen(n int) bool {
	return n == 16 || n == 24 || n == 32
}

func md5Middle16(text string) string {
	sum := md5.Sum([]byte(text))
	return hex.EncodeToString(sum[:])[8:24]
}
