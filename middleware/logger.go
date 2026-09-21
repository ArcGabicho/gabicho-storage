package middleware

import (
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
)

// RequestLogger registra cada request en formato estructurado (JSON vía
// log/slog): método, path, status, latencia, IP e id de API key si aplica.
func RequestLogger() fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		// El ErrorHandler global recién escribe el status real en la response
		// DESPUÉS de que este middleware retorne (corre por fuera de la
		// cadena de c.Next()), así que si el handler devolvió un error hay
		// que derivar el status a loguear desde ese error en vez de leer
		// c.Response().StatusCode(), que todavía tendría el valor por defecto.
		status := c.Response().StatusCode()
		if err != nil {
			var fe *fiber.Error
			if errors.As(err, &fe) {
				status = fe.Code
			} else {
				status = fiber.StatusInternalServerError
			}
		}

		attrs := []any{
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", status),
			slog.Duration("latency", time.Since(start)),
			slog.String("ip", c.IP()),
		}
		if key := CurrentAPIKey(c); key != nil {
			attrs = append(attrs, slog.String("user_id", key.UserID))
		}

		switch {
		case status >= 500:
			slog.Error("request", attrs...)
		case status >= 400:
			slog.Warn("request", attrs...)
		default:
			slog.Info("request", attrs...)
		}

		return err
	}
}
