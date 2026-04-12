"""
Hodler Doc Generator Bot
Запуск: python bot.py
"""
import asyncio
import logging
import os
from aiogram import Bot, Dispatcher
from aiogram.fsm.storage.memory import MemoryStorage

from config import BOT_TOKEN, OUTPUT_DIR
from database import init_db
from handlers import menu, companies, acts, settings as settings_handler

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
)
logger = logging.getLogger(__name__)


async def main():
    if not BOT_TOKEN:
        raise ValueError("BOT_TOKEN не задан в .env!")

    os.makedirs(OUTPUT_DIR, exist_ok=True)

    logger.info("Инициализация БД...")
    await init_db()

    bot = Bot(token=BOT_TOKEN)
    dp  = Dispatcher(storage=MemoryStorage())

    # Порядок роутеров важен
    dp.include_router(menu.router)
    dp.include_router(settings_handler.router)
    dp.include_router(companies.router)
    dp.include_router(acts.router)

    logger.info("Бот запущен. Ожидание сообщений...")
    try:
        await dp.start_polling(bot, allowed_updates=dp.resolve_used_update_types())
    finally:
        await bot.session.close()


if __name__ == "__main__":
    asyncio.run(main())
