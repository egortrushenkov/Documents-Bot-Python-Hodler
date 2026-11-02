-- Индекс для выборки подписанных документов по сделке (карточка сделки).
CREATE INDEX IF NOT EXISTS idx_signed_deal ON signed_docs (deal_id);

-- Нормализация данных, записанных Python-версией:
-- пустые/NULL значения транзакций → пустой JSON-массив.
UPDATE deals SET transactions = '[]'
 WHERE transactions IS NULL OR TRIM(transactions) = '';

-- NULL в текстовых полях компаний → значения по умолчанию.
UPDATE companies SET resident = 'Резидент' WHERE resident IS NULL OR TRIM(resident) = '';
UPDATE companies SET kio    = '-' WHERE kio    IS NULL;
UPDATE companies SET inn_rf = '-' WHERE inn_rf IS NULL;
UPDATE companies SET kpp_rf = '-' WHERE kpp_rf IS NULL;

-- deal_date_iso (YYYY-MM-DD) для отчёта ФН — восстановить из ДД.ММ.ГГГГ, где пусто.
UPDATE deals
   SET deal_date_iso = substr(TRIM(deal_date), 7, 4) || '-' || substr(TRIM(deal_date), 4, 2) || '-' || substr(TRIM(deal_date), 1, 2)
 WHERE (deal_date_iso IS NULL OR deal_date_iso = '')
   AND TRIM(deal_date) GLOB '[0-3][0-9].[01][0-9].[12][0-9][0-9][0-9]';
