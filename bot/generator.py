"""
Генерация DOCX и PDF из единого шаблона.

Два типа документа:
  contract — на основании договора
  offer    — по публичной оферте

Два направления сделки:
  buy  — мы продаём клиенту USDT
  sell — клиент продаёт нам USDT

Автоподсчёт: если переданы любые два из {usdt_amount, rub_amount, exchange_rate},
третье вычисляется автоматически.
"""

import os, re, shutil, zipfile, subprocess, unicodedata
from decimal import Decimal, ROUND_HALF_UP, InvalidOperation
from datetime import datetime

TEMPLATE_PATH = os.path.join(os.path.dirname(__file__), "..", "data", "TEMPLATE_hodler_deal.docx")
OUTPUT_DIR    = os.path.join(os.path.dirname(__file__), "..", "data", "output")

MONTH_RU = ["января","февраля","марта","апреля","мая","июня",
            "июля","августа","сентября","октября","ноября","декабря"]

STABLEX_SELLER = {
    "name":    "STABLEX LTD / Общество с ограниченной ответственностью «СТЕЙБЛЕКС» (ОсОО «СТЕЙБЛЕКС»)",
    "address": "Местонахождение: Кыргызская Республика, город Бишкек, улица Московская, дом 197",
    "reg":     "ИНН: 9909730748 КПП: 770387001\nНомер счета: 408 07 810 9 0001 4264600\nНазвание банка: КБ «Долинск» (АО)\nБИК: 046401727\nКорр. Счет: 30101810300000000727",
}

DRAGON_BLOSSOM_SELLER = {
    "name":    "DRAGON BLOSSOM LIMITED/龍花有限公司",
    "address": "Местонахождение: КВАРТИРА/КОМНАТА 1618B, 16/F, PIONEER CENTRE, 750 NATHAN ROAD, МОНГ КОК, КОУЛУН, ГОНКОНГ/香港古隆旺角彌敦道750號創始中心16樓1618B室",
    "reg":     "Регистрационный номер/註冊號碼: 78056712",
}

# ─── Автоподсчёт сумм ────────────────────────────────────────────────────────

def _parse_num(s: str) -> Decimal | None:
    """Парсит число из строки вида '14 000' или '80,88'."""
    if not s:
        return None
    try:
        cleaned = s.strip().replace(' ', '').replace('\u202f', '').replace(',', '.')
        return Decimal(cleaned)
    except InvalidOperation:
        return None

def _fmt_num(d: Decimal, decimals: int = 2) -> str:
    """Форматирует число: 14000 → '14 000', 1350000.00 → '1 350 000'."""
    quantize_str = '1.' + '0' * decimals if decimals > 0 else '1'
    rounded = d.quantize(Decimal(quantize_str), rounding=ROUND_HALF_UP)
    # Разбиваем на целую и дробную части
    parts = str(rounded).split('.')
    integer = parts[0]
    frac    = parts[1] if len(parts) > 1 else ''
    # Тысячные разделители
    result = ''
    for i, ch in enumerate(reversed(integer)):
        if i > 0 and i % 3 == 0:
            result = ' ' + result
        result = ch + result
    if frac and any(c != '0' for c in frac):
        result += ',' + frac
    return result

def auto_calculate(data: dict) -> dict:
    """Вычисляет недостающее из трёх значений: usdt, rub, rate."""
    usdt = _parse_num(data.get('usdt_amount', ''))
    rub  = _parse_num(data.get('rub_amount', ''))
    rate = _parse_num(data.get('exchange_rate', ''))

    filled = sum(x is not None for x in [usdt, rub, rate])
    if filled < 2:
        return data  # недостаточно данных

    result = dict(data)
    if usdt is None and rub and rate:
        usdt = (rub / rate).quantize(Decimal('0.001'), rounding=ROUND_HALF_UP)
        result['usdt_amount'] = _fmt_num(usdt, 3)
    elif rub is None and usdt and rate:
        rub = (usdt * rate).quantize(Decimal('0.01'), rounding=ROUND_HALF_UP)
        result['rub_amount'] = _fmt_num(rub, 0)
    elif rate is None and usdt and rub:
        rate = (rub / usdt).quantize(Decimal('0.01'), rounding=ROUND_HALF_UP)
        result['exchange_rate'] = _fmt_num(rate, 2)

    return result

# ─── Имя клиента ──────────────────────────────────────────────────────────────

def client_display_name(client: dict) -> str:
    if client.get("name_eng"):
        return f"{client['name_eng']} / {client['name_ru']}"
    return client["name_ru"]

# ─── Построение плейсхолдеров ─────────────────────────────────────────────────

def build_placeholders(data: dict) -> dict:
    data      = auto_calculate(data)
    doc_type  = data.get("doc_type", "contract")
    deal_type = data.get("deal_type", "buy")
    client    = data["client"]

    if deal_type == "buy":
        deal_direction_title = "на продажу клиенту виртуальных активов"
        va_seller = DRAGON_BLOSSOM_SELLER if doc_type == "contract" else STABLEX_SELLER
        operation_description = data.get("operation_description",
            f"Покупка виртуальных активов по заявке {data.get('contract_id','')} "
            f"от {data.get('contract_date','')} согласно Соглашения "
            f"https://hodlerexchange.io/home/documents"
        )
    else:
        deal_direction_title = "на покупку у клиента виртуальных активов"
        va_seller = {
            "name":    client_display_name(client),
            "address": f"Местонахождение: {client['address']}",
            "reg":     f"ИНН: {client['inn']}" +
                       (f" КПП: {client['kpp']}" if client.get("kpp") else "") +
                       (f"\nРег. номер: {client['reg_number']}" if client.get("reg_number") else ""),
        }
        operation_description = data.get("operation_description",
            f"Продажа виртуальных активов клиентом по заявке {data.get('contract_id','')} "
            f"от {data.get('contract_date','')} согласно Соглашения "
            f"https://hodlerexchange.io/home/documents"
        )

    # Даты
    raw_date = data.get("contract_date", datetime.today().strftime("%d.%m.%Y"))
    try:
        dt = datetime.strptime(raw_date, "%d.%m.%Y")
        date_dashes  = dt.strftime("%d-%m-%Y")
        date_act     = dt.strftime("%d/%m/%Y")   # для заголовка акта
        sign_date    = f"{dt.day}» {MONTH_RU[dt.month-1]} {dt.year} г."
    except Exception:
        date_dashes  = raw_date.replace(".", "-")
        date_act     = raw_date.replace(".", "/")
        sign_date    = raw_date

    kvvo = data.get("kvvo", "VO20200")
    if doc_type == "offer":
        payment_purpose = data.get("payment_purpose",
            f"За виртуальный актив по заявке {data.get('contract_id','')} "
            f"от {raw_date} согласно Соглашения https://hodlerexchange.io/home/documents. НДС не облагается"
        )
    else:
        payment_purpose = data.get("payment_purpose", "")

    return {
        "{{contract_id}}":           data.get("contract_id", ""),
        "{{contract_date}}":         raw_date,
        "{{contract_date_dashes}}":  date_dashes,
        "{{contract_date_act}}":     date_act,
        "{{deal_direction_title}}":  deal_direction_title,

        "{{va_seller_name}}":        va_seller["name"],
        "{{va_seller_address}}":     va_seller["address"],
        "{{va_seller_reg}}":         va_seller["reg"],

        "{{client_display_name}}":   client_display_name(client),
        "{{client_address}}":        client["address"],
        "{{client_inn}}":            client.get("inn", ""),
        "{{client_kpp}}":            client.get("kpp", ""),
        "{{client_reg_number}}":     client.get("reg_number", ""),
        "{{client_bank_account}}":   client.get("bank_account", ""),
        "{{client_bank_name}}":      client.get("bank_name", ""),
        "{{client_bank_ks}}":        client.get("bank_ks", ""),
        "{{client_bank_bik}}":       client.get("bank_bik", ""),

        "{{client_wallet}}":         data.get("client_wallet", ""),
        "{{operator_wallet}}":       data.get("operator_wallet", ""),

        "{{usdt_amount}}":           data.get("usdt_amount", ""),
        "{{rub_amount}}":            data.get("rub_amount", ""),
        "{{exchange_rate}}":         data.get("exchange_rate", ""),

        "{{kvvo}}":                  kvvo,
        "{{license_contract}}":      data.get("license_contract", ""),
        "{{payment_purpose}}":       payment_purpose,

        "{{act_operation_type}}":    data.get("act_operation_type", "Покупка виртуальных активов у клиента"),
        "{{tx_hash}}":               data.get("tx_hash", "-"),
        "{{execution_date}}":        data.get("execution_date", raw_date),
        "{{sign_date}}":             sign_date,
        "{{operation_description}}": operation_description,
    }

# ─── Генерация файлов ─────────────────────────────────────────────────────────

def _make_filename(data: dict) -> str:
    cid  = data.get("contract_id", "deal").replace("/", "-").replace(" ", "_")
    date = data.get("contract_date", "").replace(".", "_")
    usdt = data.get("usdt_amount", "").replace(" ", "_")
    return f"Закрывающие_документы_{cid}_{date}_{usdt}_USDT"


def generate_docx(data: dict) -> str:
    data = auto_calculate(data)
    os.makedirs(OUTPUT_DIR, exist_ok=True)
    placeholders = build_placeholders(data)
    out_path = os.path.join(OUTPUT_DIR, _make_filename(data) + ".docx")
    tmp_path = out_path + ".tmp.docx"

    shutil.copy2(TEMPLATE_PATH, tmp_path)
    with zipfile.ZipFile(tmp_path, "r") as z:
        xml = z.read("word/document.xml").decode("utf-8")

    xml = unicodedata.normalize("NFC", xml)
    for ph, val in placeholders.items():
        xml = xml.replace(ph, str(val))

    leftover = re.findall(r"\{\{[^}]+\}\}", xml)
    if leftover:
        print(f"⚠ Незаполненные метки: {set(leftover)}")

    with zipfile.ZipFile(tmp_path, "r") as zin:
        with zipfile.ZipFile(out_path, "w", zipfile.ZIP_DEFLATED) as zout:
            for item in zin.infolist():
                body = xml.encode("utf-8") if item.filename == "word/document.xml" else zin.read(item.filename)
                zout.writestr(item, body)

    os.remove(tmp_path)
    return out_path


def generate_pdf(docx_path: str) -> str | None:
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


def _find_soffice() -> str | None:
    for path in ["/usr/bin/libreoffice", "/usr/bin/soffice", "/usr/lib/libreoffice/program/soffice"]:
        if os.path.exists(path):
            return path
    return shutil.which("libreoffice") or shutil.which("soffice")
