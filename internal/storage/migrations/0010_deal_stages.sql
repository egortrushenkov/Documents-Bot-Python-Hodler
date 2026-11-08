-- Этапы сделки: создана → счёт на оплату → оплата ₽ получена → USDT выплачен → акт (завершена).
-- Для покупки у клиента: создана → USDT получен → ₽ выплачены → акт.
ALTER TABLE deals ADD COLUMN invoice_docx_file_id TEXT DEFAULT '';
ALTER TABLE deals ADD COLUMN invoice_pdf_file_id  TEXT DEFAULT '';
ALTER TABLE deals ADD COLUMN invoice_docx_name    TEXT DEFAULT '';
ALTER TABLE deals ADD COLUMN invoice_pdf_name     TEXT DEFAULT '';
ALTER TABLE deals ADD COLUMN invoiced_at TEXT DEFAULT '';  -- YYYY-MM-DD; '-' — счёт не нужен
ALTER TABLE deals ADD COLUMN paid_at     TEXT DEFAULT '';  -- оплата ₽ (получена или выплачена)
ALTER TABLE deals ADD COLUMN paid_amount REAL DEFAULT 0;
ALTER TABLE deals ADD COLUMN paid_note   TEXT DEFAULT '';
ALTER TABLE deals ADD COLUMN usdt_at     TEXT DEFAULT '';  -- USDT выплачен клиенту / получен от клиента
ALTER TABLE deals ADD COLUMN act_at      TEXT DEFAULT '';  -- акт сформирован → сделка завершена
CREATE INDEX IF NOT EXISTS idx_deals_act ON deals (act_at);

-- Старые акты — завершённые сделки: все этапы пройдены в дату заявки
UPDATE deals
   SET invoiced_at = '-', paid_at = deal_date_iso, paid_amount = fiat_amount,
       usdt_at = deal_date_iso, act_at = COALESCE(NULLIF(deal_date_iso, ''), substr(created_at, 1, 10))
 WHERE act_type IN ('sell', 'buy');

-- Старые отдельные счета-заявки — сделки на этапе «счёт выставлен»
UPDATE deals
   SET act_type = 'sell',
       invoice_docx_file_id = docx_file_id, invoice_pdf_file_id = pdf_file_id,
       invoice_docx_name = docx_name, invoice_pdf_name = pdf_name,
       docx_file_id = '', pdf_file_id = '', docx_name = '', pdf_name = '',
       invoiced_at = COALESCE(NULLIF(deal_date_iso, ''), substr(created_at, 1, 10))
 WHERE act_type = 'invoice';
