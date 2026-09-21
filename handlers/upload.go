package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime/multipart"
	"strconv"

	"github.com/gofiber/fiber/v3"

	"github.com/gabicho/gabicho-storage/db"
	"github.com/gabicho/gabicho-storage/middleware"
	"github.com/gabicho/gabicho-storage/models"
	"github.com/gabicho/gabicho-storage/utils"
)

// sniffLen es cuántos bytes iniciales del archivo se leen para detectar HTML
// disfrazado de otro tipo (mismo límite que usa http.DetectContentType).
const sniffLen = 512

// readSniffBytes lee hasta sniffLen bytes iniciales de un multipart file sin
// consumir el reader que después usará SaveUploadedFile: FileHeader.Open()
// devuelve un handle nuevo posicionado al inicio en cada llamada.
func readSniffBytes(fh *multipart.FileHeader) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, sniffLen)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	return buf[:n], nil
}

// UploadFile maneja POST /api/upload/:bucket (multipart/form-data, campo
// "file"). Campos opcionales del form: "public" ("true"/"false") y
// "metadata" (JSON arbitrario a asociar al archivo).
func (h *Handler) UploadFile(c fiber.Ctx) error {
	key := middleware.CurrentAPIKey(c)
	bucketName := c.Params("bucket")

	bucket, err := h.Repo.GetBucketByName(c.Context(), bucketName)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "bucket no encontrado")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "error buscando el bucket")
	}

	if bucket.Owner != key.UserID && !key.HasPermission(models.PermissionAdmin) {
		return fiber.NewError(fiber.StatusForbidden, "no tenés permiso de escritura sobre este bucket")
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "falta el campo 'file' en el multipart/form-data")
	}

	if err := utils.ValidateFileSize(fh.Size, h.Config.MaxFileSize); err != nil {
		return fiber.NewError(fiber.StatusRequestEntityTooLarge, err.Error())
	}

	mimeType := fh.Header.Get("Content-Type")
	if !utils.IsAllowedMimeType(mimeType) {
		return fiber.NewError(fiber.StatusUnsupportedMediaType, "tipo de archivo no permitido: "+mimeType)
	}

	sniff, err := readSniffBytes(fh)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "no se pudo leer el archivo para validarlo")
	}
	if utils.LooksLikeHTML(sniff) {
		return fiber.NewError(fiber.StatusUnsupportedMediaType, "el contenido del archivo no coincide con el tipo declarado")
	}

	originalName := utils.SanitizeFilename(fh.Filename)

	isPublic := bucket.IsPublic
	if v := c.FormValue("public"); v != "" {
		isPublic, err = strconv.ParseBool(v)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "el campo 'public' debe ser true/false")
		}
	}

	metadata := map[string]any{}
	if raw := c.FormValue("metadata"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "el campo 'metadata' debe ser JSON válido")
		}
	}

	storedName, relPath, err := h.Storage.SaveUploadedFile(bucket.Name, fh, utils.ExtensionForMimeType(mimeType))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "no se pudo guardar el archivo: "+err.Error())
	}

	file, err := h.Repo.CreateFile(c.Context(), &models.File{
		BucketID:     bucket.ID,
		Filename:     storedName,
		OriginalName: originalName,
		MimeType:     mimeType,
		Size:         fh.Size,
		Path:         relPath,
		IsPublic:     isPublic,
		Metadata:     metadata,
	})
	if err != nil {
		_ = h.Storage.DeleteFile(relPath)
		return fiber.NewError(fiber.StatusInternalServerError, "no se pudo guardar la metadata del archivo")
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"file":         file,
		"download_url": "/api/" + bucket.Name + "/" + file.Filename,
	})
}

// ListFiles maneja GET /api/files/:bucket con paginación vía ?page= y
// ?page_size= (defaults: 1 / 20, máximo page_size: 100).
func (h *Handler) ListFiles(c fiber.Ctx) error {
	key := middleware.CurrentAPIKey(c)
	bucketName := c.Params("bucket")

	bucket, err := h.Repo.GetBucketByName(c.Context(), bucketName)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "bucket no encontrado")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "error buscando el bucket")
	}

	if !bucket.IsPublic && bucket.Owner != key.UserID && !key.HasPermission(models.PermissionAdmin) {
		return fiber.NewError(fiber.StatusForbidden, "no tenés acceso a este bucket")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.Query("page_size", "20"))
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	files, total, err := h.Repo.ListFilesByBucket(c.Context(), bucket.ID, page, pageSize)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "no se pudieron listar los archivos")
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	return c.JSON(models.FileListResponse{
		Files:      files,
		Page:       page,
		PageSize:   pageSize,
		TotalCount: total,
		TotalPages: totalPages,
	})
}

// DeleteFile maneja DELETE /api/:bucket/:filename.
func (h *Handler) DeleteFile(c fiber.Ctx) error {
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
		return fiber.NewError(fiber.StatusForbidden, "no tenés permiso de escritura sobre este bucket")
	}

	file, err := h.Repo.GetFileByBucketAndFilename(c.Context(), bucket.ID, filename)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "archivo no encontrado")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "error buscando el archivo")
	}

	if err := h.Repo.DeleteFile(c.Context(), file.ID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "no se pudo eliminar la metadata del archivo")
	}

	if err := h.Storage.DeleteFile(file.Path); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "el archivo se eliminó de la base de datos pero falló la limpieza física")
	}

	return c.SendStatus(fiber.StatusNoContent)
}
