# CHANGELOG — aibotia-bot (Go)

> Registro de cambios de estado. Formato: fecha · tipo · cambios · archivos.
> Histórico detallado de la reescritura (fases 0–7): `.old/docs/CHANGELOG.md`.

## 2026-11-04 · .env de producción sincronizado y REMINDER_TEMPLATE en compose

### Cambiado
- `.env` de producción actualizado desde el `.env` local **adaptado**: se
  mantienen `APP_ENV=production`, `CORS_ALLOWED_ORIGINS` de producción,
  `POSTGRES_*`, `API_KEY`, `ADMIN_PASSWORD` y `ADMIN_SESSION_SECRET`; se añade
  `REMINDER_TEMPLATE` y se fija `TEST_TARGET`. El archivo **no** se versiona.
- Compose (dev, staging y prod) pasa `REMINDER_TEMPLATE` al contenedor; antes
  el valor del `.env` se ignoraba. Backups: `/root/.env.bak-<ts>` en el VPS.

### Verificación
- `docker compose config` OK en dev; stack de producción desplegado y `/ready`
  OK; dentro de `botd`: `APP_ENV=production`, `API_ENABLED=false`,
  `REMINDER_TEMPLATE` con valor, `ADMIN_PASSWORD` y `API_KEY` conservados.
- `git check-ignore` confirma que `.env` sigue fuera del repositorio.

### Archivos
- `docker-compose.yml`, `deploy/docker-compose.yml`,
  `deploy/docker-compose.prod.yml`, `docs/CHANGELOG.md`
  (más `deploy/.env` del VPS, no versionado).

## 2026-11-04 · Modo prueba: resumen del panel en la derivación

### Añadido
- `internal/summary/thread.go`: `BuildThread`/`Reader` extraen la construcción
  del resumen del hilo (conversación + usuario + cita + servicio) para que el
  panel y el bot usen exactamente la misma ruta.
- En modo prueba, las derivaciones (`message_human_request`, `fix_chat`) añaden,
  tras la nota, `📋 Resumen que vería el operador en el panel:` con los puntos
  del resumen (máx. 12 s; si falla, solo se envía la nota).
- Tests: `TestHandoffEnModoPruebaAñadeResumenDelPanel` y
  `TestHandoffNormalNoAñadeResumen`.

### Cambiado
- `/admin/api/messages/{id}/summary` ahora delega en `summary.BuildThread`
  (misma respuesta JSON).
- `actions.NewRunner` recibe un `ThreadReader`; cableado en `cmd/botd`.
- `addTestModeHandoff` centraliza nota + resumen en ambos caminos de derivación.

### Verificación
- `go build ./...`, `go vet ./...`, `go test ./...` OK.
- Comprobado contra la BD de desarrollo con usuario/cita simulados: resumen
  `fallback` y `ai` (OpenAI) generados correctamente (filas temporales borradas).

### Archivos
- `internal/summary/thread.go`, `internal/actions/runner.go`,
  `internal/actions/runner_test.go`, `internal/httpapi/admin.go`,
  `cmd/botd/main.go`, `docs/09-flujo-conversacional.md`,
  `docs/12-bot-pruebas.md`, `docs/CHANGELOG.md`.

## 2026-11-04 · API pública desactivada en desarrollo y producción

### Añadido
- Variable `API_ENABLED` (default `true`). Con `false` no se monta el grupo
  `/api/v1`; solo quedan landing, `/health`, `/ready` y `/admin`.
- Aviso en el arranque: `API_ENABLED=false; /api/v1 no se monta…`.
- Test `TestAPIEnabledFalseNoMontaAPI` (404 en `/api/v1/ping`).

### Cambiado
- `API_ENABLED=false` en `.env` local y en `deploy/.env` del VPS; ambos stacks
  recreados. La API sigue implementada, solo no se publica.
- Compose dev/prod/staging propagan `API_ENABLED`.

### Verificación
- `go build ./...`, `go vet ./...`, tests de `httpapi` y `config` OK.
- Dev: `/api/v1/ping` → 404; `/`, `/health` → 200.
- Prod: `/api/v1/ping` y `/api/v1/users` → 404; `/` → 200, `/admin` → 303.

### Archivos
- `internal/config/config.go`, `internal/httpapi/router.go`,
  `internal/httpapi/router_test.go`, `cmd/botd/main.go`,
  `docker-compose.yml`, `deploy/docker-compose.yml`,
  `deploy/docker-compose.prod.yml`, `.env.example`,
  `docs/06-api.md`, `docs/07-operacion.md`, `docs/11-despliegue-vps.md`,
  `docs/CHANGELOG.md`.

## 2026-11-04 · Landing: rebranding AIBOTIA 2.0 y plan único

### Cambiado
- Marca de la landing renombrada de `AIBOT` a `AIBOTIA 2.0` en título SEO,
  metadatos, header, footer, mockup de chat y textos de la portada.
- Eliminado el plan «Cuota mensual» (50 €/mes) de `Plans`. Queda solo
  «Instalación personalizada» (desde 90 €), ahora destacado.
- Portada de precios: copy de pago único (`paga solo por lo que necesitas` →
  `con un pago único`) y grid de planes a una columna centrada (940 → 560 px).
- Retirada la etiqueta «Recomendado» del plan (ya no hay comparativa).
- `base.html`: versión del CSS a `?v=2` para evitar caché en navegadores.

### Verificación
- `go build ./...`, `go vet ./...` OK. Render local revisado (1 plan, marca nueva).
- Desplegado en producción con `deploy/deploy.sh`; `/ready` OK.

### Archivos
- `internal/landing/content.go`, `internal/landing/handler.go`,
  `internal/landing/templates/base.html`,
  `internal/landing/templates/pages/home.html`,
  `internal/landing/templates/partials/{header,footer,chat-mockup}.html`,
  `internal/landing/static/css/style.css`, `docs/CHANGELOG.md`.

## 2026-11-03 · Repositorio público en GitHub y despliegue en producción

### Añadido
- Repositorio público `nestorPons/aibotia-bot` (rama `main`), inicializado desde
  el árbol de trabajo local.

### Cambiado
- `.gitignore`: se versionan `data/seeds/`, `data/*.json`, `data/test/cases/` y
  `data/test/scenarios/` (necesarios para build y pruebas); se excluye
  `data/test/reports/` por contener teléfonos y datos de sesión.

### Verificación
- `go build ./...` y `go vet ./...` OK.
- `go test ./...` OK (el test SSE `TestEventsStreamEnvíaEventos` es sensible a
  tiempos; pasa al reintentar en aislamiento).
- Despliegue en `93.127.203.142` con `deploy/deploy.sh` (rsync + rebuild):
  commit `da92843`, migraciones aplicadas hasta la versión 8.
- `https://aibotia.com/health` → `{"status":"ok"}`.
- `https://aibotia.com/ready` → postgres/redis `ok`.
- Landing 200, `/admin` 303 (login), `www` → apex 308.
- Informes de prueba sincronizados en `data/test/reports/` del servidor.
- WhatsApp en producción queda en `waiting_qr`: requiere vincular el número
  desde el panel (`/admin`) escaneando el QR.

### Archivos
- `.gitignore`, `docs/CHANGELOG.md`.

## 2026-11-03 · Iconos del switch de tema en SVG

### Corregido
- El icono de la luna (y el sol) del selector de tema se dibujaba con emojis
  (`🌙`/`☀️`), que dependían de la fuente del sistema y se veían borrosos,
  desalineados o como *tofu* en algunos equipos.
- Ahora son SVG en línea que heredan `currentColor`, con tamaño fijo de 14 px y
  centrado respecto al switch.

### Añadido
- El icono del tema activo se resalta en color de acento (`.theme-ico.on`),
  actualizado desde `applyTheme()`.

### Archivos
- `internal/httpapi/admin/index.html`, `docs/CHANGELOG.md`.

## 2026-11-03 · Página de cliente en forma de chat

### Añadido
- `aibotia-test-<caso>-cliente.html`: página web autocontenida tipo WhatsApp para
  **enseñar o enviar a un cliente**. Solo muestra la conversación en burbujas,
  fecha y botón de imprimir; oculta objetivo, veredicto, evaluaciones, semilla,
  teléfonos y errores internos.
- `renderClientHTML` y estilos `clientCSS`.
- El índice de `make tester-serve` abre la página de cliente como enlace
  principal y deja **Técnico / MD / JSON** como accesos secundarios.

### Cambiado
- `tester.WriteReport` genera y devuelve también la ruta de la página de cliente.
- El runner imprime la ruta nueva al terminar cada caso.
- Docs del tester y README actualizados.

### Verificación
- `go test ./internal/tester`: cubre que la página de cliente no filtre datos
  internos y que el índice enlace cliente + informe técnico sin duplicar.
- Página regenerada desde el JSON de `ambiguedad_precios`.

### Archivos
- `internal/tester/report.go`, `internal/tester/report_html.go`,
  `internal/tester/serve.go`, `internal/tester/runner.go`,
  `internal/tester/report_html_test.go`, `internal/tester/report_test.go`,
  `docs/12-bot-pruebas.md`, `README.md`, `docs/CHANGELOG.md`.

## 2026-11-03 · Tester: el cliente IA vuelve a reaccionar a la conversación

### Corregido
- Causa raíz del bucle «bot pregunta → test pregunta otra cosa»: el cliente IA
  llamaba a Chat Completions con `max_tokens`, `presence_penalty`,
  `frequency_penalty` y `temperature`, parámetros que `gpt-6.1-sol` rechaza. El
  error se silenciaba y cada turno caía al **banco fijo** de preguntas.
- Ahora usa `max_completion_tokens` (1500 en el cliente, 2500 en evaluación),
  sin `temperature` ni penalizaciones, compatible con modelos de razonamiento.
- Se valida que la respuesta del cliente IA no venga vacía (`finish_reason`).
- Si el bot pide una **aclaración** («¿a qué tratamiento te refieres?»), se
  añade un aviso al cliente (`clarifyNote`/`lastBotQuestion`) para responder
  primero con un dato concreto antes de avanzar.
- Si el cliente IA falla o repite tras el reintento, el caso se **detiene** con
  veredicto `error`; ya no degrada en silencio al banco.
- `Report.Error` se muestra en JSON, Markdown y HTML.

### Verificación
- Prueba real `-case ambiguedad_precios -turns 4`: los turnos 2-4 salen como
  `Cliente (IA)`, responde a la aclaración del bot y avanza el objetivo sin
  repetir preguntas.
- `go build ./...` y `go test ./internal/tester` en verde.

### Archivos
- `internal/tester/ai.go`, `internal/tester/runner.go`,
  `internal/tester/report.go`, `internal/tester/report_html.go`,
  `internal/tester/ai_test.go`, `internal/tester/report_test.go`,
  `docs/12-bot-pruebas.md`, `docs/CHANGELOG.md`.

## 2026-11-03 · Reportería del tester: sobrescribir por caso

### Cambiado
- `tester.WriteReport` usa un nombre estable por caso
  (`aibotia-test-<caso>.json/.md/.html`). Repetir el mismo caso **sobrescribe**
  los logs anteriores en lugar de acumular copias con fecha.
- El slug del caso se sanea (`reportSlug`) y cae a `scenario` o a la hora solo
  si no hay identificador. La suite mantiene su resumen con fecha.

### Verificación
- `go test ./internal/tester` en verde, incluido el test de sobrescritura:
  dos ejecuciones del mismo caso dejan 3 archivos y el contenido es el de la
  última.

### Archivos
- `internal/tester/report.go`, `internal/tester/report_test.go` (nuevo),
  `docs/12-bot-pruebas.md`, `docs/CHANGELOG.md`.

## 2026-11-03 · Nuevo caso de tester: ambigüedad de precios y promociones

### Añadido
- Caso `ambiguedad_precios` en `data/test/cases/`: cliente que reformula con
  suposiciones («desde 49 €», «mi amiga pagó menos», «2x1», «repaso incluido»)
  para que el bot confirme promociones o precios no oficiales.
- Criterios `must`/`must_not` centrados en precio oficial, qué incluye, corregir
  interpretaciones erróneas y derivar al equipo para excepciones.
- Pasa el catálogo de 8 a **9 casos** (suite `all`, docs y README actualizados).

### Verificación
- `go test ./internal/tester` en verde.
- `go run ./cmd/tester -list` muestra el caso nuevo.

### Archivos
- `data/test/cases/ambiguedad_precios.json` (nuevo),
  `docs/12-bot-pruebas.md`, `README.md`, `docs/CHANGELOG.md`.

## 2026-11-03 · Ubicación: envío verificado y tolerancia a fallos

### Añadido
- `actions.Runner.sendLocation` registra `ubicación enviada` con el ID del
  mensaje, latitud y longitud; antes sólo se veía un WARN cuando fallaba.
- Fallback: si WhatsApp rechaza el pin, se envía `📍 nombre — dirección` como
  texto para que el cliente reciba al menos la dirección.
- `whatsapp.LocationText`: representa un `LocationMessage` como texto legible
  (`nombre — dirección · enlace a Google Maps`).
- El cliente del **tester** ya reconoce los mensajes de ubicación
  (`media=location`) y los incluye en el reporte; antes los descartaba y una
  prueba de ubicación parecía “sin respuesta”.

### Verificación
- `botd` reiniciado: `ubicación de empresa actualizada` y `ubicación
  configurada lat=39.4555556 lng=-0.3583096`.
- `force-reply` con «¿Dónde estáis?» → `actions:["send_location"]`, sin error de
  envío.

### Archivos
- `internal/actions/runner.go`, `internal/actions/runner_test.go`,
  `internal/whatsapp/client.go`, `internal/whatsapp/client_test.go`,
  `internal/tester/client.go`, `docs/CHANGELOG.md`.

## 2026-11-03 · Modo prueba: el bot no se desactiva al derivar a un operador

### Añadido
- Columna `users.test_mode` (migración `00008`) y `domain.User.TestMode`.
- `POST /api/v1/users/{id}/test-mode` (`{"enabled":true|false}`): activa/desactiva
  el modo prueba por usuario; al activarlo también reactiva la IA.
- En modo prueba, `message_human_request` y `fix_chat` **no desactivan la IA** y
  añaden `🧪 Nota de prueba: en producción, el bot se desactivaría en este punto
  para que atendiera un operador.`
- El tester activa el modo al empezar (y tras la primera respuesta si el usuario
  se acaba de crear) y lo desactiva al terminar.
- El panel muestra un pill `🧪 Modo prueba` en la conversación.
- Tests del runner de acciones para handoff normal, modo prueba, `fix_chat` y
  deduplicación de la nota.

### Archivos
- `internal/store/migrations/sql/00008_user_test_mode.sql`, `internal/domain/user.go`,
  `internal/store/repo/users.go`, `internal/httpapi/users.go`,
  `internal/httpapi/router.go`, `internal/httpapi/admin.go`,
  `internal/httpapi/admin/index.html`, `internal/actions/runner.go`,
  `internal/actions/runner_test.go`, `internal/tester/runner.go`,
  `docs/06-api.md`, `docs/09-flujo-conversacional.md`,
  `docs/12-bot-pruebas.md`, `docs/CHANGELOG.md`.

## 2026-11-03 · Tema claro de WhatsApp en el panel

### Añadido
- Tema claro inspirado en WhatsApp Web mediante variables CSS
  (`:root[data-theme="light"]`): fondo beige del chat, burbujas blancas/verdes,
  cabeceras y tablas claras, acento verde WhatsApp (#008069).
- Switch 🌙/☀️ en la cabecera para alternar tema. Se guarda en
  `localStorage` (`aibotia.theme`) y se aplica antes del primer pintado para
  evitar el destello; se sincroniza entre pestañas con el evento `storage`.
- Variables nuevas para hover, sombras, puntos del chat, metadatos de burbujas,
  backdrop de diálogos y color del knob de los switches.

### Archivos
- `internal/httpapi/admin/index.html`, `docs/CHANGELOG.md`.

## 2026-11-03 · Remitente visible en el chat del panel

### Corregido
- El chat del panel no mostraba el nombre del bot porque `*Asistente Virtual*`
  solo se añade al enviar por WhatsApp (`pipeline.withBotPrefix`) y en la BD se
  guarda la respuesta limpia. Ahora la burbuja pinta el remitente como
  encabezado: `Asistente Virtual` para el bot y el nombre del operador para los
  mensajes manuales (que sí guardan `*Nombre*`). Funciona también con el
  histórico anterior.

### Archivos
- `internal/httpapi/admin/index.html`, `docs/CHANGELOG.md`.

## 2026-11-03 · Logs HTML de las pruebas para la clienta

### Añadido
- Cada ejecución del tester (caso o suite) genera también un **log HTML
  autocontenido** estilo conversación: burbujas cliente/asistente, fecha,
  objetivo, veredicto y evaluación. Se abre sin conexión y tiene botón
  "Imprimir / guardar PDF".
- El HTML de cliente **no** incluye teléfonos (tester/bot), semilla ni datos
  internos; solo la conversación y el resultado.
- `-serve 127.0.0.1:8091` sirve `TESTER_REPORT_DIR` con un índice navegable
  (HTML + enlaces MD/JSON). Nuevo target `make tester-serve`.
- `internal/tester/report_html_test.go`: cubre escapado, privacidad, suite e
  índice del visor.

### Cambiado
- `WriteReport`/`WriteSuiteReport` devuelven también la ruta `.html`; el runner
  la imprime al terminar.
- `docs/12-bot-pruebas.md`: sección de reportes/logs, visor local y copia de los
  logs desde el volumen `tester-reports` del VPS.

### Archivos
- `internal/tester/report_html.go`, `internal/tester/report_html_test.go`,
  `internal/tester/serve.go`, `internal/tester/report.go`,
  `internal/tester/runner.go`, `cmd/tester/main.go`, `Makefile`, `README.md`,
  `docs/12-bot-pruebas.md`, `docs/CHANGELOG.md`.

## 2026-11-03 · Chat del panel en tiempo real (SSE)

### Añadido
- `internal/realtime`: bus en memoria (`Hub`) con `Publish`/`Subscribe` y
  eventos `incoming`, `message` y `typing`.
- `GET /admin/api/events` (SSE con cookie de sesión): mensajes entrantes en
  vivo, interacciones persistidas y estado "escribiendo" del bot.
- El panel muestra el mensaje del cliente al instante (aunque la IA aún no haya
  respondido), la respuesta al persistirse, el indicador de escritura y la
  bandeja se reordena sola. Badge "En vivo / Reconectando…" y resincronización
  tras reconectar.

### Cambiado
- `repo.Interactions.Log` usa `RETURNING id, created_at` y notifica la fila
  insertada (`OnLog`).
- `pipeline.Service` publica `incoming` al recibir y `typing` durante la
  generación de respuesta.
- Las rutas SSE quedan exentas del timeout global de 60 s y renuevan el
  `WriteTimeout` con `http.ResponseController` (heartbeat 20 s).
- `docs/06-api.md` y `docs/01-arquitectura.md` actualizados.

### Archivos
- `internal/realtime/realtime.go`, `internal/realtime/realtime_test.go`,
  `internal/store/repo/interactions.go`, `internal/pipeline/service.go`,
  `internal/httpapi/admin.go`, `internal/httpapi/admin_events_test.go`,
  `internal/httpapi/router.go`, `internal/httpapi/middleware.go`,
  `internal/httpapi/admin/index.html`, `cmd/botd/main.go`,
  `docs/06-api.md`, `docs/01-arquitectura.md`, `docs/CHANGELOG.md`.

## 2026-11-03 · Tester como conversación entre dos LLM (cliente reacciona al bot)

### Cambiado
- `internal/tester/ai.go`: `AI.Next` monta un diálogo real en formato chat
  (system = rol/caso, `assistant` = cliente, `user` = respuestas del bot
  agrupadas). El cliente recibe toda la conversación y reacciona a cada turno
  del bot; si no hay respuesta lo indica (`[El bot no respondió...]`); histórico
  acotado a 24 vueltas.
- `questions` del caso pasan a ser **temas sugeridos** (no guion); se incluyen
  persona, objetivo, tono, criterios, cierre y mensajes ya enviados.
- `AI.Evaluate` recibe la conversación previa para puntuar con contexto.
- Modo por defecto `ai` (dos LLM); `-mode scenario` queda como modo heredado
  sin reacción y `ai` sin `OPENAI_API_KEY` falla con error claro.
- Reintento de mensaje repetido con aviso explícito al modelo.
- `Turn.Source` (`ia`/`guion`/`setup`) y marca `Cliente (IA)` en los reportes.
- `internal/tester/ai_test.go`: cubre orden de roles, reacción al bot, ausencia
  de respuesta, temas como sugerencias y limpieza de prefijos.
- Docs y README del tester actualizados (diálogo, requisitos, flags).

### Archivos
- `internal/tester/ai.go`, `internal/tester/runner.go`,
  `internal/tester/report.go`, `internal/tester/ai_test.go`,
  `cmd/tester/main.go`, `docs/12-bot-pruebas.md`, `README.md`,
  `docs/CHANGELOG.md`.

## 2026-11-03 · Borrado total de usuario, prompt de empresa, tester por casos y filtro de usuarios

### Añadido
- Migración `00007`: FK `ON DELETE CASCADE` en `ai_interactions` y
  `appointments` + índice `ai_interactions(session_id)`.
- `repo.Users.DeleteCascade`: borra usuario, interacciones, citas y caché de
  WhatsApp (`whatsmeow_contacts`, `whatsmeow_chat_settings`) en una transacción.
- Limpieza en Redis/memoria al borrar: historial, anti-spam, buffer debounce y
  caché de resúmenes.
- `bootstrap.Company`: fusiona `prompt_location` + `prompt_location_contact` en
  `prompt_company`, borra los antiguos y sus embeddings. Idempotente.
- `prompt_company` se inyecta siempre en el system prompt
  (`### DATOS DE EMPRESA`) y queda excluido del RAG.
- Pestaña **Empresa** en `/admin`: editor único de `prompt_company` con historial
  de versiones.
- Pin de WhatsApp desde el prompt: `<!-- pin: lat,lng | nombre | dirección -->`
  (`actions.ParsePin`), recargado en caliente al guardar.
- `cmd/tester`: 8 casos rotativos (`clienta_habitual`, `cliente_casual`,
  `adversarial`, `maleducado`, `muchas_dudas`, `otro_sector`,
  `urgente_fuera_horario`, `ingles`), selección aleatoria sin repetir el último,
  `-seed`, `-suite`, reset de historial/IA vía API, evaluación por criterios y
  veredicto pass/fail.
- Filtro segmentado **Todos / Activo / Inactivo** en Usuarios
  (`?ai=active|inactive` en API y panel).
- `make tester-case`, `make tester-suite`.
- `tester -list` / `make tester-list`: catálogo de casos (id, nombre, turnos,
  setup, objetivo y último ejecutado) sin necesidad de WhatsApp ni BD.
- README: sección «Bot de pruebas (tester)» con los comandos de listado,
  ejecución individual, aleatoria y suite.

### Cambiado
- `repo.Users.List` recibe `UserFilter{Search, Active}`.
- `DELETE /users/{id}` y `/admin/api/users/{id}` devuelven
  `{messages,appointments,contacts}`.
- `actions.Runner` admite ubicación recargable (`SetLocation`) y parser de pin.
- `cmd/embed` excluye `prompt_company` y los codes de ubicación antiguos.
- `data/test/scenarios/*.json` quedan como legado; el runner usa `cases/`.
- El tester por defecto usa `-mode ai` si hay `OPENAI_API_KEY`.

### Archivos
- `internal/store/migrations/sql/00007_user_cascade.sql`
- `internal/store/repo/users.go`
- `internal/store/repo/prompts.go`
- `internal/bootstrap/company.go`
- `internal/security/antispam.go`
- `internal/summary/summary.go`
- `internal/pipeline/service.go`
- `internal/pipeline/debounce.go`
- `internal/actions/runner.go`
- `internal/assistant/prompts.go`
- `internal/assistant/assistant.go`
- `internal/assistant/context.go`
- `internal/httpapi/router.go`
- `internal/httpapi/admin.go`
- `internal/httpapi/users.go`
- `internal/httpapi/userpurge.go`
- `internal/httpapi/admin/index.html`
- `cmd/botd/main.go`
- `cmd/embed/main.go`
- `cmd/tester/main.go`
- `internal/tester/{case,ai,runner,report,client}.go`
- `data/seeds/prompts/prompt_company.md`
- `data/test/cases/*.json`
- `Makefile`
- `docs/{05-datos,06-api,07-operacion,09-flujo-conversacional,12-bot-pruebas}.md`

### Verificación
- `go build ./...`, `go vet ./...`, `go test ./...` en verde.
- Migración 7 aplicada al arrancar; `prompt_company` creado y antiguos borrados.
- `send_location` envía pin con `39.4555556,-0.3583096`.
- Borrar usuario deja 0 filas en `users`, `ai_interactions` y `appointments`.
- Filtro `ai=active|inactive` y suite `-suite all` operativos.

## 2026-11-03 · Activación del bot a nuevos usuarios (interruptor en el panel)

### Añadido
- Migración `00006`: `settings.activate_new_users BOOLEAN NOT NULL DEFAULT TRUE`.
- `repo.Settings.ActivateNewUsers` / `SetActivateNewUsers`.
- Endpoints del panel `GET/PUT /admin/api/settings` (`{"activate_new_users": bool}`).
- Sección **Ajustes** en `/admin` con el interruptor «Activar el bot a los
  nuevos usuarios».

### Cambiado
- `pipeline.handle` usa el ajuste al crear usuarios nuevos por mensaje entrante
  (antes `active=true` fijo).
- Importación de contactos (`/admin/api/users/import` y
  `/api/v1/users/import-users`) crea los usuarios nuevos con ese mismo valor;
  `Users.ImportContacts` recibe `activeIfNew`.
- Los usuarios existentes conservan su estado de IA.

### Archivos
- `internal/store/migrations/sql/00006_activate_new_users.sql`
- `internal/store/repo/settings.go`
- `internal/store/repo/users.go`
- `internal/pipeline/service.go`
- `internal/httpapi/users.go`
- `internal/httpapi/router.go`
- `internal/httpapi/admin.go`
- `internal/httpapi/admin/index.html`
- `docs/02-flujo-mensaje.md`
- `docs/05-datos.md`
- `docs/06-api.md`
- `docs/07-operacion.md`
- `docs/09-flujo-conversacional.md`
- `docs/CHANGELOG.md`

### Verificación
- `migrate-up` → versión 6 aplicada.
- `go build ./...`, `go vet ./...` y `go test ./internal/...` OK.
- `GET/PUT /admin/api/settings` sobre el stack dev responde y persiste el valor.
- JS del panel validado con `node --check`.

## 2026-11-03 · Landing: teléfono del grupo de prueba

### Cambiado
- `Site.Phone` en la landing pasa de `660 291 797` a `686 115 433`
  (se muestra en contacto y footer, con enlace `tel:`).
- `Site.WhatsApp` pasa a `34686115433`, alineado con `TEST_TARGET` del despliegue.
- El enlace al grupo de WhatsApp (`Site.WhatsAppURL`) no cambia.

### Archivos
- `internal/landing/content.go`
- `docs/CHANGELOG.md`

### Verificación
- `go build ./...` OK.

## 2026-11-02 · `make dev-apply` para el panel

### Añadido
- Target `make dev-apply`: reinicia `botd` sin reconstruir la imagen; su `CMD`
  recompila el binario dentro del contenedor con el código montado (incluye los
  HTML embebidos del panel) y espera a que `/health` responda.
- `make dev-reload` queda como alias de `dev-apply`.

### Archivos
- `Makefile`
- `README.md`
- `docs/07-operacion.md`
- `docs/CHANGELOG.md`

### Verificación
- `make dev-apply` sobre el stack `aibotia-dev` → `/admin` responde 200.

## 2026-11-02 · Panel admin con tema oscuro de WhatsApp Web

### Cambiado
- `/admin` y `/admin/login` pasan de la paleta clara a la oscura de WhatsApp Web:
  fondo `#0b141a`, paneles `#111b21`/`#202c33`, campos `#2a3942`, texto `#e9edef`
  y acento `#00a884`.
- Bandeja: burbujas salientes `#005c4b`, entrantes `#202c33`, chat con patrón de
  puntos y avisos de operador en ámbar `#ffbc38`.
- Enlaces en `#53bdeb`, errores en `#f15c6d` y `color-scheme: dark` (scrollbars
  y controles nativos).
- Radios normalizados a 6 px (salvo pills, toggles y puntos, por diseño).

### Archivos
- `internal/httpapi/admin/index.html`
- `internal/httpapi/admin/login.html`

### Verificación
- `go build ./...` OK.

## 2026-11-02 · Bot de pruebas por consola

### Añadido
- `cmd/tester` + `internal/tester`: cliente de WhatsApp independiente que se
  vincula por QR en la consola y conversa con el chatbot como un cliente real.
- Modos `scenario` (preguntas fijas) y `ai` (cliente simulado por OpenAI),
  con evaluación opcional de cada respuesta (1-5, avisos e invenciones).
- Reportes por sesión en JSON y Markdown (turnos, latencias, notas).
- Escenarios `data/test/scenarios/{basico,dudas-precio}.json`.
- Imagen `deploy/Dockerfile.tester` y servicio Compose con perfil `tester`
  (base `aibotia_tester`, volumen `tester-reports`).
- Flags `-reset` / `-logout` para gestionar la sesión del número de pruebas.
- Targets `make tester` y `make tester-docker`.

### Archivos
- `cmd/tester/main.go`
- `internal/tester/{client,walog,ai,runner,report,scenario}.go`
- `data/test/scenarios/{basico,dudas-precio}.json`
- `deploy/Dockerfile.tester`
- `deploy/docker-compose.prod.yml`
- `deploy/.env.example`
- `Makefile`
- `docs/12-bot-pruebas.md`
- `docs/README.md`
- `go.mod` / `go.sum` (`mdp/qrterminal/v3`)
- `README.md`

### Verificación
- `go build ./...` y `go vet ./internal/tester ./cmd/tester` OK.
- QR renderizado en consola contra PostgreSQL local (`aibotia_tester`).
- `make tester` en local: destino desde `.env`, QR en consola y `-reset` OK.

## 2026-11-02 · Despliegue en producción (`aibotia.com`)

### Añadido
- Stack de producción detrás de Traefik: router `aibotia`
  (`aibotia.com` + `www`), TLS Let's Encrypt (`myresolver`), red externa
  `traefik-net` y servicio interno en `botd:3000`.
- Middleware `aibotia-www`: redirección permanente `www` → apex.
- Script `deploy/deploy.sh` (rsync + `docker compose up -d --build`).
- Doc de operación del VPS y plantilla `deploy/.env.example`.

### Cambiado
- VPS `93.127.203.142`: código en `/root/projects/aibotia.com`; stack antiguo
  `aibotiacom` (nginx/PHP + MariaDB) detenido (dump en `/root/backpups/`).
- Datos iniciales cargados: 13 bloques, 34 prompts, 48 servicios, 34 embeddings.

### Archivos
- `deploy/docker-compose.prod.yml`
- `deploy/deploy.sh`
- `deploy/.env.example`
- `docs/11-despliegue-vps.md`
- `docs/README.md`
- `docs/CHANGELOG.md`

### Verificación
- `https://aibotia.com/health` 200 · `/ready` postgres+redis ok.
- `https://aibotia.com/` landing 200 · `/admin/login` 200 · `/static/*` 200.
- `http://aibotia.com` 301 → `https://aibotia.com` · `www` 301 → apex.

## 2026-10-30 · Panel: tema visual estilo WhatsApp Web

### Cambiado
- Paleta clara tipo WhatsApp Web: fondo `#f0f2f5`, paneles blancos, texto
  `#111b21`, secundario `#667781`, acento verde `#00a884`/`#008069`.
- Chat: fondo beige `#efeae2`, burbujas propias `#d9fdd3` y recibidas blancas,
  con sombra sutil; cabecera y barra del compositor en gris claro.
- Tablas, botones, inputs, dialogs, toasts, switches, pills y chips adaptados al
  tema claro (incluidos los estados de derivación en ámbar).
- Login del panel con la misma paleta.

### Archivos
- `internal/httpapi/admin/index.html`
- `internal/httpapi/admin/login.html`
- `docs/CHANGELOG.md`

### Verificación
- Revisado en Chromium headless (Mensajes + diálogo de Servicios): contraste y
  legibilidad correctos, burbujas y cabecera tipo WhatsApp Web.

## 2026-10-30 · Resumen del hilo para el operador

### Añadido
- Tarjeta colapsable sobre la conversación (`📋 Resumen para el operador`) al
  seleccionar un usuario; recuerda si estaba abierta o cerrada.
- Chips con datos estructurados: tema detectado, servicio, preferencia horaria,
  estado de la cita, profesional y motivo de derivación.
- 2-4 puntos clave generados por IA a partir de los últimos 24 turnos:
  de qué se habla, datos de cita y preferencias, y avisos relevantes.
- Fallback determinista si OpenAI falla o no hay API key (`source=fallback`),
  con tema, servicio, preferencia y último mensaje del cliente.
- Endpoint `GET /admin/api/messages/{id}/summary` (cache 10 min por hilo y
  última interacción).
- `InteractionRow.tool_calls` expuesto y helpers `repo.NeedsOperator` /
  `repo.HandoffReason`.

### Archivos
- `internal/summary/summary.go` (nuevo), `summary_test.go` (nuevo)
- `internal/store/repo/interactions.go`, `interactions_test.go` (nuevo)
- `internal/httpapi/admin.go`, `internal/httpapi/router.go`
- `internal/httpapi/admin/index.html`
- `cmd/botd/main.go`
- `docs/06-api.md`, `docs/07-operacion.md`, `docs/CHANGELOG.md`

### Verificación
- `go build ./...`, `go vet` y `go test ./...` OK.
- E2E del resumen con IA sobre el hilo derivado: devuelve servicio, preferencia
  horaria y estado de la cita; el fallback responde si OpenAI no está disponible.

## 2026-10-30 · Bandeja: aviso de derivación al operador + filtros

### Añadido
- `ThreadRow.needs_operator`: `true` si la última interacción del hilo contiene
  una tool de derivación (`request_appointment_scheduling`, `appointment_tamara`,
  `message_human_request`, `fix_chat`). Una respuesta manual lo limpia.
- Filtros de la bandeja en `/admin`: `Todos / ⚠️ Requieren respuesta` y
  `IA: todas / activa / inactiva` (combinables con la búsqueda).
- Marca visual del hilo pendiente: borde ámbar, punto pulsante y etiqueta
  `⚠ Requiere operador`; también en la cabecera de la conversación.
- API `GET /admin/api/messages`: parámetros `needs_operator=1` y
  `ai=active|inactive`.

### Archivos
- `internal/store/repo/interactions.go`
- `internal/httpapi/admin.go`
- `internal/httpapi/admin/index.html`
- `docs/06-api.md`, `docs/07-operacion.md`, `docs/CHANGELOG.md`

### Verificación
- `go build ./...`, `go vet` y `go test ./...` OK.
- API: `needs_operator=1` devuelve el hilo derivado; combinado con
  `ai=active|inactive` y `search` filtra correctamente.

## 2026-10-30 · Servicios: campo `description` + datos de prueba completos

### Añadido
- Columna `services.description` (TEXT, default `''`) y campo `description` en
  la API de servicios (`/admin/api/services`).
- `description` se inyecta en `### SERVICIOS` del system prompt tras precio,
  duración y equipo, para dar contexto al modelo.
- Editor y tabla del panel `/admin` muestran/editan la descripción.

### Datos de prueba
- Migración `00005` rellena precio, duración y descripción de las 48 filas
  (solo huecos vacíos) y `data/seeds/services.json` cubre el catálogo completo.
- Las 5 filas sin precio (`nano_rep_cons`, `nano_rep_res`, `micro_cap_cons`,
  `rem_las_tatoo`, `corp_crio_esc`) reciben precio/duración de prueba y su nota
  pasa a "Precio orientativo sujeto a confirmación".
- Contenido placeholder de desarrollo: validar antes de producción.

### Archivos
- `internal/store/migrations/sql/00005_services_description.sql` (nuevo)
- `internal/store/repo/services.go`
- `internal/bootstrap/services.go`
- `internal/assistant/prompts.go`
- `internal/httpapi/admin/index.html`
- `data/seeds/services.json`
- `docs/04-asistente-ia.md`, `docs/05-datos.md`, `docs/06-api.md`
- `docs/CHANGELOG.md`

### Verificación
- `go build ./...` OK; `go test ./internal/bootstrap ./internal/assistant` OK.
- Migración aplicada: 48/48 servicios con precio, duración y descripción.

## 2026-10-30 · Formulario de contacto operativo + captcha Turnstile

### Añadido
- SMTP configurado en `.env` (Gmail de `nestorpons.com`) para el formulario de
  la landing; `CONTACT_TO=info@nestorpons.com`.
- Captcha Cloudflare Turnstile en el formulario de contacto (`TURNSTILE_SITE_KEY`,
  `TURNSTILE_SECRET`). Validación server-side en `POST /contacto`.
- CSP de la landing ampliada (`script-src`/`frame-src` → `challenges.cloudflare.com`).
- Re-render del widget Turnstile en formularios insertados por htmx.

### Archivos
- `.env`, `.env.example`, `docker-compose.yml`, `deploy/docker-compose.yml`
- `internal/config/config.go`, `cmd/botd/main.go`
- `internal/landing/handler.go`, `data.go`, `turnstile.go` (nuevo)
- `internal/landing/templates/base.html`
- `internal/landing/templates/fragments/contact-form.html`
- `internal/landing/static/js/main.js`, `static/css/style.css`
- `docs/CHANGELOG.md`

### Notas
- Sin `TURNSTILE_SECRET` el widget se muestra pero la verificación se omite
  (se registra un warning al arrancar).
- SMTP verificado: login OK y envío de prueba recibido en `info@nestorpons.com`.

## 2026-10-29 · Comandos de build en dev sin reconstruir contenedores

### Añadido
- `make dev-build`: compila (`go build ./...`) dentro del contenedor `botd` en
  ejecución, sin `--build` de imagen.
- `make dev-reload`: reinicia solo `botd`; su `CMD` recompila con el código
  montado y arranca el binario nuevo.

### Archivos
- `Makefile`, `README.md`, `docs/CHANGELOG.md`.

## 2026-10-29 · Bordes de la home limitados a 6px

### Cambiado
- `--radius` y `--radius-sm` pasan de 18px/11px a 6px.
- Botones, badges, inputs, cards, planes, panel de contacto, alertas, cajas de
  icono, burbujas del chat y pills de la home quedan con radio máximo de 6px.
- Se mantienen redondos solo los elementos que por diseño lo requieren
  (puntos, avatares, iconos circulares) y el marco del móvil decorativo.

### Archivos
- `internal/landing/static/css/style.css`, `docs/CHANGELOG.md`.

## 2026-10-29 · Buscador de conversaciones en Mensajes

### Añadido
- Campo de búsqueda en la bandeja de `/admin` por nombre, alias o teléfono/JID.
- Filtro en servidor (`ILIKE`) sobre el último mensaje de cada conversación,
  con paginación y contador adaptados al término buscado.
- Búsqueda con botón/Enter y filtrado automático con debounce de 300 ms.

### Cambiado
- El buscador y el paginador se muestran dentro de la columna izquierda,
  encima y debajo de la lista de nombres.
- `Interactions.ListThreads` acepta el término de búsqueda.
- `GET /admin/api/messages` admite el parámetro `search`.

### Verificación
- `go build ./...`, tests de `internal/httpapi` y sintaxis JS OK.
- Consulta SQL probada contra la base de datos de desarrollo.

### Archivos
- `internal/store/repo/interactions.go`, `internal/httpapi/admin.go`,
  `internal/httpapi/admin/index.html`, `docs/CHANGELOG.md`.

## 2026-10-29 · Composer del chat con emojis y respuestas rápidas

### Añadido
- Selector de emojis por categorías en la conversación de `/admin`.
- Panel de respuestas rápidas con alta, uso y borrado; se guardan en
  `localStorage` del navegador.
- Textarea multilínea con autoajuste: `Enter` envía y `Shift+Enter` inserta
  salto de línea.

### Cambiado
- El campo de respuesta manual pasa de `input` a `textarea` con barra de
  acciones (emoji, respuestas rápidas, enviar).
- El botón *Enviar* se habilita solo con conversación activa y texto.

### Verificación
- `go build ./...` y sintaxis del JS embebido (`node --check`) OK.
- Sin cambios de API ni de base de datos.

### Archivos
- `internal/httpapi/admin/index.html`, `docs/CHANGELOG.md`.

## 2026-10-29 · Landing AIBOT embebida en `botd` (`http://localhost:3000/`)

### Añadido
- Paquete `internal/landing` con la landing Go + htmx portada de Laravel:
  home, planes, ventajas, resumen, contacto y 404.
- Rutas públicas `/`, `/contacto`, `/contacto/formulario`, `/static/*`,
  `/robots.txt`, `/sitemap.xml` y `/favicon.ico`.
- Formulario con htmx, fallback sin JS, validación, honeypot, rate-limit y
  envío SMTP opcional.
- Variables SMTP en `config`, `.env`, compose de desarrollo y despliegue.
- `CONTACT_TO` (por defecto `info@nestorpons.com`).

### Cambiado
- `httpapi.NewRouter` acepta `Deps.Landing` y monta la landing en la raíz.
- El 404 global renderiza la página de la landing; `/api/*` y `/admin*`
  conservan el 404 por defecto.

### Verificación
- `go build ./...`, `go vet` y tests de `internal/httpapi`/`internal/config` OK.
- `docker compose restart botd`; `/` y `/static/css/style.css` responden 200 y
  `/admin/login`, `/health` y `/api/v1/ping` mantienen su comportamiento.

### Archivos
- `internal/landing/*`, `internal/httpapi/router.go`, `internal/config/config.go`,
  `cmd/botd/main.go`, `docker-compose.yml`, `deploy/docker-compose.yml`,
  `.env.example`, `docs/CHANGELOG.md`.

## 2026-10-28 · Conversación muestra mensajes del cliente y del asistente

### Corregido
- En *Mensajes* solo se pintaba un globo por interacción (priorizando
  `user_message`), por lo que **la respuesta del asistente no aparecía** cuando
  acompañaba a un mensaje del cliente. Ahora se renderizan ambos globos en orden
  (cliente → bot/operador).

### Verificación
- `go build ./...` y `go test ./internal/httpapi` OK.

### Archivos
- `internal/httpapi/admin/index.html`, `docs/CHANGELOG.md`.

## 2026-10-28 · Interruptor de IA por usuario en la bandeja de mensajes

### Añadido
- **Switch** junto al nombre de cada hilo en *Mensajes* para activar/desactivar
  la IA de ese usuario sin salir del panel (`POST /admin/api/users/{id}/active`).
- La bandeja incluye el estado `active` de cada usuario y el pill de la
  conversación se sincroniza al cambiar el switch.

### Cambiado
- Enviar un mensaje manual desde el panel **desactiva automáticamente la IA** del
  usuario tras el envío correcto.

### Verificación
- `gofmt`, `go build ./...`, `go vet ./...`, `go test ./...` OK.

### Archivos
- `internal/httpapi/admin.go`, `internal/httpapi/admin/index.html`,
  `internal/store/repo/interactions.go`, `docs/{06-api,CHANGELOG}.md`.

## 2026-10-28 · Título del operador con iniciales en mayúscula

### Cambiado
- El nombre del operador que firma los mensajes manuales (`*<nombre>*`) se
  muestra en formato título: la primera letra de cada palabra en mayúscula
  (p. ej. `maría josé` → `María José`), sin alterar el resto del nombre.

### Verificación
- `gofmt`, `go test ./internal/httpapi` y `go vet ./internal/httpapi` OK.

### Archivos
- `internal/httpapi/admin.go`, `internal/httpapi/admin_test.go`, `docs/CHANGELOG.md`.

## 2026-10-26 · Mensajes manuales firmados con el nombre del operador

### Añadido
- Campo opcional **Tu nombre (operador)** en `/admin/login`; se guarda en el
  token de sesión firmado.
- Variable **`ADMIN_OPERATOR_NAME`** (`.env`) como nombre por defecto; el nombre
  del login lo sobreescribe si se indica.
- Los envíos manuales desde el panel se prefijan con `*<nombre>*\n\n` en vez de
  aparecer como `*Asistente Virtual*` (o sin firma). Sin nombre, se envía sin prefijo.

### Verificación
- `go test ./internal/httpapi` OK; envío a sí mismo almacena `*Lledó*\n\n…` y,
  sin nombre en login, `*Operador*\n\n…` desde `ADMIN_OPERATOR_NAME`.

### Archivos
- `internal/config/config.go`, `cmd/botd/main.go`, `internal/httpapi/{admin.go,admin/login.html,admin_test.go}`,
  `docker-compose.yml`, `.env`, `.env.example`, `docs/{06-api,CHANGELOG}.md`.

## 2026-10-26 · Fix panel: conversación y envío con JID `@lid`

### Corregido
- **Seleccionar conversación no mostraba el chat / enviar daba error**: chi usa
  `RawPath`, así que `{id}` llegaba con escapes (`%40`) y se consultaba/enviaba a
  un JID inválido. Nuevo helper `pathParam` que hace `url.PathUnescape`; aplicado
  a todos los `{id}`/`{code}` (también arregla editar/borrar usuarios `@lid`).

### Verificación
- Conversación y `send` (auto-mensaje) devuelven 200; `GET` de usuario `@lid` OK.

### Archivos
- `internal/httpapi/admin.go`, `docs/CHANGELOG.md`.

## 2026-10-26 · Fix panel: pestañas vacías + importar contactos

### Corregido
- **Pestañas Mensajes/Usuarios/WhatsApp vacías**: `switchTab` generaba ids
  erróneos (`viewWhatsapp`/`tabWhatsapp` en vez de `viewWhatsApp`/`tabWhatsApp`).
  El `null.classList` lanzaba y abortaba los loaders. Ahora usa un mapa de ids
  y captura errores con toast.

### Añadido
- Botón **Importar contactos** en Usuarios (`POST /admin/api/users/import`):
  copia `whatsmeow_contacts` a `users`, mapeando PN→LID (vía `whatsmeow_lid_map`)
  y normalizando LIDs con sufijo de dispositivo (`123:71@lid` → `123@lid`).
  Idempotente (`ON CONFLICT DO NOTHING`); no sobreescribe usuarios existentes.

### Verificación
- `go build ./...`/`go vet ./...` OK; import idempotente (87 importados, 0 en
  segunda pasada); `/admin` sirve la UI nueva.

### Archivos
- `internal/httpapi/admin/index.html`, `internal/httpapi/admin.go`,
  `internal/store/repo/users.go`, `docs/{06-api,CHANGELOG}.md`.

## 2026-10-26 · Panel `/admin`: usuarios, mensajes y QR de WhatsApp
### Añadido
- Panel `/admin` con tres áreas nuevas:
  - **Usuarios**: CRUD completo (listar/buscar/crear/editar/borrar) sobre la
    tabla `users` (nombre, alias, idioma, tema, rol, buffer, temperatura, IA activa).
  - **Mensajes**: bandeja con la última interacción por usuario (`ListThreads`,
    `DISTINCT ON`) y conversación con **respuesta manual** vía WhatsApp.
  - **WhatsApp**: estado de la conexión y **QR de vinculación** (`/admin/api/whatsapp/qr.png`,
    generado con `skip2/go-qrcode`).
- `repo.Users.Create`/`Update` (campos completos) y `repo.Interactions.ListThreads`.
- La tabla del panel ahora ocupa hasta **1440px** de ancho.

### Cambiado
- `newAdminHandlers` recibe `users`, `interactions` y el proveedor WhatsApp.

### Verificación
- `go build ./...`, `go vet ./...`, `go test ./...` OK.

### Archivos
- Nuevos: `internal/store/repo` (`Users.Create`/`Update`, `Interactions.ListThreads`).
- Modificados: `internal/httpapi/{admin,router}.go`,
  `internal/httpapi/admin/index.html`, `go.mod`, `go.sum`,
  `README.md`, `docs/{README,06-api,07-operacion,CHANGELOG}.md`.

## 2026-10-26 · Mensajes fijos en BD + assets seed en `data/seeds`

### Cambiado
- Los mensajes fijos del system prompt (`FUNCIONES DISPONIBLES`) se leen en
  runtime desde `content_blocks` (BD) y no desde `data/messages.json`.
  `PromptFactory` recibe `repo.Instructions` y `Compile` pide `ctx`.
- `assistant.Config` deja de exponer `PromptsDir`/`DataDir`; `assistant.New`
  recibe `instructions` como parámetro.

### Reorganizado
- Los assets seed se agrupan en `data/seeds/`: `prompts/`, `services.json`,
  `messages.json`, `fix_messages/` y `embeddings.json`. La carpeta raíz
  `prompts/` desaparece.
- `PROMPTS_DIR` por defecto `data/seeds/prompts` y `DATA_DIR` `data/seeds`
  (config, `.env`, `.env.example`, `docker-compose.yml`, `Dockerfile.dev`,
  `deploy/Dockerfile`).

### Verificación
- `go build ./...`, `go vet ./...`, `go test ./...` OK.

### Archivos
- Movidos: `prompts/` → `data/seeds/prompts/`; `data/{services,messages,embeddings}.json`
  y `data/fix_messages/` → `data/seeds/`.
- Modificados: `internal/assistant/{assistant,prompts}.go`,
  `internal/config/config.go`, `cmd/{botd,seed,embed}/main.go`, `Dockerfile.dev`,
  `deploy/Dockerfile`, `docker-compose.yml`, `.env`, `.env.example`,
  `README.md`, `docs/{04,05,07,08,09,CHANGELOG}.md`.

## 2026-10-26 · Modelo LLM → GPT-6.1 Sol + migración a Responses API

### Cambiado
- `OPENAI_MODEL` por defecto pasa de `gpt-4.1` a `gpt-6.1-sol` (GPT-6.1 Sol:
  rendimiento cercano a Astra con mejor relación coste/precio).
- El asistente migra de **Chat Completions** a la **Responses API**, porque los
  modelos GPT-6.x solo admiten tool calling en la Responses API
  (`assistant.go`). `buildMessages`/`complete` ahora usan `CreateResponse` con
  `instructions` + `input` y `function_call`/`function_call_output`.
- `tools.go`: `buildTools` devuelve `[]openai.ResponseTool`
  (`NewResponseFunctionTool`); `execTool` recibe nombre y argumentos.

### Verificación
- `go build ./...`, `go vet ./...`, `go test ./...` OK.
- Pendiente de prueba real contra OpenAI (Respuestas API + tool calling).

### Archivos
- Modificados: `internal/assistant/{assistant,tools}.go`,
  `internal/config/config.go`, `.env`, `.env.example`, `docker-compose.yml`,
  `deploy/docker-compose.yml`, `docs/07-operacion.md`, `docs/CHANGELOG.md`.

## 2026-10-26 · Documento de diseño: modelo de decisión (guarda de entrada)

### Añadido
- `docs/10-modelo-decision.md`: diseño de una capa de decisión previa al LLM
  que clasifica seguridad (spam/malicioso/ataque), intención de función y modo
  de respuesta (mensaje fijo vs. generación libre), con enrutado determinista.
  Basada en la **OpenAI Decisions API** (`gpt-6-luna`) con una **capa de
  abstracción de proveedores** (`Provider`: OpenAI / Jev-TypeSafe / otro) y una
  **clave de API separada** (`DECISION_API_KEY`, reutilizable de OpenAI).
  Resuelve el bug del "si" (respuesta indebida de repaso) y reduce coste/latencia.

### Archivos
- Nuevos: `docs/10-modelo-decision.md`.
- Modificados: `docs/README.md` (índice), `docs/CHANGELOG.md`.

## 2026-10-23 · Panel `/admin` embebido + prompts y servicios en PostgreSQL

### Añadido
- Panel de administración embebido en `botd` (`/admin`): login por sesión HMAC,
  CRUD de servicios, CRUD de prompts, historial de versiones y rollback.
- Migraciones `00003_prompts_services.sql` (`prompts`, `prompt_versions`,
  `services`) y `00004_prompt_services_cleanup.sql` (embedding legado).
- `internal/bootstrap`: siembra inicial desde `prompts/*.md` y `data/services.json`
  (solo si las tablas están vacías); parser del catálogo markdown.
- Reindexado RAG en caliente al guardar/restaurar prompts y borrado del embedding
  al eliminarlos.

### Cambiado
- `PromptFactory` lee prompts y catálogo de servicios desde PostgreSQL, con
  instantánea en memoria invalidada por el panel.
- `cmd/embed` ingiere prompts desde la BD (no desde ficheros).
- `cmd/seed` ejecuta el bootstrap de prompts/servicios.
- Los `.md` y `services.json` pasan a ser **seed**; el runtime usa la BD.
- Corregido el ID `corp_cio_esc` → `corp_crio_esc` en `data/services.json`.

### Variables
- Nuevas: `ADMIN_ENABLED`, `ADMIN_PASSWORD`, `ADMIN_SESSION_SECRET`,
  `ADMIN_SESSION_TTL`. En dev sin `ADMIN_PASSWORD` se usa `API_KEY`; en
  producción el panel queda deshabilitado.

### Verificación
- `go build ./...`, `go vet ./...`, `go test ./...` OK.
- Migración + seed sobre la BD de desarrollo: 34 prompts, 48 servicios.
- Prueba HTTP: login 303, CRUD de servicios/prompts/versiones/restore OK.
- `docker compose restart botd`: `whatsapp conectado`, `/admin/login` 200.

### Archivos
- Nuevos: `internal/store/migrations/sql/00003_prompts_services.sql`,
  `internal/store/migrations/sql/00004_prompt_services_cleanup.sql`,
  `internal/store/repo/prompts.go`, `internal/store/repo/services.go`,
  `internal/bootstrap/{content,services,services_test}.go`,
  `internal/httpapi/{admin,admin_test}.go`, `internal/httpapi/admin/{index,login}.html`.
- Modificados: `internal/assistant/{assistant,prompts}.go`, `internal/rag/rag.go`,
  `internal/httpapi/router.go`, `internal/config/config.go`,
  `cmd/{botd,seed,embed}/main.go`, `docker-compose.yml`,
  `deploy/docker-compose.yml`, `data/services.json`, `.env.example`,
  `README.md`, `docs/{01-arquitectura,04-asistente-ia,05-datos,06-api,07-operacion,08-estructura-repo,README}.md`.

## 2026-10-23 · Eliminados Brow Lamination y Lash Lifting del catálogo

### Cambiado
- Eliminados `mir_lam_cejas` (Laminado de cejas) y `mir_lift_pest`
  (Lifting y tinte de pestañas) de listas de precios, prompts, embeddings,
  mappings, staff, menús, iniciales y mensajes fijos (ES/EN).
- Renumerados los menús/iniciales (7–9) y eliminada Laura Miquel de las
  funciones del personal (solo tenía esos dos servicios).
- Borrados `prompt_lamination_brow`, `prompt_lash_lifting`,
  `message_brow_lamination` y `message_lash_lifting`.

### Operación
- Borrar embeddings obsoletos:
  `DELETE FROM embeddings WHERE code IN ('prompt_lamination_brow','prompt_lash_lifting','prompt_services_list copy');`
- `make embed`, `make seed` y reiniciar `botd`.
- Limpiar el historial Redis de los chats de prueba para que el LLM no repita
  las listas antiguas.

### Archivos
- `prompts/`: `prompt_instructions`, `prompt_classify_message`,
  `prompt_eyebrow_services`; eliminados `prompt_lamination_brow` y
  `prompt_lash_lifting`.
- `data/`: `embeddings.json`, `staff.json`, `service_staff_mapping.json`;
  `fix_messages/`: `message_menu_es/en`, `message_initial_es/en`,
  `message_services_list_es/en`, `message_eyebrow_services_es/en`; eliminados
  `message_brow_lamination` y `message_lash_lifting`.
- `prompts/prompt_services_list.md` (edición previa del usuario).
- `docs/CHANGELOG.md`.

## 2026-10-23 · Reconexión de WhatsApp y log de mensajes entrantes

### Diagnóstico
- Tras el reinicio de `botd` (11:58) el `connect` inicial falló por timeout de
  DNS (`web.whatsapp.com`). whatsmeow no reintenta el connect inicial salvo que
  `InitialAutoReconnect` esté activo → canal `disconnected` hasta reiniciar.
- Los mensajes entrantes no se registraban en el log (solo errores/filtros).

### Cambiado
- `Client.Start`: activa `EnableAutoReconnect` + `InitialAutoReconnect` para que
  los fallos de red/DNS del arranque se reintenten en segundo plano.
- `Client.emit`: log INFO `mensaje de WhatsApp recibido` (id, chat, from_me,
  media, longitud y preview de 80 caracteres).
- Eliminado el `StatusConnected` prematuro tras `Connect`; el estado lo fija el
  evento `events.Connected`.

### Verificación
- `go build ./...` y `go test ./internal/whatsapp ./internal/pipeline` OK.
- Tras `docker compose restart botd`: `whatsapp conectado`, mensajes pendientes
  recibidos y respuesta generada (interacción 12:47:37).

### Archivos
- `internal/whatsapp/client.go`.
- `docs/CHANGELOG.md`.

## 2026-10-23 · Título “Asistente Virtual” en lugar del icono 🤖

### Cambiado
- Las respuestas del bot se prefijan con `*Asistente Virtual*` (negrita + salto de
  línea) en vez de `🤖`, sin duplicar el título si ya está presente.
- Mensaje degradado sin IA actualizado al mismo formato.

### Verificación
- `gofmt`, `go build ./...` y `go test ./internal/pipeline/...` OK.

### Archivos
- `internal/pipeline/filters.go`.
- `internal/assistant/assistant.go`.
- `docs/01-arquitectura.md`, `docs/02-flujo-mensaje.md`,
  `docs/04-asistente-ia.md`, `docs/09-flujo-conversacional.md`.
- `docs/CHANGELOG.md`.

## 2026-10-23 · Catálogo de servicios y equipo actualizados

### Cambiado
- Eliminadas depilación con hilo y visagismo del catálogo, menús y prompts.
- Eliminados Carmen Mir (`CM`) y Fabiola Betencourt (`FB`) del equipo, prompts,
  mappings y datos de ejemplo.
- Reasignados servicios que quedaban sin profesional: `nano_eq_ojos` → `LL`,
  `rem_las_eye` → `TF`.
- Eliminado el servicio de tatuaje Fine Line del catálogo y menús.
- Actualizados keywords de embeddings y contenidos fijos (menús/listas).

### Operación
- Regenerar embeddings (`make embed`), recargar contenidos (`make seed`) y
  reiniciar `botd` para aplicar los prompts.
- Borrar embeddings obsoletos si ya existían:
  `DELETE FROM embeddings WHERE code IN ('prompt_visagism','prompt_threading','prompt_threading_brown','prompt_tatoo');`

### Archivos
- `prompts/`: `prompt_instructions`, `prompt_services_list`, `prompt_services_list copy`,
  `prompt_eyebrow_services`, `prompt_lamination_brow`, `prompt_laser_diodo`,
  `prompt_classify_message`, `schema_response`; eliminados `prompt_visagism`,
  `prompt_threading`, `prompt_threading_brown`.
- `data/`: `staff.json`, `services.json`, `service_staff_mapping.json`,
  `embeddings.json` y `fix_messages/*` (menús, iniciales, listas y servicios de cejas).
- `docs/CHANGELOG.md`.

## 2026-10-23 · Documentado el flujo conversacional con decisiones y mejoras

### Añadido
- `docs/09-flujo-conversacional.md`: recorrido end-to-end (recepción, filtros,
  debounce, RAG, prompt, tools, acciones, envío, persistencia), escenarios,
  FSM de citas, comparativa legacy y mejoras priorizadas P0/P1/P2.

### Cambiado
- Índice de documentación y referencia cruzada desde `02-flujo-mensaje`.

### Archivos
- `docs/09-flujo-conversacional.md`.
- `docs/README.md`.
- `docs/02-flujo-mensaje.md`.
- `docs/CHANGELOG.md`.

## 2026-10-23 · Restaurado prefijo 🤖 en respuestas del LLM

### Cambiado
- Las respuestas enviadas por el pipeline (flujo normal y `force-reply`) se
  prefijan con `🤖` si aún no lo llevan, sin duplicar el icono.

### Verificación
- `gofmt`, `go build ./...` y `go test ./internal/pipeline/...` OK.

### Archivos
- `internal/pipeline/filters.go`.
- `internal/pipeline/service.go`.
- `docs/CHANGELOG.md`.

## 2026-10-22 · IA operativa + parseo de respuesta estructurada

### Añadido
- `assistant.parseStructuredReply`: si el modelo devuelve el JSON de
  `schema_response.md`, se envía solo `respond` (y se toman `language_code` y
  `satisfaction_level`); el texto plano se deja intacto.

### Cambiado
- `.env`: `OPENAI_API_KEY` válida (verificada con `GET /v1/models` → 200).
- `docker compose up -d botd` para aplicar el entorno; WhatsApp `connected`.

### Verificación
- `go build ./...` y `go vet ./internal/assistant/` OK.
- `force-reply` devuelve texto limpio (sin JSON).

### Archivos
- `internal/assistant/assistant.go`.
- `.env` (no versionado).
- `docs/CHANGELOG.md`.

## 2026-10-22 · Configuración de IA (.env) — bloqueada por clave inválida

### Operación
- WhatsApp re-vinculado a `34686115433` (Tester); estado `connected`.
- Creado `.env` desde `.env.example` con `API_KEY=dev-api-key` y
  `OPENAI_API_KEY` copiada de `.old/.env`.
- Prueba `force-reply` → OpenAI **401 Incorrect API key**: la clave legacy está revocada.
- Pendiente: sustituir `OPENAI_API_KEY` por una clave válida y recrear `botd`.

### Archivos
- `.env` (no versionado).
- `docs/CHANGELOG.md`.

## 2026-10-22 · Cambio de número de WhatsApp (desvinculación + nuevo QR)

### Operación
- Vinculado inicial: `34660291797` (Nestor Pons).
- `POST /whatsapp/logout` → `logged_out`; reinicio de `botd` y QR nuevo emitido.
- Pendiente de escanear con el número definitivo.
- Sin cambios de código; solo runtime.

### Archivos
- `docs/CHANGELOG.md`.

## 2026-10-22 · Reinicio de botd para regenerar QR de WhatsApp

### Operación
- Reiniciado `aibotia-dev-botd-1`: estado `waiting_qr`, QR nuevo emitido.
- Sesión de WhatsApp no vinculada (`qr_available=false` antes del reinicio).
- Sin cambios de código; solo runtime.

### Archivos
- `docs/CHANGELOG.md`.

## 2026-10-21 · Dockerfile de desarrollo (compila al arrancar)

### Añadido
- `Dockerfile.dev`: imagen `golang:1.27-alpine`; `go mod download` + `go build ./...`
  en el build (precalienta caché) y `go build -o /tmp/botd ./cmd/botd` en el arranque.
- Healthcheck propio de la imagen; assets y rutas de prompts/datos dentro de `/src`.

### Cambiado
- `docker-compose.yml`: `botd` se construye con `Dockerfile.dev` (imagen
  `aibotia-dev-botd`) en lugar de ejecutar `go run` sobre la imagen de Go.
- `.dockerignore`: excluye `.vscode/`.
- Docs de operación y README actualizados.

### Verificación
- `docker compose build botd` OK.
- `docker compose up -d` → `botd` healthy en ~43 s (incluye PostgreSQL/Redis),
  `/health` 200, `/ready` `{postgres:ok,redis:ok}`.

## 2026-10-21 · Compose de desarrollo local

### Añadido
- `docker-compose.yml` (raíz, proyecto `aibotia-dev`): PostgreSQL pgvector,
  Redis y `botd` con el repo montado (`go run ./cmd/botd`), healthchecks y
  volúmenes para datos y cachés de Go.
- `docs/07-operacion.md`: sección “Desarrollo local (compose raíz)”.

### Verificación
- `docker compose config` OK.
- E2E: dependencias healthy → migraciones (9 tablas) → `/ready`
  `{postgres:ok,redis:ok}`, `/health` 200, auth 403/200, contenedor `botd` healthy.

## 2026-10-21 · Reestructuración del repo + documentación

### Estructura
- Proyecto Go promovido de `aibotia-go/` a la **raíz** (`cmd/`, `internal/`, `deploy/`, `prompts/`, `data/`).
- Todo el sistema legacy (Node, MariaDB, Redis, tests, docs, `node_modules`, `.env`) movido a **`.old/`**.
- `.old/` excluido de git (`.gitignore`) y del contexto Docker (`.dockerignore`).
- `.vscode/sftp.json`: añadidos `ignore` para `.old`, `bin`, `deploy`.
- `deploy/CUTOVER.md`: rutas actualizadas (sin `cd aibotia-go`).

### Correcciones
- `deploy/Dockerfile`: la imagen final ahora copia `prompts/` y `data/` a `/app`
  y define `PROMPTS_DIR`/`DATA_DIR` (antes el contenedor arrancaba sin assets).
- `.dockerignore`: se deja de excluir `data/` para poder empaquetar los assets.

### Documentación (nueva)
- `docs/README.md` — índice y estado.
- `docs/01-arquitectura.md` — componentes, capas, arranque, concurrencia, deuda.
- `docs/02-flujo-mensaje.md` — recorrido completo del mensaje.
- `docs/03-whatsapp.md` — `whatsmeow`, sesión, media, límites.
- `docs/04-asistente-ia.md` — prompts, RAG, tools, memoria, idioma.
- `docs/05-datos.md` — esquema PostgreSQL, Redis, assets, migración.
- `docs/06-api.md` — endpoints, auth, errores, SSE.
- `docs/07-operacion.md` — entorno, Makefile, Docker, cutover, troubleshooting.
- `docs/08-estructura-repo.md` — árbol, convenciones, mapa legacy→Go.
- `README.md` reescrito para el nuevo layout.

### Verificación
- `go build ./...` OK.
- `go vet ./...` OK.
- `go test ./...` OK.

### Notas
- El repositorio no es git; se recomienda inicializarlo antes de operar.
- La clave OpenAI del legacy sigue en `.old/.env`: rotar y no versionar.
