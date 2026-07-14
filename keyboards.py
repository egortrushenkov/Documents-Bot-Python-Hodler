from aiogram.types import (
    InlineKeyboardMarkup, InlineKeyboardButton,
    ReplyKeyboardMarkup, KeyboardButton, ReplyKeyboardRemove
)
from aiogram.utils.keyboard import InlineKeyboardBuilder, ReplyKeyboardBuilder
from typing import List, Dict, Any

remove_kb = ReplyKeyboardRemove()


# ─── Persistent navigation reply keyboard (always visible at bottom) ──────────

def nav_kb() -> ReplyKeyboardMarkup:
    return ReplyKeyboardMarkup(
        keyboard=[
            [
                KeyboardButton(text="📋 Создать акт"),
                KeyboardButton(text="🗂 Сделки"),
            ],
            [
                KeyboardButton(text="📊 Отчёт ФН"),
                KeyboardButton(text="🏢 Компании"),
            ],
            [
                KeyboardButton(text="⚙️ Реквизиты"),
                KeyboardButton(text="🏠 Меню"),
            ],
        ],
        resize_keyboard=True,
        persistent=True,
    )


# ─── Main Menu (inline) ───────────────────────────────────────────────────────

def main_menu_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(
        InlineKeyboardButton(text="📋 Создать акт",      callback_data="menu:create_act"),
        InlineKeyboardButton(text="🏢 Компании",          callback_data="menu:companies"),
    )
    builder.row(
        InlineKeyboardButton(text="🗂 Сделки",            callback_data="deals:menu"),
        InlineKeyboardButton(text="📊 Отчёт ФН",          callback_data="report:menu"),
    )
    builder.row(
        InlineKeyboardButton(text="📄 Шаблоны",           callback_data="tpl:menu"),
        InlineKeyboardButton(text="⚙️ Реквизиты",         callback_data="menu:settings"),
    )
    return builder.as_markup()


# ─── Companies ────────────────────────────────────────────────────────────────

def companies_list_kb(companies: List[Dict[str, Any]], mode: str = "manage") -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    for c in companies:
        if mode == "select":
            builder.row(InlineKeyboardButton(
                text=f"🏢 {c['short_name']}",
                callback_data=f"sel_company:{c['id']}"
            ))
        else:
            builder.row(InlineKeyboardButton(
                text=f"🏢 {c['short_name']}",
                callback_data=f"view_company:{c['id']}"
            ))
    builder.row(InlineKeyboardButton(text="➕ Добавить компанию", callback_data="add_company"))
    builder.row(InlineKeyboardButton(text="🏠 Главное меню", callback_data="menu:main"))
    return builder.as_markup()


def company_card_kb(company_id: int) -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(
        InlineKeyboardButton(text="✏️ Редактировать", callback_data=f"edit_company:{company_id}"),
        InlineKeyboardButton(text="🗑 Удалить",        callback_data=f"del_company:{company_id}"),
    )
    builder.row(InlineKeyboardButton(text="◀️ Назад к списку", callback_data="menu:companies"))
    return builder.as_markup()


def edit_company_fields_kb(company_id: int, custom_fields: List[Dict[str, str]] | None = None) -> InlineKeyboardMarkup:
    fields = [
        ("full_name",    "Полное наименование"),
        ("short_name",   "Краткое наименование"),
        ("inn",          "ИНН"),
        ("kpp",          "КПП"),
        ("reg_number",   "Рег. номер (ОГРН/КР)"),
        ("address",      "Юр. адрес"),
        ("kio",          "КИО"),
        ("inn_rf",       "ИНН в РФ"),
        ("kpp_rf",       "КПП в РФ"),
        ("bank_name",    "Банк"),
        ("bank_account", "Р/счёт"),
        ("bank_bik",     "БИК"),
        ("wallet",       "Кошелёк (крипто)"),
        ("resident",     "Резидент/Нерезидент КР"),
    ]
    builder = InlineKeyboardBuilder()
    for fkey, flabel in fields:
        builder.row(InlineKeyboardButton(
            text=f"✏️ {flabel}",
            callback_data=f"edit_company_field:{company_id}:{fkey}"
        ))
    for f in (custom_fields or []):
        builder.row(InlineKeyboardButton(
            text=f"🧩 {f['label']}",
            callback_data=f"edit_company_field:{company_id}:c__{f['key']}"
        ))
    builder.row(InlineKeyboardButton(text="◀️ Назад", callback_data=f"view_company:{company_id}"))
    return builder.as_markup()


def confirm_delete_kb(company_id: int) -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(
        InlineKeyboardButton(text="✅ Да, удалить",  callback_data=f"confirm_del:{company_id}"),
        InlineKeyboardButton(text="❌ Отмена",        callback_data=f"view_company:{company_id}"),
    )
    return builder.as_markup()


# ─── Act creation ─────────────────────────────────────────────────────────────

def act_type_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(InlineKeyboardButton(
        text="📤 Продаём ВА клиенту (покупка у нас)",
        callback_data="act_type:sell"
    ))
    builder.row(InlineKeyboardButton(
        text="📥 Покупаем ВА у клиента (продажа нам)",
        callback_data="act_type:buy"
    ))
    builder.row(InlineKeyboardButton(
        text="🧾 Счёт-заявка на покупку клиентом",
        callback_data="act_type:invoice"
    ))
    builder.row(InlineKeyboardButton(text="❌ Отмена", callback_data="menu:main"))
    return builder.as_markup()


def split_mode_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(InlineKeyboardButton(text="📦 Одной транзакцией",          callback_data="tx:single"))
    builder.row(InlineKeyboardButton(text="✂️ Разбить (тест + остаток)",   callback_data="tx:split"))
    return builder.as_markup()


def add_tx_kb(remainder: str) -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(InlineKeyboardButton(text="➕ Добавить тест-транзакцию", callback_data="tx:add"))
    builder.row(InlineKeyboardButton(text=f"✅ Завершить (остаток {remainder})", callback_data="tx:done"))
    return builder.as_markup()


def va_type_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    for va in ["USDT", "BTC", "ETH", "USDC", "BNB", "TRX"]:
        builder.button(text=va, callback_data=f"va_type:{va}")
    builder.button(text="✏️ Другой", callback_data="va_type:custom")
    builder.adjust(3)
    return builder.as_markup()


def network_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    for net in ["TRC-20", "ERC-20", "BEP-20", "SOL", "TON", "BTC"]:
        builder.button(text=net, callback_data=f"network:{net}")
    builder.button(text="✏️ Другая", callback_data="network:custom")
    builder.adjust(3)
    return builder.as_markup()


BUILTIN_KVVO = [
    ("99082", "99082 — прочие операции с ВА"),
    ("99081", "99081 — обмен ВА"),
    ("10100", "10100 — покупка ин. валюты"),
    ("20100", "20100 — продажа ин. валюты"),
]
BUILTIN_KVVO_CODES = {c for c, _ in BUILTIN_KVVO}


def kvvo_kb(custom_codes: List[str] | None = None) -> InlineKeyboardMarkup:
    custom_codes = custom_codes or []
    builder = InlineKeyboardBuilder()
    for code, label in BUILTIN_KVVO:
        builder.row(InlineKeyboardButton(text=label, callback_data=f"kvvo:{code}"))
    for code in custom_codes:
        builder.row(InlineKeyboardButton(text=code, callback_data=f"kvvo:{code}"))
    builder.row(InlineKeyboardButton(text="✏️ Ввести другой", callback_data="kvvo:custom"))
    if custom_codes:
        builder.row(InlineKeyboardButton(text="🗑 Удалить свой КВВО", callback_data="kvvo:manage"))
    return builder.as_markup()


def kvvo_manage_kb(custom_codes: List[str]) -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    for code in custom_codes:
        builder.row(InlineKeyboardButton(text=f"🗑 {code}", callback_data=f"kvvo_del:{code}"))
    builder.row(InlineKeyboardButton(text="◀️ Назад к выбору", callback_data="kvvo:back"))
    return builder.as_markup()


def skip_kb(skip_callback: str, label: str = "Пропустить / оставить") -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(InlineKeyboardButton(text=f"⏩ {label}", callback_data=skip_callback))
    return builder.as_markup()


def confirm_act_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(InlineKeyboardButton(text="✅ Генерировать документы", callback_data="act:generate"))
    builder.row(InlineKeyboardButton(text="✏️ Редактировать поля",    callback_data="act:edit_menu"))
    builder.row(InlineKeyboardButton(text="❌ Отмена",                 callback_data="menu:main"))
    return builder.as_markup()


def edit_act_fields_kb() -> InlineKeyboardMarkup:
    fields = [
        ("deal_number",     "Номер заявки"),
        ("deal_date",       "Дата заявки"),
        ("va_type",         "Тип ВА"),
        ("network",         "Сеть"),
        ("va_amount",       "Сумма ВА"),
        ("fiat_amount",     "Сумма RUB"),
        ("exchange_rate",   "Курс"),
        ("kvvo",            "КВВО"),
        ("client_wallet",   "Кошелёк клиента"),
        ("operator_wallet", "Кошелёк оператора"),
        ("tx_hash",         "Хэш транзакции"),
        ("commission_fiat", "Комиссия в RUB"),
        ("commission_va",   "Комиссия в ВА"),
        ("execution_date",  "Дата исполнения"),
    ]
    builder = InlineKeyboardBuilder()
    for fkey, flabel in fields:
        builder.row(InlineKeyboardButton(
            text=f"✏️ {flabel}",
            callback_data=f"edit_act_field:{fkey}"
        ))
    builder.row(InlineKeyboardButton(text="◀️ Назад к подтверждению", callback_data="act:confirm_show"))
    return builder.as_markup()


# ─── Settings ─────────────────────────────────────────────────────────────────

def settings_kb() -> InlineKeyboardMarkup:
    fields = [
        ("full_name",     "Полное наименование"),
        ("short_name",    "Краткое наименование"),
        ("inn",           "ИНН"),
        ("kpp",           "КПП"),
        ("address",       "Юр. адрес (КР)"),
        ("legal_address", "Адрес для документов"),
        ("license",       "Лицензия"),
        ("kio",           "КИО"),
        ("inn_rf",        "ИНН в РФ"),
        ("kpp_rf",        "КПП в РФ"),
        ("bank_name",     "Банк"),
        ("bank_account",  "Р/счёт"),
        ("bank_bik",      "БИК"),
        ("wallet",        "Кошелёк оператора"),
        ("director_name", "ФИО директора"),
        ("director_title","Должность директора"),
    ]
    builder = InlineKeyboardBuilder()
    for fkey, flabel in fields:
        builder.row(InlineKeyboardButton(
            text=f"✏️ {flabel}",
            callback_data=f"setting:{fkey}"
        ))
    builder.row(InlineKeyboardButton(text="🏠 Главное меню", callback_data="menu:main"))
    return builder.as_markup()


# ─── Deals (журнал сделок) ────────────────────────────────────────────────────

def deals_menu_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(InlineKeyboardButton(text="🕐 Последние сделки",        callback_data="deals:recent"))
    builder.row(InlineKeyboardButton(text="🏢 Сделки по клиенту",       callback_data="deals:by_company"))
    builder.row(InlineKeyboardButton(text="📦 Архив подписанных (ZIP)", callback_data="arch:menu"))
    builder.row(InlineKeyboardButton(text="🏠 Главное меню",            callback_data="menu:main"))
    return builder.as_markup()


def deals_list_kb(deals: List[Dict[str, Any]]) -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    icons = {"sell": "📤", "buy": "📥", "invoice": "🧾"}
    for d in deals:
        icon = icons.get(d.get("act_type"), "📄")
        builder.row(InlineKeyboardButton(
            text=f"{icon} №{d.get('deal_number','—')} · {d.get('deal_date','')} · {d.get('client_name','')}"[:60],
            callback_data=f"deal:{d['id']}"
        ))
    builder.row(InlineKeyboardButton(text="◀️ Назад", callback_data="deals:menu"))
    return builder.as_markup()


def deal_card_kb(deal_id: int, has_files: bool, signed_count: int = 0) -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    if has_files:
        builder.row(InlineKeyboardButton(text="📄 Прислать документы", callback_data=f"deal_docs:{deal_id}"))
    builder.row(InlineKeyboardButton(
        text=f"📎 Прикрепить подписанный ({signed_count})" if signed_count else "📎 Прикрепить подписанный",
        callback_data=f"deal_attach:{deal_id}"
    ))
    if signed_count:
        builder.row(InlineKeyboardButton(text="📥 Прислать подписанные", callback_data=f"deal_signed:{deal_id}"))
    builder.row(InlineKeyboardButton(text="🗑 Удалить из журнала", callback_data=f"deal_del:{deal_id}"))
    builder.row(InlineKeyboardButton(text="◀️ Назад", callback_data="deals:menu"))
    return builder.as_markup()


def confirm_deal_delete_kb(deal_id: int) -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(
        InlineKeyboardButton(text="✅ Да, удалить", callback_data=f"deal_del_yes:{deal_id}"),
        InlineKeyboardButton(text="❌ Отмена",       callback_data=f"deal:{deal_id}"),
    )
    return builder.as_markup()


def company_pick_kb(companies: List[Dict[str, Any]], prefix: str,
                    back_cb: str = "deals:menu") -> InlineKeyboardMarkup:
    """Универсальный выбор компании: callback = f'{prefix}:{id}'."""
    builder = InlineKeyboardBuilder()
    for c in companies:
        builder.row(InlineKeyboardButton(
            text=f"🏢 {c['short_name']}",
            callback_data=f"{prefix}:{c['id']}"
        ))
    builder.row(InlineKeyboardButton(text="◀️ Назад", callback_data=back_cb))
    return builder.as_markup()


def after_generate_kb(deal_id: int) -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(InlineKeyboardButton(text="📎 Прикрепить подписанный акт", callback_data=f"deal_attach:{deal_id}"))
    builder.row(InlineKeyboardButton(text="📋 Новый акт",   callback_data="menu:create_act"))
    builder.row(InlineKeyboardButton(text="🏠 Главное меню", callback_data="menu:main"))
    return builder.as_markup()


def signed_done_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(InlineKeyboardButton(text="✅ Готово", callback_data="signed:done"))
    return builder.as_markup()


# ─── Reports (отчёт ФН) ───────────────────────────────────────────────────────

def report_months_kb(months: List[str], labels: Dict[str, str]) -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    for m in months:
        builder.row(InlineKeyboardButton(text=f"📅 {labels.get(m, m)}", callback_data=f"report_month:{m}"))
    builder.row(InlineKeyboardButton(text="🏠 Главное меню", callback_data="menu:main"))
    return builder.as_markup()


def report_rate_kb(saved_rate: str | None) -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    if saved_rate:
        builder.row(InlineKeyboardButton(
            text=f"⏩ Использовать {saved_rate}", callback_data="report_rate:saved"
        ))
    builder.row(InlineKeyboardButton(
        text="⏭ Без пересчёта в сомы", callback_data="report_rate:skip"
    ))
    return builder.as_markup()


# ─── Templates & variables ────────────────────────────────────────────────────

TEMPLATE_KINDS = [
    ("sell",    "📤 Акт: продаём ВА клиенту"),
    ("buy",     "📥 Акт: покупаем ВА у клиента"),
    ("invoice", "🧾 Счёт-заявка на покупку"),
]


def templates_menu_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    for kind, label in TEMPLATE_KINDS:
        builder.row(InlineKeyboardButton(text=label, callback_data=f"tpl:view:{kind}"))
    builder.row(InlineKeyboardButton(text="🧩 Свои переменные", callback_data="vars:menu"))
    builder.row(InlineKeyboardButton(text="🏠 Главное меню",    callback_data="menu:main"))
    return builder.as_markup()


def template_card_kb(kind: str) -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(InlineKeyboardButton(text="⬆️ Загрузить новый .docx", callback_data=f"tpl:upload:{kind}"))
    builder.row(InlineKeyboardButton(text="📄 Скачать текущий",        callback_data=f"tpl:download:{kind}"))
    builder.row(InlineKeyboardButton(text="◀️ Назад",                  callback_data="tpl:menu"))
    return builder.as_markup()


def vars_menu_kb(fields: List[Dict[str, str]]) -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    for f in fields:
        builder.row(InlineKeyboardButton(
            text=f"🗑 {{{{CL_{f['key'].upper()}}}}} — {f['label']}"[:60],
            callback_data=f"vars:del:{f['key']}"
        ))
    builder.row(InlineKeyboardButton(text="➕ Добавить переменную", callback_data="vars:add"))
    builder.row(InlineKeyboardButton(text="◀️ Назад к шаблонам",    callback_data="tpl:menu"))
    return builder.as_markup()


# ─── Common ───────────────────────────────────────────────────────────────────

def back_to_main_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(InlineKeyboardButton(text="🏠 Главное меню", callback_data="menu:main"))
    return builder.as_markup()


def resident_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(
        InlineKeyboardButton(text="🇰🇬 Резидент",    callback_data="resident:Резидент"),
        InlineKeyboardButton(text="🌍 Нерезидент",   callback_data="resident:Нерезидент"),
    )
    return builder.as_markup()
