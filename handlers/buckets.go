package handlers

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"

	"github.com/gabicho/gabicho-storage/db"
	"github.com/gabicho/gabicho-storage/middleware"
	"github.com/gabicho/gabicho-storage/models"
	"github.com/gabicho/gabicho-storage/utils"
)

// CreateBucket maneja POST /api/buckets.
func (h *Handler) CreateBucket(c fiber.Ctx) error {
	key := middleware.CurrentAPIKey(c)

	var req models.CreateBucketRequest
	if err := c.Bind().Body(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "body inválido: "+err.Error())
	}

	if !utils.IsValidBucketName(req.Name) {
		return fiber.NewError(fiber.StatusBadRequest, "nombre de bucket inválido: debe tener 3-63 caracteres, minúsculas, dígitos y guiones")
	}

	if err := h.Storage.EnsureBucketDir(req.Name); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "no se pudo crear el directorio del bucket")
	}

	bucket, err := h.Repo.CreateBucket(c.Context(), req.Name, key.UserID, req.IsPublic)
	if err != nil {
		if errors.Is(err, db.ErrConflict) {
			return fiber.NewError(fiber.StatusConflict, "ya existe un bucket con ese nombre")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "no se pudo crear el bucket")
	}

	return c.Status(fiber.StatusCreated).JSON(bucket)
}

// ListBuckets maneja GET /api/buckets, devolviendo los buckets del usuario
// autenticado.
func (h *Handler) ListBuckets(c fiber.Ctx) error {
	key := middleware.CurrentAPIKey(c)

	buckets, err := h.Repo.ListBucketsByOwner(c.Context(), key.UserID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "no se pudieron listar los buckets")
	}

	return c.JSON(fiber.Map{"buckets": buckets})
}

// DeleteBucket maneja DELETE /api/buckets/:id. Sólo el dueño del bucket (o
// una key con permiso admin) puede eliminarlo.
func (h *Handler) DeleteBucket(c fiber.Ctx) error {
	key := middleware.CurrentAPIKey(c)
	id := c.Params("id")

	bucket, err := h.Repo.GetBucketByID(c.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "bucket no encontrado")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "error buscando el bucket")
	}

	if bucket.Owner != key.UserID && !key.HasPermission(models.PermissionAdmin) {
		return fiber.NewError(fiber.StatusForbidden, "no sos el dueño de este bucket")
	}

	if err := h.Repo.DeleteBucket(c.Context(), id); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "no se pudo eliminar el bucket")
	}

	if err := h.Storage.RemoveBucketDir(bucket.Name); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "el bucket se eliminó de la base de datos pero falló la limpieza de archivos")
	}

	slog.Info("bucket eliminado", "id", bucket.ID, "name", bucket.Name, "owner", bucket.Owner, "deleted_by", key.UserID)

	return c.SendStatus(fiber.StatusNoContent)
}
