package secrets

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/launchpad/launchpad/pkg/launchpad"
)

func testKeyB64(t *testing.T) string {
	t.Helper()
	// Fixed 32-byte key for deterministic tests.
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 1)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	box, err := ParseKey(testKeyB64(t))
	if err != nil {
		t.Fatal(err)
	}
	plain := "postgres://user:pass@host/db"
	ct, err := box.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !IsCiphertext(ct) {
		t.Fatalf("expected ciphertext prefix, got %q", ct)
	}
	if strings.Contains(ct, plain) {
		t.Fatal("ciphertext must not contain plaintext")
	}
	got, err := box.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Fatalf("got %q want %q", got, plain)
	}
	// Distinct nonces → distinct ciphertext.
	ct2, err := box.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if ct == ct2 {
		t.Fatal("expected different ciphertext for same plaintext (nonce)")
	}
}

func TestParseKeyRejectsBadLength(t *testing.T) {
	_, err := ParseKey(base64.StdEncoding.EncodeToString([]byte("short")))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	box1, err := ParseKey(testKeyB64(t))
	if err != nil {
		t.Fatal(err)
	}
	raw2 := make([]byte, 32)
	for i := range raw2 {
		raw2[i] = byte(100 + i)
	}
	box2, err := ParseKey(base64.StdEncoding.EncodeToString(raw2))
	if err != nil {
		t.Fatal(err)
	}
	ct, err := box1.Encrypt("secret")
	if err != nil {
		t.Fatal(err)
	}
	_, err = box2.Decrypt(ct)
	if err == nil {
		t.Fatal("expected decrypt failure")
	}
}

func TestNilBoxEncrypt(t *testing.T) {
	var box *Box
	_, err := box.Encrypt("x")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv(EnvKey, "")
	box, err := LoadFromEnv()
	if err != nil || box != nil {
		t.Fatalf("empty env: box=%v err=%v", box, err)
	}
	t.Setenv(EnvKey, testKeyB64(t))
	box, err = LoadFromEnv()
	if err != nil || box == nil {
		t.Fatalf("set env: box=%v err=%v", box, err)
	}
	if _, err := box.Encrypt("a"); err != nil {
		t.Fatal(err)
	}
}

func TestParseKeyEmpty(t *testing.T) {
	_, err := ParseKey("  ")
	if err == nil || !strings.Contains(err.Error(), launchpad.ErrBadRequest.Error()) && err != launchpad.ErrBadRequest {
		// wrapped ErrBadRequest
		if err == nil {
			t.Fatal("expected error")
		}
	}
}
