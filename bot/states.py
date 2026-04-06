from aiogram.fsm.state import State, StatesGroup


class DealFSM(StatesGroup):
    # Шаг 1 — тип документа
    doc_type = State()
    # Шаг 2 — направление сделки
    deal_type = State()
    # Шаг 3 — клиент
    client_pick = State()
    # Ввод нового клиента
    client_name_ru = State()
    client_name_eng = State()
    client_inn = State()
    client_kpp = State()
    client_reg_number = State()
    client_address = State()
    client_bank_name = State()
    client_bank_account = State()
    client_bank_ks = State()
    client_bank_bik = State()
    client_save = State()
    # Шаг 4 — договор
    contract_id = State()
    contract_date = State()
    execution_date = State()
    # Только для doc_type=contract
    license_contract = State()
    # Шаг 5 — сделка
    usdt_amount = State()
    rub_amount = State()
    exchange_rate = State()
    client_wallet = State()
    operator_wallet = State()
    tx_hash = State()
    # Шаг 6 — платёж
    kvvo = State()
    payment_purpose = State()
    act_operation_type = State()
    # Подтверждение
    confirm = State()
