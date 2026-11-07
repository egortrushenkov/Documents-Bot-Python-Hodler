package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Схема Python-версии до появления колонки resident + немного данных.
const legacySchema = `
CREATE TABLE companies (id INTEGER PRIMARY KEY AUTOINCREMENT, full_name TEXT NOT NULL, short_name TEXT NOT NULL,
  inn TEXT DEFAULT '', kpp TEXT DEFAULT '', reg_number TEXT DEFAULT '', address TEXT DEFAULT '',
  kio TEXT DEFAULT '-', inn_rf TEXT DEFAULT '-', kpp_rf TEXT DEFAULT '-', bank_name TEXT DEFAULT '',
  bank_account TEXT DEFAULT '', bank_bik TEXT DEFAULT '', wallet TEXT DEFAULT '', created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE operator_settings (key TEXT PRIMARY KEY, value TEXT);
CREATE TABLE act_counter (id INTEGER PRIMARY KEY, last_number INTEGER DEFAULT 499);
INSERT INTO act_counter VALUES (1, 700);
INSERT INTO operator_settings VALUES ('short_name', 'Мой оператор');
INSERT INTO companies (full_name, short_name, inn) VALUES ('ООО Тест', 'Тест', '123');
`

func TestMigrateLegacyPythonDB(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "hodler.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.DB().Exec(legacySchema); err != nil {
		t.Fatal(err)
	}

	n, err := st.Migrate(ctx, true)
	if err != nil || n != len(Migrations) {
		t.Fatalf("migrate: n=%d err=%v", n, err)
	}
	// Повторный запуск — ничего не делает
	if n, err := st.Migrate(ctx, true); err != nil || n != 0 {
		t.Fatalf("second migrate: n=%d err=%v", n, err)
	}
	// Резервная копия создана
	matches, _ := filepath.Glob(path + ".backup-*")
	if len(matches) != 1 {
		t.Fatalf("ожидалась 1 резервная копия, есть %v", matches)
	}
	if fi, err := os.Stat(matches[0]); err != nil || fi.Size() == 0 {
		t.Fatalf("пустая копия: %v", err)
	}

	c, err := st.Company(ctx, 1)
	if err != nil || c == nil || c.Resident != "Резидент" || c.ShortName != "Тест" {
		t.Fatalf("company after migrate: %+v err=%v", c, err)
	}
	s, _ := st.Settings(ctx)
	if s["short_name"] != "Мой оператор" {
		t.Errorf("пользовательская настройка перезаписана: %q", s["short_name"])
	}
	if s["inn"] != "9909730748" {
		t.Errorf("значение по умолчанию не добавлено: %q", s["inn"])
	}
	if next, _ := st.PeekNextNumber(ctx); next != 701 {
		t.Errorf("счётчик: %d", next)
	}
	// Новые таблицы из базовой схемы доступны
	if _, err := st.AddDeal(ctx, &Deal{ActType: "sell", DealDate: "01.02.2026", DealDateISO: "2026-02-01"}); err != nil {
		t.Fatal(err)
	}
	if months, _ := st.DealMonths(ctx); len(months) != 1 || months[0] != "2026-02" {
		t.Errorf("months=%v", months)
	}
}

func TestMigrateFreshDB(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "new.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.Migrate(ctx, true); err != nil {
		t.Fatal(err)
	}
	status, _ := st.Status(ctx)
	for _, m := range status {
		if m.AppliedAt == "" {
			t.Errorf("миграция %d не применена", m.Version)
		}
	}
	id, err := st.AddCompany(ctx, map[string]string{"full_name": "A", "short_name": "A"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RecordNumberUsed(ctx, id, "act", 42); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordNumberUsed(ctx, id, "act", 10); err != nil { // назад не откатывается
		t.Fatal(err)
	}
	if n, ok, _ := st.PeekCompanyNumber(ctx, id, "act"); !ok || n != 43 {
		t.Errorf("company counter %d %v", n, ok)
	}
	if err := st.UpdateCompanyField(ctx, id, "inn; DROP TABLE companies", "x"); err == nil {
		t.Error("поле вне белого списка должно отклоняться")
	}
}

func TestMigrateAccountsBackfill(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "hodler.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.DB().Exec(legacySchema + `
		UPDATE companies SET bank_name='Банк А', bank_account='40702810', bank_bik='044', wallet='TXFEYN4C5BnesaxUXJiXJHGS7K12QutZ3r';
		INSERT INTO companies (full_name, short_name, wallet) VALUES ('ООО Б', 'Б', '-');`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	banks, _ := st.BankAccounts(ctx, OwnerCompany, 1)
	ws, _ := st.Wallets(ctx, OwnerCompany, 1)
	if len(banks) != 1 || !banks[0].IsDefault || banks[0].Account != "40702810" {
		t.Fatalf("реквизиты клиента: %+v", banks)
	}
	if len(ws) != 1 || ws[0].Network != "TRC-20" {
		t.Fatalf("кошелёк клиента: %+v", ws)
	}
	if ws2, _ := st.Wallets(ctx, OwnerCompany, 2); len(ws2) != 0 {
		t.Fatal("кошелёк «-» не должен переноситься")
	}
	op, _ := st.Wallets(ctx, OwnerOperator, 0)
	if len(op) != 1 || !op[0].WatchIn || op[0].WatchSince == 0 {
		t.Fatalf("кошелёк оператора должен отслеживаться: %+v", op)
	}
	// Смена основного копируется в старые поля компании
	id, _ := st.AddBankAccount(ctx, &BankAccount{OwnerType: OwnerCompany, OwnerID: 1, Label: "Второй", Account: "999"})
	if err := st.SetDefaultBank(ctx, id); err != nil {
		t.Fatal(err)
	}
	if c, _ := st.Company(ctx, 1); c.BankAccount != "999" {
		t.Fatalf("старые поля не синхронизированы: %q", c.BankAccount)
	}
}
