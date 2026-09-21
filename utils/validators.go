package utils

import (
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
)

// AllowedMimeTypes es la whitelist de tipos MIME aceptados para upload
// (imágenes y videos). Cualquier otro tipo es rechazado.
var AllowedMimeTypes = map[string]bool{
	// Imágenes
	"image/jpeg":    true,
	"image/png":     true,
	"image/gif":     true,
	"image/webp":    true,
	"image/svg+xml": true,
	"image/avif":    true,
	"image/heic":    true,
	// Videos
	"video/mp4":        true,
	"video/mpeg":       true,
	"video/webm":       true,
	"video/quicktime":  true,
	"video/x-msvideo":  true,
	"video/x-matroska": true,
}

// IsAllowedMimeType valida un tipo MIME contra la whitelist.
func IsAllowedMimeType(mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	return AllowedMimeTypes[mimeType]
}

// extensionByMimeType mapea cada tipo MIME permitido a UNA extensión fija.
// El nombre físico en disco siempre se genera a partir de esta tabla, nunca
// del nombre de archivo que mandó el cliente: el Content-Type del form y el
// nombre de archivo son dos campos independientes que un cliente puede
// declarar sin que se correspondan entre sí (ej: Content-Type "image/png"
// con Filename "shell.php"). Sin esto, el archivo terminaría guardado en
// disco con la extensión que el cliente haya elegido, sin relación con el
// tipo real validado.
var extensionByMimeType = map[string]string{
	"image/jpeg":       ".jpg",
	"image/png":        ".png",
	"image/gif":        ".gif",
	"image/webp":       ".webp",
	"image/svg+xml":    ".svg",
	"image/avif":       ".avif",
	"image/heic":       ".heic",
	"video/mp4":        ".mp4",
	"video/mpeg":       ".mpeg",
	"video/webm":       ".webm",
	"video/quicktime":  ".mov",
	"video/x-msvideo":  ".avi",
	"video/x-matroska": ".mkv",
}

// ExtensionForMimeType devuelve la extensión física a usar para un tipo MIME
// ya validado con IsAllowedMimeType. Sólo debe llamarse con un mimeType que
// haya pasado esa validación.
func ExtensionForMimeType(mimeType string) string {
	return extensionByMimeType[strings.ToLower(strings.TrimSpace(mimeType))]
}

var bucketNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{1,61}[a-z0-9])?$`)

// IsValidBucketName exige nombres de bucket estilo DNS (minúsculas, dígitos y
// guiones, 3-63 caracteres), igual que S3, para evitar problemas al usarlos
// como segmentos de path o de URL.
func IsValidBucketName(name string) bool {
	return len(name) >= 3 && len(name) <= 63 && bucketNameRe.MatchString(name)
}

var unsafeFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// SanitizeFilename limpia un nombre de archivo original para que sea seguro
// de usar en un path del filesystem: elimina cualquier componente de
// directorio, reemplaza caracteres no permitidos y evita nombres vacíos.
func SanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = unsafeFilenameChars.ReplaceAllString(name, "_")
	name = strings.TrimLeft(name, ".")
	if name == "" {
		name = "file"
	}
	return name
}

// LooksLikeHTML hace una detección barata de HTML disfrazado de otro tipo
// (ej. un .html subido con Content-Type: image/png para intentar servirlo
// después como si fuera una imagen), a partir de los primeros bytes del
// archivo (siguiendo las mismas reglas de sniffing que usan los browsers,
// vía http.DetectContentType). No es una validación completa de magic bytes
// por formato -- esta whitelist incluye demasiados formatos de imagen/video
// que la stdlib no sabe reconocer -- pero cierra el vector de ataque más
// importante: contenido HTML/script ejecutable por el browser escondido
// detrás de un Content-Type de imagen o video.
func LooksLikeHTML(sniffedBytes []byte) bool {
	return strings.HasPrefix(http.DetectContentType(sniffedBytes), "text/html")
}

// ValidateFileSize rechaza archivos que excedan el límite configurado.
func ValidateFileSize(size, maxSize int64) error {
	if size <= 0 {
		return fmt.Errorf("el archivo está vacío")
	}
	if size > maxSize {
		return fmt.Errorf("el archivo excede el tamaño máximo permitido (%d bytes)", maxSize)
	}
	return nil
}
