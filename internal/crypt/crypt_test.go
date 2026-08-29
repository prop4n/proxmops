package crypt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key, err := LoadOrCreateKey(filepath.Join(t.TempDir(), "key"))
	if err != nil {
		t.Fatalf("LoadOrCreateKey: %v", err)
	}

	const secret = "pve-token-secret"
	blob, err := key.Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if blob == secret {
		t.Fatal("blob equals plaintext")
	}

	got, err := key.Decrypt(blob)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != secret {
		t.Errorf("Decrypt = %q, want %q", got, secret)
	}
}

func TestEncryptDecryptEmpty(t *testing.T) {
	key := Key{}
	if blob, err := key.Encrypt(""); err != nil || blob != "" {
		t.Errorf("Encrypt(\"\") = %q, %v, want empty", blob, err)
	}
	if got, err := key.Decrypt(""); err != nil || got != "" {
		t.Errorf("Decrypt(\"\") = %q, %v, want empty", got, err)
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	dir := t.TempDir()
	first, err := LoadOrCreateKey(filepath.Join(dir, "a"))
	if err != nil {
		t.Fatalf("LoadOrCreateKey: %v", err)
	}
	second, err := LoadOrCreateKey(filepath.Join(dir, "b"))
	if err != nil {
		t.Fatalf("LoadOrCreateKey: %v", err)
	}

	blob, err := first.Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := second.Decrypt(blob); err == nil {
		t.Fatal("Decrypt with wrong key succeeded")
	}
}

func TestLoadOrCreateKeyPersistsAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "key")

	first, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("key file permissions = %o, want 600", perm)
	}

	second, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if first != second {
		t.Error("reloaded key differs from created key")
	}
}

func TestLoadKeyFromEnv(t *testing.T) {
	known := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	t.Setenv(EnvKey, known)

	key, err := LoadOrCreateKey(filepath.Join(t.TempDir(), "key"))
	if err != nil {
		t.Fatalf("LoadOrCreateKey: %v", err)
	}
	var want Key
	if key != want {
		t.Error("env key not honoured")
	}
}

func TestLoadKeyRejectsBadEnv(t *testing.T) {
	t.Setenv(EnvKey, "too-short")
	if _, err := LoadOrCreateKey(filepath.Join(t.TempDir(), "key")); err == nil {
		t.Fatal("expected error for short key")
	}
}
