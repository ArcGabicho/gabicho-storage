// Package utils incluye helpers de tokens firmados (para links de descarga
// presignados) y validación de entrada.
package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// TokenSigner firma y valida tokens de descarga presignados de forma
// stateless (sin necesidad de persistirlos en base de datos): el propio
// token incluye el bucket, el archivo y su expiración, autenticados con
// HMAC-SHA256 contra un secreto del servidor.
type TokenSigner struct {
	secret []byte
}

// NewTokenSigner crea un firmador con el secreto dado.
func NewTokenSigner(secret string) *TokenSigner {
	return &TokenSigner{secret: []byte(secret)}
}

// GenerateDownloadToken crea un token presignado para bucket/filename válido
// por ttl a partir de ahora.
func (s *TokenSigner) GenerateDownloadToken(bucket, filename string, ttl time.Duration) string {
	expiry := time.Now().Add(ttl).Unix()
	payload := fmt.Sprintf("%s|%s|%d", bucket, filename, expiry)
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte(payload))
	sig := s.sign(payloadB64)
	return payloadB64 + "." + sig
}

// ValidateDownloadToken valida la firma y expiración de un token, devolviendo
// el bucket y filename originales si es válido.
func (s *TokenSigner) ValidateDownloadToken(token string) (bucket, filename string, err error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("token malformado")
	}
	payloadB64, sig := parts[0], parts[1]

	expectedSig := s.sign(payloadB64)
	if subtle.ConstantTimeCompare([]byte(sig), []byte(expectedSig)) != 1 {
		return "", "", fmt.Errorf("firma de token inválida")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return "", "", fmt.Errorf("payload de token inválido")
	}

	fields := strings.SplitN(string(payloadBytes), "|", 3)
	if len(fields) != 3 {
		return "", "", fmt.Errorf("payload de token inválido")
	}

	expiry, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return "", "", fmt.Errorf("expiración de token inválida")
	}
	if time.Now().Unix() > expiry {
		return "", "", fmt.Errorf("token expirado")
	}

	return fields[0], fields[1], nil
}

func (s *TokenSigner) sign(payloadB64 string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payloadB64))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
