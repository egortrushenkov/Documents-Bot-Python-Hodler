from aiogram.types import (
    InlineKeyboardMarkup, InlineKeyboardButton,
    ReplyKeyboardMarkup, KeyboardButton, ReplyKeyboardRemove
)
from aiogram.utils.keyboard import InlineKeyboardBuilder, ReplyKeyboardBuilder
from typing import List, Dict, Any

remove_kb = ReplyKeyboardRemove()


# ─── Main Menu ────────────────────────────────────────────────────────────────

def main_menu_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(
        InlineKeyboardButton(text="📋 Создать акт",      callback_data="menu:create_act"),
        InlineKeyboardButton(text="🏢 Компании",          callback_data="menu:companies"),
    )
    builder.row(
        InlineKeyboardButton(text="⚙️ Реквизиты оператора", callback_data="menu:settings"),
    )
    return builder.as_markup()


# ─── Companies ────────────────────────────────────────────────────────────────

def companies_list_kb(companies: List[Dict[str, Any]], mode: str = "manage") -> InlineKeyboardMarkup:
    """mode: manage | select"""
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


def edit_company_fields_kb(company_id: int) -> InlineKeyboardMarkup:
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
    ]
    builder = InlineKeyboardBuilder()
    for fkey, flabel in fields:
        builder.row(InlineKeyboardButton(
            text=f"✏️ {flabel}",
            callback_data=f"edit_company_field:{company_id}:{fkey}"
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
    builder.row(InlineKeyboardButton(text="❌ Отмена", callback_data="menu:main"))
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


def kvvo_kb() -> InlineKeyboardMarkup:
    codes = [
        ("99082", "99082 — прочие операции с ВА"),
        ("99081", "99081 — обмен ВА"),
        ("10100", "10100 — покупка ин. валюты"),
        ("20100", "20100 — продажа ин. валюты"),
    ]
    builder = InlineKeyboardBuilder()
    for code, label in codes:
        builder.row(InlineKeyboardButton(text=label, callback_data=f"kvvo:{code}"))
    builder.row(InlineKeyboardButton(text="✏️ Ввести другой", callback_data="kvvo:custom"))
    return builder.as_markup()


def skip_kb(skip_callback: str, label: str = "Пропустить / оставить") -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(InlineKeyboardButton(text=f"⏩ {label}", callback_data=skip_callback))
    return builder.as_markup()


def confirm_act_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(InlineKeyboardButton(text="✅ Генерировать документы", callback_data="act:generate"))
    builder.row(InlineKeyboardButton(text="✏️ Редактировать поля", callback_data="act:edit_menu"))
    builder.row(InlineKeyboardButton(text="❌ Отмена", callback_data="menu:main"))
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


# ─── Common ───────────────────────────────────────────────────────────────────

def back_to_main_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.row(InlineKeyboardButton(text="🏠 Главное меню", callback_data="menu:main"))
    return builder.as_markup()
