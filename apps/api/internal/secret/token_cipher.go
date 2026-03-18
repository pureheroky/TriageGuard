package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"
)

const tokenCipherPrefix = "enc:v1:"

type TokenCipher struct {
	key []byte
}

func NewTokenCipher(rawKey string) (*TokenCipher, error) {
	trimmed := strings.TrimSpace(rawKey)
	if trimmed == "" {
		return nil, nil
	}

	decoded, err := decodeKey(trimmed)
	if err != nil {
		return nil, err
	}
	if len(decoded) != 32 {
		return nil, errors.New("TOKENS_ENCRYPTION_KEY must decode to 32 bytes")
	}
	return &TokenCipher{key: decoded}, nil
}

func decodeKey(raw string) ([]byte, error) {
	attempts := []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
		hex.DecodeString,
	}
	for _, attempt := range attempts {
		decoded, err := attempt(raw)
		if err == nil {
			return decoded, nil
		}
	}
	return nil, errors.New("invalid TOKENS_ENCRYPTION_KEY encoding")
}

func (c *TokenCipher) Encrypt(value string) (string, error) {
	if c == nil {
		return value, nil
	}
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	if strings.HasPrefix(value, tokenCipherPrefix) {
		return value, nil
	}

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	sealed := gcm.Seal(nil, nonce, []byte(value), nil)
	combined := append(nonce, sealed...)
	return tokenCipherPrefix + base64.RawURLEncoding.EncodeToString(combined), nil
}

func (c *TokenCipher) Decrypt(value string) (string, error) {
	if c == nil {
		return value, nil
	}
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, tokenCipherPrefix) {
		return value, nil
	}

	raw := strings.TrimPrefix(value, tokenCipherPrefix)
	combined, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(combined) < gcm.NonceSize() {
		return "", errors.New("invalid encrypted token payload")
	}

	nonce := combined[:gcm.NonceSize()]
	ciphertext := combined[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
