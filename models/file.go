package models

import "time"

// File representa un archivo almacenado dentro de un bucket, con su metadata
// persistida en PostgreSQL. El contenido binario vive en el filesystem local
// bajo storage.LocalStorage, en la ruta indicada por Path.
type File struct {
	ID           string         `json:"id" db:"id"`
	BucketID     string         `json:"bucket_id" db:"bucket_id"`
	Filename     string         `json:"filename" db:"filename"`
	OriginalName string         `json:"original_name" db:"original_name"`
	MimeType     string         `json:"mime_type" db:"mime_type"`
	Size         int64          `json:"size" db:"size"`
	Path         string         `json:"path" db:"path"`
	IsPublic     bool           `json:"is_public" db:"is_public"`
	CreatedAt    time.Time      `json:"created_at" db:"created_at"`
	Metadata     map[string]any `json:"metadata" db:"metadata_json"`
}

// FileListResponse es la respuesta paginada de GET /api/files/:bucket.
type FileListResponse struct {
	Files      []File `json:"files"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
	TotalCount int64  `json:"total_count"`
	TotalPages int    `json:"total_pages"`
}
