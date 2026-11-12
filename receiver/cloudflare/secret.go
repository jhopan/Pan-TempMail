package cloudflare

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"strings"
)

const secretPrefix = "v1:"

func secretKey() ([]byte, error) {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, errors.New("cloudflare: JWT_SECRET is not set")
	}
	sum := sha256.Sum256([]byte("tempmail:cf:" + jwtSecret))
	return sum[:], nil
}

func secretGCM() (cipher.AEAD, error) {
	key, err := secretKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func SealSecret(plain string) (string, error) {
	gcm, err := secretGCM()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return secretPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// OpenSecret returns legacy unsealed values unchanged so imported state keeps working.
func OpenSecret(sealed string) (string, error) {
	if !strings.HasPrefix(sealed, secretPrefix) {
		return sealed, nil
	}
	gcm, err := secretGCM()
	if err != nil {
		return "", err
	}
	blob, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, secretPrefix))
	if err != nil {
		return "", errors.New("cloudflare: sealed secret is not valid base64")
	}
	if len(blob) < gcm.NonceSize() {
		return "", errors.New("cloudflare: sealed secret is truncated")
	}
	plain, err := gcm.Open(nil, blob[:gcm.NonceSize()], blob[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("cloudflare: sealed secret failed to decrypt")
	}
	return string(plain), nil
}
