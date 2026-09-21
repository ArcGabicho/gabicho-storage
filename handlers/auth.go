package handlers

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/gabicho/gabicho-storage/db"
	"github.com/gabicho/gabicho-storage/models"
	"github.com/gabicho/gabicho-storage/utils"
)

// maxUserIDLength acota user_id a un largo razonable: sólo lo setea quien ya
// tiene la master key (un operador de confianza), pero igual conviene fallar
// con un 400 claro en vez de dejar que un valor absurdamente largo llegue al
// límite de la columna VARCHAR(255) y vuelva como un 500 de Postgres.
const maxUserIDLength = 255

var validPermissions = map[string]bool{
	models.PermissionRead:  true,
	models.PermissionWrite: true,
	models.PermissionAdmin: true,
}

// CreateAPIKeyHandler maneja POST /api/auth/keys (protegido por
// MasterKeyAuth). Genera y devuelve una nueva API key EN TEXTO PLANO: es la
// única vez que se muestra, ya que sólo se persiste su hash bcrypt.
func (h *Handler) CreateAPIKeyHandler(c fiber.Ctx) error {
	var req models.CreateAPIKeyRequest
	if err := c.Bind().Body(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "body inválido: "+err.Error())
	}

	req.UserID = strings.TrimSpace(req.UserID)
	if req.UserID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "user_id es obligatorio")
	}
	if len(req.UserID) > maxUserIDLength {
		return fiber.NewError(fiber.StatusBadRequest, "user_id demasiado largo")
	}

	if req.Permissions == "" {
		req.Permissions = strings.Join([]string{models.PermissionRead, models.PermissionWrite}, ",")
	}
	for _, p := range strings.Split(req.Permissions, ",") {
		if !validPermissions[strings.TrimSpace(p)] {
			return fiber.NewError(fiber.StatusBadRequest, "permiso inválido: "+p+" (válidos: read, write, admin)")
		}
	}

	id := uuid.NewString()
	rawKey, hash, err := utils.GenerateAPIKey(id)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "no se pudo generar la API key")
	}

	key, err := h.Repo.CreateAPIKey(c.Context(), id, req.UserID, hash, req.Permissions)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "no se pudo persistir la API key")
	}

	slog.Info("API key creada", "id", key.ID, "user_id", key.UserID, "permissions", key.Permissions, "ip", c.IP())

	return c.Status(fiber.StatusCreated).JSON(models.CreateAPIKeyResponse{
		ID:          key.ID,
		UserID:      key.UserID,
		Key:         rawKey,
		Permissions: key.Permissions,
		CreatedAt:   key.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// ListAPIKeysHandler maneja GET /api/auth/keys (protegido por MasterKeyAuth).
// Acepta ?user_id= opcional para filtrar. Nunca devuelve el hash ni la key
// en texto plano.
func (h *Handler) ListAPIKeysHandler(c fiber.Ctx) error {
	userID := c.Query("user_id")

	keys, err := h.Repo.ListAPIKeys(c.Context(), userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "no se pudieron listar las API keys")
	}

	return c.JSON(fiber.Map{"api_keys": keys})
}

// RevokeAPIKeyHandler maneja DELETE /api/auth/keys/:key_id (protegido por
// MasterKeyAuth).
func (h *Handler) RevokeAPIKeyHandler(c fiber.Ctx) error {
	id := c.Params("key_id")

	if err := h.Repo.DeleteAPIKey(c.Context(), id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "API key no encontrada")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "no se pudo revocar la API key")
	}

	slog.Info("API key revocada", "id", id, "ip", c.IP())

	return c.SendStatus(fiber.StatusNoContent)
}
