package utils

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const apiKeyPrefix = "sk"

// GenerateAPIKey crea una nueva key en formato "sk_<id>_<secret>" y el hash
// bcrypt de su secreto. El id permite un lookup O(1) en base de datos sin
// necesitar comparar bcrypt contra todas las keys existentes; sólo el
// secreto (nunca el id, que no es sensible) se hashea y verifica con bcrypt,
// tanto porque es la parte realmente confidencial como para no exceder el
// límite de 72 bytes de entrada que impone bcrypt.
func GenerateAPIKey(id string) (rawKey string, hash string, err error) {
	secretBytes := make([]byte, 32)
	if _, err = rand.Read(secretBytes); err != nil {
		return "", "", fmt.Errorf("generando secreto: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)

	rawKey = fmt.Sprintf("%s_%s_%s", apiKeyPrefix, id, secret)

	hashBytes, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", "", fmt.Errorf("hasheando key: %w", err)
	}

	return rawKey, string(hashBytes), nil
}

// ParseAPIKeyID extrae el id embebido en una raw key sin verificarla, para
// poder localizar el registro correspondiente en base de datos.
func ParseAPIKeyID(rawKey string) (string, error) {
	parts := strings.SplitN(rawKey, "_", 3)
	if len(parts) != 3 || parts[0] != apiKeyPrefix {
		return "", fmt.Errorf("formato de API key inválido")
	}
	return parts[1], nil
}

// VerifyAPIKey compara una raw key contra el hash bcrypt de su secreto.
func VerifyAPIKey(rawKey, hash string) bool {
	parts := strings.SplitN(rawKey, "_", 3)
	if len(parts) != 3 || parts[0] != apiKeyPrefix {
		return false
	}
	secret := parts[2]
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) == nil
}
