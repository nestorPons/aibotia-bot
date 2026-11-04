# Runbook de cutover (Node/MariaDB → Go/PostgreSQL)

> Migración de la aplicación legacy `aibotia-bot` (Node, en `.old/`) a la app Go (raíz del repo).

## 0. Requisitos
- PostgreSQL 16 con extensión `pgvector` y Redis 7 accesibles.
- Acceso de red a la MariaDB y (opcional) Redis legacy.
- `OPENAI_API_KEY` y una `API_KEY` nueva (distinta de la de OpenAI).
- Volumen/almacenamiento para la sesión de WhatsApp.

## 1. Preparar infraestructura
```bash
cp .env.example .env   # configura API_KEY, DATABASE_URL, REDIS_URL, OPENAI_API_KEY, LOCATION_*
```
Levantar dependencias (o usar servicios gestionados):
```bash
docker compose -f docker-compose.staging.yml up -d postgres redis
```

## 2. Crear esquema
```bash
go run ./cmd/migrate
```

## 3. Migrar datos relacionales (MariaDB → PostgreSQL)
> Migra `settings`, `users`, `content_blocks`, `content_translations`,
> `ai_interactions`, `system_logs`. **No** migra el estado volátil de Redis
> (historial 7d y citas 2h): es efímero y se regenera.

```bash
export LEGACY_DATABASE_URL="usuario:pass@tcp(HOST:3306)/aibotia?parseTime=true"
go run ./cmd/migrate-data -dry-run          # revisar conteos
go run ./cmd/migrate-data -truncate         # migración real
```

## 4. Cargar contenido y RAG
```bash
go run ./cmd/seed    # bloques de contenido (fix-messages)
go run ./cmd/embed   # embeddings del RAG (requiere OPENAI_API_KEY)
```

## 5. Arrancar y vincular WhatsApp
```bash
make run
# o en contenedor:
make docker-up
```
- Obtener el QR: `GET /api/v1/whatsapp/qr` (o los logs) y escanear con el teléfono.
- Verificar: `GET /health`, `GET /ready` (`postgres`, `redis`), `GET /api/v1/whatsapp/status`.

## 6. Verificación funcional
- `GET /api/v1/instructions` devuelve los bloques migrados.
- `GET /api/v1/users` devuelve los usuarios migrados.
- Enviar un mensaje de prueba desde un número y comprobar respuesta.
- `POST /api/v1/users/{id}/force-reply` para forzar respuesta.

## 7. Pruebas de carga (antes de exponer a producción)
```bash
go run ./cmd/loadtest -base http://localhost:3000 -key "$API_KEY" \
  -paths "/health,/ready,/api/v1/users,/api/v1/instructions" \
  -duration 30s -concurrency 50
```
Criterio orientativo: 0 errores 5xx, p99 estable dentro del presupuesto.

## 8. Corte definitivo
1. Apagar el `whatsapp-provider` legacy (`.old/`, Node) para liberar la sesión de WhatsApp.
2. Arrancar la app Go (`botd`) y vincular por QR.
3. Redirigir Tráfico/Traefik al nuevo servicio.
4. Monitorizar logs (`slog` JSON) y `/ready`.
5. Mantener MariaDB/Redis legacy en solo lectura hasta validar; luego apagar.

## Rollback
- Volver a levantar la app legacy (`.old/`, Node) y re-vincular la sesión de WhatsApp (el volumen/método de sesión cambia entre wwebjs y whatsmeow).
- Los datos migrados en PostgreSQL son idempotentes (`ON CONFLICT`), se puede reintentar.

## Notas
- La **migración de datos es idempotente**: `-truncate` reinicia; sin él usa `ON CONFLICT`.
- Redis legacy no se migra (estado efímero).
- Rotar la clave OpenAI filtrada en el repositorio legacy antes del cutover.
