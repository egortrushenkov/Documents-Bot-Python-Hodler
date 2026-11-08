package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Владелец реквизитов/кошельков: клиент (company, id) или мы (operator, 0).
const (
	OwnerCompany  = "company"
	OwnerOperator = "operator"
	OwnerSupplier = "supplier"
)

type BankAccount struct {
	ID        int64
	OwnerType string
	OwnerID   int64
	Label     string
	BankName  string
	Account   string
	BIK       string
	IsDefault bool
}

// Title — для кнопок: «Основной · Банк ··4602».
func (b *BankAccount) Title() string {
	acc := b.Account
	if r := []rune(acc); len(r) > 4 {
		acc = "··" + string(r[len(r)-4:])
	}
	parts := []string{}
	for _, p := range []string{b.Label, b.BankName, acc} {
		if strings.TrimSpace(p) != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

type Wallet struct {
	ID         int64
	OwnerType  string
	OwnerID    int64
	Label      string
	Network    string
	Address    string
	IsDefault  bool
	WatchIn    bool
	WatchOut   bool
	MinAmount  float64
	WatchSince int64
}

// Short — «TXFE…Ze3r».
func (w *Wallet) Short() string { return ShortAddr(w.Address) }

func ShortAddr(a string) string {
	if len(a) <= 12 {
		return a
	}
	return a[:4] + "…" + a[len(a)-4:]
}

// Title — «Основной · TRC-20 · TXFE…Ze3r».
func (w *Wallet) Title() string {
	parts := []string{}
	for _, p := range []string{w.Label, w.Network, w.Short()} {
		if strings.TrimSpace(p) != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ─── Банковские реквизиты ─────────────────────────────────────────────────────

const bankCols = "id, owner_type, owner_id, label, bank_name, account, bik, is_default"

func scanBank(sc interface{ Scan(...any) error }) (*BankAccount, error) {
	var b BankAccount
	var ot, label, name, acc, bik Str
	var def int
	if err := sc.Scan(&b.ID, &ot, &b.OwnerID, &label, &name, &acc, &bik, &def); err != nil {
		return nil, err
	}
	b.OwnerType, b.Label, b.BankName, b.Account, b.BIK, b.IsDefault = string(ot), string(label), string(name), string(acc), string(bik), def == 1
	return &b, nil
}

func (s *Store) BankAccounts(ctx context.Context, ownerType string, ownerID int64) ([]*BankAccount, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+bankCols+" FROM bank_accounts WHERE owner_type=? AND owner_id=? ORDER BY is_default DESC, id",
		ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*BankAccount
	for rows.Next() {
		b, err := scanBank(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) BankAccount(ctx context.Context, id int64) (*BankAccount, error) {
	b, err := scanBank(s.db.QueryRowContext(ctx, "SELECT "+bankCols+" FROM bank_accounts WHERE id=?", id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return b, err
}

// AddBankAccount добавляет реквизиты; первые у владельца становятся основными.
func (s *Store) AddBankAccount(ctx context.Context, b *BankAccount) (int64, error) {
	var n int
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM bank_accounts WHERE owner_type=? AND owner_id=?", b.OwnerType, b.OwnerID).Scan(&n)
	res, err := s.db.ExecContext(ctx, `INSERT INTO bank_accounts (owner_type, owner_id, label, bank_name, account, bik, is_default)
		VALUES (?,?,?,?,?,?,0)`, b.OwnerType, b.OwnerID, b.Label, b.BankName, b.Account, b.BIK)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if n == 0 || b.IsDefault {
		return id, s.SetDefaultBank(ctx, id)
	}
	return id, nil
}

var bankFields = map[string]bool{"label": true, "bank_name": true, "account": true, "bik": true}

func (s *Store) UpdateBankField(ctx context.Context, id int64, field, value string) error {
	if !bankFields[field] {
		return fmt.Errorf("неизвестное поле %q", field)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE bank_accounts SET "+field+"=? WHERE id=?", value, id); err != nil {
		return err
	}
	b, err := s.BankAccount(ctx, id)
	if err == nil && b != nil && b.IsDefault {
		return s.syncDefaultBank(ctx, b)
	}
	return err
}

// SetDefaultBank делает реквизиты основными и копирует их в «старые» поля
// (companies.bank_* / реквизиты оператора) — их используют шаблоны и отчёты.
func (s *Store) SetDefaultBank(ctx context.Context, id int64) error {
	b, err := s.BankAccount(ctx, id)
	if err != nil || b == nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE bank_accounts SET is_default=(id=?) WHERE owner_type=? AND owner_id=?",
		id, b.OwnerType, b.OwnerID); err != nil {
		return err
	}
	return s.syncDefaultBank(ctx, b)
}

func (s *Store) syncDefaultBank(ctx context.Context, b *BankAccount) error {
	if b.OwnerType == OwnerOperator {
		for k, v := range map[string]string{"bank_name": b.BankName, "bank_account": b.Account, "bank_bik": b.BIK} {
			if err := s.SetSetting(ctx, k, v); err != nil {
				return err
			}
		}
		return nil
	}
	if b.OwnerType != OwnerCompany { // у поставщиков «старых» полей нет
		return nil
	}
	_, err := s.db.ExecContext(ctx, "UPDATE companies SET bank_name=?, bank_account=?, bank_bik=? WHERE id=?",
		b.BankName, b.Account, b.BIK, b.OwnerID)
	return err
}

func (s *Store) DeleteBankAccount(ctx context.Context, id int64) error {
	b, err := s.BankAccount(ctx, id)
	if err != nil || b == nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM bank_accounts WHERE id=?", id); err != nil {
		return err
	}
	if b.IsDefault { // основными становятся следующие
		rest, _ := s.BankAccounts(ctx, b.OwnerType, b.OwnerID)
		if len(rest) > 0 {
			return s.SetDefaultBank(ctx, rest[0].ID)
		}
	}
	return nil
}

// ─── Кошельки ─────────────────────────────────────────────────────────────────

const walletCols = "id, owner_type, owner_id, label, network, address, is_default, watch_in, watch_out, min_amount, watch_since"

func scanWallet(sc interface{ Scan(...any) error }) (*Wallet, error) {
	var w Wallet
	var ot, label, net, addr Str
	var def, in, out int
	var min Float
	if err := sc.Scan(&w.ID, &ot, &w.OwnerID, &label, &net, &addr, &def, &in, &out, &min, &w.WatchSince); err != nil {
		return nil, err
	}
	w.OwnerType, w.Label, w.Network, w.Address = string(ot), string(label), string(net), string(addr)
	w.IsDefault, w.WatchIn, w.WatchOut, w.MinAmount = def == 1, in == 1, out == 1, float64(min)
	return &w, nil
}

func (s *Store) queryWallets(ctx context.Context, where string, args ...any) ([]*Wallet, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+walletCols+" FROM wallets "+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Wallet
	for rows.Next() {
		w, err := scanWallet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) Wallets(ctx context.Context, ownerType string, ownerID int64) ([]*Wallet, error) {
	return s.queryWallets(ctx, "WHERE owner_type=? AND owner_id=? ORDER BY is_default DESC, id", ownerType, ownerID)
}

func (s *Store) Wallet(ctx context.Context, id int64) (*Wallet, error) {
	ws, err := s.queryWallets(ctx, "WHERE id=?", id)
	if err != nil || len(ws) == 0 {
		return nil, err
	}
	return ws[0], nil
}

// WatchedWallets — кошельки оператора, по которым включены уведомления.
func (s *Store) WatchedWallets(ctx context.Context) ([]*Wallet, error) {
	return s.queryWallets(ctx, "WHERE owner_type='operator' AND (watch_in=1 OR watch_out=1) ORDER BY id")
}

// WalletsByAddress — чьи это кошельки (для подписи отправителя).
func (s *Store) WalletsByAddress(ctx context.Context, address string) ([]*Wallet, error) {
	return s.queryWallets(ctx, "WHERE address=? COLLATE NOCASE", strings.TrimSpace(address))
}

// AddWallet добавляет кошелёк; первый у владельца становится основным.
// ErrDuplicateWallet — такой адрес у владельца уже есть.
var ErrDuplicateWallet = fmt.Errorf("такой кошелёк уже добавлен")

func (s *Store) AddWallet(ctx context.Context, w *Wallet) (int64, error) {
	var dup int
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM wallets WHERE owner_type=? AND owner_id=? AND address=? COLLATE NOCASE",
		w.OwnerType, w.OwnerID, strings.TrimSpace(w.Address)).Scan(&dup)
	if dup > 0 {
		return 0, ErrDuplicateWallet
	}
	var n int
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM wallets WHERE owner_type=? AND owner_id=?", w.OwnerType, w.OwnerID).Scan(&n)
	res, err := s.db.ExecContext(ctx, `INSERT INTO wallets
		(owner_type, owner_id, label, network, address, is_default, watch_in, watch_out, min_amount, watch_since)
		VALUES (?,?,?,?,?,0,?,?,?,?)`, w.OwnerType, w.OwnerID, w.Label, w.Network, strings.TrimSpace(w.Address),
		b2i(w.WatchIn), b2i(w.WatchOut), w.MinAmount, w.WatchSince)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if n == 0 || w.IsDefault {
		return id, s.SetDefaultWallet(ctx, id)
	}
	return id, nil
}

var walletFields = map[string]bool{"label": true, "network": true, "address": true, "min_amount": true}

func (s *Store) UpdateWalletField(ctx context.Context, id int64, field string, value any) error {
	if !walletFields[field] {
		return fmt.Errorf("неизвестное поле %q", field)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE wallets SET "+field+"=? WHERE id=?", value, id); err != nil {
		return err
	}
	w, err := s.Wallet(ctx, id)
	if err == nil && w != nil && w.IsDefault {
		return s.syncDefaultWallet(ctx, w)
	}
	return err
}

// SetWalletWatch включает/выключает уведомления; при включении отслеживание
// начинается «с сейчас» (старые транзакции не присылаются).
func (s *Store) SetWalletWatch(ctx context.Context, id int64, in, out bool, sinceMs int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE wallets SET watch_in=?, watch_out=?,
		watch_since=CASE WHEN watch_since=0 OR (watch_in=0 AND watch_out=0) THEN ? ELSE watch_since END WHERE id=?`,
		b2i(in), b2i(out), sinceMs, id)
	return err
}

func (s *Store) SetDefaultWallet(ctx context.Context, id int64) error {
	w, err := s.Wallet(ctx, id)
	if err != nil || w == nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE wallets SET is_default=(id=?) WHERE owner_type=? AND owner_id=?",
		id, w.OwnerType, w.OwnerID); err != nil {
		return err
	}
	return s.syncDefaultWallet(ctx, w)
}

func (s *Store) syncDefaultWallet(ctx context.Context, w *Wallet) error {
	if w.OwnerType == OwnerOperator {
		return s.SetSetting(ctx, "wallet", w.Address)
	}
	if w.OwnerType != OwnerCompany {
		return nil
	}
	_, err := s.db.ExecContext(ctx, "UPDATE companies SET wallet=? WHERE id=?", w.Address, w.OwnerID)
	return err
}

func (s *Store) DeleteWallet(ctx context.Context, id int64) error {
	w, err := s.Wallet(ctx, id)
	if err != nil || w == nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM wallets WHERE id=?", id); err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, "DELETE FROM wallet_txs WHERE wallet_id=?", id)
	if w.IsDefault {
		rest, _ := s.Wallets(ctx, w.OwnerType, w.OwnerID)
		if len(rest) > 0 {
			return s.SetDefaultWallet(ctx, rest[0].ID)
		}
	}
	return nil
}

// ─── Увиденные транзакции ─────────────────────────────────────────────────────

type WalletTx struct {
	Hash         string
	WalletID     int64
	Direction    string
	Amount       float64
	Counterparty string
	TS           int64
}

// MarkWalletTx запоминает транзакцию; true — новая (ещё не видели).
func (s *Store) MarkWalletTx(ctx context.Context, t *WalletTx, notified bool) (bool, error) {
	res, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO wallet_txs
		(hash, wallet_id, direction, amount, counterparty, ts, notified) VALUES (?,?,?,?,?,?,?)`,
		t.Hash, t.WalletID, t.Direction, t.Amount, t.Counterparty, t.TS, b2i(notified))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SyncCompanyLegacy создаёт записи реквизитов/кошелька из полей компании,
// если у неё их ещё нет (новая компания из мастера добавления).
func (s *Store) SyncCompanyLegacy(ctx context.Context, c *Company) error {
	if banks, _ := s.BankAccounts(ctx, OwnerCompany, c.ID); len(banks) == 0 &&
		strings.Trim(c.BankName+c.BankAccount+c.BankBIK, "- ") != "" {
		if _, err := s.AddBankAccount(ctx, &BankAccount{OwnerType: OwnerCompany, OwnerID: c.ID, Label: "Основной",
			BankName: c.BankName, Account: c.BankAccount, BIK: c.BankBIK}); err != nil {
			return err
		}
	}
	if ws, _ := s.Wallets(ctx, OwnerCompany, c.ID); len(ws) == 0 && strings.Trim(c.Wallet, "- ") != "" {
		if _, err := s.AddWallet(ctx, &Wallet{OwnerType: OwnerCompany, OwnerID: c.ID, Label: "Основной",
			Network: GuessNetwork(c.Wallet), Address: c.Wallet}); err != nil {
			return err
		}
	}
	return nil
}

// GuessNetwork — сеть по виду адреса: T… — TRC-20, 0x… — ERC-20.
func GuessNetwork(addr string) string {
	a := strings.TrimSpace(addr)
	switch {
	case strings.HasPrefix(a, "T") && len(a) == 34:
		return "TRC-20"
	case strings.HasPrefix(a, "0x") && len(a) == 42:
		return "ERC-20"
	}
	return ""
}

// DeleteCompanyAccounts — при удалении компании.
func (s *Store) DeleteCompanyAccounts(ctx context.Context, companyID int64) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM bank_accounts WHERE owner_type='company' AND owner_id=?", companyID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM wallets WHERE owner_type='company' AND owner_id=?", companyID)
	return err
}
