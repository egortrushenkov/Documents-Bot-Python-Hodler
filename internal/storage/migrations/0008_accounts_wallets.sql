-- Несколько банковских реквизитов и кошельков у клиентов и у оператора,
-- отслеживание поступлений на кошельки оператора.

CREATE TABLE IF NOT EXISTS bank_accounts (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_type TEXT NOT NULL,              -- company | operator
    owner_id   INTEGER NOT NULL DEFAULT 0, -- id компании (для operator — 0)
    label      TEXT DEFAULT '',
    bank_name  TEXT DEFAULT '',
    account    TEXT DEFAULT '',
    bik        TEXT DEFAULT '',
    is_default INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_bank_owner ON bank_accounts (owner_type, owner_id);

CREATE TABLE IF NOT EXISTS wallets (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_type   TEXT NOT NULL,
    owner_id     INTEGER NOT NULL DEFAULT 0,
    label        TEXT DEFAULT '',
    network      TEXT DEFAULT '',          -- TRC-20 | ERC-20 | BEP-20 | ...
    address      TEXT NOT NULL,
    is_default   INTEGER DEFAULT 0,
    watch_in     INTEGER DEFAULT 0,        -- уведомлять о поступлениях (кошельки оператора)
    watch_out    INTEGER DEFAULT 0,        -- уведомлять о списаниях
    min_amount   REAL DEFAULT 0,           -- не уведомлять о суммах меньше
    watch_since  INTEGER DEFAULT 0,        -- unix ms: более ранние транзакции не уведомляются
    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_wallet_owner ON wallets (owner_type, owner_id);
CREATE INDEX IF NOT EXISTS idx_wallet_address ON wallets (address);

-- Уже увиденные транзакции кошельков (чтобы не уведомлять дважды)
CREATE TABLE IF NOT EXISTS wallet_txs (
    hash         TEXT NOT NULL,
    wallet_id    INTEGER NOT NULL,
    direction    TEXT DEFAULT '',          -- in | out
    amount       REAL DEFAULT 0,
    counterparty TEXT DEFAULT '',
    ts           INTEGER DEFAULT 0,
    notified     INTEGER DEFAULT 0,
    PRIMARY KEY (hash, wallet_id)
);

-- Какие реквизиты выбраны в сделке (для переделки заявки)
ALTER TABLE deals ADD COLUMN client_bank_id   INTEGER;
ALTER TABLE deals ADD COLUMN operator_bank_id INTEGER;

-- Перенос текущих реквизитов и кошельков компаний — как «Основной»
INSERT INTO bank_accounts (owner_type, owner_id, label, bank_name, account, bik, is_default)
SELECT 'company', id, 'Основной', COALESCE(bank_name,''), COALESCE(bank_account,''), COALESCE(bank_bik,''), 1
  FROM companies
 WHERE TRIM(COALESCE(bank_name,'') || COALESCE(bank_account,'') || COALESCE(bank_bik,'')) NOT IN ('', '-', '--', '---');

INSERT INTO wallets (owner_type, owner_id, label, network, address, is_default)
SELECT 'company', id, 'Основной',
       CASE WHEN TRIM(wallet) LIKE 'T%' THEN 'TRC-20' WHEN TRIM(wallet) LIKE '0x%' THEN 'ERC-20' ELSE '' END,
       TRIM(wallet), 1
  FROM companies
 WHERE TRIM(COALESCE(wallet,'')) NOT IN ('', '-');

-- Реквизиты и кошелёк оператора
INSERT INTO bank_accounts (owner_type, owner_id, label, bank_name, account, bik, is_default)
SELECT 'operator', 0, 'Основной',
       COALESCE((SELECT value FROM operator_settings WHERE key='bank_name'), ''),
       COALESCE((SELECT value FROM operator_settings WHERE key='bank_account'), ''),
       COALESCE((SELECT value FROM operator_settings WHERE key='bank_bik'), ''), 1
 WHERE EXISTS (SELECT 1 FROM operator_settings WHERE key='bank_account' AND TRIM(value) != '');

-- Кошелёк оператора: поступления отслеживаются сразу, с момента миграции
INSERT INTO wallets (owner_type, owner_id, label, network, address, is_default, watch_in, watch_since)
SELECT 'operator', 0, 'Основной',
       CASE WHEN TRIM(value) LIKE 'T%' THEN 'TRC-20' WHEN TRIM(value) LIKE '0x%' THEN 'ERC-20' ELSE '' END,
       TRIM(value), 1, 1, CAST(strftime('%s','now') AS INTEGER) * 1000
  FROM operator_settings WHERE key='wallet' AND TRIM(value) NOT IN ('', '-');

-- Куда слать уведомления о движениях по кошелькам
INSERT OR IGNORE INTO operator_settings (key, value) VALUES
    ('wallet_notify_admins', '1'),
    ('wallet_notify_groups', '0');
