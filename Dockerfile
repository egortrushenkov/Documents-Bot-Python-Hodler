# ─── Сборка ───────────────────────────────────────────────────────────────────
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
# CGO не нужен: SQLite — чистый Go (modernc.org/sqlite)
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hodler-bot ./cmd/hodler-bot

# ─── Рантайм ──────────────────────────────────────────────────────────────────
FROM debian:bookworm-slim

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
