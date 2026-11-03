# 04 — Asistente IA

## Componentes

| Pieza | Archivo | Función |
|-------|---------|---------|
| `Assistant` | `assistant/assistant.go` | Orquesta RAG, contexto, OpenAI y transcripción. |
| `PromptFactory` | `assistant/prompts.go` | Carga/cachea prompts y servicios desde PostgreSQL, lee mensajes fijos de `content_blocks` y compone el system prompt. |
| `context.go` | `assistant/context.go` | Contexto por usuario/tema/fecha. |
| `tools.go` | `assistant/tools.go` | Definición y ejecución de herramientas. |
| `rag.Service` | `rag/rag.go` | Embeddings y búsqueda por similitud (pgvector). |
| `i18n.Detect` | `i18n/detect.go` | Detección heurística es/en/pt. |
| `repo.History` | `store/repo/history.go` | Memoria de conversación en Redis. |

## System prompt

`PromptFactory.Compile()` concatena con separador `---`:

1. `### INSTRUCCIONES` ← prompt `prompt_instructions` (personalidad “Carla”).
2. `### SCHEMA RESPONSE` ← prompt `schema_response`.
3. `### FUNCIONES DISPONIBLES` ← generado desde `content_blocks` (mensajes fijos,
   sembrados con `make seed` desde `data/seeds/messages.json`):
   (`- si => <description> contesta => <content> y ejecuta => <actions>`).
4. `### SERVICIOS` ← catálogo de la tabla `services` (activos, agrupados por categoría). Cada línea incluye precio, duración, equipo, `description` (contexto del servicio) y `note`.
5. `### NOTAS DE SERVICIOS` ← prompt `prompt_services_notes` (avisos del catálogo).

- **Fuente de verdad**: PostgreSQL (`prompts`, `services` y `content_blocks`).
- Los `.md`/`.json` solo se usan para **sembrar** las tablas la primera vez.
- `PromptFactory` mantiene una instantánea de prompts/servicios en memoria; el
  panel `/admin` la invalida (`ReloadContent`) y reindexa el prompt editado
  (`ReindexPrompt`). Los mensajes fijos se leen de `content_blocks` en cada compilación.
- Los `content` de `messages.json` que empiezan por `/data/` se resuelven en el
  `seed` (se carga el fichero de `data/seeds/fix_messages/`).

> Deuda: `schema_response.md` describe un JSON que el motor **no** valida; con
> tool calling nativo el modelo debe responder texto plano y llamar tools.

## Contexto por turno (`buildContext`)

- Nombre/alias e idioma del cliente.
- Instrucción de responder en su idioma.
- Fecha/hora actual.
- Si es usuario nuevo: prompt `message_initial` + saludo/disclaimer.
- Si hay tema: `Tema actual: <code>` + contenido del prompt `<code>`.
- Instrucciones específicas (parámetro, hoy vacío en el flujo normal).

## Flujo `Reply`

1. `i18n.Detect(texto)`.
2. RAG: `MatchTopic` (embedding del texto + coseno en pgvector, `LIMIT 1`).
   - Si `score >= RAG_THRESHOLD` (0.78) y el código cambia → persiste
     `users.topic` y lo usa como contexto.
3. Sin cliente OpenAI → respuesta degradada.
4. Historial: últimos 20 turnos (máx. 40 guardados, TTL 7 d).
5. Bucle de chat completion (máx. 5 iteraciones), `temperature=0`,
   con `tools` disponibles:
   - Si el modelo pide tools → se ejecutan y se devuelve el resultado como
     mensaje `tool`.
   - Cuando no hay tool calls → el contenido es la respuesta final.
6. Devuelve texto, acciones, adjuntos, nombres de tools, tokens y modelo.

## RAG

- Tabla `embeddings` (`VECTOR(1536)`, índice HNSW coseno).
- Ingesta (`cmd/embed`):
  - Cada prompt activo de la tabla `prompts` con prefijo `prompt_` → `code` propio.
  - Cada entrada de `data/seeds/embeddings.json` → `code` propio.
- El panel reindexa al guardar/restaurar un prompt y borra el embedding al eliminarlo.
- Modelo: `OPENAI_EMBED_MODEL` (`text-embedding-3-small`).
- Si no hay `OPENAI_API_KEY`, RAG se omite.

> Los códigos de `data/seeds/embeddings.json` no siempre tienen fichero de prompt
> homónimo (p. ej. `prompt_courses_info`): el tema se detecta, pero el contexto
> textual puede quedar vacío.

## Herramientas (tool calling)

| Tool | Efecto | Acción resultante |
|------|--------|-------------------|
| `request_appointment_scheduling` | Registra cita con equipo (`availability` opcional). | `request_appointment_scheduling` |
| `appointment_tamara` | Registra cita con Tamara (`code_service` obligatorio). | `appointment_tamara` |
| `message_human_request` | Deriva a humano. | desactiva IA |
| `fix_chat` | Marca pendiente. | prefijo 📌 + desactiva IA |
| `notify_delay` | Aviso de retraso. | solo log (no envía) |
| `send_location` | Envía ubicación del Studio. | `SendLocation` si `LOCATION_*` |
| `send_attachment` | Adjunta fichero por ruta relativa. | se envía tras el texto |

Las citas se guardan en `appointments` + estado efímero en Redis (2 h) mediante
`appointments.Service.RecordRequest`.

## Memoria

- Clave Redis `history:<jid>`, lista JSON de turnos.
- `Append` recorta a 40 y renueva TTL (7 d).
- `Window(n)` devuelve los últimos `n` en orden cronológico.
- El historial se escribe **después** de enviar la respuesta.

## Idiomas y audio

- Detección por regex de palabras frecuentes; por defecto `es`.
- Audio sin texto: descarga y transcribe con `whisper-1` (`Transcribe`).

## Modo degradado

Sin `OPENAI_API_KEY`:
- `Reply` devuelve `*Asistente Virtual*` + `(IA no configurada) He recibido tu mensaje.`
- No hay RAG, tools ni transcripción.
- `cmd/embed` exige la clave.

## Tests

- `appointments/service_test.go`: transiciones de la FSM.
- `pipeline/service_test.go`: normalización y filtros.
- `whatsapp/client_test.go`: parseo de texto/media.
- `config`, `httpapi`: validación, auth/health y sesión del panel.
- `bootstrap`: parser del catálogo de servicios y merge con `services.json`.
