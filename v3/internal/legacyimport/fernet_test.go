package legacyimport

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestDecryptV2Credential(t *testing.T) {
	master, plaintext := "v2-master-secret-at-least-32-bytes", []byte("emby-api-key")
	key := sha256.Sum256([]byte(master))
	iv := []byte("0123456789abcdef")
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	padded := append(append([]byte{}, plaintext...), make([]byte, padding)...)
	for index := len(padded) - padding; index < len(padded); index++ {
		padded[index] = byte(padding)
	}
	block, _ := aes.NewCipher(key[16:])
	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, padded)
	raw := append([]byte{0x80, 0, 0, 0, 0, 0, 0, 0, 1}, iv...)
	raw = append(raw, encrypted...)
	mac := hmac.New(sha256.New, key[:16])
	_, _ = mac.Write(raw)
	raw = append(raw, mac.Sum(nil)...)
	token := base64.URLEncoding.EncodeToString(raw)
	got, err := decryptV2Credential(master, token)
	if err != nil || string(got) != string(plaintext) {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err = decryptV2Credential("wrong-master", token); err == nil {
		t.Fatal("wrong key must fail")
	}
}
