#!/usr/bin/env bash
# Prepara una VM Rocky Linux (probado en "Minimal") desde cero: instala
# Docker, clona el repo y crea el .env inicial. Pensado para correrse una
# sola vez por máquina, vía:
#   curl -fsSL https://raw.githubusercontent.com/ArcGabicho/gabicho-storage/main/scripts/setup.sh | bash
# Es idempotente: si Docker/el repo/el .env ya existen, no los toca.
set -euo pipefail

REPO_URL="${REPO_URL:-https://github.com/ArcGabicho/gabicho-storage.git}"
REPO_REF="${REPO_REF:-main}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/gabicho-storage}"

echo "==> Instalando dependencias del sistema"
sudo dnf update -y
sudo dnf install -y dnf-plugins-core git curl firewalld
sudo systemctl enable --now firewalld

if ! command -v docker &>/dev/null; then
  echo "==> Instalando Docker"
  sudo dnf config-manager --add-repo https://download.docker.com/linux/rhel/docker-ce.repo
  sudo dnf install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
  sudo systemctl enable --now docker
  sudo usermod -aG docker "$USER"
  echo "Docker instalado. Si vas a correr scripts/deploy.sh en esta misma"
  echo "sesión de shell, puede que necesites 'newgrp docker' o volver a"
  echo "entrar por SSH para que el grupo 'docker' te aplique sin sudo."
else
  echo "==> Docker ya está instalado, se omite instalación"
fi

echo "==> Clonando el proyecto en $INSTALL_DIR"
if [ -d "$INSTALL_DIR/.git" ]; then
  echo "El directorio ya existe, se omite el clone (usá 'git pull' o scripts/deploy.sh para actualizar)."
else
  git clone --branch "$REPO_REF" "$REPO_URL" "$INSTALL_DIR"
fi

cd "$INSTALL_DIR"
chmod +x scripts/*.sh

if [ ! -f .env ]; then
  cp .env.example .env
  echo "==> .env creado a partir de .env.example"
  echo "Revisalo antes de deployar, en particular CORS_ORIGIN y los límites de rate limiting."
else
  echo "==> .env ya existe, no se sobreescribe"
fi

echo
echo "Setup completo. Próximos pasos:"
echo "  cd $INSTALL_DIR"
echo "  \$EDITOR .env"
echo "  ./scripts/deploy.sh"
