"""
Журнал сделок: просмотр, повторная отправка документов (по file_id из
Telegram — на диске ничего не хранится), приём подписанных актов и
выгрузка их ZIP-архивом по клиенту.
"""
import asyncio
import json
import os
import uuid
import zipfile

from aiogram import Router, F
from aiogram.types import Message, CallbackQuery, FSInputFile
from aiogram.fsm.context import FSMContext

import database as db
from config import OUTPUT_DIR
from states import SignedForm
from keyboards import (
    deals_menu_kb, deals_list_kb, deal_card_kb, confirm_deal_delete_kb,
    company_pick_kb, signed_done_kb, back_to_main_kb
)
from services.cleanup import try_remove
from utils import esc, safe_name_part

router = Router()

ACT_LABELS = {
    "sell":    "📤 Продажа ВА клиенту",
    "buy":     "📥 Покупка ВА у клиента",
    "invoice": "🧾 Счёт-заявка",
}


def deal_card_text(d: dict, signed_count: int) -> str:
    txs = json.loads(d.get("transactions") or "[]")
    lines = [
        f"<b>{ACT_LABELS.get(d['act_type'], d['act_type'])} №{esc(d.get('deal_number','—'))}</b>",
        f"<b>Дата:</b> {esc(d.get('deal_date','—'))}   <b>Исполнение:</b> {esc(d.get('execution_date','—'))}",
        f"<b>Клиент:</b> {esc(d.get('client_name','—'))} ({esc(d.get('client_resident',''))})",
        "",
        f"<b>Актив:</b> {esc(d.get('va_type','—'))} ({esc(d.get('network','—'))})",
        f"<b>Сумма ВА:</b> {esc(d.get('va_amount','—'))}",
        f"<b>Сумма {esc(d.get('fiat_currency','RUB'))}:</b> {esc(d.get('fiat_amount','—'))}",
        f"<b>Курс:</b> {esc(d.get('exchange_rate','—'))}   <b>КВВО:</b> {esc(d.get('kvvo','—'))}",
    ]
    if len(txs) > 1:
        lines.append(f"<b>Транзакций:</b> {len(txs)}")
    lines.append("")
    lines.append(f"<b>Подписанных документов:</b> {signed_count}")
    return "\n".join(lines)


# ─── Меню журнала ─────────────────────────────────────────────────────────────

async def _show_deals_menu(answer_fn):
    await answer_fn(
        "🗂 <b>Журнал сделок</b>\n\nВсе сгенерированные акты сохраняются здесь. "
        "Документы хранятся в Telegram и доступны в любой момент.",
        reply_markup=deals_menu_kb(),
        parse_mode="HTML"
    )


@router.callback_query(F.data == "deals:menu")
async def deals_menu(cb: CallbackQuery, state: FSMContext):
    await state.clear()
    await _show_deals_menu(cb.message.edit_text)
    await cb.answer()


@router.message(F.text == "🗂 Сделки")
async def deals_menu_text(msg: Message, state: FSMContext):
    await state.clear()
    await _show_deals_menu(msg.answer)


@router.callback_query(F.data == "deals:recent")
async def deals_recent(cb: CallbackQuery):
    deals = await db.get_recent_deals(limit=10)
    if not deals:
        await cb.answer("Журнал пока пуст", show_alert=True)
        return
    await cb.message.edit_text(
        "🕐 <b>Последние сделки:</b>",
        reply_markup=deals_list_kb(deals),
        parse_mode="HTML"
    )
    await cb.answer()


@router.callback_query(F.data == "deals:by_company")
async def deals_by_company(cb: CallbackQuery):
    companies = await db.get_all_companies()
    if not companies:
        await cb.answer("Нет компаний", show_alert=True)
        return
    await cb.message.edit_text(
        "Выберите клиента:",
        reply_markup=company_pick_kb(companies, "deals_c")
    )
    await cb.answer()


@router.callback_query(F.data.startswith("deals_c:"))
async def deals_of_company(cb: CallbackQuery):
    cid = int(cb.data.split(":")[1])
    deals = await db.get_recent_deals(limit=10, company_id=cid)
    if not deals:
        await cb.answer("По этому клиенту сделок нет", show_alert=True)
        return
    await cb.message.edit_text(
        f"🏢 <b>Сделки клиента {deals[0].get('client_name','')}:</b>",
        reply_markup=deals_list_kb(deals),
        parse_mode="HTML"
    )
    await cb.answer()


# ─── Карточка сделки ──────────────────────────────────────────────────────────

@router.callback_query(F.data.startswith("deal:"))
async def deal_card(cb: CallbackQuery, state: FSMContext):
    await state.clear()
    deal_id = int(cb.data.split(":")[1])
    d = await db.get_deal(deal_id)
    if not d:
        await cb.answer("Сделка не найдена", show_alert=True)
        return
    signed = await db.count_signed_docs(deal_id)
    has_files = bool(d.get("docx_file_id") or d.get("pdf_file_id"))
    await cb.message.edit_text(
        deal_card_text(d, signed),
        reply_markup=deal_card_kb(deal_id, has_files, signed),
        parse_mode="HTML"
    )
    await cb.answer()


@router.callback_query(F.data.startswith("deal_docs:"))
async def deal_send_docs(cb: CallbackQuery):
    deal_id = int(cb.data.split(":")[1])
    d = await db.get_deal(deal_id)
    if not d:
        await cb.answer("Сделка не найдена", show_alert=True)
        return
    sent = False
    for fid, name in ((d.get("docx_file_id"), d.get("docx_name")),
                      (d.get("pdf_file_id"), d.get("pdf_name"))):
        if fid:
            try:
                await cb.message.answer_document(fid, caption=name or None)
                sent = True
            except Exception:
                pass
    await cb.answer("📄 Отправлено" if sent else "Файлы недоступны", show_alert=not sent)


@router.callback_query(F.data.startswith("deal_signed:"))
async def deal_send_signed(cb: CallbackQuery):
    deal_id = int(cb.data.split(":")[1])
    docs = await db.get_signed_docs(deal_id=deal_id)
    if not docs:
        await cb.answer("Подписанных документов нет", show_alert=True)
        return
    for doc in docs:
        try:
            await cb.message.answer_document(doc["file_id"], caption=doc.get("file_name") or None)
        except Exception:
            try:
                # file_id фото нельзя отправить как документ
                await cb.message.answer_photo(doc["file_id"], caption=doc.get("file_name") or None)
            except Exception:
                await cb.message.answer(f"⚠️ Не удалось отправить {esc(doc.get('file_name','файл'))}")
    await cb.answer("📥 Отправлено")


# ─── Удаление из журнала ──────────────────────────────────────────────────────

@router.callback_query(F.data.startswith("deal_del:"))
async def deal_delete_ask(cb: CallbackQuery):
    deal_id = int(cb.data.split(":")[1])
    await cb.message.edit_text(
        "⚠️ Удалить сделку из журнала?\n"
        "Записи о подписанных документах этой сделки тоже удалятся "
        "(сами файлы в чатах Telegram останутся).",
        reply_markup=confirm_deal_delete_kb(deal_id)
    )
    await cb.answer()


@router.callback_query(F.data.startswith("deal_del_yes:"))
async def deal_delete(cb: CallbackQuery):
    deal_id = int(cb.data.split(":")[1])
    await db.delete_deal(deal_id)
    await cb.answer("🗑 Удалено", show_alert=True)
    await _show_deals_menu(cb.message.edit_text)


# ─── Приём подписанных актов ──────────────────────────────────────────────────

@router.callback_query(F.data.startswith("deal_attach:"))
async def attach_start(cb: CallbackQuery, state: FSMContext):
    deal_id = int(cb.data.split(":")[1])
    d = await db.get_deal(deal_id)
    if not d:
        await cb.answer("Сделка не найдена", show_alert=True)
        return
    await state.set_state(SignedForm.waiting_files)
    await state.update_data(attach_deal_id=deal_id,
                            attach_company_id=d.get("company_id"),
                            attach_count=0)
    await cb.message.answer(
        f"📎 <b>Подписанные документы к сделке №{d.get('deal_number')}</b>\n\n"
        f"Пришлите файлы (PDF, DOCX, скан) — можно несколько подряд.\n"
        f"Когда закончите, нажмите «Готово».",
        reply_markup=signed_done_kb(),
        parse_mode="HTML"
    )
    await cb.answer()


@router.message(SignedForm.waiting_files, F.document)
async def attach_document(msg: Message, state: FSMContext):
    data = await state.get_data()
    await db.add_signed_doc(
        data.get("attach_company_id"), data.get("attach_deal_id"),
        msg.document.file_id, msg.document.file_name or "document"
    )
    count = data.get("attach_count", 0) + 1
    await state.update_data(attach_count=count)
    await msg.answer(f"✅ Сохранено ({count} шт.). Ещё файл или «Готово».",
                     reply_markup=signed_done_kb())


@router.message(SignedForm.waiting_files, F.photo)
async def attach_photo(msg: Message, state: FSMContext):
    data = await state.get_data()
    deal_id = data.get("attach_deal_id")
    await db.add_signed_doc(
        data.get("attach_company_id"), deal_id,
        msg.photo[-1].file_id, f"scan_deal_{deal_id}.jpg"
    )
    count = data.get("attach_count", 0) + 1
    await state.update_data(attach_count=count)
    await msg.answer(f"✅ Скан сохранён ({count} шт.). Ещё файл или «Готово».",
                     reply_markup=signed_done_kb())


@router.message(SignedForm.waiting_files)
async def attach_other(msg: Message):
    await msg.answer("Пришлите файл документом или фото, либо нажмите «Готово».",
                     reply_markup=signed_done_kb())


@router.callback_query(SignedForm.waiting_files, F.data == "signed:done")
async def attach_done(cb: CallbackQuery, state: FSMContext):
    data = await state.get_data()
    count = data.get("attach_count", 0)
    await state.clear()
    await cb.message.edit_text(
        f"📎 Готово: прикреплено документов — {count}.",
        reply_markup=back_to_main_kb()
    )
    await cb.answer()


# ─── Архив подписанных по клиенту (ZIP) ───────────────────────────────────────

@router.callback_query(F.data == "arch:menu")
async def archive_menu(cb: CallbackQuery):
    companies = await db.get_all_companies()
    if not companies:
        await cb.answer("Нет компаний", show_alert=True)
        return
    await cb.message.edit_text(
        "📦 <b>Архив подписанных документов</b>\n\nВыберите клиента — "
        "бот соберёт все его подписанные документы в ZIP:",
        reply_markup=company_pick_kb(companies, "arch"),
        parse_mode="HTML"
    )
    await cb.answer()


def _zip_files(zip_path: str, files: list[tuple[str, str]]):
    """files: [(tmp_path, arcname)]. Синхронно — вызывается через to_thread."""
    with zipfile.ZipFile(zip_path, "w", zipfile.ZIP_DEFLATED) as zf:
        for tmp, name in files:
            zf.write(tmp, arcname=name)


@router.callback_query(F.data.startswith("arch:") & (F.data != "arch:menu"))
async def archive_build(cb: CallbackQuery):
    cid = int(cb.data.split(":")[1])
    company = await db.get_company(cid)
    if not company:
        await cb.answer("Компания не найдена", show_alert=True)
        return
    docs = await db.get_signed_docs(company_id=cid)
    if not docs:
        await cb.answer("У клиента нет подписанных документов", show_alert=True)
        return

    await cb.answer("⏳ Собираю архив...")
    status = await cb.message.answer(f"⏳ Скачиваю {len(docs)} файлов из Telegram...")

    short = safe_name_part(company.get("short_name", ""))
    os.makedirs(OUTPUT_DIR, exist_ok=True)
    # uuid в путях: два одновременных запроса не затирают файлы друг друга
    run_id = uuid.uuid4().hex[:8]
    zip_path = os.path.join(OUTPUT_DIR, f"signed_{cid}_{run_id}.zip")

    downloaded: list[tuple[str, str]] = []   # (tmp_path, имя внутри архива)
    failed = 0
    try:
        used_names = set()
        for i, doc in enumerate(docs, 1):
            tmp = os.path.join(OUTPUT_DIR, f"tmp_signed_{run_id}_{i}")
            try:
                await cb.bot.download(doc["file_id"], destination=tmp)
            except Exception:
                failed += 1
                try_remove(tmp)
                continue
            name = doc.get("file_name") or f"document_{i}"
            base, ext = os.path.splitext(name)
            while name in used_names:
                name = f"{base}_{i}{ext}"
            used_names.add(name)
            downloaded.append((tmp, name))

        if downloaded:
            await asyncio.to_thread(_zip_files, zip_path, downloaded)
            note = f"\n⚠️ Не удалось скачать: {failed}" if failed else ""
            await cb.message.answer_document(
                FSInputFile(zip_path, filename=f"Подписанные_{short}.zip"),
                caption=f"📦 {company.get('short_name')}: {len(downloaded)} документ(ов){note}"
            )
        else:
            await cb.message.answer("❌ Не удалось скачать ни одного файла из Telegram.")
    finally:
        try_remove(zip_path, *(tmp for tmp, _ in downloaded))
        try:
            await status.delete()
        except Exception:
            pass
