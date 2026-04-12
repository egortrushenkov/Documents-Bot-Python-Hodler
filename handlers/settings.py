from aiogram import Router, F
from aiogram.types import Message, CallbackQuery
from aiogram.fsm.context import FSMContext
from states import SettingsForm
from keyboards import settings_kb, back_to_main_kb
import database as db

router = Router()

FIELD_LABELS = {
    "full_name":     "Полное наименование",
    "short_name":    "Краткое наименование",
    "inn":           "ИНН",
    "kpp":           "КПП",
    "address":       "Юр. адрес (КР)",
    "legal_address": "Адрес для документов",
    "license":       "Лицензия",
    "kio":           "КИО",
    "inn_rf":        "ИНН в РФ",
    "kpp_rf":        "КПП в РФ",
    "bank_name":     "Банк",
    "bank_account":  "Р/счёт",
    "bank_bik":      "БИК",
    "wallet":        "Кошелёк оператора",
    "director_name": "ФИО директора",
    "director_title":"Должность директора",
}


def settings_text(s: dict) -> str:
    lines = ["<b>⚙️ Реквизиты оператора</b>\n"]
    lines.append(f"<b>Наименование:</b> {s.get('short_name','—')}")
    lines.append(f"<b>Полное:</b> {s.get('full_name','—')}")
    lines.append(f"<b>ИНН:</b> {s.get('inn','—')}  <b>КПП:</b> {s.get('kpp','—')}")
    lines.append(f"<b>Адрес:</b> {s.get('address','—')}")
    lines.append(f"<b>Лицензия:</b> {s.get('license','—')}")
    lines.append(f"<b>КИО:</b> {s.get('kio','—')}")
    lines.append(f"<b>ИНН/КПП в РФ:</b> {s.get('inn_rf','—')} / {s.get('kpp_rf','—')}")
    lines.append(f"\n<b>Банк:</b> {s.get('bank_name','—')}")
    lines.append(f"<b>Р/счёт:</b> {s.get('bank_account','—')}")
    lines.append(f"<b>БИК:</b> {s.get('bank_bik','—')}")
    lines.append(f"\n<b>Кошелёк:</b> <code>{s.get('wallet','—')}</code>")
    lines.append(f"\n<b>Директор:</b> {s.get('director_name','—')} ({s.get('director_title','—')})")
    return "\n".join(lines)


@router.callback_query(F.data == "menu:settings")
async def show_settings(cb: CallbackQuery, state: FSMContext):
    await state.clear()
    s = await db.get_all_settings()
    await cb.message.edit_text(
        settings_text(s),
        reply_markup=settings_kb(),
        parse_mode="HTML"
    )
    await cb.answer()


@router.callback_query(F.data.startswith("setting:"))
async def edit_setting(cb: CallbackQuery, state: FSMContext):
    fkey = cb.data.split(":", 1)[1]
    s = await db.get_all_settings()
    current = s.get(fkey, "")
    label = FIELD_LABELS.get(fkey, fkey)
    await state.set_state(SettingsForm.editing)
    await state.update_data(setting_key=fkey)
    await cb.message.edit_text(
        f"✏️ <b>{label}</b>\n\n"
        f"Текущее значение:\n<code>{current}</code>\n\n"
        f"Введите новое значение:",
        parse_mode="HTML"
    )
    await cb.answer()


@router.message(SettingsForm.editing)
async def save_setting(msg: Message, state: FSMContext):
    data = await state.get_data()
    fkey = data.get("setting_key", "")
    await db.set_setting(fkey, msg.text.strip())
    await state.clear()
    s = await db.get_all_settings()
    await msg.answer(
        f"✅ Сохранено!\n\n" + settings_text(s),
        reply_markup=settings_kb(),
        parse_mode="HTML"
    )


# ─── Reply keyboard shortcut ──────────────────────────────────────────────────

@router.message(F.text == "⚙️ Реквизиты")
async def shortcut_settings(msg: Message, state: FSMContext):
    await state.clear()
    s = await db.get_all_settings()
    await msg.answer(settings_text(s), reply_markup=settings_kb(), parse_mode="HTML")
