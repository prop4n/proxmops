// Package crypt provides the symmetric encryption used to store secrets in
// the proxmops database: AES-256-GCM under a key held in a local key file, so
// the secrets never sit in clear text on disk. The key file lives next to the
// database; losing it makes the stored secrets unreadable.
package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvKey is the environment variable overriding the key file with a base64
// encoded 32-byte key.
const EnvKey = "PROXMOPS_ENCRYPTION_KEY"

// keySize is the AES-256 key length in bytes.
const keySize = 32

// Key is a 32-byte symmetric key.
type Key [keySize]byte

// ErrBadKey is returned when a supplied key has the wrong length.
var ErrBadKey = errors.New("crypt: key must be 32 bytes")

// LoadOrCreateKey returns the encryption key from, in order of precedence, the
// PROXMOPS_ENCRYPTION_KEY environment variable (base64 encoded) or the file at
// path, creating the file with restrictive permissions when missing.
func LoadOrCreateKey(path string) (Key, error) {
	if env := strings.TrimSpace(os.Getenv(EnvKey)); env != "" {
		return parseKey(env)
	}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key, genErr := generateKeyFile(path)
		if genErr != nil {
			return Key{}, genErr
		}
		return key, nil
	}
	if err != nil {
		return Key{}, fmt.Errorf("read key file: %w", err)
	}

	key, err := parseKey(string(raw))
	if err != nil {
		return Key{}, fmt.Errorf("key file %s: %w", path, err)
	}
	return key, nil
}

// Encrypt seals plaintext under the key, returning a base64 blob. An empty
// plaintext returns an empty blob so callers do not need special cases.
func (k Key) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	aead, err := k.aead()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("crypt: nonce: %w", err)
	}
	sealed := aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt opens a blob produced by Encrypt. An empty blob returns an empty
// plaintext.
func (k Key) Decrypt(blob string) (string, error) {
	if blob == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return "", fmt.Errorf("crypt: decode blob: %w", err)
	}
	aead, err := k.aead()
	if err != nil {
		return "", err
	}
	if len(raw) < aead.NonceSize() {
		return "", errors.New("crypt: blob too short")
	}
	nonce, ciphertext := raw[:aead.NonceSize()], raw[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("crypt: decrypt: %w", err)
	}
	return string(plaintext), nil
}

// aead builds the AES-GCM cipher for the key.
func (k Key) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, fmt.Errorf("crypt: aes: %w", err)
	}
	return cipher.NewGCM(block)
}

// parseKey decodes a base64 encoded key, checking its length.
func parseKey(encoded string) (Key, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return Key{}, fmt.Errorf("crypt: decode key: %w", err)
	}
	if len(raw) != keySize {
		return Key{}, ErrBadKey
	}
	var key Key
	copy(key[:], raw)
	return key, nil
}

// generateKeyFile writes a fresh random key to path and returns it.
func generateKeyFile(path string) (Key, error) {
	var key Key
	if _, err := rand.Read(key[:]); err != nil {
		return Key{}, fmt.Errorf("crypt: generate key: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(key[:])
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Key{}, fmt.Errorf("crypt: key directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(encoded), 0o600); err != nil {
		return Key{}, fmt.Errorf("crypt: write key file: %w", err)
	}
	return key, nil
}
