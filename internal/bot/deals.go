package bot

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tg "github.com/go-telegram/bot"

	"hodler-doc-bot/internal/cleanup"
	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/util"
)

var actLabels = map[string]string{
	"sell":    "📤 Продажа ВА клиенту",
	"buy":     "📥 Покупка ВА у клиента",
	"invoice": "🧾 Счёт-заявка",
}

func editPlain(chatID int64, msgID int, text string) *tg.EditMessageTextParams {
	return &tg.EditMessageTextParams{ChatID: chatID, MessageID: msgID, Text: text}
}

func dealCardText(d *storage.Deal, signed int) string {
	label := actLabels[d.ActType]
	if label == "" {
		label = escape(d.ActType)
	}
	or := func(s string) string {
		if s == "" {
			return "—"
		}
		return escape(s)
	}
	lines := []string{
		fmt.Sprintf("<b>%s №%s</b>", label, or(d.DealNumber)),
		fmt.Sprintf("<b>Дата:</b> %s   <b>Исполнение:</b> %s", or(d.DealDate), or(d.ExecutionDate)),
		fmt.Sprintf("<b>Клиент:</b> %s (%s)", or(d.ClientName), escape(d.ClientResident)),
		"",
		fmt.Sprintf("<b>Актив:</b> %s (%s)", or(d.VAType), or(d.Network)),
		"<b>Сумма ВА:</b> " + util.Num(d.VAAmount),
		fmt.Sprintf("<b>Сумма %s:</b> %s", escape(d.FiatCurrency), util.Num(d.FiatAmount)),
		fmt.Sprintf("<b>Курс:</b> %s   <b>КВВО:</b> %s", or(d.ExchangeRate), or(d.KVVO)),
	}
	if txs := storage.ParseTxs(d.Transactions); len(txs) > 1 {
		lines = append(lines, fmt.Sprintf("<b>Транзакций:</b> %d", len(txs)))
	}
	lines = append(lines, "", fmt.Sprintf("<b>Подписанных документов:</b> %d", signed))
	return strings.Join(lines, "\n")
}

func (a *App) showDealsMenu(c *Ctx) {
	c.Reply("🗂 <b>Журнал сделок</b>\n\nВсе сгенерированные акты сохраняются здесь. "+
		"Документы хранятся в Telegram и доступны в любой момент.", dealsMenuKB())
}

func (a *App) dealsCallback(c *Ctx, head, rest string) {
	id, _ := parseID(rest)
	switch head {
	case "deals":
		switch rest {
		case "menu":
			c.s.Clear()
			a.showDealsMenu(c)
		case "recent":
			deals, err := a.st.RecentDeals(c, 10, 0)
			if err != nil {
				c.fail(err)
				return
			}
			if len(deals) == 0 {
				c.Answer("Журнал пока пуст", true)
				return
			}
			c.Edit("🕐 <b>Последние сделки:</b>", dealsListKB(deals))
		case "by_company":
			cs, err := a.st.Companies(c)
			if err != nil {
				c.fail(err)
				return
			}
			if len(cs) == 0 {
				c.Answer("Нет компаний", true)
				return
			}
			c.EditPlain("Выберите клиента:", companyPickKB(cs, "deals_c", "deals:menu"))
		}

	case "deals_c":
		deals, err := a.st.RecentDeals(c, 10, id)
		if err != nil {
			c.fail(err)
			return
		}
		if len(deals) == 0 {
			c.Answer("По этому клиенту сделок нет", true)
			return
		}
		c.Edit("🏢 <b>Сделки клиента "+escape(deals[0].ClientName)+":</b>", dealsListKB(deals))

	case "deal":
		c.s.Clear()
		a.showDealCard(c, id)

	case "deal_docs":
		d, err := a.st.Deal(c, id)
		if err != nil || d == nil {
			c.Answer("Сделка не найдена", true)
			return
		}
		sent := false
		for _, f := range [][2]string{{d.DocxFileID, d.DocxName}, {d.PDFFileID, d.PDFName}} {
			if f[0] != "" && c.SendFileID(f[0], f[1]) == nil {
				sent = true
			}
		}
		if sent {
			c.Answer("📄 Отправлено", false)
		} else {
			c.Answer("Файлы недоступны", true)
		}

	case "deal_signed":
		docs, err := a.st.SignedDocsByDeal(c, id)
		if err != nil {
			c.fail(err)
			return
		}
		if len(docs) == 0 {
			c.Answer("Подписанных документов нет", true)
			return
		}
		c.Answer("📥 Отправляю...", false)
		for _, doc := range docs {
			if c.SendFileID(doc.FileID, doc.FileName) != nil {
				// file_id фото нельзя отправить как документ
				if c.SendPhotoID(doc.FileID, doc.FileName) != nil {
					name := doc.FileName
					if name == "" {
						name = "файл"
					}
					c.Send("⚠️ Не удалось отправить "+escape(name), nil)
				}
			}
		}

	case "deal_del":
		c.EditPlain("⚠️ Удалить сделку из журнала?\n"+
			"Записи о подписанных документах этой сделки тоже удалятся "+
			"(сами файлы в чатах Telegram останутся).", confirmDealDeleteKB(id))

	case "deal_del_yes":
		if err := a.st.DeleteDeal(c, id); err != nil {
			c.fail(err)
			return
		}
		c.Answer("🗑 Удалено", true)
		a.showDealsMenu(c)

	case "deal_attach":
		d, err := a.st.Deal(c, id)
		if err != nil || d == nil {
			c.Answer("Сделка не найдена", true)
			return
		}
		c.s.Clear()
		c.s.State = stSignedWaiting
		c.s.Data["attach_deal_id"] = rest
		if d.CompanyID.Valid {
			c.s.Data["attach_company_id"] = fmt.Sprint(d.CompanyID.Int64)
		}
		c.s.Data["attach_count"] = "0"
		c.Send("📎 <b>Подписанные документы к сделке №"+escape(d.DealNumber)+"</b>\n\n"+
			"Пришлите файлы (PDF, DOCX, скан) — можно несколько подряд.\n"+
			"Когда закончите, нажмите «Готово».", signedDoneKB())

	case "signed":
		if rest != "done" || c.s.State != stSignedWaiting {
			return
		}
		count := c.s.Data["attach_count"]
		c.s.Clear()
		c.EditPlain("📎 Готово: прикреплено документов — "+count+".", backToMainKB())

	case "arch":
		if rest == "menu" {
			cs, err := a.st.Companies(c)
			if err != nil {
				c.fail(err)
				return
			}
			if len(cs) == 0 {
				c.Answer("Нет компаний", true)
				return
			}
			c.Edit("📦 <b>Архив подписанных документов</b>\n\nВыберите клиента — "+
				"бот соберёт все его подписанные документы в ZIP:", companyPickKB(cs, "arch", "deals:menu"))
			return
		}
		a.buildArchive(c, id)
	}
}

func (a *App) showDealCard(c *Ctx, id int64) {
	d, err := a.st.Deal(c, id)
	if err != nil {
		c.fail(err)
		return
	}
	if d == nil {
		c.Answer("Сделка не найдена", true)
		return
	}
	signed, err := a.st.CountSignedDocs(c, id)
	if err != nil {
		c.fail(err)
		return
	}
	c.Edit(dealCardText(d, signed), dealCardKB(id, d.DocxFileID != "" || d.PDFFileID != "", signed))
}

// signedInput — приём файлов подписанных актов (в БД — только file_id).
func (a *App) signedInput(c *Ctx) {
	var fileID, name string
	switch {
	case c.msg.Document != nil:
		fileID, name = c.msg.Document.FileID, c.msg.Document.FileName
		if name == "" {
			name = "document"
		}
	case len(c.msg.Photo) > 0:
		fileID = c.msg.Photo[len(c.msg.Photo)-1].FileID
		name = fmt.Sprintf("scan_deal_%s.jpg", c.s.Data["attach_deal_id"])
	default:
		c.SendPlain("Пришлите файл документом или фото, либо нажмите «Готово».", signedDoneKB())
		return
	}
	nullID := func(s string) sql.NullInt64 {
		if n, ok := parseID(s); ok {
			return sql.NullInt64{Int64: n, Valid: true}
		}
		return sql.NullInt64{}
	}
	if err := a.st.AddSignedDoc(c, nullID(c.s.Data["attach_company_id"]), nullID(c.s.Data["attach_deal_id"]),
		fileID, name); err != nil {
		c.fail(err)
		return
	}
	n := 0
	fmt.Sscan(c.s.Data["attach_count"], &n)
	n++
	c.s.Data["attach_count"] = fmt.Sprint(n)
	what := "Сохранено"
	if c.msg.Document == nil {
		what = "Скан сохранён"
	}
	c.SendPlain(fmt.Sprintf("✅ %s (%d шт.). Ещё файл или «Готово».", what, n), signedDoneKB())
}

// buildArchive собирает подписанные документы клиента в ZIP.
func (a *App) buildArchive(c *Ctx, companyID int64) {
	co, err := a.st.Company(c, companyID)
	if err != nil || co == nil {
		c.Answer("Компания не найдена", true)
		return
	}
	docs, err := a.st.SignedDocsByCompany(c, companyID)
	if err != nil {
		c.fail(err)
		return
	}
	if len(docs) == 0 {
		c.Answer("У клиента нет подписанных документов", true)
		return
	}
	c.Answer("⏳ Собираю архив...", false)
	status := c.SendPlain(fmt.Sprintf("⏳ Скачиваю %d файлов из Telegram...", len(docs)), nil)
	defer func() {
		if status != nil {
			c.Delete(status.ID)
		}
	}()

	if err := os.MkdirAll(a.cfg.OutputDir, 0o755); err != nil {
		c.fail(err)
		return
	}
	// случайный суффикс: параллельные запросы не затирают файлы друг друга
	tmpDir, err := os.MkdirTemp(a.cfg.OutputDir, fmt.Sprintf("signed_%d_", companyID))
	if err != nil {
		c.fail(err)
		return
	}
	defer os.RemoveAll(tmpDir)

	type item struct{ path, name string }
	var files []item
	used := map[string]bool{}
	failed := 0
	for i, doc := range docs {
		tmp := filepath.Join(tmpDir, fmt.Sprintf("f_%d", i+1))
		if err := c.Download(doc.FileID, tmp); err != nil {
			failed++
			continue
		}
		name := doc.FileName
		if name == "" {
			name = fmt.Sprintf("document_%d", i+1)
		}
		name = filepath.Base(name)
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		for n := 2; used[name]; n++ {
			name = fmt.Sprintf("%s_%d%s", base, n, ext)
		}
		used[name] = true
		files = append(files, item{tmp, name})
	}
	if len(files) == 0 {
		c.SendPlain("❌ Не удалось скачать ни одного файла из Telegram.", nil)
		return
	}
	zipPath := filepath.Join(tmpDir, "archive.zip")
	if err := writeZip(zipPath, func(add func(path, name string) error) error {
		for _, f := range files {
			if err := add(f.path, f.name); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		c.fail(err)
		return
	}
	note := ""
	if failed > 0 {
		note = fmt.Sprintf("\n⚠️ Не удалось скачать: %d", failed)
	}
	short := util.SafeNamePart(co.ShortName, 20, "client")
	if _, err := c.SendFile(zipPath, "Подписанные_"+short+".zip",
		fmt.Sprintf("📦 %s: %d документ(ов)%s", co.ShortName, len(files), note), false); err != nil {
		c.fail(err)
	}
	cleanup.TryRemove(zipPath)
}

func writeZip(path string, fill func(add func(path, name string) error) error) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(out)
	err = fill(func(src, name string) error {
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Flags: 0x800})
		if err != nil {
			return err
		}
		_, err = io.Copy(w, in)
		return err
	})
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
