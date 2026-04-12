# Hodler Doc Generator Bot

Telegram-бот для автоматической генерации актов и договоров на сделки с виртуальными активами для юридических лиц.

## Что умеет

- **2 типа акта**: «Продаём ВА клиенту» и «Покупаем ВА у клиента»
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

### 3. Положить оригинальные шаблоны

В папку `templates/` поместить оба файла:
- `Акт_покупке_у_нас_ЮЛ.docx`
- `Акт_продаже_нам_от_ЮЛ.docx`

### 4. Запустить

```bash
docker compose up -d --build
```

При первом запуске автоматически:
- подготовятся шаблоны (`prepare_templates.py`)
- создастся БД с реквизитами оператора по умолчанию

---

## Локальный запуск (без Docker)

### Требования
- Python 3.11+
- LibreOffice (`libreoffice` в PATH)

```bash
pip install -r requirements.txt

# Подготовить шаблоны (один раз)
python prepare_templates.py

# Запустить бота
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
├── prepare_templates.py    # Подготовка шаблонов (один раз)
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
│   ├── Акт_покупке_у_нас_ЮЛ.docx   # Оригинал (не трогать)
│   ├── Акт_продаже_нам_от_ЮЛ.docx  # Оригинал (не трогать)
│   ├── template_sell.docx           # Генерируется автоматически
│   └── template_buy.docx            # Генерируется автоматически
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
`{{OP_FULL_NAME}}`, `{{OP_SHORT_NAME}}`, `{{OP_INN}}`, `{{OP_KPP}}`, `{{OP_ADDRESS}}`, `{{OP_LICENSE}}`, `{{OP_BANK_NAME}}`, `{{OP_BANK_ACCOUNT}}`, `{{OP_BANK_BIK}}`, `{{OP_DIRECTOR}}`

### Клиент (`CL_*`)
`{{CL_FULL_NAME}}`, `{{CL_SHORT_NAME}}`, `{{CL_INN}}`, `{{CL_KPP}}`, `{{CL_REG_NUMBER}}`, `{{CL_ADDRESS}}`, `{{CL_KIO}}`, `{{CL_INN_RF}}`, `{{CL_KPP_RF}}`, `{{CL_BANK_NAME}}`, `{{CL_BANK_ACCOUNT}}`, `{{CL_BANK_BIK}}`

---

## Обновление шаблонов

Если изменились оригинальные DOCX:
1. Заменить файлы в `templates/`
2. Запустить `python prepare_templates.py`
3. Перезапустить бота

---

## Деплой на Timeweb VPS (без Docker)

```bash
# На сервере
cd /opt/hodler-doc-bot
pip install -r requirements.txt
python prepare_templates.py

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
