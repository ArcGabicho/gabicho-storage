#!/bin/bash
# La imagen oficial de mssql/server no soporta la convención "_FILE" que sí
# tienen postgres/mysql (ej. POSTGRES_PASSWORD_FILE) para leer secretos
# desde un archivo en vez de una variable de entorno en texto plano. Este
# wrapper la agrega: si MSSQL_SA_PASSWORD_FILE apunta a un archivo, lo lee y
# exporta MSSQL_SA_PASSWORD con ese contenido antes de arrancar el
# entrypoint real de la imagen -- así el password nunca necesita pasarse
# por `environment:` en docker-compose.yml (que sí queda en texto plano en
# `docker inspect`/`docker compose config`), sólo por `secrets:`.
set -euo pipefail

if [ -n "${MSSQL_SA_PASSWORD_FILE:-}" ] && [ -f "$MSSQL_SA_PASSWORD_FILE" ]; then
  export MSSQL_SA_PASSWORD
  MSSQL_SA_PASSWORD="$(cat "$MSSQL_SA_PASSWORD_FILE")"
fi

exec /opt/mssql/bin/launch_sqlservr.sh "$@"
