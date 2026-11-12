package cloudflare

import (
	"encoding/base64"
	"strings"
	"testing"
)

const testJWTSecret = "unit-test-jwt-secret"

func TestSealOpenSecretRoundTrip(t *testing.T) {
	t.Setenv("JWT_SECRET", testJWTSecret)

	sealed, err := SealSecret("cf-api-token-value")
	if err != nil {
		t.Fatalf("SealSecret: %v", err)
	}
	if !strings.HasPrefix(sealed, "v1:") {
		t.Fatalf("sealed = %q, want v1: prefix", sealed)
	}
	blob, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, "v1:"))
	if err != nil {
		t.Fatalf("decode sealed payload: %v", err)
	}
	if len(blob) != 12+len("cf-api-token-value")+16 {
		t.Fatalf("blob length = %d", len(blob))
	}

	plain, err := OpenSecret(sealed)
	if err != nil {
		t.Fatalf("OpenSecret: %v", err)
	}
	if plain != "cf-api-token-value" {
		t.Fatalf("plain = %q", plain)
	}
}

func TestSealSecretUsesFreshNonce(t *testing.T) {
	t.Setenv("JWT_SECRET", testJWTSecret)

	first, err := SealSecret("same-value")
	if err != nil {
		t.Fatalf("SealSecret: %v", err)
	}
	second, err := SealSecret("same-value")
	if err != nil {
		t.Fatalf("SealSecret: %v", err)
	}
	if first == second {
		t.Fatal("two seals of the same value are identical")
	}
}

func TestOpenSecretPassesThroughPlainValue(t *testing.T) {
	t.Setenv("JWT_SECRET", testJWTSecret)

	plain, err := OpenSecret("legacy-plain-token")
	if err != nil {
		t.Fatalf("OpenSecret: %v", err)
	}
	if plain != "legacy-plain-token" {
		t.Fatalf("plain = %q", plain)
	}
}

func TestOpenSecretRejectsTamperedCiphertext(t *testing.T) {
	t.Setenv("JWT_SECRET", testJWTSecret)

	sealed, err := SealSecret("cf-api-token-value")
	if err != nil {
		t.Fatalf("SealSecret: %v", err)
	}
	blob, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, "v1:"))
	if err != nil {
		t.Fatalf("decode sealed payload: %v", err)
	}
	blob[len(blob)-1] ^= 0x01
	tampered := "v1:" + base64.StdEncoding.EncodeToString(blob)

	if _, err := OpenSecret(tampered); err == nil {
		t.Fatal("OpenSecret accepted tampered ciphertext")
	}
}

func TestOpenSecretRejectsWrongKey(t *testing.T) {
	t.Setenv("JWT_SECRET", testJWTSecret)
	sealed, err := SealSecret("cf-api-token-value")
	if err != nil {
		t.Fatalf("SealSecret: %v", err)
	}

	t.Setenv("JWT_SECRET", "another-jwt-secret")
	if _, err := OpenSecret(sealed); err == nil {
		t.Fatal("OpenSecret accepted a payload sealed with another key")
	}
}

func TestSecretWithoutJWTSecretFailsClosed(t *testing.T) {
	t.Setenv("JWT_SECRET", "")

	if _, err := SealSecret("value"); err == nil {
		t.Fatal("SealSecret without JWT_SECRET returned no error")
	}
	if _, err := OpenSecret("v1:AAAA"); err == nil {
		t.Fatal("OpenSecret without JWT_SECRET returned no error")
	}
	plain, err := OpenSecret("legacy-plain-token")
	if err != nil {
		t.Fatalf("OpenSecret passthrough: %v", err)
	}
	if plain != "legacy-plain-token" {
		t.Fatalf("plain = %q", plain)
	}
}
