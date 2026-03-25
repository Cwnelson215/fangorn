package crypto

import (
	"crypto/rand"
	"encoding/hex"
	"testing"
)

func testKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(key)
}

func TestRoundTrip(t *testing.T) {
	key := testKey(t)
	plaintext := "access-sandbox-abc123-test-token"

	encrypted, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	if encrypted == plaintext {
		t.Fatal("encrypted should differ from plaintext")
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if decrypted != plaintext {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}
}

func TestWrongKey(t *testing.T) {
	key1 := testKey(t)
	key2 := testKey(t)

	encrypted, err := Encrypt("secret", key1)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err = Decrypt(encrypted, key2)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestInvalidKeyLength(t *testing.T) {
	_, err := Encrypt("test", "abcd")
	if err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestUniqueNonces(t *testing.T) {
	key := testKey(t)
	enc1, _ := Encrypt("same", key)
	enc2, _ := Encrypt("same", key)
	if enc1 == enc2 {
		t.Fatal("encryptions of same plaintext should differ (unique nonces)")
	}
}
