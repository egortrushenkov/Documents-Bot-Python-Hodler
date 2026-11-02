// Package cleanup — фоновая автоочистка.
//
// Документы после отправки живут в Telegram (file_id в БД), поэтому локальные
// копии удаляются сразу после отправки, а этот цикл подчищает остатки после
// сбоев и временные профили LibreOffice.
package cleanup

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hodler-doc-bot/internal/pdf"
)

func removeOldFiles(dir string, maxAge time.Duration) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	now := time.Now()
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > maxAge {
			if os.Remove(filepath.Join(dir, e.Name())) == nil {
				n++
			}
		}
	}
	return n
}

func removeStaleProfiles(maxAge time.Duration) int {
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return 0
	}
	n := 0
	now := time.Now()
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), pdf.ProfilePrefix) {
			continue
		}
		info, err := e.Info()
		if err != nil || now.Sub(info.ModTime()) <= maxAge {
			continue
		}
		if os.RemoveAll(filepath.Join(os.TempDir(), e.Name())) == nil {
			n++
		}
	}
	return n
}

// Loop работает до отмены ctx.
func Loop(ctx context.Context, outputDir string, maxAge, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		nf := removeOldFiles(outputDir, maxAge)
		np := removeStaleProfiles(time.Hour) // профили LO старше часа
		if nf > 0 || np > 0 {
			slog.Info("Автоочистка", "files", nf, "lo_profiles", np)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// TryRemove тихо удаляет файлы.
func TryRemove(paths ...string) {
	for _, p := range paths {
		if p != "" {
			_ = os.Remove(p)
		}
	}
}
