package handlers

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/gabicho/gabicho-storage/db"
	"github.com/gabicho/gabicho-storage/middleware"
	"github.com/gabicho/gabicho-storage/models"
)

// DownloadFile maneja GET /api/:bucket/:filename. Sirve el archivo
// directamente si el bucket/archivo son públicos, o si la request trae una
// API key válida con acceso (dueño o permiso admin).
func (h *Handler) DownloadFile(c fiber.Ctx) error {
	bucketName := c.Params("bucket")
	filename := c.Params("filename")

	bucket, err := h.Repo.GetBucketByName(c.Context(), bucketName)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "bucket no encontrado")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "error buscando el bucket")
	}

	file, err := h.Repo.GetFileByBucketAndFilename(c.Context(), bucket.ID, filename)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "archivo no encontrado")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "error buscando el archivo")
	}

	if !file.IsPublic {
		key := middleware.CurrentAPIKey(c)
		if key == nil || (bucket.Owner != key.UserID && !key.HasPermission(models.PermissionAdmin)) {
			return fiber.NewError(fiber.StatusForbidden, "archivo privado: se requiere una API key con acceso")
		}
	}

	c.Set(fiber.HeaderContentType, file.MimeType)
	c.Set(fiber.HeaderContentDisposition, `inline; filename="`+file.OriginalName+`"`)
	return c.SendFile(h.Storage.FullPath(file.Path))
}

// GeneratePresignedURL maneja POST /api/presign/:bucket/:filename, emitiendo
// un link de descarga temporal (TTL configurable, default TOKEN_EXPIRY_MINUTES)
// que no requiere API key para consumirse.
func (h *Handler) GeneratePresignedURL(c fiber.Ctx) error {
	key := middleware.CurrentAPIKey(c)
	bucketName := c.Params("bucket")
	filename := c.Params("filename")

	bucket, err := h.Repo.GetBucketByName(c.Context(), bucketName)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "bucket no encontrado")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "error buscando el bucket")
	}

	if bucket.Owner != key.UserID && !key.HasPermission(models.PermissionAdmin) {
		return fiber.NewError(fiber.StatusForbidden, "no tenés acceso a este bucket")
	}

	if _, err := h.Repo.GetFileByBucketAndFilename(c.Context(), bucket.ID, filename); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "archivo no encontrado")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "error buscando el archivo")
	}

	ttl := time.Duration(h.Config.TokenExpiryMinutes) * time.Minute
	token := h.Signer.GenerateDownloadToken(bucket.Name, filename, ttl)

	return c.JSON(fiber.Map{
		"token":      token,
		"url":        "/api/download/" + token,
		"expires_in": int(ttl.Seconds()),
	})
}

// PresignedDownload maneja GET /api/download/:token: valida el token firmado
// y, si es válido y no expiró, sirve el archivo sin requerir API key.
func (h *Handler) PresignedDownload(c fiber.Ctx) error {
	token := c.Params("token")

	bucketName, filename, err := h.Signer.ValidateDownloadToken(token)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "token inválido o expirado")
	}

	bucket, err := h.Repo.GetBucketByName(c.Context(), bucketName)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "bucket no encontrado")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "error buscando el bucket")
	}

	file, err := h.Repo.GetFileByBucketAndFilename(c.Context(), bucket.ID, filename)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "archivo no encontrado")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "error buscando el archivo")
	}

	c.Set(fiber.HeaderContentType, file.MimeType)
	c.Set(fiber.HeaderContentDisposition, `inline; filename="`+file.OriginalName+`"`)
	return c.SendFile(h.Storage.FullPath(file.Path))
}
