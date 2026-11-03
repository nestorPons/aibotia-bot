# 03 — Canal WhatsApp

## Implementación

- Librería: `go.mau.fi/whatsmeow` (protocolo nativo, sin navegador).
- Archivo: `internal/whatsapp/client.go`; contrato en `internal/whatsapp/provider.go`.
- Sesión persistente en PostgreSQL vía `sqlstore` (dialecto `postgres`, driver `pgx`).
- DSN: `WHATSAPP_STORE_DSN`; si está vacío usa `DATABASE_URL`.

## Interfaz `Provider`

| Método | Uso |
|--------|-----|
| `Start` / `Stop` | Inicializa store, conecta/desconecta, libera DB. |
| `Status` / `QRCode` | Estado (`disconnected`, `waiting_qr`, `connected`, `logged_out`) y último QR. |
| `Incoming` | Canal de `IncomingMessage` (buffer 256). |
| `SendText` / `SendMedia` / `SendLocation` | Envío; devuelven ID de mensaje. |
| `DownloadMedia` | Descarga media por ID desde el cache en memoria. |
| `MarkSeen` / `SendTyping` | Recibos de lectura y presencia. |
| `GetContact` / `ListContacts` | Consulta e importación de contactos. |
| `Logout` | Cierra sesión (requiere nuevo QR). |

## Ciclo de sesión

1. `sqlstore.Upgrade` crea/actualiza las tablas de whatsmeow en PostgreSQL.
2. `GetFirstDevice`:
   - Sin device → estado `waiting_qr`, `GetQRChannel` + `Connect`, y una goroutine
     consume códigos QR (`success` → `connected`, `timeout` → `disconnected`).
   - Con device → `Connect` directo y estado `connected`.
3. Eventos manejados: `Connected`, `Disconnected`, `LoggedOut`, `Message`.

QR: logs del proceso y `GET /api/v1/whatsapp/qr`; también por SSE (`event: qr`).

## Normalización de entrada

| Campo | Origen |
|-------|--------|
| `ID` | `evt.Info.ID` |
| `Chat` / `Sender` | JID (`...@c.us`, `...@g.us`, etc.) |
| `PushName` | nombre visible |
| `Text` | conversación, texto extendido o caption de imagen/vídeo/documento |
| `MediaType` | `image`, `video`, `audio`, `document`, `sticker`, `""` |
| `FromMe` | `Info.IsFromMe` |

Los mensajes con media se guardan en un cache en memoria (máx. 128, FIFO) para
poder descargarlos después (`DownloadMedia`).

## Envío

- Texto: `Conversation`.
- Imagen/documento: `Upload` + `ImageMessage`/`DocumentMessage` con caption.
- Ubicación: `LocationMessage` (acción `send_location`; config `LOCATION_*`).
- Presencia: `composing`/`paused` antes y después de responder.
- Lectura: `MarkRead` del último mensaje del lote.

## Filtros aplicados (en `pipeline`)

| Filtro | Regla |
|--------|-------|
| Grupos/difusión | Se ignoran JIDs `@g.us`, `@broadcast`, `status@broadcast`. |
| Watermark | Texto con `\u200B` se considera del propio bot y se ignora. |
| Staff | Mensaje `FromMe` sin watermark desactiva la IA del chat. |
| Anti-spam | Ventana deslizante por JID en Redis. |

## Limitaciones

- `fix_chat` no puede etiquetar/fijar chats: son mutaciones de app-state no
  expuestas a alto nivel. Se aplica el efecto funcional (desactivar IA + prefijo 📌).
- El cache de media es por proceso y en memoria; tras reiniciar no se puede
  descargar media antigua.
- El canal de entrada descarta mensajes si está lleno (se registra en logs).
- La sesión solo puede estar activa en un proceso: hay que parar el proveedor
  legacy antes del cutover para liberar el número.
