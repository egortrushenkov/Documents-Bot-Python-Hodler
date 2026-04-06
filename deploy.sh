#!/bin/bash
# =============================================================
# Первичная установка бота на Timeweb VPS (Ubuntu)
# Запускать один раз: bash deploy.sh
# =============================================================
set -e

REPO_URL="https://github.com/YOUR_USER/deal-docs-bot.git"  # <-- вставь свой репо
APP_DIR="/opt/deal-docs-bot"
DATA_DIR="$APP_DIR/data"

echo "=== [1/5] Установка Docker ==="
if ! command -v docker &>/dev/null; then
    curl -fsSL https://get.docker.com | sh
    systemctl enable docker
    systemctl start docker
fi
if ! command -v docker compose &>/dev/null; then
    apt-get install -y docker-compose-plugin
fi

echo "=== [2/5] Клонирование репозитория ==="
if [ -d "$APP_DIR" ]; then
    echo "Директория уже существует, обновляю..."
    cd "$APP_DIR" && git pull
else
    git clone "$REPO_URL" "$APP_DIR"
    cd "$APP_DIR"
fi

echo "=== [3/5] Создание папок и конфига ==="
mkdir -p "$DATA_DIR/output"

if [ ! -f "$APP_DIR/.env" ]; then
    cp "$APP_DIR/.env.example" "$APP_DIR/.env"
    echo ""
    echo "⚠️  Заполните .env файл:"
    echo "    nano $APP_DIR/.env"
    echo ""
    echo "Затем положите шаблон:"
    echo "    $DATA_DIR/TEMPLATE_hodler_deal.docx"
    echo ""
    echo "И перезапустите: bash $APP_DIR/deploy.sh"
    exit 0
fi

if [ ! -f "$DATA_DIR/TEMPLATE_hodler_deal.docx" ]; then
    echo "❌ Шаблон не найден: $DATA_DIR/TEMPLATE_hodler_deal.docx"
    echo "   Скопируйте шаблон и повторите запуск."
    exit 1
fi

echo "=== [4/5] Сборка и запуск ==="
cd "$APP_DIR"
docker compose pull 2>/dev/null || true
docker compose up -d --build

echo "=== [5/5] Готово ==="
docker compose ps
echo ""
echo "✅ Бот запущен!"
echo "   Логи: docker compose -f $APP_DIR/docker-compose.yml logs -f"
echo "   Стоп: docker compose -f $APP_DIR/docker-compose.yml down"
