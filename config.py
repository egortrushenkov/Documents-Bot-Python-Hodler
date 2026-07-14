import os
from dotenv import load_dotenv

load_dotenv()

BOT_TOKEN = os.getenv("BOT_TOKEN", "")
ADMIN_IDS_RAW = os.getenv("ADMIN_IDS", "")
ADMIN_IDS = [int(x.strip()) for x in ADMIN_IDS_RAW.split(",") if x.strip()]

DB_PATH = os.getenv("DB_PATH", "data/hodler.db")
TEMPLATES_DIR = os.getenv("TEMPLATES_DIR", "templates")
OUTPUT_DIR = os.getenv("OUTPUT_DIR", "data/output")
LIBREOFFICE_PATH = os.getenv("LIBREOFFICE_PATH", "libreoffice")

# Автоочистка: файлы старше N часов удаляются из OUTPUT_DIR
# (документы после отправки живут в Telegram — локальные копии не нужны)
CLEANUP_MAX_AGE_HOURS = int(os.getenv("CLEANUP_MAX_AGE_HOURS", "24"))
CLEANUP_INTERVAL_MIN  = int(os.getenv("CLEANUP_INTERVAL_MIN", "60"))

# Сколько конвертаций DOCX→PDF может идти одновременно
PDF_CONCURRENCY = int(os.getenv("PDF_CONCURRENCY", "1"))

# Template filenames (лежат в templates/, загружаются через меню бота)
# Названия — со стороны клиента:
TEMPLATE_BUY  = "template_buy.docx"          # Клиент покупает ВА у нас (мы продаём)
TEMPLATE_SELL = "template_sell.docx"         # Клиент продаёт ВА нам (мы покупаем)
TEMPLATE_INVOICE_BUY = "template_invoice_buy.docx"  # Счёт-заявка на покупку клиентом

# Единственный источник соответствия «тип операции в боте → файл шаблона».
# act_type 'sell' = мы продаём = клиент покупает → template_buy (и наоборот).
ACT_TEMPLATE_FILES = {
    "sell":    TEMPLATE_BUY,
    "buy":     TEMPLATE_SELL,
    "invoice": TEMPLATE_INVOICE_BUY,
}
