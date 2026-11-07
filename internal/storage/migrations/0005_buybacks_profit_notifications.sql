-- Откупки, расчёт прибыли, рефералы, статусы сделок, уведомления руководителям.

-- Рефералы: процент — доля от (ЧКД − налог)
CREATE TABLE IF NOT EXISTS referrers (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    percent    REAL NOT NULL DEFAULT 20,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Реферал закреплён за клиентом; процент можно переопределить для клиента
ALTER TABLE companies ADD COLUMN referrer_id      INTEGER;
ALTER TABLE companies ADD COLUMN referral_percent REAL;     -- NULL = процент реферала

-- Поставщики (у кого откупаем USDT)
CREATE TABLE IF NOT EXISTS suppliers (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Откупка: одна покупка USDT за рубли, закрывает одну или несколько заявок клиентов
CREATE TABLE IF NOT EXISTS buybacks (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    buy_date      TEXT DEFAULT '',          -- ДД.ММ.ГГГГ
    buy_date_iso  TEXT DEFAULT '',
    supplier_id   INTEGER,
    supplier_name TEXT DEFAULT '',
    rub_amount    REAL DEFAULT 0,           -- сколько рублей переведено поставщику
    rate          REAL DEFAULT 0,           -- курс откупки ₽ за 1 USDT
    fee_percent   REAL DEFAULT 0.05,        -- комиссия банка за перевод, %
    status        TEXT DEFAULT 'active',    -- active | cancelled
    created_by    TEXT DEFAULT '',
    note          TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_buybacks_date ON buybacks (buy_date_iso);

-- Сделки: статус, версия, откупка и снимок параметров прибыли на момент сделки
ALTER TABLE deals ADD COLUMN status           TEXT DEFAULT 'active';  -- active | cancelled
ALTER TABLE deals ADD COLUMN version          INTEGER DEFAULT 1;
ALTER TABLE deals ADD COLUMN updated_at       TEXT DEFAULT '';
ALTER TABLE deals ADD COLUMN created_by       TEXT DEFAULT '';
ALTER TABLE deals ADD COLUMN buyback_id       INTEGER;
ALTER TABLE deals ADD COLUMN tax_percent      REAL DEFAULT 5;
ALTER TABLE deals ADD COLUMN fee_percent      REAL DEFAULT 0.05;
ALTER TABLE deals ADD COLUMN referrer_name    TEXT DEFAULT '';
ALTER TABLE deals ADD COLUMN referral_percent REAL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_deals_buyback ON deals (buyback_id);
CREATE INDEX IF NOT EXISTS idx_deals_status  ON deals (status);

-- Чаты руководителей, куда бот шлёт уведомления
CREATE TABLE IF NOT EXISTS notify_chats (
    chat_id  INTEGER PRIMARY KEY,
    title    TEXT DEFAULT '',
    added_by TEXT DEFAULT '',
    added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Отправленные уведомления — чтобы обновлять их при правке/отмене и отвечать на них
CREATE TABLE IF NOT EXISTS notifications (
    entity     TEXT NOT NULL,      -- deal | buyback
    entity_id  INTEGER NOT NULL,
    chat_id    INTEGER NOT NULL,
    message_id INTEGER NOT NULL,
    PRIMARY KEY (entity, entity_id, chat_id)
);

-- Параметры прибыли (редактируются в «⚙️ Реквизиты»)
INSERT OR IGNORE INTO operator_settings (key, value) VALUES
    ('tax_percent',      '5'),
    ('bank_fee_percent', '0.05'),
    ('report_title',     'Классика');
