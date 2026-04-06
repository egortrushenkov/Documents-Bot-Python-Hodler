from aiogram.types import InlineKeyboardMarkup, InlineKeyboardButton, ReplyKeyboardMarkup, KeyboardButton
from aiogram.utils.keyboard import InlineKeyboardBuilder


def deal_type_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.button(text="📥 BUY — мы продаём клиенту USDT",  callback_data="deal_buy")
    builder.button(text="📤 SELL — клиент продаёт нам USDT", callback_data="deal_sell")
    builder.adjust(1)
    return builder.as_markup()


def clients_kb(clients: list) -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    for c in clients:
        builder.button(text=f"🏢 {c['name']} (ИНН {c['inn']})", callback_data=f"client_{c['inn']}")
    builder.button(text="✏️ Новый клиент", callback_data="client_new")
    builder.adjust(1)
    return builder.as_markup()


def confirm_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.button(text="✅ Подтвердить и создать документы", callback_data="confirm_yes")
    builder.button(text="🔄 Начать заново",                   callback_data="confirm_no")
    builder.adjust(1)
    return builder.as_markup()


def save_client_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.button(text="💾 Да, сохранить", callback_data="save_yes")
    builder.button(text="❌ Нет",           callback_data="save_no")
    builder.adjust(2)
    return builder.as_markup()


def cancel_kb() -> ReplyKeyboardMarkup:
    return ReplyKeyboardMarkup(
        keyboard=[[KeyboardButton(text="❌ Отмена")]],
        resize_keyboard=True,
        one_time_keyboard=True,
    )
