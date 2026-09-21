#!/usr/bin/env bash
# Purga TODO lo que docker-compose creó para este proyecto: contenedores,
# imágenes, volúmenes (incluye los datos de SQL Server y los archivos
# subidos) y la red. Destructivo e irreversible. NO borra secrets/ ni .env
# (borralos a mano si también los querés eliminar).
#
# Uso:
#   ./scripts/clear.sh          # pide confirmación interactiva
#   ./scripts/clear.sh --force  # sin confirmación, para uso en CI/scripts
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

COMPOSE_FILE="docker/docker-compose.yml"
PROJECT_NAME="gabicho-storage"

echo "Esto va a eliminar TODOS los contenedores, imágenes, volúmenes y la red"
echo "del proyecto '$PROJECT_NAME', incluyendo los datos de SQL Server y los"
echo "archivos subidos (volumen storage_data). Esta acción NO se puede deshacer."
echo

if [ "${1:-}" != "--force" ]; then
  read -rp "Escribí 'yes' para confirmar: " confirm
  if [ "$confirm" != "yes" ]; then
    echo "Cancelado."
    exit 1
  fi
fi

if [ -f "$COMPOSE_FILE" ]; then
  echo "==> Bajando el stack, borrando contenedores y volúmenes"
  docker compose -f "$COMPOSE_FILE" down -v --remove-orphans --rmi local
fi

echo "==> Borrando imágenes remanentes del proyecto"
docker images --filter "reference=${PROJECT_NAME}*" -q | xargs -r docker rmi -f

echo "==> Borrando volúmenes remanentes"
docker volume ls --filter "name=${PROJECT_NAME}" -q | xargs -r docker volume rm -f

echo "==> Borrando red remanente"
docker network ls --filter "name=${PROJECT_NAME}" -q | xargs -r docker network rm

echo
echo "Listo, el proyecto quedó purgado. secrets/ y .env no se tocaron."
