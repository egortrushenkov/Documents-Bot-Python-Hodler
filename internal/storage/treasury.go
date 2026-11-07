package storage

import (
	"context"
	"database/sql"
)

// TreasuryOp — ручная операция с остатками.
type TreasuryOp struct {
	ID           int64
	OpDate       string // YYYY-MM-DD
	Kind         string
	USDT, Rub    float64
	ReferrerName string
	Note         string
	CreatedBy    string
}

const treasuryCols = "id, op_date, kind, usdt, rub, referrer_name, note, created_by"

func scanOp(sc interface{ Scan(...any) error }) (*TreasuryOp, error) {
	var o TreasuryOp
	var date, kind, ref, note, by Str
	var usdt, rub Float
	if err := sc.Scan(&o.ID, &date, &kind, &usdt, &rub, &ref, &note, &by); err != nil {
		return nil, err
	}
	o.OpDate, o.Kind, o.ReferrerName, o.Note, o.CreatedBy = string(date), string(kind), string(ref), string(note), string(by)
	o.USDT, o.Rub = float64(usdt), float64(rub)
	return &o, nil
}

func (s *Store) AddTreasuryOp(ctx context.Context, o *TreasuryOp) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO treasury_ops
		(op_date, kind, usdt, rub, referrer_name, note, created_by) VALUES (?,?,?,?,?,?,?)`,
		o.OpDate, o.Kind, o.USDT, o.Rub, o.ReferrerName, o.Note, o.CreatedBy)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SetOpening заменяет начальный остаток (он один).
func (s *Store) SetOpening(ctx context.Context, o *TreasuryOp) (int64, error) {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM treasury_ops WHERE kind='opening'"); err != nil {
		return 0, err
	}
	o.Kind = "opening"
	return s.AddTreasuryOp(ctx, o)
}

// Opening — начальный остаток или nil (учёт с начала журнала).
func (s *Store) Opening(ctx context.Context) (*TreasuryOp, error) {
	o, err := scanOp(s.db.QueryRowContext(ctx,
		"SELECT "+treasuryCols+" FROM treasury_ops WHERE kind='opening' ORDER BY id DESC LIMIT 1"))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return o, err
}

// TreasuryOps — операции (кроме начального остатка) с даты fromISO включительно.
func (s *Store) TreasuryOps(ctx context.Context, fromISO string) ([]*TreasuryOp, error) {
	return s.queryOps(ctx, "WHERE kind!='opening' AND op_date>=? ORDER BY op_date, id", fromISO)
}

func (s *Store) RecentTreasuryOps(ctx context.Context, limit int) ([]*TreasuryOp, error) {
	return s.queryOps(ctx, "WHERE kind!='opening' ORDER BY id DESC LIMIT ?", limit)
}

func (s *Store) TreasuryOp(ctx context.Context, id int64) (*TreasuryOp, error) {
	ops, err := s.queryOps(ctx, "WHERE id=?", id)
	if err != nil || len(ops) == 0 {
		return nil, err
	}
	return ops[0], nil
}

func (s *Store) DeleteTreasuryOp(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM treasury_ops WHERE id=? AND kind!='opening'", id)
	return err
}

func (s *Store) queryOps(ctx context.Context, where string, args ...any) ([]*TreasuryOp, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+treasuryCols+" FROM treasury_ops "+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*TreasuryOp
	for rows.Next() {
		o, err := scanOp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// DealsSince — активные сделки (без счетов-заявок) с даты fromISO.
func (s *Store) DealsSince(ctx context.Context, fromISO string) ([]*Deal, error) {
	return s.queryDeals(ctx, `WHERE d.status='active' AND d.act_type!='invoice'
		AND d.deal_date_iso!='' AND d.deal_date_iso>=? ORDER BY d.deal_date_iso, d.id`, fromISO)
}

// DealsBetween — активные сделки за период [from, to] (YYYY-MM-DD).
func (s *Store) DealsBetween(ctx context.Context, fromISO, toISO string) ([]*Deal, error) {
	return s.queryDeals(ctx, `WHERE d.status='active' AND d.deal_date_iso BETWEEN ? AND ?
		ORDER BY d.deal_date_iso, d.id`, fromISO, toISO)
}

// BuybacksSince — активные откупки с даты fromISO.
func (s *Store) BuybacksSince(ctx context.Context, fromISO string) ([]*Buyback, error) {
	return s.queryBuybacks(ctx, "WHERE status='active' AND buy_date_iso>=? ORDER BY buy_date_iso, id", fromISO)
}

// BuybacksBetween — активные откупки за период.
func (s *Store) BuybacksBetween(ctx context.Context, fromISO, toISO string) ([]*Buyback, error) {
	return s.queryBuybacks(ctx, "WHERE status='active' AND buy_date_iso BETWEEN ? AND ? ORDER BY buy_date_iso, id", fromISO, toISO)
}
