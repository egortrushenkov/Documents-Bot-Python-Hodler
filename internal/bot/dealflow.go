package bot

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"hodler-doc-bot/internal/chain"
	"hodler-doc-bot/internal/cleanup"
	"hodler-doc-bot/internal/docgen"
	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/util"
)

// Этапы сделки:
//
//	продаём клиенту:   создана → 🧾 счёт → 💰 оплата ₽ получена → 💸 USDT выплачен → 📄 акт
//	покупаем у клиента: создана → 📥 USDT получен → 💸 ₽ выплачены → 📄 акт
const (
	stDealPaidAmount = "deal:paid_amount"
	stDealPaidDate   = "deal:paid_date"
	stDealUSDTDate   = "deal:usdt_date"
)

func todayISO() string { return time.Now().Format("2006-01-02") }

// ─── Прогресс ─────────────────────────────────────────────────────────────────

type stageItem struct {
	key, title string
	done       bool
	detail     string
}

func dealStages(d *storage.Deal) []stageItem {
	paid := ""
	if d.PaidAt != "" {
		amt := d.PaidAmount
		if amt == 0 {
			amt = d.FiatAmount
		}
		paid = fmt.Sprintf("%s ₽ · %s", util.RUAmount(amt), isoToRU(d.PaidAt))
	}
	usdt := ""
	if d.USDTAt != "" {
		txs := storage.ParseTxs(d.Transactions)
		usdt = fmt.Sprintf("%s %s · %s", util.RU(d.VAAmount, 2), d.VAType, isoToRU(d.USDTAt))
		if len(txs) > 1 {
			usdt += fmt.Sprintf(" · %d транзакции", len(txs))
		}
	}
	act := ""
	if d.ActAt != "" {
		act = isoToRU(d.ActAt)
	}
	if d.ActType == "buy" {
		return []stageItem{
			{"usdt", "📥 USDT получен от клиента", d.USDTAt != "", usdt},
			{"payment", "💸 Рубли выплачены клиенту", d.PaidAt != "", paid},
			{"act", "📄 Акт", d.ActAt != "", act},
		}
	}
	inv := ""
	switch {
	case d.InvoicedAt == "-":
		inv = "не нужен"
	case d.InvoicedAt != "":
		inv = isoToRU(d.InvoicedAt)
	}
	return []stageItem{
		{"invoice", "🧾 Счёт на оплату", d.InvoiceDone(), inv},
		{"payment", "💰 Оплата ₽ получена", d.PaidAt != "", paid},
		{"usdt", "💸 USDT выплачен клиенту", d.USDTAt != "", usdt},
		{"act", "📄 Акт", d.ActAt != "", act},
	}
}

var stageNames = map[string]string{
	"invoice": "⏳ ждёт счёта", "payment": "⏳ ждёт оплаты", "usdt": "⏳ ждёт USDT",
	"act": "⏳ ждёт акта", "completed": "✅ завершена", "cancelled": "❌ отменена",
}

func stageName(d *storage.Deal) string {
	s := d.Stage()
	if d.ActType == "buy" {
		switch s {
		case "usdt":
			return "⏳ ждём USDT от клиента"
		case "payment":
			return "⏳ выплатить рубли"
		}
	}
	return stageNames[s]
}

// progressText — этапы сделки с отметками (HTML).
func progressText(d *storage.Deal) string {
	lines := []string{"<b>Статус:</b> " + stageName(d)}
	cur := d.Stage()
	for _, st := range dealStages(d) {
		mark := "⬜"
		switch {
		case st.done:
			mark = "✅"
		case st.key == cur:
			mark = "👉"
		}
		line := mark + " " + st.title
		if st.detail != "" {
			line += " — " + escape(st.detail)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// ─── Черновик ↔ сделка ────────────────────────────────────────────────────────

// draftFromDeal — данные сделки для шаблона; kind = invoice | act.
func draftFromDeal(d *storage.Deal, kind string) *docgen.Draft {
	txs := storage.ParseTxs(d.Transactions)
	f := map[string]string{
		"act_type": d.ActType, "company_id": fmt.Sprint(d.CompanyID.Int64), "deal_number": d.DealNumber,
		"deal_date": d.DealDate, "execution_date": d.ExecutionDate, "va_type": d.VAType, "network": d.Network,
		"va_amount": util.Num(d.VAAmount), "fiat_amount": util.Num(d.FiatAmount), "exchange_rate": d.ExchangeRate,
		"kvvo": d.KVVO, "client_wallet": d.ClientWallet, "operator_wallet": d.OperatorWallet,
		"commission_fiat": d.CommissionFiat, "commission_va": d.CommissionVA,
	}
	if kind == "invoice" {
		f["act_type"] = "invoice"
	}
	if d.ClientBankID.Valid {
		f["cl_bank_id"] = fmt.Sprint(d.ClientBankID.Int64)
	}
	if d.OperatorBankID.Valid {
		f["op_bank_id"] = fmt.Sprint(d.OperatorBankID.Int64)
	}
	if len(txs) == 1 {
		f["tx_hash"] = txs[0].TxHash
	}
	if len(txs) <= 1 {
		txs = nil // одна транзакция — берётся из полей сделки
	}
	return &docgen.Draft{Fields: f, Txs: txs}
}

// dealFromDraft — запись журнала из данных диалога (без документов и этапов).
func (a *App) dealFromDraft(c *Ctx, d *docgen.Draft, client *storage.Company, op map[string]string) *storage.Deal {
	companyID, _ := parseID(d.Get("company_id"))
	txsJSON, _ := json.Marshal(d.Txs)
	if d.Txs == nil {
		txsJSON = []byte("[]")
	}
	nd := &storage.Deal{
		ActType: d.GetOr("act_type", "sell"), DealNumber: d.Get("deal_number"), DealDate: d.Get("deal_date"),
		DealDateISO: dateISO(d.Get("deal_date")), ExecutionDate: d.Get("execution_date"),
		CompanyID:  sql.NullInt64{Int64: companyID, Valid: true},
		ClientName: client.ShortName, ClientINN: client.INN, ClientResident: client.Resident,
		VAType: d.Get("va_type"), Network: d.Get("network"),
		VAAmount: util.ToFloat(d.Get("va_amount")), FiatAmount: util.ToFloat(d.Get("fiat_amount")), FiatCurrency: "RUB",
		ExchangeRate: d.Get("exchange_rate"), KVVO: d.Get("kvvo"),
		ClientWallet: d.Get("client_wallet"), OperatorWallet: d.Get("operator_wallet"),
		CommissionFiat: d.Get("commission_fiat"), CommissionVA: d.Get("commission_va"),
		Transactions: string(txsJSON), CreatedBy: c.userName,
	}
	if id, ok := parseID(d.Get("cl_bank_id")); ok {
		nd.ClientBankID = sql.NullInt64{Int64: id, Valid: true}
	}
	if id, ok := parseID(d.Get("op_bank_id")); ok {
		nd.OperatorBankID = sql.NullInt64{Int64: id, Valid: true}
	}
	// Снимок параметров прибыли: поздняя смена настроек историю не переписывает
	nd.TaxPercent, nd.FeePercent = 5, 0.05
	if v, ok := util.ParseFloat(op["tax_percent"]); ok {
		nd.TaxPercent = v
	}
	if v, ok := util.ParseFloat(op["bank_fee_percent"]); ok {
		nd.FeePercent = v
	}
	nd.ReferrerName, nd.ReferralPercent, _ = a.st.CompanyReferral(c, client)
	return nd
}

// createDeal — «✅ Создать сделку»: запись без документов, этап «счёт» / «USDT».
func (a *App) createDeal(c *Ctx) {
	d := c.act()
	companyID, _ := parseID(d.Get("company_id"))
	client, err := a.st.Company(c, companyID)
	if err != nil || client == nil {
		c.Send("❌ Компания не найдена — начните заново.", backToMainKB())
		return
	}
	op, err := a.st.Settings(c)
	if err != nil {
		c.fail(err)
		return
	}
	nd := a.dealFromDraft(c, d, client, op)
	nd.Transactions = "[]"
	id, err := a.st.AddDeal(c, nd)
	if err != nil {
		c.fail(err)
		return
	}
	if n, ok := parseDealNumber(nd.DealNumber); ok {
		_ = a.st.RecordNumberUsed(c, companyID, "act", n)
	}
	c.Answer("✅ Сделка создана", false)
	c.s.Clear()
	if saved, _ := a.st.Deal(c, id); saved != nil {
		a.notifyNew("deal", id, dealNotifyText(saved))
	}
	warn := ""
	if nd.DealDateISO == "" {
		warn = "⚠️ Дата заявки не распознана — сделка не попадёт в отчёты по датам.\n\n"
	}
	a.showDealCardMsg(c, id, "✅ <b>Сделка создана</b>\n"+warn+"\n")
}

// saveDealEdit — «💾 Сохранить» при переделке; уже сформированные документы
// пересоздаются, руководителям — «было → стало».
func (a *App) saveDealEdit(c *Ctx) {
	d := c.act()
	id, _ := parseID(d.Get("editing_deal_id"))
	old, err := a.st.Deal(c, id)
	if err != nil || old == nil {
		c.Send("❌ Сделка не найдена", backToMainKB())
		return
	}
	client, _ := a.st.Company(c, old.CompanyID.Int64)
	op, _ := a.st.Settings(c)
	if client == nil {
		c.Send("❌ Клиент удалён из базы", backToMainKB())
		return
	}
	nd := a.dealFromDraft(c, d, client, op)
	nd.ID = id
	if len(d.Txs) == 0 { // транзакции не трогали — оставить записанные на этапе выплаты
		nd.Transactions = old.Transactions
		if txs := storage.ParseTxs(old.Transactions); len(txs) == 1 && d.Get("tx_hash") != "" {
			b, _ := json.Marshal([]storage.Tx{{VAAmount: d.Get("va_amount"), TxHash: d.Get("tx_hash")}})
			nd.Transactions = string(b)
		}
	}
	if err := a.st.UpdateDeal(c, nd); err != nil {
		c.fail(err)
		return
	}
	c.Answer("💾 Сохранено", false)
	c.s.Clear()
	cur, _ := a.st.Deal(c, id)
	regen := []string{}
	if cur != nil && cur.InvoiceDocxFileID != "" {
		if a.makeDoc(c, cur, "invoice") == nil {
			regen = append(regen, "счёт")
		}
	}
	if cur != nil && cur.Completed() && cur.DocxFileID != "" {
		if a.makeDoc(c, cur, "act") == nil {
			regen = append(regen, "акт")
		}
	}
	changes := dealDiff(old, nd)
	if len(changes) == 0 {
		changes = []string{"• данные без изменений"}
	}
	if len(regen) > 0 {
		changes = append(changes, "• пересозданы: "+strings.Join(regen, ", "))
	}
	a.refreshDealNotify(id, fmt.Sprintf("✏️ Сделка №%s изменена (версия %d)%s:\n%s",
		escape(nd.DealNumber), old.Version+1, by(c.userName), strings.Join(changes, "\n")))
	if old.BuybackID.Valid {
		a.refreshBuybackNotify(old.BuybackID.Int64, "")
	}
	a.showDealCardMsg(c, id, "💾 <b>Изменения сохранены</b>\n\n")
}

// ─── Документы ────────────────────────────────────────────────────────────────

// makeDoc формирует счёт (invoice) или акт (act), отправляет и сохраняет file_id.
func (a *App) makeDoc(c *Ctx, deal *storage.Deal, kind string) error {
	client, err := a.st.Company(c, deal.CompanyID.Int64)
	if err != nil || client == nil {
		c.Send("❌ Клиент сделки не найден в базе", nil)
		return errors.New("нет клиента")
	}
	op, err := a.st.Settings(c)
	if err != nil {
		c.fail(err)
		return err
	}
	opMap, clientMap, custom := a.docMaps(c, op, client, deal.ClientBankID, deal.OperatorBankID)
	word := "Акт"
	if kind == "invoice" {
		word = "Счёт на оплату"
	}
	status := c.SendPlain("⏳ "+word+": создаю DOCX...", nil)
	defer func() {
		if status != nil {
			c.Delete(status.ID)
		}
	}()
	docxPath, err := docgen.Generate(a.cfg.TemplatesDir, a.cfg.OutputDir, draftFromDeal(deal, kind), opMap, clientMap, custom)
	if err != nil {
		if errors.Is(err, docgen.ErrTemplateNotFound) {
			c.Send("❌ <b>Шаблон не найден</b>\n\nЗагрузите его через меню 📄 Шаблоны.\n\nДетали: "+escape(err.Error()), nil)
		} else {
			c.Send("❌ <b>Ошибка генерации</b>\n<code>"+escape(err.Error())+"</code>", nil)
		}
		return err
	}
	if status != nil {
		_, _ = a.b.EditMessageText(c, editPlain(c.chatID, status.ID, "⏳ "+word+": конвертирую в PDF..."))
	}
	pdfPath, pdfErr := a.pdf.Convert(c, docxPath)
	if pdfErr != nil {
		pdfPath = ""
	}
	defer cleanup.TryRemove(docxPath, pdfPath)

	caption := fmt.Sprintf("✅ <b>%s №%s от %s</b>\n%s\nКлиент: %s", word, escape(deal.DealNumber), escape(deal.DealDate),
		directionLabels[deal.ActType], escape(deal.ClientName))
	mDocx, err := c.SendFile(docxPath, filepath.Base(docxPath), caption+"\n📄 DOCX", true)
	if err != nil {
		c.Send("❌ Не удалось отправить документ: <code>"+escape(err.Error())+"</code>", nil)
		return err
	}
	var docxID, pdfID, pdfName string
	if mDocx.Document != nil {
		docxID = mDocx.Document.FileID
	}
	if pdfErr == nil {
		if m, err := c.SendFile(pdfPath, filepath.Base(pdfPath), caption+"\n📑 PDF", true); err == nil && m.Document != nil {
			pdfID, pdfName = m.Document.FileID, filepath.Base(pdfPath)
		}
	} else {
		c.Send("⚠️ PDF не создан:\n<code>"+escape(pdfErr.Error())+"</code>", nil)
	}
	if kind == "invoice" {
		date := deal.InvoicedAt
		if date == "" || date == "-" {
			date = todayISO()
		}
		return a.st.SetDealInvoice(c, deal.ID, docxID, pdfID, filepath.Base(docxPath), pdfName, date)
	}
	date := deal.ActAt
	if date == "" {
		date = todayISO()
	}
	return a.st.SetDealAct(c, deal.ID, docxID, pdfID, filepath.Base(docxPath), pdfName, date)
}

// ─── Этапы: кнопки ────────────────────────────────────────────────────────────

func (a *App) stageCallback(c *Ctx, rest string) {
	parts := strings.Split(rest, ":")
	id := mustID(parts[1])
	d, err := a.st.Deal(c, id)
	if err != nil || d == nil {
		c.Answer("Сделка не найдена", true)
		return
	}
	if d.Cancelled() && parts[0] != "more" {
		c.Answer("Сделка отменена", true)
		return
	}
	arg := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}
	switch parts[0] {
	case "inv": // сформировать счёт
		c.Answer("⏳ Формирую счёт...", false)
		if a.makeDoc(c, d, "invoice") != nil {
			return
		}
		a.refreshDealNotify(id, fmt.Sprintf("🧾 Сделка №%s: счёт на оплату выставлен%s", escape(d.DealNumber), by(c.userName)))
		a.showDealCardMsg(c, id, "🧾 <b>Счёт сформирован</b> — отправьте его клиенту.\n\n")
	case "invskip":
		_ = a.st.SetDealInvoice(c, id, "", "", "", "", "-")
		a.showDealCard(c, id)
	case "paid": // подтвердить оплату ₽: сумма → дата
		c.s.Clear()
		c.s.State = stDealPaidAmount
		c.s.Data["stage_deal"] = parts[1]
		word := "Оплата от клиента получена"
		if d.ActType == "buy" {
			word = "Рубли клиенту выплачены"
		}
		c.Edit(fmt.Sprintf("💰 <b>%s</b> — сделка №%s\nСумма по сделке: %s ₽\n\nНажмите, если сумма совпадает, или введите фактическую:",
			word, escape(d.DealNumber), util.RUAmount(d.FiatAmount)),
			ikb(row(btn("✅ "+util.RUAmount(d.FiatAmount)+" ₽", "st:paidamt:"+parts[1])), row(btn("◀️ Назад", "deal:"+parts[1]))))
	case "paidamt":
		if c.s.State != stDealPaidAmount {
			return
		}
		c.s.Data["paid_amount"] = util.Num(d.FiatAmount)
		a.askStageDate(c, stDealPaidDate, "st:paidtoday:"+parts[1])
	case "paidtoday":
		if c.s.State != stDealPaidDate {
			return
		}
		a.savePaid(c, d, todayISO())
	case "usdt": // выплата/получение USDT: транзакции (одна или тест + остаток)
		c.s.Clear()
		c.s.Act = draftFromDeal(d, "act")
		c.s.Act.Txs = nil
		c.s.Act.Fields["stage_deal"] = parts[1]
		c.s.State = stActSplitMode
		word := "💸 <b>Выплата USDT клиенту</b>"
		if d.ActType == "buy" {
			word = "📥 <b>USDT от клиента</b>"
		}
		to := d.ClientWallet
		if d.ActType == "buy" {
			to = d.OperatorWallet
		}
		c.Edit(fmt.Sprintf("%s — сделка №%s\nСумма: <b>%s %s</b> (%s)\nНа кошелёк: <code>%s</code>\n\nОдной транзакцией или частями (тест + остаток)?",
			word, escape(d.DealNumber), util.RU(d.VAAmount, 2), escape(d.VAType), escape(d.Network), escape(to)), splitModeKB())
	case "usdttoday":
		if c.s.State != stDealUSDTDate {
			return
		}
		a.saveUSDT(c, todayISO())
	case "act":
		if d.Stage() != "act" && arg(2) != "force" {
			missing := []string{}
			if d.PaidAt == "" {
				missing = append(missing, "оплата ₽")
			}
			if d.USDTAt == "" {
				missing = append(missing, "выплата USDT")
			}
			c.EditPlain("⚠️ Ещё не отмечено: "+strings.Join(missing, ", ")+".\nСформировать акт всё равно?", ikb(row(
				btn("📄 Да, сформировать", "st:act:"+parts[1]+":force"), btn("❌ Нет", "deal:"+parts[1]))))
			return
		}
		c.Answer("⏳ Формирую акт...", false)
		if a.makeDoc(c, d, "act") != nil {
			return
		}
		a.refreshDealNotify(id, fmt.Sprintf("✅ Сделка №%s завершена — акт сформирован%s", escape(d.DealNumber), by(c.userName)))
		a.showDealCardMsg(c, id, "✅ <b>Сделка завершена</b> — акт сформирован.\n\n")
	case "more":
		a.showDealMore(c, d)
	case "undo":
		step, msg := arg(2), ""
		switch step {
		case "invoice":
			_ = a.st.SetDealInvoice(c, id, "", "", "", "", "")
			msg = "счёт"
		case "payment":
			_ = a.st.SetDealPaid(c, id, "", 0, "")
			msg = "оплата ₽"
		case "usdt":
			_ = a.st.SetDealUSDT(c, id, "", "")
			msg = "выплата USDT"
		case "act":
			_ = a.st.SetDealAct(c, id, "", "", "", "", "")
			msg = "акт (сделка снова в работе)"
		}
		a.refreshDealNotify(id, fmt.Sprintf("↩️ Сделка №%s: снята отметка — %s%s", escape(d.DealNumber), msg, by(c.userName)))
		a.showDealCard(c, id)
	case "regen":
		kind := arg(2)
		c.Answer("⏳ Пересоздаю...", false)
		if a.makeDoc(c, d, kind) == nil {
			a.showDealCardMsg(c, id, "🔄 Документ пересоздан\n\n")
		}
	}
}

func (a *App) showDealMore(c *Ctx, d *storage.Deal) {
	sid := fmt.Sprint(d.ID)
	var rows []kbRow
	if !d.Cancelled() {
		rows = append(rows, row(btn("✏️ Изменить данные сделки", "dealx:edit:"+sid)))
		if d.InvoiceDocxFileID != "" {
			rows = append(rows, row(btn("🔄 Пересоздать счёт", "st:regen:"+sid+":invoice")))
		}
		if d.Completed() {
			rows = append(rows, row(btn("🔄 Пересоздать акт", "st:regen:"+sid+":act")))
		}
		for _, st := range dealStages(d) {
			if st.done && !(st.key == "invoice" && d.InvoicedAt == "") {
				rows = append(rows, row(btn("↩️ Снять отметку: "+st.title, "st:undo:"+sid+":"+st.key)))
			}
		}
		rows = append(rows, row(btn("❌ Отменить сделку", "dealx:cancel:"+sid)))
	} else {
		rows = append(rows, row(btn("↩️ Восстановить", "dealx:restore:"+sid)), row(btn("🗑 Удалить из журнала", "deal_del:"+sid)))
	}
	rows = append(rows, row(btn("◀️ К сделке", "deal:"+sid)))
	c.EditPlain("⚙️ Сделка №"+d.DealNumber+" — действия:", ikb(rows...))
}

func (a *App) askStageDate(c *Ctx, state, todayCB string) {
	c.s.State = state
	c.Reply("Дата (ДД.ММ.ГГГГ) или «Сегодня»:", skipKB(todayCB, "Сегодня ("+today()+")"))
}

func (a *App) savePaid(c *Ctx, d *storage.Deal, dateISO string) {
	amount := util.ToFloat(c.s.Data["paid_amount"])
	c.s.Clear()
	if err := a.st.SetDealPaid(c, d.ID, dateISO, amount, ""); err != nil {
		c.fail(err)
		return
	}
	word := "оплата получена"
	if d.ActType == "buy" {
		word = "рубли выплачены клиенту"
	}
	diff := ""
	if amount != d.FiatAmount {
		diff = fmt.Sprintf(" (по сделке %s ₽)", util.RUAmount(d.FiatAmount))
	}
	a.refreshDealNotify(d.ID, fmt.Sprintf("💰 Сделка №%s: %s — %s ₽%s, %s%s", escape(d.DealNumber), word,
		util.RUAmount(amount), diff, isoToRU(dateISO), by(c.userName)))
	a.showDealCardMsg(c, d.ID, "💰 <b>Оплата отмечена</b>\n\n")
}

// afterTxs — транзакции введены на этапе выплаты USDT → дата.
func (a *App) afterTxs(c *Ctx) {
	if c.act().Get("standalone") != "" { // документ без сделки — дальше комиссия и подтверждение
		a.askCommission(c)
		return
	}
	sid := c.act().Get("stage_deal")
	a.askStageDate(c, stDealUSDTDate, "st:usdttoday:"+sid)
}

func (a *App) saveUSDT(c *Ctx, dateISO string) {
	d := c.act()
	id := mustID(d.Get("stage_deal"))
	txs := d.Txs
	if len(txs) == 0 {
		txs = []storage.Tx{{VAAmount: d.Get("va_amount"), TxHash: d.Get("tx_hash")}}
	}
	b, _ := json.Marshal(txs)
	c.s.Clear()
	deal, _ := a.st.Deal(c, id)
	if deal == nil {
		return
	}
	if err := a.st.SetDealUSDT(c, id, dateISO, string(b)); err != nil {
		c.fail(err)
		return
	}
	var hashes []string
	for _, t := range txs {
		h := t.TxHash
		if h != "" && h != "-" {
			if u := chain.ExplorerURL(deal.Network, h); u != "" {
				h = fmt.Sprintf(`<a href="%s">%s</a>`, u, escape(storage.ShortAddr(h)))
			} else {
				h = escape(storage.ShortAddr(h))
			}
		} else {
			h = "без хэша"
		}
		hashes = append(hashes, fmt.Sprintf("%s %s (%s)", escape(t.VAAmount), escape(deal.VAType), h))
	}
	word := "USDT отправлен клиенту"
	if deal.ActType == "buy" {
		word = "USDT получен от клиента"
	}
	a.refreshDealNotify(id, fmt.Sprintf("💸 Сделка №%s: %s — %s · %s%s", escape(deal.DealNumber), word,
		strings.Join(hashes, " + "), isoToRU(dateISO), by(c.userName)))
	a.showDealCardMsg(c, id, "💸 <b>Выплата USDT отмечена</b>\n\n")
}

func (a *App) stageMessage(c *Ctx, text string) {
	switch c.s.State {
	case stDealPaidAmount:
		v, ok := util.ParseFloat(text)
		if !ok || v <= 0 {
			c.SendPlain("❌ Введите сумму в рублях", nil)
			return
		}
		c.s.Data["paid_amount"] = util.Num(v)
		a.askStageDate(c, stDealPaidDate, "st:paidtoday:"+c.s.Data["stage_deal"])
	case stDealPaidDate:
		if !validDate(text) {
			c.SendPlain("❌ Формат: ДД.ММ.ГГГГ", nil)
			return
		}
		d, _ := a.st.Deal(c, mustID(c.s.Data["stage_deal"]))
		if d == nil {
			c.s.Clear()
			return
		}
		a.savePaid(c, d, dateISO(text))
	case stDealUSDTDate:
		if !validDate(text) {
			c.SendPlain("❌ Формат: ДД.ММ.ГГГГ", nil)
			return
		}
		a.saveUSDT(c, dateISO(text))
	}
}

// nextStepButtons — главная кнопка следующего этапа.
func nextStepButtons(d *storage.Deal) []kbRow {
	sid := fmt.Sprint(d.ID)
	switch d.Stage() {
	case "invoice":
		return []kbRow{row(btn("🧾 Сформировать счёт на оплату", "st:inv:"+sid)), row(btn("⏭ Без счёта", "st:invskip:"+sid))}
	case "payment":
		if d.ActType == "buy" {
			return []kbRow{row(btn("💸 Рубли выплачены клиенту", "st:paid:"+sid))}
		}
		return []kbRow{row(btn("💰 Оплата получена", "st:paid:"+sid))}
	case "usdt":
		if d.ActType == "buy" {
			return []kbRow{row(btn("📥 USDT получен от клиента", "st:usdt:"+sid))}
		}
		return []kbRow{row(btn("💸 USDT выплачен клиенту", "st:usdt:"+sid))}
	case "act":
		return []kbRow{row(btn("📄 Сформировать акт", "st:act:"+sid))}
	}
	return nil
}

// showDealCardMsg — карточка сделки новым сообщением (после документов) или правкой.
func (a *App) showDealCardMsg(c *Ctx, id int64, prefix string) {
	d, err := a.st.Deal(c, id)
	if err != nil || d == nil {
		return
	}
	signed, _ := a.st.CountSignedDocs(c, id)
	c.Send(prefix+dealCardText(d, signed), dealCardKB2(d, d.DocxFileID != "" || d.InvoiceDocxFileID != "", signed))
}

// docMaps — реквизиты оператора и клиента для шаблона с учётом выбранных счетов.
func (a *App) docMaps(c *Ctx, op map[string]string, client *storage.Company, clBank, opBank sql.NullInt64) (
	opMap, clientMap, custom map[string]string) {
	custom, _ = a.st.CompanyCustom(c, client.ID)
	clientMap, opMap = client.Map(), map[string]string{}
	for k, v := range op {
		opMap[k] = v
	}
	if clBank.Valid {
		if b, _ := a.st.BankAccount(c, clBank.Int64); b != nil {
			clientMap["bank_name"], clientMap["bank_account"], clientMap["bank_bik"] = b.BankName, b.Account, b.BIK
		}
	}
	if opBank.Valid {
		if b, _ := a.st.BankAccount(c, opBank.Int64); b != nil {
			opMap["bank_name"], opMap["bank_account"], opMap["bank_bik"] = b.BankName, b.Account, b.BIK
		}
	}
	return opMap, clientMap, custom
}

// ─── Документ без сделки ──────────────────────────────────────────────────────

func (a *App) startStandalone(c *Ctx) {
	c.s.Clear()
	c.s.State = stActSelectType
	c.act().Fields["standalone"] = "1"
	c.Reply("📄 <b>Документ без сделки</b>\n\nСформировать акт или счёт по данным, которые вы введёте. "+
		"В журнал сделок, остатки, отчёты и уведомления руководителям он <b>не попадёт</b>, нумерация не сдвинется.\n\n"+
		"Что сформировать?", standaloneTypeKB())
}

// generateStandalone — сформировать и отправить документ, ничего не сохраняя.
func (a *App) generateStandalone(c *Ctx) {
	d := c.act()
	companyID, _ := parseID(d.Get("company_id"))
	client, err := a.st.Company(c, companyID)
	if err != nil || client == nil {
		c.Send("❌ Компания не найдена — начните заново.", backToMainKB())
		return
	}
	op, err := a.st.Settings(c)
	if err != nil {
		c.fail(err)
		return
	}
	var clBank, opBank sql.NullInt64
	if id, ok := parseID(d.Get("cl_bank_id")); ok {
		clBank = sql.NullInt64{Int64: id, Valid: true}
	}
	if id, ok := parseID(d.Get("op_bank_id")); ok {
		opBank = sql.NullInt64{Int64: id, Valid: true}
	}
	opMap, clientMap, custom := a.docMaps(c, op, client, clBank, opBank)
	c.Answer("⏳ Формирую документ...", false)
	status := c.SendPlain("⏳ Создаю DOCX...", nil)
	dropStatus := func() {
		if status != nil {
			c.Delete(status.ID)
			status = nil
		}
	}
	defer dropStatus()
	docxPath, err := docgen.Generate(a.cfg.TemplatesDir, a.cfg.OutputDir, d, opMap, clientMap, custom)
	if err != nil {
		if errors.Is(err, docgen.ErrTemplateNotFound) {
			c.Send("❌ <b>Шаблон не найден</b>\n\nЗагрузите его через меню 📄 Шаблоны.\n\nДетали: "+escape(err.Error()), nil)
		} else {
			c.Send("❌ <b>Ошибка генерации</b>\n<code>"+escape(err.Error())+"</code>", nil)
		}
		return
	}
	pdfPath, pdfErr := a.pdf.Convert(c, docxPath)
	if pdfErr != nil {
		pdfPath = ""
	}
	defer cleanup.TryRemove(docxPath, pdfPath)

	word := map[string]string{"sell": "Акт", "buy": "Акт", "invoice": "Счёт на оплату"}[d.Get("act_type")]
	caption := fmt.Sprintf("📄 <b>%s №%s от %s</b>\n%s\nКлиент: %s\n<i>Без сохранения в журнал</i>", word,
		escape(d.Get("deal_number")), escape(d.Get("deal_date")), directionLabels[d.Get("act_type")], escape(client.ShortName))
	if _, err := c.SendFile(docxPath, filepath.Base(docxPath), caption+"\n📄 DOCX", true); err != nil {
		c.Send("❌ Не удалось отправить документ: <code>"+escape(err.Error())+"</code>", nil)
		return
	}
	if pdfErr == nil {
		_, _ = c.SendFile(pdfPath, filepath.Base(pdfPath), caption+"\n📑 PDF", true)
	} else {
		c.Send("⚠️ PDF не создан:\n<code>"+escape(pdfErr.Error())+"</code>", nil)
	}
	dropStatus()
	c.s.Clear()
	c.SendPlain("✅ Готово. Документ не сохранён в журнал.", ikb(
		row(btn("📄 Ещё документ без сделки", "menu:standalone")),
		row(btn("🏠 Главное меню", "menu:main"))))
}
