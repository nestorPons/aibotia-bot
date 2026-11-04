#!/usr/bin/env bash
# Despliegue de aibotia.com en el VPS (rsync + docker compose).
# Uso: ./deploy.sh
set -euo pipefail

VPS="${VPS:-root@93.127.203.142}"
REMOTE_DIR="${REMOTE_DIR:-/root/projects/aibotia.com}"
COMPOSE_FILE="docker-compose.prod.yml"

cd "$(dirname "$0")"

# El compose de producción vive en la raíz del repo desde 2026-11-04.
# Se excluyen los artefactos de desarrollo para que el servidor no pueda
# arrancar el stack de dev por error con el .env de producción.
rsync -az --delete \
  --exclude='.env' \
  --exclude='.git' \
  --exclude='.vscode' \
  --exclude='bin' \
  --exclude='.old' \
  --exclude='node_modules' \
  --exclude='*.sqlite' \
  --exclude='coverage.out' \
  --exclude='.DS_Store' \
  --exclude='docker-compose.yml' \
  --exclude='docker-compose.staging.yml' \
  --exclude='Dockerfile.dev' \
  ./ "$VPS:$REMOTE_DIR/"

ssh "$VPS" "cd $REMOTE_DIR && docker compose -f $COMPOSE_FILE up -d --build"

ssh "$VPS" "curl -fsS http://127.0.0.1:3000/ready && echo"
