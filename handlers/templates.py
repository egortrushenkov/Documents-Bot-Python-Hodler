"""
Управление шаблонами DOCX прямо из бота (без захода на сервер):
просмотр, скачивание, загрузка нового (со сканом плейсхолдеров и
резервной копией старого), плюс свои переменные {{CL_...}}, которые
спрашиваются при добавлении каждой новой компании.
"""
import asyncio
import os
import re
import shutil
from datetime import datetime

from aiogram import Router, F
from aiogram.types import Message, CallbackQuery, FSInputFile
from aiogram.fsm.context import FSMContext

import database as db
from config import TEMPLATES_DIR, ACT_TEMPLATE_FILES
from states import TemplateForm, VarForm
from keyboards import (
    templates_menu_kb, template_card_kb, vars_menu_kb,
    back_to_main_kb, TEMPLATE_KINDS
)
from services.doc_generator import scan_placeholders, known_placeholders
from utils import esc

router = Router()

# kind (тип операции в боте) → файл шаблона: единый источник в config
KIND_FILES = ACT_TEMPLATE_FILES
KIND_LABELS = dict(TEMPLATE_KINDS)

MAX_TEMPLATE_MB = 15
BACKUPS_KEEP = 10   # сколько резервных копий хранить на каждый шаблон


def _tpl_path(kind: str) -> str:
    return os.path.join(TEMPLATES_DIR, KIND_FILES[kind])


async def _template_card(kind: str) -> str:
    path = _tpl_path(kind)
    lines = [f"<b>{KIND_LABELS.get(kind, kind)}</b>",
             f"Файл: <code>{KIND_FILES[kind]}</code>\n"]
    if not os.path.exists(path):
        lines.append("⚠️ <b>Файл не найден!</b> Загрузите шаблон .docx.")
        return "\n".join(lines)

    st = os.stat(path)
    lines.append(f"Размер: {st.st_size // 1024} КБ · "
                 f"Обновлён: {datetime.fromtimestamp(st.st_mtime):%d.%m.%Y %H:%M}")

    try:
        # python-docx синхронный — сканируем в потоке, чтобы не блокировать бота
        found = await asyncio.to_thread(scan_placeholders, path)
        op = await db.get_all_settings()
        custom = await db.get_custom_fields()
        known = known_placeholders(op, custom)
        unknown = [p for p in found if p not in known]
        lines.append(f"\n<b>Плейсхолдеры ({len(found)}):</b>")
        lines.append("<code>" + " ".join(found) + "</code>" if found
                     else "<i>не найдены — проверьте шаблон</i>")
        if unknown:
            lines.append(f"\n❗️ <b>Неизвестные (останутся незаполненными):</b>")
            lines.append("<code>" + " ".join(unknown) + "</code>")
            lines.append("<i>Создайте для них свои переменные (🧩) или исправьте шаблон.</i>")
    except Exception as e:
        lines.append(f"\n⚠️ Не удалось прочитать шаблон: <code>{esc(e)}</code>")
    return "\n".join(lines)


# ─── Меню шаблонов ────────────────────────────────────────────────────────────

@router.callback_query(F.data == "tpl:menu")
async def tpl_menu(cb: CallbackQuery, state: FSMContext):
    await state.clear()
    await cb.message.edit_text(
        "📄 <b>Шаблоны документов</b>\n\n"
        "Здесь можно посмотреть, скачать и заменить шаблоны, "
        "а также завести свои переменные для новых плейсхолдеров.",
        reply_markup=templates_menu_kb(),
        parse_mode="HTML"
    )
    await cb.answer()


@router.callback_query(F.data.startswith("tpl:view:"))
async def tpl_view(cb: CallbackQuery):
    kind = cb.data.split(":")[2]
    await cb.message.edit_text(
        await _template_card(kind),
        reply_markup=template_card_kb(kind),
        parse_mode="HTML"
    )
    await cb.answer()


@router.callback_query(F.data.startswith("tpl:download:"))
async def tpl_download(cb: CallbackQuery):
    kind = cb.data.split(":")[2]
    path = _tpl_path(kind)
    if not os.path.exists(path):
        await cb.answer("Файл не найден", show_alert=True)
        return
    await cb.message.answer_document(
        FSInputFile(path, filename=KIND_FILES[kind]),
        caption=f"📄 Текущий шаблон: {KIND_LABELS.get(kind, kind)}"
    )
    await cb.answer()


# ─── Загрузка нового шаблона ──────────────────────────────────────────────────

@router.callback_query(F.data.startswith("tpl:upload:"))
async def tpl_upload_start(cb: CallbackQuery, state: FSMContext):
    kind = cb.data.split(":")[2]
    await state.set_state(TemplateForm.waiting_file)
    await state.update_data(tpl_kind=kind)
    await cb.message.edit_text(
        f"⬆️ <b>Загрузка шаблона</b>\n{KIND_LABELS.get(kind, kind)}\n\n"
        f"Пришлите файл <b>.docx</b> с плейсхолдерами вида "
        f"<code>{{{{DEAL_NUMBER}}}}</code>.\n"
        f"Старый шаблон сохранится в резервную копию.",
        parse_mode="HTML"
    )
    await cb.answer()


@router.message(TemplateForm.waiting_file, F.document)
async def tpl_upload_file(msg: Message, state: FSMContext):
    doc = msg.document
    if not (doc.file_name or "").lower().endswith(".docx"):
        await msg.answer("❌ Нужен файл .docx — пришлите шаблон Word.")
        return
    if doc.file_size and doc.file_size > MAX_TEMPLATE_MB * 1024 * 1024:
        await msg.answer(f"❌ Файл больше {MAX_TEMPLATE_MB} МБ.")
        return

    data = await state.get_data()
    kind = data.get("tpl_kind", "sell")
    target = _tpl_path(kind)
    tmp = target + ".new"

    os.makedirs(TEMPLATES_DIR, exist_ok=True)
    await msg.bot.download(doc, destination=tmp)

    # Проверяем, что это валидный DOCX и в нём есть плейсхолдеры
    try:
        found = await asyncio.to_thread(scan_placeholders, tmp)
    except Exception as e:
        os.remove(tmp)
        await msg.answer(f"❌ Файл не читается как DOCX:\n<code>{esc(e)}</code>", parse_mode="HTML")
        return
    if not found:
        os.remove(tmp)
        await msg.answer(
            "❌ В файле нет ни одного плейсхолдера {{...}} — "
            "похоже, это не шаблон. Файл не сохранён."
        )
        return

    # Бэкап старого и замена (храним последние BACKUPS_KEEP копий)
    if os.path.exists(target):
        backup_dir = os.path.join(TEMPLATES_DIR, "backups")
        os.makedirs(backup_dir, exist_ok=True)
        stamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        shutil.copy(target, os.path.join(backup_dir, f"{stamp}_{KIND_FILES[kind]}"))
        old = sorted(f for f in os.listdir(backup_dir) if f.endswith(KIND_FILES[kind]))
        for name in old[:-BACKUPS_KEEP]:
            try:
                os.remove(os.path.join(backup_dir, name))
            except OSError:
                pass
    os.replace(tmp, target)
    await state.clear()

    await msg.answer("✅ Шаблон обновлён!", parse_mode="HTML")
    await msg.answer(
        await _template_card(kind),
        reply_markup=template_card_kb(kind),
        parse_mode="HTML"
    )


@router.message(TemplateForm.waiting_file)
async def tpl_upload_other(msg: Message):
    await msg.answer("Пришлите шаблон файлом .docx (или /menu для отмены).")


# ─── Свои переменные ──────────────────────────────────────────────────────────

async def _vars_text() -> str:
    fields = await db.get_custom_fields()
    lines = [
        "🧩 <b>Свои переменные</b>\n",
        "Каждая переменная — это плейсхолдер <code>{{CL_КЛЮЧ}}</code> для шаблонов. "
        "При добавлении новой компании бот задаст вопрос по каждой переменной.\n",
    ]
    if fields:
        lines.append("<b>Текущие (нажмите, чтобы удалить):</b>")
    else:
        lines.append("<i>Пока нет ни одной переменной.</i>")
    return "\n".join(lines)


@router.callback_query(F.data == "vars:menu")
async def vars_menu(cb: CallbackQuery, state: FSMContext):
    await state.clear()
    await cb.message.edit_text(
        await _vars_text(),
        reply_markup=vars_menu_kb(await db.get_custom_fields()),
        parse_mode="HTML"
    )
    await cb.answer()


@router.callback_query(F.data == "vars:add")
async def vars_add(cb: CallbackQuery, state: FSMContext):
    await state.set_state(VarForm.key)
    await cb.message.edit_text(
        "➕ <b>Новая переменная — шаг 1/2</b>\n\n"
        "Введите ключ латиницей (буквы, цифры, подчёркивание).\n"
        "Например: <code>contract_number</code> → в шаблоне будет "
        "<code>{{CL_CONTRACT_NUMBER}}</code>",
        parse_mode="HTML"
    )
    await cb.answer()


@router.message(VarForm.key)
async def vars_key_input(msg: Message, state: FSMContext):
    key = msg.text.strip().lower().lstrip("{").rstrip("}")
    if key.startswith("cl_"):
        key = key[3:]
    if not re.fullmatch(r"[a-z][a-z0-9_]{0,30}", key):
        await msg.answer("❌ Ключ — латиницей, начинается с буквы, без пробелов. Например: contract_number")
        return
    await state.update_data(var_key=key)
    await state.set_state(VarForm.label)
    await msg.answer(
        f"Ключ: <code>{{{{CL_{key.upper()}}}}}</code>\n\n"
        f"<b>Шаг 2/2:</b> введите вопрос-подпись, который бот будет задавать "
        f"при добавлении компании.\nНапример: <i>Номер договора с клиентом</i>",
        parse_mode="HTML"
    )


@router.message(VarForm.label)
async def vars_label_input(msg: Message, state: FSMContext):
    data = await state.get_data()
    key = data.get("var_key", "")
    label = msg.text.strip()
    await db.add_custom_field(key, label)
    await state.clear()
    await msg.answer(
        f"✅ Переменная <code>{{{{CL_{key.upper()}}}}}</code> — «{esc(label)}» создана.\n"
        f"Она появится в вопросах при добавлении компании и в карточках компаний.",
        parse_mode="HTML"
    )
    await msg.answer(
        await _vars_text(),
        reply_markup=vars_menu_kb(await db.get_custom_fields()),
        parse_mode="HTML"
    )


@router.callback_query(F.data.startswith("vars:del:"))
async def vars_delete(cb: CallbackQuery):
    key = cb.data.split(":", 2)[2]
    await db.delete_custom_field(key)
    await cb.answer(f"Удалена {key}")
    await cb.message.edit_text(
        await _vars_text(),
        reply_markup=vars_menu_kb(await db.get_custom_fields()),
        parse_mode="HTML"
    )
