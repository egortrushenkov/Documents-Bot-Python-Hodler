"""
Полный FSM-флоу создания акта.
Шаги: тип → компания → реквизиты сделки (12 полей) → подтверждение → генерация.
"""
import os
from datetime import date
from aiogram import Router, F
from aiogram.types import Message, CallbackQuery, FSInputFile
from aiogram.fsm.context import FSMContext
from states import ActForm
from keyboards import (
    act_type_kb, companies_list_kb, va_type_kb, network_kb,
    kvvo_kb, skip_kb, confirm_act_kb, edit_act_fields_kb, back_to_main_kb,
    add_tx_kb
)
import database as db
from services.doc_generator import generate_docx
from services.pdf_converter import convert_to_pdf

router = Router()

# ─── Helpers ──────────────────────────────────────────────────────────────────

TODAY = lambda: date.today().strftime("%d.%m.%Y")

FIELD_LABELS = {
    "deal_number":     "Номер заявки",
    "deal_date":       "Дата заявки",
    "va_type":         "Тип ВА",
    "network":         "Сеть",
    "va_amount":       "Сумма ВА",
    "fiat_amount":     "Сумма RUB",
    "exchange_rate":   "Курс обмена",
    "kvvo":            "КВВО",
    "client_wallet":   "Кошелёк клиента",
    "operator_wallet": "Кошелёк оператора",
    "tx_hash":         "Хэш транзакции",
    "commission_fiat": "Комиссия в RUB",
    "commission_va":   "Комиссия в ВА",
    "execution_date":  "Дата исполнения",
}


DIRECTION_LABELS = {
    "sell":    "📤 Продаём ВА клиенту (покупка у нас)",
    "buy":     "📥 Покупаем ВА у клиента (продажа нам)",
    "invoice": "🧾 Счёт-заявка на покупку клиентом",
}


def _transactions(data: dict) -> list:
    """Текущий список транзакций (минимум одна — из основных полей)."""
    txs = data.get("transactions")
    if txs and len(txs) > 1:
        return txs
    return [{"va_amount": data.get("va_amount", ""), "tx_hash": data.get("tx_hash", "")}]


def _va_total(data: dict) -> float:
    total = 0.0
    for t in _transactions(data):
        try:
            total += float(str(t.get("va_amount", "0")).replace(",", ".").replace(" ", ""))
        except Exception:
            pass
    return total


def deal_summary(data: dict, client: dict) -> str:
    act_type  = data.get("act_type", "sell")
    direction = DIRECTION_LABELS.get(act_type, act_type)
    doc_word  = "счёта" if act_type == "invoice" else "акта"

    lines = [
        f"<b>📄 Сводка {doc_word}</b>\n",
        f"<b>Тип:</b> {direction}",
        f"<b>Клиент:</b> {client.get('short_name','—')}",
        f"",
        f"<b>Номер заявки:</b> №{data.get('deal_number','—')}",
        f"<b>Дата заявки:</b> {data.get('deal_date','—')}",
        f"<b>Дата исполнения:</b> {data.get('execution_date','—')}",
        f"",
        f"<b>Актив:</b> {data.get('va_type','—')} ({data.get('network','—')})",
    ]

    txs = _transactions(data)
    if act_type != "invoice" and len(txs) > 1:
        lines.append(f"<b>Транзакций:</b> {len(txs)}")
        for i, t in enumerate(txs, 1):
            lines.append(
                f"  {i}) {t.get('va_amount','—')} {data.get('va_type','')} — "
                f"<code>{t.get('tx_hash','—')}</code>"
            )
        lines.append(f"<b>Итого ВА:</b> {_va_total(data):g} {data.get('va_type','')}")
    else:
        lines.append(f"<b>Сумма ВА:</b> {data.get('va_amount','—')} {data.get('va_type','')}")

    lines += [
        f"<b>Сумма RUB:</b> {data.get('fiat_amount','—')} RUB",
        f"<b>Курс:</b> {data.get('exchange_rate','—')}",
        f"<b>КВВО:</b> {data.get('kvvo','—')}",
        f"",
        f"<b>Кошелёк клиента:</b>\n<code>{data.get('client_wallet','—')}</code>",
        f"<b>Кошелёк оператора:</b>\n<code>{data.get('operator_wallet','—')}</code>",
    ]
    if act_type != "invoice" and len(txs) == 1:
        lines.append(f"<b>Хэш:</b>\n<code>{data.get('tx_hash','—')}</code>")
    lines += [
        f"",
        f"<b>Комиссия (RUB):</b> {data.get('commission_fiat','0%')}",
        f"<b>Комиссия (ВА):</b> {data.get('commission_va','0%')}",
    ]
    return "\n".join(lines)


# ─── Entry point ──────────────────────────────────────────────────────────────

@router.callback_query(F.data == "menu:create_act")
async def start_create_act(cb: CallbackQuery, state: FSMContext):
    await state.clear()
    await state.set_state(ActForm.select_type)
    await cb.message.edit_text(
        "📋 <b>Создание акта</b>\n\nВыберите тип операции:",
        reply_markup=act_type_kb(),
        parse_mode="HTML"
    )
    await cb.answer()


# ─── Step 1: Type ─────────────────────────────────────────────────────────────

@router.callback_query(ActForm.select_type, F.data.startswith("act_type:"))
async def step_type(cb: CallbackQuery, state: FSMContext):
    act_type = cb.data.split(":")[1]  # sell | buy
    await state.update_data(act_type=act_type)
    await state.set_state(ActForm.select_company)

    companies = await db.get_all_companies()
    if not companies:
        await cb.message.edit_text(
            "⚠️ Нет ни одной компании в базе.\n"
            "Сначала добавьте компанию через раздел 🏢 Компании.",
            reply_markup=back_to_main_kb()
        )
        await cb.answer()
        return

    await cb.message.edit_text(
        "Выберите компанию-клиента:",
        reply_markup=companies_list_kb(companies, mode="select")
    )
    await cb.answer()


# ─── Step 2: Company ──────────────────────────────────────────────────────────

@router.callback_query(ActForm.select_company, F.data.startswith("sel_company:"))
async def step_company(cb: CallbackQuery, state: FSMContext):
    cid = int(cb.data.split(":")[1])
    client = await db.get_company(cid)
    if not client:
        await cb.answer("Компания не найдена", show_alert=True)
        return

    # Pre-fill client wallet from company
    await state.update_data(
        company_id=cid,
        client_wallet=client.get("wallet", ""),
    )

    # Pre-fill operator wallet from settings
    op = await db.get_all_settings()
    await state.update_data(operator_wallet=op.get("wallet", ""))

    # Next act number
    next_num = await db.peek_next_number()
    await state.update_data(deal_number=str(next_num))
    await state.set_state(ActForm.deal_number)

    await cb.message.edit_text(
        f"✅ Клиент: <b>{client['short_name']}</b>\n\n"
        f"<b>Шаг: Номер заявки</b>\n"
        f"Предлагаемый номер: <b>{next_num}</b>\n\n"
        f"Введите номер или нажмите «Использовать»:",
        reply_markup=skip_kb("skip:deal_number", f"Использовать №{next_num}"),
        parse_mode="HTML"
    )
    await cb.answer()


# ─── Step 3: Deal number ──────────────────────────────────────────────────────

@router.callback_query(ActForm.deal_number, F.data == "skip:deal_number")
async def skip_deal_number(cb: CallbackQuery, state: FSMContext):
    data = await state.get_data()
    num = data.get("deal_number", "")
    # Increment counter in DB
    await db.next_act_number()
    await _ask_deal_date(cb.message.edit_text, state, num)
    await cb.answer()


@router.message(ActForm.deal_number)
async def input_deal_number(msg: Message, state: FSMContext):
    val = msg.text.strip()
    await state.update_data(deal_number=val)
    await _ask_deal_date(msg.answer, state, val)


async def _ask_deal_date(answer_fn, state, deal_number):
    today = TODAY()
    await state.update_data(deal_date=today)
    await state.set_state(ActForm.deal_date)
    await answer_fn(
        f"<b>Шаг: Дата заявки</b>\n"
        f"Сегодня: <b>{today}</b>\n\n"
        f"Введите дату (ДД.ММ.ГГГГ) или нажмите «Сегодня»:",
        reply_markup=skip_kb("skip:deal_date", f"Сегодня ({today})"),
        parse_mode="HTML"
    )


# ─── Step 4: Deal date ────────────────────────────────────────────────────────

@router.callback_query(ActForm.deal_date, F.data == "skip:deal_date")
async def skip_deal_date(cb: CallbackQuery, state: FSMContext):
    await _ask_va_type(cb.message.edit_text, state)
    await cb.answer()


@router.message(ActForm.deal_date)
async def input_deal_date(msg: Message, state: FSMContext):
    val = msg.text.strip()
    # Basic validation
    if len(val) == 10 and val[2] == "." and val[5] == ".":
        await state.update_data(deal_date=val, execution_date=val)
        await _ask_va_type(msg.answer, state)
    else:
        await msg.answer("❌ Формат: ДД.ММ.ГГГГ — например, 12.04.2026")


async def _ask_va_type(answer_fn, state):
    await state.set_state(ActForm.va_type)
    await answer_fn(
        "<b>Шаг: Тип виртуального актива</b>\nВыберите или введите свой:",
        reply_markup=va_type_kb(),
        parse_mode="HTML"
    )


# ─── Step 5: VA type ─────────────────────────────────────────────────────────

@router.callback_query(ActForm.va_type, F.data.startswith("va_type:"))
async def step_va_type(cb: CallbackQuery, state: FSMContext):
    val = cb.data.split(":", 1)[1]
    if val == "custom":
        await cb.message.edit_text("Введите тип актива (например: USDT, BTC):")
        await cb.answer()
        return
    await state.update_data(va_type=val)
    await _ask_network(cb.message.edit_text, state)
    await cb.answer()


@router.message(ActForm.va_type)
async def input_va_type(msg: Message, state: FSMContext):
    await state.update_data(va_type=msg.text.strip().upper())
    await _ask_network(msg.answer, state)


async def _ask_network(answer_fn, state):
    await state.set_state(ActForm.network)
    await answer_fn(
        "<b>Шаг: Сеть (blockchain)</b>\nВыберите или введите свою:",
        reply_markup=network_kb(),
        parse_mode="HTML"
    )


# ─── Step 6: Network ─────────────────────────────────────────────────────────

@router.callback_query(ActForm.network, F.data.startswith("network:"))
async def step_network(cb: CallbackQuery, state: FSMContext):
    val = cb.data.split(":", 1)[1]
    if val == "custom":
        await cb.message.edit_text("Введите название сети (например: TRC-20, ERC-20):")
        await cb.answer()
        return
    await state.update_data(network=val)
    await _ask_va_amount(cb.message.edit_text, state)
    await cb.answer()


@router.message(ActForm.network)
async def input_network(msg: Message, state: FSMContext):
    await state.update_data(network=msg.text.strip())
    await _ask_va_amount(msg.answer, state)


async def _ask_va_amount(answer_fn, state):
    data = await state.get_data()
    await state.set_state(ActForm.va_amount)
    await answer_fn(
        f"<b>Шаг: Сумма виртуального актива</b>\n"
        f"Актив: {data.get('va_type','ВА')}\n\n"
        f"Введите сумму (например: 6287.726):",
        parse_mode="HTML"
    )


# ─── Step 7: VA amount ────────────────────────────────────────────────────────

@router.message(ActForm.va_amount)
async def input_va_amount(msg: Message, state: FSMContext):
    val = msg.text.strip().replace(",", ".")
    await state.update_data(va_amount=val)
    await state.set_state(ActForm.fiat_amount)
    await msg.answer(
        "<b>Шаг: Сумма в рублях (RUB)</b>\n"
        "Введите сумму (например: 500000):",
        parse_mode="HTML"
    )


# ─── Step 8: Fiat amount ──────────────────────────────────────────────────────

@router.message(ActForm.fiat_amount)
async def input_fiat_amount(msg: Message, state: FSMContext):
    val = msg.text.strip().replace(" ", "").replace(",", ".")
    await state.update_data(fiat_amount=val)
    data = await state.get_data()

    # Auto-calculate rate if both amounts provided
    try:
        va  = float(data.get("va_amount","0").replace(",","."))
        rub = float(val)
        rate = round(rub / va, 2) if va else 0
        rate_str = str(rate).replace(".", ",")
        await state.update_data(exchange_rate=rate_str)
    except Exception:
        rate_str = ""

    await state.set_state(ActForm.exchange_rate)
    hint = f"\n<i>Автовычисленный курс: {rate_str}</i>" if rate_str else ""
    await msg.answer(
        f"<b>Шаг: Курс обмена ВА к RUB</b>{hint}\n\n"
        f"Введите курс или нажмите «Использовать» авторасчёт:",
        reply_markup=skip_kb("skip:exchange_rate", f"Использовать {rate_str}") if rate_str else None,
        parse_mode="HTML"
    )


# ─── Step 9: Exchange rate ────────────────────────────────────────────────────

@router.callback_query(ActForm.exchange_rate, F.data == "skip:exchange_rate")
async def skip_rate(cb: CallbackQuery, state: FSMContext):
    await _ask_kvvo(cb.message.edit_text, state)
    await cb.answer()


@router.message(ActForm.exchange_rate)
async def input_rate(msg: Message, state: FSMContext):
    await state.update_data(exchange_rate=msg.text.strip())
    await _ask_kvvo(msg.answer, state)


async def _ask_kvvo(answer_fn, state):
    await state.set_state(ActForm.kvvo)
    await answer_fn(
        "<b>Шаг: КВВО (код вида валютной операции)</b>\nВыберите или введите свой:",
        reply_markup=kvvo_kb(),
        parse_mode="HTML"
    )


# ─── Step 10: KVVO ───────────────────────────────────────────────────────────

@router.callback_query(ActForm.kvvo, F.data.startswith("kvvo:"))
async def step_kvvo(cb: CallbackQuery, state: FSMContext):
    val = cb.data.split(":", 1)[1]
    if val == "custom":
        await cb.message.edit_text("Введите код КВВО:")
        await cb.answer()
        return
    await state.update_data(kvvo=val)
    await _ask_client_wallet(cb.message.edit_text, state)
    await cb.answer()


@router.message(ActForm.kvvo)
async def input_kvvo(msg: Message, state: FSMContext):
    await state.update_data(kvvo=msg.text.strip())
    await _ask_client_wallet(msg.answer, state)


async def _ask_client_wallet(answer_fn, state):
    data = await state.get_data()
    current = data.get("client_wallet", "")
    await state.set_state(ActForm.client_wallet)
    await answer_fn(
        f"<b>Шаг: Кошелёк клиента</b>\n"
        f"Из карточки компании: <code>{current}</code>\n\n"
        f"Введите другой адрес или нажмите «Использовать»:",
        reply_markup=skip_kb("skip:client_wallet", "Из карточки компании"),
        parse_mode="HTML"
    )


# ─── Step 11: Client wallet ───────────────────────────────────────────────────

@router.callback_query(ActForm.client_wallet, F.data == "skip:client_wallet")
async def skip_client_wallet(cb: CallbackQuery, state: FSMContext):
    await _ask_operator_wallet(cb.message.edit_text, state)
    await cb.answer()


@router.message(ActForm.client_wallet)
async def input_client_wallet(msg: Message, state: FSMContext):
    await state.update_data(client_wallet=msg.text.strip())
    await _ask_operator_wallet(msg.answer, state)


async def _ask_operator_wallet(answer_fn, state):
    data = await state.get_data()
    current = data.get("operator_wallet", "")
    await state.set_state(ActForm.operator_wallet)
    await answer_fn(
        f"<b>Шаг: Кошелёк оператора</b>\n"
        f"Из настроек: <code>{current}</code>\n\n"
        f"Введите другой адрес или нажмите «Использовать»:",
        reply_markup=skip_kb("skip:operator_wallet", "Из настроек"),
        parse_mode="HTML"
    )


# ─── Step 12: Operator wallet ─────────────────────────────────────────────────

async def _after_operator_wallet(answer_fn, state):
    """Счёт-заявка не содержит хэшей транзакций — сразу к комиссии."""
    data = await state.get_data()
    if data.get("act_type") == "invoice":
        await _ask_commission(answer_fn, state)
    else:
        await _ask_tx_hash(answer_fn, state)


@router.callback_query(ActForm.operator_wallet, F.data == "skip:operator_wallet")
async def skip_op_wallet(cb: CallbackQuery, state: FSMContext):
    await _after_operator_wallet(cb.message.edit_text, state)
    await cb.answer()


@router.message(ActForm.operator_wallet)
async def input_op_wallet(msg: Message, state: FSMContext):
    await state.update_data(operator_wallet=msg.text.strip())
    await _after_operator_wallet(msg.answer, state)


async def _ask_tx_hash(answer_fn, state):
    await state.set_state(ActForm.tx_hash)
    await answer_fn(
        "<b>Шаг: Хэш транзакции в блокчейне</b>\n"
        "Введите tx hash (или <code>-</code> если ещё нет):",
        reply_markup=skip_kb("skip:tx_hash", "Пропустить (заполнить позже)"),
        parse_mode="HTML"
    )


# ─── Step 13: TX hash ─────────────────────────────────────────────────────────

@router.callback_query(ActForm.tx_hash, F.data == "skip:tx_hash")
async def skip_tx_hash(cb: CallbackQuery, state: FSMContext):
    await state.update_data(tx_hash="-")
    await _init_transactions(state)
    await _ask_add_tx(cb.message.edit_text, state)
    await cb.answer()


@router.message(ActForm.tx_hash)
async def input_tx_hash(msg: Message, state: FSMContext):
    await state.update_data(tx_hash=msg.text.strip())
    await _init_transactions(state)
    await _ask_add_tx(msg.answer, state)


# ─── Step 13b: несколько транзакций ───────────────────────────────────────────

async def _init_transactions(state):
    """Создаёт список транзакций из первой (основной) транзакции."""
    data = await state.get_data()
    await state.update_data(transactions=[{
        "va_amount": data.get("va_amount", ""),
        "tx_hash":   data.get("tx_hash", ""),
    }])


async def _ask_add_tx(answer_fn, state):
    data = await state.get_data()
    txs  = data.get("transactions", [])
    await state.set_state(ActForm.add_tx)
    lines = [f"<b>Транзакций в акте: {len(txs)}</b>"]
    for i, t in enumerate(txs, 1):
        lines.append(f"  {i}) {t.get('va_amount','—')} {data.get('va_type','')} — <code>{t.get('tx_hash','—')}</code>")
    lines.append("\nДобавить ещё одну транзакцию (строку в таблицу) или продолжить?")
    await answer_fn("\n".join(lines), reply_markup=add_tx_kb(), parse_mode="HTML")


@router.callback_query(ActForm.add_tx, F.data == "tx:add")
async def add_tx_start(cb: CallbackQuery, state: FSMContext):
    await state.set_state(ActForm.extra_va)
    await cb.message.edit_text(
        "<b>Доп. транзакция — сумма ВА</b>\n"
        "Введите сумму виртуального актива (например: 6287.726):",
        parse_mode="HTML"
    )
    await cb.answer()


@router.message(ActForm.extra_va)
async def add_tx_va(msg: Message, state: FSMContext):
    await state.update_data(_extra_va=msg.text.strip().replace(",", "."))
    await state.set_state(ActForm.extra_tx_hash)
    await msg.answer(
        "<b>Доп. транзакция — хэш</b>\n"
        "Введите tx hash (или <code>-</code> если ещё нет):",
        reply_markup=skip_kb("tx:extra_skip_hash", "Пропустить (-)"),
        parse_mode="HTML"
    )


async def _append_transaction(state, tx_hash: str):
    data = await state.get_data()
    txs  = list(data.get("transactions", []))
    txs.append({"va_amount": data.get("_extra_va", ""), "tx_hash": tx_hash})
    await state.update_data(transactions=txs)


@router.callback_query(ActForm.extra_tx_hash, F.data == "tx:extra_skip_hash")
async def add_tx_skip_hash(cb: CallbackQuery, state: FSMContext):
    await _append_transaction(state, "-")
    await _ask_add_tx(cb.message.edit_text, state)
    await cb.answer()


@router.message(ActForm.extra_tx_hash)
async def add_tx_hash(msg: Message, state: FSMContext):
    await _append_transaction(state, msg.text.strip())
    await _ask_add_tx(msg.answer, state)


@router.callback_query(ActForm.add_tx, F.data == "tx:done")
async def add_tx_done(cb: CallbackQuery, state: FSMContext):
    await _ask_commission(cb.message.edit_text, state)
    await cb.answer()


async def _ask_commission(answer_fn, state):
    await state.set_state(ActForm.commission_fiat)
    await state.update_data(commission_fiat="0%", commission_va="0%")
    await answer_fn(
        "<b>Шаг: Комиссия в RUB (%)</b>\n"
        "Введите значение (например: 1.5%) или нажмите «0%»:",
        reply_markup=skip_kb("skip:commission_fiat", "0%"),
        parse_mode="HTML"
    )


# ─── Step 14: Commission fiat ─────────────────────────────────────────────────

@router.callback_query(ActForm.commission_fiat, F.data == "skip:commission_fiat")
async def skip_comm_fiat(cb: CallbackQuery, state: FSMContext):
    await _ask_commission_va(cb.message.edit_text, state)
    await cb.answer()


@router.message(ActForm.commission_fiat)
async def input_comm_fiat(msg: Message, state: FSMContext):
    await state.update_data(commission_fiat=msg.text.strip())
    await _ask_commission_va(msg.answer, state)


async def _ask_commission_va(answer_fn, state):
    await state.set_state(ActForm.commission_va)
    await answer_fn(
        "<b>Шаг: Комиссия в ВА (%)</b>\n"
        "Введите значение или нажмите «0%»:",
        reply_markup=skip_kb("skip:commission_va", "0%"),
        parse_mode="HTML"
    )


# ─── Step 15: Commission VA ───────────────────────────────────────────────────

@router.callback_query(ActForm.commission_va, F.data == "skip:commission_va")
async def skip_comm_va(cb: CallbackQuery, state: FSMContext):
    await _ask_execution_date(cb.message.edit_text, state)
    await cb.answer()


@router.message(ActForm.commission_va)
async def input_comm_va(msg: Message, state: FSMContext):
    await state.update_data(commission_va=msg.text.strip())
    await _ask_execution_date(msg.answer, state)


async def _ask_execution_date(answer_fn, state):
    data = await state.get_data()
    deal_date = data.get("deal_date", TODAY())
    await state.update_data(execution_date=deal_date)
    await state.set_state(ActForm.execution_date)
    await answer_fn(
        f"<b>Шаг: Дата исполнения акта</b>\n"
        f"По умолчанию = дата заявки: <b>{deal_date}</b>\n\n"
        f"Введите другую дату или нажмите «Использовать»:",
        reply_markup=skip_kb("skip:execution_date", f"= дата заявки ({deal_date})"),
        parse_mode="HTML"
    )


# ─── Step 16: Execution date ──────────────────────────────────────────────────

@router.callback_query(ActForm.execution_date, F.data == "skip:execution_date")
async def skip_exec_date(cb: CallbackQuery, state: FSMContext):
    await _show_confirm(cb.message.edit_text, state)
    await cb.answer()


@router.message(ActForm.execution_date)
async def input_exec_date(msg: Message, state: FSMContext):
    val = msg.text.strip()
    await state.update_data(execution_date=val)
    await _show_confirm(msg.answer, state)


async def _show_confirm(answer_fn, state: FSMContext):
    await state.set_state(ActForm.confirm)
    data = await state.get_data()
    client = await db.get_company(data["company_id"])
    await answer_fn(
        deal_summary(data, client),
        reply_markup=confirm_act_kb(),
        parse_mode="HTML"
    )


# ─── Confirmation callbacks ───────────────────────────────────────────────────

@router.callback_query(ActForm.confirm, F.data == "act:confirm_show")
async def reshow_confirm(cb: CallbackQuery, state: FSMContext):
    data = await state.get_data()
    client = await db.get_company(data["company_id"])
    await cb.message.edit_text(
        deal_summary(data, client),
        reply_markup=confirm_act_kb(),
        parse_mode="HTML"
    )
    await cb.answer()


@router.callback_query(ActForm.confirm, F.data == "act:edit_menu")
async def show_edit_menu(cb: CallbackQuery, state: FSMContext):
    await state.set_state(ActForm.edit_field)
    await cb.message.edit_text(
        "Выберите поле для изменения:",
        reply_markup=edit_act_fields_kb()
    )
    await cb.answer()


@router.callback_query(ActForm.edit_field, F.data.startswith("edit_act_field:"))
async def edit_act_field(cb: CallbackQuery, state: FSMContext):
    fkey = cb.data.split(":", 1)[1]
    label = FIELD_LABELS.get(fkey, fkey)
    data = await state.get_data()
    current = data.get(fkey, "")
    await state.update_data(editing_field=fkey)
    await state.set_state(ActForm.confirm)  # temporarily back to confirm to catch next message
    # Re-use a generic message state
    await state.set_state(ActForm.edit_field)
    await cb.message.edit_text(
        f"✏️ <b>{label}</b>\n\nТекущее: <code>{current}</code>\n\nВведите новое значение:",
        parse_mode="HTML"
    )
    await cb.answer()


@router.message(ActForm.edit_field)
async def save_edited_field(msg: Message, state: FSMContext):
    data = await state.get_data()
    fkey = data.get("editing_field")
    if fkey:
        await state.update_data(**{fkey: msg.text.strip()})
    await _show_confirm(msg.answer, state)


# ─── Generate ─────────────────────────────────────────────────────────────────

@router.callback_query(ActForm.confirm, F.data == "act:generate")
async def generate_act(cb: CallbackQuery, state: FSMContext):
    await cb.answer("⏳ Генерирую документы...")
    data   = await state.get_data()
    client = await db.get_company(data["company_id"])
    op     = await db.get_all_settings()

    try:
        status_msg = await cb.message.answer("⏳ Создаю DOCX...")

        docx_path = generate_docx(data, op, client)

        await status_msg.edit_text("⏳ Конвертирую в PDF...")
        try:
            pdf_path = await convert_to_pdf(docx_path)
            has_pdf = True
        except Exception as e:
            has_pdf = False
            pdf_err = str(e)

        act_type  = data.get("act_type","sell")
        doc_word  = "Счёт-заявка" if act_type == "invoice" else "Акт"
        direction = DIRECTION_LABELS.get(act_type, act_type)
        n_tx      = len(_transactions(data))
        tx_line   = f"\nТранзакций: {n_tx}" if act_type != "invoice" and n_tx > 1 else ""
        caption   = (
            f"✅ <b>{doc_word} №{data.get('deal_number')} от {data.get('deal_date')}</b>\n"
            f"Тип: {direction}\n"
            f"Клиент: {client.get('short_name')}{tx_line}"
        )

        await cb.message.answer_document(
            FSInputFile(docx_path, filename=os.path.basename(docx_path)),
            caption=caption + "\n📄 DOCX",
            parse_mode="HTML"
        )

        if has_pdf:
            await cb.message.answer_document(
                FSInputFile(pdf_path, filename=os.path.basename(pdf_path)),
                caption=caption + "\n📑 PDF",
                parse_mode="HTML"
            )
        else:
            await cb.message.answer(
                f"⚠️ PDF не создан (LibreOffice недоступен):\n<code>{pdf_err}</code>",
                parse_mode="HTML"
            )

        await status_msg.delete()
        await state.clear()

    except FileNotFoundError as e:
        await cb.message.answer(
            f"❌ <b>Шаблон не найден</b>\n\n"
            f"Проверьте, что нужный .docx лежит в папке <code>templates/</code>.\n\n"
            f"Детали: {e}",
            parse_mode="HTML"
        )
    except Exception as e:
        await cb.message.answer(
            f"❌ <b>Ошибка генерации</b>\n<code>{e}</code>",
            parse_mode="HTML"
        )


# ─── Reply keyboard shortcut ──────────────────────────────────────────────────

@router.message(F.text == "📋 Создать акт")
async def shortcut_create_act(msg: Message, state: FSMContext):
    await state.clear()
    await state.set_state(ActForm.select_type)
    await msg.answer(
        "📋 <b>Создание акта</b>\n\nВыберите тип операции:",
        reply_markup=act_type_kb(),
        parse_mode="HTML"
    )
