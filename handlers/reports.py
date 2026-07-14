"""
Отчёт для Финнадзора: выбор месяца → курс RUB→KGS → XLSX c листами
«Продажа» (Приложение 4/о) и «Покупка» (Приложение 5/о).
"""
import asyncio
import os
import uuid

from aiogram import Router, F
from aiogram.types import Message, CallbackQuery, FSInputFile
from aiogram.fsm.context import FSMContext

import database as db
from config import OUTPUT_DIR
from states import ReportForm
from keyboards import report_months_kb, report_rate_kb, back_to_main_kb
from services.report_xlsx import build_fn_report, month_label
from services.doc_generator import _to_float
from services.cleanup import try_remove

router = Router()


async def _show_months(answer_fn):
    months = await db.get_deal_months()
    if not months:
        await answer_fn(
            "📊 <b>Отчёт ФН</b>\n\nВ журнале пока нет сделок — "
            "создайте первый акт, и он попадёт в отчёт автоматически.",
            reply_markup=back_to_main_kb(),
            parse_mode="HTML"
        )
        return
    labels = {m: month_label(m) for m in months}
    await answer_fn(
        "📊 <b>Отчёт ФН</b>\n\nВыберите месяц для выгрузки:",
        reply_markup=report_months_kb(months, labels),
        parse_mode="HTML"
    )


@router.callback_query(F.data == "report:menu")
async def report_menu(cb: CallbackQuery, state: FSMContext):
    await state.clear()
    await _show_months(cb.message.edit_text)
    await cb.answer()


@router.message(F.text == "📊 Отчёт ФН")
async def report_menu_text(msg: Message, state: FSMContext):
    await state.clear()
    await _show_months(msg.answer)


@router.callback_query(F.data.startswith("report_month:"))
async def report_pick_month(cb: CallbackQuery, state: FSMContext):
    month = cb.data.split(":")[1]
    settings = await db.get_all_settings()
    saved = settings.get("kgs_rate", "")
    await state.set_state(ReportForm.kgs_rate)
    await state.update_data(report_month=month)
    await cb.message.edit_text(
        f"📅 Месяц: <b>{month_label(month)}</b>\n\n"
        f"<b>Курс RUB → KGS</b> для колонки «Объём в сомах»\n"
        f"Введите курс (например: 1.2072):",
        reply_markup=report_rate_kb(saved or None),
        parse_mode="HTML"
    )
    await cb.answer()


async def _generate_report(answer_fn, bot_send_document, state: FSMContext, rate: float):
    data = await state.get_data()
    month = data.get("report_month", "")
    await state.clear()

    deals = await db.get_deals_for_month(month)
    reportable = [d for d in deals if d.get("act_type") in ("sell", "buy")]
    if not reportable:
        await answer_fn(
            f"За {month_label(month)} нет актов продажи/покупки "
            f"(счета-заявки в отчёт не входят).",
            reply_markup=back_to_main_kb()
        )
        return

    os.makedirs(OUTPUT_DIR, exist_ok=True)
    fname = f"ФН_{month_label(month).replace(' ', '_')}.xlsx"
    # uuid в пути: два одновременных запроса отчёта не мешают друг другу;
    # openpyxl синхронный — собираем в потоке, чтобы не блокировать бота
    path = os.path.join(OUTPUT_DIR, f"fn_report_{month}_{uuid.uuid4().hex[:8]}.xlsx")
    await asyncio.to_thread(build_fn_report, path, month, reportable, rate)

    n_sell = sum(1 for d in reportable if d["act_type"] == "sell")
    n_buy = len(reportable) - n_sell
    try:
        await bot_send_document(
            FSInputFile(path, filename=fname),
            caption=(f"📊 Отчёт ФН за {month_label(month)}\n"
                     f"Продажа: {n_sell} сделок · Покупка: {n_buy} сделок"
                     + (f"\nКурс RUB→KGS: {rate}" if rate else "\nБез пересчёта в сомы"))
        )
    finally:
        try_remove(path)


@router.callback_query(ReportForm.kgs_rate, F.data == "report_rate:saved")
async def report_rate_saved(cb: CallbackQuery, state: FSMContext):
    settings = await db.get_all_settings()
    rate = _to_float(settings.get("kgs_rate", "0"))
    await cb.answer()
    await _generate_report(cb.message.answer, cb.message.answer_document, state, rate)


@router.callback_query(ReportForm.kgs_rate, F.data == "report_rate:skip")
async def report_rate_skip(cb: CallbackQuery, state: FSMContext):
    await cb.answer()
    await _generate_report(cb.message.answer, cb.message.answer_document, state, 0.0)


@router.message(ReportForm.kgs_rate)
async def report_rate_input(msg: Message, state: FSMContext):
    rate = _to_float(msg.text)
    if rate <= 0:
        await msg.answer("❌ Введите число, например: 1.2072")
        return
    await db.set_setting("kgs_rate", str(rate))  # запомним на следующий раз
    await _generate_report(msg.answer, msg.answer_document, state, rate)
