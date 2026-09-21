// Package middleware contiene los middlewares HTTP del servicio: autenticación
// (API keys y master key de administración) y logging de requests.
package middleware

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"

	"github.com/gabicho/gabicho-storage/db"
	"github.com/gabicho/gabicho-storage/models"
	"github.com/gabicho/gabicho-storage/utils"
)

// LocalsAPIKey es la clave usada para guardar la *models.APIKey autenticada
// en fiber.Ctx.Locals.
const LocalsAPIKey = "api_key"

// MasterKeyAuth protege endpoints de administración (gestión de API keys)
// exigiendo el header X-Master-Key, comparado contra ADMIN_MASTER_KEY.
func MasterKeyAuth(masterKey string) fiber.Handler {
	masterKeyBytes := []byte(masterKey)

	return func(c fiber.Ctx) error {
		provided := c.Get("X-Master-Key")

		// subtle.ConstantTimeCompare exige igual longitud para no filtrar
		// nada por tiempo; si difieren en longitud ya sabemos que no matchea,
		// pero igual corremos la comparación de tiempo constante contra la
		// propia master key para no crear una rama de tiempo distinguible
		// entre "longitud distinta" y "longitud igual pero contenido distinto".
		match := subtle.ConstantTimeCompare([]byte(provided), masterKeyBytes) == 1
		if provided == "" || !match {
			slog.Warn("intento de acceso con master key inválida", "ip", c.IP(), "path", c.Path())
			return fiber.NewError(fiber.StatusUnauthorized, "master key inválida o faltante")
		}
		return c.Next()
	}
}

// APIKeyAuth exige una API key válida (header X-API-Key, o
// "Authorization: Bearer <key>") y la deja disponible en Locals para
// handlers y middlewares posteriores (p.ej. RequirePermission).
func APIKeyAuth(repo *db.Repository) fiber.Handler {
	return func(c fiber.Ctx) error {
		rawKey := extractAPIKey(c)
		if rawKey == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "falta la API key (header X-API-Key)")
		}

		keyID, err := utils.ParseAPIKeyID(rawKey)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "API key inválida")
		}

		key, err := repo.GetAPIKeyByID(c.Context(), keyID)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return fiber.NewError(fiber.StatusUnauthorized, "API key inválida")
			}
			return fiber.NewError(fiber.StatusInternalServerError, "error validando API key")
		}

		if !utils.VerifyAPIKey(rawKey, key.KeyHash) {
			return fiber.NewError(fiber.StatusUnauthorized, "API key inválida")
		}

		go func(id string) {
			_ = repo.TouchLastUsed(context.Background(), id)
		}(key.ID)

		c.Locals(LocalsAPIKey, key)
		return c.Next()
	}
}

// OptionalAPIKeyAuth intenta autenticar la API key si viene presente en la
// request, pero permite continuar sin ella (para endpoints que sirven tanto
// contenido público como privado, decidiendo el acceso en el propio handler).
// Una API key presente pero inválida sí es rechazada explícitamente.
func OptionalAPIKeyAuth(repo *db.Repository) fiber.Handler {
	return func(c fiber.Ctx) error {
		rawKey := extractAPIKey(c)
		if rawKey == "" {
			return c.Next()
		}

		keyID, err := utils.ParseAPIKeyID(rawKey)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "API key inválida")
		}

		key, err := repo.GetAPIKeyByID(c.Context(), keyID)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return fiber.NewError(fiber.StatusUnauthorized, "API key inválida")
			}
			return fiber.NewError(fiber.StatusInternalServerError, "error validando API key")
		}

		if !utils.VerifyAPIKey(rawKey, key.KeyHash) {
			return fiber.NewError(fiber.StatusUnauthorized, "API key inválida")
		}

		go func(id string) {
			_ = repo.TouchLastUsed(context.Background(), id)
		}(key.ID)

		c.Locals(LocalsAPIKey, key)
		return c.Next()
	}
}

// RequirePermission exige que la API key autenticada (ya validada por
// APIKeyAuth) tenga el permiso indicado.
func RequirePermission(permission string) fiber.Handler {
	return func(c fiber.Ctx) error {
		key, ok := c.Locals(LocalsAPIKey).(*models.APIKey)
		if !ok || key == nil {
			return fiber.NewError(fiber.StatusUnauthorized, "no autenticado")
		}
		if !key.HasPermission(permission) {
			return fiber.NewError(fiber.StatusForbidden, "permiso insuficiente: se requiere '"+permission+"'")
		}
		return c.Next()
	}
}

// CurrentAPIKey obtiene la API key autenticada del contexto de la request.
func CurrentAPIKey(c fiber.Ctx) *models.APIKey {
	key, _ := c.Locals(LocalsAPIKey).(*models.APIKey)
	return key
}

// RateLimitKey identifica al llamante para el rate limiter: por user_id si
// ya hay una API key autenticada en el contexto (más preciso, no se ve
// afectado por IPs compartidas como las de un frontend en Vercel haciendo de
// proxy), o por IP si todavía no corrió APIKeyAuth en la cadena de
// middlewares (rutas públicas como la descarga presignada).
func RateLimitKey(c fiber.Ctx) string {
	if key := CurrentAPIKey(c); key != nil {
		return key.UserID
	}
	return c.IP()
}

func extractAPIKey(c fiber.Ctx) string {
	if v := c.Get("X-API-Key"); v != "" {
		return v
	}
	auth := c.Get("Authorization")
	const prefix = "Bearer "
	if len(auth) > len(prefix) && auth[:len(prefix)] == prefix {
		return auth[len(prefix):]
	}
	return ""
}
