// Package db maneja la conexión a PostgreSQL y las migraciones de esquema.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect abre un pool de conexiones a PostgreSQL y verifica conectividad
// con un ping, reintentando durante startupTimeout (útil cuando Postgres
// todavía está arrancando en docker-compose).
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("creando pool de conexiones: %w", err)
	}

	const startupTimeout = 30 * time.Second
	deadline := time.Now().Add(startupTimeout)

	var pingErr error
	for time.Now().Before(deadline) {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		pingErr = pool.Ping(pingCtx)
		cancel()
		if pingErr == nil {
			return pool, nil
		}
		time.Sleep(1 * time.Second)
	}

	pool.Close()
	return nil, fmt.Errorf("no se pudo conectar a PostgreSQL tras %s: %w", startupTimeout, pingErr)
}
