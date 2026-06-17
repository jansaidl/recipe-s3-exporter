package crypto

import (
	"crypto/rand"
	"io"
	"testing"
)

func newKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, k); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c, err := New(newKey(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, pt := range []string{"", "hello", "a-zerops-integration-token-value", "🔒 secret"} {
		enc, err := c.Encrypt(pt)
		if err != nil {
			t.Fatalf("encrypt %q: %v", pt, err)
		}
		got, err := c.Decrypt(enc)
		if err != nil {
			t.Fatalf("decrypt %q: %v", pt, err)
		}
		if got != pt {
			t.Fatalf("round trip mismatch: got %q want %q", got, pt)
		}
	}
}

func TestEncryptIsNondeterministic(t *testing.T) {
	c, _ := New(newKey(t))
	a, _ := c.Encrypt("same")
	b, _ := c.Encrypt("same")
	if a == b {
		t.Fatal("expected distinct ciphertexts due to random nonce")
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	c1, _ := New(newKey(t))
	c2, _ := New(newKey(t))
	enc, _ := c1.Encrypt("top secret")
	if _, err := c2.Decrypt(enc); err == nil {
		t.Fatal("expected decryption with wrong key to fail")
	}
}

func TestNewRejectsBadKeyLength(t *testing.T) {
	if _, err := New([]byte("short")); err == nil {
		t.Fatal("expected error for short key")
	}
}
