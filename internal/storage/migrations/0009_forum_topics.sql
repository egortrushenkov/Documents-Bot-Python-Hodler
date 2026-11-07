-- Чаты с ветками (темами): в какую ветку слать каждый вид уведомлений.
-- 0 — общая ветка (General) или чат без веток.
ALTER TABLE notify_chats  ADD COLUMN thread_id          INTEGER DEFAULT 0; -- сделки, откупки
ALTER TABLE notify_chats  ADD COLUMN wallets_thread_id  INTEGER DEFAULT 0; -- поступления на кошельки (0 — как сделки)
ALTER TABLE notify_chats  ADD COLUMN treasury_thread_id INTEGER DEFAULT 0; -- выводы, выплаты рефералам (0 — как сделки)
ALTER TABLE notifications ADD COLUMN thread_id          INTEGER DEFAULT 0; -- куда ушло уведомление (для ответов)
