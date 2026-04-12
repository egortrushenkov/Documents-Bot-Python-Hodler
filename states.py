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
    confirm      = State()


class ActForm(StatesGroup):
    """Создание акта"""
    select_type      = State()   # sell / buy
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
    tx_hash          = State()
    commission_fiat  = State()
    commission_va    = State()
    execution_date   = State()
    confirm          = State()
    edit_field       = State()   # редактирование конкретного поля


class SettingsForm(StatesGroup):
    """Редактирование реквизитов оператора"""
    editing = State()
