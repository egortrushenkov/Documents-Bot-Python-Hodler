// Package storage — SQLite-хранилище бота: компании, реквизиты, журнал сделок,
// счётчики номеров, подписанные документы. Схема совместима с Python-версией.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite" // чистый Go, без cgo
)

type Store struct {
	db   *sql.DB
	path string
}

// Open открывает базу (WAL, busy_timeout — несколько сотрудников пишут параллельно).
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(30000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_txlock", "immediate")
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, path: path}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// DB — доступ к *sql.DB (для тестов и утилит).
func (s *Store) DB() *sql.DB { return s.db }

// ─── Сканеры, устойчивые к динамической типизации SQLite ──────────────────────

// Str принимает из SQLite что угодно (NULL, число, текст) и хранит строку.
type Str string

func (s *Str) Scan(v any) error {
	switch x := v.(type) {
	case nil:
		*s = ""
	case string:
		*s = Str(x)
	case []byte:
		*s = Str(x)
	case int64:
		*s = Str(strconv.FormatInt(x, 10))
	case float64:
		*s = Str(strconv.FormatFloat(x, 'f', -1, 64))
	case bool:
		*s = Str(strconv.FormatBool(x))
	case time.Time:
		*s = Str(x.Format("2006-01-02 15:04:05"))
	default:
		*s = Str(fmt.Sprint(x))
	}
	return nil
}

// Float принимает число или текст с числом.
type Float float64

func (f *Float) Scan(v any) error {
	switch x := v.(type) {
	case nil:
		*f = 0
	case float64:
		*f = Float(x)
	case int64:
		*f = Float(x)
	case string:
		n, _ := strconv.ParseFloat(x, 64)
		*f = Float(n)
	case []byte:
		n, _ := strconv.ParseFloat(string(x), 64)
		*f = Float(n)
	default:
		*f = 0
	}
	return nil
}

// ─── Компании ─────────────────────────────────────────────────────────────────

type Company struct {
	ID          int64
	FullName    string
	ShortName   string
	INN         string
	KPP         string
	RegNumber   string
	Address     string
	KIO         string
	INNRF       string
	KPPRF       string
	BankName    string
	BankAccount string
	BankBIK     string
	Wallet      string
	Resident    string
}

// CompanyColumns — редактируемые поля компании (белый список для UPDATE).
var CompanyColumns = []string{
	"full_name", "short_name", "inn", "kpp", "reg_number", "address", "kio",
	"inn_rf", "kpp_rf", "bank_name", "bank_account", "bank_bik", "wallet", "resident",
}

// Map — реквизиты в виде ключ → значение (ключи как колонки БД).
func (c *Company) Map() map[string]string {
	return map[string]string{
		"id": strconv.FormatInt(c.ID, 10), "full_name": c.FullName, "short_name": c.ShortName,
		"inn": c.INN, "kpp": c.KPP, "reg_number": c.RegNumber, "address": c.Address,
		"kio": c.KIO, "inn_rf": c.INNRF, "kpp_rf": c.KPPRF, "bank_name": c.BankName,
		"bank_account": c.BankAccount, "bank_bik": c.BankBIK, "wallet": c.Wallet,
		"resident": c.Resident,
	}
}

const companySelect = `SELECT id, full_name, short_name, inn, kpp, reg_number, address, kio,
	inn_rf, kpp_rf, bank_name, bank_account, bank_bik, wallet, resident FROM companies`

func scanCompany(sc interface{ Scan(...any) error }) (*Company, error) {
	var c Company
	var f [14]Str
	if err := sc.Scan(&c.ID, &f[0], &f[1], &f[2], &f[3], &f[4], &f[5], &f[6], &f[7],
		&f[8], &f[9], &f[10], &f[11], &f[12], &f[13]); err != nil {
		return nil, err
	}
	c.FullName, c.ShortName, c.INN, c.KPP = string(f[0]), string(f[1]), string(f[2]), string(f[3])
	c.RegNumber, c.Address, c.KIO, c.INNRF = string(f[4]), string(f[5]), string(f[6]), string(f[7])
	c.KPPRF, c.BankName, c.BankAccount, c.BankBIK = string(f[8]), string(f[9]), string(f[10]), string(f[11])
	c.Wallet, c.Resident = string(f[12]), string(f[13])
	return &c, nil
}

func (s *Store) Companies(ctx context.Context) ([]*Company, error) {
	rows, err := s.db.QueryContext(ctx, companySelect+" ORDER BY short_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Company
	for rows.Next() {
		c, err := scanCompany(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Company возвращает компанию или nil, если её нет.
func (s *Store) Company(ctx context.Context, id int64) (*Company, error) {
	c, err := scanCompany(s.db.QueryRowContext(ctx, companySelect+" WHERE id=?", id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func orDefault(m map[string]string, k, def string) string {
	if v, ok := m[k]; ok {
		return v
	}
	return def
}

func (s *Store) AddCompany(ctx context.Context, d map[string]string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO companies
		(full_name,short_name,inn,kpp,reg_number,address,kio,inn_rf,kpp_rf,
		 bank_name,bank_account,bank_bik,wallet,resident)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		d["full_name"], d["short_name"], d["inn"], d["kpp"], d["reg_number"], d["address"],
		orDefault(d, "kio", "-"), orDefault(d, "inn_rf", "-"), orDefault(d, "kpp_rf", "-"),
		d["bank_name"], d["bank_account"], d["bank_bik"], d["wallet"],
		orDefault(d, "resident", "Резидент"))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateCompanyField меняет одно поле (только из белого списка CompanyColumns).
func (s *Store) UpdateCompanyField(ctx context.Context, id int64, field, value string) error {
	ok := false
	for _, c := range CompanyColumns {
		if c == field {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("неизвестное поле компании: %q", field)
	}
	_, err := s.db.ExecContext(ctx, "UPDATE companies SET "+field+"=? WHERE id=?", value, id)
	return err
}

func (s *Store) DeleteCompany(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		"DELETE FROM companies WHERE id=?",
		"DELETE FROM company_custom WHERE company_id=?",
		"DELETE FROM company_counters WHERE company_id=?",
	} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ─── Свои переменные компаний ─────────────────────────────────────────────────

type CustomField struct {
	Key   string
	Label string
}

func (s *Store) CustomFields(ctx context.Context) ([]CustomField, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT key, label FROM custom_fields ORDER BY created_at, rowid")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomField
	for rows.Next() {
		var k, l Str
		if err := rows.Scan(&k, &l); err != nil {
			return nil, err
		}
		out = append(out, CustomField{Key: string(k), Label: string(l)})
	}
	return out, rows.Err()
}

func (s *Store) AddCustomField(ctx context.Context, key, label string) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO custom_fields (key, label) VALUES (?,?)", key, label)
	return err
}

func (s *Store) DeleteCustomField(ctx context.Context, key string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM custom_fields WHERE key=?", key); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM company_custom WHERE key=?", key); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CompanyCustom(ctx context.Context, companyID int64) (map[string]string, error) {
	return s.kv(ctx, "SELECT key, value FROM company_custom WHERE company_id=?", companyID)
}

func (s *Store) SetCompanyCustom(ctx context.Context, companyID int64, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO company_custom (company_id, key, value) VALUES (?,?,?)",
		companyID, key, value)
	return err
}

func (s *Store) kv(ctx context.Context, q string, args ...any) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v Str
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[string(k)] = string(v)
	}
	return out, rows.Err()
}

// ─── Реквизиты оператора ──────────────────────────────────────────────────────

func (s *Store) Settings(ctx context.Context) (map[string]string, error) {
	return s.kv(ctx, "SELECT key, value FROM operator_settings")
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO operator_settings (key, value) VALUES (?,?)", key, value)
	return err
}

// ─── Нумерация (общий счётчик + по клиентам) ──────────────────────────────────

// PeekNextNumber — следующий общий номер без инкремента.
func (s *Store) PeekNextNumber(ctx context.Context) (int64, error) {
	var n sql.NullInt64
	err := s.db.QueryRowContext(ctx, "SELECT last_number FROM act_counter WHERE id=1").Scan(&n)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	if !n.Valid {
		return 500, nil
	}
	return n.Int64 + 1, nil
}

// PeekCompanyNumber — следующий номер клиента (kind: act|invoice); ok=false, если истории нет.
func (s *Store) PeekCompanyNumber(ctx context.Context, companyID int64, kind string) (int64, bool, error) {
	var n sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		"SELECT last_number FROM company_counters WHERE company_id=? AND kind=?", companyID, kind).Scan(&n)
	if err == sql.ErrNoRows || (err == nil && !n.Valid) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return n.Int64 + 1, true, nil
}

// RecordNumberUsed двигает счётчик клиента и общий счётчик вперёд (назад не откатывает).
func (s *Store) RecordNumberUsed(ctx context.Context, companyID int64, kind string, n int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO company_counters (company_id, kind, last_number)
		VALUES (?,?,?)
		ON CONFLICT(company_id, kind)
		DO UPDATE SET last_number = MAX(last_number, excluded.last_number)`,
		companyID, kind, n); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE act_counter SET last_number = MAX(last_number, ?) WHERE id=1", n); err != nil {
		return err
	}
	return tx.Commit()
}

// ─── Свои коды КВВО ───────────────────────────────────────────────────────────

func (s *Store) KVVOCodes(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT code FROM kvvo_codes ORDER BY code")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c Str
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, string(c))
	}
	return out, rows.Err()
}

func (s *Store) AddKVVO(ctx context.Context, code string) error {
	_, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO kvvo_codes (code) VALUES (?)", code)
	return err
}

func (s *Store) DeleteKVVO(ctx context.Context, code string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM kvvo_codes WHERE code=?", code)
	return err
}
