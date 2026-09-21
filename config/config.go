// Package config carga y valida la configuración del servicio desde variables
// de entorno (con soporte opcional de un archivo .env para desarrollo local).
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config agrupa toda la configuración runtime del servicio.
type Config struct {
	// Servidor
	APIPort     string
	CORSOrigin  string
	Environment string

	// Base de datos
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// Almacenamiento
	StoragePath string
	MaxFileSize int64 // bytes

	// Seguridad
	TokenExpiryMinutes int
	SigningSecret      string
	MasterKey          string

	// Rate limiting
	// APIRateLimit* aplica a toda /api/* (protección general anti-abuso).
	// UploadRateLimit* es un límite adicional y más estricto sólo para
	// /api/upload/:bucket, clave por API key (no por IP).
	APIRateLimitMax       int
	APIRateLimitWindow    int // segundos
	UploadRateLimitMax    int
	UploadRateLimitWindow int // segundos
}

// Load lee la configuración desde el entorno. Si existe un archivo .env en el
// directorio de trabajo, sus valores se cargan primero (sin sobrescribir
// variables ya presentes en el entorno real).
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		APIPort:     getEnv("API_PORT", "8080"),
		CORSOrigin:  getEnv("CORS_ORIGIN", "*"),
		Environment: getEnv("ENVIRONMENT", "production"),

		DBHost:    getEnv("DB_HOST", "localhost"),
		DBPort:    getEnv("DB_PORT", "5432"),
		DBUser:    getEnv("DB_USER", "storage"),
		DBName:    getEnv("DB_NAME", "storage"),
		DBSSLMode: getEnv("DB_SSLMODE", "disable"),

		StoragePath: getEnv("STORAGE_PATH", "/data/storage"),
	}

	// DB_PASSWORD, SIGNING_SECRET y ADMIN_MASTER_KEY son secretos: se
	// resuelven vía getSecret, que soporta tanto la variable de entorno
	// directa (desarrollo local) como su variante "_FILE" (Docker secrets --
	// ver docker-compose.yml). Nunca deben pasarse como valor por defecto ni
	// loguearse.
	var err error
	cfg.DBPassword, err = getSecret("DB_PASSWORD")
	if err != nil {
		return nil, err
	}
	cfg.SigningSecret, err = getSecret("SIGNING_SECRET")
	if err != nil {
		return nil, err
	}
	cfg.MasterKey, err = getSecret("ADMIN_MASTER_KEY")
	if err != nil {
		return nil, err
	}

	maxFileSizeMB, err := getEnvInt("MAX_FILE_SIZE_MB", 500)
	if err != nil {
		return nil, fmt.Errorf("MAX_FILE_SIZE_MB inválido: %w", err)
	}
	cfg.MaxFileSize = int64(maxFileSizeMB) * 1024 * 1024

	cfg.TokenExpiryMinutes, err = getEnvInt("TOKEN_EXPIRY_MINUTES", 15)
	if err != nil {
		return nil, fmt.Errorf("TOKEN_EXPIRY_MINUTES inválido: %w", err)
	}

	cfg.APIRateLimitMax, err = getEnvInt("API_RATE_LIMIT_MAX", 300)
	if err != nil {
		return nil, fmt.Errorf("API_RATE_LIMIT_MAX inválido: %w", err)
	}

	cfg.APIRateLimitWindow, err = getEnvInt("API_RATE_LIMIT_WINDOW_SECONDS", 60)
	if err != nil {
		return nil, fmt.Errorf("API_RATE_LIMIT_WINDOW_SECONDS inválido: %w", err)
	}

	cfg.UploadRateLimitMax, err = getEnvInt("UPLOAD_RATE_LIMIT_MAX", 30)
	if err != nil {
		return nil, fmt.Errorf("UPLOAD_RATE_LIMIT_MAX inválido: %w", err)
	}

	cfg.UploadRateLimitWindow, err = getEnvInt("UPLOAD_RATE_LIMIT_WINDOW_SECONDS", 60)
	if err != nil {
		return nil, fmt.Errorf("UPLOAD_RATE_LIMIT_WINDOW_SECONDS inválido: %w", err)
	}

	if cfg.DBPassword == "" {
		return nil, fmt.Errorf("DB_PASSWORD es obligatorio")
	}
	if cfg.SigningSecret == "" {
		return nil, fmt.Errorf("SIGNING_SECRET es obligatorio (usado para firmar tokens presignados)")
	}
	if cfg.MasterKey == "" {
		return nil, fmt.Errorf("ADMIN_MASTER_KEY es obligatorio (usado para administrar API keys)")
	}

	return cfg, nil
}

// DSN construye el connection string de PostgreSQL. Usa net/url para
// escapar correctamente usuario/contraseña -- un password con caracteres
// especiales (@, :, /, %, etc.) generado a mano en vez de con
// `openssl rand -hex` rompería una interpolación simple con fmt.Sprintf.
func (c *Config) DSN() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.DBUser, c.DBPassword),
		Host:     c.DBHost + ":" + c.DBPort,
		Path:     "/" + c.DBName,
		RawQuery: "sslmode=" + url.QueryEscape(c.DBSSLMode),
	}
	return u.String()
}

// CORSOrigins parte CORS_ORIGIN (coma-separado) en una lista de orígenes.
func (c *Config) CORSOrigins() []string {
	parts := strings.Split(c.CORSOrigin, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			origins = append(origins, p)
		}
	}
	if len(origins) == 0 {
		return []string{"*"}
	}
	return origins
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// getSecret resuelve un valor sensible con la misma convención "_FILE" que
// usan las imágenes oficiales de Docker (ej. POSTGRES_PASSWORD_FILE): si
// <KEY>_FILE está seteada, lee el secreto desde ese archivo -- pensado para
// Docker secrets, montados en /run/secrets/<nombre> y por lo tanto NUNCA
// visibles vía `docker inspect` ni `docker compose config` (a diferencia de
// un valor puesto directo en `environment:`, que sí queda en texto plano en
// ambos). Si no está seteada, cae a la variable de entorno <KEY> directa,
// para poder seguir usando un .env simple en desarrollo local.
func getSecret(key string) (string, error) {
	if filePath, ok := os.LookupEnv(key + "_FILE"); ok && filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("leyendo %s_FILE (%s): %w", key, filePath, err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return getEnv(key, ""), nil
}

func getEnvInt(key string, fallback int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	return strconv.Atoi(v)
}
