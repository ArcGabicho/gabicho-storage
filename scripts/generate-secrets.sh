#!/usr/bin/env bash
# Genera los archivos de secretos que consume docker-compose.yml (bloque
# "secrets"), con permisos 644. No pisa un archivo que ya exista, para no
# romper una base de datos/credenciales en uso por accidente.
#
# 644 (no 600): docker-compose monta "secrets:" fuera de Swarm como un bind
# mount plano del archivo del host, preservando sus permisos/dueño tal cual
# -- no los remapea al uid del usuario dentro del contenedor (ni el de la
# API ni el de SQL Server). La confidencialidad real acá pasa por otro
# lado: el archivo nunca sale de este directorio (gitignored, nunca se
# commitea) ni se expone vía `docker inspect`/`docker compose config`.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

mkdir -p secrets

generate() {
  local file="secrets/$1"
  local value="$2"
  if [ -f "$file" ]; then
    echo "ya existe, no se toca: $file"
    return
  fi
  printf '%s' "$value" > "$file"
  chmod 644 "$file"
  echo "generado: $file"
}

# db_password se usa como password de "sa" en SQL Server, que exige
# complejidad (mínimo 8 caracteres, y al menos 3 de estas 4 clases:
# mayúsculas, minúsculas, dígitos, símbolos). Un hex puro (0-9a-f) sólo
# cubre 2 clases y SQL Server lo rechaza -- pasó exactamente eso al probar
# este script: el contenedor de SQL Server crasheaba en el arranque con
# "Password validation failed". base64 sí cubre mayúsculas+minúsculas+dígitos
# (y normalmente símbolos), así que alcanza la complejidad exigida.
generate db_password.txt "$(openssl rand -base64 32)"
generate signing_secret.txt "$(openssl rand -hex 32)"
generate admin_master_key.txt "$(openssl rand -hex 32)"

echo
echo "Listo. Estos archivos NUNCA deben commitearse (ya están en .gitignore)."
echo "Para levantar el stack: docker compose -f docker/docker-compose.yml up -d --build"
