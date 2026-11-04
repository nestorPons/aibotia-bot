# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
ARG COMMIT=none
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags "-s -w -X github.com/aibotia/aibotia-go/internal/version.Version=${VERSION} -X github.com/aibotia/aibotia-go/internal/version.Commit=${COMMIT}" \
    -o /out/botd ./cmd/botd

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/botd /usr/local/bin/botd
COPY data ./data
USER app
EXPOSE 3000
ENV TZ=Europe/Madrid \
    PROMPTS_DIR=/app/data/seeds/prompts \
    DATA_DIR=/app/data/seeds
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O- http://127.0.0.1:3000/health || exit 1
ENTRYPOINT ["botd"]
