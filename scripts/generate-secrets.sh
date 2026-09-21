#!/usr/bin/env bash
# Genera los archivos de secretos que consume docker-compose.yml (bloque
# "secrets"), con permisos restrictivos. No pisa un archivo que ya exista,
# para no romper una base de datos/credenciales en uso por accidente.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

mkdir -p secrets

generate() {
  local file="secrets/$1"
  if [ -f "$file" ]; then
    echo "ya existe, no se toca: $file"
    return
  fi
  openssl rand -hex 32 > "$file"
  # 644 (no 600): docker-compose monta "secrets:" fuera de Swarm como un
  # bind mount plano del archivo del host, preservando sus permisos/dueño
  # tal cual -- no los remapea al uid del usuario dentro del contenedor. El
  # proceso corre como el usuario no-root "app" (uid distinto al del host),
  # así que si el archivo quedara en 600 (sólo legible por su dueño en el
  # host) el contenedor no podría leerlo. La confidencialidad real acá pasa
  # por otro lado: el archivo nunca sale de este directorio (gitignored,
  # nunca se commitea) ni se expone vía `docker inspect`/`docker compose
  # config` -- que era el problema que se quería resolver.
  chmod 644 "$file"
  echo "generado: $file"
}

generate db_password.txt
generate signing_secret.txt
generate admin_master_key.txt

echo
echo "Listo. Estos archivos NUNCA deben commitearse (ya están en .gitignore)."
echo "Para levantar el stack: docker compose up -d --build"
