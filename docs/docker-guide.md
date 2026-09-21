# Guía de Docker

Todos los comandos `docker compose` de esta guía se corren desde la raíz
del repo, pasando explícitamente el compose file (`docker/docker-compose.yml`
tiene su build context apuntando a la raíz para poder acceder a `core/`).

```bash
cp .env.example .env
./scripts/generate-secrets.sh   # genera secrets/{db_password,signing_secret,admin_master_key}.txt
docker compose -f docker/docker-compose.yml up -d --build
curl http://localhost:8080/health
```

Los tres secretos (`DB_PASSWORD`, `SIGNING_SECRET`, `ADMIN_MASTER_KEY`) **no
van en `.env`**: viven en archivos bajo `secrets/` (gitignored) que
docker-compose monta como [Docker
secrets](https://docs.docker.com/compose/how-tos/use-secrets/), para que
nunca queden en texto plano en `docker inspect` ni en `docker compose
config`. La imagen oficial de SQL Server no soporta esa convención
nativamente (a diferencia de Postgres/MySQL); `docker/database/entrypoint-secrets.sh` se
la agrega.

> **Nota**: `db_password` (usado como password de `sa`) se genera con
> `openssl rand -base64 32`, no `-hex`: SQL Server exige que el password
> tenga caracteres de al menos 3 de 4 clases (mayúsculas, minúsculas,
> dígitos, símbolos), y un hex puro sólo cubre 2.

## Comandos útiles del día a día

```bash
docker compose -f docker/docker-compose.yml logs -f api          # logs JSON estructurados
docker compose -f docker/docker-compose.yml logs -f sqlserver
docker compose -f docker/docker-compose.yml ps
```
