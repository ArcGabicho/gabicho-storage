package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// schema define las tablas del servicio. Se ejecuta con IF NOT EXISTS en
// startup, lo que la hace idempotente y suficiente para un servicio de este
// tamaño sin necesitar una herramienta de migraciones dedicada.
const schema = `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS buckets (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(63) NOT NULL UNIQUE,
    owner      VARCHAR(255) NOT NULL,
    is_public  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_buckets_owner ON buckets (owner);

CREATE TABLE IF NOT EXISTS files (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bucket_id      UUID NOT NULL REFERENCES buckets (id) ON DELETE CASCADE,
    filename       VARCHAR(255) NOT NULL,
    original_name  VARCHAR(255) NOT NULL,
    mime_type      VARCHAR(127) NOT NULL,
    size           BIGINT NOT NULL,
    path           TEXT NOT NULL,
    is_public      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata_json  JSONB NOT NULL DEFAULT '{}'::JSONB,
    UNIQUE (bucket_id, filename)
);

CREATE INDEX IF NOT EXISTS idx_files_bucket_id ON files (bucket_id);
CREATE INDEX IF NOT EXISTS idx_files_bucket_created ON files (bucket_id, created_at DESC);

CREATE TABLE IF NOT EXISTS api_keys (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     VARCHAR(255) NOT NULL,
    key_hash    TEXT NOT NULL,
    permissions VARCHAR(255) NOT NULL DEFAULT 'read,write',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys (user_id);
`

// migrationLockID es un id arbitrario para el advisory lock de Postgres que
// serializa aplicaciones concurrentes del esquema.
const migrationLockID = 727384910

// Migrate crea el esquema de base de datos si no existe. Serializa la
// aplicación del esquema con un advisory lock de Postgres tomado sobre una
// única conexión: sin esto, dos procesos arrancando contra la misma base al
// mismo tiempo (típicamente varios tests corriendo en paralelo) pueden
// pisarse al ejecutar CREATE EXTENSION IF NOT EXISTS -- hay una condición de
// carrera conocida de Postgres ahí que puede devolver un duplicate key en el
// catálogo aunque el IF NOT EXISTS debería evitarlo.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("adquiriendo conexión para migrar: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("adquiriendo advisory lock de migración: %w", err)
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockID)

	if _, err := conn.Exec(ctx, schema); err != nil {
		return fmt.Errorf("aplicando esquema: %w", err)
	}
	return nil
}
