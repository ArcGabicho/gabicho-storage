package utils

import (
	"strings"
	"testing"
)

func TestIsAllowedMimeType(t *testing.T) {
	cases := []struct {
		mime string
		want bool
	}{
		{"image/jpeg", true},
		{"image/png", true},
		{"video/mp4", true},
		{"video/webm", true},
		{"IMAGE/PNG", true},        // case-insensitive
		{"  image/png  ", true},    // espacios
		{"text/plain", false},      // no está en la whitelist
		{"application/pdf", false}, // no está en la whitelist
		{"application/json", false},
		{"", false},
	}

	for _, tc := range cases {
		got := IsAllowedMimeType(tc.mime)
		if got != tc.want {
			t.Errorf("IsAllowedMimeType(%q) = %v, want %v", tc.mime, got, tc.want)
		}
	}
}

func TestIsValidBucketName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"my-bucket", true},
		{"bucket123", true},
		{"abc", true},                    // mínimo 3 caracteres
		{"a", false},                     // muy corto
		{"ab", false},                    // muy corto
		{"My-Bucket", false},             // mayúsculas no permitidas
		{"my_bucket", false},             // guion bajo no permitido
		{"-my-bucket", false},            // no puede empezar con guion
		{"my-bucket-", false},            // no puede terminar con guion
		{"", false},                      // vacío
		{"has a space", false},           // espacios no permitidos
		{strings.Repeat("a", 63), true},  // límite superior exacto
		{strings.Repeat("a", 64), false}, // demasiado largo
	}

	for _, tc := range cases {
		got := IsValidBucketName(tc.name)
		if got != tc.want {
			t.Errorf("IsValidBucketName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"photo.jpg", "photo.jpg"},
		{"my file (1).png", "my_file__1_.png"},
		{"../../etc/passwd", "passwd"},
		{"/etc/passwd", "passwd"},
		{"..hidden", "hidden"},
		{"", "file"},
		{"日本語.png", "___.png"},
	}

	for _, tc := range cases {
		got := SanitizeFilename(tc.name)
		if got != tc.want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSanitizeFilename_NeverEmpty(t *testing.T) {
	cases := []string{"", ".", "..", "/", "///", "..."}
	for _, tc := range cases {
		got := SanitizeFilename(tc)
		if got == "" {
			t.Errorf("SanitizeFilename(%q) returned empty string", tc)
		}
	}
}

func TestExtensionForMimeType(t *testing.T) {
	cases := []struct {
		mime string
		want string
	}{
		{"image/png", ".png"},
		{"image/svg+xml", ".svg"},
		{"video/quicktime", ".mov"},
		{"IMAGE/PNG", ".png"}, // case-insensitive, igual que IsAllowedMimeType
		{"application/octet-stream", ""},
		{"", ""},
	}

	for _, tc := range cases {
		got := ExtensionForMimeType(tc.mime)
		if got != tc.want {
			t.Errorf("ExtensionForMimeType(%q) = %q, want %q", tc.mime, got, tc.want)
		}
	}
}

func TestExtensionForMimeType_NeverDerivesFromClientFilename(t *testing.T) {
	// Regresión: la extensión física SIEMPRE debe salir de esta tabla fija a
	// partir del Content-Type ya validado, nunca del nombre de archivo que
	// mandó el cliente (que es un campo separado, no correlacionado). Este
	// test documenta esa garantía para cada tipo permitido.
	for mime := range AllowedMimeTypes {
		if ExtensionForMimeType(mime) == "" {
			t.Errorf("tipo permitido %q no tiene extensión mapeada", mime)
		}
	}
}

func TestLooksLikeHTML(t *testing.T) {
	cases := []struct {
		name    string
		content []byte
		want    bool
	}{
		{"doctype html", []byte("<!DOCTYPE html><html><body>hi</body></html>"), true},
		{"html tag sin doctype", []byte("<html><script>alert(1)</script></html>"), true},
		{"png real", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), false},
		{"jpeg real", []byte("\xff\xd8\xff\xe0\x00\x10JFIF"), false},
		{"texto plano inocuo", []byte("hola mundo"), false},
		{"vacío", []byte{}, false},
	}

	for _, tc := range cases {
		got := LooksLikeHTML(tc.content)
		if got != tc.want {
			t.Errorf("%s: LooksLikeHTML(%q) = %v, want %v", tc.name, tc.content, got, tc.want)
		}
	}
}

func TestValidateFileSize(t *testing.T) {
	const maxSize = 500 * 1024 * 1024

	cases := []struct {
		name    string
		size    int64
		wantErr bool
	}{
		{"tamaño válido", 1024, false},
		{"exactamente el máximo", maxSize, false},
		{"excede el máximo", maxSize + 1, true},
		{"tamaño cero", 0, true},
		{"tamaño negativo", -1, true},
	}

	for _, tc := range cases {
		err := ValidateFileSize(tc.size, maxSize)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: ValidateFileSize(%d, %d) error = %v, wantErr %v", tc.name, tc.size, maxSize, err, tc.wantErr)
		}
	}
}
