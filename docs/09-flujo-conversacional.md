# 09 — Flujo conversacional end-to-end (primer mensaje → respuesta)

> Objetivo: radiografía del recorrido real y de cada punto de decisión para
> auditar el comportamiento actual y priorizar mejoras.
> Fuente: código Go vigente + comparación puntual con `.old/` (Node).

## 1. Mapa general

```
WhatsApp
  │ events.Message
  ▼
whatsapp.Client.emit                 normaliza + cachea media
  │ IncomingMessage (canal 256)
  ▼
pipeline.handle                      filtros de entrada
  │ debounce por chat
  ▼
pipeline.flush                       agrupa, normaliza, transcribe
  │
  ├─► user / historial / RAG ──► assistant.Reply
  │                                │ system prompt + contexto + tools
  │                                ▼
  │                              OpenAI (máx. 5 iteraciones)
  │                                │ texto / tool calls
  │                                ▼
  │                              parseStructuredReply
  ▼
actions.Run                          efectos laterales
  │
  ▼
SendTyping → SendText(*Asistente Virtual* + watermark) → adjuntos → SendTyping OFF → MarkSeen
  │
  ▼
history.Append(user + assistant) → interactions.Log
```

## 2. Fase 1 — Recepción (`whatsapp/client.go`)

| Paso | Qué ocurre |
|------|------------|
| Evento | `events.Message` de whatsmeow. |
| Media | Si el mensaje lleva media se guarda en memoria (`mediaCache`, máx. 128). |
| Texto | `Conversation`, texto extendido o caption de imagen/vídeo/documento. |
| Tipo | `image`, `video`, `audio`, `document`, `sticker` o vacío. |
| Salida | `IncomingMessage{ID, Chat, Sender, PushName, Text, Timestamp, FromMe, HasMedia, MediaType}`. |
| Cola | Canal de 256; si está lleno el mensaje se descarta con warning. |

> El cache de media es volátil: tras reiniciar, un mensaje antiguo ya no se
> puede descargar (`DownloadMedia` falla).

## 3. Fase 2 — Portero de entrada (`pipeline.handle`)

Orden exacto de decisiones. La primera que corta, termina el mensaje.

| # | Condición | Efecto | Log |
|---|-----------|--------|-----|
| 1 | `ID` ya visto en 10 min | Ignora (dedup en memoria). | — |
| 2 | Chat `@g.us`, `@broadcast` o `status@broadcast` | Ignora. | — |
| 3 | `FromMe` + watermark `\u200B` | Ignora (es el propio bot). | — |
| 4 | `FromMe` sin watermark + usuario activo | Desactiva la IA de ese chat (intervención del personal). | `IA desactivada por intervención del personal` |
| 5 | `settings.is_active == false` | Ignora todo. | — (errores de lectura se asumen como activo) |
| 6 | Usuario inexistente | `FindOrCreate(jid, pushName, active=settings.activate_new_users)` (por defecto `true`); alias = pushName. | — |
| 7 | Usuario inactivo | Intenta reactivar si la última interacción supera `HOURS_TO_ACTIVE` (168 h). Si no, ignora. | `usuario inactivo; mensaje ignorado` |
| 8 | Anti-spam | Ventana Redis: `> SPAM_LIMIT` (5) en `SPAM_WINDOW` (60 s). Si supera, ignora. | `spam detectado; mensaje ignorado` |
| 9 | Resto | Pasa a `debouncer.add`. | — |

Notas de comportamiento:

- Todo `FromMe` termina la ejecución, aunque no haya usuario o esté inactivo.
- La reactivación automática puede revertir una desactivación manual del
  personal pasadas 168 h desde la última interacción.
- El anti-spam **falla abierto**: si Redis devuelve error, el mensaje continúa.
- No se comprueba la antigüedad del mensaje (el legacy descartaba > 60 s).
- `settings.IsActive` devuelve `true` si no hay fila o hay error de BD.

## 4. Fase 3 — Agrupación, normalización y audio (`pipeline/debounce.go`, `flush`)

| Paso | Regla |
|------|-------|
| Debounce | Timer por chat; cada mensaje lo reinicia. Espera = `BUFFER_MIN_MS` (1 s) + 1,5 s por mensaje extra, tope `BUFFER_MAX_MS` (15 s). |
| Normalización | Une textos con `\n`; ignora vacíos. |
| Cortesías | Si hay > 1 parte, elimina mensajes que sean solo “gracias”, “ok”, “vale”, “perfecto”, emojis, etc. |
| Media | Marca `hasMedia` y conserva el primer `mediaType`. |
| Audio | Solo si no hay texto y el primer tipo es `audio`: descarga y transcribe con `whisper-1`. |
| Error audio | Sigue con texto vacío. |
| Contexto | `ctx` con timeout de 120 s. |
| Reinicio | El buffer vive en memoria: se pierde al apagar. |

## 5. Fase 4 — Usuario e “IsNew”

1. `users.Get` ya devolvió el usuario en el portero.
2. Antes de responder, se consulta `history.Window(chatID, 1)`.
3. `IsNew = true` si no hay ningún turno guardado; si Redis falla, `IsNew = false`.
4. El flag no se persiste: solo decide si el contexto pide saludo inicial +
   disclaimer (`message_initial.md`).

## 6. Fase 5 — Ensamblado del prompt (`assistant`)

### 6.1 System prompt (fijo en arranque)

`PromptFactory.Compile()` concatena con `---`:

| Bloque | Fuente |
|--------|--------|
| Personalidad y normas | `prompt_instructions` (tabla `prompts`) |
| `### DATOS DE EMPRESA` | `prompt_company` (siempre, sin RAG) |
| `### SCHEMA RESPONSE` | `schema_response` (tabla `prompts`) |
| `### FUNCIONES DISPONIBLES` | `content_blocks` (texto informativo) |
| `### SERVICIOS` | tabla `services` |

- Los prompts/servicios se cachean en memoria; el panel `/admin` invalida la caché.
- Los mensajes fijos de `content_blocks` se convierten en líneas
  `- si => <description> contesta => <content> y ejecuta => [actions]`.
- Esas “actions” son **texto**, no herramientas nativas.

### 6.2 Contexto por turno (`buildContext`)

```
Nombre del cliente: <user.Alias>
Idioma del cliente: <user.Lang>
Contesta en el idioma del cliente.
Fecha actual: <fecha/hora>
[si IsNew] Es un usuario nuevo: incluye saludo inicial y disclaimer
           + contenido del prompt message_initial
[si topic] Tema actual: <code>
           Información relevante: contenido del prompt <code>
[si instructions] Instrucciones específicas (hoy siempre vacío)
```

> `user.Lang` no se persiste al detectar el idioma; normalmente llega vacío.

### 6.3 Historial y mensajes enviados a OpenAI

| Elemento | Detalle |
|----------|---------|
| Historial | Últimos 20 turnos de Redis (se guardan 40, TTL 7 días). |
| Orden | `system` → turnos usuario/asistente → turno actual. |
| Turno actual | `contextStr + "\n\nUsuario: " + texto`. |
| Temperatura | `0` fija (se ignora `users.temperature`). |
| Tools | Las 7 funciones de `tools.go`. |

## 7. Fase 6 — Motor LLM y bucle de herramientas

| Decisión | Comportamiento |
|----------|----------------|
| Sin `OPENAI_API_KEY` | Respuesta degradada `*Asistente Virtual*` + `(IA no configurada) He recibido tu mensaje.` sin RAG ni tools. |
| RAG activo | `MatchTopic`: embedding del texto + coseno en pgvector. Si `score >= 0.78` y cambia el tema, persiste `users.topic`. |
| Sin tool calls | El contenido del mensaje es la respuesta final. |
| Con tool calls | Ejecuta cada tool, añade el resultado como mensaje `tool` y repite. |
| Tope | 5 iteraciones. Al agotarlas responde `Disculpa, ¿puedes repetirme tu consulta?`. |
| Tokens | Se conserva solo el `usage` de la última iteración. |
| Acciones | Se acumulan entre iteraciones (puede haber duplicados). |

### 7.1 Herramientas reales

| Tool | Argumentos | Efecto en `execTool` | Acción devuelta | Efecto en `actions.Run` |
|------|------------|----------------------|-----------------|--------------------------|
| `request_appointment_scheduling` | `code_service`, `availability` | Inserta en `appointments` + Redis. | `request_appointment_scheduling` | No tiene `case`: solo debug. |
| `appointment_tamara` | `code_service` obligatorio | Inserta cita `team=false`. | `appointment_tamara` | No tiene `case`: solo debug. |
| `message_human_request` | — | Nada. | `message_human_request` | Desactiva la IA del usuario (salvo en modo prueba). |
| `fix_chat` | — | Nada. | `fix_chat` | Prefija `📌` y desactiva la IA (salvo en modo prueba). |
| `notify_delay` | — | Nada. | `notify_delay` | Solo log. |
| `send_location` | — | Nada. | `send_location` | `SendLocation` con el pin de `prompt_company` (fallback `LOCATION_*`). |
| `send_attachment` | `path` | Nada. | `send_attachment` + ruta | Envía la ruta indicada por el modelo, sin validar. |

> Los mensajes fijos incluyen acciones como `message_menu`,
> `message_instant_human_attention` o `appointment_adjustment` que **no son
> tools**. El modelo no puede ejecutarlas; como mucho copiar su texto.

## 8. Fase 7 — Interpretación de la respuesta

`parseStructuredReply` busca el primer `{` y el último `}` y deserializa:

| Campo | Uso real |
|-------|----------|
| `respond` | Texto que se envía. |
| `language_code` | Sustituye al detectado por `i18n`. |
| `satisfaction_level` | Se guarda en analítica si es `> 0`. |
| `function` | **Ignorado**. |
| `appointment.*` | **Ignorado** (no alimenta la FSM). |
| `client_name` | **Ignorado** (no actualiza el usuario). |
| `staff` | **Ignorado**. |

Si no es JSON válido o falta `respond`, se usa el texto crudo tal cual.

> Hay una tensión de diseño: el system prompt pide JSON, pero la ejecución real
> depende de tool calling nativo. El JSON solo aporta el texto final.

## 9. Fase 8 — Acciones antes del envío (`actions/runner.go`)

Se ejecutan **antes** de enviar. Pueden modificar `reply.Text` o enviar cosas.

| Acción | Resultado observable |
|--------|----------------------|
| `fix_chat` | `reply.Text = "📌 " + texto` (si no empezaba ya por 📌) + `active=false`. En modo prueba mantiene la IA activa y añade una nota. |
| `message_human_request` | `active=false`. En modo prueba mantiene la IA activa y añade una nota. |
| `notify_delay` | Solo log; no envía nada. |
| `send_location` | Envía el pin de `prompt_company`; si falta, fallback `LOCATION_*`; si no hay, warning. |
| `send_attachment` | Sin efecto aquí; los adjuntos ya van en `reply.Attachments`. |

> **Modo prueba** (`users.test_mode`): lo activa el tester antes de cada
> conversación. En las derivaciones no desactiva la IA y añade
> `🧪 Nota de prueba: en producción, el bot se desactivaría en este punto para
> que atendiera un operador.` Al terminar la prueba se desactiva.

El título va antes de las acciones: `fix_chat` produce `*Asistente Virtual*` seguido de `📌 …`.

## 10. Fase 9 — Envío (`pipeline.flush`)

| Paso | Regla |
|------|-------|
| Respuesta vacía | Si `Text == ""` y no hay adjuntos → no envía nada y **no guarda historial ni analítica**. |
| Typing | `SendTyping(true)`. |
| Texto | `*Asistente Virtual*` + salto de línea + texto (si no lleva ya el título) + watermark `\u200B`. |
| Adjuntos | Se leen de disco y se envían como imagen o documento. |
| Cierre | `SendTyping(false)` y `MarkSeen(last.Sender, last.ID)`. |
| Errores | Fallo de texto: log y se siguen intentando adjuntos. Fallo de adjunto: warning. |

## 11. Fase 10 — Persistencia y analítica

| Dato | Momento | Observación |
|------|---------|-------------|
| `users.topic` | Durante RAG (antes de responder). | Solo si cambia y supera umbral. |
| `history` usuario | Después de enviar. | Si no hay respuesta, el turno del usuario se pierde. |
| `history` asistente | Después de enviar. | Texto sin título ni watermark. |
| `ai_interactions` | Después de enviar. | Incluye tokens, tools, latencia, idioma, satisfacción. |
| Adjuntos | No se registran en analítica. | Solo el texto. |

No hay transacción entre envío y persistencia: un fallo de BD después del envío
pierde el registro, no el mensaje.

### 11.1 Respuestas fijas y plantillas

| Situación | Texto/plantilla | Origen |
|-----------|-----------------|--------|
| Usuario nuevo | Saludo, “Soy Carla…”, petición de consulta, “escribe *Agente*” y disclaimer. | `message_initial` (vía contexto) |
| IA no configurada | `*Asistente Virtual*` + `(IA no configurada) He recibido tu mensaje.` | `assistant.Reply` |
| Bucle de tools agotado | `Disculpa, ¿puedes repetirme tu consulta?` | `assistant.complete` |
| Derivación a humano | “Te paso con una persona del equipo… horario… disculpa las molestias.” | mensajes fijos / prompt |
| Solicitud de cita | “Perfecto, paso tu solicitud al equipo para que revisen disponibilidad…” | `prompt_instructions` |
| Cita con Tamara | Mensaje de plazas limitadas + contacto del equipo. | `prompt_instructions` |
| Confirmación de cita | “Gracias por confirmar tu cita, te esperamos…” | `prompt_instructions` |
| Recordatorio | `REMINDER_TEMPLATE` (`📅 Hola {name}… {date}…`). | `scheduler` |
| Cierre natural | Sin texto (no se envía nada). | Prompt + `flush` |
| Error de LLM | Sin texto: el usuario no recibe respuesta. | `flush` (gap) |

## 12. Árbol de decisión resumido

```
¿Mensaje del bot? ──sí──► ignorar
        │no
¿Grupo/broadcast? ──sí──► ignorar
        │no
¿Bot global activo? ──no──► ignorar
        │sí
¿Usuario activo? ──no──► ¿>168 h? ──sí──► reactivar
        │                        └──no──► ignorar
        sí
¿Spam? ──sí──► ignorar
        │no
 debounce ──► ¿solo audio? ──sí──► Whisper
        │no
 RAG: ¿tema con score ≥ 0.78? ──sí──► persistir topic
        │
 OpenAI ──► ¿tool call? ──sí──► ejecutar y repetir (máx. 5)
        │no
 ¿respond vacío? ──sí──► cierre silencioso
        │no
 acciones ──► enviar texto + adjuntos ──► historial + analítica
```

## 13. Escenario: primer mensaje paso a paso

Ejemplo: `"Hola, quiero información de cejas"`.

| t | Etapa | Resultado |
|---|-------|-----------|
| 0 s | WhatsApp | `events.Message`; texto normalizado. |
| 0 s | Portero | No es bot/grupo; bot activo; usuario no existe → se crea `active=true`. |
| 0 s | Debounce | Programa flush a ~1 s. |
| 1 s | Normalización | `text = "Hola, quiero información de cejas"`, sin media. |
| 1 s | IsNew | Historial vacío → `true`. |
| 1 s | RAG | Puede detectar `prompt_eyebrow_services` o similar y persistir el tema. |
| 1 s | Contexto | Saludo inicial + disclaimer + tema + información del servicio. |
| 1-3 s | LLM | Responde texto (posible llamada a tool si pide cita). |
| 3 s | Acciones | Si `fix_chat`, añade 📌 y desactiva. |
| 3 s | Envío | `*Asistente Virtual*` + texto + watermark. |
| 3 s | Persistencia | Turnos user/assistant + interacción. |

Variantes relevantes:

| Entrada | Decisión |
|---------|----------|
| “Quiero cita” sin servicio | El prompt pide servicio; `status` del schema se ignora. |
| “Quiero cita con Tamara” | Debe llamar `appointment_tamara` con `code_service`. |
| “Quiero cita el martes a las 16:00” | El prompt prohíbe confirmar hora; debe derivar al equipo. |
| “Gracias, perfecto” | Debe cerrar sin texto; el pipeline no envía ni guarda. |
| “Agente” | Debe derivar a humano (`message_human_request` o texto fijo). |
| Solo audio | Se transcribe y sigue el flujo normal. |
| Sin API key | Respuesta degradada; no hay tools. |

## 14. Flujo de citas

### 14.1 FSM definida (`appointments/service.go`)

| Estado | Significado | Evento → estado |
|--------|-------------|-----------------|
| `idle` | Sin intención. | `provide_service` → `pending_availability` |
| `pending_service` | Falta servicio. | `provide_service` → `pending_availability`; `provide_availability` → `ready` |
| `pending_availability` | Falta horario. | `provide_availability` → `ready` |
| `ready` | Servicio + disponibilidad. | `confirm`/`finish` → `finish` |
| `finish` | Cerrada. | — |
| `cancel` | Cancelada. | — |

### 14.2 Uso real

| Aspecto | Realidad |
|---------|----------|
| `NextStatus` | Solo se usa en tests; el pipeline no lo invoca. |
| `RecordRequest` | Calcula el estado directamente: sin servicio → `pending_service`; sin disponibilidad → `pending_availability`; ambos → `ready`. |
| Persistencia | Inserta en `appointments` y guarda estado en Redis 2 h. |
| Schema `appointment` | El modelo lo rellena, pero el código lo descarta. |
| Agenda | El bot nunca confirma horas/fechas; el equipo revisa. |

## 15. Recordatorios (`scheduler`)

| Paso | Regla |
|------|-------|
| Cron | `REMINDER_CRON` (`*/10 * * * *`). |
| Selección | Citas `status='ready'`, `reminder_sent=false`, `scheduled_at` en las próximas 24 h. |
| Envío | `SendText` con plantilla; solo se sustituye `{date}`. |
| Marca | `reminder_sent=true` tras envío correcto. |

> La plantilla por defecto incluye `{name}`, pero el scheduler **no** lo
> sustituye: se envía literal.

## 16. Camino administrativo: `force-reply`

| Diferencia vs flujo normal | Detalle |
|----------------------------|---------|
| Filtros | No pasa por dedup, grupos, `FromMe`, estado global, spam ni debounce. |
| Usuario | Debe existir; si no, error. |
| Generación | Mismo `assistant.Reply` + `actions.Run`. |
| Envío | Solo texto; **no envía adjuntos**. |
| Persistencia | Solo guarda el turno del asistente; no registra `ai_interactions`. |
| Respuesta HTTP | Devuelve el texto sin el título (se añade solo al enviar). |

## 17. Legacy vs actual (decisiones clave)

| Tema | Legacy Node | Go actual |
|------|-------------|-----------|
| Prefijo de bot | `🤖 ` en todo mensaje saliente | Título `*Asistente Virtual*` en mensajes salientes (no en adjuntos) |
| Markdown | `cleanMarkdown` convertía encabezados en 📌 | No existe; se envía el texto del modelo tal cual |
| Antigüedad | Descartaba mensajes > 60 s | No se comprueba |
| Debounce | 20 s base, `buffer_time` por usuario | 1 s + 1,5 s/mensaje, configuración global |
| Anti-spam | Memoria, ban inoperante | Redis, ventana deslizante, falla abierto |
| Citas | LLM decidía | Tools + FSM definida pero no conectada |
| Menú | Detector no cableado | Sin detector |
| Adjuntos | Acción `message_capilar` | Tool `send_attachment` con ruta del modelo |
| Recordatorios | No implementados | Cron + tabla, con bug `{name}` |
| Memoria | Redis por chat | Redis por chat, 40 turnos / 7 días |

## 18. Oportunidades de mejora (priorizadas)

### P0 — riesgo operativo o de seguridad

| Hallazgo | Evidencia | Propuesta |
|----------|-----------|-----------|
| `send_attachment` acepta cualquier ruta del modelo | `assistant/tools.go` + `pipeline.sendAttachment` | Lista blanca de directorio/extensiones y canonicalización. |
| La reactivación automática revierte bajas humanas | `tryReactivate` a las 168 h | Distinguir `active` de `paused_by_staff` o no reactivar si hay intervención. |
| Silencio ante error del LLM o respuesta vacía | `flush` retorna sin enviar | Mensaje de fallback y registro del turno del usuario. |
| El turno del usuario se pierde si no hay respuesta | `history.Append` posterior al envío | Guardar el turno al entrar en `flush`, antes de llamar al LLM. |
| `{name}` literal en recordatorios | `scheduler.runReminders` | Sustituir también `{name}` o quitarlo de la plantilla. |

### P1 — coherencia funcional

| Hallazgo | Evidencia | Propuesta |
|----------|-----------|-----------|
| `schema_response` y el motor no están alineados | `function`, `appointment`, `client_name`, `staff` se ignoran | O eliminar esos campos del schema, o parsearlos y aplicarlos. |
| Acciones del catálogo no son tools | `content_blocks` vs `buildTools` | Convertirlas en tools o resolverlas con un dispatcher determinista. |
| Adjuntos de `messages.json` nunca se envían | Campo `attachments` no usado | Cargarlos al ejecutar la acción correspondiente. |
| El idioma detectado no se persiste | `Reply` no actualiza `users.lang`; `buildContext` lee vacío | Persistir `lang` o usar `req.Language` en el contexto. |
| Tema RAG sin prompt homónimo | `data/seeds/embeddings.json` vs `prompts` | Validar que cada `code` tenga prompt o usar `additional_content`. |
| `temperature` y `buffer_time` por usuario sin uso | `users` los guarda; el pipeline usa valores fijos | Cablearlos o retirarlos del modelo de datos. |
| Falta formateo tipo `cleanMarkdown` | No hay equivalente Go | Formateador de salida (encabezados, listas, negritas WhatsApp). |
| `force-reply` omite adjuntos y analítica | `ForceReply` | Reutilizar el envío completo del pipeline. |
| `fix_chat` + título deja `*Asistente Virtual*` + `📌` | `withBotPrefix` + `actions` | Título siempre antes del pin. |

### P2 — robustez y observabilidad

| Hallazgo | Evidencia | Propuesta |
|----------|-----------|-----------|
| Debounce en memoria por instancia | `debouncer` | Estado en Redis + cola de trabajos (como ya preveía el legacy). |
| Se pierden mensajes al apagar | `stopAll` no hace flush | Flush ordenado en `SIGTERM`. |
| Cache de media volátil y limitado | `mediaCache` 128 | Persistir media temporal o descargar al recibir. |
| Tokens solo de la última iteración | `complete` sobrescribe `usage` | Acumular usage de todas las iteraciones. |
| Anti-spam silencioso y fail-open | `IsSpamming` | Aviso al usuario, métrica y política explícita. |
| Dedup O(n) en memoria | `markSeen` | Redis SET con TTL. |
| Acciones/tools acumulables y duplicables | `complete` | Deduplicar por nombre/argumentos. |
| Sin métricas de decisiones | Solo logs | Contadores por rama (ignorado por spam, sin respuesta, tool, etc.). |
| Imágenes no se entienden | Solo se usa el caption | Visión multimodal si el caso de negocio lo requiere. |
| Sin rate limit HTTP | Router | Middleware por IP para API/SSE. |

## 19. Checklist de validación tras cambios

- [ ] Primer mensaje de usuario nuevo: saludo + disclaimer + respuesta útil.
- [ ] Segundo mensaje: sin saludo inicial repetido.
- [ ] “Agente” / petición humana: deriva y desactiva IA.
- [ ] Intervención del personal: desactiva y no responde.
- [ ] Bot global apagado: no responde nada.
- [ ] Spam: sexto mensaje en 60 s se ignora.
- [ ] Solo audio: se transcribe y responde.
- [ ] Sin `OPENAI_API_KEY`: respuesta degradada.
- [ ] Cita normal: registra en `appointments` y no confirma hora.
- [ ] Cita con Tamara: exige servicio y registra.
- [ ] “Gracias/ok”: cierre silencioso y sin historial.
- [ ] Adjunto: se envía después del texto y no permite rutas arbitrarias.
- [ ] Recordatorio: `{date}` y `{name}` resueltos.
- [ ] `force-reply`: envía texto, adjuntos y registra analítica.
