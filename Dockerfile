# Откуда брать базовые образы. По умолчанию — зеркало Docker Hub от Google:
# у Docker Hub жёсткий лимит анонимных скачиваний (ошибка 429 Too Many Requests).
# Переопределить: DOCKER_REGISTRY=docker.io/library в .env
ARG REGISTRY=mirror.gcr.io/library

# ─── Сборка ───────────────────────────────────────────────────────────────────
FROM ${REGISTRY}/golang:1.26 AS build
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
# CGO не нужен: SQLite — чистый Go (modernc.org/sqlite)
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hodler-bot ./cmd/hodler-bot

# ─── Рантайм ──────────────────────────────────────────────────────────────────
FROM ${REGISTRY}/debian:bookworm-slim

# LibreOffice для PDF-конвертации, сертификаты для HTTPS к Telegram
RUN apt-get update && apt-get install -y --no-install-recommends \
    libreoffice-writer \
    fonts-liberation \
    ca-certificates \
    tzdata \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /out/hodler-bot /usr/local/bin/hodler-bot
COPY templates ./templates
RUN mkdir -p data/output

CMD ["hodler-bot"]
