#!/usr/bin/env bash
# Levanta (o actualiza) el stack completo en la VM: genera secrets si
# faltan, hace pull de la última versión si el directorio ya es un repo
# git, y levanta docker-compose. Se corre desde la raíz del repo clonado
# (lo hace scripts/setup.sh):
#   cd gabicho-storage && ./scripts/deploy.sh
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

COMPOSE_FILE="docker/docker-compose.yml"

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "Error: no se encontró $COMPOSE_FILE. Corré este script desde un checkout del repo." >&2
  exit 1
fi

if [ ! -f .env ]; then
  echo "Error: falta .env. Corré ./scripts/setup.sh primero, o 'cp .env.example .env'." >&2
  exit 1
fi

if [ -d .git ]; then
  echo "==> Actualizando el código (git pull)"
  git pull --ff-only || echo "Aviso: no se pudo hacer pull (¿cambios locales?), se sigue con el código actual."
fi

if [ ! -f secrets/db_password.txt ] || [ ! -f secrets/signing_secret.txt ] || [ ! -f secrets/admin_master_key.txt ]; then
  echo "==> Generando secrets/"
  ./scripts/generate-secrets.sh
else
  echo "==> secrets/ ya existe, se omite generación (borralos a mano si querés rotarlos)"
fi

echo "==> Levantando el stack"
docker compose -f "$COMPOSE_FILE" up -d --build

echo "==> Esperando a que los servicios queden healthy..."
for i in $(seq 1 20); do
  if curl -fsS http://localhost:8080/health >/dev/null 2>&1; then
    echo
    echo "OK: el servicio responde en http://localhost:8080/health"
    docker compose -f "$COMPOSE_FILE" ps
    exit 0
  fi
  sleep 3
done

echo "El servicio no respondió a tiempo. Revisá los logs:" >&2
echo "  docker compose -f $COMPOSE_FILE logs -f api" >&2
echo "  docker compose -f $COMPOSE_FILE logs -f sqlserver" >&2
exit 1
