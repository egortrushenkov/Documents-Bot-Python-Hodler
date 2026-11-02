package transfer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hodler-doc-bot/internal/config"
	"hodler-doc-bot/internal/storage"
)

func server(t *testing.T) (string, *config.Config) {
	dir := t.TempDir()
	return dir, &config.Config{DBPath: filepath.Join(dir, "data", "hodler.db"), TemplatesDir: filepath.Join(dir, "templates")}
}

func TestExportImport(t *testing.T) {
	ctx := context.Background()

	// ─── Сервер A ───
	dirA, cfgA := server(t)
	st, err := storage.Open(cfgA.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	id, _ := st.AddCompany(ctx, map[string]string{"full_name": "ООО А", "short_name": "А"})
	_ = st.SetSetting(ctx, "wallet", "TWALLET")
	_ = st.RecordNumberUsed(ctx, id, "act", 777)
	os.MkdirAll(filepath.Join(cfgA.TemplatesDir, "backups"), 0o755)
	os.WriteFile(filepath.Join(cfgA.TemplatesDir, config.TemplateBuy), []byte("tpl-buy"), 0o644)
	os.WriteFile(filepath.Join(cfgA.TemplatesDir, "backups", "old_"+config.TemplateBuy), []byte("old"), 0o644)
	envA := filepath.Join(dirA, ".env")
	os.WriteFile(envA, []byte("BOT_TOKEN=123:SECRET\nADMIN_IDS=1\n"), 0o600)

	archive := filepath.Join(dirA, "copy.tar.gz")
	res, err := Export(ctx, cfgA, st, archive, ExportOptions{EnvFile: envA})
	st.Close()
	if err != nil {
		t.Fatal(err)
	}
	if res.EnvSource != "file" || res.Files != 4 {
		t.Fatalf("%+v", res)
	}
	if fi, _ := os.Stat(archive); fi.Mode().Perm() != 0o600 {
		t.Errorf("архив с секретами должен быть 0600, а не %v", fi.Mode().Perm())
	}

	// ─── Сервер B ───
	dirB, cfgB := server(t)
	envB := filepath.Join(dirB, ".env")
	if _, err := Import(cfgB, archive, ImportOptions{EnvFile: envB}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(envB); !strings.Contains(string(b), "123:SECRET") {
		t.Errorf(".env не перенесён: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(cfgB.TemplatesDir, "backups", "old_"+config.TemplateBuy)); string(b) != "old" {
		t.Error("резервные копии шаблонов не перенесены")
	}
	stB, err := storage.Open(cfgB.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	cs, _ := stB.Companies(ctx)
	s, _ := stB.Settings(ctx)
	next, _ := stB.PeekNextNumber(ctx)
	stB.Close()
	if len(cs) != 1 || s["wallet"] != "TWALLET" || next != 778 {
		t.Fatalf("данные не перенесены: %d %q %d", len(cs), s["wallet"], next)
	}

	// Повторный импорт без -force не затирает данные
	if _, err := Import(cfgB, archive, ImportOptions{EnvFile: envB}); err == nil {
		t.Fatal("ожидался отказ без -force")
	}
	r, err := Import(cfgB, archive, ImportOptions{EnvFile: envB, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Saved) < 2 {
		t.Errorf("прежние БД и .env должны быть сохранены: %v", r.Saved)
	}
}

func TestExportEnvFromEnvironment(t *testing.T) {
	ctx := context.Background()
	dir, cfg := server(t)
	st, _ := storage.Open(cfg.DBPath)
	defer st.Close()
	st.Migrate(ctx, false)
	t.Setenv("BOT_TOKEN", "999:DOCKER")
	archive := filepath.Join(dir, "x.tar.gz")
	res, err := Export(ctx, cfg, st, archive, ExportOptions{EnvFile: filepath.Join(dir, "missing.env")})
	if err != nil || res.EnvSource != "environment" {
		t.Fatalf("%+v %v", res, err)
	}
	_, cfgB := server(t)
	envB := filepath.Join(t.TempDir(), ".env")
	if _, err := Import(cfgB, archive, ImportOptions{EnvFile: envB}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(envB); !strings.Contains(string(b), "BOT_TOKEN=999:DOCKER") {
		t.Errorf("%q", b)
	}
}

func TestImportRejectsBadArchive(t *testing.T) {
	dir, cfg := server(t)
	p := filepath.Join(dir, "evil.tar.gz")
	f, _ := os.Create(p)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "../../etc/passwd", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
	tw.Write([]byte("x"))
	tw.Close()
	gz.Close()
	f.Close()
	if _, err := Import(cfg, p, ImportOptions{}); err == nil || !strings.Contains(err.Error(), "недопустимый") {
		t.Fatalf("ожидался отказ: %v", err)
	}
}
