"""FSM-диалог для генерации документов Hodler."""

import os, logging, asyncio
from datetime import datetime

from aiogram import Dispatcher, F, Router
from aiogram.filters import CommandStart, Command
from aiogram.fsm.context import FSMContext
from aiogram.types import Message, CallbackQuery, FSInputFile

from .states import DealFSM
from .keyboards import doc_type_kb, deal_type_kb, clients_kb, confirm_kb, save_client_kb
from . import clients as db
from . import generator

log = logging.getLogger(__name__)
router = Router()

OWNER_ID = int(os.environ.get("OWNER_ID", "0"))

def _allowed(user_id: int) -> bool:
    return OWNER_ID == 0 or user_id == OWNER_ID


# ─── Утилиты ──────────────────────────────────────────────────────────────────

def _fmt_summary(data: dict) -> str:
    c    = data.get("client", {})
    dt   = "BUY (продаём клиенту)" if data.get("deal_type") == "buy" else "SELL (покупаем у клиента)"
    doc  = "По договору" if data.get("doc_type") == "contract" else "По публичной оферте"
    name = f"{c.get('name_eng')} / {c.get('name_ru')}" if c.get("name_eng") else c.get("name_ru", "")

    lines = [
        f"<b>Тип документа:</b> {doc}",
        f"<b>Тип сделки:</b> {dt}",
        f"<b>Договор:</b> {data.get('contract_id')} от {data.get('contract_date')}",
        f"<b>Исполнение:</b> {data.get('execution_date')}",
        "",
        f"<b>Клиент:</b> {name}",
        f"<b>ИНН:</b> {c.get('inn')}  <b>КПП:</b> {c.get('kpp', '—')}",
        f"<b>Рег. номер:</b> {c.get('reg_number', '—')}",
        f"<b>Адрес:</b> {c.get('address')}",
        f"<b>Банк:</b> {c.get('bank_name')}  БИК {c.get('bank_bik')}",
        f"<b>Счёт:</b> {c.get('bank_account')}",
        "",
        f"<b>USDT:</b> {data.get('usdt_amount')}",
        f"<b>RUB:</b>  {data.get('rub_amount')}",
        f"<b>Курс:</b> {data.get('exchange_rate')}",
        f"<b>КВВО:</b> {data.get('kvvo')}",
        "",
        f"<b>Кошелёк клиента:</b>\n<code>{data.get('client_wallet')}</code>",
        f"<b>Кошелёк оператора:</b>\n<code>{data.get('operator_wallet')}</code>",
        f"<b>TX Hash:</b>\n<code>{data.get('tx_hash', '-')}</code>",
    ]
    if data.get("doc_type") == "contract":
        lines.append(f"\n<b>Договор-основание:</b>\n{data.get('license_contract')}")
    lines.append(f"\n<b>Назначение платежа:</b>\n{data.get('payment_purpose')}")
    return "\n".join(lines)


def _today() -> str:
    return datetime.today().strftime("%d.%m.%Y")


# ─── /start и /cancel ─────────────────────────────────────────────────────────

@router.message(CommandStart())
async def cmd_start(message: Message, state: FSMContext):
    if not _allowed(message.from_user.id):
        return
    await state.clear()
    await message.answer(
        "👋 <b>Hodler Deal Docs</b>\n\n"
        "/new — создать документы по сделке\n"
        "/clients — список клиентов\n"
        "/cancel — отменить",
        parse_mode="HTML",
    )


@router.message(Command("cancel"))
async def cmd_cancel(message: Message, state: FSMContext):
    await state.clear()
    await message.answer("❌ Отменено. /new — начать заново.")


@router.message(Command("clients"))
async def cmd_clients(message: Message):
    if not _allowed(message.from_user.id):
        return
    all_clients = db.list_clients()
    if not all_clients:
        await message.answer("📭 База клиентов пуста.")
        return
    lines = ["<b>Клиенты в базе:</b>\n"]
    for c in all_clients:
        name = f"{c['name_eng']} / {c['name_ru']}" if c.get("name_eng") else c["name_ru"]
        lines.append(f"• <b>{name}</b>\n  ИНН {c['inn']} | {c['bank_name']}")
    await message.answer("\n".join(lines), parse_mode="HTML")


# ─── /new ─────────────────────────────────────────────────────────────────────

@router.message(Command("new"))
async def cmd_new(message: Message, state: FSMContext):
    if not _allowed(message.from_user.id):
        await message.answer("⛔️ Нет доступа.")
        return
    await state.clear()
    await message.answer(
        "📋 <b>Шаг 1 — Тип документа</b>\n\nНа основании чего оформляем?",
        reply_markup=doc_type_kb(),
        parse_mode="HTML",
    )
    await state.set_state(DealFSM.doc_type)


@router.callback_query(DealFSM.doc_type, F.data.in_({"doc_contract", "doc_offer"}))
async def on_doc_type(call: CallbackQuery, state: FSMContext):
    doc_type = "contract" if call.data == "doc_contract" else "offer"
    await state.update_data(doc_type=doc_type)
    await call.message.edit_reply_markup()
    await call.answer()
    await call.message.answer(
        "📋 <b>Шаг 2 — Направление сделки</b>",
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
    await _show_client_picker(call.message, state)


# ─── Выбор клиента ────────────────────────────────────────────────────────────

async def _show_client_picker(message: Message, state: FSMContext):
    all_clients = db.list_clients()
    await message.answer(
        "🏢 <b>Шаг 3 — Клиент</b>",
        reply_markup=clients_kb(all_clients),
        parse_mode="HTML",
    )
    await state.set_state(DealFSM.client_pick)


@router.callback_query(DealFSM.client_pick, F.data.startswith("client_"))
async def on_client_pick(call: CallbackQuery, state: FSMContext):
    await call.message.edit_reply_markup()
    await call.answer()

    if call.data == "client_new":
        await call.message.answer("Введите <b>название на русском</b> (напр. ОсОО «Алтынкопрю»):", parse_mode="HTML")
        await state.set_state(DealFSM.client_name_ru)
        return

    client_id = call.data.removeprefix("client_")
    client = db.get_by_id(client_id)
    if not client:
        await call.message.answer("Клиент не найден, введите вручную.")
        await state.set_state(DealFSM.client_name_ru)
        return

    await state.update_data(client=client)
    name = f"{client['name_eng']} / {client['name_ru']}" if client.get("name_eng") else client["name_ru"]
    await call.message.answer(f"✅ Клиент: <b>{name}</b>", parse_mode="HTML")
    await _ask_contract_id(call.message, state)


# ─── Ввод нового клиента ──────────────────────────────────────────────────────

@router.message(DealFSM.client_name_ru)
async def on_name_ru(message: Message, state: FSMContext):
    await state.update_data(c_name_ru=message.text.strip())
    await message.answer(
        "Название на <b>английском</b> (напр. ALTYNKOPRY LLC)\n"
        "Для российских юрлиц — отправьте <b>-</b>",
        parse_mode="HTML"
    )
    await state.set_state(DealFSM.client_name_eng)

@router.message(DealFSM.client_name_eng)
async def on_name_eng(message: Message, state: FSMContext):
    val = "" if message.text.strip() == "-" else message.text.strip()
    await state.update_data(c_name_eng=val)
    await message.answer("<b>ИНН</b>:", parse_mode="HTML")
    await state.set_state(DealFSM.client_inn)

@router.message(DealFSM.client_inn)
async def on_inn(message: Message, state: FSMContext):
    await state.update_data(c_inn=message.text.strip())
    await message.answer("<b>КПП</b> (или <b>-</b> если нет):", parse_mode="HTML")
    await state.set_state(DealFSM.client_kpp)

@router.message(DealFSM.client_kpp)
async def on_kpp(message: Message, state: FSMContext):
    val = "" if message.text.strip() == "-" else message.text.strip()
    await state.update_data(c_kpp=val)
    await message.answer(
        "<b>Рег. номер</b> (ОГРН для РФ, рег. номер КР для Кыргызстана):",
        parse_mode="HTML"
    )
    await state.set_state(DealFSM.client_reg_number)

@router.message(DealFSM.client_reg_number)
async def on_reg(message: Message, state: FSMContext):
    await state.update_data(c_reg=message.text.strip())
    await message.answer("<b>Юридический адрес</b>:", parse_mode="HTML")
    await state.set_state(DealFSM.client_address)

@router.message(DealFSM.client_address)
async def on_address(message: Message, state: FSMContext):
    await state.update_data(c_address=message.text.strip())
    await message.answer("<b>Название банка</b>:", parse_mode="HTML")
    await state.set_state(DealFSM.client_bank_name)

@router.message(DealFSM.client_bank_name)
async def on_bank_name(message: Message, state: FSMContext):
    await state.update_data(c_bank_name=message.text.strip())
    await message.answer("<b>Номер счёта</b> (р/с или Сч.№):", parse_mode="HTML")
    await state.set_state(DealFSM.client_bank_account)

@router.message(DealFSM.client_bank_account)
async def on_bank_account(message: Message, state: FSMContext):
    await state.update_data(c_bank_account=message.text.strip())
    await message.answer("<b>Корр. счёт к/с</b> (или <b>-</b> если нет):", parse_mode="HTML")
    await state.set_state(DealFSM.client_bank_ks)

@router.message(DealFSM.client_bank_ks)
async def on_bank_ks(message: Message, state: FSMContext):
    val = "" if message.text.strip() == "-" else message.text.strip()
    await state.update_data(c_bank_ks=val)
    await message.answer("<b>БИК</b>:", parse_mode="HTML")
    await state.set_state(DealFSM.client_bank_bik)

@router.message(DealFSM.client_bank_bik)
async def on_bank_bik(message: Message, state: FSMContext):
    d = await state.get_data()
    await state.update_data(c_bank_bik=message.text.strip())
    d = await state.get_data()

    client = {
        "id":           d["c_inn"],
        "name_ru":      d["c_name_ru"],
        "name_eng":     d.get("c_name_eng", ""),
        "inn":          d["c_inn"],
        "kpp":          d.get("c_kpp", ""),
        "reg_number":   d.get("c_reg", ""),
        "address":      d["c_address"],
        "bank_name":    d["c_bank_name"],
        "bank_account": d["c_bank_account"],
        "bank_ks":      d.get("c_bank_ks", ""),
        "bank_bik":     d["c_bank_bik"],
    }
    await state.update_data(client=client)
    await message.answer(
        "💾 Сохранить клиента в базу?",
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


# ─── Реквизиты договора ───────────────────────────────────────────────────────

async def _ask_contract_id(message: Message, state: FSMContext):
    await message.answer(
        "📝 <b>Шаг 4 — Договор</b>\n\nВведите <b>номер договора</b> (напр. №01-EMPL):",
        parse_mode="HTML",
    )
    await state.set_state(DealFSM.contract_id)


@router.message(DealFSM.contract_id)
async def on_contract_id(message: Message, state: FSMContext):
    await state.update_data(contract_id=message.text.strip())
    await message.answer(f"<b>Дата договора</b> ДД.ММ.ГГГГ [{_today()}]:", parse_mode="HTML")
    await state.set_state(DealFSM.contract_date)

@router.message(DealFSM.contract_date)
async def on_contract_date(message: Message, state: FSMContext):
    val = message.text.strip() or _today()
    await state.update_data(contract_date=val)
    await message.answer(f"<b>Дата исполнения</b> ДД.ММ.ГГГГ [{val}]:", parse_mode="HTML")
    await state.set_state(DealFSM.execution_date)

@router.message(DealFSM.execution_date)
async def on_execution_date(message: Message, state: FSMContext):
    d = await state.get_data()
    val = message.text.strip() or d.get("contract_date", _today())
    await state.update_data(execution_date=val)

    # Только для типа "contract" спрашиваем договор-основание
    if d.get("doc_type") == "contract":
        await message.answer(
            "📄 Введите <b>договор-основание</b>\n(напр. ЛИЦЕНЗИОННЫЙ ДОГОВОР №1 от 31-03-2026 НА ИСПОЛЬЗОВАНИЕ ПРОГРАММЫ ДЛЯ ЭВМ):",
            parse_mode="HTML",
        )
        await state.set_state(DealFSM.license_contract)
    else:
        await _ask_deal_params(message, state)


@router.message(DealFSM.license_contract)
async def on_license(message: Message, state: FSMContext):
    await state.update_data(license_contract=message.text.strip())
    await _ask_deal_params(message, state)


# ─── Параметры сделки ────────────────────────────────────────────────────────

async def _ask_deal_params(message: Message, state: FSMContext):
    await message.answer(
        "💰 <b>Шаг 5 — Параметры сделки</b>\n\n"
        "Введите <b>сумму USDT</b> (напр. 14 000)\n"
        "Или <b>-</b> чтобы посчитать из RUB ÷ курс:",
        parse_mode="HTML",
    )
    await state.set_state(DealFSM.usdt_amount)


@router.message(DealFSM.usdt_amount)
async def on_usdt(message: Message, state: FSMContext):
    val = "" if message.text.strip() == "-" else message.text.strip()
    await state.update_data(usdt_amount=val)
    await message.answer(
        "<b>Сумма в рублях</b> (напр. 1 350 000)\n"
        "Или <b>-</b> чтобы посчитать автоматически из USDT × курс:",
        parse_mode="HTML"
    )
    await state.set_state(DealFSM.rub_amount)

@router.message(DealFSM.rub_amount)
async def on_rub(message: Message, state: FSMContext):
    val = "" if message.text.strip() == "-" else message.text.strip()
    await state.update_data(rub_amount=val)
    await message.answer(
        "<b>Курс обмена</b> (напр. 80,88)\n"
        "Или <b>-</b> чтобы посчитать автоматически из RUB ÷ USDT:",
        parse_mode="HTML"
    )
    await state.set_state(DealFSM.exchange_rate)

@router.message(DealFSM.exchange_rate)
async def on_rate(message: Message, state: FSMContext):
    from .generator import auto_calculate
    val = "" if message.text.strip() == "-" else message.text.strip()
    await state.update_data(exchange_rate=val)
    d = await state.get_data()

    # Считаем недостающее
    calc = auto_calculate({
        'usdt_amount':  d.get('usdt_amount', ''),
        'rub_amount':   d.get('rub_amount', ''),
        'exchange_rate': val,
    })
    await state.update_data(
        usdt_amount=calc.get('usdt_amount', d.get('usdt_amount', '')),
        rub_amount=calc.get('rub_amount', d.get('rub_amount', '')),
        exchange_rate=calc.get('exchange_rate', val),
    )
    d = await state.get_data()
    await message.answer(
        f"✅ Суммы:\n"
        f"  USDT: <b>{d.get('usdt_amount')}</b>\n"
        f"  RUB:  <b>{d.get('rub_amount')}</b>\n"
        f"  Курс: <b>{d.get('exchange_rate')}</b>",
        parse_mode="HTML"
    )
    await message.answer("<b>Кошелёк клиента</b> TRC-20:", parse_mode="HTML")
    await state.set_state(DealFSM.client_wallet)

@router.message(DealFSM.client_wallet)
async def on_client_wallet(message: Message, state: FSMContext):
    await state.update_data(client_wallet=message.text.strip())
    await message.answer("<b>Кошелёк оператора</b> TRC-20:", parse_mode="HTML")
    await state.set_state(DealFSM.operator_wallet)

@router.message(DealFSM.operator_wallet)
async def on_operator_wallet(message: Message, state: FSMContext):
    await state.update_data(operator_wallet=message.text.strip())
    await message.answer("<b>Хэш транзакции</b> (или <b>-</b> если ещё нет):", parse_mode="HTML")
    await state.set_state(DealFSM.tx_hash)

@router.message(DealFSM.tx_hash)
async def on_tx_hash(message: Message, state: FSMContext):
    await state.update_data(tx_hash=message.text.strip() or "-")
    d = await state.get_data()

    # КВВО: для оферты дефолт VO99085, для договора VO20200
    default_kvvo = "VO99085" if d.get("doc_type") == "offer" else "VO20200"
    await message.answer(
        f"📄 <b>Шаг 6 — Платёж</b>\n\n<b>КВВО</b> [{default_kvvo}]:",
        parse_mode="HTML",
    )
    await state.set_state(DealFSM.kvvo)

@router.message(DealFSM.kvvo)
async def on_kvvo(message: Message, state: FSMContext):
    d = await state.get_data()
    default_kvvo = "VO99085" if d.get("doc_type") == "offer" else "VO20200"
    val = message.text.strip() or default_kvvo
    await state.update_data(kvvo=val)

    # Для оферты назначение генерируется автоматически, спросим только подтверждение
    if d.get("doc_type") == "offer":
        cid  = d.get("contract_id", "")
        date = d.get("contract_date", "")
        auto = f"За виртуальный актив по заявке {cid} от {date} согласно Соглашения https://hodlerexchange.io/home/documents. НДС не облагается"
        await state.update_data(payment_purpose=auto)
        await message.answer(
            f"<b>Назначение платежа</b> (авто):\n<i>{auto}</i>\n\n"
            f"Отправьте <b>ок</b> чтобы оставить или введите своё:",
            parse_mode="HTML",
        )
    else:
        await message.answer("<b>Назначение платежа</b>:", parse_mode="HTML")

    await state.set_state(DealFSM.payment_purpose)

@router.message(DealFSM.payment_purpose)
async def on_purpose(message: Message, state: FSMContext):
    d = await state.get_data()
    if message.text.strip().lower() not in ("ок", "ok"):
        await state.update_data(payment_purpose=message.text.strip())

    await message.answer(
        "<b>Тип операции в акте</b>\n[по умолчанию: Покупка виртуальных активов у клиента]:",
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


# ─── Подтверждение ────────────────────────────────────────────────────────────

@router.callback_query(DealFSM.confirm, F.data == "confirm_no")
async def on_no(call: CallbackQuery, state: FSMContext):
    await call.message.edit_reply_markup()
    await call.answer()
    await state.clear()
    await call.message.answer("🔄 Начните заново: /new")


@router.callback_query(DealFSM.confirm, F.data == "edit_client")
async def on_edit_client(call: CallbackQuery, state: FSMContext):
    await call.message.edit_reply_markup()
    await call.answer()
    await _show_client_picker(call.message, state)


@router.callback_query(DealFSM.confirm, F.data == "confirm_yes")
async def on_yes(call: CallbackQuery, state: FSMContext):
    await call.message.edit_reply_markup()
    await call.answer()

    data = await state.get_data()
    await state.clear()

    wait = await call.message.answer("⏳ Генерирую документы...")
    try:
        loop = asyncio.get_event_loop()
        docx_path = await loop.run_in_executor(None, generator.generate_docx, data)
        pdf_path  = await loop.run_in_executor(None, generator.generate_pdf, docx_path)

        await wait.delete()

        docx_file = FSInputFile(docx_path, filename=os.path.basename(docx_path))
        await call.message.answer_document(docx_file, caption="📄 DOCX")

        if pdf_path and os.path.exists(pdf_path):
            pdf_file = FSInputFile(pdf_path, filename=os.path.basename(pdf_path))
            await call.message.answer_document(pdf_file, caption="📕 PDF")
        else:
            await call.message.answer(
                "⚠️ PDF не создан — LibreOffice не установлен или ошибка конвертации."
            )

    except Exception as e:
        log.exception("Ошибка генерации")
        await wait.delete()
        await call.message.answer(f"❌ Ошибка:\n<code>{e}</code>", parse_mode="HTML")


def register(dp: Dispatcher):
    dp.include_router(router)