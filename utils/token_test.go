package utils

import (
	"strings"
	"testing"
	"time"
)

func TestTokenSigner_RoundTrip(t *testing.T) {
	signer := NewTokenSigner("test-secret")

	token := signer.GenerateDownloadToken("my-bucket", "photo.jpg", 15*time.Minute)

	bucket, filename, err := signer.ValidateDownloadToken(token)
	if err != nil {
		t.Fatalf("unexpected error validating fresh token: %v", err)
	}
	if bucket != "my-bucket" {
		t.Errorf("bucket = %q, want %q", bucket, "my-bucket")
	}
	if filename != "photo.jpg" {
		t.Errorf("filename = %q, want %q", filename, "photo.jpg")
	}
}

func TestTokenSigner_Expired(t *testing.T) {
	signer := NewTokenSigner("test-secret")

	token := signer.GenerateDownloadToken("my-bucket", "photo.jpg", -1*time.Minute)

	if _, _, err := signer.ValidateDownloadToken(token); err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestTokenSigner_TamperedSignature(t *testing.T) {
	signer := NewTokenSigner("test-secret")

	token := signer.GenerateDownloadToken("my-bucket", "photo.jpg", 15*time.Minute)
	parts := strings.SplitN(token, ".", 2)
	tampered := parts[0] + ".tamperedSignatureXXXXXXXXXXXXXXXXXXXXXXXXX"

	if _, _, err := signer.ValidateDownloadToken(tampered); err == nil {
		t.Fatal("expected error for tampered signature, got nil")
	}
}

func TestTokenSigner_TamperedPayload(t *testing.T) {
	signer := NewTokenSigner("test-secret")

	// Un token válido para OTRO archivo no debería validar para "secret.pdf":
	// si alguien reemplaza el payload por el de otro token, la firma deja de
	// matchear.
	tokenA := signer.GenerateDownloadToken("bucket-a", "public.jpg", 15*time.Minute)
	tokenB := signer.GenerateDownloadToken("bucket-b", "secret.pdf", 15*time.Minute)

	payloadA := strings.SplitN(tokenA, ".", 2)[0]
	sigB := strings.SplitN(tokenB, ".", 2)[1]
	frankensteinToken := payloadA + "." + sigB

	if _, _, err := signer.ValidateDownloadToken(frankensteinToken); err == nil {
		t.Fatal("expected error mixing payload/signature from different tokens, got nil")
	}
}

func TestTokenSigner_WrongSecret(t *testing.T) {
	signer := NewTokenSigner("secret-a")
	other := NewTokenSigner("secret-b")

	token := signer.GenerateDownloadToken("my-bucket", "photo.jpg", 15*time.Minute)

	if _, _, err := other.ValidateDownloadToken(token); err == nil {
		t.Fatal("expected error validating token signed with a different secret, got nil")
	}
}

func TestTokenSigner_Malformed(t *testing.T) {
	signer := NewTokenSigner("test-secret")

	cases := []string{
		"",
		"no-dot-separator",
		"a.b.c",
		"not-base64!!!.not-base64!!!",
	}

	for _, tc := range cases {
		if _, _, err := signer.ValidateDownloadToken(tc); err == nil {
			t.Errorf("token %q: expected error, got nil", tc)
		}
	}
}
