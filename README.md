# gabicho-storage

Servicio de storage self-hosted en Go (Fiber v3) como alternativa a Firebase
Storage / Cloudflare R2: buckets, upload/descarga de imágenes y videos,
metadata en PostgreSQL, autenticación con API keys y links de descarga
presignados con expiración.

## Stack

- Go 1.26 + [Fiber v3](https://github.com/gofiber/fiber)
- PostgreSQL 15 (metadata)
- Filesystem local organizado por buckets (contenido binario)
- Docker + Docker Compose
- Cloudflare Tunnel (exposición pública con TLS automático, sin abrir puertos)

## Arquitectura del proyecto

```
.
├── docker-compose.yml       # Stack: API + Postgres
├── Dockerfile                # Build multi-stage (Go builder -> alpine runtime)
├── .env.example               # Variables de entorno documentadas
├── SECURITY.md                # Mapeo detallado contra OWASP Top 10
├── main.go                   # Bootstrap: config, DB, rutas, graceful shutdown
├── main_test.go               # Tests de integración HTTP (app completa vía app.Test())
├── config/                   # config.go + config_test.go
├── models/                   # bucket.go, file.go, api_key.go (+ api_key_test.go)
├── handlers/                 # buckets.go, upload.go, download.go, auth.go
├── middleware/                # auth.go (API key / master key), logger.go
├── db/                        # postgres.go (pool+ping), migrations.go, repository.go (+ repository_test.go)
├── storage/                  # local_storage.go (+ local_storage_test.go)
├── utils/                    # token.go, validators.go, apikey.go (+ *_test.go)
└── cloudflared/config.yml.example
```

## Modelo de permisos

Cada **API key** pertenece a un `user_id` (string libre: username, email, lo
que uses para identificar al dueño) y tiene una lista de permisos
(`read`, `write`, `admin`, separados por coma):

- `read`: listar buckets propios, listar archivos, generar presigned URLs.
- `write`: crear/eliminar buckets propios, subir/eliminar archivos.
- `admin`: además de lo anterior, permite operar sobre buckets de **otros**
  usuarios (útil para una key de "sistema"/backend propio).

Los **buckets** y **archivos** individuales tienen un flag `is_public`. Un
archivo público se puede descargar con `GET /api/:bucket/:filename` sin
ninguna API key; uno privado exige una key con acceso al bucket.

La creación/listado/revocación de API keys (`/api/auth/keys/*`) está
protegida aparte por una **master key** (`ADMIN_MASTER_KEY`), no por API
keys normales — es el mecanismo de bootstrap para emitir las primeras keys.

## Levantar en local

```bash
cp .env.example .env
./scripts/generate-secrets.sh   # genera secrets/{db_password,signing_secret,admin_master_key}.txt
docker compose up -d --build
curl http://localhost:8080/health
```

Los tres secretos (`DB_PASSWORD`, `SIGNING_SECRET`, `ADMIN_MASTER_KEY`) **no
van en `.env`**: viven en archivos bajo `secrets/` (gitignored) que
docker-compose monta como [Docker
secrets](https://docs.docker.com/compose/how-tos/use-secrets/), para que
nunca queden en texto plano en `docker inspect` ni en `docker compose
config` (a diferencia de ponerlos en `environment:`, donde sí quedan
expuestos a cualquiera con acceso al Docker daemon del host). Ver la sección
de Seguridad más abajo para el detalle completo.

## Tests

Hay dos capas de tests:

- **Unitarios** (`utils/`, `models/`, `config/`, `storage/`): sin dependencias
  externas, corren siempre.
- **De integración** (`db/repository_test.go`, `main_test.go`): ejercitan la
  app completa (handlers + middlewares + rutas reales vía `app.Test()` de
  Fiber, o la capa SQL directamente) contra una PostgreSQL real — no hay
  mocks de la base, porque lo que hay que validar son las queries, los
  constraints y el flujo HTTP end-to-end tal cual lo usa un cliente real.
  Requieren la variable `TEST_DATABASE_URL`; si no está seteada, se saltean
  automáticamente (`go test` da `PASS`/`SKIP`, no falla).

Levantar una Postgres descartable para los tests:

```bash
docker run --rm -d --name storage-test-db \
  -e POSTGRES_PASSWORD=test -e POSTGRES_DB=storage_test \
  -p 127.0.0.1:5433:5432 postgres:15-alpine
```

Correr toda la suite (unitarios + integración):

```bash
export TEST_DATABASE_URL="postgres://postgres:test@localhost:5433/storage_test?sslmode=disable"
go test ./... -v
```

Sólo los unitarios (sin Postgres):

```bash
go test ./utils/... ./models/... ./config/... ./storage/...
```

Al terminar:

```bash
docker stop storage-test-db
```

## Ejemplos de uso de la API

Todas las respuestas son JSON. Los ejemplos asumen `API_BASE=http://localhost:8080`
y que ya generaste los secretos:

```bash
export API_BASE=http://localhost:8080
export ADMIN_MASTER_KEY=$(cat secrets/admin_master_key.txt)
```

### 1. Generar una API key (requiere master key)

```bash
curl -X POST $API_BASE/api/auth/keys \
  -H "X-Master-Key: $ADMIN_MASTER_KEY" \
  -H "Content-Type: application/json" \
  -d '{"user_id": "mi-app", "permissions": "read,write"}'
```

```json
{
  "id": "f350e8f7-...",
  "user_id": "mi-app",
  "key": "sk_f350e8f7-..._8Aqv8cYw-...",
  "permissions": "read,write",
  "created_at": "2026-09-21T03:21:29Z"
}
```

⚠️ El campo `key` sólo se muestra una vez: guardalo, en la base sólo se
persiste su hash bcrypt. A partir de acá, usalo como `X-API-Key: <key>` (o
`Authorization: Bearer <key>`) en el resto de las requests.

```bash
export API_KEY="sk_f350e8f7-..._8Aqv8cYw-..."
```

Listar / revocar keys (también con master key):

```bash
curl $API_BASE/api/auth/keys -H "X-Master-Key: $ADMIN_MASTER_KEY"
curl -X DELETE $API_BASE/api/auth/keys/<key_id> -H "X-Master-Key: $ADMIN_MASTER_KEY"
```

### 2. Crear un bucket

```bash
curl -X POST $API_BASE/api/buckets \
  -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" \
  -d '{"name": "avatares", "is_public": false}'
```

Nombres válidos: 3-63 caracteres, minúsculas/dígitos/guiones (estilo DNS,
igual que S3).

```bash
curl $API_BASE/api/buckets -H "X-API-Key: $API_KEY"
curl -X DELETE $API_BASE/api/buckets/<bucket_id> -H "X-API-Key: $API_KEY"
```

### 3. Subir un archivo

```bash
curl -X POST $API_BASE/api/upload/avatares \
  -H "X-API-Key: $API_KEY" \
  -F "file=@./foto.jpg" \
  -F "public=false" \
  -F 'metadata={"user_id":"123","tag":"perfil"}'
```

```json
{
  "file": {
    "id": "6eaeb809-...",
    "bucket_id": "1658beb3-...",
    "filename": "6bf7dc91-....jpg",
    "original_name": "foto.jpg",
    "mime_type": "image/jpeg",
    "size": 234981,
    "is_public": false,
    "metadata": { "user_id": "123", "tag": "perfil" }
  },
  "download_url": "/api/avatares/6bf7dc91-....jpg"
}
```

Tipos MIME permitidos: imágenes (`jpeg`, `png`, `gif`, `webp`, `svg+xml`,
`avif`, `heic`) y videos (`mp4`, `mpeg`, `webm`, `quicktime`, `x-msvideo`,
`x-matroska`). Tamaño máximo configurable vía `MAX_FILE_SIZE_MB` (default
500MB). El nombre físico se genera como UUID para evitar colisiones y path
traversal; el nombre original se conserva en `original_name`.

### 4. Listar archivos de un bucket (paginado)

```bash
curl "$API_BASE/api/files/avatares?page=1&page_size=20" -H "X-API-Key: $API_KEY"
```

```json
{
  "files": [ ... ],
  "page": 1,
  "page_size": 20,
  "total_count": 1,
  "total_pages": 1
}
```

### 5. Descargar un archivo

```bash
# Privado: requiere la API key del dueño (o una key admin)
curl "$API_BASE/api/avatares/6bf7dc91-....jpg" -H "X-API-Key: $API_KEY" -o foto.jpg

# Público: sin ninguna key
curl "$API_BASE/api/avatares/6bf7dc91-....jpg" -o foto.jpg
```

### 6. Generar un link de descarga presignado (temporal)

```bash
curl -X POST "$API_BASE/api/presign/avatares/6bf7dc91-....jpg" -H "X-API-Key: $API_KEY"
```

```json
{
  "token": "bXktdGVzdC1i...",
  "url": "/api/download/bXktdGVzdC1i...",
  "expires_in": 900
}
```

El token es autocontenido (bucket + archivo + expiración, firmado con
HMAC-SHA256 contra `SIGNING_SECRET`), no requiere estado en base de datos, y
puede compartirse públicamente: cualquiera con el link puede descargar el
archivo hasta que expire (`TOKEN_EXPIRY_MINUTES`, default 15 min).

```bash
curl "$API_BASE/api/download/bXktdGVzdC1i..." -o foto.jpg
```

### 7. Eliminar un archivo

```bash
curl -X DELETE "$API_BASE/api/avatares/6bf7dc91-....jpg" -H "X-API-Key: $API_KEY"
```

### Health check

```bash
curl $API_BASE/health   # {"status":"ok"}
```

## Seguridad implementada

> Ver [`SECURITY.md`](SECURITY.md) para el detalle completo mapeado contra
> cada categoría del OWASP Top 10 2021, incluyendo qué se mitiga, cómo, y qué
> riesgos quedan aceptados/fuera de alcance a propósito.

- **MIME whitelist**: sólo imágenes/videos declarados explícitamente.
- **Content sniffing en upload**: se rechaza contenido HTML/script real
  aunque el `Content-Type` declarado sea de imagen/video (ver SECURITY.md,
  A03).
- **Extensión física derivada del MIME validado**, nunca del nombre de
  archivo del cliente.
- **Headers de seguridad globales** (`nosniff`, CSP, `X-Frame-Options`,
  etc.) vía `helmet`, con `Cross-Origin-Resource-Policy` explícitamente
  abierto para permitir el uso desde un frontend en otro origen.
- **Límite de tamaño** configurable (`MAX_FILE_SIZE_MB`), rechazado antes de
  escribir a disco (Fiber `BodyLimit`) y también validado por handler.
- **Sanitización de nombres**: se descarta cualquier componente de path del
  nombre original y se genera un nombre físico UUID; imposible hacer path
  traversal.
- **API keys con bcrypt**: formato `sk_<id>_<secret>` — el `id` (no
  sensible) permite lookup O(1) en base, el `secret` es lo único hasheado
  con bcrypt y comparado en tiempo constante vía `bcrypt.CompareHashAndPassword`.
- **Tokens presignados con HMAC-SHA256** y expiración, validados en tiempo
  constante (`crypto/subtle`).
- **CORS configurable** por `CORS_ORIGIN` (lista separada por coma, o `*`).
- **Rate limiting en dos niveles** (ver detalle más abajo): uno general sobre
  toda `/api/*` y uno adicional, más estricto, sólo para uploads.
- **Aislamiento de permisos** dueño/admin por bucket y por archivo
  (público/privado independientes).
- **IP real detrás de Cloudflare Tunnel**: la app confía en el header
  `CF-Connecting-IP` (que Cloudflare siempre setea con la IP real del
  visitante) para `c.IP()`, logs y rate limiting — sin esto, todas las
  requests llegarían con la IP interna de Docker/loopback.

### Rate limiting

Dos limiters independientes, configurables por entorno (ver
[`.env.example`](.env.example)):

| Alcance                    | Variables                                            | Default        | Clave                                    |
|-----------------------------|-------------------------------------------------------|----------------|-------------------------------------------|
| Toda `/api/*`                | `API_RATE_LIMIT_MAX`, `API_RATE_LIMIT_WINDOW_SECONDS`  | 300 req / 60s  | `user_id` de la API key si está autenticada, sino IP |
| Sólo `/api/upload/:bucket`  | `UPLOAD_RATE_LIMIT_MAX`, `UPLOAD_RATE_LIMIT_WINDOW_SECONDS` | 30 req / 60s | ídem |

Al superar el límite, la API responde `429 Too Many Requests`. Todas las
respuestas de `/api/*` incluyen los headers `X-RateLimit-Limit`,
`X-RateLimit-Remaining` y `X-RateLimit-Reset` (expuestos también vía CORS
para que un frontend pueda leerlos con `fetch`).

Si el tráfico llega **proxyado por el backend de un frontend** (ver sección
siguiente), todos los usuarios finales comparten las IPs salientes de ese
proxy (p.ej. las de las funciones serverless de Vercel, que rotan y no son
fijas), así que la clave por `user_id` de la API key es la que realmente
importa en ese caso — subí `API_RATE_LIMIT_MAX` si vas a servir tráfico real
de muchos usuarios a través de una única API key compartida por el backend.

## Uso desde un frontend Next.js en Vercel

Arquitectura objetivo: **este servicio en Go corre únicamente en la VM Rocky
Linux**, expuesto vía Cloudflare Tunnel bajo tu dominio (ej.
`https://storage.tu-dominio.com`); el frontend Next.js vive en Vercel, un
origen distinto. Hay dos formas de conectarlos:

### Opción recomendada: proxy server-side (la API key nunca llega al browser)

El browser del usuario NUNCA debe ver la API key: si la ponés en una env var
`NEXT_PUBLIC_*` o la mandás en un `fetch` desde un Client Component, queda
visible en el bundle/DevTools de cualquier visitante, que podría usarla para
subir/borrar archivos directamente. En vez de eso, el server de Next.js
(Route Handlers o Server Actions, que corren en las funciones de Vercel, no
en el browser) guarda la key en una env var **sin** el prefijo `NEXT_PUBLIC_`
y hace de proxy:

```
Browser ──(same-origin, sin API key)──> Next.js (Vercel) ──(server-to-server, con API key)──> Go storage (Cloudflare Tunnel)
```

Con este approach **no hace falta configurar CORS_ORIGIN más allá del
default**, porque el browser sólo habla con su propio origen (Next.js); la
llamada de Next.js al servicio Go es server-to-server y los navegadores no
aplican CORS ahí.

`.env.local` / variables de entorno del proyecto en Vercel:

```bash
STORAGE_API_URL=https://storage.tu-dominio.com
STORAGE_API_KEY=sk_...   # generada con POST /api/auth/keys, permisos read,write
```

Route Handler que proxya listado y upload (`app/api/storage/[bucket]/route.ts`):

```typescript
import { NextRequest, NextResponse } from "next/server";

const STORAGE_API_URL = process.env.STORAGE_API_URL!;
const STORAGE_API_KEY = process.env.STORAGE_API_KEY!;

export async function GET(
  req: NextRequest,
  { params }: { params: { bucket: string } },
) {
  const qs = req.nextUrl.search; // reenvía ?page=&page_size=
  const res = await fetch(`${STORAGE_API_URL}/api/files/${params.bucket}${qs}`, {
    headers: { "X-API-Key": STORAGE_API_KEY },
    cache: "no-store",
  });
  return new NextResponse(res.body, { status: res.status, headers: res.headers });
}

export async function POST(
  req: NextRequest,
  { params }: { params: { bucket: string } },
) {
  // Reenvía el multipart/form-data tal cual llegó del browser.
  const res = await fetch(`${STORAGE_API_URL}/api/upload/${params.bucket}`, {
    method: "POST",
    headers: { "X-API-Key": STORAGE_API_KEY },
    body: req.body,
    duplex: "half",
  } as RequestInit);
  return new NextResponse(res.body, { status: res.status, headers: res.headers });
}
```

Desde un Client Component, el fetch va a tu propio dominio (`/api/storage/...`),
nunca a la VM directamente:

```typescript
"use client";

async function upload(bucket: string, file: File) {
  const form = new FormData();
  form.append("file", file);
  const res = await fetch(`/api/storage/${bucket}`, { method: "POST", body: form });
  return res.json();
}
```

Para archivos **públicos** podés directamente renderizar
`https://storage.tu-dominio.com/api/<bucket>/<filename>` en un `<img src>` o
`<video src>` sin pasar por Next.js (no requiere API key ni CORS, es una
descarga GET normal); para links temporales, generá el presign server-side
(con la API key) y devolvé sólo la URL firmada al browser.

### Alternativa: browser llamando directo a la VM

Si preferís que el browser hable directo con `storage.tu-dominio.com`
(por ejemplo para subir archivos grandes sin pasar por el límite de tamaño
de body de las funciones de Vercel), necesitás:

1. Configurar `CORS_ORIGIN` con el/los dominios exactos del frontend:
   ```bash
   CORS_ORIGIN=https://tu-app.vercel.app,https://tu-dominio-custom.com
   ```
   Los preview deployments de Vercel usan subdominios aleatorios de
   `vercel.app` compartidos por todos los proyectos de la plataforma —
   `CORS_ORIGIN=https://*.vercel.app` los cubriría a todos (soporte de
   wildcard de subdominio), pero también dejaría pedir con CORS a **cualquier
   otro proyecto** hosteado en Vercel, no sólo el tuyo. Para preview
   deployments preferí fijar un dominio propio estable en la config de
   dominios de tu proyecto de Vercel, o agregar temporalmente la URL de
   preview puntual mientras testeás.
2. Nunca poner la API key en código que corre en el browser. En este
   esquema, la única forma segura de que el browser suba/liste archivos sin
   exponer una key de escritura permanente es emitir, desde tu backend, una
   API key de vida corta por sesión (usando el mismo mecanismo de
   `POST /api/auth/keys`, revocándola después con `DELETE
   /api/auth/keys/:key_id`), o limitarse a descargas de archivos públicos y
   links presignados generados server-side (que sí son seguros de exponer,
   ya que expiran solos).

## Variables de entorno

Ver [`.env.example`](.env.example) para la lista completa (config no
sensible) y sus defaults. Los tres valores obligatorios y sensibles --
`DB_PASSWORD`, `SIGNING_SECRET`, `ADMIN_MASTER_KEY` -- **no van en `.env`**:
se generan con `./scripts/generate-secrets.sh` como archivos en `secrets/` y
docker-compose los monta como Docker secrets (ver "Levantar en local" más
arriba y la sección de Seguridad).

Si corrés el binario directo sin Docker (`go run .`, desarrollo local), esos
mismos tres valores se pueden seguir seteando como variables de entorno
planas (`DB_PASSWORD=...`) o apuntando a un archivo con
`DB_PASSWORD_FILE=/ruta/al/archivo` -- `config.Load()` soporta ambas formas.

## Deploy en Rocky Linux Minimal + Cloudflare Tunnel

### 1. Preparar el servidor

```bash
sudo dnf update -y
sudo dnf install -y git curl firewalld
sudo systemctl enable --now firewalld
```

### 2. Instalar Docker

```bash
sudo dnf install -y dnf-plugins-core
sudo dnf config-manager --add-repo https://download.docker.com/linux/rhel/docker-ce.repo
sudo dnf install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
sudo systemctl enable --now docker
sudo usermod -aG docker "$USER"   # cerrar sesión y volver a entrar para aplicar
```

### 3. Clonar y configurar el proyecto

```bash
git clone <tu-repo> gabicho-storage
cd gabicho-storage
cp .env.example .env
chmod +x scripts/generate-secrets.sh
./scripts/generate-secrets.sh
```

Esto crea `secrets/{db_password,signing_secret,admin_master_key}.txt` con
valores aleatorios de 256 bits (`openssl rand -hex 32`), permisos `600` y
sin pisar archivos existentes. **Nunca se commitean** (están en
`.gitignore`); si el servidor se reconstruye desde cero, hay que volver a
generarlos o restaurarlos desde un backup seguro.

### 4. Levantar el stack

```bash
docker compose up -d --build
docker compose ps          # ambos servicios deben quedar "healthy"
curl http://localhost:8080/health
```

Notar que en `docker-compose.yml` el puerto 8080 sólo se publica en
`127.0.0.1`, y Postgres **no** se expone al host: todo el tráfico externo
entra exclusivamente vía Cloudflare Tunnel, sin abrir puertos en el
firewall del servidor.

### 5. Instalar y configurar Cloudflare Tunnel

```bash
curl -L --output cloudflared.rpm \
  https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-x86_64.rpm
sudo dnf install -y ./cloudflared.rpm

cloudflared tunnel login
cloudflared tunnel create mi-storage
```

Copiar `cloudflared/config.yml.example` a `~/.cloudflared/config.yml` (o
`/etc/cloudflared/config.yml` si se instala como servicio system-wide) y
completar `<TUNNEL_ID>` y el hostname:

```yaml
tunnel: <TUNNEL_ID>
credentials-file: /root/.cloudflared/<TUNNEL_ID>.json

ingress:
  - hostname: storage.tu-dominio.com
    service: http://localhost:8080
  - service: http_status:404
```

```bash
cloudflared tunnel route dns mi-storage storage.tu-dominio.com
sudo cloudflared service install
sudo systemctl enable --now cloudflared
```

Cloudflare emite y renueva el certificado TLS automáticamente para el
hostname — no hace falta configurar SSL/TLS en el servidor ni en Fiber.

### 6. Verificar

```bash
curl https://storage.tu-dominio.com/health
```

### 7. Monitoreo de logs

```bash
docker compose logs -f storage          # logs de la API (JSON estructurado)
docker compose logs -f postgres         # logs de PostgreSQL
sudo journalctl -u cloudflared -f       # logs del tunnel
```

Los logs de la API son JSON (`log/slog`) con `method`, `path`, `status`,
`latency`, `ip` y `user_id` (cuando hay una API key autenticada), pensados
para ingestarse fácilmente en cualquier stack de logging.

### 8. Actualizar el servicio

```bash
cd gabicho-storage
git pull
docker compose up -d --build
```

### Backups

Los datos viven en dos volúmenes de Docker: `postgres_data` (metadata) y
`storage_data` (archivos). Para backupear:

```bash
docker compose exec postgres pg_dump -U storage storage > backup-$(date +%F).sql
docker run --rm -v gabicho-storage_storage_data:/data -v "$PWD":/backup \
  alpine tar czf /backup/files-$(date +%F).tar.gz -C /data .
```
