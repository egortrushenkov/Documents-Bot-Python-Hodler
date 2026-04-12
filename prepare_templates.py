"""
Запускается один раз для подготовки шаблонов из оригинальных DOCX.
Создаёт templates/template_sell.docx и templates/template_buy.docx
с плейсхолдерами вида {{VARIABLE}}.

Использование:
    python prepare_templates.py
"""
import shutil
import os
from docx import Document


# ─── Core replace helpers ─────────────────────────────────────────────────────

def _merge_para(para):
    """Merge all runs of a paragraph into the first run."""
    if not para.runs:
        return
    full = "".join(r.text for r in para.runs)
    full = full.replace("\u00a0", " ").replace("\xa0", " ")  # normalize non-breaking spaces
    para.runs[0].text = full
    for r in para.runs[1:]:
        r.text = ""


def _all_paras(doc):
    """Yield all paragraphs: body + every table cell."""
    for p in doc.paragraphs:
        yield p
    for table in doc.tables:
        for row in table.rows:
            for cell in row.cells:
                for p in cell.paragraphs:
                    yield p


def merge_all(doc):
    """Merge runs in all paragraphs (call once before any replacement)."""
    for p in _all_paras(doc):
        _merge_para(p)


def replace_all(doc, old: str, new: str):
    """Replace ALL occurrences of old → new throughout the document."""
    for p in _all_paras(doc):
        if old in p.runs[0].text if p.runs else False:
            p.runs[0].text = p.runs[0].text.replace(old, new)


def replace_first(doc, old: str, new: str) -> bool:
    """Replace only the FIRST occurrence. Returns True if replaced."""
    for p in _all_paras(doc):
        if p.runs and old in p.runs[0].text:
            p.runs[0].text = p.runs[0].text.replace(old, new, 1)
            return True
    return False


def replace_ordered(doc, replacements: list):
    """Apply a list of (old, new) replacements to ALL occurrences, in order."""
    for old, new in replacements:
        replace_all(doc, old, new)


# ─── Common replacements (apply to both templates) ────────────────────────────

BASE_REPLACEMENTS = [
    # ── Сделка (специфичные сначала) ──────────────────────────────────────────
    ("VO 99082 За виртуальный актив по заявке №499 от 17.02.2026",
     "VO {{KVVO}} За виртуальный актив по заявке №{{DEAL_NUMBER}} от {{DEAL_DATE}}"),
    ("Покупка виртуальных активов по заявке №499 от 17.02.2026",
     "{{OPERATION_TYPE}} по заявке №{{DEAL_NUMBER}} от {{DEAL_DATE}}"),
    ("Продажа виртуальных активов по заявке №499 от 17.02.2026",
     "{{OPERATION_TYPE}} по заявке №{{DEAL_NUMBER}} от {{DEAL_DATE}}"),
    ("Договор №499 от 17.02.2026",   "Договор №{{DEAL_NUMBER}} от {{DEAL_DATE}}"),
    ("Акт №499 от 17.02.2026",       "Акт №{{DEAL_NUMBER}} от {{DEAL_DATE}}"),
    ("Заявка №499 от 17.02.2026",    "Заявка №{{DEAL_NUMBER}} от {{DEAL_DATE}}"),
    ("по заявке №499 от 17.02.2026", "по заявке №{{DEAL_NUMBER}} от {{DEAL_DATE}}"),
    ("№499 от 17.02.2026",           "№{{DEAL_NUMBER}} от {{DEAL_DATE}}"),
    ("№499  от 17.02.2026",          "№{{DEAL_NUMBER}} от {{DEAL_DATE}}"),  # double space variant
    ("500 000  RUR",                 "{{FIAT_AMOUNT}} RUR"),                 # double space variant
    ("«17» февраля 2026 г.",         "{{DEAL_DATE_FULL}}"),
    ("17.02.2026",                   "{{DEAL_DATE}}"),

    # ── Справка: полные предложения с ИНН (до коротких замен!) ─────────────────
    # Эти строки содержат "ИНН XXXXXXXXX" (пробел, без двоеточия) — нужно заменить
    # до того как меняем короткие «ИНН:XXXX»
    ('Общество с ограниченной ответственностью "СТЕЙБЛЕКС", ИНН 9909730748, Местонахождение: Кыргызская Республика, город Бишкек, Свердловский район, улица Московская, дом 197',
     '{{OP_FULL_NAME}}, ИНН {{OP_INN}}, Местонахождение: {{OP_ADDRESS_FULL}}'),
    ('Общество с ограниченной ответственностью "Алтынкопрю", ИНН 9909745705, Местонахождение: Кыргызстан, г. Бишкек, ул. Целинная 47',
     '{{CL_FULL_NAME}}, ИНН {{CL_INN}}, Местонахождение: {{CL_ADDRESS}}'),

    # ── Оператор ──────────────────────────────────────────────────────────────
    ("Общество с ограниченной ответственностью «СТЕЙБЛЕКС»", "{{OP_FULL_NAME}}"),
    ('Общество с ограниченной ответственностью "СТЕЙБЛЕКС"', "{{OP_FULL_NAME}}"),
    ("ОсОО «СТЕЙБЛЕКС»",    "{{OP_SHORT_NAME}}"),
    ("ОсОО «Стейблекс»",    "{{OP_SHORT_NAME}}"),
    ("ОсОО «Стеблекс»",     "{{OP_SHORT_NAME}}"),
    ("ИНН:9909730748",       "ИНН:{{OP_INN}}"),
    ("ИНН: 9909730748",      "ИНН: {{OP_INN}}"),
    ("ИНН 9909730748",       "ИНН {{OP_INN}}"),
    ("КПП:770387001",        "КПП:{{OP_KPP}}"),
    ("КПП: 770387001",       "КПП: {{OP_KPP}}"),
    ("693010, г.Южно-Сахалинск, ул Комсомольская, 145", "{{OP_LEGAL_ADDRESS}}"),
    ("720009, г. Бишкек, ул. Московская, д. 197",       "{{OP_ADDRESS}}"),
    ("Кыргызская Республика, город Бишкек, Свердловский район, улица Московская, дом 197",
     "{{OP_ADDRESS_FULL}}"),
    # Лицензия в тексте акта
    ("ИНН: 02504202410133",  "ИНН: {{OP_INN}}"),
    ("02504202410133",       "{{OP_INN}}"),
    ("Лицензия: 150 от 28-03-2025", "Лицензия: {{OP_LICENSE}}"),
    # Директор
    ("Зенков И.В.",          "{{OP_DIRECTOR}}"),

    # ── Клиент ────────────────────────────────────────────────────────────────
    ("Общество с ограниченной ответственностью «Алтынкопрю»", "{{CL_FULL_NAME}}"),
    ('Общество с ограниченной ответственностью "Алтынкопрю"', "{{CL_FULL_NAME}}"),
    ("ОсОО «Алтынкопрю»",   "{{CL_SHORT_NAME}}"),
    ("ИНН: 9909745705",      "ИНН: {{CL_INN}}"),
    ("ИНН:9909745705",       "ИНН:{{CL_INN}}"),
    ("ИНН 9909745705",       "ИНН {{CL_INN}}"),
    ("КПП: 770887001",       "КПП: {{CL_KPP}}"),
    ("КПП:770887001",        "КПП:{{CL_KPP}}"),
    ("317744-3301-ООО",      "{{CL_REG_NUMBER}}"),
    ("Кыргызстан, г. Бишкек, ул. Целинная 47", "{{CL_ADDRESS}}"),

    # ── Оператор: расчётный счёт (делаем ДО банка, т.к. банк у обоих одинаковый) ──
    ("40807810500014264602", "{{OP_BANK_ACCOUNT}}"),
    # ── Клиент: расчётный счёт ────────────────────────────────────────────────
    ("40807810600014672000", "{{CL_BANK_ACCOUNT}}"),

    # ── Кошельки ──────────────────────────────────────────────────────────────
    ("TXFEYN4C5BnesaxUXJiXJHGS7K12QutZ3r", "{{OP_WALLET}}"),
    ("THtSiaKaPF1R1dhZpBAcmgkvchnDmoA9Pi", "{{CL_WALLET}}"),

    # ── КВВО ──────────────────────────────────────────────────────────────────
    ("99082", "{{KVVO}}"),
    ("73074", "{{CL_KIO_PLACEHOLDER}}"),

    # ── Суммы ─────────────────────────────────────────────────────────────────
    ("68 883.559500 RUB",  "{{FIAT_AMOUNT_SHORT}} RUB"),
    ("500 USDT",           "{{VA_AMOUNT_SHORT}} {{VA_TYPE}}"),
    ("6 287.726 USDT",     "{{VA_AMOUNT}} {{VA_TYPE}}"),
    ("6287.726",           "{{VA_AMOUNT}}"),
    ("500 000 RUR",        "{{FIAT_AMOUNT}} RUR"),
    ("500 000 RUB",        "{{FIAT_AMOUNT}} RUB"),
    ("500 000",            "{{FIAT_AMOUNT}}"),
    ("500000",             "{{FIAT_AMOUNT}}"),
    ("79,52",              "{{EXCHANGE_RATE}}"),
    ("USDT_TRC20",         "{{VA_TICKER}}"),
    ("TRC-20",             "{{NETWORK}}"),
    ("d30791fb7a5a460fce1ff756e0467aff26efea899c265d51abcb247af50f31e6", "{{TX_HASH}}"),
]

# ─── Per-template extras ──────────────────────────────────────────────────────

SELL_EXTRA = [
    ("{{CL_KIO_PLACEHOLDER}}", "-"),
]

BUY_EXTRA = [
    ("{{CL_KIO_PLACEHOLDER}}", "{{OP_KIO}}"),
]


# ─── Bank placeholder: positional (first=client, second=operator) ─────────────

def apply_bank_placeholders(doc, sell: bool):
    """
    Банк и БИК одинаковы у обоих участников в шаблоне.
    В таблице заявки:
      - «Банковские реквизиты Клиента» идёт ПЕРВОЙ строкой  → CL_BANK_*
      - «Банковские реквизиты Оператора» идёт ВТОРОЙ строкой → OP_BANK_*
    Используем replace_first дважды подряд.
    """
    # Банк (КБ "Долинск" или любой другой — заменяем первое вхождение)
    bank_str = 'КБ "Долинск" (АО)'
    replace_first(doc, bank_str, "{{CL_BANK_NAME}}")
    replace_first(doc, bank_str, "{{OP_BANK_NAME}}")

    # БИК
    bik_str = "046401727"
    replace_first(doc, bik_str, "{{CL_BANK_BIK}}")
    replace_first(doc, bik_str, "{{OP_BANK_BIK}}")


# ─── Main ─────────────────────────────────────────────────────────────────────

def prepare(src: str, dst: str, extra: list, sell: bool):
    print(f"  {os.path.basename(src)} → {os.path.basename(dst)}")
    shutil.copy(src, dst)
    doc = Document(dst)

    # 1. Merge runs first
    merge_all(doc)

    # 2. Apply base replacements
    replace_ordered(doc, BASE_REPLACEMENTS + extra)

    # 3. Positional bank replacement (first=CL, second=OP)
    apply_bank_placeholders(doc, sell=sell)

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
        sell=True,
    )

    print("Подготовка шаблона BUY (покупаем ВА у клиента)...")
    prepare(
        src=os.path.join(tpl, "Акт_продаже_нам_от_ЮЛ.docx"),
        dst=os.path.join(tpl, "template_buy.docx"),
        extra=BUY_EXTRA,
        sell=False,
    )

    print("\n✅ Шаблоны готовы в папке templates/")
