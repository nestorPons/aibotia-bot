# 07 — Operación

## Requisitos

- Go 1.27+ (build local) o Docker 24+/Compose 2.20+.
- PostgreSQL 16 con pgvector.
- Redis 7.
- `OPENAI_API_KEY` válida para IA/RAG/Whisper.

## Variables de entorno

| Variable | Default | Obligatoria | Uso |
|----------|---------|-------------|-----|
| `APP_ENV` | `development` | no | En `production` exige OpenAI. |
| `INSTANCE_ID` | `default` | no | Identificador de instancia. |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `json` | no | `json` o `text`. |
| `HTTP_ADDR` | `:3000` | no | Escucha HTTP. |
| `HTTP_*_TIMEOUT` | 10s/30s/60s/15s | no | Read/write/idle/shutdown. |
| `API_KEY` | — | **sí** | Auth; no puede igualar a OpenAI. |
| `API_ENABLED` | `true` | no | `false` ⇒ no se monta `/api/v1` (solo landing, `/health`, `/ready`, `/admin`). |
| `CORS_ALLOWED_ORIGINS` | `*` | no | Lista separada por comas. |
| `ADMIN_ENABLED` | `true` | no | Panel embebido `/admin`. |
| `ADMIN_PASSWORD` | vacío | no | Contraseña del panel. En producción vacío ⇒ panel deshabilitado. En dev vacío ⇒ usa `API_KEY`. |
| `ADMIN_SESSION_SECRET` | `ADMIN_PASSWORD` | no | Clave HMAC de la cookie de sesión. |
| `ADMIN_SESSION_TTL` | `12h` | no | Duración de la sesión. |
| `OPENAI_API_KEY` | — | prod | IA, RAG, audio. |
| `OPENAI_MODEL` | `gpt-6.1-sol` | no | Modelo de chat (Responses API + tool calling). |
| `OPENAI_EMBED_MODEL` | `text-embedding-3-small` | no | Embeddings (1536 dim). |
| `RAG_THRESHOLD` | `0.78` | no | Similitud mínima de tema. |
| `PROMPTS_DIR` / `DATA_DIR` | `data/seeds/prompts` / `data/seeds` | no | Assets seed (en Docker `/app/...`). |
| `DATABASE_URL` | — | **sí** | PostgreSQL (pool pgx). |
| `REDIS_URL` | `redis://localhost:6379/0` | no | Memoria/estado/anti-spam. |
| `AUTO_MIGRATE` | `true` | no | Migraciones al arrancar. |
| `WHATSAPP_ENABLED` | `true` | no | Desactiva el canal para probar API. |
| `WHATSAPP_STORE_DSN` | `DATABASE_URL` | no | Store de sesión whatsmeow. |
| `HOURS_TO_ACTIVE` | `168` | no | Reactivación por inactividad. |
| `BUFFER_MIN_MS` / `BUFFER_MAX_MS` | `1000` / `15000` | no | Debounce por chat. |
| `REMINDER_CRON` | `*/10 * * * *` | no | Frecuencia de recordatorios. |
| `REMINDER_TEMPLATE` | plantilla | no | `{date}` se sustituye. |
| `SPAM_LIMIT` / `SPAM_WINDOW` | `5` / `60s` | no | Anti-spam. |
| `LOCATION_NAME` / `LOCATION_ADDRESS` / `LOCATION_LAT` / `LOCATION_LNG` | vacío/0 | no | Fallback de `send_location`; lo normal es editar el pin en **Empresa** (`prompt_company`). |
| `LEGACY_DATABASE_URL` | — | solo migración | DSN MySQL legacy. |

## Panel de administración

- URL: `http://<host>:3000/admin` (UI embebida en `botd`, sin build de frontend).
- Funciones: CRUD de servicios, prompts y usuarios, historial/rollback y
  reindexado RAG, bandeja de mensajes (ver/contestar) y vinculación de WhatsApp por QR.
- `Empresa`: editor único del prompt `prompt_company` (dirección, ubicación,
  contacto, horarios, parking, accesos). Se inyecta siempre en el system prompt
  y su primera línea fija el pin de WhatsApp:
  `<!-- pin: 39.4555556,-0.3583096 | Studio Tamara Freitas | dirección -->`.
  Los antiguos `prompt_location*` se fusionan aquí al arrancar.
- `Usuarios`: filtro segmentado **Todos / Activo / Inactivo** por estado del bot
  (`ai=active|inactive`). Borrar un usuario elimina también sus conversaciones,
  citas, historial Redis, anti-spam y caché de WhatsApp.
- `Ajustes`: interruptor **Activar el bot a los nuevos usuarios**
  (`settings.activate_new_users`). Actúa al crear usuarios por mensaje entrante
  o por importación de contactos; no modifica los usuarios existentes.
- Bandeja de mensajes: búsqueda por nombre/alias/teléfono + filtros por
  `⚠️ Requieren respuesta` (el bot derivó al equipo: cita por valorar/agendar,
  atención humana o `fix_chat`) y por estado de la IA (activa/inactiva). Los
  hilos pendientes se marcan con borde ámbar, punto pulsante y etiqueta
  `⚠ Requiere operador`. Al responder el operador, la marca desaparece sola.
- Al abrir una conversación aparece una tarjeta **colapsable** sobre el chat
  (`Resumen para el operador`): chips de tema, servicio, preferencia horaria,
  estado de la cita y motivo de derivación, más 2-4 puntos clave generados a
  partir de los últimos mensajes (o resumen básico si OpenAI falla). El estado
  colapsado se recuerda en el navegador.
- Acceso: `ADMIN_PASSWORD`; en desarrollo, si está vacío se usa `API_KEY`.
- En producción, sin `ADMIN_PASSWORD` el panel **no se monta** (aviso en logs).
- Recomendado: `ADMIN_PASSWORD` distinta de `API_KEY` + `ADMIN_SESSION_SECRET`
  aleatorio; usar HTTPS (`SecureCookie` se activa con `APP_ENV=production`).

```bash
# Dev con compose: el panel queda en http://localhost:3000/admin
ADMIN_PASSWORD=mi-clave docker compose up -d botd
```

- Guardar un prompt reindexa su embedding (requiere `OPENAI_API_KEY`). Si falla,
  el cambio se guarda igualmente y el error queda en el log.
- Los cambios se aplican en caliente (sin reiniciar `botd`).
- Los assets `.md`/`services.json` solo siembran la BD la primera vez.

## Local

```bash
cp .env.example .env
go mod download
make run
```

## Desarrollo local (compose raíz)

`docker-compose.yml` en la raíz = entorno de **desarrollo**. Proyecto Compose: `aibotia-dev`.

```bash
cp .env.example .env          # opcional: compose tiene defaults de dev
docker compose up -d          # postgres + redis + botd
docker compose logs -f botd
```

| Servicio | Imagen | Puerto host | Notas |
|----------|--------|-------------|-------|
| `postgres` | `pgvector/pgvector:pg16` | `${POSTGRES_PORT:-5432}` | Volumen `pgdata-dev`. |
| `redis` | `redis:7.4-alpine` | `${REDIS_PORT:-6379}` | AOF, volumen `redisdata-dev`. |
| `botd` | build `Dockerfile.dev` (`aibotia-dev-botd`) | `${HTTP_PORT:-3000}` | Código montado en `/src`; **compila al arrancar**. |

- `AUTO_MIGRATE=true`: el esquema se aplica al arrancar.
- `API_KEY` default de dev: `dev-api-key` (cambiar en `.env`).
- Contraseñas de PostgreSQL default: `aibotia/aibotia` (solo local).
- El compose lee `.env` de la raíz para interpolar; las variables de `environment`
  tienen prioridad sobre el `.env` que carga `godotenv` dentro del contenedor.
- `Dockerfile.dev`: `go mod download` + `go build ./...` durante el build (precalienta
  la caché) y `go build -o /tmp/botd ./cmd/botd` **al iniciar el contenedor**.
- Tras cambiar código Go o los HTML del panel → `make dev-apply`
  (`docker compose restart botd`; recompila con el código montado).
- Tras cambiar `go.mod`/`go.sum` → `docker compose build botd`.
- Arranque en frío tras el build: ~30–60 s (compilación incremental + enlace).
- Solo dependencias (app con `make run` en el host):

  ```bash
  docker compose up -d postgres redis
  # .env: DATABASE_URL=postgres://aibotia:aibotia@localhost:5432/aibotia?sslmode=disable
  #       REDIS_URL=redis://localhost:6379/0
  ```

- Probar solo API sin vincular WhatsApp: `WHATSAPP_ENABLED=false docker compose up -d botd`.
- La imagen descarga módulos y precompila durante el build; los volúmenes
  `go-mod-cache` y `go-build-cache` aceleran los arranques. `start_period`: 300 s.
- Limpiar todo (incluye datos locales): `docker compose down -v`.

## Docker (producción / staging)

```bash
docker compose -f deploy/docker-compose.yml up -d --build
docker compose -f deploy/docker-compose.yml logs -f botd
```

- Contexto de build: raíz del repo (`deploy/docker-compose.yml`: `context: ..`).
- Imagen final: `/app/botd` + `/app/data` (incluye `data/seeds/`); usuario no root.
- Servicios: `postgres` (pgvector/pg16), `redis` (7.4, AOF), `botd` (:3000).
- Volúmenes: `pgdata`, `redisdata`.

## Makefile

| Target | Acción |
|--------|--------|
| `run` | `go run ./cmd/botd` |
| `build` | binario `bin/botd` con ldflags |
| `test` | `go test ./...` |
| `vet` / `fmt` / `lint` | Calidad (`golangci-lint` opcional) |
| `migrate-up` / `migrate-down` | goose |
| `seed` / `embed` | contenido y RAG |
| `migrate-data` | MariaDB → PostgreSQL |
| `loadtest` | carga HTTP (`-key "$(API_KEY)"`) |
| `docker-up` / `docker-down` | Compose de `deploy/` |

## Cutover desde legacy

Runbook completo: [`deploy/CUTOVER.md`](../deploy/CUTOVER.md).

Resumen:
1. Infra PostgreSQL+Redis y `.env`.
2. `make migrate-up`.
3. `make migrate-data` (`-dry-run` → `-truncate`).
4. `make seed` + `make embed`.
5. Arrancar, escanear QR y verificar `/health`, `/ready`, `/whatsapp/status`.
6. Parar el proveedor Node (`.old/`) antes de vincular el número.
7. Prueba funcional y redirección de tráfico.

Rollback: volver a arrancar el legacy y revincular sesión (el store cambia entre
`whatsapp-web.js` y `whatsmeow`).

## Observabilidad

- Logs JSON `slog` con `request_id`, latencia HTTP y errores.
- `/health` (liveness) y `/ready` (PostgreSQL+Redis).
- SSE para estado/QR de WhatsApp.
- `cmd/loadtest` para p50/p90/p99 y códigos.

## Troubleshooting

| Síntoma | Causa probable | Acción |
|---------|----------------|--------|
| Falla al arrancar: `API_KEY es obligatoria` | `.env` incompleto | Definir `API_KEY`/`DATABASE_URL`. |
| `OPENAI_API_KEY es obligatoria en producción` | `APP_ENV=production` sin clave | Definir clave o cambiar entorno. |
| `/ready` 503 | PostgreSQL/Redis caídos | Revisar contenedores y DSN. |
| WhatsApp `waiting_qr` sin avanzar | QR caducado | Reiniciar `botd` y escanear de nuevo. |
| `logged_out` | Sesión cerrada desde el teléfono | Revincular con QR. |
| Bot no responde | `settings.is_active=false` o usuario `active=false` | `PUT /assistant/status` o `PUT /users/{id}`. |
| Respuestas sin contexto | `make embed` no ejecutado o sin key | Ejecutar `embed` o guardar el prompt desde `/admin`. |
| `/admin` 404 o 401 | Panel deshabilitado / sin sesión | Definir `ADMIN_PASSWORD`; entrar en `/admin/login`. |
| Citas sin recordatorio | `scheduled_at` nulo | Limitación actual. |

## Pendientes de seguridad/operación

- Rotar la clave OpenAI expuesta en `.old/.env` y no versionar `.old/`.
- Restringir `CORS_ALLOWED_ORIGINS` en producción.
- Añadir rate limit HTTP si se expone públicamente.
- El repo **no es git**: inicializar y hacer el primer commit antes de tocar más.
