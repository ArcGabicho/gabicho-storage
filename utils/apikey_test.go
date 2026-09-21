package utils

import (
	"strings"
	"testing"
)

func TestGenerateAndVerifyAPIKey(t *testing.T) {
	id := "f350e8f7-c377-4083-ba2e-e372a5f07554" // uuid-shaped, largo real
	raw, hash, err := GenerateAPIKey(id)
	if err != nil {
		t.Fatalf("GenerateAPIKey returned error: %v", err)
	}

	if !strings.HasPrefix(raw, "sk_"+id+"_") {
		t.Fatalf("raw key %q does not have expected prefix sk_%s_", raw, id)
	}

	if !VerifyAPIKey(raw, hash) {
		t.Fatal("VerifyAPIKey should succeed for the key that produced the hash")
	}
}

func TestGenerateAPIKey_UnderBcryptLimit(t *testing.T) {
	// Regresión: bcrypt rechaza inputs > 72 bytes. Con un id largo tipo UUID,
	// la key completa "sk_<uuid>_<secret>" supera ese límite; sólo el secreto
	// debe hashearse, nunca la key completa.
	id := "f350e8f7-c377-4083-ba2e-e372a5f07554"
	raw, _, err := GenerateAPIKey(id)
	if err != nil {
		t.Fatalf("GenerateAPIKey returned error: %v", err)
	}
	if len(raw) <= 72 {
		t.Fatalf("test setup inválido: la raw key (%d bytes) debería superar el límite de bcrypt para ser una regresión útil", len(raw))
	}
}

func TestParseAPIKeyID(t *testing.T) {
	id := "some-id-123"
	raw, _, err := GenerateAPIKey(id)
	if err != nil {
		t.Fatalf("GenerateAPIKey returned error: %v", err)
	}

	parsed, err := ParseAPIKeyID(raw)
	if err != nil {
		t.Fatalf("ParseAPIKeyID returned error: %v", err)
	}
	if parsed != id {
		t.Errorf("parsed id = %q, want %q", parsed, id)
	}
}

func TestParseAPIKeyID_InvalidFormat(t *testing.T) {
	cases := []string{"", "notasgnkey", "wrongprefix_id_secret", "sk_onlytwoparts"}
	for _, tc := range cases {
		if _, err := ParseAPIKeyID(tc); err == nil {
			t.Errorf("ParseAPIKeyID(%q): expected error, got nil", tc)
		}
	}
}

func TestVerifyAPIKey_WrongSecret(t *testing.T) {
	id := "some-id-123"
	_, hash, err := GenerateAPIKey(id)
	if err != nil {
		t.Fatalf("GenerateAPIKey returned error: %v", err)
	}

	forged := "sk_" + id + "_totally-wrong-secret"
	if VerifyAPIKey(forged, hash) {
		t.Fatal("VerifyAPIKey should fail for a forged secret")
	}
}

func TestVerifyAPIKey_MalformedKey(t *testing.T) {
	_, hash, err := GenerateAPIKey("some-id")
	if err != nil {
		t.Fatalf("GenerateAPIKey returned error: %v", err)
	}

	if VerifyAPIKey("garbage-without-underscores", hash) {
		t.Fatal("VerifyAPIKey should fail for a malformed key")
	}
}

func TestGenerateAPIKey_UniqueSecrets(t *testing.T) {
	raw1, _, err := GenerateAPIKey("id")
	if err != nil {
		t.Fatalf("GenerateAPIKey returned error: %v", err)
	}
	raw2, _, err := GenerateAPIKey("id")
	if err != nil {
		t.Fatalf("GenerateAPIKey returned error: %v", err)
	}
	if raw1 == raw2 {
		t.Fatal("two calls to GenerateAPIKey produced the same secret")
	}
}
