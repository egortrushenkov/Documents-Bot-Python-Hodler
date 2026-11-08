// Hodler Doc Generator Bot (Go).
//
//	hodler-bot                 — запуск бота (миграции БД применяются автоматически)
//	hodler-bot migrate         — только применить миграции и выйти
//	hodler-bot migrate -status — показать состояние миграций
//	hodler-bot backup <файл>   — консистентная копия БД
//	hodler-bot export [-no-env] [файл.tar.gz] — полная копия (БД + шаблоны + .env) для переноса
//	hodler-bot import [-force] [-no-env] <файл.tar.gz> — развернуть копию на новом сервере
//	hodler-bot purge-deals [-treasury] [-yes] — очистить журнал сделок (и остатки с -treasury);
//	                                          компании, реквизиты, нумерация остаются
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"hodler-doc-bot/internal/bot"
	"hodler-doc-bot/internal/cleanup"
	"hodler-doc-bot/internal/config"
	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/transfer"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	// import — до открытия БД: файл базы будет заменён
	if cmd == "import" {
		return runImport(ctx, cfg, os.Args[2:])
	}

	st, err := storage.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("открытие БД %s: %w", cfg.DBPath, err)
	}
	defer st.Close()

	switch cmd {
	case "migrate":
		fs := flag.NewFlagSet("migrate", flag.ExitOnError)
		status := fs.Bool("status", false, "показать состояние миграций")
		_ = fs.Parse(os.Args[2:])
		if *status {
			list, err := st.Status(ctx)
			if err != nil {
				return err
			}
			for _, m := range list {
				state := "ожидает"
				if m.AppliedAt != "" {
					state = "применена " + m.AppliedAt
				}
				fmt.Printf("%04d  %-28s %s\n", m.Version, m.Name, state)
			}
			return nil
		}
		n, err := st.Migrate(ctx, cfg.BackupBeforeMigrate)
		if err != nil {
			return err
		}
		fmt.Printf("Применено миграций: %d\n", n)
		return nil
	case "backup":
		if len(os.Args) < 3 {
			return fmt.Errorf("укажите путь: hodler-bot backup data/backup.db")
		}
		return st.BackupTo(ctx, os.Args[2])
	case "export":
		return runExport(ctx, cfg, st, os.Args[2:])
	case "purge-deals":
		return runPurge(ctx, cfg, st, os.Args[2:])
	case "", "run":
	default:
		return fmt.Errorf("неизвестная команда %q (run | migrate [-status] | backup <файл> | export | import | purge-deals)", cmd)
	}

	if cfg.BotToken == "" {
		return fmt.Errorf("BOT_TOKEN не задан в .env")
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return err
	}
	slog.Info("Миграции БД...")
	if _, err := st.Migrate(ctx, cfg.BackupBeforeMigrate); err != nil {
		return err
	}

	if len(cfg.AdminIDs) == 0 {
		slog.Warn("⚠️ ADMIN_IDS не задан — бот отвечает ЛЮБОМУ пользователю Telegram! Укажите ID сотрудников в .env")
	} else {
		slog.Info("Доступ ограничен", "users", len(cfg.AdminIDs))
	}
	app, err := bot.New(cfg, st)
	if err != nil {
		return err
	}
	// Фоновая автоочистка сгенерированных файлов и временных профилей LibreOffice
	go cleanup.Loop(ctx, cfg.OutputDir,
		time.Duration(cfg.CleanupMaxAgeHours)*time.Hour,
		time.Duration(cfg.CleanupIntervalMin)*time.Minute)

	slog.Info("Бот запущен. Ожидание сообщений...")
	app.Run(ctx)
	slog.Info("Остановлен")
	return nil
}

func runExport(ctx context.Context, cfg *config.Config, st *storage.Store, args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	noEnv := fs.Bool("no-env", false, "не включать .env (секреты) в архив")
	envFile := fs.String("env", ".env", "путь к .env")
	_ = fs.Parse(args)
	dst := fs.Arg(0)
	if dst == "" {
		dst = filepath.Join(filepath.Dir(cfg.DBPath),
			fmt.Sprintf("hodler-export-%s.tar.gz", time.Now().Format("20060102-150405")))
	}
	res, err := transfer.Export(ctx, cfg, st, dst, transfer.ExportOptions{EnvFile: *envFile, NoEnv: *noEnv})
	if err != nil {
		return err
	}
	fmt.Printf("Готово: %s (%d КБ, файлов: %d)\n", res.Path, res.Size/1024, res.Files)
	switch res.EnvSource {
	case "file":
		fmt.Println(".env: взят из файла " + *envFile)
	case "environment":
		fmt.Println(".env: файла нет — собран из переменных окружения (Docker env_file)")
	default:
		fmt.Println(".env: не включён")
	}
	if res.EnvSource != "none" {
		fmt.Println("⚠️  В архиве BOT_TOKEN — передавайте его только по защищённому каналу (scp/ssh) и удалите после переноса.")
	}
	return nil
}

func runImport(ctx context.Context, cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	force := fs.Bool("force", false, "заменить существующие БД и .env (старые сохраняются как *.before-import-*)")
	noEnv := fs.Bool("no-env", false, "не записывать .env из архива")
	envFile := fs.String("env", ".env", "куда записать .env")
	_ = fs.Parse(args)
	if fs.Arg(0) == "" {
		return fmt.Errorf("укажите архив: hodler-bot import hodler-export-....tar.gz")
	}
	res, err := transfer.Import(cfg, fs.Arg(0), transfer.ImportOptions{EnvFile: *envFile, Force: *force, NoEnv: *noEnv})
	if err != nil {
		return err
	}
	fmt.Printf("Копия от %s (сервер %s) развёрнута:\n", res.Manifest.CreatedAt, res.Manifest.Host)
	fmt.Printf("  БД:      %s\n  Шаблоны: %d файл(ов) в %s\n", res.DB, res.Templates, cfg.TemplatesDir)
	if res.Env != "" {
		fmt.Printf("  .env:    %s\n", res.Env)
	}
	for _, p := range res.Saved {
		fmt.Printf("  прежний файл сохранён: %s\n", p)
	}
	// Копия могла быть снята более старой версией бота — догоняем схему
	st, err := storage.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()
	n, err := st.Migrate(ctx, false)
	if err != nil {
		return err
	}
	if n > 0 {
		fmt.Printf("  применено миграций: %d\n", n)
	}
	fmt.Println("Можно запускать бота.")
	return nil
}

func runPurge(ctx context.Context, cfg *config.Config, st *storage.Store, args []string) error {
	fs := flag.NewFlagSet("purge-deals", flag.ExitOnError)
	yes := fs.Bool("yes", false, "подтвердить удаление")
	treasury := fs.Bool("treasury", false, "удалить и остатки: начальный остаток, выводы, выплаты рефералам")
	_ = fs.Parse(args)
	if _, err := st.Migrate(ctx, cfg.BackupBeforeMigrate); err != nil {
		return err
	}
	var deals, signed, buybacks, ops int
	db := st.DB()
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM deals").Scan(&deals)
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM signed_docs WHERE deal_id IS NOT NULL").Scan(&signed)
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM buybacks").Scan(&buybacks)
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM treasury_ops").Scan(&ops)
	fmt.Printf("Будет удалено: сделок — %d, подписанных документов — %d, откупок — %d.\n", deals, signed, buybacks)
	if *treasury {
		fmt.Printf("И остатки: начальный остаток и ручные операции (выводы, выплаты рефералам) — %d записей.\n", ops)
	} else if ops > 0 {
		fmt.Printf("Остатки (начальный остаток и %d операций) НЕ удаляются — добавьте -treasury, чтобы сбросить и их.\n", ops)
	}
	fmt.Println("Останутся: компании, поставщики, рефералы, реквизиты, кошельки, шаблоны, свои переменные, КВВО, нумерация.")
	if !*yes {
		fmt.Println("Чтобы удалить, повторите с флагом -yes (перед удалением будет сделана резервная копия).")
		return nil
	}
	backup := fmt.Sprintf("%s.before-purge-%s", cfg.DBPath, time.Now().Format("20060102-150405"))
	if err := st.BackupTo(ctx, backup); err != nil {
		return fmt.Errorf("резервная копия: %w", err)
	}
	fmt.Println("Резервная копия:", backup)
	d, s, err := st.PurgeDeals(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Готово: удалено сделок — %d, подписанных документов — %d.\n", d, s)
	if *treasury {
		n, err := st.PurgeTreasury(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("Остатки сброшены: удалено записей — %d. Задайте начальный остаток в боте: 💰 Остатки → 📥 Начальный остаток.\n", n)
	}
	return nil
}
