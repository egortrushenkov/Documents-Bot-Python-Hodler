from aiogram.fsm.state import State, StatesGroup


class DealFSM(StatesGroup):
    # Шаг 1 — тип сделки
    deal_type = State()

    # Шаг 2 — выбор/ввод клиента
    client_pick = State()
    client_name = State()
    client_address = State()
    client_ogrn = State()
    client_inn = State()
    client_kpp = State()
    client_bank_account = State()
    client_bank_name = State()
    client_bank_ks = State()
    client_bank_bik = State()
    client_save = State()

    # Шаг 3 — параметры договора
    contract_id = State()
    contract_date = State()
    execution_date = State()

    # Шаг 4 — параметры сделки
    usdt_amount = State()
    rub_amount = State()
    exchange_rate = State()
    client_wallet = State()
    operator_wallet = State()
    tx_hash = State()

    # Шаг 5 — платёж
    kvvo = State()
    license_contract = State()
    payment_purpose = State()
    act_operation_type = State()

    # Шаг 6 — подтверждение
    confirm = State()
