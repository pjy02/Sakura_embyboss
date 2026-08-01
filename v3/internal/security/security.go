package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/scrypt"
)

func RandomToken(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func HashToken(value string) []byte     { sum := sha256.Sum256([]byte(value)); return sum[:] }
func EqualHash(left, right []byte) bool { return subtle.ConstantTimeCompare(left, right) == 1 }

func HashPassword(password string) (string, error) {
	if len(password) < 10 || len(password) > 128 {
		return "", errors.New("password must contain 10 to 128 characters")
	}
	if strings.TrimSpace(password) != password {
		return "", errors.New("password must not start or end with whitespace")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	digest := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(digest)), nil
}

func VerifyPassword(password, encoded string) bool {
	if strings.HasPrefix(encoded, "$argon2id$") {
		return verifyArgon(password, encoded)
	}
	if strings.HasPrefix(encoded, "scrypt$") {
		return verifyLegacyScrypt(password, encoded)
	}
	return false
}

func verifyArgon(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false
	}
	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	expected, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil || memory < 8*1024 || memory > 256*1024 || iterations < 1 || iterations > 10 || parallelism < 1 || parallelism > 16 || len(salt) < 8 || len(salt) > 64 || len(expected) < 16 || len(expected) > 64 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expected)))
	return EqualHash(actual, expected)
}

func verifyLegacyScrypt(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false
	}
	n, e1 := strconv.Atoi(parts[1])
	r, e2 := strconv.Atoi(parts[2])
	p, e3 := strconv.Atoi(parts[3])
	salt, e4 := base64.URLEncoding.DecodeString(parts[4])
	expected, e5 := base64.URLEncoding.DecodeString(parts[5])
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || n != 1<<14 || r != 8 || p != 1 || len(salt) != 16 || len(expected) != 32 {
		return false
	}
	actual, err := scrypt.Key([]byte(password), salt, n, r, p, len(expected))
	return err == nil && EqualHash(actual, expected)
}

type Vault struct {
	key     []byte
	version int
}

func NewVault(encoded string) (*Vault, error) {
	var key []byte
	var err error
	if len(encoded) == 64 {
		key, err = hex.DecodeString(encoded)
	} else {
		key, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil || len(key) != 32 {
		return nil, errors.New("credential master key must be 32 bytes encoded as 64 hex or unpadded base64")
	}
	return &Vault{key: key, version: 1}, nil
}

func (v *Vault) Encrypt(plaintext []byte) ([]byte, []byte, int, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, nil, 0, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, 0, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, nil, 0, err
	}
	return aead.Seal(nil, nonce, plaintext, nil), nonce, v.version, nil
}

func (v *Vault) Decrypt(ciphertext, nonce []byte, version int) ([]byte, error) {
	if version != v.version {
		return nil, fmt.Errorf("unsupported credential key version %d", version)
	}
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, ciphertext, nil)
}
