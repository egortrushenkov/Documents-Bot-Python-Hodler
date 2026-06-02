# Hodler Doc Generator Bot

Telegram-бот для автоматической генерации актов и договоров на сделки с виртуальными активами для юридических лиц.

## Что умеет

- **2 типа акта**: «Продаём ВА клиенту» и «Покупаем ВА у клиента»
- **Счёт-заявка**: отдельный документ на покупку клиентом (`template_invoice_buy`) — формируется до акта
- **Несколько транзакций в акте**: добавляются строками в таблицу, сумма ВА суммируется в «Итого»
- **База компаний**: добавить один раз — реквизиты подтянутся автоматически
- **Автодата**: сегодняшнее число по умолчанию, можно изменить
- **Автономер**: счётчик заявок с возможностью ручного ввода
- **Автокурс**: вычисляется из суммы ВА и суммы RUB, можно скорректировать
- **КВВО**: выбор из часто используемых или ввод своего
- **PDF**: автоматическая конвертация через LibreOffice
- **Редактирование**: любое поле можно изменить перед генерацией

---

## Быстрый старт (Docker — рекомендуется)

### 1. Клонировать / скопировать проект

```bash
git clone <repo> hodler-doc-bot
cd hodler-doc-bot
```

### 2. Создать `.env`

```bash
cp .env.example .env
nano .env
```

Заполнить `BOT_TOKEN` и `ADMIN_IDS`.

### 3. Положить готовые шаблоны

В папку `templates/` поместить заполненные плейсхолдерами `{{...}}` файлы:
- `template_buy.docx` — клиент покупает ВА у нас
- `template_sell.docx` — клиент продаёт ВА нам
- `template_invoice_buy.docx` — счёт-заявка на покупку клиентом

Шаблоны создаются вручную (см. список плейсхолдеров ниже).

### 4. Запустить

```bash
docker compose up -d --build
```

При первом запуске автоматически создастся БД с реквизитами оператора по умолчанию.

---

## Локальный запуск (без Docker)

### Требования
- Python 3.11+
- LibreOffice (`libreoffice` в PATH)

```bash
pip install -r requirements.txt

# Запустить бота (шаблоны должны уже лежать в templates/)
python bot.py
```

---

## Структура проекта

```
hodler-doc-bot/
├── bot.py                  # Точка входа
├── config.py               # Конфиг из .env
├── database.py             # SQLite: компании, настройки, счётчик
├── states.py               # FSM-состояния
├── keyboards.py            # Все клавиатуры
│
├── handlers/
│   ├── menu.py             # Главное меню
│   ├── companies.py        # CRUD компаний
│   ├── acts.py             # Создание акта (15 шагов FSM)
│   └── settings.py         # Реквизиты оператора
│
├── services/
│   ├── doc_generator.py    # Генерация DOCX из шаблона
│   └── pdf_converter.py    # Конвертация DOCX → PDF
│
├── templates/
│   ├── template_buy.docx           # Клиент покупает ВА у нас
│   ├── template_sell.docx          # Клиент продаёт ВА нам
│   └── template_invoice_buy.docx   # Счёт-заявка на покупку клиентом
│
├── data/
│   ├── hodler.db           # SQLite-база
│   └── output/             # Сгенерированные файлы
│
├── Dockerfile
├── docker-compose.yml
├── requirements.txt
└── .env.example
```

---

## Поля компании (сохраняются в базе)

| Поле | Описание |
|------|----------|
| Полное наименование | «Общество с ограниченной ответственностью...» |
| Краткое наименование | «ОсОО «...»» |
| ИНН | |
| КПП | |
| Рег. номер (ОГРН/КР) | |
| Юридический адрес | |
| КИО | Код иностранной организации в РФ (или `-`) |
| ИНН в РФ | (или `-`) |
| КПП в РФ | (или `-`) |
| Банк | Наименование банка |
| Расчётный счёт | |
| БИК | |
| Кошелёк | Адрес крипто-кошелька |

---

## Переменные в документах

Шаблоны содержат плейсхолдеры вида `{{VARIABLE}}`:

### Сделка
| Плейсхолдер | Значение |
|-------------|----------|
| `{{DEAL_NUMBER}}` | Номер заявки |
| `{{DEAL_DATE}}` | Дата заявки (ДД.ММ.ГГГГ) |
| `{{DEAL_DATE_FULL}}` | «ДД» месяц ГГГГ г. |
| `{{EXECUTION_DATE}}` | Дата исполнения |
| `{{OPERATION_TYPE}}` | Покупка / Продажа виртуальных активов |
| `{{KVVO}}` | Код вида валютной операции |
| `{{VA_TYPE}}` | USDT, BTC... |
| `{{NETWORK}}` | TRC-20, ERC-20... |
| `{{VA_TICKER}}` | USDT_TRC20... |
| `{{VA_AMOUNT}}` | Сумма ВА |
| `{{FIAT_AMOUNT}}` | Сумма RUB |
| `{{EXCHANGE_RATE}}` | Курс обмена |
| `{{TX_HASH}}` | Хэш транзакции |
| `{{COMMISSION_FIAT}}` | Комиссия в % (RUB) |
| `{{COMMISSION_VA}}` | Комиссия в % (ВА) |
| `{{CL_WALLET}}` | Кошелёк клиента |
| `{{OP_WALLET}}` | Кошелёк оператора |

### Оператор (`OP_*`)
`{{OP_FULL_NAME}}`, `{{OP_SHORT_NAME}}`, `{{OP_INN}}`, `{{OP_KPP}}`, `{{OP_ADDRESS}}`, `{{OP_ADDRESS_FULL}}`, `{{OP_LEGAL_ADDRESS}}`, `{{OP_LICENSE}}`, `{{OP_KIO}}`, `{{OP_INN_RF}}`, `{{OP_KPP_RF}}`, `{{OP_BANK_NAME}}`, `{{OP_BANK_ACCOUNT}}`, `{{OP_BANK_BIK}}`, `{{OP_DIRECTOR}}`

### Клиент (`CL_*`)
`{{CL_FULL_NAME}}`, `{{CL_SHORT_NAME}}`, `{{CL_INN}}`, `{{CL_KPP}}`, `{{CL_REG_NUMBER}}`, `{{CL_ADDRESS}}`, `{{CL_KIO}}`, `{{CL_INN_RF}}`, `{{CL_KPP_RF}}`, `{{CL_BANK_NAME}}`, `{{CL_BANK_ACCOUNT}}`, `{{CL_BANK_BIK}}`

---

## Обновление шаблонов

Шаблоны редактируются вручную в Word. Чтобы значения подставлялись автоматически,
используйте плейсхолдеры `{{VARIABLE}}` (см. список ниже).
1. Отредактировать `.docx` в `templates/`
2. Перезапустить бота

---

## Деплой на Timeweb VPS (без Docker)

```bash
# На сервере
cd /opt/hodler-doc-bot
pip install -r requirements.txt

# Создать systemd-сервис
sudo nano /etc/systemd/system/hodler-bot.service
```

```ini
[Unit]
Description=Hodler Doc Generator Bot
After=network.target

[Service]
WorkingDirectory=/opt/hodler-doc-bot
ExecStart=/usr/bin/python3 bot.py
Restart=always
RestartSec=5
EnvironmentFile=/opt/hodler-doc-bot/.env

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable hodler-bot
sudo systemctl start hodler-bot
sudo journalctl -u hodler-bot -f
```
