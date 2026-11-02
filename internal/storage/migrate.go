package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migration — один шаг эволюции схемы. Либо SQL-файл, либо Go-функция
// (когда нужна логика, которую не выразить в SQLite, например «добавить
// колонку, если её нет»).
type Migration struct {
	Version int
	Name    string
	SQLFile string
	Func    func(ctx context.Context, tx *sql.Tx) error
}

// Migrations — упорядоченный список миграций. Новые добавлять только в конец,
// уже выпущенные не менять.
var Migrations = []Migration{
	{Version: 1, Name: "baseline_schema", SQLFile: "migrations/0001_baseline_schema.sql"},
	{Version: 2, Name: "companies_resident", Func: migrateCompaniesResident},
	{Version: 3, Name: "seed_operator_defaults", SQLFile: "migrations/0003_seed_operator_defaults.sql"},
	{Version: 4, Name: "data_cleanup_and_indexes", SQLFile: "migrations/0004_data_cleanup_and_indexes.sql"},
}

// Колонка resident появилась в Python-версии позже — в старых базах её нет.
func migrateCompaniesResident(ctx context.Context, tx *sql.Tx) error {
	has, err := hasColumn(ctx, tx, "companies", "resident")
	if err != nil || has {
		return err
	}
	_, err = tx.ExecContext(ctx, `ALTER TABLE companies ADD COLUMN resident TEXT DEFAULT 'Резидент'`)
	return err
}

type queryer interface {
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
}

func hasColumn(ctx context.Context, q queryer, table, column string) (bool, error) {
	rows, err := q.QueryContext(ctx, "SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&n)
	return n > 0, err
}

// MigrationStatus — строка для `migrate -status`.
type MigrationStatus struct {
	Version   int
	Name      string
	AppliedAt string // пусто — не применена
}

func (s *Store) ensureMigrationsTable(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`)
	return err
}

func (s *Store) appliedVersions(ctx context.Context) (map[int]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT version, applied_at FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]string{}
	for rows.Next() {
		var v int
		var at string
		if err := rows.Scan(&v, &at); err != nil {
			return nil, err
		}
		out[v] = at
	}
	return out, rows.Err()
}

// Status возвращает состояние всех миграций.
func (s *Store) Status(ctx context.Context) ([]MigrationStatus, error) {
	applied := map[int]string{}
	tracked, err := tableExists(ctx, s.db, "schema_migrations")
	if err != nil {
		return nil, err
	}
	if tracked { // только чтение: таблицу не создаём
		if applied, err = s.appliedVersions(ctx); err != nil {
			return nil, err
		}
	}
	var out []MigrationStatus
	for _, m := range Migrations {
		out = append(out, MigrationStatus{Version: m.Version, Name: m.Name, AppliedAt: applied[m.Version]})
	}
	return out, nil
}

// Migrate применяет недостающие миграции. Если база уже содержит данные
// (в т.ч. база Python-версии без таблицы schema_migrations) и есть что
// применять — перед этим делается резервная копия файла БД.
func (s *Store) Migrate(ctx context.Context, backup bool) (int, error) {
	legacy, err := tableExists(ctx, s.db, "companies")
	if err != nil {
		return 0, err
	}
	hadTracking, err := tableExists(ctx, s.db, "schema_migrations")
	if err != nil {
		return 0, err
	}
	if err := s.ensureMigrationsTable(ctx); err != nil {
		return 0, err
	}
	applied, err := s.appliedVersions(ctx)
	if err != nil {
		return 0, err
	}
	var pending []Migration
	for _, m := range Migrations {
		if _, ok := applied[m.Version]; !ok {
			pending = append(pending, m)
		}
	}
	if len(pending) == 0 {
		return 0, nil
	}

	if legacy && !hadTracking {
		slog.Info("Обнаружена база Python-версии бота — переносим на версионные миграции", "path", s.path)
	}
	if legacy && backup && s.path != "" && s.path != ":memory:" {
		dst := fmt.Sprintf("%s.backup-%s", s.path, time.Now().Format("20060102-150405"))
		if err := s.BackupTo(ctx, dst); err != nil {
			return 0, fmt.Errorf("резервная копия перед миграцией: %w", err)
		}
		slog.Info("Резервная копия БД создана", "path", dst)
	}

	for _, m := range pending {
		if err := s.apply(ctx, m); err != nil {
			return 0, fmt.Errorf("миграция %04d_%s: %w", m.Version, m.Name, err)
		}
		slog.Info("Миграция применена", "version", m.Version, "name", m.Name)
	}
	return len(pending), nil
}

func (s *Store) apply(ctx context.Context, m Migration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if m.SQLFile != "" {
		body, err := migrationFS.ReadFile(m.SQLFile)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			return err
		}
	}
	if m.Func != nil {
		if err := m.Func(ctx, tx); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, name, applied_at) VALUES (?,?,?)",
		m.Version, m.Name, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return tx.Commit()
}

// BackupTo делает консистентную копию базы (VACUUM INTO — корректно и в WAL-режиме).
func (s *Store) BackupTo(ctx context.Context, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "VACUUM INTO ?", dst)
	return err
}
