// Package storage implementa el backend de almacenamiento físico de
// archivos. Los archivos se organizan en disco como
// <root>/<bucket>/<filename-generado>, separado de sus metadatos en
// PostgreSQL, de forma análoga a cómo S3/GCS separan objeto y metadata.
package storage

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// LocalStorage guarda los archivos en el filesystem local bajo RootPath.
type LocalStorage struct {
	RootPath string
}

// NewLocalStorage crea el almacenamiento local, asegurando que RootPath
// exista.
func NewLocalStorage(rootPath string) (*LocalStorage, error) {
	if err := os.MkdirAll(rootPath, 0o750); err != nil {
		return nil, fmt.Errorf("creando storage path %q: %w", rootPath, err)
	}
	return &LocalStorage{RootPath: rootPath}, nil
}

// EnsureBucketDir crea el directorio físico de un bucket.
func (s *LocalStorage) EnsureBucketDir(bucket string) error {
	return os.MkdirAll(s.bucketDir(bucket), 0o750)
}

// RemoveBucketDir elimina el directorio físico completo de un bucket
// (usado al eliminar un bucket).
func (s *LocalStorage) RemoveBucketDir(bucket string) error {
	return os.RemoveAll(s.bucketDir(bucket))
}

// SaveUploadedFile persiste un multipart.FileHeader dentro del bucket,
// generando un nombre físico único (UUID + extensión) para evitar
// colisiones y ataques de path traversal. La extensión la decide el
// caller (handlers/upload.go, a partir del Content-Type ya validado) y
// nunca el nombre de archivo que mandó el cliente: el nombre de archivo y el
// Content-Type de un multipart son dos campos independientes que el cliente
// puede declarar sin que se correspondan entre sí. Devuelve el nombre
// físico generado y la ruta relativa a RootPath guardada en la BD.
func (s *LocalStorage) SaveUploadedFile(bucket string, fh *multipart.FileHeader, ext string) (storedName string, relativePath string, err error) {
	if err = s.EnsureBucketDir(bucket); err != nil {
		return "", "", err
	}

	storedName = uuid.NewString() + ext
	relativePath = filepath.Join(bucket, storedName)
	fullPath := filepath.Join(s.RootPath, relativePath)

	src, err := fh.Open()
	if err != nil {
		return "", "", fmt.Errorf("abriendo archivo subido: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return "", "", fmt.Errorf("creando archivo destino: %w", err)
	}
	defer dst.Close()

	if _, err = io.Copy(dst, src); err != nil {
		os.Remove(fullPath)
		return "", "", fmt.Errorf("copiando contenido: %w", err)
	}

	return storedName, relativePath, nil
}

// FullPath resuelve la ruta absoluta de un path relativo almacenado en BD.
func (s *LocalStorage) FullPath(relativePath string) string {
	return filepath.Join(s.RootPath, relativePath)
}

// DeleteFile elimina el archivo físico correspondiente a un path relativo.
func (s *LocalStorage) DeleteFile(relativePath string) error {
	err := os.Remove(s.FullPath(relativePath))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *LocalStorage) bucketDir(bucket string) string {
	return filepath.Join(s.RootPath, bucket)
}
