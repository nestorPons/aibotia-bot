# 01 — Arquitectura

## Vista general

```
WhatsApp  ⇄  whatsmeow (internal/whatsapp)
                │  IncomingMessage
                ▼
        pipeline.Service (goroutine)
          dedup → filtros → staff → settings → usuario → anti-spam → debounce
                │  ReplyRequest
                ▼
        assistant.Assistant ──► OpenAI (chat + tools + embeddings + Whisper)
          ├─ rag.Service ──────► pgvector (embeddings)
          ├─ repo.History ─────► Redis (memoria 7 d)
          └─ appointments ─────► Redis + PostgreSQL
                │  Reply{Text, Actions, Attachments}
                ▼
        actions.Runner (fix_chat, humano, ubicación, ...)
                ▼
        whatsapp.Provider ──► envío (texto, media, ubicación, typing, seen)
                ▼
        repo.Users / repo.Interactions ──► PostgreSQL
```

En paralelo:

- `scheduler` (cron `robfig/cron`) envía recordatorios de citas.
- `httpapi` sirve la API y el SSE.
- El handler de eventos de `whatsmeow` alimenta el canal de entrada.

## Capas y paquetes

| Capa | Paquete | Responsabilidad |
|------|---------|-----------------|
| Entrada | `cmd/botd` | Composición de dependencias, arranque y apagado ordenado. |
| Entrada | `cmd/migrate`, `cmd/seed`, `cmd/embed`, `cmd/migrate-data`, `cmd/loadtest` | Tareas de administración. |
| Transporte | `internal/httpapi` | Router chi, middlewares, handlers, SSE. |
| Dominio/aplicación | `internal/pipeline` | Orquestación del mensaje, filtros, debounce, persistencia del turno. |
| Dominio/aplicación | `internal/assistant` | Prompt, contexto, tool calling, transcripción. |
| Dominio/aplicación | `internal/actions` | Efectos de las acciones del modelo. |
| Dominio/aplicación | `internal/appointments` | FSM y persistencia de citas. |
| Infra | `internal/whatsapp` | Adaptador `whatsmeow`. |
| Infra | `internal/store/postgres`, `internal/store/redis`, `internal/store/repo`, `internal/store/migrations` | Persistencia y migraciones. |
| Soporte | `internal/rag`, `internal/security`, `internal/i18n`, `internal/scheduler` | Búsqueda semántica, anti-spam, idioma, cron. |
| Soporte | `internal/bootstrap` | Siembra inicial de prompts y servicios desde assets. |
| Transporte | `internal/httpapi/admin.go` | Panel `/admin` (sesión propia, CRUD, UI embebida). |
| Soporte | `internal/platform`, `internal/config`, `internal/version` | Logger, HTTP server, entorno, build info. |

Regla de dependencias: `httpapi`/`pipeline` dependen de interfaces propias
(`pipeline.Assistant`, `pipeline.SpamChecker`, `pipeline.ActionRunner`, `whatsapp.Provider`).
La infraestructura implementa esas interfaces y se inyecta en `cmd/botd`.

## Secuencia de arranque (`cmd/botd/main.go`)

1. Carga `.env` (opcional) y valida configuración (`config.Load` falla rápido).
2. Construye logger `slog` (`json` o `text`).
3. Contexto cancelable con `SIGINT`/`SIGTERM`.
4. Conecta PostgreSQL (pool pgx con reintentos).
5. Si `AUTO_MIGRATE=true`, aplica migraciones goose embebidas.
6. Conecta Redis.
7. Construye repositorios (`Users`, `Settings`, `History`, `Interactions`, `Instructions`, `Prompts`, `Services`).
   Siembra prompts/servicios desde assets si las tablas están vacías (bootstrap).
8. Si `WHATSAPP_ENABLED`, inicia `whatsapp.Client` (store de sesión en PostgreSQL).
9. Construye anti-spam, RAG, citas, asistente y ejecutor de acciones.
10. Si hay WhatsApp, arranca `pipeline.Run` (goroutine) y el scheduler de recordatorios.
11. Monta el router HTTP (API + panel `/admin`) y sirve con timeouts y shutdown ordenado.

Sin `OPENAI_API_KEY` el servicio arranca en **modo degradado**: responde
`*Asistente Virtual*` + `(IA no configurada) He recibido tu mensaje.` y no usa RAG/tools.

## Concurrencia

| Goroutine | Origen | Vida |
|-----------|--------|------|
| Pipeline | `go pipelineSvc.Run(ctx, incoming)` | Hasta `ctx.Done()` (para debouncers). |
| Debounce | `time.AfterFunc` por chat | Hasta flush. |
| Eventos WhatsApp | `whatsmeow` (interno) | Hasta `Stop()`. |
| Scheduler | `robfig/cron` | Hasta `sched.Stop()`. |
| SSE | handler HTTP por cliente | Hasta desconexión; heartbeat 20 s. |
| Bus realtime | `realtime.Hub` en memoria | Toda la vida del proceso; fan-out a SSE del panel. |

El pipeline consume un canal con buffer 256; si se llena, el mensaje se descarta
con log (`internal/whatsapp/client.go`).

## Decisiones clave

| Decisión | Motivo |
|----------|--------|
| `whatsmeow` en lugar de `whatsapp-web.js` | Sin Chromium/Puppeteer; sesión persistente en PostgreSQL. |
| PostgreSQL + pgvector | Datos relacionales y RAG en un solo motor. |
| Tool calling nativo | Sustituye el pseudo-function-calling por string del legacy. |
| FSM de citas | Transiciones deterministas y testeables. |
| Redis para memoria/estado efímero | Historial 7 d, estado de cita 2 h, anti-spam. |
| SSE en lugar de Socket.IO | Sin dependencia de cliente JS; auth por `x-api-key`/`?key=`. |
| Bus realtime en memoria | `botd` escribe las interacciones y sirve el panel en el mismo proceso; evita Redis/NOTIFY para el chat en vivo. |
| goose embebido | Migraciones dentro del binario. |

## Deuda / límites conocidos

- `schema_response` pide JSON, pero el motor usa tool calling nativo: el
  esquema no se valida; el modelo puede devolver JSON como texto.
- `scheduler` solo recuerda citas con `scheduled_at`, que hoy nunca se rellena
  desde el flujo del asistente → no hay recordatorios reales aún.
- El filtro de intervención del personal puede auto-desactivar la IA si el bot
  envía solo media/ubicación (sin watermark de texto).
- Dedup en memoria: no coordinado entre instancias.
- Sin rate limit HTTP; solo anti-spam por usuario en Redis.
- `send_attachment` acepta rutas relativas sin confinarlas al directorio de datos.
- `fix_chat` no aplica etiqueta/pin reales (limitación de `whatsmeow`).
