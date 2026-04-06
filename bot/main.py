import logging, asyncio, os
from aiogram import Bot, Dispatcher
from aiogram.fsm.storage.memory import MemoryStorage
from . import handlers

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(name)s: %(message)s")

async def main():
    bot = Bot(token=os.environ["BOT_TOKEN"])
    dp  = Dispatcher(storage=MemoryStorage())
    handlers.register(dp)
    await dp.start_polling(bot, allowed_updates=dp.resolve_used_update_types())

if __name__ == "__main__":
    asyncio.run(main())
