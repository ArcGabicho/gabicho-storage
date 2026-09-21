package config

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// setRequiredEnv setea las tres variables sin default que Load() exige,
// dejando que el test override lo que necesite validar.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("SIGNING_SECRET", "signing-secret")
	t.Setenv("ADMIN_MASTER_KEY", "master-key")
}

func TestLoad_Success(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.APIPort != "8080" {
		t.Errorf("APIPort = %q, want %q", cfg.APIPort, "8080")
	}
	if cfg.MaxFileSize != 500*1024*1024 {
		t.Errorf("MaxFileSize = %d, want %d (default 500MB)", cfg.MaxFileSize, 500*1024*1024)
	}
	if cfg.TokenExpiryMinutes != 15 {
		t.Errorf("TokenExpiryMinutes = %d, want 15", cfg.TokenExpiryMinutes)
	}
	if cfg.APIRateLimitMax != 300 {
		t.Errorf("APIRateLimitMax = %d, want 300", cfg.APIRateLimitMax)
	}
	if cfg.UploadRateLimitMax != 30 {
		t.Errorf("UploadRateLimitMax = %d, want 30", cfg.UploadRateLimitMax)
	}
}

func TestLoad_MissingRequiredVars(t *testing.T) {
	cases := []string{"DB_PASSWORD", "SIGNING_SECRET", "ADMIN_MASTER_KEY"}

	for _, missing := range cases {
		t.Run(missing, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv(missing, "")

			if _, err := Load(); err == nil {
				t.Fatalf("Load() with empty %s: expected error, got nil", missing)
			}
		})
	}
}

func TestLoad_InvalidIntEnvVar(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("MAX_FILE_SIZE_MB", "not-a-number")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with invalid MAX_FILE_SIZE_MB: expected error, got nil")
	}
}

func TestLoad_CustomValues(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("API_PORT", "9090")
	t.Setenv("MAX_FILE_SIZE_MB", "10")
	t.Setenv("TOKEN_EXPIRY_MINUTES", "5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.APIPort != "9090" {
		t.Errorf("APIPort = %q, want %q", cfg.APIPort, "9090")
	}
	if cfg.MaxFileSize != 10*1024*1024 {
		t.Errorf("MaxFileSize = %d, want %d", cfg.MaxFileSize, 10*1024*1024)
	}
	if cfg.TokenExpiryMinutes != 5 {
		t.Errorf("TokenExpiryMinutes = %d, want 5", cfg.TokenExpiryMinutes)
	}
}

// TestLoad_SecretsFromFile verifica el soporte de Docker secrets: si
// DB_PASSWORD_FILE/SIGNING_SECRET_FILE/ADMIN_MASTER_KEY_FILE apuntan a un
// archivo, el secreto se lee de ahí en vez de la variable de entorno
// directa -- así nunca necesita pasarse por `environment:` en
// docker-compose.yml, que queda en texto plano en `docker inspect` y
// `docker compose config`.
func TestLoad_SecretsFromFile(t *testing.T) {
	dir := t.TempDir()

	writeSecretFile := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%s): %v", path, err)
		}
		return path
	}

	dbPasswordPath := writeSecretFile("db_password", "secret-from-file\n") // con salto de línea final, como suele quedar un archivo escrito a mano
	signingSecretPath := writeSecretFile("signing_secret", "signing-from-file")
	masterKeyPath := writeSecretFile("master_key", "master-from-file")

	// No debe haber variables planas seteadas: si Load() las necesitara,
	// fallaría, así que este test también prueba que _FILE tiene prioridad
	// y es suficiente por sí solo.
	t.Setenv("DB_PASSWORD_FILE", dbPasswordPath)
	t.Setenv("SIGNING_SECRET_FILE", signingSecretPath)
	t.Setenv("ADMIN_MASTER_KEY_FILE", masterKeyPath)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.DBPassword != "secret-from-file" {
		t.Errorf("DBPassword = %q, want %q (sin el salto de línea final)", cfg.DBPassword, "secret-from-file")
	}
	if cfg.SigningSecret != "signing-from-file" {
		t.Errorf("SigningSecret = %q, want %q", cfg.SigningSecret, "signing-from-file")
	}
	if cfg.MasterKey != "master-from-file" {
		t.Errorf("MasterKey = %q, want %q", cfg.MasterKey, "master-from-file")
	}
}

func TestLoad_SecretFile_TakesPriorityOverPlainVar(t *testing.T) {
	setRequiredEnv(t) // deja DB_PASSWORD=secret, etc. seteadas

	dir := t.TempDir()
	path := filepath.Join(dir, "db_password")
	if err := os.WriteFile(path, []byte("from-file-wins"), 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	t.Setenv("DB_PASSWORD_FILE", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.DBPassword != "from-file-wins" {
		t.Errorf("DBPassword = %q, want %q (la variante _FILE debería ganar)", cfg.DBPassword, "from-file-wins")
	}
}

func TestLoad_SecretFile_MissingFileErrors(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DB_PASSWORD_FILE", "/no/existe/este/archivo")

	if _, err := Load(); err == nil {
		t.Fatal("Load() con DB_PASSWORD_FILE apuntando a un archivo inexistente: se esperaba error")
	}
}

func TestConfig_DSN(t *testing.T) {
	cfg := &Config{
		DBUser:     "storage",
		DBPassword: "s3cr3t",
		DBHost:     "postgres",
		DBPort:     "5432",
		DBName:     "storage",
		DBSSLMode:  "disable",
	}

	want := "postgres://storage:s3cr3t@postgres:5432/storage?sslmode=disable"
	if got := cfg.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}

// TestConfig_DSN_EscapesSpecialCharacters es una regresión: armar el DSN con
// fmt.Sprintf en vez de net/url rompía (o generaba un connection string mal
// formado, apuntando al host/db equivocado) si el password tenía
// caracteres reservados de una URL como '@', ':' o '/'.
func TestConfig_DSN_EscapesSpecialCharacters(t *testing.T) {
	cfg := &Config{
		DBUser:     "storage",
		DBPassword: "p@ss:w/rd#1",
		DBHost:     "postgres",
		DBPort:     "5432",
		DBName:     "storage",
		DBSSLMode:  "disable",
	}

	dsn := cfg.DSN()

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("el DSN generado no es una URL válida: %v (dsn=%q)", err, dsn)
	}
	if got, _ := parsed.User.Password(); got != cfg.DBPassword {
		t.Errorf("password parseado de vuelta = %q, want %q (dsn=%q)", got, cfg.DBPassword, dsn)
	}
	if parsed.Hostname() != cfg.DBHost {
		t.Errorf("host parseado de vuelta = %q, want %q (dsn=%q)", parsed.Hostname(), cfg.DBHost, dsn)
	}
}

func TestConfig_CORSOrigins(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"single origin", "https://example.com", []string{"https://example.com"}},
		{"multiple, sin espacios", "https://a.com,https://b.com", []string{"https://a.com", "https://b.com"}},
		{"multiple, con espacios", "https://a.com, https://b.com , https://c.com", []string{"https://a.com", "https://b.com", "https://c.com"}},
		{"wildcard", "*", []string{"*"}},
		{"vacío cae a wildcard", "", []string{"*"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{CORSOrigin: tc.input}
			got := cfg.CORSOrigins()
			if len(got) != len(tc.want) {
				t.Fatalf("CORSOrigins() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("CORSOrigins()[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
