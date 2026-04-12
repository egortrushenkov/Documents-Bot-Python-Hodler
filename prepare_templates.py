"""
Запускается один раз для подготовки шаблонов из оригинальных DOCX.
Создаёт templates/template_sell.docx и templates/template_buy.docx
с плейсхолдерами вида {{VARIABLE}}.

Использование:
    python prepare_templates.py
"""
import copy
import shutil
import os
from docx import Document
from docx.oxml.ns import qn


# ─── Helpers ──────────────────────────────────────────────────────────────────

def merge_and_replace(doc: Document, replacements: list):
    """
    Проходит по всем параграфам (включая ячейки таблиц),
    объединяет раны в один, применяет замены.
    """
    def process_paragraph(para):
        if not para.runs:
            return
        full_text = "".join(r.text for r in para.runs)
        if not any(old in full_text for old, _ in replacements):
            return
        # Применяем замены в порядке списка (специфичные → общие)
        for old, new in replacements:
            full_text = full_text.replace(old, new)
        # Пишем весь текст в первый ран, очищаем остальные
        para.runs[0].text = full_text
        for run in para.runs[1:]:
            run.text = ""

    for para in doc.paragraphs:
        process_paragraph(para)

    for table in doc.tables:
        for row in table.rows:
            for cell in row.cells:
                for para in cell.paragraphs:
                    process_paragraph(para)


# ─── Replacements ─────────────────────────────────────────────────────────────
# Порядок важен: более длинные/специфичные строки — раньше

BASE_REPLACEMENTS = [
    # ── Сделка (специфичные сначала) ──────────────────────────────────────────
    ("VO 99082 За виртуальный актив по заявке №499 от 17.02.2026",
     "VO {{KVVO}} За виртуальный актив по заявке №{{DEAL_NUMBER}} от {{DEAL_DATE}}"),
    ("Покупка виртуальных активов по заявке №499 от 17.02.2026",
     "{{OPERATION_TYPE}} по заявке №{{DEAL_NUMBER}} от {{DEAL_DATE}}"),
    ("Продажа виртуальных активов по заявке №499 от 17.02.2026",
     "{{OPERATION_TYPE}} по заявке №{{DEAL_NUMBER}} от {{DEAL_DATE}}"),
    ("Договор №499 от 17.02.2026",    "Договор №{{DEAL_NUMBER}} от {{DEAL_DATE}}"),
    ("Акт №499 от 17.02.2026",        "Акт №{{DEAL_NUMBER}} от {{DEAL_DATE}}"),
    ("Заявка №499 от 17.02.2026",     "Заявка №{{DEAL_NUMBER}} от {{DEAL_DATE}}"),
    ("по заявке №499 от 17.02.2026",  "по заявке №{{DEAL_NUMBER}} от {{DEAL_DATE}}"),
    ("№499 от 17.02.2026",            "№{{DEAL_NUMBER}} от {{DEAL_DATE}}"),
    ("«17» февраля 2026 г.",          "{{DEAL_DATE_FULL}}"),
    ("17.02.2026",                    "{{DEAL_DATE}}"),

    # ── Оператор ──────────────────────────────────────────────────────────────
    ("Общество с ограниченной ответственностью «СТЕЙБЛЕКС»", "{{OP_FULL_NAME}}"),
    ('Общество с ограниченной ответственностью "СТЕЙБЛЕКС"', "{{OP_FULL_NAME}}"),
    ("ОсОО «СТЕЙБЛЕКС»",              "{{OP_SHORT_NAME}}"),
    ("ОсОО «Стейблекс»",              "{{OP_SHORT_NAME}}"),
    ("ОсОО «Стеблекс»",               "{{OP_SHORT_NAME}}"),  # опечатка в оригинале
    ("ИНН:9909730748",                "ИНН:{{OP_INN}}"),
    ("ИНН: 9909730748",               "ИНН: {{OP_INN}}"),
    ("КПП:770387001",                 "КПП:{{OP_KPP}}"),
    ("КПП: 770387001",                "КПП: {{OP_KPP}}"),
    ("693010, г.Южно-Сахалинск, ул Комсомольская, 145", "{{OP_LEGAL_ADDRESS}}"),
    ("720009, г. Бишкек, ул. Московская, д. 197",       "{{OP_ADDRESS}}"),
    ("Кыргызская Республика, город Бишкек, Свердловский район, улица Московская, дом 197",
     "{{OP_ADDRESS_FULL}}"),
    # Расчётный счет оператора
    ("40807810500014264602",          "{{OP_BANK_ACCOUNT}}"),
    # Директор
    ("Зенков И.В.",                   "{{OP_DIRECTOR}}"),
    # Лицензия
    ("02504202410133",                "{{OP_INN}}"),   # встречается в тексте акта
    ("Лицензия: 150 от 28-03-2025",   "Лицензия: {{OP_LICENSE}}"),
    ("https://hodlerexchange.io/",    "https://hodlerexchange.io/"),  # не меняем

    # ── Клиент ────────────────────────────────────────────────────────────────
    ("Общество с ограниченной ответственностью «Алтынкопрю»", "{{CL_FULL_NAME}}"),
    ('Общество с ограниченной ответственностью "Алтынкопрю"', "{{CL_FULL_NAME}}"),
    ("ОсОО «Алтынкопрю»",            "{{CL_SHORT_NAME}}"),
    ("ИНН: 9909745705",               "ИНН: {{CL_INN}}"),
    ("ИНН:9909745705",                "ИНН:{{CL_INN}}"),
    ("КПП: 770887001",                "КПП: {{CL_KPP}}"),
    ("КПП:770887001",                 "КПП:{{CL_KPP}}"),
    ("317744-3301-ООО",               "{{CL_REG_NUMBER}}"),
    ("Кыргызстан, г. Бишкек, ул. Целинная 47", "{{CL_ADDRESS}}"),
    # Расчётный счет клиента
    ("40807810600014672000",          "{{CL_BANK_ACCOUNT}}"),
    # Кошелёк клиента
    ("THtSiaKaPF1R1dhZpBAcmgkvchnDmoA9Pi", "{{CL_WALLET}}"),

    # ── Кошелёк оператора ─────────────────────────────────────────────────────
    ("TXFEYN4C5BnesaxUXJiXJHGS7K12QutZ3r", "{{OP_WALLET}}"),

    # ── Банк (общий для обоих — одинаковый банк) ──────────────────────────────
    ('КБ "Долинск" (АО)',             "{{BANK_NAME}}"),

    # ── БИК (один для обоих) ──────────────────────────────────────────────────
    ("046401727",                     "{{BANK_BIK}}"),

    # ── КВВО ──────────────────────────────────────────────────────────────────
    ("99082",                         "{{KVVO}}"),
    ("73074",                         "{{CL_KIO_SELL}}"),   # placeholder; в sell шаблоне "-"

    # ── Суммы (в договоре — числа из тестовых данных) ─────────────────────────
    ("68 883.559500 RUB",             "{{FIAT_AMOUNT_SHORT}} RUB"),
    ("500 USDT",                      "{{VA_AMOUNT_SHORT}} {{VA_TYPE}}"),
    # В акте / заявке:
    ("6 287.726 USDT",                "{{VA_AMOUNT}} {{VA_TYPE}}"),
    ("6287.726",                      "{{VA_AMOUNT}}"),
    ("500 000 RUR",                   "{{FIAT_AMOUNT}} RUR"),
    ("500 000 RUB",                   "{{FIAT_AMOUNT}} RUB"),
    ("500 000",                       "{{FIAT_AMOUNT}}"),
    ("500000",                        "{{FIAT_AMOUNT}}"),
    # Курс
    ("79,52",                         "{{EXCHANGE_RATE}}"),
    ("79.52",                         "{{EXCHANGE_RATE}}"),
    # Валюта
    ("USDT_TRC20",                    "{{VA_TICKER}}"),
    ("TRC-20",                        "{{NETWORK}}"),
    # Хэш
    ("d30791fb7a5a460fce1ff756e0467aff26efea899c265d51abcb247af50f31e6", "{{TX_HASH}}"),
    # Комиссии
    # "0%" — не заменяем чтобы не сломать текст публичной оферты; заменим осторожнее:
]

# В sell-шаблоне КИО = "-", в buy-шаблоне = {{OP_KIO}}
SELL_EXTRA = [
    ("{{CL_KIO_SELL}}", "-"),   # в продаже у нас КИО прочерк
]

BUY_EXTRA = [
    ("{{CL_KIO_SELL}}", "{{OP_KIO}}"),  # в покупке нам = наш КИО
]


# ─── Main ─────────────────────────────────────────────────────────────────────

def prepare(src: str, dst: str, extra: list):
    print(f"  {src} → {dst}")
    shutil.copy(src, dst)
    doc = Document(dst)
    merge_and_replace(doc, BASE_REPLACEMENTS + extra)
    doc.save(dst)
    print(f"  ✓ сохранён: {dst}")


if __name__ == "__main__":
    base = os.path.dirname(os.path.abspath(__file__))
    tpl  = os.path.join(base, "templates")

    print("Подготовка шаблона SELL (продаём ВА клиенту)...")
    prepare(
        src=os.path.join(tpl, "Акт_покупке_у_нас_ЮЛ.docx"),
        dst=os.path.join(tpl, "template_sell.docx"),
        extra=SELL_EXTRA,
    )

    print("Подготовка шаблона BUY (покупаем ВА у клиента)...")
    prepare(
        src=os.path.join(tpl, "Акт_продаже_нам_от_ЮЛ.docx"),
        dst=os.path.join(tpl, "template_buy.docx"),
        extra=BUY_EXTRA,
    )

    print("\n✅ Шаблоны готовы в папке templates/")
