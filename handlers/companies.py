from aiogram import Router, F
from aiogram.types import Message, CallbackQuery
from aiogram.fsm.context import FSMContext
from states import CompanyForm
from keyboards import (
    companies_list_kb, company_card_kb, edit_company_fields_kb,
    confirm_delete_kb, back_to_main_kb, resident_kb
)
import database as db
from utils import esc

router = Router()

# ─── Field definitions ────────────────────────────────────────────────────────

FIELDS = [
    ("full_name",    "📋 Полное наименование",       "ООО «Ромашка»"),
    ("short_name",   "📋 Краткое наименование",       "ОсОО «Ромашка»"),
    ("inn",          "🔢 ИНН",                         "1234567890"),
    ("kpp",          "🔢 КПП",                         "123456789"),
    ("reg_number",   "📎 Рег. номер (ОГРН/КР)",       "1234567890123"),
    ("address",      "🏠 Юридический адрес",           "г. Бишкек, ул. ..."),
    ("kio",          "🌐 КИО (или - если нет)",        "-"),
    ("inn_rf",       "🇷🇺 ИНН в РФ (или -)",          "-"),
    ("kpp_rf",       "🇷🇺 КПП в РФ (или -)",          "-"),
    ("bank_name",    "🏦 Наименование банка",           "КБ «Долинск» (АО)"),
    ("bank_account", "💳 Расчётный счёт",              "40807810..."),
    ("bank_bik",     "🔢 БИК",                         "046401727"),
    ("wallet",       "₿ Крипто-кошелёк",               "TXxxx..."),
]

FIELD_KEYS = [f[0] for f in FIELDS]


def company_card_text(c: dict, custom: dict | None = None,
                      custom_defs: list | None = None) -> str:
    lines = [f"<b>🏢 {esc(c['short_name'])}</b>\n"]
    lines.append(f"<b>Полное наименование:</b> {esc(c['full_name'])}")
    lines.append(f"<b>ИНН:</b> {esc(c['inn'])}  <b>КПП:</b> {esc(c['kpp'])}")
    if c.get("reg_number"):
        lines.append(f"<b>Рег. номер:</b> {esc(c['reg_number'])}")
    lines.append(f"<b>Адрес:</b> {esc(c['address'])}")
    lines.append(f"<b>Статус в КР:</b> {esc(c.get('resident') or 'Резидент')}")
    if c.get("kio") and c["kio"] != "-":
        lines.append(f"<b>КИО:</b> {esc(c['kio'])}")
    if c.get("inn_rf") and c["inn_rf"] != "-":
        lines.append(f"<b>ИНН/КПП в РФ:</b> {esc(c['inn_rf'])} / {esc(c.get('kpp_rf',''))}")
    lines.append(f"\n<b>Банк:</b> {esc(c.get('bank_name','—'))}")
    lines.append(f"<b>Р/счёт:</b> {esc(c.get('bank_account','—'))}")
    lines.append(f"<b>БИК:</b> {esc(c.get('bank_bik','—'))}")
    if c.get("wallet"):
        lines.append(f"\n<b>Кошелёк:</b> <code>{esc(c['wallet'])}</code>")
    if custom_defs and custom:
        extra = [
            f"<b>{esc(f['label'])}:</b> {esc(custom[f['key']])}"
            for f in custom_defs if custom.get(f['key'])
        ]
        if extra:
            lines.append("")
            lines.extend(extra)
    return "\n".join(lines)


async def _send_company_card(answer_fn, c: dict, prefix: str = ""):
    custom_defs = await db.get_custom_fields()
    custom = await db.get_company_custom(c["id"]) if custom_defs else {}
    await answer_fn(
        prefix + company_card_text(c, custom, custom_defs),
        reply_markup=company_card_kb(c["id"]),
        parse_mode="HTML"
    )


# ─── List / menu ──────────────────────────────────────────────────────────────

@router.callback_query(F.data == "menu:companies")
async def show_companies(cb: CallbackQuery, state: FSMContext):
    await state.clear()
    companies = await db.get_all_companies()
    if companies:
        text = f"<b>🏢 Компании</b> ({len(companies)} шт.)\n\nВыберите для просмотра или добавьте новую:"
    else:
        text = "📭 Список компаний пуст.\nДобавьте первую компанию:"
    await cb.message.edit_text(text, reply_markup=companies_list_kb(companies), parse_mode="HTML")
    await cb.answer()


# ─── View card ────────────────────────────────────────────────────────────────

@router.callback_query(F.data.startswith("view_company:"))
async def view_company(cb: CallbackQuery, state: FSMContext):
    await state.clear()
    cid = int(cb.data.split(":")[1])
    c = await db.get_company(cid)
    if not c:
        await cb.answer("Компания не найдена", show_alert=True)
        return
    await _send_company_card(cb.message.edit_text, c)
    await cb.answer()


# ─── Delete ───────────────────────────────────────────────────────────────────

@router.callback_query(F.data.startswith("del_company:"))
async def ask_delete(cb: CallbackQuery):
    cid = int(cb.data.split(":")[1])
    c = await db.get_company(cid)
    await cb.message.edit_text(
        f"⚠️ Удалить компанию <b>{c['short_name']}</b>?\nЭто действие нельзя отменить.",
        reply_markup=confirm_delete_kb(cid),
        parse_mode="HTML"
    )
    await cb.answer()


@router.callback_query(F.data.startswith("confirm_del:"))
async def do_delete(cb: CallbackQuery):
    cid = int(cb.data.split(":")[1])
    await db.delete_company(cid)
    await cb.answer("✅ Компания удалена", show_alert=True)
    companies = await db.get_all_companies()
    text = f"<b>🏢 Компании</b> ({len(companies)} шт.)" if companies else "📭 Список компаний пуст."
    await cb.message.edit_text(text, reply_markup=companies_list_kb(companies), parse_mode="HTML")


# ─── Add new company ──────────────────────────────────────────────────────────

@router.callback_query(F.data == "add_company")
async def start_add(cb: CallbackQuery, state: FSMContext):
    await state.set_state(CompanyForm.full_name)
    await state.update_data(editing_id=None, field_index=0, company_data={})
    key, label, example = FIELDS[0]
    await cb.message.edit_text(
        f"➕ <b>Добавление компании</b>\n\n"
        f"Шаг 1/{len(FIELDS)} — {label}:\n"
        f"<i>Пример: {example}</i>",
        parse_mode="HTML"
    )
    await cb.answer()


# ─── Edit existing field ──────────────────────────────────────────────────────

@router.callback_query(F.data.startswith("edit_company:"))
async def start_edit(cb: CallbackQuery, state: FSMContext):
    cid = int(cb.data.split(":")[1])
    await cb.message.edit_text(
        "Выберите поле для редактирования:",
        reply_markup=edit_company_fields_kb(cid, await db.get_custom_fields())
    )
    await cb.answer()


@router.callback_query(F.data.startswith("edit_company_field:"))
async def edit_one_field(cb: CallbackQuery, state: FSMContext):
    _, cid_str, fkey = cb.data.split(":")
    cid = int(cid_str)
    c = await db.get_company(cid)

    # Резидентство меняется кнопками
    if fkey == "resident":
        await state.set_state(CompanyForm.resident)
        await state.update_data(editing_id=cid, single_field="resident")
        await cb.message.edit_text(
            f"✏️ <b>Резидент/Нерезидент КР</b>\n\n"
            f"Текущее значение: <code>{c.get('resident') or 'Резидент'}</code>",
            reply_markup=resident_kb(),
            parse_mode="HTML"
        )
        await cb.answer()
        return

    # Своя переменная (c__<key>)
    if fkey.startswith("c__"):
        key = fkey[3:]
        defs = await db.get_custom_fields()
        label = next((f["label"] for f in defs if f["key"] == key), key)
        custom = await db.get_company_custom(cid)
        current = custom.get(key, "")
    else:
        label = next((f[1] for f in FIELDS if f[0] == fkey), fkey)
        current = c.get(fkey, "")

    await state.set_state(CompanyForm.full_name)  # reuse state
    await state.update_data(editing_id=cid, single_field=fkey, company_data=dict(c))
    await cb.message.edit_text(
        f"✏️ <b>{esc(label)}</b>\n\n"
        f"Текущее значение: <code>{esc(current)}</code>\n\n"
        f"Введите новое значение:",
        parse_mode="HTML"
    )
    await cb.answer()


# ─── Message handler for all company form states ──────────────────────────────

STATES_ORDER = [
    CompanyForm.full_name, CompanyForm.short_name, CompanyForm.inn,
    CompanyForm.kpp, CompanyForm.reg_number, CompanyForm.address,
    CompanyForm.kio, CompanyForm.inn_rf, CompanyForm.kpp_rf,
    CompanyForm.bank_name, CompanyForm.bank_account,
    CompanyForm.bank_bik, CompanyForm.wallet,
]


@router.message(CompanyForm.full_name)
@router.message(CompanyForm.short_name)
@router.message(CompanyForm.inn)
@router.message(CompanyForm.kpp)
@router.message(CompanyForm.reg_number)
@router.message(CompanyForm.address)
@router.message(CompanyForm.kio)
@router.message(CompanyForm.inn_rf)
@router.message(CompanyForm.kpp_rf)
@router.message(CompanyForm.bank_name)
@router.message(CompanyForm.bank_account)
@router.message(CompanyForm.bank_bik)
@router.message(CompanyForm.wallet)
async def handle_company_input(msg: Message, state: FSMContext):
    data = await state.get_data()
    value = msg.text.strip()

    # Single-field edit mode
    if data.get("single_field"):
        fkey = data["single_field"]
        cid  = data["editing_id"]
        if fkey.startswith("c__"):
            await db.set_company_custom(cid, fkey[3:], "" if value == "-" else value)
        else:
            await db.update_company(cid, {fkey: value})
        await state.clear()
        c = await db.get_company(cid)
        await _send_company_card(msg.answer, c, "✅ Поле обновлено!\n\n")
        return

    # Sequential add mode
    idx = data.get("field_index", 0)
    company_data = data.get("company_data", {})
    fkey = FIELDS[idx][0]
    company_data[fkey] = value

    next_idx = idx + 1
    if next_idx < len(FIELDS):
        key, label, example = FIELDS[next_idx]
        await state.update_data(field_index=next_idx, company_data=company_data)
        await state.set_state(STATES_ORDER[next_idx])
        await msg.answer(
            f"Шаг {next_idx+1}/{len(FIELDS)} — {label}:\n"
            f"<i>Пример: {example}</i>\n"
            f"(Введите <code>-</code> чтобы пропустить)",
            parse_mode="HTML"
        )
    else:
        # Реквизиты заполнены → резидентство (кнопками)
        await state.update_data(company_data=company_data)
        await state.set_state(CompanyForm.resident)
        await msg.answer(
            f"Шаг {len(FIELDS)+1} — Резидент или нерезидент Кыргызской Республики?\n"
            f"<i>(нужно для отчёта в Финнадзор)</i>",
            reply_markup=resident_kb(),
            parse_mode="HTML"
        )


# ─── Резидентство ─────────────────────────────────────────────────────────────

@router.callback_query(CompanyForm.resident, F.data.startswith("resident:"))
async def set_resident(cb: CallbackQuery, state: FSMContext):
    value = cb.data.split(":", 1)[1]
    data = await state.get_data()

    # Режим правки одного поля существующей компании
    if data.get("single_field") == "resident" and data.get("editing_id"):
        cid = data["editing_id"]
        await db.update_company(cid, {"resident": value})
        await state.clear()
        c = await db.get_company(cid)
        await _send_company_card(cb.message.edit_text, c, "✅ Поле обновлено!\n\n")
        await cb.answer()
        return

    # Режим добавления новой компании
    company_data = data.get("company_data", {})
    company_data["resident"] = value
    await state.update_data(company_data=company_data)
    await cb.answer(value)
    await _next_custom_or_save(cb.message.edit_text, state)


@router.message(CompanyForm.resident)
async def resident_text_input(msg: Message, state: FSMContext):
    """Резидентство, введённое текстом (вместо кнопки)."""
    value = "Нерезидент" if msg.text.strip().lower().startswith("нерез") else "Резидент"
    data = await state.get_data()

    if data.get("single_field") == "resident" and data.get("editing_id"):
        cid = data["editing_id"]
        await db.update_company(cid, {"resident": value})
        await state.clear()
        c = await db.get_company(cid)
        await _send_company_card(msg.answer, c, "✅ Поле обновлено!\n\n")
        return

    company_data = data.get("company_data", {})
    company_data["resident"] = value
    await state.update_data(company_data=company_data)
    await _next_custom_or_save(msg.answer, state)


async def _next_custom_or_save(answer_fn, state: FSMContext):
    """Задаёт вопросы по своим переменным, затем сохраняет компанию."""
    data = await state.get_data()
    defs = data.get("custom_defs")
    if defs is None:
        defs = await db.get_custom_fields()
        await state.update_data(custom_defs=defs, custom_index=0, custom_values={})
        data = await state.get_data()

    idx = data.get("custom_index", 0)
    if idx < len(defs):
        f = defs[idx]
        await state.set_state(CompanyForm.custom)
        await answer_fn(
            f"🧩 <b>{esc(f['label'])}</b>\n"
            f"<i>(своя переменная {{{{CL_{f['key'].upper()}}}}}; "
            f"введите <code>-</code> чтобы пропустить)</i>",
            parse_mode="HTML"
        )
        return

    # Всё собрано — сохраняем
    company_data = data.get("company_data", {})
    cid = await db.add_company(company_data)
    for key, value in (data.get("custom_values") or {}).items():
        await db.set_company_custom(cid, key, value)
    await state.clear()
    c = await db.get_company(cid)
    await _send_company_card(answer_fn, c, "✅ <b>Компания добавлена!</b>\n\n")


@router.message(CompanyForm.custom)
async def handle_custom_input(msg: Message, state: FSMContext):
    data = await state.get_data()
    defs = data.get("custom_defs", [])
    idx = data.get("custom_index", 0)
    values = data.get("custom_values", {})
    value = msg.text.strip()
    if idx < len(defs) and value != "-":
        values[defs[idx]["key"]] = value
    await state.update_data(custom_index=idx + 1, custom_values=values)
    await _next_custom_or_save(msg.answer, state)


# ─── Reply keyboard shortcuts ─────────────────────────────────────────────────

@router.message(F.text == "🏢 Компании")
async def shortcut_companies(msg: Message, state: FSMContext):
    await state.clear()
    companies = await db.get_all_companies()
    text = f"<b>🏢 Компании</b> ({len(companies)} шт.)" if companies else "📭 Список компаний пуст."
    await msg.answer(text, reply_markup=companies_list_kb(companies), parse_mode="HTML")
