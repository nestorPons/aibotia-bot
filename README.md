# aibotia-bot

Bot de WhatsApp con asistente IA (Go) para el Studio Tamara Freitas.

- WhatsApp nativo con `whatsmeow` (sin Chromium).
- IA con OpenAI (tool calling), RAG sobre pgvector, memoria en Redis.
- API HTTP `/api/v1` + SSE para estado/QR.
- Landing pública en `/` (Go + htmx), con formulario de contacto.
- Citas con FSM y scheduler de recordatorios.
- Panel `/admin` embebido: CRUD de servicios, prompts y usuarios, bandeja de
  mensajes (ver/contestar) y vinculación de WhatsApp por QR (BD como fuente de verdad).
- El sistema anterior (Node/MariaDB) está en `.old/` y no forma parte del build.

Documentación completa: [`docs/`](docs/README.md).

## Requisitos

- Go 1.27+ · PostgreSQL 16 + `pgvector` · Redis 7 · `OPENAI_API_KEY`.
- Alternativa: Docker + Compose.

## Desarrollo local (Docker)

```bash
cp .env.example .env          # opcional: hay defaults de desarrollo
docker compose up -d          # postgres + redis + botd (código montado)
docker compose logs -f botd
```

- API: `http://localhost:3000` · Panel: `http://localhost:3000/admin` · API key de dev: `dev-api-key`.
- `botd` se compila con `Dockerfile.dev` **al arrancar** el contenedor.
- Tras cambiar código Go o el panel (`internal/httpapi/admin/*.html`):
  `make dev-apply` (recompila y reinicia `botd`) — sin reconstruir la imagen.
- Solo dependencias: `docker compose up -d postgres redis` y luego `make run`.
- Limpiar datos: `docker compose down -v`.

Guía completa: [`docs/07-operacion.md`](docs/07-operacion.md).
#### Accesi a la bbdd

```bash
lazysql "postgres://aibotia:aibotia@localhost:5432/aibotia?sslmode=disable"
```

## Puesta en marcha sin Docker

```bash
cp .env.example .env          # API_KEY, DATABASE_URL, REDIS_URL, OPENAI_API_KEY
docker compose up -d postgres redis

make migrate-up               # esquema
make seed                     # bloques de contenido
make embed                    # embeddings del RAG
make run                      # botd
```

Vincular WhatsApp: escanear el QR de los logs o `GET /api/v1/whatsapp/qr`.

## Docker (producción)

```bash
make docker-up
docker compose -f deploy/docker-compose.yml logs -f botd
```

## Comandos

| Comando | Acción |
|---------|--------|
| `make run` / `make build` | Ejecutar / compilar `botd` |
| `make test` / `make vet` / `make fmt` | Calidad |
| `make migrate-up` / `make migrate-down` | Migraciones |
| `make seed` / `make embed` | Contenido (BD) / RAG |
| `make migrate-data` | MariaDB legacy → PostgreSQL |
| `make loadtest` | Prueba de carga HTTP |
| `make tester` / `make tester-case` / `make tester-suite` | Bot de pruebas (casos rotativos, ver [`docs/12-bot-pruebas.md`](docs/12-bot-pruebas.md)) |
| `make tester-list` | Lista los casos de prueba disponibles |
| `make tester-serve` | Sirve los logs HTML de las pruebas en `http://127.0.0.1:8091` |
| `make docker-up` / `make docker-down` | Compose |
| `make dev-build` / `make dev-apply` | Compilar / recompilar+reiniciar `botd` en dev sin `--build` |

## Bot de pruebas (tester)

Cliente WhatsApp independiente que **conversa** con el chatbot usando **casos
rotativos** (clienta habitual, casual, adversarial, caza de promociones y
precios ambiguos, maleducado, muchas dudas, otro sector, urgente fuera de
horario e inglés). El test es una conversación
entre dos LLM: uno hace de cliente con el rol del caso y el otro es el bot real;
el cliente recibe cada respuesta del bot y reacciona a ella.

Requisitos: un número de WhatsApp distinto al del bot, la base
`aibotia_tester` y `OPENAI_API_KEY` (ver
[`docs/12-bot-pruebas.md`](docs/12-bot-pruebas.md)).

```bash
# 1) Ver los casos disponibles (no necesita WhatsApp ni BD)
make tester-list

# 2) Ejecutar UNA sesión con un caso concreto
make tester-case CASE=adversarial TESTER_FLAGS=-eval
make tester-case CASE=maleducado
make tester-case CASE=muchas_dudas TESTER_FLAGS="-turns 8 -seed 42"

# 3) Ejecutar un caso aleatorio (evita el último usado)
make tester

# 4) Suite completa (tarda ~35-55 min por el anti-spam de 15 s)
make tester-suite SUITE=all TESTER_FLAGS=-eval
```

| Comando | Qué hace |
|---|---|
| `make tester-list` | Lista `id`, nombre, turnos, setup y objetivo de cada caso; marca el último usado. |
| `make tester-case CASE=<id>` | Ejecuta **solo** ese caso. Falla con ayuda si falta `CASE`. |
| `make tester` | Caso aleatorio entre los 9, sin repetir el de la sesión anterior. |
| `make tester-suite SUITE=all` | Ejecuta varios casos (`all` o `id1,id2`). |
| `make tester-serve` | Abre el visor de logs guardados (`127.0.0.1:8091`). |

- Cada ejecución es una sesión independiente: `-fresh` (por defecto) limpia el
  historial del bot y reactiva la IA antes de empezar.
- `-eval` puntúa cada respuesta (con la conversación previa como contexto) y
  genera veredicto (`pass`/`fail`).
- `-mode scenario` es un modo heredado sin IA: banco fijo que **no reacciona**
  al bot; solo para smoke tests.
- `-seed N` reproduce una sesión exacta; `-delay` ajusta la pausa entre mensajes.
- Reportes en `data/test/reports/`: JSON + Markdown + **informe técnico HTML** y
  **página de cliente HTML** (solo el chat, en burbujas, sin datos internos) para
  enseñar, enviar por WhatsApp o imprimir en PDF. `make tester-serve` abre un
  índice navegable: el enlace principal lleva a la página de cliente y los
  botones Técnico/MD/JSON al resto.

Detalle completo de flags y reportes: [`docs/12-bot-pruebas.md`](docs/12-bot-pruebas.md).

## Variables mínimas

`API_KEY`, `DATABASE_URL`, `REDIS_URL`, `OPENAI_API_KEY`.
Panel: `ADMIN_PASSWORD` (en dev vacío ⇒ usa `API_KEY`; en producción vacío ⇒ panel deshabilitado).
El resto (modelo, RAG, debounce, anti-spam, recordatorios, ubicación) está en
[`docs/07-operacion.md`](docs/07-operacion.md).

## Estructura

```
cmd/        botd, migrate, seed, embed, migrate-data, loadtest, tester
internal/   pipeline, assistant, whatsapp, httpapi (+admin), bootstrap, actions, appointments, store...
data/       seeds (prompts, servicios, fix_messages, embeddings) + catálogos de referencia
deploy/     Dockerfile, compose, CUTOVER.md
docs/       documentación del sistema actual
.old/       legacy Node/MariaDB (ignorado)
```

Cutover desde el sistema anterior: [`deploy/CUTOVER.md`](deploy/CUTOVER.md).