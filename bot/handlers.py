"""
Все хэндлеры бота. FSM-диалог для генерации документов.
"""

import os
import logging
import asyncio
from datetime import datetime

from aiogram import Dispatcher, F, Router
from aiogram.filters import CommandStart, Command, StateFilter
from aiogram.fsm.context import FSMContext
from aiogram.types import Message, CallbackQuery, FSInputFile

from .states import DealFSM
from .keyboards import deal_type_kb, clients_kb, confirm_kb, save_client_kb, cancel_kb
from . import clients as db
from . import generator

log = logging.getLogger(__name__)

# Whitelist — читается из env, формат: "123456789,987654321"
def _get_whitelist() -> set:
    raw = os.environ.get("ALLOWED_USER_IDS", "")
    return {int(x.strip()) for x in raw.split(",") if x.strip().isdigit()}


def _allowed(user_id: int) -> bool:
    wl = _get_whitelist()
    return not wl or user_id in wl  # если whitelist пуст — пускаем всех


router = Router()

# ─── Утилиты ─────────────────────────────────────────────────────────────────

def _fmt_summary(data: dict) -> str:
    c = data.get("client", {})
    lines = [
        f"<b>Тип сделки:</b> {'BUY (продаём клиенту)' if data.get('deal_type') == 'buy' else 'SELL (покупаем у клиента)'}",
        f"<b>Договор:</b> {data.get('contract_id')} от {data.get('contract_date')}",
        f"<b>Исполнение:</b> {data.get('execution_date')}",
        "",
        f"<b>Клиент:</b> {c.get('name')}",
        f"<b>ИНН/КПП:</b> {c.get('inn')} / {c.get('kpp')}",
        f"<b>ОГРН:</b> {c.get('ogrn')}",
        f"<b>Банк:</b> {c.get('bank_name')}",
        f"<b>р/с:</b> {c.get('bank_account')}",
        "",
        f"<b>USDT:</b> {data.get('usdt_amount')}",
        f"<b>RUB:</b>  {data.get('rub_amount')}",
        f"<b>Курс:</b> {data.get('exchange_rate')}",
        f"<b>КВВО:</b> {data.get('kvvo')}",
        "",
        f"<b>Кошелёк клиента:</b>\n<code>{data.get('client_wallet')}</code>",
        f"<b>Кошелёк оператора:</b>\n<code>{data.get('operator_wallet')}</code>",
        f"<b>TX Hash:</b>\n<code>{data.get('tx_hash')}</code>",
        "",
        f"<b>Договор-основание:</b>\n{data.get('license_contract')}",
    ]
    return "\n".join(lines)


async def _ask(message: Message, text: str, state: FSMContext, next_state, default: str = None):
    hint = f" [по умолчанию: <i>{default}</i>]" if default else ""
    await message.answer(text + hint + "\n\nОтправьте /cancel чтобы отменить.", parse_mode="HTML")
    await state.set_state(next_state)


# ─── Гарды ───────────────────────────────────────────────────────────────────

@router.message(CommandStart())
async def cmd_start(message: Message, state: FSMContext):
    if not _allowed(message.from_user.id):
        await message.answer("⛔️ У вас нет доступа к этому боту.")
        return
    await state.clear()
    await message.answer(
        "👋 <b>Hodler Deal Docs</b>\n\n"
        "Генерирую закрывающие документы для сделок.\n\n"
        "Команды:\n"
        "/new — создать новый документ\n"
        "/clients — список сохранённых клиентов\n"
        "/cancel — отменить текущий диалог",
        parse_mode="HTML",
    )


@router.message(Command("cancel"))
@router.message(F.text == "❌ Отмена")
async def cmd_cancel(message: Message, state: FSMContext):
    await state.clear()
    await message.answer("❌ Отменено. /new — начать заново.", reply_markup=None)


@router.message(Command("clients"))
async def cmd_clients(message: Message):
    if not _allowed(message.from_user.id):
        return
    all_clients = db.list_clients()
    if not all_clients:
        await message.answer("📭 База клиентов пуста. Они сохраняются при создании документов.")
        return
    lines = ["<b>Сохранённые клиенты:</b>\n"]
    for c in all_clients:
        lines.append(f"• <b>{c['name']}</b>\n  ИНН {c['inn']} | {c['bank_name']}")
    await message.answer("\n".join(lines), parse_mode="HTML")


# ─── /new — начало диалога ────────────────────────────────────────────────────

@router.message(Command("new"))
async def cmd_new(message: Message, state: FSMContext):
    if not _allowed(message.from_user.id):
        await message.answer("⛔️ У вас нет доступа.")
        return
    await state.clear()
    await message.answer(
        "📋 <b>Шаг 1/5 — Тип сделки</b>\n\nВыберите направление:",
        reply_markup=deal_type_kb(),
        parse_mode="HTML",
    )
    await state.set_state(DealFSM.deal_type)


@router.callback_query(DealFSM.deal_type, F.data.in_({"deal_buy", "deal_sell"}))
async def on_deal_type(call: CallbackQuery, state: FSMContext):
    deal_type = "buy" if call.data == "deal_buy" else "sell"
    await state.update_data(deal_type=deal_type)
    await call.message.edit_reply_markup()
    await call.answer()

    all_clients = db.list_clients()
    if all_clients:
        await call.message.answer(
            "🏢 <b>Шаг 2/5 — Клиент</b>\n\nВыберите из базы или введите нового:",
            reply_markup=clients_kb(all_clients),
            parse_mode="HTML",
        )
        await state.set_state(DealFSM.client_pick)
    else:
        await call.message.answer("🏢 <b>Шаг 2/5 — Клиент</b>\n\nВведите название клиента:", parse_mode="HTML")
        await state.set_state(DealFSM.client_name)


# ─── Выбор клиента ───────────────────────────────────────────────────────────

@router.callback_query(DealFSM.client_pick, F.data.startswith("client_"))
async def on_client_pick(call: CallbackQuery, state: FSMContext):
    await call.message.edit_reply_markup()
    await call.answer()

    if call.data == "client_new":
        await call.message.answer("Введите <b>название</b> клиента:", parse_mode="HTML")
        await state.set_state(DealFSM.client_name)
        return

    inn = call.data.removeprefix("client_")
    client = db.get_by_inn(inn)
    if not client:
        await call.message.answer("Клиент не найден, введите вручную.")
        await state.set_state(DealFSM.client_name)
        return

    await state.update_data(client=client)
    await call.message.answer(f"✅ Клиент: <b>{client['name']}</b>", parse_mode="HTML")
    await _ask_contract_id(call.message, state)


# ─── Ввод нового клиента ──────────────────────────────────────────────────────

@router.message(DealFSM.client_name)
async def on_client_name(message: Message, state: FSMContext):
    await state.update_data(client_name=message.text.strip())
    await message.answer("Введите <b>адрес</b> клиента (без «Местонахождение:»):", parse_mode="HTML")
    await state.set_state(DealFSM.client_address)

@router.message(DealFSM.client_address)
async def on_client_address(message: Message, state: FSMContext):
    await state.update_data(client_address=message.text.strip())
    await message.answer("Введите <b>ОГРН</b>:", parse_mode="HTML")
    await state.set_state(DealFSM.client_ogrn)

@router.message(DealFSM.client_ogrn)
async def on_client_ogrn(message: Message, state: FSMContext):
    await state.update_data(client_ogrn=message.text.strip())
    await message.answer("Введите <b>ИНН</b>:", parse_mode="HTML")
    await state.set_state(DealFSM.client_inn)

@router.message(DealFSM.client_inn)
async def on_client_inn(message: Message, state: FSMContext):
    await state.update_data(client_inn=message.text.strip())
    await message.answer("Введите <b>КПП</b>:", parse_mode="HTML")
    await state.set_state(DealFSM.client_kpp)

@router.message(DealFSM.client_kpp)
async def on_client_kpp(message: Message, state: FSMContext):
    await state.update_data(client_kpp=message.text.strip())
    await message.answer("Введите <b>расчётный счёт р/с</b>:", parse_mode="HTML")
    await state.set_state(DealFSM.client_bank_account)

@router.message(DealFSM.client_bank_account)
async def on_bank_account(message: Message, state: FSMContext):
    await state.update_data(client_bank_account=message.text.strip())
    await message.answer("Введите <b>название банка</b>:", parse_mode="HTML")
    await state.set_state(DealFSM.client_bank_name)

@router.message(DealFSM.client_bank_name)
async def on_bank_name(message: Message, state: FSMContext):
    await state.update_data(client_bank_name=message.text.strip())
    await message.answer("Введите <b>корр. счёт к/с</b>:", parse_mode="HTML")
    await state.set_state(DealFSM.client_bank_ks)

@router.message(DealFSM.client_bank_ks)
async def on_bank_ks(message: Message, state: FSMContext):
    await state.update_data(client_bank_ks=message.text.strip())
    await message.answer("Введите <b>БИК</b>:", parse_mode="HTML")
    await state.set_state(DealFSM.client_bank_bik)

@router.message(DealFSM.client_bank_bik)
async def on_bank_bik(message: Message, state: FSMContext):
    d = await state.get_data()
    await state.update_data(client_bank_bik=message.text.strip())
    d = await state.get_data()

    client = {
        "name":         d["client_name"],
        "address":      d["client_address"],
        "ogrn":         d["client_ogrn"],
        "inn":          d["client_inn"],
        "kpp":          d["client_kpp"],
        "bank_account": d["client_bank_account"],
        "bank_name":    d["client_bank_name"],
        "bank_ks":      d["client_bank_ks"],
        "bank_bik":     d["client_bank_bik"],
    }
    await state.update_data(client=client)

    await message.answer(
        "💾 Сохранить клиента в базу для следующего раза?",
        reply_markup=save_client_kb(),
    )
    await state.set_state(DealFSM.client_save)


@router.callback_query(DealFSM.client_save, F.data.in_({"save_yes", "save_no"}))
async def on_save_client(call: CallbackQuery, state: FSMContext):
    await call.message.edit_reply_markup()
    await call.answer()
    if call.data == "save_yes":
        d = await state.get_data()
        db.add(d["client"])
        await call.message.answer("✅ Клиент сохранён.")
    await _ask_contract_id(call.message, state)


# ─── Договор ─────────────────────────────────────────────────────────────────

async def _ask_contract_id(message: Message, state: FSMContext):
    await message.answer(
        "📝 <b>Шаг 3/5 — Договор</b>\n\nВведите <b>номер договора</b> (напр. №01-EMPL):",
        parse_mode="HTML",
    )
    await state.set_state(DealFSM.contract_id)


@router.message(DealFSM.contract_id)
async def on_contract_id(message: Message, state: FSMContext):
    await state.update_data(contract_id=message.text.strip())
    today = datetime.today().strftime("%d.%m.%Y")
    await message.answer(f"Введите <b>дату договора</b> в формате ДД.ММ.ГГГГ [по умолчанию: {today}]:", parse_mode="HTML")
    await state.set_state(DealFSM.contract_date)


@router.message(DealFSM.contract_date)
async def on_contract_date(message: Message, state: FSMContext):
    val = message.text.strip() or datetime.today().strftime("%d.%m.%Y")
    await state.update_data(contract_date=val)
    await message.answer(f"Введите <b>дату исполнения</b> в формате ДД.ММ.ГГГГ [по умолчанию: {val}]:", parse_mode="HTML")
    await state.set_state(DealFSM.execution_date)


@router.message(DealFSM.execution_date)
async def on_execution_date(message: Message, state: FSMContext):
    d = await state.get_data()
    val = message.text.strip() or d.get("contract_date", "")
    await state.update_data(execution_date=val)
    await message.answer(
        "💰 <b>Шаг 4/5 — Параметры сделки</b>\n\nВведите <b>сумму USDT</b> (напр. 140 000):",
        parse_mode="HTML",
    )
    await state.set_state(DealFSM.usdt_amount)


# ─── Параметры сделки ────────────────────────────────────────────────────────

@router.message(DealFSM.usdt_amount)
async def on_usdt(message: Message, state: FSMContext):
    await state.update_data(usdt_amount=message.text.strip())
    await message.answer("Введите <b>сумму в рублях</b> (напр. 11 091 730,38):", parse_mode="HTML")
    await state.set_state(DealFSM.rub_amount)

@router.message(DealFSM.rub_amount)
async def on_rub(message: Message, state: FSMContext):
    await state.update_data(rub_amount=message.text.strip())
    await message.answer("Введите <b>курс обмена</b> (напр. 79,22):", parse_mode="HTML")
    await state.set_state(DealFSM.exchange_rate)

@router.message(DealFSM.exchange_rate)
async def on_rate(message: Message, state: FSMContext):
    await state.update_data(exchange_rate=message.text.strip())
    await message.answer("Введите <b>адрес кошелька клиента</b> (TRC-20):", parse_mode="HTML")
    await state.set_state(DealFSM.client_wallet)

@router.message(DealFSM.client_wallet)
async def on_client_wallet(message: Message, state: FSMContext):
    await state.update_data(client_wallet=message.text.strip())
    await message.answer("Введите <b>адрес кошелька оператора</b> (TRC-20):", parse_mode="HTML")
    await state.set_state(DealFSM.operator_wallet)

@router.message(DealFSM.operator_wallet)
async def on_operator_wallet(message: Message, state: FSMContext):
    await state.update_data(operator_wallet=message.text.strip())
    await message.answer("Введите <b>хэш транзакции</b> (или - если ещё нет):", parse_mode="HTML")
    await state.set_state(DealFSM.tx_hash)

@router.message(DealFSM.tx_hash)
async def on_tx_hash(message: Message, state: FSMContext):
    await state.update_data(tx_hash=message.text.strip() or "-")
    await message.answer(
        "📄 <b>Шаг 5/5 — Договор и платёж</b>\n\n"
        "Введите <b>КВВО</b> (код вида валютной операции) [по умолчанию: VO20200]:",
        parse_mode="HTML",
    )
    await state.set_state(DealFSM.kvvo)

@router.message(DealFSM.kvvo)
async def on_kvvo(message: Message, state: FSMContext):
    val = message.text.strip() or "VO20200"
    await state.update_data(kvvo=val)
    await message.answer("Введите <b>название договора-основания</b>:", parse_mode="HTML")
    await state.set_state(DealFSM.license_contract)

@router.message(DealFSM.license_contract)
async def on_license(message: Message, state: FSMContext):
    await state.update_data(license_contract=message.text.strip())
    await message.answer("Введите <b>назначение платежа</b> (текст после {КВВО}):", parse_mode="HTML")
    await state.set_state(DealFSM.payment_purpose)

@router.message(DealFSM.payment_purpose)
async def on_purpose(message: Message, state: FSMContext):
    await state.update_data(payment_purpose=message.text.strip())
    await message.answer(
        "Введите <b>тип операции в акте</b>\n"
        "[по умолчанию: Покупка виртуальных активов у клиента]:",
        parse_mode="HTML",
    )
    await state.set_state(DealFSM.act_operation_type)

@router.message(DealFSM.act_operation_type)
async def on_act_op(message: Message, state: FSMContext):
    val = message.text.strip() or "Покупка виртуальных активов у клиента"
    await state.update_data(act_operation_type=val)
    data = await state.get_data()

    await message.answer(
        "📋 <b>Проверьте данные:</b>\n\n" + _fmt_summary(data) + "\n\nВсё верно?",
        reply_markup=confirm_kb(),
        parse_mode="HTML",
    )
    await state.set_state(DealFSM.confirm)


# ─── Подтверждение и генерация ───────────────────────────────────────────────

@router.callback_query(DealFSM.confirm, F.data == "confirm_no")
async def on_confirm_no(call: CallbackQuery, state: FSMContext):
    await call.message.edit_reply_markup()
    await call.answer()
    await state.clear()
    await call.message.answer("🔄 Начните заново: /new")


@router.callback_query(DealFSM.confirm, F.data == "confirm_yes")
async def on_confirm_yes(call: CallbackQuery, state: FSMContext):
    await call.message.edit_reply_markup()
    await call.answer()

    data = await state.get_data()
    await state.clear()

    wait_msg = await call.message.answer("⏳ Генерирую документы...")

    try:
        # Генерация в пуле потоков (blocking IO)
        loop = asyncio.get_event_loop()
        docx_path = await loop.run_in_executor(None, generator.generate_docx, data)
        pdf_path  = await loop.run_in_executor(None, generator.generate_pdf, docx_path)

        await wait_msg.delete()
        await call.message.answer("✅ Готово! Отправляю файлы...")

        # Отправляем DOCX
        docx_file = FSInputFile(docx_path, filename=os.path.basename(docx_path))
        await call.message.answer_document(docx_file, caption="📄 DOCX")

        # Отправляем PDF если есть
        if pdf_path and os.path.exists(pdf_path):
            pdf_file = FSInputFile(pdf_path, filename=os.path.basename(pdf_path))
            await call.message.answer_document(pdf_file, caption="📕 PDF")
        else:
            await call.message.answer("⚠️ PDF не создан (LibreOffice недоступен на сервере).")

    except Exception as e:
        log.exception("Error generating document")
        await wait_msg.delete()
        await call.message.answer(f"❌ Ошибка при генерации:\n<code>{e}</code>", parse_mode="HTML")


# ─── Регистрация ─────────────────────────────────────────────────────────────

def register(dp: Dispatcher):
    dp.include_router(router)
