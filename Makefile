BINARY  := botd
PKG     := github.com/aibotia/aibotia-go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X $(PKG)/internal/version.Version=$(VERSION) \
	-X $(PKG)/internal/version.Commit=$(COMMIT) \
	-X $(PKG)/internal/version.Date=$(DATE)

.PHONY: run build test lint fmt vet tidy migrate-up migrate-down docker-up docker-down dev-build dev-apply dev-reload tester tester-list tester-case tester-suite tester-docker tester-serve

run:
	go run ./cmd/botd

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/botd

test:
	go test ./...

lint:
	golangci-lint run

fmt:
	gofmt -s -w .

vet:
	go vet ./...

tidy:
	go mod tidy

migrate-up:
	go run ./cmd/migrate

migrate-down:
	go run ./cmd/migrate -down

embed:
	go run ./cmd/embed

seed:
	go run ./cmd/seed

migrate-data:
	go run ./cmd/migrate-data

loadtest:
	go run ./cmd/loadtest -key "$(API_KEY)"

docker-up:
	docker compose -f deploy/docker-compose.yml up --build -d

docker-down:
	docker compose -f deploy/docker-compose.yml down

# Compila dentro del contenedor de desarrollo sin reconstruir la imagen.
dev-build:
	docker compose exec -T botd go build ./...

# Aplica cambios (incluido el panel) sin reconstruir la imagen: reinicia botd y
# su CMD recompila el binario dentro del contenedor con el código montado.
dev-apply:
	docker compose restart botd
	@printf "recompilando botd"
	@for i in $$(seq 1 60); do \
		if curl -fsS "http://localhost:$${HTTP_PORT:-3000}/health" >/dev/null 2>&1; then \
			echo " · listo: http://localhost:$${HTTP_PORT:-3000}/admin"; exit 0; \
		fi; \
		printf "."; sleep 1; \
	done; \
	echo " · timeout; revisa: docker compose logs botd"; exit 1

# Alias histórico de dev-apply.
dev-reload: dev-apply

# Bot de pruebas: conversa con el chatbot desde otro WhatsApp (QR por consola).
# El destino se toma de TEST_TARGET (.env o entorno).
tester:
	go run ./cmd/tester $(TESTER_FLAGS)

# Catálogo de casos disponibles (no necesita WhatsApp ni BD)
tester-list:
	go run ./cmd/tester -list

# Un caso concreto: make tester-case CASE=adversarial TESTER_FLAGS=-eval
tester-case:
	@test -n "$(CASE)" || { echo "Uso: make tester-case CASE=<id>   (ver: make tester-list)"; exit 1; }
	go run ./cmd/tester $(TESTER_FLAGS) -case $(CASE)

# Suite completa (o varios): make tester-suite SUITE=all
tester-suite:
	go run ./cmd/tester $(TESTER_FLAGS) -suite $(or $(SUITE),all)

tester-docker:
	docker compose -f deploy/docker-compose.prod.yml --profile tester run --rm --build tester $(TESTER_FLAGS)

# Ver los logs guardados en el navegador (no necesita WhatsApp ni BD).
# Uso: make tester-serve  ·  luego abrir http://127.0.0.1:8091
tester-serve:
	go run ./cmd/tester -serve 127.0.0.1:$(or $(PORT),8091)
