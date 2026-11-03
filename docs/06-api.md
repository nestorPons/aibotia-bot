# 06 — API HTTP

## Convenciones

- Base: `/api/v1`.
- Auth: cabecera `x-api-key` (comparación constant-time). Públicos: `/health`, `/ready`.
- SSE acepta también `?key=<API_KEY>`.
- JSON UTF-8; error uniforme:
  ```json
  { "error": { "code": "not_found", "message": "Usuario no encontrado" } }
  ```
- Paginación: `?page=1&perPage=20` (máx. 200 en usuarios, 500 en mensajes).
- Timeout global de petición: 60 s (las rutas SSE de `/events` están exentas).
- CORS: `CORS_ALLOWED_ORIGINS` (coma-separado; `*` = cualquiera).
- Rutas `GET`/`PUT`/`DELETE`; no hay versionado adicional.

## Endpoints

### Salud (públicos)

| Método | Path | Respuesta |
|--------|------|-----------|
| GET | `/health` | `{"status":"ok","version":"..."}` |
| GET | `/ready` | `{"status":"ok|degraded","dependencies":{"postgres":"ok","redis":"ok"}}` (503 si falla) |

### WhatsApp

| Método | Path | Descripción |
|--------|------|-------------|
| GET | `/whatsapp/status` | `{enabled,status,qr_available}` |
| GET | `/whatsapp/qr` | `{status,qr}` |
| GET | `/whatsapp/contact?jid=` | Datos del contacto |
| POST | `/whatsapp/logout` | Cierra sesión (requiere re-QR) |

### Asistente

| Método | Path | Body / respuesta |
|--------|------|------------------|
| GET | `/assistant/status` | `{"active":true}` |
| PUT | `/assistant/status` | `{"active":false}` → `{success,active}` |

### Usuarios

| Método | Path | Notas |
|--------|------|-------|
| GET | `/users?search=&ai=&page=&perPage=` | `{data,meta{total,page,perPage,lastPage}}`; `ai=active\|inactive` filtra por bot activo |
| POST | `/users/import-users` | Importa contactos de WhatsApp (`{success,count,imported}`) |
| GET | `/users/{id}` | Usuario por JID |
| PUT | `/users/{id}` | `{active?,name?,topic?}` |
| DELETE | `/users/{id}` | Borra el usuario y **todos** sus datos; devuelve `{messages,appointments,contacts}` |
| DELETE | `/users/{id}/history` | Limpia memoria Redis |
| GET | `/users/{id}/conversation` | `{user,conversation[]}` (50 interacciones) |
| POST | `/users/{id}/force-reply` | `{"text":"..."}`; salta filtros y envía respuesta |
| POST | `/users/{id}/test-mode` | `{"enabled":true}`: modo prueba (no desactiva la IA al derivar y añade nota); al activarlo reactiva la IA |

### Mensajes / instrucciones

| Método | Path | Notas |
|--------|------|-------|
| GET | `/messages?page=&perPage=` | Interacciones IA globales |
| GET | `/messages/{id}` | Interacciones de un usuario (100) |
| GET | `/instructions` | Bloques + traducciones |
| GET | `/instructions/{code}` | Bloque por `code` |
| PUT | `/instructions/{code}` | `{block:{...},translations:[{language,content}]}` (reemplaza traducciones) |
| DELETE | `/instructions/{code}` | Borra bloque (CASCADE traducciones) |

### Panel de administración (`/admin`)

Auth propia por **sesión** (cookie HMAC `aibotia_admin`), independiente de `x-api-key`.
Mutaciones exigen cabecera `X-Admin-Request: 1` (protección CSRF básica).

| Método | Path | Descripción |
|--------|------|-------------|
| GET | `/admin/login` | Formulario de acceso. |
| POST | `/admin/login` | `password` (form) + `name` opcional (operador) → cookie de sesión. |
| POST | `/admin/logout` | Cierra sesión. |
| GET | `/admin` | UI embebida (servicios, prompts, mensajes, usuarios y WhatsApp). |
| GET | `/admin/api/services` | Lista catálogo (incluye inactivos). |
| POST | `/admin/api/services` | Crea servicio. |
| GET/PUT/DELETE | `/admin/api/services/{id}` | Obtiene/edita/borra servicio. |
| GET | `/admin/api/prompts` | Lista prompts (sin contenido). |
| POST | `/admin/api/prompts` | Crea prompt + versión inicial. |
| GET/PUT/DELETE | `/admin/api/prompts/{code}` | Obtiene/edita/borra prompt. |
| GET | `/admin/api/prompts/{code}/versions` | Historial de versiones. |
| POST | `/admin/api/prompts/{code}/versions/{id}` | Restaura una versión. |
| GET/PUT | `/admin/api/prompts/prompt_company` | Datos de empresa (pestaña **Empresa**); se inyectan siempre en el system prompt. |
| GET | `/admin/api/users` | Usuarios paginados (`page`,`perPage`,`search`,`ai=active\|inactive`). |
| POST | `/admin/api/users` | Crea usuario (id = JID). |
| POST | `/admin/api/users/import` | Importa contactos de WhatsApp (`whatsmeow_contacts`) a usuarios; no sobreescribe existentes. |
| GET/PUT | `/admin/api/users/{id}` | Obtiene/edita usuario. |
| DELETE | `/admin/api/users/{id}` | Borra usuario, interacciones, citas, historial Redis, anti-spam y caché WhatsApp; devuelve contadores. |
| POST | `/admin/api/users/{id}/active` | `{active}` → activa/desactiva la IA para el usuario. |
| GET/PUT | `/admin/api/settings` | Ajustes del bot; hoy `{activate_new_users}` → activa la IA en usuarios nuevos. |
| GET | `/admin/api/messages` | Bandeja: última interacción por usuario (incluye `active` y `needs_operator`). Filtros: `search`, `needs_operator=1`, `ai=active|inactive`. |
| GET | `/admin/api/messages/{id}` | Conversación de un usuario + sus datos. |
| GET | `/admin/api/messages/{id}/summary` | Resumen para el operador: datos de cita + puntos clave de los últimos mensajes. |
| POST | `/admin/api/messages/{id}/send` | `{text}` → envía respuesta manual por WhatsApp, prefijada con `*<nombre operador>*`; desactiva la IA del usuario. |
| GET | `/admin/api/whatsapp` | Estado de conexión + `qr_available`/`paired`. |
| GET | `/admin/api/whatsapp/qr.png` | Código QR de vinculación en PNG. |

- Guardar/restaurar un prompt invalida la caché del asistente y reindexa su
  embedding si el código empieza por `prompt_`.
- Borrar un prompt elimina también su embedding.
- Editar servicios refresca el catálogo inyectado en el system prompt.
- `needs_operator` se calcula sobre la última interacción del hilo: `true` si
  incluye una tool de derivación (`request_appointment_scheduling`,
  `appointment_tamara`, `message_human_request`, `fix_chat`). Una respuesta
  manual registra una nueva interacción (`model=manual`) y limpia la marca.
- El resumen (`/summary`) combina la cita registrada (`service`, `availability`,
  `status`, `team`) con 2-4 puntos generados por el modelo a partir de los
  últimos mensajes (`source=ai`). Si OpenAI falla o no hay API key devuelve un
  resumen determinista (`source=fallback`). Se cachea 10 min por hilo/última
  interacción.
- Los servicios aceptan `description` (texto libre): se concatena al nombre en el
  prompt para dar contexto al modelo; vacío = no se muestra.
- El nombre que firma los envíos manuales sale del campo del login; si está vacío
  se usa `ADMIN_OPERATOR_NAME` (`.env`). La primera letra de cada palabra se
  muestra en mayúscula.
- Enviar un mensaje manual desde el panel desactiva automáticamente la IA de ese
  usuario (`users.active = false`).

### Realtime (SSE)

```
GET /api/v1/events?key=<API_KEY>
event: status   data: {"status":"connected"}
event: qr       data: {"qr":"2@..."}
```

- Emite al conectar y cada 2 s si cambia estado/QR.
- `X-Accel-Buffering: no` para proxies.

### Realtime del panel (SSE)

```
GET /admin/api/events        (cookie de sesión del panel)
event: incoming  data: {"type":"incoming","user_id":"…","at":"…","data":{"text":"…","has_media":false,"from_me":false}}
event: message   data: {"type":"message","user_id":"…","at":"…","data":{…interacción persistida…}}
event: typing    data: {"type":"typing","user_id":"…","data":{"typing":true}}
```

- `incoming`: mensaje recibido de WhatsApp antes de persistir (incluye texto/medio).
- `message`: interacción insertada en `ai_interactions` (respuesta del bot u
  operador); el cliente la pinta en el hilo abierto y refresca la bandeja.
- `typing`: el bot está generando respuesta (se apaga al terminar).
- `retry: 3000` y heartbeat cada 20 s. Las rutas SSE quedan fuera del timeout
  global de 60 s y renuevan el `WriteTimeout` en cada escritura.

## Códigos de error usados

| HTTP | `code` | Caso |
|------|--------|------|
| 400 | `invalid_body`, `missing_jid` | JSON/parámetro inválido. |
| 403 | `unauthorized` | API key ausente/incorrecta. |
| 404 | `not_found` | Usuario/bloque inexistente. |
| 500 | `*_failed`, `internal_error` | Fallo interno/BD. |
| 502 | `force_reply_failed`, `contact_failed`, `logout_failed` | Fallo de dependencia/WhatsApp. |
| 503 | `whatsapp_disabled`, `pipeline_unavailable` | Función deshabilitada. |

## Notas

- `force-reply` no persiste el turno de usuario (solo el de asistente).
- No hay rate limit HTTP; el único control de abuso es el anti-spam de mensajes.
- La key por query en SSE puede aparecer en logs de proxies: preferir cabecera.
