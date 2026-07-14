"""
Hodler Doc Generator Bot
Запуск: python bot.py
"""
import asyncio
import logging
import os
from typing import Any, Awaitable, Callable, Dict

from aiogram import BaseMiddleware, Bot, Dispatcher
from aiogram.fsm.storage.memory import MemoryStorage
from aiogram.types import TelegramObject

from config import BOT_TOKEN, OUTPUT_DIR, ADMIN_IDS
from database import init_db
from handlers import (
    menu, companies, acts, deals, reports, templates,
    settings as settings_handler,
)
from services.cleanup import cleanup_loop

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
)
logger = logging.getLogger(__name__)


class AccessMiddleware(BaseMiddleware):
    """Если задан ADMIN_IDS — бот отвечает только этим пользователям."""

    async def __call__(
        self,
        handler: Callable[[TelegramObject, Dict[str, Any]], Awaitable[Any]],
        event: TelegramObject,
        data: Dict[str, Any],
    ) -> Any:
        if ADMIN_IDS:
            user = data.get("event_from_user")
            if not user or user.id not in ADMIN_IDS:
                return  # тихо игнорируем посторонних
        return await handler(event, data)


async def main():
    if not BOT_TOKEN:
        raise ValueError("BOT_TOKEN не задан в .env!")

    os.makedirs(OUTPUT_DIR, exist_ok=True)

    logger.info("Инициализация БД...")
    await init_db()

    bot = Bot(token=BOT_TOKEN)
    dp  = Dispatcher(storage=MemoryStorage())

    dp.message.outer_middleware(AccessMiddleware())
    dp.callback_query.outer_middleware(AccessMiddleware())

    # Порядок роутеров важен
    dp.include_router(menu.router)
    dp.include_router(settings_handler.router)
    dp.include_router(companies.router)
    dp.include_router(deals.router)
    dp.include_router(reports.router)
    dp.include_router(templates.router)
    dp.include_router(acts.router)

    # Фоновая автоочистка сгенерированных файлов и временных профилей LibreOffice
    cleanup_task = asyncio.create_task(cleanup_loop())

    logger.info("Бот запущен. Ожидание сообщений...")
    try:
        await dp.start_polling(bot, allowed_updates=dp.resolve_used_update_types())
    finally:
        cleanup_task.cancel()
        await bot.session.close()


if __name__ == "__main__":
    asyncio.run(main())
