package bot

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"hodler-doc-bot/internal/config"
	"hodler-doc-bot/internal/docgen"
)

const (
	maxTemplateMB = 15
	backupsKeep   = 10 // сколько резервных копий хранить на каждый шаблон
)

func (a *App) tplPath(kind string) (string, string, bool) {
	name, ok := config.ActTemplateFiles[kind]
	if !ok {
		return "", "", false
	}
	return filepath.Join(a.cfg.TemplatesDir, name), name, true
}

func (a *App) templateCard(c *Ctx, kind string) string {
	path, name, _ := a.tplPath(kind)
	lines := []string{"<b>" + templateKindLabel(kind) + "</b>", "Файл: <code>" + name + "</code>\n"}
	st, err := os.Stat(path)
	if err != nil {
		lines = append(lines, "⚠️ <b>Файл не найден!</b> Загрузите шаблон .docx.")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, fmt.Sprintf("Размер: %d КБ · Обновлён: %s", st.Size()/1024, st.ModTime().Format("02.01.2006 15:04")))

	found, err := docgen.ScanPlaceholders(path)
	if err != nil {
		lines = append(lines, "\n⚠️ Не удалось прочитать шаблон: <code>"+escape(err.Error())+"</code>")
		return strings.Join(lines, "\n")
	}
	op, _ := a.st.Settings(c)
	custom, _ := a.st.CustomFields(c)
	known := docgen.KnownPlaceholders(op, custom)
	var unknown []string
	for _, p := range found {
		if !known[p] {
			unknown = append(unknown, p)
		}
	}
	lines = append(lines, fmt.Sprintf("\n<b>Плейсхолдеры (%d):</b>", len(found)))
	if len(found) > 0 {
		lines = append(lines, "<code>"+escape(strings.Join(found, " "))+"</code>")
	} else {
		lines = append(lines, "<i>не найдены — проверьте шаблон</i>")
	}
	if len(unknown) > 0 {
		lines = append(lines, "\n❗️ <b>Неизвестные (останутся незаполненными):</b>",
			"<code>"+escape(strings.Join(unknown, " "))+"</code>",
			"<i>Создайте для них свои переменные (🧩) или исправьте шаблон.</i>")
	}
	return strings.Join(lines, "\n")
}

func (a *App) templatesCallback(c *Ctx, head, rest string) {
	if head == "vars" {
		a.varsCallback(c, rest)
		return
	}
	action, kind, _ := strings.Cut(rest, ":")
	switch action {
	case "menu":
		c.s.Clear()
		c.Edit("📄 <b>Шаблоны документов</b>\n\n"+
			"Здесь можно посмотреть, скачать и заменить шаблоны, "+
			"а также завести свои переменные для новых плейсхолдеров.", templatesMenuKB())
	case "view":
		if _, _, ok := a.tplPath(kind); !ok {
			return
		}
		c.Edit(a.templateCard(c, kind), templateCardKB(kind))
	case "download":
		path, name, ok := a.tplPath(kind)
		if !ok {
			return
		}
		if _, err := os.Stat(path); err != nil {
			c.Answer("Файл не найден", true)
			return
		}
		if _, err := c.SendFile(path, name, "📄 Текущий шаблон: "+templateKindLabel(kind), false); err != nil {
			c.fail(err)
		}
	case "upload":
		if _, _, ok := a.tplPath(kind); !ok {
			return
		}
		c.s.Clear()
		c.s.State = stTplWaiting
		c.s.Data["tpl_kind"] = kind
		c.Edit("⬆️ <b>Загрузка шаблона</b>\n"+templateKindLabel(kind)+"\n\n"+
			"Пришлите файл <b>.docx</b> с плейсхолдерами вида <code>{{DEAL_NUMBER}}</code>.\n"+
			"Старый шаблон сохранится в резервную копию.", nil)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func (a *App) tplUploadFile(c *Ctx) {
	doc := c.msg.Document
	if doc == nil {
		c.SendPlain("Пришлите шаблон файлом .docx (или /menu для отмены).", nil)
		return
	}
	if !strings.HasSuffix(strings.ToLower(doc.FileName), ".docx") {
		c.SendPlain("❌ Нужен файл .docx — пришлите шаблон Word.", nil)
		return
	}
	if doc.FileSize > maxTemplateMB*1024*1024 {
		c.SendPlain(fmt.Sprintf("❌ Файл больше %d МБ.", maxTemplateMB), nil)
		return
	}
	kind := c.s.Data["tpl_kind"]
	target, name, ok := a.tplPath(kind)
	if !ok {
		c.s.Clear()
		return
	}
	if err := os.MkdirAll(a.cfg.TemplatesDir, 0o755); err != nil {
		c.fail(err)
		return
	}
	tmp := target + ".new"
	if err := c.Download(doc.FileID, tmp); err != nil {
		c.fail(err)
		return
	}
	// Проверяем, что это валидный DOCX и в нём есть плейсхолдеры
	found, err := docgen.ScanPlaceholders(tmp)
	if err != nil {
		os.Remove(tmp)
		c.Send("❌ Файл не читается как DOCX:\n<code>"+escape(err.Error())+"</code>", nil)
		return
	}
	if len(found) == 0 {
		os.Remove(tmp)
		c.SendPlain("❌ В файле нет ни одного плейсхолдера {{...}} — похоже, это не шаблон. Файл не сохранён.", nil)
		return
	}
	// Бэкап старого и замена (храним последние backupsKeep копий)
	if _, err := os.Stat(target); err == nil {
		backupDir := filepath.Join(a.cfg.TemplatesDir, "backups")
		if err := os.MkdirAll(backupDir, 0o755); err == nil {
			stamp := time.Now().Format("20060102_150405")
			if err := copyFile(target, filepath.Join(backupDir, stamp+"_"+name)); err != nil {
				c.fail(err)
				return
			}
			entries, _ := os.ReadDir(backupDir)
			var old []string
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), name) {
					old = append(old, e.Name())
				}
			}
			sort.Strings(old)
			for len(old) > backupsKeep {
				os.Remove(filepath.Join(backupDir, old[0]))
				old = old[1:]
			}
		}
	}
	if err := os.Rename(tmp, target); err != nil {
		c.fail(err)
		return
	}
	c.s.Clear()
	c.Send("✅ Шаблон обновлён!", nil)
	c.Send(a.templateCard(c, kind), templateCardKB(kind))
}

// ─── Свои переменные ──────────────────────────────────────────────────────────

func (a *App) varsText(c *Ctx) string {
	fields, _ := a.st.CustomFields(c)
	lines := []string{
		"🧩 <b>Свои переменные</b>\n",
		"Каждая переменная — это плейсхолдер <code>{{CL_КЛЮЧ}}</code> для шаблонов. " +
			"При добавлении новой компании бот задаст вопрос по каждой переменной.\n",
	}
	if len(fields) > 0 {
		lines = append(lines, "<b>Текущие (нажмите, чтобы удалить):</b>")
	} else {
		lines = append(lines, "<i>Пока нет ни одной переменной.</i>")
	}
	return strings.Join(lines, "\n")
}

func (a *App) showVars(c *Ctx, send bool) {
	fields, err := a.st.CustomFields(c)
	if err != nil {
		c.fail(err)
		return
	}
	if send {
		c.Send(a.varsText(c), varsMenuKB(fields))
	} else {
		c.Edit(a.varsText(c), varsMenuKB(fields))
	}
}

func (a *App) varsCallback(c *Ctx, rest string) {
	switch {
	case rest == "menu":
		c.s.Clear()
		a.showVars(c, false)
	case rest == "add":
		c.s.Clear()
		c.s.State = stVarKey
		c.Edit("➕ <b>Новая переменная — шаг 1/2</b>\n\n"+
			"Введите ключ латиницей (буквы, цифры, подчёркивание).\n"+
			"Например: <code>contract_number</code> → в шаблоне будет <code>{{CL_CONTRACT_NUMBER}}</code>", nil)
	case strings.HasPrefix(rest, "del:"):
		key := strings.TrimPrefix(rest, "del:")
		if err := a.st.DeleteCustomField(c, key); err != nil {
			c.fail(err)
			return
		}
		c.Answer("Удалена "+key, false)
		a.showVars(c, false)
	}
}

var varKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)

func (a *App) varsKeyInput(c *Ctx, text string) {
	key := strings.TrimRight(strings.TrimLeft(strings.ToLower(text), "{"), "}")
	key = strings.TrimPrefix(key, "cl_")
	if !varKeyRe.MatchString(key) {
		c.SendPlain("❌ Ключ — латиницей, начинается с буквы, без пробелов. Например: contract_number", nil)
		return
	}
	c.s.Data["var_key"] = key
	c.s.State = stVarLabel
	c.Send(fmt.Sprintf("Ключ: <code>{{CL_%s}}</code>\n\n<b>Шаг 2/2:</b> введите вопрос-подпись, который бот будет задавать "+
		"при добавлении компании.\nНапример: <i>Номер договора с клиентом</i>", strings.ToUpper(key)), nil)
}

func (a *App) varsLabelInput(c *Ctx, label string) {
	key := c.s.Data["var_key"]
	c.s.Clear()
	if key == "" {
		return
	}
	if err := a.st.AddCustomField(c, key, label); err != nil {
		c.fail(err)
		return
	}
	c.Send(fmt.Sprintf("✅ Переменная <code>{{CL_%s}}</code> — «%s» создана.\n"+
		"Она появится в вопросах при добавлении компании и в карточках компаний.",
		strings.ToUpper(key), escape(label)), nil)
	a.showVars(c, true)
}
