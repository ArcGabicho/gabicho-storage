# Infraestructura y despliegue

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
├── docker/
│   ├── docker-compose.yml      # Stack: API + SQL Server (contexto de build = raíz del repo)
│   ├── Dockerfile               # Build multi-stage (SDK -> runtime ASP.NET)
│   └── database/                # Wrapper de la imagen oficial de SQL Server
│       ├── Dockerfile           #   con soporte de Docker secrets (_FILE)
│       └── entrypoint-secrets.sh
├── .env.example                 # Config no sensible
├── SECURITY.md                  # Mapeo detallado contra OWASP Top 10
├── core/
│   ├── Program.cs               # Bootstrap: opciones, DB, middlewares, rutas
│   ├── Options/                 # StorageOptions (config + secretos), naming policy JSON
│   ├── Entities/                 # Bucket, FileObject, ApiKey (+ EF Core config en Data/)
│   ├── Data/                     # AppDbContext, migraciones EF Core
│   ├── Dtos/                     # Request/response records
│   ├── Services/                 # TokenSigner, ApiKeyGenerator, MimeValidator,
│   │                             #   FilenameSanitizer, LocalFileStorage, ApiKeyAuthenticator
│   ├── Auth/                     # Filtros de autenticación/autorización (API key, master key, permisos)
│   ├── Middleware/               # Security headers, IP real detrás de Cloudflare, logging
│   └── Controllers/              # BucketsController, FilesController, AuthController
├── test/
│   ├── Unit/                     # Sin dependencias externas
│   └── Integration/              # WebApplicationFactory contra SQL Server real
└── scripts/generate-secrets.sh
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
docker compose -f docker/docker-compose.yml up -d --build
docker compose -f docker/docker-compose.yml ps   # ambos servicios deben quedar "healthy"
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
docker compose -f docker/docker-compose.yml logs -f api          # logs JSON estructurados
docker compose -f docker/docker-compose.yml logs -f sqlserver
sudo journalctl -u cloudflared -f

# actualizar:
git pull && docker compose -f docker/docker-compose.yml up -d --build
```

### Backups

```bash
docker run --rm -v gabicho-storage_mssql_data:/data -v "$PWD":/backup \
  alpine tar czf /backup/mssql-$(date +%F).tar.gz -C /data .
docker run --rm -v gabicho-storage_storage_data:/data -v "$PWD":/backup \
  alpine tar czf /backup/files-$(date +%F).tar.gz -C /data .
```
