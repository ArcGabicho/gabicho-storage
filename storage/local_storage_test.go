package storage

import (
	"bytes"
	"mime/multipart"
	"os"
	"path/filepath"
	"testing"
)

// newTestFileHeader construye un *multipart.FileHeader real (como el que
// produce Fiber en un upload) a partir de un nombre y contenido en memoria,
// para poder testear LocalStorage sin necesitar un servidor HTTP.
func newTestFileHeader(t *testing.T, fieldName, filename string, content []byte) *multipart.FileHeader {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("writing part content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}

	reader := multipart.NewReader(&buf, writer.Boundary())
	form, err := reader.ReadForm(int64(len(content)) + 1024)
	if err != nil {
		t.Fatalf("ReadForm: %v", err)
	}
	t.Cleanup(func() { _ = form.RemoveAll() })

	files := form.File[fieldName]
	if len(files) != 1 {
		t.Fatalf("expected 1 file in form, got %d", len(files))
	}
	return files[0]
}

func TestNewLocalStorage_CreatesRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nested", "storage")

	s, err := NewLocalStorage(root)
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}

	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("expected root dir to exist: %v", err)
	}
	if s.RootPath != root {
		t.Errorf("RootPath = %q, want %q", s.RootPath, root)
	}
}

func TestLocalStorage_SaveAndDeleteFile(t *testing.T) {
	s, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}

	content := []byte("fake image bytes")
	fh := newTestFileHeader(t, "file", "photo.jpg", content)

	storedName, relPath, err := s.SaveUploadedFile("my-bucket", fh, ".jpg")
	if err != nil {
		t.Fatalf("SaveUploadedFile: %v", err)
	}

	if filepath.Ext(storedName) != ".jpg" {
		t.Errorf("stored name %q should keep the original extension", storedName)
	}
	if storedName == "photo.jpg" {
		t.Error("stored name should be a generated UUID, not the original filename (avoids collisions/traversal)")
	}

	fullPath := s.FullPath(relPath)
	got, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("saved content = %q, want %q", got, content)
	}

	if err := s.DeleteFile(relPath); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
		t.Error("expected file to be removed from disk after DeleteFile")
	}
}

func TestLocalStorage_DeleteFile_NotExist(t *testing.T) {
	s, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}

	// Borrar un archivo que nunca existió no debe ser un error (idempotente).
	if err := s.DeleteFile("nonexistent-bucket/nonexistent-file.jpg"); err != nil {
		t.Errorf("DeleteFile on missing file should not error, got: %v", err)
	}
}

func TestLocalStorage_RemoveBucketDir(t *testing.T) {
	s, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}

	if err := s.EnsureBucketDir("my-bucket"); err != nil {
		t.Fatalf("EnsureBucketDir: %v", err)
	}

	content := []byte("data")
	fh := newTestFileHeader(t, "file", "a.png", content)
	_, relPath, err := s.SaveUploadedFile("my-bucket", fh, ".png")
	if err != nil {
		t.Fatalf("SaveUploadedFile: %v", err)
	}

	if err := s.RemoveBucketDir("my-bucket"); err != nil {
		t.Fatalf("RemoveBucketDir: %v", err)
	}

	if _, err := os.Stat(s.FullPath(relPath)); !os.IsNotExist(err) {
		t.Error("expected file to be gone after removing its bucket dir")
	}
	if _, err := os.Stat(filepath.Join(s.RootPath, "my-bucket")); !os.IsNotExist(err) {
		t.Error("expected bucket directory itself to be removed")
	}
}

func TestLocalStorage_SaveUploadedFile_UniqueNames(t *testing.T) {
	s, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}

	fh1 := newTestFileHeader(t, "file", "same-name.png", []byte("one"))
	fh2 := newTestFileHeader(t, "file", "same-name.png", []byte("two"))

	name1, _, err := s.SaveUploadedFile("bucket", fh1, ".png")
	if err != nil {
		t.Fatalf("SaveUploadedFile (1): %v", err)
	}
	name2, _, err := s.SaveUploadedFile("bucket", fh2, ".png")
	if err != nil {
		t.Fatalf("SaveUploadedFile (2): %v", err)
	}

	if name1 == name2 {
		t.Error("two uploads with the same original filename should get different stored names")
	}
}
