# 05 — Datos

## PostgreSQL (fuente de verdad)

Migraciones goose embebidas: `internal/store/migrations/sql/`.

### `00001_init.sql`

| Tabla | Campos clave | Notas |
|-------|--------------|-------|
| `users` | `id` (JID) PK, `name`, `alias`, `active`, `topic`, `lang`, `temperature`, `buffer_time`, `role` | `active` = IA activa para ese chat. Rol 2 = cliente. |
| `settings` | `id`, `is_active`, `activate_new_users` | Fila única; estado global del bot y activación automática de usuarios nuevos. |
| `ai_interactions` | `user_id`, `session_id`, mensajes, `language_code`, `prompt_context`, `tool_calls`, `model`, tokens, `satisfaction_level`, `latency_ms` | Analítica por turno. |
| `content_blocks` | `code` UNIQUE, `name`, `type`, `description`, `actions`, `attachments`, `properties` | Fix-messages/instrucciones. |
| `content_translations` | `block_id` FK, `language`, `content` | UNIQUE (`block_id`,`language`). |
| `system_logs` | `level`, `message`, `meta` | Disponible; el runtime usa `slog`. |
| `embeddings` | `code` UNIQUE, `topic`, `content`, `embedding VECTOR(1536)`, `model` | Índice HNSW coseno. |

### `00002_appointments.sql`

| Campo | Notas |
|-------|-------|
| `user_id`, `service`, `availability`, `team` | `team=false` ⇒ cita con Tamara. |
| `status` | CHECK: `idle`, `pending_service`, `pending_availability`, `ready`, `finish`, `cancel`. |
| `scheduled_at`, `reminder_sent` | Usados por el scheduler. |
| Índices | `(user_id, created_at DESC)`; parcial para recordatorios. |

### `00003_prompts_services.sql` y `00004_prompt_services_cleanup.sql`

| Tabla | Campos clave | Notas |
|-------|--------------|-------|
| `prompts` | `code` UNIQUE, `name`, `description`, `content`, `is_active` | Fuente de verdad del system prompt. |
| `prompt_versions` | `prompt_id` FK, `name`, `content`, `created_at` | Historial/rollback (CASCADE). |
| `services` | `id` PK, `category`, `name`, `description`, `price`, `duration_min`, `note`, `providers` JSONB, `is_active`, `sort_order` | Catálogo inyectado en el prompt. `description` = contexto libre para el modelo. |

`00004` elimina el embedding legado de `prompt_services_list`. `00005` añade
`services.description` y rellena con datos de prueba precio, duración y
descripción de las filas existentes (solo huecos vacíos). **Ese contenido es
placeholder de desarrollo**: validar precios, duraciones y textos antes de
publicar el catálogo en producción. `00006` añade
`settings.activate_new_users` (por defecto `true`) para el interruptor del panel.

### `00007_user_cascade.sql`

- Limpia huérfanos de borrados anteriores.
- FK `ON DELETE CASCADE` en `ai_interactions.user_id` y `appointments.user_id`.
- Índice `ai_interactions(session_id)`.
- Borrar un usuario elimina en una transacción: usuario, interacciones, citas y
  caché de WhatsApp (`whatsmeow_contacts`, `whatsmeow_chat_settings`). El
  historial de Redis, el anti-spam y los buffers en memoria se limpian desde la
  capa HTTP.

### Prompt de empresa

`prompt_company` sustituye a `prompt_location` y `prompt_location_contact`
(se fusionan al arrancar y los antiguos se borran con sus embeddings). Se
inyecta **siempre** en el system prompt (`### DATOS DE EMPRESA`) y no se indexa
en el RAG. La primera línea admite el pin de WhatsApp:
`<!-- pin: lat,lng | nombre | dirección -->`.

Además, al iniciar WhatsApp se crean las tablas de `whatsmeow` (sqlstore) en la
misma base: `whatsmeow_device`, `whatsmeow_identity_keys`, etc.

## Redis (estado efímero)

| Clave | Tipo | TTL | Uso |
|-------|------|-----|-----|
| `history:<jid>` | Lista JSON | 7 d | Turnos; máx. 40. |
| `appointments:<jid>` | String JSON | 2 h | Estado de cita (fallback BD). |
| `antispam:<jid>` | ZSET | 2 × ventana | Ventana deslizante de mensajes. |

## Assets

| Ruta | Contenido | Consumidor |
|------|-----------|------------|
| `data/seeds/prompts/*.md` | Seed de `prompts` (personalidad, temas, esquema, notas). | `bootstrap` en el primer arranque. |
| `data/seeds/messages.json` | Catálogo de fix-messages (código, descripción, acciones). | `cmd/seed` → `content_blocks`. |
| `data/seeds/fix_messages/*.md` | Textos largos referenciados como `/data/...`. | `cmd/seed` (resuelve en el seed). |
| `data/seeds/embeddings.json` | Catálogo semántico de temas. | `cmd/embed`. |
| `data/seeds/services.json` | Metadatos del catálogo (descripción, precio, duración, providers). | Merge con `prompt_services_list.md` al sembrar `services`. |
| `data/functions.json`, `staff.json`, `service_staff_mapping.json` | Catálogos de apoyo. | Referencia / prompts (no todos cableados). |

## Comandos de datos

```bash
make migrate-up     # esquema
make migrate-down   # revierte la última migración
make seed           # data/seeds/messages.json → content_blocks; siembra prompts/services si están vacíos
make embed          # prompts activos (BD) + data/seeds/embeddings.json → embeddings
make migrate-data   # MariaDB legacy → PostgreSQL
```

`cmd/migrate-data`:
- Migra `settings`, `users`, `content_blocks`, `content_translations`,
  `ai_interactions`, `system_logs`.
- Detecta `content_id` vs `block_id` y une traducciones por `code`.
- Flags: `-mysql`, `-truncate`, `-dry-run`.
- **No** migra Redis (historial/citas) ni `embeddings` (se regeneran).

## Notas de integridad

- `settings.SetActive` actualiza todas las filas y crea una si no existe.
- `users.id` es el JID completo (`34600...@c.us`), no el teléfono.
- `content_blocks.code` es la clave lógica usada por el prompt/acciones.
- `seed` solo crea traducción `es`; otros idiomas deben añadirse por API.
- El bootstrap de prompts/services solo inserta si la tabla está **vacía**: lo
  borrado desde `/admin` no reaparece al reiniciar.
- Los `.md` de `data/seeds/prompts/` ya no se leen en runtime; para aplicar cambios manuales
  usa el panel `/admin` o actualiza la tabla `prompts`.
- Los mensajes fijos (`FUNCIONES DISPONIBLES`) se leen en runtime desde
  `content_blocks`; `data/seeds/messages.json` solo los siembra vía `make seed`.
- Citas creadas por tools quedan sin `scheduled_at` → no generan recordatorio.
- Secretos: `.env` no se versiona; el `.env` legacy está en `.old/.env`
  (contiene una clave OpenAI real: rotar y no commitear).
