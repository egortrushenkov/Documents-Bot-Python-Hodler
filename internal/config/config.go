// Package config читает настройки бота из окружения (.env).
package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Имена файлов шаблонов (лежат в TEMPLATES_DIR, загружаются через меню бота).
// Названия — со стороны клиента.
const (
	TemplateBuy        = "template_buy.docx"         // клиент покупает ВА у нас (мы продаём)
	TemplateSell       = "template_sell.docx"        // клиент продаёт ВА нам (мы покупаем)
	TemplateInvoiceBuy = "template_invoice_buy.docx" // счёт-заявка на покупку клиентом
)

// ActTemplateFiles — единственный источник соответствия «тип операции → файл шаблона».
// act_type "sell" = мы продаём = клиент покупает → template_buy (и наоборот).
var ActTemplateFiles = map[string]string{
	"sell":    TemplateBuy,
	"buy":     TemplateSell,
	"invoice": TemplateInvoiceBuy,
}

type Config struct {
	BotToken            string
	AdminIDs            map[int64]bool
	DBPath              string
	TemplatesDir        string
	OutputDir           string
	LibreOfficePath     string
	CleanupMaxAgeHours  int
	CleanupIntervalMin  int
	PDFConcurrency      int
	BackupBeforeMigrate bool
}

func Load() *Config {
	_ = godotenv.Load() // .env необязателен: переменные могут прийти из окружения

	c := &Config{
		BotToken:            os.Getenv("BOT_TOKEN"),
		AdminIDs:            map[int64]bool{},
		DBPath:              env("DB_PATH", "data/hodler.db"),
		TemplatesDir:        env("TEMPLATES_DIR", "templates"),
		OutputDir:           env("OUTPUT_DIR", "data/output"),
		LibreOfficePath:     env("LIBREOFFICE_PATH", "libreoffice"),
		CleanupMaxAgeHours:  envInt("CLEANUP_MAX_AGE_HOURS", 24),
		CleanupIntervalMin:  envInt("CLEANUP_INTERVAL_MIN", 60),
		PDFConcurrency:      envInt("PDF_CONCURRENCY", 1),
		BackupBeforeMigrate: env("DB_BACKUP_BEFORE_MIGRATE", "1") != "0",
	}
	ids := os.Getenv("ADMIN_IDS")
	if strings.TrimSpace(ids) == "" {
		ids = os.Getenv("ALLOWED_USER_IDS") // старое имя переменной в некоторых .env
	}
	for _, part := range strings.Split(ids, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if id, err := strconv.ParseInt(part, 10, 64); err == nil {
			c.AdminIDs[id] = true
		}
	}
	if c.PDFConcurrency < 1 {
		c.PDFConcurrency = 1
	}
	if c.CleanupIntervalMin < 1 {
		c.CleanupIntervalMin = 1
	}
	return c
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil {
		return v
	}
	return def
}
