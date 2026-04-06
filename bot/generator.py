"""
Генерация DOCX и PDF из шаблона.
"""

import os
import re
import shutil
import zipfile
import subprocess
import unicodedata
from datetime import datetime

TEMPLATE_PATH = os.path.join(os.path.dirname(__file__), "..", "data", "TEMPLATE_hodler_deal.docx")
OUTPUT_DIR    = os.path.join(os.path.dirname(__file__), "..", "data", "output")

DRAGON_BLOSSOM = {
    "name":    "DRAGON BLOSSOM LIMITED/龍花有限公司",
    "address": "КВАРТИРА/КОМНАТА 1618B, 16/F, PIONEER CENTRE, 750 NATHAN ROAD, МОНГ КОК, КОУЛУН, ГОНКОНГ/香港古隆旺角彌敦道750號創始中心16樓1618B室",
    "reg":     "Регистрационный номер/註冊號碼: 78056712",
}

MONTH_RU = ["января","февраля","марта","апреля","мая","июня",
            "июля","августа","сентября","октября","ноября","декабря"]


def build_placeholders(data):
    deal_type = data.get("deal_type", "buy").lower()
    client    = data["client"]

    if deal_type == "buy":
        va_seller_name        = DRAGON_BLOSSOM["name"]
        va_seller_address     = "Местонахождение: " + DRAGON_BLOSSOM["address"]
        va_seller_reg         = DRAGON_BLOSSOM["reg"]
        deal_direction_title  = "на продажу клиенту виртуальных активов"
        operation_description = (
            "Продажа виртуальных активов с перечислением средств клиенту "
            "полученным от его контрагента в счет оплаты договора"
        )
    else:
        va_seller_name        = client["name"]
        va_seller_address     = "Местонахождение: " + client["address"]
        va_seller_reg         = f"ОГРН {client['ogrn']} ИНН {client['inn']} КПП {client['kpp']}"
        deal_direction_title  = "на покупку у клиента виртуальных активов"
        operation_description = (
            "Покупка виртуальных активов у клиента с перечислением средств оператору"
        )

    raw_date = data.get("contract_date", datetime.today().strftime("%d.%m.%Y"))
    try:
        dt = datetime.strptime(raw_date, "%d.%m.%Y")
        date_dashes = dt.strftime("%d-%m-%Y")
        sign_date   = f"{dt.day}» {MONTH_RU[dt.month-1]} {dt.year} г."
    except Exception:
        date_dashes = raw_date.replace(".", "-")
        sign_date   = raw_date

    return {
        "{{contract_id}}":           data.get("contract_id", ""),
        "{{contract_date}}":         raw_date,
        "{{contract_date_dashes}}":  date_dashes,
        "{{deal_direction_title}}":  deal_direction_title,
        "{{va_seller_name}}":        va_seller_name,
        "{{va_seller_address}}":     va_seller_address,
        "{{va_seller_reg}}":         va_seller_reg,
        "{{client_name}}":           client["name"],
        "{{client_address}}":        client["address"],
        "{{client_ogrn}}":           client["ogrn"],
        "{{client_inn}}":            client["inn"],
        "{{client_kpp}}":            client["kpp"],
        "{{client_bank_account}}":   client["bank_account"],
        "{{client_bank_name}}":      client["bank_name"],
        "{{client_bank_ks}}":        client["bank_ks"],
        "{{client_bank_bik}}":       client["bank_bik"],
        "{{client_wallet}}":         data.get("client_wallet", ""),
        "{{operator_wallet}}":       data.get("operator_wallet", ""),
        "{{usdt_amount}}":           data.get("usdt_amount", ""),
        "{{rub_amount}}":            data.get("rub_amount", ""),
        "{{exchange_rate}}":         data.get("exchange_rate", ""),
        "{{kvvo}}":                  data.get("kvvo", "VO20200"),
        "{{license_contract}}":      data.get("license_contract", ""),
        "{{payment_purpose}}":       data.get("payment_purpose", ""),
        "{{act_operation_type}}":    data.get("act_operation_type", "Покупка виртуальных активов у клиента"),
        "{{tx_hash}}":               data.get("tx_hash", "-"),
        "{{execution_date}}":        data.get("execution_date", raw_date),
        "{{sign_date}}":             sign_date,
        "{{operation_description}}": operation_description,
    }


def _make_filename(data):
    cid  = data.get("contract_id", "deal").replace("/", "-").replace(" ", "_")
    date = data.get("contract_date", "").replace(".", "_")
    usdt = data.get("usdt_amount", "").replace(" ", "_")
    return f"Закрывающие_документы_{cid}_{date}_{usdt}_USDT"


def generate_docx(data):
    """Возвращает путь к готовому DOCX."""
    os.makedirs(OUTPUT_DIR, exist_ok=True)
    placeholders = build_placeholders(data)
    base     = _make_filename(data)
    out_path = os.path.join(OUTPUT_DIR, base + ".docx")
    tmp_path = out_path + ".tmp.docx"

    shutil.copy2(TEMPLATE_PATH, tmp_path)

    with zipfile.ZipFile(tmp_path, "r") as z:
        xml = z.read("word/document.xml").decode("utf-8")

    xml = unicodedata.normalize("NFC", xml)
    for ph, val in placeholders.items():
        xml = xml.replace(ph, str(val))

    with zipfile.ZipFile(tmp_path, "r") as zin:
        with zipfile.ZipFile(out_path, "w", zipfile.ZIP_DEFLATED) as zout:
            for item in zin.infolist():
                content = xml.encode("utf-8") if item.filename == "word/document.xml" else zin.read(item.filename)
                zout.writestr(item, content)

    os.remove(tmp_path)
    return out_path


def generate_pdf(docx_path):
    """Конвертирует DOCX в PDF через LibreOffice. Возвращает путь или None."""
    soffice = _find_soffice()
    if not soffice:
        return None
    out_dir = os.path.dirname(os.path.abspath(docx_path))
    cmd = [soffice, "--headless", "--convert-to", "pdf", "--outdir", out_dir, os.path.abspath(docx_path)]
    try:
        subprocess.run(cmd, capture_output=True, timeout=60, check=False)
        pdf_path = docx_path.replace(".docx", ".pdf")
        return pdf_path if os.path.exists(pdf_path) else None
    except Exception:
        return None


def _find_soffice():
    for path in ["/usr/bin/libreoffice", "/usr/bin/soffice", "/usr/lib/libreoffice/program/soffice"]:
        if os.path.exists(path):
            return path
    return shutil.which("libreoffice") or shutil.which("soffice")
