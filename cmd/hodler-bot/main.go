// Hodler Doc Generator Bot (Go).
//
//	hodler-bot                 — запуск бота (миграции БД применяются автоматически)
//	hodler-bot migrate         — только применить миграции и выйти
//	hodler-bot migrate -status — показать состояние миграций
//	hodler-bot backup <файл>   — консистентная копия БД
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hodler-doc-bot/internal/bot"
	"hodler-doc-bot/internal/cleanup"
	"hodler-doc-bot/internal/config"
	"hodler-doc-bot/internal/storage"
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
	case "", "run":
	default:
		return fmt.Errorf("неизвестная команда %q (run | migrate [-status] | backup <файл>)", cmd)
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
