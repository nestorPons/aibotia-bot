# Documentación — aibotia-bot (Go)

> Bot de WhatsApp con asistente IA para el Studio Tamara Freitas.
> Esta carpeta describe **solo** el sistema actual (Go). El sistema anterior (Node) está en `/.old/`.

## Estado

| Aspecto | Valor |
|---------|-------|
| Runtime | Go 1.27 |
| Binario principal | `botd` (`cmd/botd`) |
| WhatsApp | `whatsmeow` (sin Chromium) |
| Persistencia | PostgreSQL 16 + pgvector · Redis 7 |
| IA | OpenAI Chat Completions + tool calling + Whisper |
| HTTP | `net/http` + chi v5 |
| API | `/api/v1`, auth por `x-api-key` |
| Panel | `/admin` embebido: CRUD servicios/prompts/usuarios, bandeja de mensajes y QR de WhatsApp |
| Realtime | SSE (`/api/v1/events`) |
| Legacy | `/.old/` (Node, MariaDB, whatsapp-web.js) — no versionado |

Verificación actual: `go build ./...` y `go test ./...` OK.

## Arranque rápido

```bash
cp .env.example .env         # API_KEY, DATABASE_URL, REDIS_URL, OPENAI_API_KEY
docker compose -f deploy/docker-compose.yml up -d postgres redis
make migrate-up              # esquema
make seed                    # bloques de contenido (fix-messages)
make embed                   # embeddings del RAG
make run                     # botd
```

Vincular WhatsApp: escanear el QR de los logs o `GET /api/v1/whatsapp/qr`.

## Índice

| Doc | Contenido |
|-----|-----------|
| [01-arquitectura](01-arquitectura.md) | Componentes, capas, arranque, concurrencia, decisiones. |
| [02-flujo-mensaje](02-flujo-mensaje.md) | Recorrido completo de un mensaje (filtros → IA → envío → datos). |
| [03-whatsapp](03-whatsapp.md) | `whatsmeow`, sesión/QR, normalización, media, límites. |
| [04-asistente-ia](04-asistente-ia.md) | Prompts, RAG, tools, acciones, memoria, idioma, audio. |
| [05-datos](05-datos.md) | Esquema PostgreSQL, claves Redis, assets, seed/embed, migración. |
| [06-api](06-api.md) | Endpoints, auth, payloads, errores, SSE y panel `/admin`. |
| [07-operacion](07-operacion.md) | Variables de entorno, Makefile, Docker, cutover, troubleshooting. |
| [08-estructura-repo](08-estructura-repo.md) | Árbol del repo, convenciones, mapa legacy→Go. |
| [09-flujo-conversacional](09-flujo-conversacional.md) | Recorrido detallado con decisiones, escenarios y mejoras priorizadas. |
| [10-modelo-decision](10-modelo-decision.md) | Capa de decisión previa al LLM (seguridad, función, tipo de respuesta). |
| [11-despliegue-vps](11-despliegue-vps.md) | Producción en VPS Hostinger: Traefik, TLS, volúmenes y rutina de deploy. |
| [12-bot-pruebas](12-bot-pruebas.md) | Bot de pruebas por consola: QR, escenarios, modo IA y reportes de calidad. |
| [CHANGELOG](CHANGELOG.md) | Registro de cambios de estado. |

## Reglas de este repositorio

- El código legacy **no se toca**: vive en `.old/` y está excluido de git/Docker.
- Los secretos nunca se versionan: usar `.env` (ignorado) a partir de `.env.example`.
- Los `.md` son esquemáticos: tablas, listas y referencias a rutas.
- Cambios de estado → entrada en `docs/CHANGELOG.md`.
