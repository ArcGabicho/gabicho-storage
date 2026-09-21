# Seguridad y cumplimiento OWASP Top 10 (2021)

Mitigaciones implementadas mediante los mecanismos concretos de ASP.NET
Core/EF Core/SQL Server.

## A01:2021 — Broken Access Control

- Cada bucket tiene un `Owner` (el `UserId` de la API key que lo creó).
  Las operaciones de escritura verifican `bucket.Owner == key.UserId`, salvo
  permiso `admin`. Ver `Controllers/BucketsController.cs`,
  `Controllers/FilesController.cs`.
- Los permisos se validan en `Auth/RequirePermissionAttribute.cs`, un
  `IAsyncAuthorizationFilter` con `Order` explícito para garantizar que
  corre DESPUÉS de `ApiKeyAuthAttribute` (que puebla
  `HttpContext.GetApiKey()`) -- ambos declarados como atributos en la
  definición de cada endpoint, no opt-in manual dentro del handler.
- Archivos/buckets privados exigen una API key con acceso; los públicos no
  requieren autenticación, por diseño (`FilesController.DownloadFile`).
- **Riesgo aceptado**: los links presignados (`/api/download/{token}`) son
  stateless (HMAC, sin fila en base) -- válidos hasta su expiración aunque
  la key que los generó se revoque después. Default 15 min
  (`TOKEN_EXPIRY_MINUTES`).

## A02:2021 — Cryptographic Failures

- Las API keys nunca se persisten en texto plano: sólo el hash bcrypt del
  secreto (`BCrypt.Net-Next`), nunca el id. Formato `sk_<guid>_<secret>`.
- La comparación de la master key usa
  `CryptographicOperations.FixedTimeEquals` (`Auth/MasterKeyAuthAttribute.cs`),
  para no filtrar nada por timing side-channel.
- Los tokens presignados usan `HMACSHA256` (`Services/TokenSigner.cs`) y se
  validan con `CryptographicOperations.FixedTimeEquals`, no con `==`.
- TLS lo termina Cloudflare en el borde (Cloudflare Tunnel).
- **Riesgo aceptado**: no hay rotación automática de
  `SIGNING_SECRET`/`ADMIN_MASTER_KEY`.

## A03:2021 — Injection

- **SQL**: 100% vía Entity Framework Core (LINQ + parámetros), cero SQL
  crudo concatenado con input de usuario en todo el proyecto.
- **Path traversal**: el nombre físico en disco es siempre un GUID generado
  server-side (`Services/LocalFileStorage.cs`); la extensión sale de una
  tabla fija indexada por el Content-Type ya validado
  (`Services/MimeValidator.ExtensionForMimeType`), nunca del nombre que
  mandó el cliente. `Services/FilenameSanitizer.cs` sanitiza el nombre
  "original" (sólo metadata para mostrar).
- **XSS almacenado vía upload disfrazado**: mitigado en dos capas:
  1. **Content sniffing en upload**: se leen los primeros 512 bytes reales
     y se rechaza (`415`) si el contenido es HTML
     (`MimeValidator.LooksLikeHtml`), aunque el `Content-Type` declarado
     esté en la whitelist.
  2. **CSP + sandbox al servir**: toda respuesta lleva
     `Content-Security-Policy: default-src 'none'; sandbox` y
     `X-Content-Type-Options: nosniff`
     (`Middleware/SecurityHeadersMiddleware.cs`), neutralizando la
     ejecución de script aunque el contenido sea un SVG legítimo con
     `<script>` embebido.
- **Riesgo aceptado**: no hay validación completa de magic bytes por
  formato, sólo el descarte de HTML disfrazado.

## A04:2021 — Insecure Design

- Whitelist explícita de tipos MIME, no blacklist.
- Límite de tamaño configurable (`MAX_FILE_SIZE_MB`) en dos capas: Kestrel
  (`KestrelServerOptions.Limits.MaxRequestBodySize`, con 1MB de margen sobre
  el límite real para que el 413 con JSON del propio endpoint sea el camino
  normal en vez de un connection reset) y `FormOptions.MultipartBodyLengthLimit`.
- Rate limiting en dos niveles vía `Microsoft.AspNetCore.RateLimiting`
  (nativo de ASP.NET Core, sin dependencias externas): política `"api"`
  sobre todo `/api/*` y `"upload"` más estricta sólo en
  `/api/upload/{bucket}` -- el atributo `[EnableRateLimiting]` a nivel
  action reemplaza al de nivel controller/grupo, así que en upload aplica
  únicamente la política estricta (que de todos modos es más restrictiva
  que la general).
- Emisión de API keys centralizada detrás de master key.
- **Riesgo aceptado**: sin antivirus/malware scanning de archivos subidos.

## A05:2021 — Security Misconfiguration

- Headers de seguridad globales (`Middleware/SecurityHeadersMiddleware.cs`):
  `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`,
  `Content-Security-Policy`, `Referrer-Policy: no-referrer`,
  `Cross-Origin-Resource-Policy: cross-origin` (explícitamente permisivo
  para permitir que un frontend en otro origen embeba imágenes/videos).
- CORS restringido a los orígenes de `CORS_ORIGIN` vía
  `Microsoft.AspNetCore.Cors`.
- Sin credenciales hardcodeadas: `StorageOptions.Load()` lanza
  `InvalidOperationException` si `DB_PASSWORD`/`SIGNING_SECRET`/`ADMIN_MASTER_KEY`
  faltan -- el servicio ni arranca.
- **Credenciales fuera de `docker inspect`/`docker compose config`**: los
  tres secretos se pasan como Docker secrets basados en archivo, montados
  en `/run/secrets/<nombre>`. `StorageOptions.GetSecret()` soporta la
  convención `<VAR>_FILE`. La imagen oficial de SQL Server NO soporta esto
  nativamente (a diferencia de Postgres/MySQL) -- `docker/database/entrypoint-secrets.sh`
  se lo agrega mediante un wrapper que exporta `MSSQL_SA_PASSWORD` desde
  `MSSQL_SA_PASSWORD_FILE` antes de invocar el entrypoint real de la imagen.
- El contenedor de la API corre como el usuario no-root que ya trae la
  imagen oficial `mcr.microsoft.com/dotnet/aspnet` (`app`, uid 1654).
- SQL Server no se expone al host; la API sólo se publica en `127.0.0.1`.
- Mensajes de error genéricos al cliente: `app.UseExceptionHandler(...)` en
  `Program.cs` devuelve siempre `{"message":"error interno del servidor"}`
  en cualquier excepción no manejada, nunca el detalle crudo -- que sí va a
  los logs del servidor.
- **Riesgo aceptado**: `db_password` se genera con `openssl rand -base64
  32` en vez de `-hex` (encontrado durante el desarrollo: SQL Server exige
  password con caracteres de al menos 3 de 4 clases -- mayúsculas,
  minúsculas, dígitos, símbolos -- y un hex puro sólo cubre 2, el contenedor
  no arrancaba).

## A06:2021 — Vulnerable and Outdated Components

- Dependencias fijadas por versión exacta en los `.csproj`.
- Imágenes base oficiales de Microsoft (`mcr.microsoft.com/dotnet/aspnet`,
  `mcr.microsoft.com/dotnet/sdk`, `mcr.microsoft.com/mssql/server`), con
  tag de minor version, no `:latest`.
- **Recomendación operativa**: correr `dotnet list package --vulnerable
  --include-transitive` periódicamente o en CI, y `docker compose build
  --pull` para traer parches de las imágenes base.

## A07:2021 — Identification and Authentication Failures

- API key formato `sk_<guid>_<secret>`: el guid permite lookup O(1) sin
  comparar bcrypt contra todas las keys existentes; el secreto (256 bits de
  entropía) es lo único hasheado y verificado con bcrypt.
- Un id que no parsea como GUID válido (`ApiKeyGenerator.ParseId`) se
  rechaza antes de tocar la base, devolviendo 401/404 en vez de que EF Core
  lance una excepción de conversión que terminaría en un 500.
- Emisión/listado/revocación de API keys detrás de master key separada.
- Rate limiting general sobre `/api/*` (incluye los endpoints de auth)
  dificulta fuerza bruta.
- **Riesgo aceptado**: sin expiración automática de API keys.

## A08:2021 — Software and Data Integrity Failures

- Sin deserialización de datos no confiables más allá de `System.Text.Json`
  contra tipos conocidos (records/DTOs), salvo el campo `metadata`, que se
  persiste como JSON de vuelta sin ejecutarse ni interpretarse como código.
- Build multi-stage desde código fuente propio, sin binarios
  pre-compilados de terceros.
- **Riesgo aceptado**: los tags de imagen base no están fijados por
  digest.

## A09:2021 — Security Logging and Monitoring Failures

- Todas las requests se loguean estructuradas en JSON
  (`Middleware/RequestLoggingMiddleware.cs`, formatter `AddJsonConsole`):
  método, path, status, latencia, IP real, `user_id` si hay API key
  autenticada. El status logueado se toma de `context.Response.StatusCode`
  DESPUÉS de que `next()` retorna -- en ASP.NET Core esto sí refleja el
  código real incluso cuando un filtro de autorización cortó la request.
- Acciones administrativas sensibles generan una línea de log explícita:
  creación de API key (`AuthController.Create`), revocación
  (`AuthController.Revoke`), eliminación de bucket
  (`BucketsController.Delete`).
- Intentos fallidos de autenticación con la master key generan un `Warning`
  explícito con la IP de origen (`MasterKeyAuthAttribute`).
- **Riesgo aceptado**: sin alerting activo; los logs van a stdout del
  contenedor, para producción real conviene enviarlos a un agregador.

## A10:2021 — Server-Side Request Forgery (SSRF)

- El servicio no hace ningún request saliente basado en input del usuario.
  No aplica.

---

## Hallazgos encontrados durante esta migración

| # | Hallazgo | Fix |
|---|----------|-----|
| 1 | El rate limiter de ASP.NET Core devuelve `503` por default, no `429` | `RateLimiterOptions.RejectionStatusCode = StatusCodes.Status429TooManyRequests` |
| 2 | `InvariantGlobalization=true` rompe `Microsoft.Data.SqlClient` (`NotSupportedException` al abrir conexión) | No activar esa propiedad en el `.csproj` |
| 3 | `openssl rand -hex 32` no cumple la política de complejidad de password de SQL Server (sólo 2 de 4 clases de caracteres) | `openssl rand -base64 32` para `db_password` específicamente |
| 4 | La imagen oficial de SQL Server no soporta `_FILE` para el password de `sa` | Wrapper de entrypoint (`docker/database/entrypoint-secrets.sh`) que lo agrega |
| 5 | `IClassFixture<T>` de xUnit exige un único constructor público y no considera defaults de C# al resolver argumentos | Fixture con constructor parameterless + propiedades mutables para customización |
| 6 | Tests que mutan variables de entorno del proceso se pisaban entre sí al correr en paralelo (xUnit paraleliza test classes por default) | Todos agrupados en una `[CollectionDefinition(DisableParallelization = true)]` compartida |
| 7 | El constructor de una clase de test llamaba `CreateClient()` (dispara el arranque completo del host) antes de que `Skip.IfNot()` pudiera evaluarse, rompiendo el skip limpio sin DB de test configurada | Diferir `CreateClient()` detrás del mismo chequeo de disponibilidad |

Todos verificados manualmente contra el stack real en Docker, no sólo en
tests (ver historial de la sesión de migración).
