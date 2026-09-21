// Command gabicho-storage levanta el servicio de almacenamiento self-hosted:
// un servidor Fiber que expone una API tipo Firebase Storage / R2 sobre
// PostgreSQL (metadata) y el filesystem local (contenido binario).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"github.com/gabicho/gabicho-storage/config"
	"github.com/gabicho/gabicho-storage/db"
	"github.com/gabicho/gabicho-storage/handlers"
	"github.com/gabicho/gabicho-storage/middleware"
	"github.com/gabicho/gabicho-storage/storage"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuración inválida", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DSN())
	if err != nil {
		slog.Error("no se pudo conectar a PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		slog.Error("no se pudo migrar el esquema", "error", err)
		os.Exit(1)
	}
	slog.Info("esquema de base de datos listo")

	store, err := storage.NewLocalStorage(cfg.StoragePath)
	if err != nil {
		slog.Error("no se pudo inicializar el storage local", "error", err)
		os.Exit(1)
	}

	repo := db.NewRepository(pool)
	h := handlers.New(repo, store, cfg)

	app := newApp(cfg, h, repo)

	go func() {
		addr := ":" + cfg.APIPort
		slog.Info("servidor escuchando", "addr", addr)
		if err := app.Listen(addr); err != nil {
			slog.Error("el servidor se detuvo con error", "error", err)
		}
	}()

	<-ctx.Done()
	slog.Info("apagando servidor...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		slog.Error("error durante el shutdown", "error", err)
	}
	slog.Info("servidor detenido correctamente")
}

// newApp construye la *fiber.App con toda su configuración, middlewares y
// rutas. Separado de main() para que los tests de integración puedan
// levantar exactamente la misma app (con un *config.Config de test y una
// base/almacenamiento descartables) sin duplicar el wiring.
func newApp(cfg *config.Config, h *handlers.Handler, repo *db.Repository) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: "gabicho-storage",
		// Con margen por encima de MaxFileSize (que valida el handler de
		// upload y devuelve un 413 con JSON prolijo): si BodyLimit fuera
		// exactamente MaxFileSize, fasthttp cortaría la conexión al leer un
		// body más grande ANTES de que la request llegue al handler, y el
		// cliente nunca vería una respuesta HTTP legible, sólo un connection
		// reset. El límite de fasthttp queda como techo duro contra bodies
		// verdaderamente abusivos; el 413 prolijo es el camino esperado.
		BodyLimit:    int(cfg.MaxFileSize) + 1*1024*1024,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,

		// El servicio sólo se expone vía Cloudflare Tunnel: cloudflared corre
		// en el mismo host y es el único proceso que puede alcanzar el puerto
		// publicado (ver docker-compose.yml, bindeado a 127.0.0.1). Por eso es
		// seguro confiar en el header CF-Connecting-IP que Cloudflare siempre
		// setea con la IP real del visitante -- sin esto, c.IP() devolvería la
		// IP interna de Docker/loopback para TODAS las requests, rompiendo el
		// rate limiting por IP y el campo "ip" de los logs.
		ProxyHeader: "CF-Connecting-IP",
		TrustProxy:  true,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Loopback: true,
			Private:  true,
		},
	})

	app.Use(recover.New())
	app.Use(middleware.RequestLogger())
	app.Use(helmet.New(helmet.Config{
		// Nunca queremos que un response nuestro (JSON o archivo) se embeba
		// en un <frame>/<iframe> de otro sitio.
		XFrameOptions: "DENY",
		// "sandbox" desactiva scripts, forms y navegación dentro del propio
		// documento servido; "default-src 'none'" bloquea cualquier carga de
		// subrecursos. Es la mitigación estándar (la misma que usa GitHub
		// para raw.githubusercontent.com) contra XSS almacenado vía SVG/HTML
		// subido con un Content-Type de imagen: aunque el archivo contenga
		// <script>, el browser no lo va a poder ejecutar al servirlo, se
		// abra directamente o se embeba en un <img>/<video>.
		ContentSecurityPolicy: "default-src 'none'; sandbox",
		// Por default el helmet de Fiber pone "same-origin", que bloquearía
		// que un frontend en otro origen (ej. Next.js en Vercel) cargue
		// imágenes/videos públicos de este servicio vía <img>/<video src>.
		// Este servicio está pensado exactamente para eso, así que se
		// habilita explícitamente.
		CrossOriginResourcePolicy: "cross-origin",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.CORSOrigins(),
		AllowMethods: []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Content-Type", "Authorization", "X-API-Key", "X-Master-Key"},
		// Headers propios que el JS del browser puede leer de la response
		// (por default el browser sólo expone los headers "simples" de CORS).
		ExposeHeaders: []string{"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"},
		// Cachea la respuesta al preflight OPTIONS en el browser, para que un
		// frontend en Next.js/Vercel no dispare un preflight en cada request.
		MaxAge: 300,
	}))

	registerRoutes(app, h, cfg, repo)

	return app
}

func registerRoutes(app *fiber.App, h *handlers.Handler, cfg *config.Config, repo *db.Repository) {
	app.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// Rate limit general: protege toda /api/* de abuso (fuerza bruta de API
	// keys, scraping, DoS de bajo esfuerzo). Se suma al de upload, que es más
	// estricto y sólo aplica a ese endpoint.
	apiLimiter := limiter.New(limiter.Config{
		Max:          cfg.APIRateLimitMax,
		Expiration:   time.Duration(cfg.APIRateLimitWindow) * time.Second,
		KeyGenerator: middleware.RateLimitKey,
	})

	uploadLimiter := limiter.New(limiter.Config{
		Max:          cfg.UploadRateLimitMax,
		Expiration:   time.Duration(cfg.UploadRateLimitWindow) * time.Second,
		KeyGenerator: middleware.RateLimitKey,
	})

	api := app.Group("/api", apiLimiter)

	// --- Autenticación / administración de API keys (protegido por master key) ---
	authGroup := api.Group("/auth/keys", middleware.MasterKeyAuth(cfg.MasterKey))
	authGroup.Post("/", h.CreateAPIKeyHandler)
	authGroup.Get("/", h.ListAPIKeysHandler)
	authGroup.Delete("/:key_id", h.RevokeAPIKeyHandler)

	// --- Buckets (requieren API key) ---
	buckets := api.Group("/buckets", middleware.APIKeyAuth(repo))
	buckets.Post("/", middleware.RequirePermission("write"), h.CreateBucket)
	buckets.Get("/", middleware.RequirePermission("read"), h.ListBuckets)
	buckets.Delete("/:id", middleware.RequirePermission("write"), h.DeleteBucket)

	// --- Upload / listado / borrado de archivos (requieren API key) ---
	api.Post("/upload/:bucket", middleware.APIKeyAuth(repo), middleware.RequirePermission("write"), uploadLimiter, h.UploadFile)
	api.Get("/files/:bucket", middleware.APIKeyAuth(repo), middleware.RequirePermission("read"), h.ListFiles)
	api.Delete("/:bucket/:filename", middleware.APIKeyAuth(repo), middleware.RequirePermission("write"), h.DeleteFile)

	// --- Links presignados ---
	api.Post("/presign/:bucket/:filename", middleware.APIKeyAuth(repo), middleware.RequirePermission("read"), h.GeneratePresignedURL)
	api.Get("/download/:token", h.PresignedDownload)

	// --- Descarga directa (pública si el archivo es público, o con API key) ---
	api.Get("/:bucket/:filename", middleware.OptionalAPIKeyAuth(repo), h.DownloadFile)
}
