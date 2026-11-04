# Despliegue en VPS (Hostinger) con Traefik

> Producción de `aibotia.com` en `93.127.203.142`.
> Compose: [`docker-compose.prod.yml`](../docker-compose.prod.yml) (raíz del repo).

## Topología

| Elemento | Valor |
|----------|-------|
| Host | `root@93.127.203.142` |
| Directorio | `/root/projects/aibotia.com` |
| Compose | `docker-compose.prod.yml` en la raíz (proyecto `aibotia`) |
| Dominio | `aibotia.com` (+ redirección `www` → apex) |
| Proxy | Traefik `/root/traefik` (red externa `traefik-net`, resolver `myresolver`) |
| TLS | Let's Encrypt (HTTP-01) |
| Publicación | Traefik → `botd:3000`; host solo `127.0.0.1:3000` para diagnóstico |

```
Internet → Traefik (80/443) → aibotia-botd-1:3000
                                  ├── aibotia-postgres-1 (pgvector/pg16)
                                  └── aibotia-redis-1 (redis 7)
```

## Archivos

| Archivo | Función |
|---------|---------|
| `docker-compose.prod.yml` | Stack de producción + labels de Traefik. |
| `.env` (servidor, `chmod 600`) | Secretos y configuración; **no se sube** con rsync. |
| `.env.example` | Plantilla de variables (dev y prod). |
| `Dockerfile` / `Dockerfile.tester` | Imágenes de `botd` y del bot de pruebas. |
| `deploy.sh` | Sincroniza código y reconstruye el stack. |

## Variables clave

| Variable | Nota |
|----------|------|
| `APP_ENV=production` | Exige `OPENAI_API_KEY` y desactiva `/admin` si falta `ADMIN_PASSWORD`. |
| `API_KEY` | Auth de `/api/v1` (distinta de OpenAI). |
| `API_ENABLED` | `false` en producción: la API pública no se monta. |
| `ADMIN_PASSWORD` / `ADMIN_SESSION_SECRET` | Acceso y firma de cookie del panel. |
| `POSTGRES_PASSWORD` | Password de PostgreSQL (se interpola en `DATABASE_URL`). |
| `CORS_ALLOWED_ORIGINS` | `https://aibotia.com,https://www.aibotia.com`. |

## Rutina de despliegue

```bash
# Desde la raíz del repo local (excluye .env y los compose de dev)
./deploy.sh
```

Equivalente manual:

```bash
rsync -az --delete --exclude='.env' --exclude='docker-compose.yml' ... ./ root@93.127.203.142:/root/projects/aibotia.com/
ssh root@93.127.203.142 'cd /root/projects/aibotia.com && docker compose -f docker-compose.prod.yml up -d --build'
```

## Primer arranque / datos

`AUTO_MIGRATE=true` aplica el esquema y `bootstrap.Content` siembra prompts y
servicios si las tablas están vacías. Los bloques de contenido y los embeddings
se cargan una vez con un contenedor efímero:

```bash
ssh root@93.127.203.142 'set -a; . /root/projects/aibotia.com/.env; set +a
docker run --rm -v /root/projects/aibotia.com:/src -w /src --network aibotia_internal \
  -e APP_ENV=production -e API_KEY="$API_KEY" -e OPENAI_API_KEY="$OPENAI_API_KEY" \
  -e PROMPTS_DIR=data/seeds/prompts -e DATA_DIR=data/seeds \
  -e DATABASE_URL="postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@postgres:5432/$POSTGRES_DB?sslmode=disable" \
  golang:1.27-alpine sh -c "go run ./cmd/seed && go run ./cmd/embed"'
```

## Verificación

```bash
curl -s https://aibotia.com/health           # {"status":"ok"}
curl -s https://aibotia.com/ready            # postgres/redis ok
curl -sI https://www.aibotia.com/            # 301 → https://aibotia.com/
curl -s -H "x-api-key: $API_KEY" https://aibotia.com/api/v1/whatsapp/status
```

- Panel: `https://aibotia.com/admin` (usuario único `ADMIN_PASSWORD`).
- WhatsApp: vincular desde el panel (QR); la sesión `whatsmeow` persiste en PostgreSQL.

## Persistencia

| Volumen | Contenido |
|---------|-----------|
| `aibotia_pgdata` | PostgreSQL (datos del bot + sesión WhatsApp). |
| `aibotia_redisdata` | Historial/debounce y citas en curso. |

## Notas

- El stack antiguo `aibotiacom` (nginx/PHP + MariaDB) está detenido; su dump
  previo vive en `/root/backpups/aibotia-mariadb-*.sql` y el volumen
  `aibotiacom_dbdata` se conserva.
- Traefik enruta por labels: router `aibotia`, servicio `aibotia:3000`,
  middleware `aibotia-www` (redirect permanente).
- No abrir `3000`, `5432` ni `6379` al exterior.
