package models

import "time"

// Bucket representa un contenedor lógico de archivos, análogo a un bucket de
// S3/Firebase Storage. Cada bucket pertenece a un "owner" (el user_id
// asociado a la API key que lo creó).
type Bucket struct {
	ID        string    `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Owner     string    `json:"owner" db:"owner"`
	IsPublic  bool      `json:"is_public" db:"is_public"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// CreateBucketRequest es el payload de POST /api/buckets.
type CreateBucketRequest struct {
	Name     string `json:"name"`
	IsPublic bool   `json:"is_public"`
}
