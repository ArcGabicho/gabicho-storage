// Tests de integración de la capa de acceso a datos: corren contra una
// instancia real de PostgreSQL (no un mock), porque lo que se quiere validar
// son las queries SQL en sí (constraints, cascadas, paginación) y no tiene
// sentido stubearlas.
//
// Requieren la variable de entorno TEST_DATABASE_URL. Si no está seteada,
// se saltean con t.Skip (ver README.md, sección "Tests").
package db

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gabicho/gabicho-storage/models"
)

// setupTestRepo conecta a TEST_DATABASE_URL, aplica el esquema y devuelve un
// Repository limpio para el test (cada test usa nombres/ids únicos, así que
// no hace falta un TRUNCATE entre tests).
func setupTestRepo(t *testing.T) *Repository {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL no seteada: saltando test de integración con PostgreSQL")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("no se pudo conectar a TEST_DATABASE_URL: %v", err)
	}

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	return NewRepository(pool)
}

func uniqueName(prefix string) string {
	return prefix + "-" + uuid.NewString()[:8]
}

func TestRepository_BucketLifecycle(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	name := uniqueName("bucket")

	created, err := repo.CreateBucket(ctx, name, "owner-1", false)
	if err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	if created.Name != name || created.Owner != "owner-1" || created.IsPublic {
		t.Fatalf("unexpected bucket returned: %+v", created)
	}

	if _, err := repo.CreateBucket(ctx, name, "owner-1", false); !errors.Is(err, ErrConflict) {
		t.Fatalf("CreateBucket duplicado: err = %v, want ErrConflict", err)
	}

	byName, err := repo.GetBucketByName(ctx, name)
	if err != nil {
		t.Fatalf("GetBucketByName: %v", err)
	}
	if byName.ID != created.ID {
		t.Errorf("GetBucketByName devolvió un id distinto: %s != %s", byName.ID, created.ID)
	}

	byID, err := repo.GetBucketByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetBucketByID: %v", err)
	}
	if byID.Name != name {
		t.Errorf("GetBucketByID name = %q, want %q", byID.Name, name)
	}

	list, err := repo.ListBucketsByOwner(ctx, "owner-1")
	if err != nil {
		t.Fatalf("ListBucketsByOwner: %v", err)
	}
	found := false
	for _, b := range list {
		if b.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Error("ListBucketsByOwner no incluyó el bucket recién creado")
	}

	if err := repo.DeleteBucket(ctx, created.ID); err != nil {
		t.Fatalf("DeleteBucket: %v", err)
	}

	if _, err := repo.GetBucketByID(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetBucketByID tras delete: err = %v, want ErrNotFound", err)
	}

	if err := repo.DeleteBucket(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteBucket duplicado: err = %v, want ErrNotFound", err)
	}
}

func TestRepository_FileLifecycleAndCascadeDelete(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	bucket, err := repo.CreateBucket(ctx, uniqueName("bucket"), "owner-1", false)
	if err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}

	file, err := repo.CreateFile(ctx, &models.File{
		BucketID:     bucket.ID,
		Filename:     "abc123.png",
		OriginalName: "photo.png",
		MimeType:     "image/png",
		Size:         1024,
		Path:         bucket.Name + "/abc123.png",
		IsPublic:     true,
		Metadata:     map[string]any{"alt": "una foto"},
	})
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	if file.Metadata["alt"] != "una foto" {
		t.Errorf("metadata no persistida correctamente: %+v", file.Metadata)
	}

	// Mismo bucket + filename debe violar el UNIQUE constraint.
	_, err = repo.CreateFile(ctx, &models.File{
		BucketID: bucket.ID, Filename: "abc123.png", OriginalName: "otra.png",
		MimeType: "image/png", Size: 1, Path: "x", Metadata: map[string]any{},
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("CreateFile duplicado: err = %v, want ErrConflict", err)
	}

	got, err := repo.GetFileByBucketAndFilename(ctx, bucket.ID, "abc123.png")
	if err != nil {
		t.Fatalf("GetFileByBucketAndFilename: %v", err)
	}
	if got.ID != file.ID {
		t.Errorf("GetFileByBucketAndFilename devolvió otro archivo")
	}

	// DeleteBucket debe borrar en cascada el archivo (FK ON DELETE CASCADE).
	if err := repo.DeleteBucket(ctx, bucket.ID); err != nil {
		t.Fatalf("DeleteBucket: %v", err)
	}
	if _, err := repo.GetFileByBucketAndFilename(ctx, bucket.ID, "abc123.png"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("el archivo debería haberse eliminado en cascada, err = %v", err)
	}
}

func TestRepository_ListFilesByBucket_Pagination(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	bucket, err := repo.CreateBucket(ctx, uniqueName("bucket"), "owner-1", false)
	if err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}

	const total = 5
	for i := 0; i < total; i++ {
		_, err := repo.CreateFile(ctx, &models.File{
			BucketID: bucket.ID, Filename: uniqueName("file") + ".png", OriginalName: "f.png",
			MimeType: "image/png", Size: 1, Path: "x", Metadata: map[string]any{},
		})
		if err != nil {
			t.Fatalf("CreateFile[%d]: %v", i, err)
		}
	}

	page1, count, err := repo.ListFilesByBucket(ctx, bucket.ID, 1, 2)
	if err != nil {
		t.Fatalf("ListFilesByBucket page 1: %v", err)
	}
	if count != total {
		t.Errorf("total count = %d, want %d", count, total)
	}
	if len(page1) != 2 {
		t.Errorf("len(page1) = %d, want 2", len(page1))
	}

	page3, _, err := repo.ListFilesByBucket(ctx, bucket.ID, 3, 2)
	if err != nil {
		t.Fatalf("ListFilesByBucket page 3: %v", err)
	}
	if len(page3) != 1 {
		t.Errorf("len(page3) = %d, want 1 (última página parcial: 5 items, pageSize 2)", len(page3))
	}
}

func TestRepository_APIKeyLifecycle(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	id := uuid.NewString()
	created, err := repo.CreateAPIKey(ctx, id, "user-1", "hashed-value", "read,write")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if created.LastUsed != nil {
		t.Error("una API key recién creada no debería tener last_used")
	}

	got, err := repo.GetAPIKeyByID(ctx, id)
	if err != nil {
		t.Fatalf("GetAPIKeyByID: %v", err)
	}
	if got.KeyHash != "hashed-value" {
		t.Errorf("KeyHash = %q, want %q", got.KeyHash, "hashed-value")
	}

	if err := repo.TouchLastUsed(ctx, id); err != nil {
		t.Fatalf("TouchLastUsed: %v", err)
	}
	got, err = repo.GetAPIKeyByID(ctx, id)
	if err != nil {
		t.Fatalf("GetAPIKeyByID tras touch: %v", err)
	}
	if got.LastUsed == nil {
		t.Error("last_used debería estar seteado tras TouchLastUsed")
	}

	list, err := repo.ListAPIKeys(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	found := false
	for _, k := range list {
		if k.ID == id {
			found = true
		}
	}
	if !found {
		t.Error("ListAPIKeys(user-1) no incluyó la key recién creada")
	}

	if err := repo.DeleteAPIKey(ctx, id); err != nil {
		t.Fatalf("DeleteAPIKey: %v", err)
	}
	if _, err := repo.GetAPIKeyByID(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetAPIKeyByID tras revocar: err = %v, want ErrNotFound", err)
	}
}

func TestRepository_GetBucketByName_NotFound(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	if _, err := repo.GetBucketByName(ctx, "nunca-existio"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestRepository_MalformedID_ReturnsNotFound es una regresión: un id que no
// es un UUID válido (típicamente un path param o una API key manipulados por
// un cliente) antes rompía la query contra la columna UUID de Postgres con
// un error genérico -- que las capas de arriba mapeaban a 500 en vez de
// 404/401, filtrando además el mensaje de error crudo de Postgres. Debe
// comportarse exactamente igual que un id válido pero inexistente.
func TestRepository_MalformedID_ReturnsNotFound(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	malformed := []string{"not-a-uuid", "", "'; DROP TABLE buckets; --", "1"}

	for _, id := range malformed {
		if _, err := repo.GetBucketByID(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetBucketByID(%q): err = %v, want ErrNotFound", id, err)
		}
		if err := repo.DeleteBucket(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("DeleteBucket(%q): err = %v, want ErrNotFound", id, err)
		}
		if _, err := repo.GetAPIKeyByID(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetAPIKeyByID(%q): err = %v, want ErrNotFound", id, err)
		}
		if err := repo.DeleteAPIKey(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("DeleteAPIKey(%q): err = %v, want ErrNotFound", id, err)
		}
	}
}
