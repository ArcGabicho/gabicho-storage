// Tests de integración a nivel HTTP: levantan la app completa (fiber.App +
// middlewares + rutas reales) contra una PostgreSQL real y un directorio de
// storage temporal, y ejercitan los endpoints con requests reales via
// app.Test(), tal como los ejercité manualmente con curl durante el
// desarrollo.
//
// Requieren TEST_DATABASE_URL (ver README.md, sección "Tests"); si no está
// seteada, se saltean.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gabicho/gabicho-storage/config"
	"github.com/gabicho/gabicho-storage/db"
	"github.com/gabicho/gabicho-storage/handlers"
	"github.com/gabicho/gabicho-storage/storage"
	"github.com/gabicho/gabicho-storage/utils"
)

const testTimeout = 5 * time.Second

// testApp agrupa la app bajo test y los datos necesarios para armar
// requests (master key, config para firmar tokens, etc).
type testApp struct {
	app *fiber.App
	cfg *config.Config
}

// newTestApp levanta una app completa (misma que main(), vía newApp) contra
// TEST_DATABASE_URL y un storage en un directorio temporal. cfgOverride
// permite ajustar límites (rate limiting, tamaño máximo) para un test
// puntual; puede ser nil para usar los defaults de test.
func newTestApp(t *testing.T, cfgOverride func(*config.Config)) *testApp {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL no seteada: saltando test de integración HTTP")
	}

	ctx := t.Context()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("no se pudo conectar a TEST_DATABASE_URL: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	store, err := storage.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}

	cfg := &config.Config{
		APIPort:               "0",
		CORSOrigin:            "https://mi-app.vercel.app",
		Environment:           "test",
		MaxFileSize:           5 * 1024 * 1024, // 5MB para no subir archivos gigantes en tests
		TokenExpiryMinutes:    15,
		SigningSecret:         "test-signing-secret",
		MasterKey:             "test-master-key",
		APIRateLimitMax:       1000,
		APIRateLimitWindow:    60,
		UploadRateLimitMax:    1000,
		UploadRateLimitWindow: 60,
	}
	if cfgOverride != nil {
		cfgOverride(cfg)
	}

	repo := db.NewRepository(pool)
	h := handlers.New(repo, store, cfg)

	return &testApp{app: newApp(cfg, h, repo), cfg: cfg}
}

func (ta *testApp) do(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	resp, err := ta.app.Test(req, fiber.TestConfig{Timeout: testTimeout, FailOnTimeout: true})
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	return string(b)
}

func decodeJSON(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decoding JSON response: %v", err)
	}
}

// createTestAPIKey emite una API key vía el propio endpoint administrativo,
// como lo haría un operador real.
func (ta *testApp) createTestAPIKey(t *testing.T, userID, permissions string) string {
	t.Helper()

	body, _ := json.Marshal(map[string]string{"user_id": userID, "permissions": permissions})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/keys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Master-Key", ta.cfg.MasterKey)

	resp := ta.do(t, req)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("createTestAPIKey: status = %d, body = %s", resp.StatusCode, readBody(t, resp))
	}

	var out struct {
		Key string `json:"key"`
	}
	decodeJSON(t, resp, &out)
	return out.Key
}

func (ta *testApp) createTestBucket(t *testing.T, apiKey, name string, isPublic bool) {
	t.Helper()

	body, _ := json.Marshal(map[string]any{"name": name, "is_public": isPublic})
	req := httptest.NewRequest(http.MethodPost, "/api/buckets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	resp := ta.do(t, req)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("createTestBucket: status = %d, body = %s", resp.StatusCode, readBody(t, resp))
	}
}

func uniqueBucketName(prefix string) string {
	return prefix + "-" + uuid.NewString()[:8]
}

func multipartUpload(t *testing.T, filename, mimeType string, content []byte, extraFields map[string]string) (*bytes.Buffer, string) {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	h := make(map[string][]string)
	h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename)}
	if mimeType != "" {
		h["Content-Type"] = []string{mimeType}
	}
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("writing multipart content: %v", err)
	}

	for k, v := range extraFields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("WriteField(%s): %v", k, err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}

	return &buf, w.FormDataContentType()
}

// ---------------------------------------------------------------------------

func TestHealthCheck(t *testing.T) {
	ta := newTestApp(t, nil)

	resp := ta.do(t, httptest.NewRequest(http.MethodGet, "/health", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if body := readBody(t, resp); body != `{"status":"ok"}` {
		t.Errorf("body = %q", body)
	}
}

func TestAPIKeyAdmin_RequiresMasterKey(t *testing.T) {
	ta := newTestApp(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/keys", nil)
	resp := ta.do(t, req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status sin master key = %d, want 401", resp.StatusCode)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/auth/keys", nil)
	req.Header.Set("X-Master-Key", "wrong-key")
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status con master key incorrecta = %d, want 401", resp.StatusCode)
	}
}

func TestAPIKeyAdmin_CreateListRevoke(t *testing.T) {
	ta := newTestApp(t, nil)
	userID := uniqueBucketName("user")

	body, _ := json.Marshal(map[string]string{"user_id": userID, "permissions": "read,write"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/keys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Master-Key", ta.cfg.MasterKey)
	resp := ta.do(t, req)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: status = %d, body = %s", resp.StatusCode, readBody(t, resp))
	}

	var created struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	}
	decodeJSON(t, resp, &created)
	if created.Key == "" {
		t.Fatal("la respuesta no incluyó la key en texto plano")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/auth/keys?user_id="+userID, nil)
	req.Header.Set("X-Master-Key", ta.cfg.MasterKey)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: status = %d", resp.StatusCode)
	}
	var list struct {
		APIKeys []map[string]any `json:"api_keys"`
	}
	decodeJSON(t, resp, &list)
	if len(list.APIKeys) != 1 {
		t.Fatalf("len(api_keys) = %d, want 1", len(list.APIKeys))
	}
	if _, leaked := list.APIKeys[0]["key_hash"]; leaked {
		t.Error("el listado de API keys no debería exponer key_hash")
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/auth/keys/"+created.ID, nil)
	req.Header.Set("X-Master-Key", ta.cfg.MasterKey)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke: status = %d, want 204", resp.StatusCode)
	}

	// La key revocada ya no debería servir para autenticar.
	req = httptest.NewRequest(http.MethodGet, "/api/buckets", nil)
	req.Header.Set("X-API-Key", created.Key)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("uso de key revocada: status = %d, want 401", resp.StatusCode)
	}
}

func TestBucketCRUD_FullFlow(t *testing.T) {
	ta := newTestApp(t, nil)
	apiKey := ta.createTestAPIKey(t, uniqueBucketName("user"), "read,write")
	bucketName := uniqueBucketName("bucket")

	body, _ := json.Marshal(map[string]any{"name": bucketName, "is_public": false})
	req := httptest.NewRequest(http.MethodPost, "/api/buckets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	resp := ta.do(t, req)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: status = %d, body = %s", resp.StatusCode, readBody(t, resp))
	}
	var created struct {
		ID string `json:"id"`
	}
	decodeJSON(t, resp, &created)

	// Nombre duplicado -> 409.
	req = httptest.NewRequest(http.MethodPost, "/api/buckets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("create duplicado: status = %d, want 409", resp.StatusCode)
	}

	// Nombre inválido -> 400.
	body, _ = json.Marshal(map[string]any{"name": "AB", "is_public": false})
	req = httptest.NewRequest(http.MethodPost, "/api/buckets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("create con nombre inválido: status = %d, want 400", resp.StatusCode)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/buckets", nil)
	req.Header.Set("X-API-Key", apiKey)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: status = %d", resp.StatusCode)
	}
	var list struct {
		Buckets []map[string]any `json:"buckets"`
	}
	decodeJSON(t, resp, &list)
	if len(list.Buckets) != 1 {
		t.Fatalf("len(buckets) = %d, want 1", len(list.Buckets))
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/buckets/"+created.ID, nil)
	req.Header.Set("X-API-Key", apiKey)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want 204", resp.StatusCode)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/buckets/"+created.ID, nil)
	req.Header.Set("X-API-Key", apiKey)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("delete de nuevo: status = %d, want 404", resp.StatusCode)
	}
}

func TestBucketCreate_RequiresWritePermission(t *testing.T) {
	ta := newTestApp(t, nil)
	readOnlyKey := ta.createTestAPIKey(t, uniqueBucketName("user"), "read")

	body, _ := json.Marshal(map[string]any{"name": uniqueBucketName("bucket"), "is_public": false})
	req := httptest.NewRequest(http.MethodPost, "/api/buckets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", readOnlyKey)
	resp := ta.do(t, req)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (key sin permiso write)", resp.StatusCode)
	}
}

func TestOtherUsersBucket_IsNotVisible(t *testing.T) {
	ta := newTestApp(t, nil)
	ownerKey := ta.createTestAPIKey(t, uniqueBucketName("owner"), "read,write")
	otherKey := ta.createTestAPIKey(t, uniqueBucketName("other"), "read,write")
	bucketName := uniqueBucketName("bucket")
	ta.createTestBucket(t, ownerKey, bucketName, false)

	// El otro usuario no debería poder subir a un bucket que no le pertenece.
	buf, contentType := multipartUpload(t, "a.png", "image/png", []byte("\x89PNG\r\n\x1a\n"), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/upload/"+bucketName, buf)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-API-Key", otherKey)
	resp := ta.do(t, req)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("upload a bucket ajeno: status = %d, want 403", resp.StatusCode)
	}
}

func TestUpload_RejectsDisallowedMimeType(t *testing.T) {
	ta := newTestApp(t, nil)
	apiKey := ta.createTestAPIKey(t, uniqueBucketName("user"), "read,write")
	bucketName := uniqueBucketName("bucket")
	ta.createTestBucket(t, apiKey, bucketName, false)

	buf, contentType := multipartUpload(t, "doc.txt", "text/plain", []byte("hello"), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/upload/"+bucketName, buf)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-API-Key", apiKey)
	resp := ta.do(t, req)
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", resp.StatusCode)
	}
}

func TestUpload_RejectsSpoofedContentType(t *testing.T) {
	// Un cliente puede declarar cualquier Content-Type para el part del
	// multipart sin que tenga relación con el contenido real. Este test
	// sube HTML/JS ejecutable declarado como "image/png" -- el escenario
	// clásico de subida de contenido malicioso disfrazado de imagen (stored
	// XSS) -- y espera que el sniffing de contenido lo rechace igual,
	// aunque el Content-Type declarado esté en la whitelist.
	ta := newTestApp(t, nil)
	apiKey := ta.createTestAPIKey(t, uniqueBucketName("user"), "read,write")
	bucketName := uniqueBucketName("bucket")
	ta.createTestBucket(t, apiKey, bucketName, false)

	malicious := []byte(`<!DOCTYPE html><html><body><script>alert(document.cookie)</script></body></html>`)
	buf, contentType := multipartUpload(t, "fake.png", "image/png", malicious, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/upload/"+bucketName, buf)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-API-Key", apiKey)
	resp := ta.do(t, req)
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415 (HTML disfrazado de image/png), body = %s", resp.StatusCode, readBody(t, resp))
	}
}

func TestUpload_RejectsOversizedFile(t *testing.T) {
	ta := newTestApp(t, func(c *config.Config) { c.MaxFileSize = 10 }) // 10 bytes
	apiKey := ta.createTestAPIKey(t, uniqueBucketName("user"), "read,write")
	bucketName := uniqueBucketName("bucket")
	ta.createTestBucket(t, apiKey, bucketName, false)

	buf, contentType := multipartUpload(t, "big.png", "image/png", bytes.Repeat([]byte("a"), 1024), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/upload/"+bucketName, buf)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-API-Key", apiKey)
	resp := ta.do(t, req)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413, body = %s", resp.StatusCode, readBody(t, resp))
	}
}

func TestUploadListDownloadDelete_FullFlow(t *testing.T) {
	ta := newTestApp(t, nil)
	apiKey := ta.createTestAPIKey(t, uniqueBucketName("user"), "read,write")
	bucketName := uniqueBucketName("bucket")
	ta.createTestBucket(t, apiKey, bucketName, false) // bucket privado

	content := []byte("\x89PNG\r\n\x1a\nfake-png-bytes")
	buf, contentType := multipartUpload(t, "photo.png", "image/png", content, map[string]string{
		"metadata": `{"alt":"una foto de prueba"}`,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/upload/"+bucketName, buf)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-API-Key", apiKey)
	resp := ta.do(t, req)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload: status = %d, body = %s", resp.StatusCode, readBody(t, resp))
	}

	var uploaded struct {
		File struct {
			Filename string         `json:"filename"`
			Metadata map[string]any `json:"metadata"`
		} `json:"file"`
	}
	decodeJSON(t, resp, &uploaded)
	if uploaded.File.Metadata["alt"] != "una foto de prueba" {
		t.Errorf("metadata no se guardó correctamente: %+v", uploaded.File.Metadata)
	}
	filename := uploaded.File.Filename

	// Listado con paginación.
	req = httptest.NewRequest(http.MethodGet, "/api/files/"+bucketName+"?page=1&page_size=10", nil)
	req.Header.Set("X-API-Key", apiKey)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: status = %d", resp.StatusCode)
	}
	var listed struct {
		Files      []map[string]any `json:"files"`
		TotalCount int              `json:"total_count"`
	}
	decodeJSON(t, resp, &listed)
	if listed.TotalCount != 1 || len(listed.Files) != 1 {
		t.Fatalf("listado inesperado: %+v", listed)
	}

	// Descarga sin API key sobre archivo privado -> 403.
	req = httptest.NewRequest(http.MethodGet, "/api/"+bucketName+"/"+filename, nil)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("download sin auth: status = %d, want 403", resp.StatusCode)
	}

	// Descarga con API key -> 200, contenido correcto.
	req = httptest.NewRequest(http.MethodGet, "/api/"+bucketName+"/"+filename, nil)
	req.Header.Set("X-API-Key", apiKey)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download con auth: status = %d", resp.StatusCode)
	}
	if got := readBody(t, resp); got != string(content) {
		t.Errorf("contenido descargado no coincide: %q != %q", got, string(content))
	}

	// Borrado.
	req = httptest.NewRequest(http.MethodDelete, "/api/"+bucketName+"/"+filename, nil)
	req.Header.Set("X-API-Key", apiKey)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want 204", resp.StatusCode)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/"+bucketName+"/"+filename, nil)
	req.Header.Set("X-API-Key", apiKey)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("download tras delete: status = %d, want 404", resp.StatusCode)
	}
}

func TestDownload_PublicFileNoAuthNeeded(t *testing.T) {
	ta := newTestApp(t, nil)
	apiKey := ta.createTestAPIKey(t, uniqueBucketName("user"), "read,write")
	bucketName := uniqueBucketName("bucket")
	ta.createTestBucket(t, apiKey, bucketName, true) // bucket público

	content := []byte("public-content")
	buf, contentType := multipartUpload(t, "public.png", "image/png", content, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/upload/"+bucketName, buf)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-API-Key", apiKey)
	resp := ta.do(t, req)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload: status = %d, body = %s", resp.StatusCode, readBody(t, resp))
	}
	var uploaded struct {
		File struct {
			Filename string `json:"filename"`
		} `json:"file"`
	}
	decodeJSON(t, resp, &uploaded)

	// Sin ninguna API key debe poder descargarse.
	req = httptest.NewRequest(http.MethodGet, "/api/"+bucketName+"/"+uploaded.File.Filename, nil)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 para archivo público sin auth", resp.StatusCode)
	}
	if got := readBody(t, resp); got != string(content) {
		t.Errorf("contenido = %q, want %q", got, string(content))
	}
}

func TestPresignedDownload_FullFlow(t *testing.T) {
	ta := newTestApp(t, nil)
	apiKey := ta.createTestAPIKey(t, uniqueBucketName("user"), "read,write")
	bucketName := uniqueBucketName("bucket")
	ta.createTestBucket(t, apiKey, bucketName, false)

	content := []byte("presigned-content")
	buf, contentType := multipartUpload(t, "secret.png", "image/png", content, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/upload/"+bucketName, buf)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-API-Key", apiKey)
	resp := ta.do(t, req)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload: status = %d", resp.StatusCode)
	}
	var uploaded struct {
		File struct {
			Filename string `json:"filename"`
		} `json:"file"`
	}
	decodeJSON(t, resp, &uploaded)

	req = httptest.NewRequest(http.MethodPost, "/api/presign/"+bucketName+"/"+uploaded.File.Filename, nil)
	req.Header.Set("X-API-Key", apiKey)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("presign: status = %d, body = %s", resp.StatusCode, readBody(t, resp))
	}
	var presigned struct {
		Token string `json:"token"`
	}
	decodeJSON(t, resp, &presigned)

	// El link presignado no requiere API key.
	req = httptest.NewRequest(http.MethodGet, "/api/download/"+presigned.Token, nil)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("consumo de token: status = %d", resp.StatusCode)
	}
	if got := readBody(t, resp); got != string(content) {
		t.Errorf("contenido = %q, want %q", got, string(content))
	}

	// Token con firma inválida.
	req = httptest.NewRequest(http.MethodGet, "/api/download/garbage.token", nil)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token inválido: status = %d, want 401", resp.StatusCode)
	}

	// Token válido pero expirado.
	signer := utils.NewTokenSigner(ta.cfg.SigningSecret)
	expired := signer.GenerateDownloadToken(bucketName, uploaded.File.Filename, -time.Minute)
	req = httptest.NewRequest(http.MethodGet, "/api/download/"+expired, nil)
	resp = ta.do(t, req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token expirado: status = %d, want 401", resp.StatusCode)
	}
}

func TestRateLimiting_GeneralAPI(t *testing.T) {
	ta := newTestApp(t, func(c *config.Config) {
		c.APIRateLimitMax = 2
		c.APIRateLimitWindow = 60
	})

	var lastStatus int
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/buckets", nil)
		req.Header.Set("X-API-Key", "sk_nonexistent_x") // no importa si autentica, el limiter corre antes
		resp := ta.do(t, req)
		lastStatus = resp.StatusCode
		if i < 2 && resp.StatusCode == http.StatusTooManyRequests {
			t.Fatalf("request %d: se limitó antes de tiempo (max=2)", i+1)
		}
	}
	if lastStatus != http.StatusTooManyRequests {
		t.Fatalf("última request: status = %d, want 429 tras superar el límite", lastStatus)
	}
}

func TestMalformedID_ReturnsNotFoundNotServerError(t *testing.T) {
	// Regresión: un id que no es un UUID válido no debe tumbar la query
	// contra Postgres con un 500 (que además filtraría el error crudo de la
	// base en la respuesta); debe comportarse como "no encontrado".
	ta := newTestApp(t, nil)
	apiKey := ta.createTestAPIKey(t, uniqueBucketName("user"), "read,write")

	garbage := []string{"not-a-uuid", "../../etc/passwd", "1' OR '1'='1"}

	for _, id := range garbage {
		escaped := url.PathEscape(id)

		req := httptest.NewRequest(http.MethodDelete, "/api/buckets/"+escaped, nil)
		req.Header.Set("X-API-Key", apiKey)
		resp := ta.do(t, req)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("DELETE /api/buckets/%s: status = %d, want 404", id, resp.StatusCode)
		}

		req = httptest.NewRequest(http.MethodDelete, "/api/auth/keys/"+escaped, nil)
		req.Header.Set("X-Master-Key", ta.cfg.MasterKey)
		resp = ta.do(t, req)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("DELETE /api/auth/keys/%s: status = %d, want 404", id, resp.StatusCode)
		}
	}

	// Una API key con un id (no un secreto) malformado en el header debe
	// autenticar como inválida (401), no romper con un 500.
	req := httptest.NewRequest(http.MethodGet, "/api/buckets", nil)
	req.Header.Set("X-API-Key", "sk_not-a-uuid_whatever")
	resp := ta.do(t, req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("API key con id malformado: status = %d, want 401", resp.StatusCode)
	}
}

func TestSecurityHeaders_ArePresent(t *testing.T) {
	ta := newTestApp(t, nil)

	resp := ta.do(t, httptest.NewRequest(http.MethodGet, "/health", nil))

	cases := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Cross-Origin-Resource-Policy": "cross-origin",
	}
	for header, want := range cases {
		if got := resp.Header.Get(header); got != want {
			t.Errorf("header %s = %q, want %q", header, got, want)
		}
	}
	if csp := resp.Header.Get("Content-Security-Policy"); csp == "" {
		t.Error("falta el header Content-Security-Policy")
	}
}
