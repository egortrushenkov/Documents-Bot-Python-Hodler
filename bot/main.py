"""
Hodler Deal Docs — Telegram Bot
Aiogram 3, FSM, whitelist, генерация DOCX + PDF
"""

import logging
import asyncio
import os
from aiogram import Bot, Dispatcher, F
from aiogram.filters import CommandStart, Command
from aiogram.types import Message, CallbackQuery
from aiogram.fsm.storage.memory import MemoryStorage

from . import handlers

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
)
log = logging.getLogger(__name__)


async def main():
    token = os.environ["BOT_TOKEN"]
    bot   = Bot(token=token)
    dp    = Dispatcher(storage=MemoryStorage())

    handlers.register(dp)

    log.info("Bot started")
    await dp.start_polling(bot, allowed_updates=dp.resolve_used_update_types())


if __name__ == "__main__":
    asyncio.run(main())
