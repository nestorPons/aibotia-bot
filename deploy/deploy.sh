#!/usr/bin/env bash
# Despliegue de aibotia.com en el VPS (rsync + docker compose).
# Uso: ./deploy/deploy.sh
set -euo pipefail

VPS="${VPS:-root@93.127.203.142}"
REMOTE_DIR="${REMOTE_DIR:-/root/projects/aibotia.com}"
COMPOSE_FILE="docker-compose.prod.yml"

cd "$(dirname "$0")/.."

rsync -az --delete \
  --exclude='.env' \
  --exclude='deploy/.env' \
  --exclude='.git' \
  --exclude='.vscode' \
  --exclude='bin' \
  --exclude='.old' \
  --exclude='node_modules' \
  --exclude='*.sqlite' \
  --exclude='coverage.out' \
  --exclude='.DS_Store' \
  ./ "$VPS:$REMOTE_DIR/"

ssh "$VPS" "cd $REMOTE_DIR/deploy && docker compose -f $COMPOSE_FILE up -d --build"

ssh "$VPS" "curl -fsS http://127.0.0.1:3000/ready && echo"
