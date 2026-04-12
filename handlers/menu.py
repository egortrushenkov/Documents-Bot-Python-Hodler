from aiogram import Router, F
from aiogram.types import Message, CallbackQuery
from aiogram.filters import CommandStart, Command
from aiogram.fsm.context import FSMContext
from keyboards import main_menu_kb, nav_kb

router = Router()

WELCOME = (
    "🏦 <b>Hodler Doc Generator</b>\n\n"
    "Автоматическое формирование актов и договоров\n"
    "для сделок с виртуальными активами.\n\n"
    "Выберите действие:"
)


@router.message(CommandStart())
@router.message(Command("menu"))
@router.message(Command("new"))
@router.message(F.text == "🏠 Меню")
async def cmd_start(msg: Message, state: FSMContext):
    await state.clear()
    await msg.answer(WELCOME, reply_markup=main_menu_kb(), parse_mode="HTML")
    await msg.answer("⬇️ Быстрый доступ:", reply_markup=nav_kb())


@router.callback_query(F.data == "menu:main")
async def cb_main(cb: CallbackQuery, state: FSMContext):
    await state.clear()
    await cb.message.edit_text(WELCOME, reply_markup=main_menu_kb(), parse_mode="HTML")
    await cb.answer()
