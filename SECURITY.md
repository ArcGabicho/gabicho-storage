# Seguridad y cumplimiento OWASP Top 10 (2021)

Este documento mapea cada categoría del [OWASP Top 10 2021](https://owasp.org/Top10/)
contra lo implementado en este servicio, e indica explícitamente los riesgos
aceptados/fuera de alcance. "Cumplir con OWASP Top 10" no es un checkbox
binario -- es una lista de categorías de riesgo a mitigar de forma razonable
para el contexto del servicio. Esto es lo que se hizo para cada una.

## A01:2021 — Broken Access Control

- Cada bucket tiene un `owner` (el `user_id` de la API key que lo creó).
  Todas las operaciones de escritura (upload, delete de archivo/bucket)
  verifican `bucket.Owner == key.UserID`, salvo que la key tenga permiso
  `admin`. Ver [`handlers/buckets.go`](handlers/buckets.go),
  [`handlers/upload.go`](handlers/upload.go).
- Los permisos (`read` / `write` / `admin`) están asociados a la API key, no
  al request, y se validan en middleware
  ([`middleware.RequirePermission`](middleware/auth.go)) antes de llegar al
  handler -- no hay forma de alcanzar un handler sin pasar por esa
  verificación (está en la definición de rutas en `main.go`, no es opt-in
  por handler).
- Los archivos/buckets privados exigen una API key con acceso; los públicos
  no requieren autenticación en absoluto, por diseño (ver
  [`handlers/download.go`](handlers/download.go)).
- Las API keys se identifican por un `id` público + un `secret`
  independiente; conocer el `id` (que viaja en la propia URL/headers, no es
  secreto) no alcanza para autenticar.
- **Riesgo aceptado**: los links presignados (`/api/download/:token`) son
  stateless (HMAC, sin fila en base) por diseño -- son válidos hasta su
  expiración aunque la API key que los generó se revoque después, o el
  archivo pase a privado. La expiración default es de 15 minutos
  (`TOKEN_EXPIRY_MINUTES`); ajustarla según el caso de uso. No hay revocación
  individual de tokens ya emitidos.

## A02:2021 — Cryptographic Failures

- Las API keys nunca se persisten en texto plano: sólo se guarda el hash
  bcrypt del secreto (nunca el `id`, que no es sensible). La key completa se
  muestra una única vez, en la respuesta de creación.
- La comparación de la master key (`X-Master-Key`) usa
  `crypto/subtle.ConstantTimeCompare` en vez de `==`, para no filtrar nada
  por timing side-channel -- una comparación de string común en Go
  cortocircuita en el primer byte distinto, lo que en teoría permite adivinar
  un secreto byte a byte midiendo latencia. Ver
  [`middleware/auth.go`](middleware/auth.go).
- Los tokens presignados usan HMAC-SHA256 (`crypto/hmac` + `crypto/sha256`)
  con un secreto de servidor (`SIGNING_SECRET`) y se validan con
  `crypto/subtle.ConstantTimeCompare`, no con `==`. Ver
  [`utils/token.go`](utils/token.go).
- TLS/HTTPS lo termina Cloudflare en el borde (Cloudflare Tunnel): el
  tráfico entre el visitante y Cloudflare va cifrado con certificados que
  Cloudflare emite y renueva automáticamente. El tramo `cloudflared` ↔
  contenedor es loopback local en la misma VM, no sale a la red.
- **Riesgo aceptado**: no hay rotación automática de `SIGNING_SECRET` ni
  `ADMIN_MASTER_KEY`. Rotarlos manualmente invalida todos los tokens
  presignados en vuelo (aceptable, dado que son de vida corta) y no afecta a
  las API keys existentes (que no dependen de esos secretos).

## A03:2021 — Injection

- **SQL**: el 100% de las queries usa parámetros posicionales de pgx
  (`$1, $2, ...`); no hay una sola concatenación de input de usuario en un
  string SQL en todo el proyecto. Verificado explícitamente (no hay
  `fmt.Sprintf` ni `"..." + userInput` armando SQL en `db/repository.go`).
- **Path traversal**: los nombres de archivo del cliente nunca se usan tal
  cual para escribir a disco. `utils.SanitizeFilename` limita el nombre
  "original" (guardado sólo como metadata, para mostrar) a
  `[a-zA-Z0-9._-]`; el nombre físico real es siempre un UUID generado en el
  servidor (`storage.SaveUploadedFile`), y la extensión sale de una tabla
  fija indexada por el Content-Type ya validado
  (`utils.ExtensionForMimeType`), nunca del nombre que mandó el cliente. Los
  nombres de bucket están limitados a un patrón estilo DNS
  (`utils.IsValidBucketName`).
- **Header injection**: el único header que se arma con datos de usuario es
  `Content-Disposition` (con `file.OriginalName`), y ese valor ya pasó por
  `SanitizeFilename`, que no permite comillas, `\r`, `\n` ni ningún
  caracter fuera de `[a-zA-Z0-9._-]`.
- **XSS almacenado vía upload disfrazado**: un cliente puede declarar
  cualquier `Content-Type` para el archivo sin que tenga relación real con
  el contenido (dos campos independientes de un multipart). Se mitiga en dos
  capas:
  1. **Content sniffing en upload**: se leen los primeros 512 bytes reales
     del archivo y se rechaza (`415`) si el contenido sniffeado es HTML
     (`utils.LooksLikeHTML`, usando `http.DetectContentType`), aunque el
     `Content-Type` declarado esté en la whitelist. Cubre el caso clásico de
     subir un `.html` con script declarado como `image/png`.
  2. **CSP + sandbox en el serving**: toda respuesta (incluida la de
     archivos) lleva `Content-Security-Policy: default-src 'none'; sandbox`
     y `X-Content-Type-Options: nosniff` (vía
     [`helmet`](https://github.com/gofiber/fiber/tree/main/middleware/helmet)
     en `main.go`). Esto neutraliza la ejecución de script aunque el
     contenido sea un SVG legítimo con `<script>` embebido -- el sniffing de
     contenido no puede (ni debe) cubrir ese caso sin romper SVGs válidos,
     así que la defensa real ahí es el sandboxing al servir, igual que hace
     GitHub con `raw.githubusercontent.com`.
- **Riesgo aceptado**: no hay un validador de "magic bytes" completo por
  formato (sólo se descarta HTML disfrazado). Un archivo binario realmente
  corrupto/inválido pasa la validación de todos modos; el daño potencial
  queda acotado por el sandboxing de arriba, no por rechazo en upload.

## A04:2021 — Insecure Design

- Whitelist explícita de tipos MIME (imágenes y videos conocidos), no
  blacklist.
- Límite de tamaño configurable (`MAX_FILE_SIZE_MB`), validado tanto a nivel
  transporte (Fiber `BodyLimit`, con margen para que el 413 del handler sea
  el camino normal en vez de un connection reset -- ver el comentario en
  `main.go`) como en el handler (`utils.ValidateFileSize`).
- Rate limiting en dos niveles (general sobre `/api/*` + uno más estricto
  sólo para upload), clave por `user_id` de la API key cuando hay una
  autenticada. Ver sección de rate limiting en [`README.md`](README.md).
- Emisión de API keys centralizada detrás de una master key: no hay
  self-signup ni endpoint público que permita a un cliente crearse
  credenciales por su cuenta.
- **Riesgo aceptado, fuera de alcance**: no hay escaneo antivirus/malware de
  los archivos subidos. Para un despliegue con usuarios no confiables subiendo
  contenido, se recomienda integrar ClamAV u otro escáner delante o como
  paso async post-upload; no está implementado acá.
- **Riesgo aceptado**: los endpoints con body JSON (crear bucket, crear API
  key) comparten el mismo límite de tamaño de body que el endpoint de
  upload (~501MB) porque Fiber no soporta un `BodyLimit` distinto por ruta
  sin tocar la config del servidor fasthttp subyacente. El impacto real es
  bajo (memoria transitoria, acotada por ese techo y por el rate limiter),
  pero si se quiere un límite más chico específicamente para JSON, se puede
  imponer en el borde con una regla de Cloudflare (WAF / tamaño de body por
  path).

## A05:2021 — Security Misconfiguration

- Headers de seguridad globales via
  [`helmet`](https://github.com/gofiber/fiber/tree/main/middleware/helmet):
  `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`,
  `Content-Security-Policy`, `Referrer-Policy: no-referrer`,
  `Cross-Origin-Resource-Policy: cross-origin` (explícitamente permisivo acá
  porque el servicio está pensado para que un frontend en otro origen
  embeba imágenes/videos vía `<img>`/`<video>`; ver `main.go` para el
  razonamiento completo).
- CORS restringido a los orígenes configurados en `CORS_ORIGIN` (no `*` por
  default recomendado en producción).
- Nada de credenciales hardcodeadas: `DB_PASSWORD`, `SIGNING_SECRET` y
  `ADMIN_MASTER_KEY` son obligatorios sin default -- el servicio ni arranca
  si faltan (`config.Load()`).
- **Credenciales fuera de `docker inspect` / `docker compose config`**: los
  tres secretos se pasan como [Docker
  secrets](https://docs.docker.com/compose/how-tos/use-secrets/) basados en
  archivo (`docker-compose.yml`, bloque `secrets:`), montados de sólo
  lectura en `/run/secrets/<nombre>`, en vez de como `environment:` en texto
  plano. Se verificó explícitamente el problema que esto evita: con el
  password puesto directo en `environment:` (como estaba antes de este
  fix), tanto `docker inspect <container>` como `docker compose config`
  imprimen el valor en texto plano -- visible para cualquiera con acceso al
  socket de Docker del host, o si ese output termina pegado en un ticket de
  soporte o un log de CI. `config.Load()` soporta leer cada secreto desde
  `<VAR>_FILE` (la misma convención que usa la imagen oficial de Postgres
  con `POSTGRES_PASSWORD_FILE`), con fallback a la variable de entorno
  directa para desarrollo local sin Docker. Los archivos de secretos
  (`secrets/*.txt`) están en `.gitignore` y se generan con
  `scripts/generate-secrets.sh` (permisos `600`, no pisa archivos
  existentes).
- El contenedor de la API corre como usuario no-root (`USER app` en el
  `Dockerfile`).
- Postgres no se expone al host (sin `ports:` en `docker-compose.yml`); la
  API sólo se publica en `127.0.0.1`, nunca en `0.0.0.0` -- el único punto de
  entrada externo es Cloudflare Tunnel.
- Mensajes de error genéricos hacia el cliente en fallos internos (no se
  devuelven stack traces ni errores crudos de Postgres/filesystem); el
  detalle completo va sólo a los logs del servidor.
- **Riesgo aceptado**: `.env.example` trae `CORS_ORIGIN=*` como default
  cómodo para arrancar en desarrollo; en producción hay que fijarlo a los
  dominios reales (documentado en el README).

## A06:2021 — Vulnerable and Outdated Components

- Dependencias fijadas por `go.sum` (build reproducible, sin
  auto-actualización silenciosa).
- Imágenes base oficiales y con tag de minor version, no `:latest`:
  `golang:1.26-alpine` (build), `alpine:3.20` (runtime), `postgres:15-alpine`.
  El tag de minor (`1.26-alpine`, no un patch exacto) hace que cada rebuild
  traiga automáticamente los últimos parches de esa línea -- verificado: al
  momento de escribir esto, `golang:1.26-alpine` resuelve a `go1.26.8`.
- Se corrió [`govulncheck`](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)
  contra el módulo: cero vulnerabilidades alcanzables por el código en las
  dependencias directas o transitivas. Los únicos hallazgos fueron de la
  stdlib de una toolchain de Go vieja (`go1.26.0` exacto) usada sólo para
  pruebas locales puntuales, ya resueltos por el `go1.26.8` que efectivamente
  usa el build de Docker.
- **Recomendación operativa** (no automatizable desde el código): correr
  `docker compose build --pull` periódicamente para traer parches de las
  imágenes base, y `govulncheck ./...` en CI o antes de cada release.

## A07:2021 — Identification and Authentication Failures

- Autenticación por API key (`X-API-Key` o `Authorization: Bearer`),
  formato `sk_<id>_<secret>`: el `id` permite lookup O(1) sin comparar
  bcrypt contra todas las keys existentes (no escalable ni necesario), y el
  `secret` (la parte realmente confidencial, de 256 bits de entropía) es lo
  único hasheado y verificado con bcrypt en tiempo constante
  (`bcrypt.CompareHashAndPassword`).
- Un `id` de API key o de bucket que no es un UUID válido se rechaza antes
  de tocar la base (`db.isValidUUID`), devolviendo 401/404 según corresponda
  en vez de un 500 -- evita tanto filtrar detalles internos de Postgres en
  la respuesta como generar ruido de logs ERROR por lo que en realidad es
  input inválido de un llamante no autenticado.
- Emisión/listado/revocación de API keys detrás de una master key separada
  (`ADMIN_MASTER_KEY`), no de una API key normal -- separación de
  privilegios entre "puedo usar el storage" y "puedo emitir credenciales".
- Rate limiting general sobre `/api/*` (incluye los endpoints de auth)
  dificulta fuerza bruta contra la master key o contra el `secret` de una
  API key -- aunque dado que ambos son valores aleatorios de 256 bits
  (`openssl rand -hex 32` / `crypto/rand`), la fuerza bruta ya es
  computacionalmente inviable independientemente del rate limit.
- **Riesgo aceptado**: no hay expiración automática de API keys (quedan
  válidas indefinidamente hasta revocación manual). Para un caso de uso que
  lo requiera, se puede rotar periódicamente vía el propio endpoint de admin.

## A08:2021 — Software and Data Integrity Failures

- No hay deserialización de datos no confiables más allá de `encoding/json`
  contra structs tipados (sin `interface{}`/`any` arbitrario salvo el campo
  `metadata`, que se persiste como JSON de vuelta sin nunca ejecutarse ni
  interpretarse como código).
- El Dockerfile es un build multi-stage desde código fuente propio (no baja
  binarios pre-compilados de terceros ni ejecuta scripts de instalación
  remotos).
- **Riesgo aceptado**: los tags de imagen base (`golang:1.26-alpine`, etc.)
  no están fijados por digest (`@sha256:...`), así que un rebuild en
  distintos momentos puede traer una imagen base distinta (mismo minor,
  parches nuevos). Es una decisión consciente para no tener que actualizar
  manualmente el digest en cada parche de seguridad upstream; quien prefiera
  reproducibilidad exacta puede fijar el digest.

## A09:2021 — Security Logging and Monitoring Failures

- Todas las requests se loguean estructuradas en JSON (`log/slog`) con
  método, path, status, latencia, IP real (ver A10 más abajo sobre
  `CF-Connecting-IP`) y `user_id` cuando hay una API key autenticada. Ver
  [`middleware/logger.go`](middleware/logger.go).
- El status logueado refleja el código HTTP real devuelto al cliente
  incluso cuando el handler retornó un error (bug real que se encontró y
  corrigió: sin este fix, cualquier request que fallara se logueaba
  siempre como `200`, ocultando por completo los 401/403/429/500 reales en
  los logs).
- Acciones administrativas sensibles generan una línea de log explícita
  además del log de acceso genérico: creación de API key (con `user_id` y
  permisos otorgados), revocación de API key, y eliminación de bucket (con
  quién la ejecutó). Ver `handlers/auth.go` y `handlers/buckets.go`.
- Intentos fallidos de autenticación con la master key generan un `WARN`
  explícito con la IP de origen, distinto del log de acceso genérico.
- **Riesgo aceptado**: no hay alerting/monitoreo activo incluido (esto es
  responsabilidad de la capa de infraestructura). Los logs van a stdout del
  contenedor (`docker compose logs`); para producción real se recomienda
  enviarlos a un agregador (Loki, CloudWatch, etc.) con alertas sobre
  patrones de 401/429 sostenidos.

## A10:2021 — Server-Side Request Forgery (SSRF)

- El servicio no hace ningún request saliente basado en input del usuario
  (no descarga archivos desde una URL, no sigue redirects de terceros, no
  tiene ninguna feature de "importar desde una URL"). Todo archivo llega
  como bytes subidos directamente por el cliente vía multipart. No aplica.

---

## Resumen de hallazgos corregidos en esta revisión

| # | Hallazgo | Categoría | Fix |
|---|----------|-----------|-----|
| 1 | Comparación de la master key con `==` (timing attack) | A02/A07 | `subtle.ConstantTimeCompare` |
| 2 | ID malformado (no-UUID) en bucket/API key devolvía 500 con detalle de Postgres | A05/A07 | Validación de formato antes de la query, `db.isValidUUID` |
| 3 | Extensión física del archivo derivada del nombre del cliente, no del Content-Type validado | A03/A04 | `utils.ExtensionForMimeType`, tabla fija MIME→extensión |
| 4 | Sin validación de que el contenido real coincida con el Content-Type declarado (HTML disfrazado de imagen) | A03 | `utils.LooksLikeHTML` + rechazo en upload |
| 5 | Sin headers de seguridad (`nosniff`, CSP, frame options) | A05 | `helmet` middleware global |
| 6 | `BodyLimit` de Fiber igual a `MaxFileSize`: un archivo grande cortaba la conexión antes de que el handler pudiera responder un 413 legible | A04 | Margen de 1MB sobre `MaxFileSize` en `BodyLimit` |
| 7 | Logger registraba siempre status 200 en responses de error | A09 | Derivar el status logueado del `*fiber.Error` cuando el handler devuelve error |
| 8 | Sin logging explícito de acciones administrativas (alta/baja de API keys, borrado de buckets) | A09 | Logs de auditoría dedicados en los handlers correspondientes |
| 9 | `DB_PASSWORD`/`SIGNING_SECRET`/`ADMIN_MASTER_KEY` en `environment:` de docker-compose quedaban en texto plano en `docker inspect` y `docker compose config` | A05 | Docker secrets basados en archivo (`secrets:` + convención `_FILE`), `scripts/generate-secrets.sh` |
| 10 | `DSN()` armaba el connection string con `fmt.Sprintf`: un password con `@`, `:` o `/` lo rompía silenciosamente | A05 | Reescrito con `net/url` (`url.UserPassword`, escapado correcto) |

Todos los fixes tienen test de regresión (`utils/`, `db/repository_test.go`,
`config/config_test.go`, `main_test.go`) y fueron validados también
manualmente contra el stack real en Docker (ver historial de la sesión;
para el hallazgo #9 en particular se confirmó con `docker inspect` +
`docker compose config` mostrando las credenciales en texto plano ANTES del
fix, y ausentes de ambos comandos DESPUÉS).
