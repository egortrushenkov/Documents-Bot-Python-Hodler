"""
Генерация DOCX-документов из шаблонов.
Шаблоны должны быть подготовлены скриптом prepare_templates.py.
"""
import os
import shutil
from datetime import datetime
from docx import Document
from config import TEMPLATES_DIR, OUTPUT_DIR, TEMPLATE_SELL, TEMPLATE_BUY

MONTHS_RU = {
    1: "января", 2: "февраля",  3: "марта",    4: "апреля",
    5: "мая",    6: "июня",     7: "июля",      8: "августа",
    9: "сентября", 10: "октября", 11: "ноября", 12: "декабря",
}


def format_date_full(date_str: str) -> str:
    """'12.04.2026' → '«12» апреля 2026 г.'"""
    try:
        d = datetime.strptime(date_str, "%d.%m.%Y")
        return f"«{d.day}» {MONTHS_RU[d.month]} {d.year} г."
    except Exception:
        return date_str


def format_number(value: str) -> str:
    """Форматирует число с пробелом-разделителем тысяч: '500000' → '500 000'"""
    try:
        n = float(value.replace(",", ".").replace(" ", ""))
        if n == int(n):
            return f"{int(n):,}".replace(",", " ")
        return f"{n:,.3f}".replace(",", " ").rstrip("0").rstrip(".")
    except Exception:
        return value


def _merge_and_replace(doc: Document, replacements: dict):
    """
    Объединяет раны в каждом параграфе и применяет замены плейсхолдеров.
    """
    def process_para(para):
        if not para.runs:
            return
        full_text = "".join(r.text for r in para.runs)
        if not any(k in full_text for k in replacements):
            return
        for k, v in replacements.items():
            full_text = full_text.replace(k, str(v) if v is not None else "")
        para.runs[0].text = full_text
        for run in para.runs[1:]:
            run.text = ""

    for para in doc.paragraphs:
        process_para(para)
    for table in doc.tables:
        for row in table.rows:
            for cell in row.cells:
                for para in cell.paragraphs:
                    process_para(para)


def build_replacements(deal: dict, op: dict, client: dict) -> dict:
    """
    Формирует словарь всех плейсхолдеров для подстановки.
    deal    — данные сделки
    op      — настройки оператора (из БД)
    client  — данные компании-клиента (из БД)
    """
    deal_date      = deal.get("deal_date", "")
    deal_date_full = format_date_full(deal_date)
    exec_date      = deal.get("execution_date", deal_date)

    va_amount_raw  = deal.get("va_amount", "0")
    fiat_amount_raw= deal.get("fiat_amount", "0")
    va_amount_fmt  = format_number(va_amount_raw)
    fiat_amount_fmt= format_number(fiat_amount_raw)

    va_type    = deal.get("va_type", "USDT")
    network    = deal.get("network", "TRC-20")
    va_ticker  = f"{va_type}_{network.replace('-','')}"  # USDT_TRC20

    kvvo       = deal.get("kvvo", "99082")
    act_type   = deal.get("act_type", "sell")  # sell | buy

    if act_type == "sell":
        operation_type = "Покупка виртуальных активов"
    else:
        operation_type = "Продажа виртуальных активов"

    deal_number = str(deal.get("deal_number", ""))
    payment_purpose = (
        f"VO {kvvo} За виртуальный актив по заявке "
        f"№{deal_number} от {deal_date} согласно Соглашения "
        f"https://hodlerexchange.io/home/documents. НДС не облагается."
    )

    return {
        # ── Сделка ────────────────────────────────────────────────────────────
        "{{DEAL_NUMBER}}":       deal_number,
        "{{DEAL_DATE}}":         deal_date,
        "{{DEAL_DATE_FULL}}":    deal_date_full,
        "{{OPERATION_TYPE}}":    operation_type,
        "{{KVVO}}":              kvvo,
        "{{PAYMENT_PURPOSE}}":   payment_purpose,
        "{{VA_TYPE}}":           va_type,
        "{{NETWORK}}":           network,
        "{{VA_TICKER}}":         va_ticker,
        "{{VA_AMOUNT}}":         va_amount_fmt,
        "{{VA_AMOUNT_SHORT}}":   va_amount_fmt,
        "{{FIAT_AMOUNT}}":       fiat_amount_fmt,
        "{{FIAT_AMOUNT_SHORT}}": fiat_amount_fmt,
        "{{FIAT_CURRENCY}}":     "RUB",
        "{{EXCHANGE_RATE}}":     str(deal.get("exchange_rate", "")).replace(".", ","),
        "{{COMMISSION_FIAT}}":   str(deal.get("commission_fiat", "0%")),
        "{{COMMISSION_VA}}":     str(deal.get("commission_va", "0%")),
        "{{TX_HASH}}":           deal.get("tx_hash", ""),
        "{{EXECUTION_DATE}}":    exec_date,
        "{{CL_WALLET}}":         deal.get("client_wallet", client.get("wallet", "")),
        "{{OP_WALLET}}":         deal.get("operator_wallet", op.get("wallet", "")),

        # ── Оператор ──────────────────────────────────────────────────────────
        "{{OP_FULL_NAME}}":      op.get("full_name", ""),
        "{{OP_SHORT_NAME}}":     op.get("short_name", ""),
        "{{OP_INN}}":            op.get("inn", ""),
        "{{OP_KPP}}":            op.get("kpp", ""),
        "{{OP_ADDRESS}}":        op.get("address", ""),
        "{{OP_ADDRESS_FULL}}":   op.get("address", ""),
        "{{OP_LEGAL_ADDRESS}}":  op.get("legal_address", op.get("address", "")),
        "{{OP_LICENSE}}":        op.get("license", ""),
        "{{OP_KIO}}":            op.get("kio", ""),
        "{{OP_INN_RF}}":         op.get("inn_rf", op.get("inn", "")),
        "{{OP_KPP_RF}}":         op.get("kpp_rf", op.get("kpp", "")),
        "{{OP_BANK_NAME}}":      op.get("bank_name", ""),
        "{{OP_BANK_ACCOUNT}}":   op.get("bank_account", ""),
        "{{OP_BANK_BIK}}":       op.get("bank_bik", ""),
        "{{OP_DIRECTOR}}":       op.get("director_name", ""),
        "{{BANK_NAME}}":         op.get("bank_name", ""),
        "{{BANK_BIK}}":          op.get("bank_bik", ""),

        # ── Клиент ────────────────────────────────────────────────────────────
        "{{CL_FULL_NAME}}":      client.get("full_name", ""),
        "{{CL_SHORT_NAME}}":     client.get("short_name", ""),
        "{{CL_INN}}":            client.get("inn", ""),
        "{{CL_KPP}}":            client.get("kpp", ""),
        "{{CL_REG_NUMBER}}":     client.get("reg_number", ""),
        "{{CL_ADDRESS}}":        client.get("address", ""),
        "{{CL_KIO}}":            client.get("kio", "-"),
        "{{CL_INN_RF}}":         client.get("inn_rf", "-"),
        "{{CL_KPP_RF}}":         client.get("kpp_rf", "-"),
        "{{CL_BANK_NAME}}":      client.get("bank_name", ""),
        "{{CL_BANK_ACCOUNT}}":   client.get("bank_account", ""),
        "{{CL_BANK_BIK}}":       client.get("bank_bik", ""),
    }


def generate_docx(deal: dict, op: dict, client: dict) -> str:
    """
    Генерирует DOCX и возвращает путь к файлу.
    """
    act_type = deal.get("act_type", "sell")
    template_name = TEMPLATE_SELL if act_type == "sell" else TEMPLATE_BUY
    template_path = os.path.join(TEMPLATES_DIR, template_name)

    if not os.path.exists(template_path):
        raise FileNotFoundError(
            f"Шаблон не найден: {template_path}\n"
            "Запустите: python prepare_templates.py"
        )

    os.makedirs(OUTPUT_DIR, exist_ok=True)
    deal_number = deal.get("deal_number", "0")
    deal_date   = deal.get("deal_date", "").replace(".", "")
    direction   = "sell" if act_type == "sell" else "buy"
    out_name    = f"act_{direction}_{deal_number}_{deal_date}.docx"
    out_path    = os.path.join(OUTPUT_DIR, out_name)

    shutil.copy(template_path, out_path)
    doc          = Document(out_path)
    replacements = build_replacements(deal, op, client)
    _merge_and_replace(doc, replacements)
    doc.save(out_path)
    return out_path
