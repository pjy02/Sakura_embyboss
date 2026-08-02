package legacyimport

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
)

// decryptV2Credential implements the Fernet token format used by the Python
// v2 credential center. v2 derives the 32-byte Fernet key as SHA-256(master).
func decryptV2Credential(master, token string) ([]byte, error) {
	key := sha256.Sum256([]byte(master))
	raw, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(token)
	}
	if err != nil || len(raw) < 1+8+aes.BlockSize+sha256.Size || raw[0] != 0x80 {
		return nil, errors.New("invalid v2 Fernet token")
	}
	signed, providedMAC := raw[:len(raw)-sha256.Size], raw[len(raw)-sha256.Size:]
	mac := hmac.New(sha256.New, key[:16])
	_, _ = mac.Write(signed)
	if subtle.ConstantTimeCompare(mac.Sum(nil), providedMAC) != 1 {
		return nil, errors.New("v2 credential authentication failed")
	}
	iv := raw[9 : 9+aes.BlockSize]
	ciphertext := raw[9+aes.BlockSize : len(raw)-sha256.Size]
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, errors.New("invalid v2 credential ciphertext")
	}
	block, err := aes.NewCipher(key[16:])
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)
	padding := int(plaintext[len(plaintext)-1])
	if padding < 1 || padding > aes.BlockSize || padding > len(plaintext) {
		return nil, errors.New("invalid v2 credential padding")
	}
	for _, value := range plaintext[len(plaintext)-padding:] {
		if int(value) != padding {
			return nil, errors.New("invalid v2 credential padding")
		}
	}
	return plaintext[:len(plaintext)-padding], nil
}
