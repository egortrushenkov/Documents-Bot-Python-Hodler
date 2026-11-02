-- Базовая схема — та же, что создавала Python-версия бота (database.py).
-- IF NOT EXISTS: на существующей базе Python-версии ничего не меняется,
-- на пустой — создаётся структура с нуля.

CREATE TABLE IF NOT EXISTS companies (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    full_name   TEXT NOT NULL,
    short_name  TEXT NOT NULL,
    inn         TEXT DEFAULT '',
    kpp         TEXT DEFAULT '',
    reg_number  TEXT DEFAULT '',
    address     TEXT DEFAULT '',
    kio         TEXT DEFAULT '-',
    inn_rf      TEXT DEFAULT '-',
    kpp_rf      TEXT DEFAULT '-',
    bank_name   TEXT DEFAULT '',
    bank_account TEXT DEFAULT '',
    bank_bik    TEXT DEFAULT '',
    wallet      TEXT DEFAULT '',
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS operator_settings (
    key   TEXT PRIMARY KEY,
    value TEXT
);

CREATE TABLE IF NOT EXISTS act_counter (
    id          INTEGER PRIMARY KEY,
    last_number INTEGER DEFAULT 499
);

CREATE TABLE IF NOT EXISTS kvvo_codes (
    code       TEXT PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS deals (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    act_type        TEXT NOT NULL,
    deal_number     TEXT DEFAULT '',
    deal_date       TEXT DEFAULT '',
    deal_date_iso   TEXT DEFAULT '',
    execution_date  TEXT DEFAULT '',
    company_id      INTEGER,
    client_name     TEXT DEFAULT '',
    client_inn      TEXT DEFAULT '',
    client_resident TEXT DEFAULT '',
    va_type         TEXT DEFAULT '',
    network         TEXT DEFAULT '',
    va_amount       REAL DEFAULT 0,
    fiat_amount     REAL DEFAULT 0,
    fiat_currency   TEXT DEFAULT 'RUB',
    exchange_rate   TEXT DEFAULT '',
    kvvo            TEXT DEFAULT '',
    client_wallet   TEXT DEFAULT '',
    operator_wallet TEXT DEFAULT '',
    commission_fiat TEXT DEFAULT '',
    commission_va   TEXT DEFAULT '',
    transactions    TEXT DEFAULT '[]',
    docx_file_id    TEXT DEFAULT '',
    pdf_file_id     TEXT DEFAULT '',
    docx_name       TEXT DEFAULT '',
    pdf_name        TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_deals_month   ON deals (deal_date_iso);
CREATE INDEX IF NOT EXISTS idx_deals_company ON deals (company_id);

CREATE TABLE IF NOT EXISTS signed_docs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    company_id  INTEGER,
    deal_id     INTEGER,
    file_id     TEXT NOT NULL,
    file_name   TEXT DEFAULT '',
    uploaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_signed_company ON signed_docs (company_id);

CREATE TABLE IF NOT EXISTS company_counters (
    company_id  INTEGER,
    kind        TEXT,
    last_number INTEGER,
    PRIMARY KEY (company_id, kind)
);

CREATE TABLE IF NOT EXISTS custom_fields (
    key        TEXT PRIMARY KEY,
    label      TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS company_custom (
    company_id INTEGER,
    key        TEXT,
    value      TEXT DEFAULT '',
    PRIMARY KEY (company_id, key)
);
