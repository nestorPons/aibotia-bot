# 2026-11-18 · WhatsApp sin dispositivo vinculado

## Síntoma
- El chatbot de WhatsApp no responde.

## Diagnóstico
| Comprobación | Resultado |
|---|---|
| `docker ps` en VPS | `aibotia-botd-1` Up 13 días (healthy) → proceso OK |
| `docker logs aibotia-botd-1` | 2026-11-04 14:18 `whatsapp iniciado paired=false` |
| Logs 14:19–14:21 | QRs generados; 14:21 `QR caducado; reinicia para generar uno nuevo` |
| `select count(*) from whatsmeow_device` | **0** → ninguna sesión vinculada |
| `GET /admin/api/whatsapp` | `{"paired":false,"status":"waiting_qr"}` |

**Causa:** el canal nunca quedó emparejado tras el redespliegue del 4-nov.
Sin dispositivo vinculado no entran eventos `events.Message`, por eso no responde.

## Acción
- `docker compose -f docker-compose.prod.yml restart botd` → QR nuevo a las 09:49:31.
- Vinculación manual: panel `https://aibotia.com/admin` → pestaña WhatsApp (QR válido ~160 s).

## Corrección aplicada
- `pairLoop`: al caducar una tanda de QR pide otra automáticamente (3 s de pausa,
  hasta 10 tandas ≈ 25 min) mientras siga sin vincular.
- `Provider.Reconnect`: reconecta si hay sesión; si el device fue borrado por
  WhatsApp (`events.LoggedOut` → `Store.Deleted`) reconstruye el cliente sobre
  un device vacío y reabre el emparejamiento.
- Panel: `POST /admin/api/whatsapp/reconnect` + botón «Generar QR» y refresco
  del estado/QR cada 5 s; el QR ya no obliga a reiniciar el contenedor.
- `clientMu` protege el puntero `whatsmeow.Client`, que ahora puede cambiar en
  caliente.

### Verificación
- `go build ./...`, `go vet` y tests de `internal/whatsapp` OK.
- `TestEventsStreamEnvíaEventos` (httpapi) es inestable por una carrera del
  propio test (falla igual en `HEAD`): no relacionado con este cambio.

### Archivos
- `internal/whatsapp/client.go`, `internal/whatsapp/provider.go`
- `internal/whatsapp/client_test.go`
- `internal/httpapi/admin.go`, `internal/httpapi/admin/index.html`
- `docs/03-whatsapp.md`

## Notas de despliegue
- El compose del servidor es `docker-compose.prod.yml`; `docker compose <cmd>` sin `-f` no resuelve `botd`.
- `API_ENABLED=false` en el VPS: `/api/v1/whatsapp/*` devuelve 404 (solo panel admin).

## Notas de despliegue
- El compose del servidor es `docker-compose.prod.yml`; `docker compose <cmd>` sin `-f` no resuelve `botd`.
