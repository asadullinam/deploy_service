//go:build !integration

package crypto

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	svc, err := NewService(key)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	plaintext := "apiVersion: v1\nclusters: []\nkind: Config"
	enc, err := svc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	dec, err := svc.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != plaintext {
		t.Errorf("roundtrip mismatch: got %q", dec)
	}
}

func TestEncryptNonDeterministic(t *testing.T) {
	key := make([]byte, 32)
	svc, _ := NewService(key)
	enc1, _ := svc.Encrypt("hello")
	enc2, _ := svc.Encrypt("hello")
	if enc1 == enc2 {
		t.Error("encrypt should be non-deterministic (random nonce)")
	}
}

func TestWrongKeyFails(t *testing.T) {
	key1 := make([]byte, 32)
	key1[0] = 1
	key2 := make([]byte, 32)
	svc1, _ := NewService(key1)
	svc2, _ := NewService(key2)
	enc, _ := svc1.Encrypt("secret data")
	_, err := svc2.Decrypt(enc)
	if err == nil {
		t.Error("wrong key should fail decryption")
	}
}

func TestInvalidKeyLength(t *testing.T) {
	for _, size := range []int{0, 16, 31, 33, 64} {
		_, err := NewService(make([]byte, size))
		if err == nil {
			t.Errorf("key of %d bytes should be rejected", size)
		}
	}
}

func TestDecryptInvalidBase64(t *testing.T) {
	key := make([]byte, 32)
	svc, _ := NewService(key)
	_, err := svc.Decrypt("not-valid-base64!!!")
	if err == nil {
		t.Error("invalid base64 should fail")
	}
}

func TestDecryptTooShort(t *testing.T) {
	key := make([]byte, 32)
	svc, _ := NewService(key)
	// Base64 из 5 байт — короче nonce GCM (12 байт)
	_, err := svc.Decrypt("AAAAAAAAAA==")
	if err == nil || !strings.Contains(err.Error(), "short") {
		t.Errorf("too-short ciphertext should fail with 'short', got: %v", err)
	}
}
