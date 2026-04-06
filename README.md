# Hodler Deal Docs Bot

Telegram-бот для генерации закрывающих документов (DOCX + PDF) по сделкам Hodler Exchange.

## Быстрый старт

### 1. Создать бота
Напишите @BotFather → `/newbot` → получите `BOT_TOKEN`

### 2. Узнать свой Telegram ID
Напишите @userinfobot → скопируйте ID

### 3. Первичная установка на сервере
```bash
# На VPS (Ubuntu):
curl -fsSL https://raw.githubusercontent.com/egortrushenkov/deal-docs-bot/main/deploy.sh | bash
```

Скрипт установит Docker, склонирует репо и попросит заполнить `.env`.

### 4. Заполнить `.env`
```bash
nano /opt/deal-docs-bot/.env
```
```env
BOT_TOKEN=1234567890:AAxxxxxxxxxxxxxxxx
ALLOWED_USER_IDS=123456789,987654321
```

### 5. Положить шаблон
```bash
# С локальной машины:
scp TEMPLATE_hodler_deal.docx user@YOUR_VPS:/opt/deal-docs-bot/data/
```

### 6. Запустить
```bash
bash /opt/deal-docs-bot/deploy.sh
```

---

## Автодеплой (GitHub Actions)

При каждом `git push main` бот автоматически обновляется на сервере.

**Настройка секретов** в GitHub → Settings → Secrets → Actions:

| Secret | Значение |
|--------|---------|
| `VPS_HOST` | IP вашего сервера |
| `VPS_USER` | `root` или ваш пользователь |
| `VPS_SSH_KEY` | Приватный SSH-ключ (содержимое `~/.ssh/id_rsa`) |

Если SSH-ключа нет — создайте на сервере:
```bash
ssh-keygen -t ed25519 -C "github-actions"
cat ~/.ssh/id_ed25519.pub >> ~/.ssh/authorized_keys
cat ~/.ssh/id_ed25519  # скопируйте в VPS_SSH_KEY
```

---

## Команды бота

| Команда | Действие |
|---------|---------|
| `/new` | Создать новый документ |
| `/clients` | Список сохранённых клиентов |
| `/cancel` | Отменить текущий диалог |

---

## Структура проекта

```
deal-docs-bot/
├── bot/
│   ├── main.py        — точка входа
│   ├── handlers.py    — FSM-диалог
│   ├── states.py      — состояния диалога
│   ├── generator.py   — генерация DOCX/PDF
│   ├── clients.py     — база клиентов
│   └── keyboards.py   — кнопки
├── data/              — монтируется как volume
│   ├── TEMPLATE_hodler_deal.docx  ← положить вручную
│   ├── clients.json               ← создаётся автоматически
│   └── output/                    ← готовые документы
├── Dockerfile
├── docker-compose.yml
├── .env.example
├── deploy.sh          — первичная установка
└── .github/workflows/deploy.yml  — автодеплой
```

## Полезные команды на сервере

```bash
# Логи в реальном времени
docker compose -f /opt/deal-docs-bot/docker-compose.yml logs -f

# Перезапуск
docker compose -f /opt/deal-docs-bot/docker-compose.yml restart

# Остановка
docker compose -f /opt/deal-docs-bot/docker-compose.yml down

# Ручное обновление без автодеплоя
cd /opt/deal-docs-bot && git pull && docker compose up -d --build
```
