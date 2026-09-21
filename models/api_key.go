package models

import (
	"strings"
	"time"
)

// Permisos soportados por una API key. "admin" implica read+write y además
// permite operaciones de administración de buckets ajenos.
const (
	PermissionRead  = "read"
	PermissionWrite = "write"
	PermissionAdmin = "admin"
)

// APIKey representa una credencial emitida para un usuario. El secreto en
// texto plano nunca se persiste: sólo se guarda su hash bcrypt (KeyHash).
type APIKey struct {
	ID          string     `json:"id" db:"id"`
	UserID      string     `json:"user_id" db:"user_id"`
	KeyHash     string     `json:"-" db:"key_hash"`
	Permissions string     `json:"permissions" db:"permissions"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	LastUsed    *time.Time `json:"last_used,omitempty" db:"last_used"`
}

// HasPermission indica si la key tiene el permiso solicitado (admin implica
// todos los demás).
func (k *APIKey) HasPermission(perm string) bool {
	for _, p := range strings.Split(k.Permissions, ",") {
		p = strings.TrimSpace(p)
		if p == PermissionAdmin || p == perm {
			return true
		}
	}
	return false
}

// CreateAPIKeyRequest es el payload de POST /api/auth/keys.
type CreateAPIKeyRequest struct {
	UserID      string `json:"user_id"`
	Permissions string `json:"permissions"` // ej: "read,write"
}

// CreateAPIKeyResponse devuelve la key en texto plano UNA sola vez.
type CreateAPIKeyResponse struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	Key         string `json:"key"`
	Permissions string `json:"permissions"`
	CreatedAt   string `json:"created_at"`
}
