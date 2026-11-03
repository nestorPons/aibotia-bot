# 08 — Estructura del repositorio

## Árbol (raíz)

```
.
├── cmd/                    binarios
│   ├── botd/               servicio principal (API + pipeline + WhatsApp)
│   ├── migrate/            migraciones go/up-down
│   ├── seed/               data/seeds/messages.json → content_blocks + bootstrap de prompts/services
│   ├── embed/              prompts (BD) + temas → embeddings
│   ├── migrate-data/       MariaDB legacy → PostgreSQL
│   └── loadtest/           prueba de carga HTTP
├── internal/
│   ├── actions/            efectos de acciones (fix_chat, humano, ubicación)
│   ├── appointments/       FSM + persistencia de citas
│   ├── assistant/          motor IA (prompt, contexto, tools, Whisper)
│   ├── bootstrap/          seed inicial de prompts/services desde assets
│   ├── config/             entorno + validación
│   ├── domain/             User, Turn, Interaction
│   ├── httpapi/            router chi, middleware, handlers, SSE, panel /admin
│   ├── i18n/               detección de idioma
│   ├── pipeline/           filtros, debounce, orquestación, puertos
│   ├── platform/           logger slog + HTTP server
│   ├── rag/                embeddings + pgvector
│   ├── scheduler/          cron de recordatorios
│   ├── security/           anti-spam Redis
│   ├── store/              postgres, redis, repos, migraciones embebidas
│   ├── version/            build info (ldflags)
│   └── whatsapp/           adaptador whatsmeow
├── data/                   seeds (prompts, servicios, fix_messages, embeddings) + catálogos de referencia
├── deploy/                 Dockerfile, compose, CUTOVER.md
├── docs/                   esta documentación
├── bin/                    binario compilado (ignorado)
├── .env.example            plantilla de entorno
├── Makefile
├── go.mod / go.sum
└── .old/                   legacy Node/MariaDB (ignorado, no versionar)
```

## Convenciones

| Tema | Regla |
|------|-------|
| Paquetes | `internal/<área>`; sin dependencias circulares. |
| Puertos | Interfaces en el paquete consumidor (`pipeline`, `httpapi`). |
| Errores | `fmt.Errorf("paquete: contexto: %w", err)`; handlers → `writeError`. |
| Logs | `slog` estructurado; nunca imprimir secretos. |
| HTTP | Un archivo por recurso en `httpapi`; JSON con `writeJSON`. |
| SQL | pgx con parámetros (`$1`); migraciones goose inmutables. |
| Tests | `_test.go` junto al paquete; sin dependencias externas cuando sea posible. |
| Documentación | `.md` esquemáticos en `docs/`; cambio de estado → `docs/CHANGELOG.md`. |

## Dónde tocar cada cosa

| Necesidad | Archivo(s) |
|-----------|------------|
| Cambiar personalidad/normas | Panel `/admin` → prompt `prompt_instructions` (o seed `data/seeds/prompts/prompt_instructions.md` + reset BD) |
| Cambiar mensajes fijos | `data/seeds/messages.json` + `make seed` (runtime lee `content_blocks`) |
| Añadir tema semántico | `/admin` → nuevo prompt `prompt_<tema>` (reindexa) y/o `data/seeds/embeddings.json` + `make embed` |
| Añadir una tool | `assistant/tools.go` + efecto en `actions/runner.go` |
| Cambiar filtros/anti-spam | `pipeline/filters.go`, `security/antispam.go` |
| Nuevo endpoint | `httpapi/<recurso>.go` + registro en `httpapi/router.go` |
| Nuevo campo de servicio | migración + `store/repo/services.go` + `httpapi/admin.go` + UI |
| Nuevo estado/campo | migración nueva en `store/migrations/sql/` + repo |
| Recordatorios | `appointments/service.go`, `scheduler/scheduler.go` |

## Mapa legacy → Go

| Legacy (`.old/`) | Actual (Go) |
|------------------|-------------|
| `services/whatsapp/whatsapp-provider.js` (whatsapp-web.js) | `internal/whatsapp` (whatsmeow) |
| `src/assistant/*` + LangChain | `internal/assistant` + OpenAI SDK |
| `src/chatbot.js` (flujo) | `internal/pipeline` |
| `src/assistant/actions/*` | `internal/actions` |
| `src/api/*` + Socket.IO | `internal/httpapi` + SSE |
| MariaDB + Knex | PostgreSQL + goose |
| `src/database`, `models/*` | `internal/store/repo` |
| `src/scripts/generateEmbeddings.js` | `cmd/embed` + `internal/rag` |
| `src/scripts/migrate*` | `cmd/migrate-data` |
| Jest (`tests/`) | `go test ./...` |

## Legacy

- Todo el sistema anterior está en `.old/` (código, `node_modules`, docs de
  análisis, `.env`, datos de Redis, logs).
- `.old/` está en `.gitignore` y `.dockerignore`; no forma parte del build.
- Documentación histórica: `.old/docs/` (incluye `CHANGELOG.md` con las fases 0–7).
- No ejecutar el legacy en paralelo con `botd`: compiten por la sesión de WhatsApp.

## Estado del versionado

- El directorio **no es un repositorio git** actualmente.
- Recomendado: `git init` en la raíz, commit inicial y etiqueta antes del cutover.
