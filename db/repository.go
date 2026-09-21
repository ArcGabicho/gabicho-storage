package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gabicho/gabicho-storage/models"
)

// ErrNotFound se devuelve cuando una consulta no encuentra el registro
// solicitado, para que las capas superiores puedan mapearlo a un 404 HTTP.
var ErrNotFound = errors.New("registro no encontrado")

// ErrConflict se devuelve ante violaciones de restricciones únicas (nombre de
// bucket duplicado, archivo duplicado dentro de un bucket, etc).
var ErrConflict = errors.New("el recurso ya existe")

// isValidUUID valida el formato antes de mandar un id a una columna UUID.
// Sin este chequeo, un id malformado (típicamente llegado directo de un path
// param o de una API key parseada, ambos con input de cliente) hace fallar
// la query con un error de encoding/tipo de Postgres que no es
// pgx.ErrNoRows, y las capas de arriba lo tratarían como 500 en vez de
// 404/401 -- devolviendo detalles internos de la base en el mensaje de error
// y generando ruido de logs de nivel ERROR por lo que en realidad es un
// simple input inválido de un llamante no autenticado.
func isValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// Repository centraliza el acceso a datos del servicio sobre PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository crea un Repository sobre el pool de conexiones dado.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ---------------------------------------------------------------------------
// Buckets
// ---------------------------------------------------------------------------

// CreateBucket inserta un nuevo bucket.
func (r *Repository) CreateBucket(ctx context.Context, name, owner string, isPublic bool) (*models.Bucket, error) {
	const q = `
		INSERT INTO buckets (name, owner, is_public)
		VALUES ($1, $2, $3)
		RETURNING id, name, owner, is_public, created_at`

	var b models.Bucket
	err := r.pool.QueryRow(ctx, q, name, owner, isPublic).Scan(&b.ID, &b.Name, &b.Owner, &b.IsPublic, &b.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("creando bucket: %w", err)
	}
	return &b, nil
}

// GetBucketByName busca un bucket por su nombre único.
func (r *Repository) GetBucketByName(ctx context.Context, name string) (*models.Bucket, error) {
	const q = `SELECT id, name, owner, is_public, created_at FROM buckets WHERE name = $1`

	var b models.Bucket
	err := r.pool.QueryRow(ctx, q, name).Scan(&b.ID, &b.Name, &b.Owner, &b.IsPublic, &b.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("buscando bucket %q: %w", name, err)
	}
	return &b, nil
}

// GetBucketByID busca un bucket por su id.
func (r *Repository) GetBucketByID(ctx context.Context, id string) (*models.Bucket, error) {
	if !isValidUUID(id) {
		return nil, ErrNotFound
	}

	const q = `SELECT id, name, owner, is_public, created_at FROM buckets WHERE id = $1`

	var b models.Bucket
	err := r.pool.QueryRow(ctx, q, id).Scan(&b.ID, &b.Name, &b.Owner, &b.IsPublic, &b.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("buscando bucket %q: %w", id, err)
	}
	return &b, nil
}

// ListBucketsByOwner lista los buckets propiedad de un usuario.
func (r *Repository) ListBucketsByOwner(ctx context.Context, owner string) ([]models.Bucket, error) {
	const q = `SELECT id, name, owner, is_public, created_at FROM buckets WHERE owner = $1 ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, q, owner)
	if err != nil {
		return nil, fmt.Errorf("listando buckets: %w", err)
	}
	defer rows.Close()

	buckets := make([]models.Bucket, 0)
	for rows.Next() {
		var b models.Bucket
		if err := rows.Scan(&b.ID, &b.Name, &b.Owner, &b.IsPublic, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("leyendo bucket: %w", err)
		}
		buckets = append(buckets, b)
	}
	return buckets, rows.Err()
}

// DeleteBucket elimina un bucket (y en cascada sus archivos, vía FK).
func (r *Repository) DeleteBucket(ctx context.Context, id string) error {
	if !isValidUUID(id) {
		return ErrNotFound
	}

	tag, err := r.pool.Exec(ctx, `DELETE FROM buckets WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("eliminando bucket: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Files
// ---------------------------------------------------------------------------

// CreateFile inserta la metadata de un archivo recién subido.
func (r *Repository) CreateFile(ctx context.Context, f *models.File) (*models.File, error) {
	metadataBytes, err := json.Marshal(f.Metadata)
	if err != nil {
		return nil, fmt.Errorf("serializando metadata: %w", err)
	}

	const q = `
		INSERT INTO files (bucket_id, filename, original_name, mime_type, size, path, is_public, metadata_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, bucket_id, filename, original_name, mime_type, size, path, is_public, created_at, metadata_json`

	var out models.File
	var metaRaw []byte
	err = r.pool.QueryRow(ctx, q, f.BucketID, f.Filename, f.OriginalName, f.MimeType, f.Size, f.Path, f.IsPublic, metadataBytes).
		Scan(&out.ID, &out.BucketID, &out.Filename, &out.OriginalName, &out.MimeType, &out.Size, &out.Path, &out.IsPublic, &out.CreatedAt, &metaRaw)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("creando archivo: %w", err)
	}
	if err := json.Unmarshal(metaRaw, &out.Metadata); err != nil {
		return nil, fmt.Errorf("deserializando metadata: %w", err)
	}
	return &out, nil
}

// GetFileByBucketAndFilename busca un archivo por bucket + nombre físico.
func (r *Repository) GetFileByBucketAndFilename(ctx context.Context, bucketID, filename string) (*models.File, error) {
	const q = `
		SELECT id, bucket_id, filename, original_name, mime_type, size, path, is_public, created_at, metadata_json
		FROM files WHERE bucket_id = $1 AND filename = $2`

	var out models.File
	var metaRaw []byte
	err := r.pool.QueryRow(ctx, q, bucketID, filename).
		Scan(&out.ID, &out.BucketID, &out.Filename, &out.OriginalName, &out.MimeType, &out.Size, &out.Path, &out.IsPublic, &out.CreatedAt, &metaRaw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("buscando archivo: %w", err)
	}
	if err := json.Unmarshal(metaRaw, &out.Metadata); err != nil {
		return nil, fmt.Errorf("deserializando metadata: %w", err)
	}
	return &out, nil
}

// ListFilesByBucket devuelve una página de archivos de un bucket junto con el
// conteo total, para armar la paginación.
func (r *Repository) ListFilesByBucket(ctx context.Context, bucketID string, page, pageSize int) ([]models.File, int64, error) {
	offset := (page - 1) * pageSize

	const countQ = `SELECT COUNT(*) FROM files WHERE bucket_id = $1`
	var total int64
	if err := r.pool.QueryRow(ctx, countQ, bucketID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("contando archivos: %w", err)
	}

	const q = `
		SELECT id, bucket_id, filename, original_name, mime_type, size, path, is_public, created_at, metadata_json
		FROM files WHERE bucket_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, q, bucketID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listando archivos: %w", err)
	}
	defer rows.Close()

	files := make([]models.File, 0)
	for rows.Next() {
		var f models.File
		var metaRaw []byte
		if err := rows.Scan(&f.ID, &f.BucketID, &f.Filename, &f.OriginalName, &f.MimeType, &f.Size, &f.Path, &f.IsPublic, &f.CreatedAt, &metaRaw); err != nil {
			return nil, 0, fmt.Errorf("leyendo archivo: %w", err)
		}
		if err := json.Unmarshal(metaRaw, &f.Metadata); err != nil {
			return nil, 0, fmt.Errorf("deserializando metadata: %w", err)
		}
		files = append(files, f)
	}
	return files, total, rows.Err()
}

// DeleteFile elimina la metadata de un archivo por su id.
func (r *Repository) DeleteFile(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM files WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("eliminando archivo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// API Keys
// ---------------------------------------------------------------------------

// CreateAPIKey inserta una nueva API key ya hasheada.
func (r *Repository) CreateAPIKey(ctx context.Context, id, userID, keyHash, permissions string) (*models.APIKey, error) {
	const q = `
		INSERT INTO api_keys (id, user_id, key_hash, permissions)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, permissions, created_at, last_used`

	var k models.APIKey
	err := r.pool.QueryRow(ctx, q, id, userID, keyHash, permissions).
		Scan(&k.ID, &k.UserID, &k.Permissions, &k.CreatedAt, &k.LastUsed)
	if err != nil {
		return nil, fmt.Errorf("creando api key: %w", err)
	}
	k.KeyHash = keyHash
	return &k, nil
}

// GetAPIKeyByID busca una API key por id, incluyendo su hash (para
// verificación en el middleware de autenticación).
func (r *Repository) GetAPIKeyByID(ctx context.Context, id string) (*models.APIKey, error) {
	if !isValidUUID(id) {
		return nil, ErrNotFound
	}

	const q = `SELECT id, user_id, key_hash, permissions, created_at, last_used FROM api_keys WHERE id = $1`

	var k models.APIKey
	err := r.pool.QueryRow(ctx, q, id).Scan(&k.ID, &k.UserID, &k.KeyHash, &k.Permissions, &k.CreatedAt, &k.LastUsed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("buscando api key: %w", err)
	}
	return &k, nil
}

// ListAPIKeys lista todas las API keys, opcionalmente filtradas por usuario.
func (r *Repository) ListAPIKeys(ctx context.Context, userID string) ([]models.APIKey, error) {
	var rows pgx.Rows
	var err error
	if userID == "" {
		rows, err = r.pool.Query(ctx, `SELECT id, user_id, permissions, created_at, last_used FROM api_keys ORDER BY created_at DESC`)
	} else {
		rows, err = r.pool.Query(ctx, `SELECT id, user_id, permissions, created_at, last_used FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	}
	if err != nil {
		return nil, fmt.Errorf("listando api keys: %w", err)
	}
	defer rows.Close()

	keys := make([]models.APIKey, 0)
	for rows.Next() {
		var k models.APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.Permissions, &k.CreatedAt, &k.LastUsed); err != nil {
			return nil, fmt.Errorf("leyendo api key: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// DeleteAPIKey revoca (elimina) una API key por id.
func (r *Repository) DeleteAPIKey(ctx context.Context, id string) error {
	if !isValidUUID(id) {
		return ErrNotFound
	}

	tag, err := r.pool.Exec(ctx, `DELETE FROM api_keys WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("eliminando api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchLastUsed actualiza el timestamp de último uso de una API key.
func (r *Repository) TouchLastUsed(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE api_keys SET last_used = NOW() WHERE id = $1`, id)
	return err
}

// ---------------------------------------------------------------------------

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}
