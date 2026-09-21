# Guía de uso

## Modelo de permisos

Cada **API key** pertenece a un `user_id`
(string libre) y tiene permisos `read`/`write`/`admin` (comma-separated,
`admin` implica los otros dos). Los **buckets** y **archivos** tienen un
flag `is_public` independiente. La administración de API keys
(`/api/auth/keys/*`) está protegida por una **master key** separada
(`ADMIN_MASTER_KEY`), no por API keys normales.

## Ejemplos de uso de la API

Contrato JSON en snake_case:

```bash
export API_BASE=http://localhost:8080
export ADMIN_MASTER_KEY=$(cat secrets/admin_master_key.txt)

# 1. Generar una API key
curl -X POST $API_BASE/api/auth/keys \
  -H "X-Master-Key: $ADMIN_MASTER_KEY" -H "Content-Type: application/json" \
  -d '{"user_id": "mi-app", "permissions": "read,write"}'
export API_KEY="sk_...")

# 2. Crear un bucket
curl -X POST $API_BASE/api/buckets -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" -d '{"name": "avatares", "is_public": false}'

# 3. Subir un archivo
curl -X POST $API_BASE/api/upload/avatares -H "X-API-Key: $API_KEY" \
  -F "file=@./foto.jpg" -F "public=false" -F 'metadata={"tag":"perfil"}'

# 4. Listar archivos (paginado)
curl "$API_BASE/api/files/avatares?page=1&page_size=20" -H "X-API-Key: $API_KEY"

# 5. Descargar
curl "$API_BASE/api/avatares/<filename>" -H "X-API-Key: $API_KEY" -o foto.jpg

# 6. Link presignado temporal
curl -X POST "$API_BASE/api/presign/avatares/<filename>" -H "X-API-Key: $API_KEY"
curl "$API_BASE/api/download/<token>" -o foto.jpg

# 7. Eliminar
curl -X DELETE "$API_BASE/api/avatares/<filename>" -H "X-API-Key: $API_KEY"
```

## Uso desde un frontend Next.js en Vercel

Se recomienda proxear las requests desde un
Route Handler de Next.js (server-side) para que la API key nunca llegue al
browser, ya que `Cross-Origin-Resource-Policy: cross-origin` está
habilitado explícitamente para permitir que un frontend en otro origen
embeba imágenes/videos públicos vía `<img>`/`<video>` directamente. Ver
[`SECURITY.md`](../SECURITY.md) para el detalle de headers.

Configurar `CORS_ORIGIN` con el dominio real del frontend (`.env`, no es
sensible):

```bash
CORS_ORIGIN=https://mi-app.vercel.app,https://mi-dominio.com
```
