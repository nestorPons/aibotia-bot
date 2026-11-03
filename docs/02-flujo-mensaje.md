# 02 — Flujo de un mensaje

> Versión detallada con cada decisión, escenarios y mejoras priorizadas:
> [09-flujo-conversacional](09-flujo-conversacional.md).

## Diagrama

```
[WhatsApp] → events.Message
   → whatsapp.Client.emit          normaliza + cachea media
   → IncomingMessage (canal 256)
   → pipeline.handle
       1 dedup por ID (10 min, memoria)
       2 descarta grupos/broadcast
       3 FromMe sin watermark → desactiva IA del chat y termina
       4 settings.is_active == false → termina
       5 users.FindOrCreate(jid, pushName, active=settings.activate_new_users)
       6 usuario inactivo → intenta reactivar por inactividad
       7 anti-spam (Redis) → termina si supera límite
       8 debounce.add(chat, msg)
   → debouncer.flush (1 s + 1,5 s por mensaje, máx. 15 s)
   → normalizeBuffered (une textos; quita cortesías si hay contenido)
   → [audio solo] DownloadMedia + Whisper
   → assistant.Reply
   → actions.Run
   → typing ON → SendText(*Asistente Virtual* + watermark) → adjuntos → typing OFF → MarkSeen
   → history.Append(user) + history.Append(assistant)
   → interactions.Log
```

## Detalle por etapa

| Etapa | Regla | Código |
|-------|-------|--------|
| Dedup | `ID` ya visto en 10 min → ignora. Limpieza perezosa del mapa. | `pipeline/service.go` |
| Grupos | Descarta `@g.us`, `@broadcast`, `status@broadcast`. | `pipeline/filters.go` |
| Staff | Mensaje propio (`FromMe`) sin `\u200B` → `active=false` para ese usuario. | `pipeline/service.go` |
| Estado global | `settings.is_active`; si no hay fila se asume activo. | `store/repo/settings.go` |
| Usuario | Crea el JID si no existe con `active=settings.activate_new_users` (por defecto `true`), rol cliente, alias = pushName. | `store/repo/users.go` |
| Reactivación | Si inactivo y última interacción > `HOURS_TO_ACTIVE` (168 h) → activa. | `pipeline/service.go` |
| Anti-spam | Ventana deslizante Redis: `SPAM_LIMIT` (5) en `SPAM_WINDOW` (60 s). | `security/antispam.go` |
| Debounce | `BUFFER_MIN_MS` (1000) + 1500 ms por mensaje extra, tope `BUFFER_MAX_MS` (15000). | `pipeline/debounce.go` |
| Normalización | Junta textos con `\n`; si hay >1 parte elimina cortesías (“ok”, “gracias”, emojis). | `pipeline/service.go` |
| Audio | Solo si no hay texto: descarga media del cache y transcribe con Whisper. | `pipeline/service.go` |
| Nuevo usuario | `IsNew` = historial vacío → el contexto pide saludo + disclaimer. | `pipeline/service.go` |
| Acciones | Se ejecutan **antes** de enviar (pueden cambiar el texto o enviar ubicación). | `actions/runner.go` |
| Envío | Texto + watermark invisible; luego adjuntos; después `MarkSeen`. | `pipeline/service.go` |
| Memoria | Se guarda el turno de usuario y de asistente (Redis, 40 máx., 7 d). | `store/repo/history.go` |
| Analítica | Inserta en `ai_interactions` con tokens, tools, latencia e idioma. | `store/repo/interactions.go` |

## Reglas de respuesta

- Si `reply.Text == ""` y no hay adjuntos → no se envía nada (cierre silencioso).
- El watermark (`\u200B`) se añade al texto para que el propio bot no se procese.
- `ForceReply` (admin) salta filtros, anti-spam y debounce; usa el mismo asistente,
  ejecuta acciones y envía solo texto.

## Errores y degradación

| Fallo | Efecto |
|-------|--------|
| PostgreSQL no conecta al arrancar | El proceso no arranca. |
| Redis no conecta al arrancar | El proceso no arranca. |
| OpenAI sin key | Respuesta degradada; no RAG ni tools. |
| Error de chat completion | Log de error; no se envía respuesta ni se guarda historial. |
| Error enviando texto | Log; se intentan adjuntos. |
| Transcript falla | Se continúa con texto vacío. |
| Acción falla | Log de warning; la respuesta se envía igual. |

## Persistencia (orden real)

1. `users` (alta/actualización de tema).
2. Envío a WhatsApp.
3. `history` (Redis) — solo si el envío de texto no falló de forma bloqueante.
4. `ai_interactions` — siempre que se generó respuesta.

No hay transacción entre envío y persistencia: un fallo de BD después del envío
pierde el registro, no el mensaje.
