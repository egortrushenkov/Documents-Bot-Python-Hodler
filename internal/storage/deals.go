package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// Tx — одна транзакция-часть сделки (тест или остаток).
type Tx struct {
	VAAmount string `json:"va_amount"`
	TxHash   string `json:"tx_hash"`
}

// UnmarshalJSON принимает и строки, и числа (на случай ручных правок БД).
func (t *Tx) UnmarshalJSON(b []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	t.VAAmount = anyStr(raw["va_amount"])
	t.TxHash = anyStr(raw["tx_hash"])
	return nil
}

func anyStr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		b, _ := json.Marshal(x)
		return string(b)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// ParseTxs разбирает JSON-список транзакций; при ошибке — пустой список.
func ParseTxs(s string) []Tx {
	var out []Tx
	if strings.TrimSpace(s) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

// Deal — запись журнала сделок.
type Deal struct {
	ID             int64
	ActType        string
	DealNumber     string
	DealDate       string
	DealDateISO    string
	ExecutionDate  string
	CompanyID      sql.NullInt64
	ClientName     string
	ClientINN      string
	ClientResident string
	VAType         string
	Network        string
	VAAmount       float64
	FiatAmount     float64
	FiatCurrency   string
	ExchangeRate   string
	KVVO           string
	ClientWallet   string
	OperatorWallet string
	CommissionFiat string
	CommissionVA   string
	Transactions   string // JSON
	DocxFileID     string
	PDFFileID      string
	DocxName       string
	PDFName        string

	// Статус, версия и параметры прибыли (миграция 0005)
	Status          string // active | cancelled
	Version         int
	UpdatedAt       string
	CreatedBy       string
	BuybackID       sql.NullInt64
	TaxPercent      float64
	FeePercent      float64
	ReferrerName    string
	ReferralPercent float64

	// Выбранные в сделке реквизиты (миграция 0008)
	ClientBankID   sql.NullInt64
	OperatorBankID sql.NullInt64

	// Заполняется при чтении из связанной откупки
	BuyRate      float64
	SupplierName string
}

// Cancelled — сделка отменена (в отчёты не попадает).
func (d *Deal) Cancelled() bool { return d.Status == "cancelled" }

const dealCols = `act_type, deal_number, deal_date, deal_date_iso, execution_date,
	company_id, client_name, client_inn, client_resident, va_type, network,
	va_amount, fiat_amount, fiat_currency, exchange_rate, kvvo, client_wallet,
	operator_wallet, commission_fiat, commission_va, transactions,
	docx_file_id, pdf_file_id, docx_name, pdf_name,
	status, version, updated_at, created_by, buyback_id,
	tax_percent, fee_percent, referrer_name, referral_percent,
	client_bank_id, operator_bank_id`

func dealArgs(d *Deal) []any {
	return []any{d.ActType, d.DealNumber, d.DealDate, d.DealDateISO, d.ExecutionDate,
		d.CompanyID, d.ClientName, d.ClientINN, d.ClientResident, d.VAType, d.Network,
		d.VAAmount, d.FiatAmount, d.FiatCurrency, d.ExchangeRate, d.KVVO, d.ClientWallet,
		d.OperatorWallet, d.CommissionFiat, d.CommissionVA, d.Transactions,
		d.DocxFileID, d.PDFFileID, d.DocxName, d.PDFName,
		d.Status, d.Version, d.UpdatedAt, d.CreatedBy, d.BuybackID,
		d.TaxPercent, d.FeePercent, d.ReferrerName, d.ReferralPercent,
		d.ClientBankID, d.OperatorBankID}
}

func normalizeDeal(d *Deal) {
	if d.Transactions == "" {
		d.Transactions = "[]"
	}
	if d.FiatCurrency == "" {
		d.FiatCurrency = "RUB"
	}
	if d.Status == "" {
		d.Status = "active"
	}
	if d.Version == 0 {
		d.Version = 1
	}
}

func (s *Store) AddDeal(ctx context.Context, d *Deal) (int64, error) {
	normalizeDeal(d)
	args := dealArgs(d)
	marks := strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")
	res, err := s.db.ExecContext(ctx, "INSERT INTO deals ("+dealCols+") VALUES ("+marks+")", args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateDeal перезаписывает сделку (переделка заявки): версия +1, статус и
// привязка к откупке сохраняются.
func (s *Store) UpdateDeal(ctx context.Context, d *Deal) error {
	normalizeDeal(d)
	cols := strings.Split(dealCols, ",")
	args := dealArgs(d)
	var set []string
	var vals []any
	for i, c := range cols {
		c = strings.TrimSpace(c)
		switch c {
		case "status", "version", "buyback_id", "created_by", "updated_at":
			continue
		}
		set = append(set, c+"=?")
		vals = append(vals, args[i])
	}
	set = append(set, "version=version+1", "updated_at=?")
	vals = append(vals, nowStamp(), d.ID)
	_, err := s.db.ExecContext(ctx, "UPDATE deals SET "+strings.Join(set, ", ")+" WHERE id=?", vals...)
	return err
}

func nowStamp() string { return time.Now().Format("2006-01-02 15:04:05") }

// SetDealStatus — отмена (cancelled) или восстановление (active).
func (s *Store) SetDealStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE deals SET status=?, updated_at=? WHERE id=?", status, nowStamp(), id)
	return err
}

// LinkDealsToBuyback привязывает сделки к откупке (buybackID=0 — отвязать).
func (s *Store) LinkDealsToBuyback(ctx context.Context, buybackID int64, dealIDs ...int64) error {
	var bb sql.NullInt64
	if buybackID != 0 {
		bb = sql.NullInt64{Int64: buybackID, Valid: true}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range dealIDs {
		if _, err := tx.ExecContext(ctx, "UPDATE deals SET buyback_id=?, updated_at=? WHERE id=?", bb, nowStamp(), id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func scanDeal(sc interface{ Scan(...any) error }) (*Deal, error) {
	var d Deal
	var s [22]Str
	var va, fiat Float
	var status, updated, createdBy, refName, supplier Str
	var version Float
	var tax, fee, refPct, buyRate Float
	err := sc.Scan(&d.ID, &s[0], &s[1], &s[2], &s[3], &s[4], &d.CompanyID, &s[5], &s[6], &s[7],
		&s[8], &s[9], &va, &fiat, &s[10], &s[11], &s[12], &s[13], &s[14], &s[15], &s[16],
		&s[17], &s[18], &s[19], &s[20], &s[21],
		&status, &version, &updated, &createdBy, &d.BuybackID, &tax, &fee, &refName, &refPct,
		&d.ClientBankID, &d.OperatorBankID, &buyRate, &supplier)
	if err != nil {
		return nil, err
	}
	d.ActType, d.DealNumber, d.DealDate, d.DealDateISO, d.ExecutionDate =
		string(s[0]), string(s[1]), string(s[2]), string(s[3]), string(s[4])
	d.ClientName, d.ClientINN, d.ClientResident, d.VAType, d.Network =
		string(s[5]), string(s[6]), string(s[7]), string(s[8]), string(s[9])
	d.VAAmount, d.FiatAmount = float64(va), float64(fiat)
	d.FiatCurrency, d.ExchangeRate, d.KVVO, d.ClientWallet, d.OperatorWallet =
		string(s[10]), string(s[11]), string(s[12]), string(s[13]), string(s[14])
	d.CommissionFiat, d.CommissionVA, d.Transactions =
		string(s[15]), string(s[16]), string(s[17])
	d.DocxFileID, d.PDFFileID, d.DocxName, d.PDFName =
		string(s[18]), string(s[19]), string(s[20]), string(s[21])
	d.Status, d.Version, d.UpdatedAt, d.CreatedBy = string(status), int(version), string(updated), string(createdBy)
	d.TaxPercent, d.FeePercent, d.ReferrerName, d.ReferralPercent = float64(tax), float64(fee), string(refName), float64(refPct)
	d.BuyRate, d.SupplierName = float64(buyRate), string(supplier)
	normalizeDeal(&d)
	return &d, nil
}

func (s *Store) queryDeals(ctx context.Context, where string, args ...any) ([]*Deal, error) {
	// курс откупки и поставщик — из активной связанной откупки
	q := "SELECT d.id, " + prefixCols("d.", dealCols) + `,
		CASE WHEN b.status='active' THEN b.rate ELSE 0 END,
		CASE WHEN b.status='active' THEN b.supplier_name ELSE '' END
		FROM deals d LEFT JOIN buybacks b ON b.id = d.buyback_id ` + where
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Deal
	for rows.Next() {
		d, err := scanDeal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Deal возвращает сделку или nil.
func (s *Store) Deal(ctx context.Context, id int64) (*Deal, error) {
	ds, err := s.queryDeals(ctx, "WHERE d.id=?", id)
	if err != nil || len(ds) == 0 {
		return nil, err
	}
	return ds[0], nil
}

// RecentDeals — последние сделки (опционально по клиенту).
func (s *Store) RecentDeals(ctx context.Context, limit int, companyID int64) ([]*Deal, error) {
	if companyID != 0 {
		return s.queryDeals(ctx, "WHERE d.company_id=? ORDER BY d.id DESC LIMIT ?", companyID, limit)
	}
	return s.queryDeals(ctx, "ORDER BY d.id DESC LIMIT ?", limit)
}

// DealsForMonth — активные сделки месяца 'YYYY-MM' по дате заявки.
func (s *Store) DealsForMonth(ctx context.Context, month string) ([]*Deal, error) {
	return s.queryDeals(ctx, "WHERE substr(d.deal_date_iso,1,7)=? AND d.status='active' ORDER BY d.deal_date_iso, d.id", month)
}

// DealsForDate — активные сделки за день (dateISO = YYYY-MM-DD).
func (s *Store) DealsForDate(ctx context.Context, dateISO string) ([]*Deal, error) {
	return s.queryDeals(ctx, "WHERE d.deal_date_iso=? AND d.status='active' ORDER BY d.id", dateISO)
}

// DealsOfBuyback — активные сделки, закрытые откупкой.
func (s *Store) DealsOfBuyback(ctx context.Context, buybackID int64) ([]*Deal, error) {
	return s.queryDeals(ctx, "WHERE d.buyback_id=? AND d.status='active' ORDER BY d.id", buybackID)
}

// OpenSellDeals — активные продажи клиентам, ещё не закрытые откупкой
// (USDT выдан «наперёд» из прибыли или откупка ещё не сделана).
func (s *Store) OpenSellDeals(ctx context.Context, limit int) ([]*Deal, error) {
	return s.queryDeals(ctx, `WHERE d.act_type='sell' AND d.status='active'
		AND (d.buyback_id IS NULL OR b.id IS NULL OR b.status!='active')
		ORDER BY d.id DESC LIMIT ?`, limit)
}

// PurgeDeals удаляет все сделки, их подписанные документы, откупки и
// уведомления. Компании, реквизиты, шаблоны и счётчики номеров не трогаются.
func (s *Store) PurgeDeals(ctx context.Context) (deals, signed int64, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	r, err := tx.ExecContext(ctx, "DELETE FROM deals")
	if err != nil {
		return 0, 0, err
	}
	deals, _ = r.RowsAffected()
	r, err = tx.ExecContext(ctx, "DELETE FROM signed_docs WHERE deal_id IS NOT NULL")
	if err != nil {
		return 0, 0, err
	}
	signed, _ = r.RowsAffected()
	for _, q := range []string{"DELETE FROM buybacks", "DELETE FROM notifications"} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return 0, 0, err
		}
	}
	return deals, signed, tx.Commit()
}

func prefixCols(prefix, cols string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = prefix + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

// DealMonths — месяцы 'YYYY-MM', по которым есть сделки (новые сверху, до 12).
func (s *Store) DealMonths(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT substr(deal_date_iso,1,7) AS m FROM deals
		WHERE deal_date_iso != '' AND status='active' ORDER BY m DESC LIMIT 12`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var m Str
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		out = append(out, string(m))
	}
	return out, rows.Err()
}

func (s *Store) DeleteDeal(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM deals WHERE id=?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM signed_docs WHERE deal_id=?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM notifications WHERE entity='deal' AND entity_id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// ─── Подписанные документы (хранится только file_id Telegram) ────────────────

type SignedDoc struct {
	ID       int64
	FileID   string
	FileName string
}

func (s *Store) AddSignedDoc(ctx context.Context, companyID, dealID sql.NullInt64, fileID, fileName string) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO signed_docs (company_id, deal_id, file_id, file_name) VALUES (?,?,?,?)",
		companyID, dealID, fileID, fileName)
	return err
}

func (s *Store) signedDocs(ctx context.Context, where string, arg any) ([]SignedDoc, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, file_id, file_name FROM signed_docs WHERE "+where+" ORDER BY uploaded_at, id", arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SignedDoc
	for rows.Next() {
		var d SignedDoc
		var fid, name Str
		if err := rows.Scan(&d.ID, &fid, &name); err != nil {
			return nil, err
		}
		d.FileID, d.FileName = string(fid), string(name)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) SignedDocsByDeal(ctx context.Context, dealID int64) ([]SignedDoc, error) {
	return s.signedDocs(ctx, "deal_id=?", dealID)
}

func (s *Store) SignedDocsByCompany(ctx context.Context, companyID int64) ([]SignedDoc, error) {
	return s.signedDocs(ctx, "company_id=?", companyID)
}

func (s *Store) CountSignedDocs(ctx context.Context, dealID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM signed_docs WHERE deal_id=?", dealID).Scan(&n)
	return n, err
}
