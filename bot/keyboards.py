from aiogram.types import InlineKeyboardMarkup
from aiogram.utils.keyboard import InlineKeyboardBuilder


def doc_type_kb() -> InlineKeyboardMarkup:
    b = InlineKeyboardBuilder()
    b.button(text="📋 По договору (лицензионный, контракт)", callback_data="doc_contract")
    b.button(text="🌐 По публичной оферте (hodlerexchange.io)", callback_data="doc_offer")
    b.adjust(1)
    return b.as_markup()


def deal_type_kb() -> InlineKeyboardMarkup:
    b = InlineKeyboardBuilder()
    b.button(text="📥 BUY — мы продаём клиенту USDT",  callback_data="deal_buy")
    b.button(text="📤 SELL — клиент продаёт нам USDT", callback_data="deal_sell")
    b.adjust(1)
    return b.as_markup()


def clients_kb(clients: list) -> InlineKeyboardMarkup:
    b = InlineKeyboardBuilder()
    for c in clients:
        name = f"{c['name_eng']} / {c['name_ru']}" if c.get("name_eng") else c["name_ru"]
        b.button(text=f"🏢 {name}", callback_data=f"client_{c['id']}")
    b.button(text="✏️ Новый клиент", callback_data="client_new")
    b.adjust(1)
    return b.as_markup()


def confirm_kb() -> InlineKeyboardMarkup:
    b = InlineKeyboardBuilder()
    b.button(text="✅ Всё верно — создать документы", callback_data="confirm_yes")
    b.button(text="✏️ Изменить клиента",              callback_data="edit_client")
    b.button(text="🔄 Начать заново",                  callback_data="confirm_no")
    b.adjust(1)
    return b.as_markup()


def save_client_kb() -> InlineKeyboardMarkup:
    b = InlineKeyboardBuilder()
    b.button(text="💾 Сохранить", callback_data="save_yes")
    b.button(text="❌ Нет",       callback_data="save_no")
    b.adjust(2)
    return b.as_markup()
