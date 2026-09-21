# gabicho-storage (.NET)

Servicio de storage self-hosted en **ASP.NET Core Web API + Entity Framework
Core + SQL Server** como alternativa a Firebase Storage / Cloudflare R2:
buckets, upload/descarga de imágenes y videos, metadata en SQL Server,
autenticación con API keys y links de descarga presignados con expiración.

Es la migración funcionalmente equivalente de la versión original en Go
(Fiber + PostgreSQL): mismo modelo de datos, mismos endpoints, mismo
contrato JSON (snake_case) y las mismas propiedades de seguridad -- ver
[`SECURITY.md`](SECURITY.md).

## Stack

- .NET 10 + ASP.NET Core Web API (controllers)
- Entity Framework Core 10 + SQL Server (`Microsoft.EntityFrameworkCore.SqlServer`)
- Filesystem local organizado por buckets (contenido binario)
- Docker + Docker Compose, con Docker secrets basados en archivo
- Cloudflare Tunnel (exposición pública con TLS automático, sin abrir puertos)

## Arquitectura del proyecto

```
.
├── GabichoStorage.slnx
├── docker-compose.yml         # Stack: API + SQL Server
├── Dockerfile                  # Build multi-stage (SDK -> runtime ASP.NET)
├── db/                         # Wrapper de la imagen oficial de SQL Server
│   ├── Dockerfile              #   con soporte de Docker secrets (_FILE)
│   └── entrypoint-secrets.sh
├── .env.example                # Config no sensible
├── SECURITY.md                 # Mapeo detallado contra OWASP Top 10
├── src/GabichoStorage.Api/
│   ├── Program.cs              # Bootstrap: opciones, DB, middlewares, rutas
│   ├── Options/                # StorageOptions (config + secretos), naming policy JSON
│   ├── Entities/                # Bucket, FileObject, ApiKey (+ EF Core config en Data/)
│   ├── Data/                    # AppDbContext, migraciones EF Core
│   ├── Dtos/                    # Request/response records
│   ├── Services/                # TokenSigner, ApiKeyGenerator, MimeValidator,
│   │                            #   FilenameSanitizer, LocalFileStorage, ApiKeyAuthenticator
│   ├── Auth/                    # Filtros de autenticación/autorización (API key, master key, permisos)
│   ├── Middleware/              # Security headers, IP real detrás de Cloudflare, logging
│   └── Controllers/             # BucketsController, FilesController, AuthController
├── tests/GabichoStorage.Tests/
│   ├── Unit/                    # Sin dependencias externas
│   └── Integration/             # WebApplicationFactory contra SQL Server real
└── scripts/generate-secrets.sh
```

## Modelo de permisos

Idéntico al de la versión en Go: cada **API key** pertenece a un `user_id`
(string libre) y tiene permisos `read`/`write`/`admin` (comma-separated,
`admin` implica los otros dos). Los **buckets** y **archivos** tienen un
flag `is_public` independiente. La administración de API keys
(`/api/auth/keys/*`) está protegida por una **master key** separada
(`ADMIN_MASTER_KEY`), no por API keys normales.

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
config`. La imagen oficial de SQL Server no soporta esa convención
nativamente (a diferencia de Postgres/MySQL); `db/entrypoint-secrets.sh` se
la agrega.

> **Nota**: `db_password` (usado como password de `sa`) se genera con
> `openssl rand -base64 32`, no `-hex`: SQL Server exige que el password
> tenga caracteres de al menos 3 de 4 clases (mayúsculas, minúsculas,
> dígitos, símbolos), y un hex puro sólo cubre 2.

## Tests

```bash
# Sólo unitarios (sin SQL Server):
dotnet test --filter "FullyQualifiedName~Unit"

# Suite completa (requiere SQL Server real):
docker run --rm -d --name storage-test-mssql \
  -e ACCEPT_EULA=Y -e "MSSQL_SA_PASSWORD=Str0ng!Passw0rd123" \
  -p 127.0.0.1:1433:1433 mcr.microsoft.com/mssql/server:2022-latest

export TEST_DB_HOST=localhost TEST_DB_PORT=1433 TEST_DB_USER=sa TEST_DB_PASSWORD='Str0ng!Passw0rd123'
dotnet test

docker stop storage-test-mssql
```

Los tests de integración levantan la app completa (mismo `Program.cs`, vía
`WebApplicationFactory<Program>`) contra una base fresca (nombre random) en
esa instancia de SQL Server, y se saltean automáticamente
(`[SkippableFact]`) si `TEST_DB_HOST`/`TEST_DB_PASSWORD` no están seteadas.

## Ejemplos de uso de la API

Idénticos a la versión en Go (mismo contrato JSON snake_case):

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

Misma recomendación que la versión en Go: proxear las requests desde un
Route Handler de Next.js (server-side) para que la API key nunca llegue al
browser, ya que `Cross-Origin-Resource-Policy: cross-origin` está
habilitado explícitamente para permitir que un frontend en otro origen
embeba imágenes/videos públicos vía `<img>`/`<video>` directamente. Ver
[`SECURITY.md`](SECURITY.md) para el detalle de headers.

Configurar `CORS_ORIGIN` con el dominio real del frontend (`.env`, no es
sensible):

```bash
CORS_ORIGIN=https://mi-app.vercel.app,https://mi-dominio.com
```

## Deploy en Rocky Linux Minimal + Cloudflare Tunnel

### 1. Preparar el servidor e instalar Docker

```bash
sudo dnf update -y
sudo dnf install -y dnf-plugins-core git curl firewalld
sudo systemctl enable --now firewalld
sudo dnf config-manager --add-repo https://download.docker.com/linux/rhel/docker-ce.repo
sudo dnf install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
sudo systemctl enable --now docker
sudo usermod -aG docker "$USER"   # cerrar sesión y volver a entrar para aplicar
```

**Nota de recursos**: SQL Server recomienda al menos 2GB de RAM disponibles
para el contenedor; verificar que la VM lo soporte (`MSSQL_PID: Express` en
`docker-compose.yml` usa la edición gratuita, con límite de 10GB por base --
de sobra para la metadata de este servicio, ya que los archivos en sí viven
en el filesystem, no en la base).

### 2. Clonar, configurar y levantar

```bash
git clone <tu-repo> gabicho-storage
cd gabicho-storage
cp .env.example .env
chmod +x scripts/generate-secrets.sh
./scripts/generate-secrets.sh
docker compose up -d --build
docker compose ps          # ambos servicios deben quedar "healthy"
curl http://localhost:8080/health
```

El puerto 8080 sólo se publica en `127.0.0.1`, y SQL Server **no** se
expone al host: todo el tráfico externo entra exclusivamente vía Cloudflare
Tunnel.

### 3. Cloudflare Tunnel

```bash
curl -L --output cloudflared.rpm \
  https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-x86_64.rpm
sudo dnf install -y ./cloudflared.rpm

cloudflared tunnel login
cloudflared tunnel create mi-storage
```

`~/.cloudflared/config.yml`:

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
curl https://storage.tu-dominio.com/health
```

### 4. Monitoreo y actualización

```bash
docker compose logs -f api          # logs JSON estructurados
docker compose logs -f sqlserver
sudo journalctl -u cloudflared -f

# actualizar:
git pull && docker compose up -d --build
```

### Backups

```bash
docker run --rm -v gabicho-storage-dotnet_mssql_data:/data -v "$PWD":/backup \
  alpine tar czf /backup/mssql-$(date +%F).tar.gz -C /data .
docker run --rm -v gabicho-storage-dotnet_storage_data:/data -v "$PWD":/backup \
  alpine tar czf /backup/files-$(date +%F).tar.gz -C /data .
```
