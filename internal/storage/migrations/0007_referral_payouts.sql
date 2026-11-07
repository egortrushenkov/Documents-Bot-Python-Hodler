-- Выплаты рефералам: за какой период выплачено (для расчётов по датам).
-- Ручные начисления рефералу (kind = 'referral_adj') меняют только долг
-- перед рефералом, а не остатки USDT/₽.
ALTER TABLE treasury_ops ADD COLUMN period_from TEXT DEFAULT '';
ALTER TABLE treasury_ops ADD COLUMN period_to   TEXT DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_treasury_ref ON treasury_ops (referrer_name, kind);
CREATE INDEX IF NOT EXISTS idx_deals_referrer ON deals (referrer_name);
