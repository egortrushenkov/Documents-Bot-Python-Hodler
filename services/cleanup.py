"""
Фоновая автоочистка.

Сгенерированные документы после отправки живут в Telegram (file_id хранится
в БД), поэтому локальные копии не нужны: acts.py удаляет их сразу после
отправки, а этот цикл подчищает всё, что осталось после сбоев, плюс
временные профили LibreOffice.
"""
import asyncio
import glob
import logging
import os
import shutil
import tempfile
import time

from config import OUTPUT_DIR, CLEANUP_MAX_AGE_HOURS, CLEANUP_INTERVAL_MIN

logger = logging.getLogger(__name__)


def _remove_old_files(directory: str, max_age_sec: float) -> int:
    removed = 0
    if not os.path.isdir(directory):
        return 0
    now = time.time()
    for entry in os.scandir(directory):
        try:
            if entry.is_file() and now - entry.stat().st_mtime > max_age_sec:
                os.remove(entry.path)
                removed += 1
        except OSError:
            pass
    return removed


def _remove_stale_lo_profiles(max_age_sec: float) -> int:
    removed = 0
    now = time.time()
    for path in glob.glob(os.path.join(tempfile.gettempdir(), "lo_profile_*")):
        try:
            if now - os.path.getmtime(path) > max_age_sec:
                shutil.rmtree(path, ignore_errors=True)
                removed += 1
        except OSError:
            pass
    return removed


async def cleanup_loop():
    max_age = CLEANUP_MAX_AGE_HOURS * 3600
    interval = CLEANUP_INTERVAL_MIN * 60
    while True:
        try:
            n_files = _remove_old_files(OUTPUT_DIR, max_age)
            n_prof = _remove_stale_lo_profiles(3600)  # профили LO старше часа
            if n_files or n_prof:
                logger.info("Автоочистка: удалено файлов=%d, LO-профилей=%d", n_files, n_prof)
        except Exception:
            logger.exception("Ошибка автоочистки")
        await asyncio.sleep(interval)


def try_remove(*paths: str):
    """Тихо удалить файлы (после успешной отправки в Telegram)."""
    for p in paths:
        if p:
            try:
                os.remove(p)
            except OSError:
                pass
