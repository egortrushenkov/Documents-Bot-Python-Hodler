-- Учёт остатков USDT и рублей.
-- Движения по сделкам и откупкам считаются автоматически из журнала;
-- здесь — только ручные операции: начальный остаток, пополнение, вывод
-- прибыли, выплата рефералу, налог, корректировка.
CREATE TABLE IF NOT EXISTS treasury_ops (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    op_date       TEXT NOT NULL,            -- YYYY-MM-DD
    kind          TEXT NOT NULL,            -- opening | deposit | withdraw | referral | tax | correction
    usdt          REAL DEFAULT 0,           -- со знаком: + приход, − расход
    rub           REAL DEFAULT 0,
    referrer_name TEXT DEFAULT '',          -- для выплат рефералам
    note          TEXT DEFAULT '',
    created_by    TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_treasury_date ON treasury_ops (op_date);
