from aiogram.fsm.state import State, StatesGroup


class CompanyForm(StatesGroup):
    """Добавление / редактирование компании"""
    full_name    = State()
    short_name   = State()
    inn          = State()
    kpp          = State()
    reg_number   = State()
    address      = State()
    kio          = State()
    inn_rf       = State()
    kpp_rf       = State()
    bank_name    = State()
    bank_account = State()
    bank_bik     = State()
    wallet       = State()
    resident     = State()   # Резидент / Нерезидент КР
    custom       = State()   # свои переменные (по одному вопросу на каждую)
    confirm      = State()


class ActForm(StatesGroup):
    """Создание акта"""
    select_type      = State()   # sell / buy / invoice
    select_company   = State()   # выбор из списка
    deal_number      = State()
    deal_date        = State()
    va_type          = State()   # USDT / BTC / ...
    network          = State()   # TRC-20 / ERC-20 / ...
    va_amount        = State()
    fiat_amount      = State()
    exchange_rate    = State()
    kvvo             = State()
    client_wallet    = State()
    operator_wallet  = State()
    split_mode       = State()   # одной транзакцией / разбить (тест + остаток)
    tx_one_hash      = State()   # хэш единственной транзакции
    add_tx           = State()   # хаб разбивки: добавить тест / завершить
    extra_va         = State()   # сумма ВА тест-транзакции
    extra_tx_hash    = State()   # хэш тест-транзакции
    remainder_hash   = State()   # хэш остатка
    commission_fiat  = State()
    commission_va    = State()
    execution_date   = State()
    confirm          = State()
    edit_field       = State()   # редактирование конкретного поля


class SettingsForm(StatesGroup):
    """Редактирование реквизитов оператора"""
    editing = State()


class ReportForm(StatesGroup):
    """Выгрузка отчёта ФН"""
    kgs_rate = State()   # курс RUB → KGS для колонки «в сомах»


class TemplateForm(StatesGroup):
    """Загрузка шаблона DOCX через бота"""
    waiting_file = State()


class VarForm(StatesGroup):
    """Создание своей переменной для шаблонов"""
    key   = State()
    label = State()


class SignedForm(StatesGroup):
    """Приём подписанных актов (файлы → file_id в БД)"""
    waiting_files = State()
