package storage

import (
	"context"
	"database/sql"
	"strings"
)

// ─── Рефералы ─────────────────────────────────────────────────────────────────

type Referrer struct {
	ID      int64
	Name    string
	Percent float64
}

func (s *Store) Referrers(ctx context.Context) ([]Referrer, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, percent FROM referrers ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Referrer
	for rows.Next() {
		var r Referrer
		var name Str
		var pct Float
		if err := rows.Scan(&r.ID, &name, &pct); err != nil {
			return nil, err
		}
		r.Name, r.Percent = string(name), float64(pct)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Referrer возвращает реферала или nil.
func (s *Store) Referrer(ctx context.Context, id int64) (*Referrer, error) {
	var r Referrer
	var name Str
	var pct Float
	err := s.db.QueryRowContext(ctx, "SELECT id, name, percent FROM referrers WHERE id=?", id).Scan(&r.ID, &name, &pct)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	r.Name, r.Percent = string(name), float64(pct)
	return &r, err
}

func (s *Store) AddReferrer(ctx context.Context, name string, percent float64) (int64, error) {
	res, err := s.db.ExecContext(ctx, "INSERT INTO referrers (name, percent) VALUES (?,?)", name, percent)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateReferrer меняет имя/процент. При переименовании имя обновляется и в
// сделках и выплатах — иначе расчёт реферала «потеряет» историю.
func (s *Store) UpdateReferrer(ctx context.Context, id int64, name string, percent float64) error {
	old, err := s.Referrer(ctx, id)
	if err != nil || old == nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE referrers SET name=?, percent=? WHERE id=?", name, percent, id); err != nil {
		return err
	}
	if old.Name != name {
		for _, q := range []string{
			"UPDATE deals SET referrer_name=? WHERE referrer_name=?",
			"UPDATE treasury_ops SET referrer_name=? WHERE referrer_name=?",
		} {
			if _, err := tx.ExecContext(ctx, q, name, old.Name); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// DeleteReferrer удаляет реферала и открепляет его от клиентов
// (в уже проведённых сделках имя и процент сохранены снимком).
func (s *Store) DeleteReferrer(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE companies SET referrer_id=NULL, referral_percent=NULL WHERE referrer_id=?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM referrers WHERE id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// SetCompanyReferrer закрепляет реферала за клиентом (referrerID=0 — без реферала).
func (s *Store) SetCompanyReferrer(ctx context.Context, companyID, referrerID int64) error {
	var ref sql.NullInt64
	if referrerID != 0 {
		ref = sql.NullInt64{Int64: referrerID, Valid: true}
	}
	_, err := s.db.ExecContext(ctx, "UPDATE companies SET referrer_id=?, referral_percent=NULL WHERE id=?", ref, companyID)
	return err
}

// SetCompanyReferralPercent — свой процент для клиента (valid=false — как у реферала).
func (s *Store) SetCompanyReferralPercent(ctx context.Context, companyID int64, pct float64, valid bool) error {
	_, err := s.db.ExecContext(ctx, "UPDATE companies SET referral_percent=? WHERE id=?",
		sql.NullFloat64{Float64: pct, Valid: valid}, companyID)
	return err
}

// CompanyReferral — имя и процент реферала клиента ("" — реферала нет).
func (s *Store) CompanyReferral(ctx context.Context, c *Company) (string, float64, error) {
	if c == nil || !c.ReferrerID.Valid {
		return "", 0, nil
	}
	r, err := s.Referrer(ctx, c.ReferrerID.Int64)
	if err != nil || r == nil {
		return "", 0, err
	}
	pct := r.Percent
	if c.ReferralPercent.Valid {
		pct = c.ReferralPercent.Float64
	}
	return r.Name, pct, nil
}

// ─── Поставщики ───────────────────────────────────────────────────────────────

type Supplier struct {
	ID   int64
	Name string
}

func (s *Store) Suppliers(ctx context.Context) ([]Supplier, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name FROM suppliers ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Supplier
	for rows.Next() {
		var sp Supplier
		var name Str
		if err := rows.Scan(&sp.ID, &name); err != nil {
			return nil, err
		}
		sp.Name = string(name)
		out = append(out, sp)
	}
	return out, rows.Err()
}

// EnsureSupplier возвращает id поставщика, создавая его при необходимости.
func (s *Store) EnsureSupplier(ctx context.Context, name string) (int64, error) {
	name = strings.TrimSpace(name)
	if _, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO suppliers (name) VALUES (?)", name); err != nil {
		return 0, err
	}
	var id int64
	err := s.db.QueryRowContext(ctx, "SELECT id FROM suppliers WHERE name=?", name).Scan(&id)
	return id, err
}

func (s *Store) SupplierName(ctx context.Context, id int64) (string, error) {
	var n Str
	err := s.db.QueryRowContext(ctx, "SELECT name FROM suppliers WHERE id=?", id).Scan(&n)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return string(n), err
}

// DeleteSupplier удаляет поставщика; в откупках имя остаётся (снимок).
func (s *Store) DeleteSupplier(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		"UPDATE buybacks SET supplier_id=NULL WHERE supplier_id=?",
		"DELETE FROM bank_accounts WHERE owner_type='supplier' AND owner_id=?",
		"DELETE FROM wallets WHERE owner_type='supplier' AND owner_id=?",
		"DELETE FROM suppliers WHERE id=?",
	} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RenameSupplier — новое имя и в карточке, и во всех его откупках.
func (s *Store) RenameSupplier(ctx context.Context, id int64, name string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE suppliers SET name=? WHERE id=?", strings.TrimSpace(name), id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE buybacks SET supplier_name=? WHERE supplier_id=?", strings.TrimSpace(name), id); err != nil {
		return err
	}
	return tx.Commit()
}

// SupplierBuybacks — активные откупки поставщика (по id или, для старых, по имени).
func (s *Store) SupplierBuybacks(ctx context.Context, id int64, name string) ([]*Buyback, error) {
	return s.queryBuybacks(ctx, "WHERE status='active' AND (supplier_id=? OR (supplier_id IS NULL AND supplier_name=?)) ORDER BY id DESC", id, name)
}

// ─── Откупки ──────────────────────────────────────────────────────────────────

type Buyback struct {
	ID           int64
	BuyDate      string
	BuyDateISO   string
	SupplierID   sql.NullInt64
	SupplierName string
	RubAmount    float64
	Rate         float64
	FeePercent   float64
	Status       string
	CreatedBy    string
	Note         string
}

// USDT — сколько USDT куплено.
func (b *Buyback) USDT() float64 {
	if b.Rate <= 0 {
		return 0
	}
	return b.RubAmount / b.Rate
}

// FeeRub — комиссия банка за перевод поставщику.
func (b *Buyback) FeeRub() float64 { return b.RubAmount * b.FeePercent / 100 }

func (b *Buyback) Cancelled() bool { return b.Status == "cancelled" }

const buybackCols = `id, buy_date, buy_date_iso, supplier_id, supplier_name, rub_amount, rate,
	fee_percent, status, created_by, note`

func scanBuyback(sc interface{ Scan(...any) error }) (*Buyback, error) {
	var b Buyback
	var date, iso, sup, status, by, note Str
	var rub, rate, fee Float
	if err := sc.Scan(&b.ID, &date, &iso, &b.SupplierID, &sup, &rub, &rate, &fee, &status, &by, &note); err != nil {
		return nil, err
	}
	b.BuyDate, b.BuyDateISO, b.SupplierName = string(date), string(iso), string(sup)
	b.RubAmount, b.Rate, b.FeePercent = float64(rub), float64(rate), float64(fee)
	b.Status, b.CreatedBy, b.Note = string(status), string(by), string(note)
	if b.Status == "" {
		b.Status = "active"
	}
	return &b, nil
}

func (s *Store) AddBuyback(ctx context.Context, b *Buyback, dealIDs []int64) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO buybacks
		(buy_date, buy_date_iso, supplier_id, supplier_name, rub_amount, rate, fee_percent, status, created_by, note)
		VALUES (?,?,?,?,?,?,?,'active',?,?)`,
		b.BuyDate, b.BuyDateISO, b.SupplierID, b.SupplierName, b.RubAmount, b.Rate, b.FeePercent, b.CreatedBy, b.Note)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, d := range dealIDs {
		if _, err := tx.ExecContext(ctx, "UPDATE deals SET buyback_id=?, updated_at=? WHERE id=?", id, nowStamp(), d); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

// Buyback возвращает откупку или nil.
func (s *Store) Buyback(ctx context.Context, id int64) (*Buyback, error) {
	b, err := scanBuyback(s.db.QueryRowContext(ctx, "SELECT "+buybackCols+" FROM buybacks WHERE id=?", id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return b, err
}

func (s *Store) RecentBuybacks(ctx context.Context, limit int) ([]*Buyback, error) {
	return s.queryBuybacks(ctx, "ORDER BY id DESC LIMIT ?", limit)
}

// BuybacksForMonth — активные откупки месяца 'YYYY-MM'.
func (s *Store) BuybacksForMonth(ctx context.Context, month string) ([]*Buyback, error) {
	return s.queryBuybacks(ctx, "WHERE substr(buy_date_iso,1,7)=? AND status='active' ORDER BY buy_date_iso, id", month)
}

func (s *Store) queryBuybacks(ctx context.Context, where string, args ...any) ([]*Buyback, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+buybackCols+" FROM buybacks "+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Buyback
	for rows.Next() {
		b, err := scanBuyback(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BuybackColumns — редактируемые поля откупки.
var BuybackColumns = map[string]bool{"buy_date": true, "rub_amount": true, "rate": true, "fee_percent": true, "note": true}

func (s *Store) UpdateBuybackField(ctx context.Context, id int64, field string, value any) error {
	if !BuybackColumns[field] {
		return sql.ErrNoRows
	}
	q := "UPDATE buybacks SET " + field + "=? WHERE id=?"
	if field == "buy_date" {
		if str, ok := value.(string); ok && len(str) == 10 {
			if _, err := s.db.ExecContext(ctx, "UPDATE buybacks SET buy_date_iso=? WHERE id=?",
				str[6:10]+"-"+str[3:5]+"-"+str[0:2], id); err != nil {
				return err
			}
		}
	}
	_, err := s.db.ExecContext(ctx, q, value, id)
	return err
}

func (s *Store) SetBuybackSupplier(ctx context.Context, id, supplierID int64, name string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE buybacks SET supplier_id=?, supplier_name=? WHERE id=?", supplierID, name, id)
	return err
}

// SetBuybackStatus — отмена откупки: связанные сделки снова становятся открытыми.
func (s *Store) SetBuybackStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE buybacks SET status=? WHERE id=?", status, id)
	return err
}

// ─── Чаты руководителей и уведомления ─────────────────────────────────────────

type NotifyChat struct {
	ChatID           int64
	Title            string
	ThreadID         int // сделки и откупки
	WalletsThreadID  int // поступления на кошельки (0 — как ThreadID)
	TreasuryThreadID int // выводы, выплаты рефералам (0 — как ThreadID)
}

// Thread — ветка для вида уведомлений: deal | wallet | treasury.
func (c NotifyChat) Thread(kind string) int {
	switch kind {
	case "wallet":
		if c.WalletsThreadID != 0 {
			return c.WalletsThreadID
		}
	case "treasury":
		if c.TreasuryThreadID != 0 {
			return c.TreasuryThreadID
		}
	}
	return c.ThreadID
}

func (s *Store) NotifyChats(ctx context.Context) ([]NotifyChat, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT chat_id, title, COALESCE(thread_id,0), COALESCE(wallets_thread_id,0),
		COALESCE(treasury_thread_id,0) FROM notify_chats ORDER BY added_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NotifyChat
	for rows.Next() {
		var c NotifyChat
		var t Str
		if err := rows.Scan(&c.ChatID, &t, &c.ThreadID, &c.WalletsThreadID, &c.TreasuryThreadID); err != nil {
			return nil, err
		}
		c.Title = string(t)
		out = append(out, c)
	}
	return out, rows.Err()
}

// NotifyChat — настройки чата или nil.
func (s *Store) NotifyChat(ctx context.Context, chatID int64) (*NotifyChat, error) {
	chats, err := s.NotifyChats(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range chats {
		if c.ChatID == chatID {
			return &c, nil
		}
	}
	return nil, nil
}

func (s *Store) IsNotifyChat(ctx context.Context, chatID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notify_chats WHERE chat_id=?", chatID).Scan(&n)
	return n > 0, err
}

// BindNotifyChat подключает чат; kind = deal | wallet | treasury — какой вид
// уведомлений слать в ветку thread. Остальные настройки ветвей сохраняются.
func (s *Store) BindNotifyChat(ctx context.Context, chatID int64, title, by, kind string, thread int) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO notify_chats (chat_id, title, added_by, thread_id) VALUES (?,?,?,?)
		ON CONFLICT(chat_id) DO UPDATE SET title=excluded.title`, chatID, title, by, thread); err != nil {
		return err
	}
	col := map[string]string{"deal": "thread_id", "wallet": "wallets_thread_id", "treasury": "treasury_thread_id"}[kind]
	if col == "" {
		col = "thread_id"
	}
	_, err := s.db.ExecContext(ctx, "UPDATE notify_chats SET "+col+"=? WHERE chat_id=?", thread, chatID)
	return err
}

// AddNotifyChat — подключить чат без веток (совместимость).
func (s *Store) AddNotifyChat(ctx context.Context, chatID int64, title, by string) error {
	return s.BindNotifyChat(ctx, chatID, title, by, "deal", 0)
}

func (s *Store) RemoveNotifyChat(ctx context.Context, chatID int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM notify_chats WHERE chat_id=?", chatID)
	return err
}

type Notification struct {
	ChatID    int64
	MessageID int
	ThreadID  int
}

func (s *Store) SaveNotification(ctx context.Context, entity string, id, chatID int64, msgID, thread int) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO notifications (entity, entity_id, chat_id, message_id, thread_id) VALUES (?,?,?,?,?)",
		entity, id, chatID, msgID, thread)
	return err
}

func (s *Store) Notifications(ctx context.Context, entity string, id int64) ([]Notification, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT chat_id, message_id, COALESCE(thread_id,0) FROM notifications WHERE entity=? AND entity_id=?", entity, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ChatID, &n.MessageID, &n.ThreadID); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
