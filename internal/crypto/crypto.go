// Package crypto provides authenticated symmetric encryption for secrets stored
// at rest (Zerops tokens, S3 credentials). It uses AES-256-GCM with a random
// per-message nonce and a master key supplied via the environment.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// Cipher encrypts and decrypts short secrets with a fixed master key.
type Cipher struct {
	aead cipher.AEAD
}

// New builds a Cipher from a 32-byte master key (AES-256).
func New(masterKey []byte) (*Cipher, error) {
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes, got %d", len(masterKey))
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt seals plaintext and returns a base64 string of nonce||ciphertext.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. It returns an error if the data is corrupt or was
// produced with a different key.
func (c *Cipher) Decrypt(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode secret: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := raw[:ns], raw[ns:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt secret (wrong MASTER_KEY?): %w", err)
	}
	return string(plaintext), nil
}
