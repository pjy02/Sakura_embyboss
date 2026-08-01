package security

import (
	"encoding/base64"
	"fmt"
	"testing"

	"golang.org/x/crypto/scrypt"
)

func TestPasswordAndLegacyCompatibility(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("correct horse battery staple", hash) || VerifyPassword("wrong password", hash) {
		t.Fatal("argon2 verification failed")
	}
	salt := []byte("0123456789abcdef")
	digest, err := scrypt.Key([]byte("legacy password"), salt, 1<<14, 8, 1, 32)
	if err != nil {
		t.Fatal(err)
	}
	legacy := fmt.Sprintf("scrypt$16384$8$1$%s$%s", base64.URLEncoding.EncodeToString(salt), base64.URLEncoding.EncodeToString(digest))
	if !VerifyPassword("legacy password", legacy) || VerifyPassword("wrong password", legacy) {
		t.Fatal("v2 scrypt verification failed")
	}
}

func TestVaultRoundTrip(t *testing.T) {
	vault, err := NewVault("00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, nonce, version, err := vault.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := vault.Decrypt(ciphertext, nonce, version)
	if err != nil || string(plain) != "secret" {
		t.Fatal("vault round trip failed")
	}
}
