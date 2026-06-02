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

# Template filenames (создаются вручную, лежат в templates/)
# Названия — со стороны клиента:
TEMPLATE_BUY  = "template_buy.docx"          # Клиент покупает ВА у нас (мы продаём)
TEMPLATE_SELL = "template_sell.docx"         # Клиент продаёт ВА нам (мы покупаем)
TEMPLATE_INVOICE_BUY = "template_invoice_buy.docx"  # Счёт-заявка на покупку клиентом
