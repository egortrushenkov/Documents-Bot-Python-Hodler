// Package transfer — полная копия инсталляции бота для переноса на другой сервер.
//
// Архив .tar.gz повторяет раскладку каталога проекта, поэтому его можно и
// просто распаковать `tar xzf` в каталог проекта на новом сервере:
//
//	manifest.json       — дата, хост, версии миграций, sha256 файлов
//	.env                — настройки и секреты (BOT_TOKEN!)
//	data/hodler.db      — консистентный снимок БД (VACUUM INTO)
//	templates/...       — шаблоны DOCX вместе с резервными копиями
//
// Сгенерированные файлы (data/output) не переносятся: документы живут в
// Telegram, в БД — только их file_id.
package transfer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"hodler-doc-bot/internal/config"
	"hodler-doc-bot/internal/storage"
)

const (
	manifestName = "manifest.json"
	envName      = ".env"
	dbName       = "data/hodler.db"
	tplPrefix    = "templates/"
)

// EnvKeys — переменные, из которых собирается .env, если файла нет
// (например, в Docker настройки приходят через env_file).
var EnvKeys = []string{
	"BOT_TOKEN", "ADMIN_IDS", "ALLOWED_USER_IDS", "DB_PATH", "TEMPLATES_DIR", "OUTPUT_DIR", "LIBREOFFICE_PATH",
	"CLEANUP_MAX_AGE_HOURS", "CLEANUP_INTERVAL_MIN", "PDF_CONCURRENCY", "DB_BACKUP_BEFORE_MIGRATE", "DOCKER_REGISTRY", "GOPROXY",
}

type Manifest struct {
	CreatedAt  string            `json:"created_at"`
	Host       string            `json:"host"`
	Migrations []int             `json:"migrations"`
	EnvSource  string            `json:"env_source"` // file | environment | none
	Files      map[string]string `json:"files"`      // путь → sha256
}

type ExportOptions struct {
	EnvFile string // путь к .env (по умолчанию ".env")
	NoEnv   bool   // не класть .env в архив
}

type ExportResult struct {
	Path      string
	Files     int
	EnvSource string
	Size      int64
}

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func envFromProcess() []byte {
	var b strings.Builder
	b.WriteString("# Собрано командой hodler-bot export из переменных окружения\n")
	for _, k := range EnvKeys {
		if v, ok := os.LookupEnv(k); ok {
			fmt.Fprintf(&b, "%s=%s\n", k, v)
		}
	}
	return []byte(b.String())
}

// Export пишет полную копию в dst.
func Export(ctx context.Context, cfg *config.Config, st *storage.Store, dst string, opt ExportOptions) (*ExportResult, error) {
	if opt.EnvFile == "" {
		opt.EnvFile = envName
	}
	tmpDir, err := os.MkdirTemp("", "hodler-export-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	type entry struct {
		name string
		data []byte
		mode int64
		mod  time.Time
	}
	var entries []entry
	now := time.Now()

	// 1. Снимок БД
	snap := filepath.Join(tmpDir, "hodler.db")
	if err := st.BackupTo(ctx, snap); err != nil {
		return nil, fmt.Errorf("снимок БД: %w", err)
	}
	dbBytes, err := os.ReadFile(snap)
	if err != nil {
		return nil, err
	}
	entries = append(entries, entry{dbName, dbBytes, 0o600, now})

	// 2. .env
	envSource := "none"
	if !opt.NoEnv {
		if b, err := os.ReadFile(opt.EnvFile); err == nil {
			envSource = "file"
			entries = append(entries, entry{envName, b, 0o600, now})
		} else if errors.Is(err, fs.ErrNotExist) {
			envSource = "environment"
			entries = append(entries, entry{envName, envFromProcess(), 0o600, now})
		} else {
			return nil, err
		}
	}

	// 3. Шаблоны (включая templates/backups)
	err = filepath.WalkDir(cfg.TemplatesDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() || strings.HasSuffix(p, ".new") {
			return nil
		}
		rel, err := filepath.Rel(cfg.TemplatesDir, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, _ := d.Info()
		entries = append(entries, entry{tplPrefix + filepath.ToSlash(rel), b, 0o644, info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("шаблоны: %w", err)
	}

	// 4. Манифест
	host, _ := os.Hostname()
	man := Manifest{CreatedAt: now.UTC().Format(time.RFC3339), Host: host, EnvSource: envSource, Files: map[string]string{}}
	if status, err := st.Status(ctx); err == nil {
		for _, m := range status {
			if m.AppliedAt != "" {
				man.Migrations = append(man.Migrations, m.Version)
			}
		}
	}
	for _, e := range entries {
		man.Files[e.name] = sha(e.data)
	}
	manBytes, _ := json.MarshalIndent(man, "", "  ")

	// Пишем во временный файл рядом и переименовываем — без полуготовых архивов
	if dir := filepath.Dir(dst); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	part := dst + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) // внутри секреты
	if err != nil {
		return nil, err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	write := func(e entry) error {
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Mode: e.mode, Size: int64(len(e.data)),
			ModTime: e.mod, Typeflag: tar.TypeReg, Format: tar.FormatPAX}); err != nil {
			return err
		}
		_, err := tw.Write(e.data)
		return err
	}
	err = write(entry{manifestName, manBytes, 0o644, now})
	for _, e := range entries {
		if err != nil {
			break
		}
		err = write(e)
	}
	if cerr := tw.Close(); err == nil {
		err = cerr
	}
	if cerr := gz.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(part)
		return nil, err
	}
	if err := os.Rename(part, dst); err != nil {
		return nil, err
	}
	fi, _ := os.Stat(dst)
	return &ExportResult{Path: dst, Files: len(entries), EnvSource: envSource, Size: fi.Size()}, nil
}

type ImportOptions struct {
	EnvFile string // куда положить .env (по умолчанию ".env")
	Force   bool   // перезаписать существующие БД и .env (старые сохраняются рядом)
	NoEnv   bool   // не трогать .env
}

type ImportResult struct {
	Manifest  Manifest
	DB        string
	Templates int
	Env       string // путь, куда записан .env ("" — не записан)
	Saved     []string
}

// safeName — защита от путей вида ../ и абсолютных в архиве.
func safeName(name string) (string, bool) {
	clean := path.Clean(name)
	if clean != name || path.IsAbs(clean) || strings.HasPrefix(clean, "../") || clean == ".." {
		return "", false
	}
	return clean, true
}

// Import разворачивает архив. Бот на этом сервере должен быть остановлен.
func Import(cfg *config.Config, src string, opt ImportOptions) (*ImportResult, error) {
	if opt.EnvFile == "" {
		opt.EnvFile = envName
	}
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("это не архив .tar.gz: %w", err)
	}
	tr := tar.NewReader(gz)

	files := map[string][]byte{}
	var man *Manifest
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		name, ok := safeName(h.Name)
		if !ok || !(name == manifestName || name == envName || name == dbName || strings.HasPrefix(name, tplPrefix)) {
			return nil, fmt.Errorf("недопустимый файл в архиве: %q", h.Name)
		}
		b, err := io.ReadAll(io.LimitReader(tr, 2<<30))
		if err != nil {
			return nil, err
		}
		if name == manifestName {
			man = &Manifest{}
			if err := json.Unmarshal(b, man); err != nil {
				return nil, fmt.Errorf("manifest.json: %w", err)
			}
			continue
		}
		files[name] = b
	}
	if man == nil {
		return nil, fmt.Errorf("в архиве нет manifest.json — это не копия hodler-bot export")
	}
	for name, want := range man.Files {
		b, ok := files[name]
		if !ok {
			return nil, fmt.Errorf("в архиве не хватает %s", name)
		}
		if sha(b) != want {
			return nil, fmt.Errorf("контрольная сумма не совпала: %s (архив повреждён)", name)
		}
	}
	if _, ok := files[dbName]; !ok {
		return nil, fmt.Errorf("в архиве нет базы данных")
	}

	// Проверки до записи: ничего не перезаписываем молча
	stamp := time.Now().Format("20060102-150405")
	if _, err := os.Stat(cfg.DBPath); err == nil && !opt.Force {
		return nil, fmt.Errorf("база %s уже существует — остановите бота и повторите с -force (текущая будет сохранена как %s.before-import-%s)",
			cfg.DBPath, cfg.DBPath, stamp)
	}
	_, hasEnv := files[envName]
	writeEnv := hasEnv && !opt.NoEnv
	if writeEnv {
		if _, err := os.Stat(opt.EnvFile); err == nil && !opt.Force {
			return nil, fmt.Errorf("%s уже существует — повторите с -force (текущий будет сохранён) или с -no-env", opt.EnvFile)
		}
	}

	res := &ImportResult{Manifest: *man, DB: cfg.DBPath}
	keep := func(p string) error {
		if _, err := os.Stat(p); err != nil {
			return nil
		}
		dst := p + ".before-import-" + stamp
		if err := os.Rename(p, dst); err != nil {
			return err
		}
		res.Saved = append(res.Saved, dst)
		return nil
	}

	// БД: старую (вместе с -wal/-shm) отодвигаем, новую пишем атомарно
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return nil, err
	}
	for _, p := range []string{cfg.DBPath, cfg.DBPath + "-wal", cfg.DBPath + "-shm"} {
		if err := keep(p); err != nil {
			return nil, err
		}
	}
	if err := writeAtomic(cfg.DBPath, files[dbName], 0o600); err != nil {
		return nil, err
	}

	// Шаблоны: изменившийся существующий файл сохраняем рядом
	names := make([]string, 0, len(files))
	for n := range files {
		if strings.HasPrefix(n, tplPrefix) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		dst := filepath.Join(cfg.TemplatesDir, filepath.FromSlash(strings.TrimPrefix(n, tplPrefix)))
		if old, err := os.ReadFile(dst); err == nil {
			if sha(old) == sha(files[n]) {
				res.Templates++
				continue
			}
			if err := keep(dst); err != nil {
				return nil, err
			}
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := writeAtomic(dst, files[n], 0o644); err != nil {
			return nil, err
		}
		res.Templates++
	}

	if writeEnv {
		if err := keep(opt.EnvFile); err != nil {
			return nil, err
		}
		if err := writeAtomic(opt.EnvFile, files[envName], 0o600); err != nil {
			return nil, err
		}
		res.Env = opt.EnvFile
	}
	return res, nil
}

func writeAtomic(p string, b []byte, mode os.FileMode) error {
	tmp := p + ".importing"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
