package bot

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"hodler-doc-bot/internal/chain"
	"hodler-doc-bot/internal/docgen"
	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/util"
)

var actFieldLabels = map[string]string{
	"deal_number":     "Номер заявки",
	"deal_date":       "Дата заявки",
	"va_type":         "Тип ВА",
	"network":         "Сеть",
	"va_amount":       "Сумма ВА",
	"fiat_amount":     "Сумма RUB",
	"exchange_rate":   "Курс обмена",
	"kvvo":            "КВВО",
	"client_wallet":   "Кошелёк клиента",
	"operator_wallet": "Кошелёк оператора",
	"tx_hash":         "Хэш транзакции",
	"commission_fiat": "Комиссия в RUB",
	"commission_va":   "Комиссия в ВА",
	"execution_date":  "Дата исполнения",
}

var directionLabels = map[string]string{
	"sell":    "📤 Продаём ВА клиенту (покупка у нас)",
	"buy":     "📥 Покупаем ВА у клиента (продажа нам)",
	"invoice": "🧾 Счёт-заявка на покупку клиентом",
}

func today() string { return time.Now().Format("02.01.2006") }

func validDate(s string) bool {
	_, err := time.Parse("02.01.2006", strings.TrimSpace(s))
	return err == nil
}

func dateISO(s string) string {
	t, err := time.Parse("02.01.2006", strings.TrimSpace(s))
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func (c *Ctx) act() *docgen.Draft {
	if c.s.Act == nil {
		c.s.Act = &docgen.Draft{Fields: map[string]string{}}
	}
	return c.s.Act
}

func testsSum(d *docgen.Draft) float64 {
	s := 0.0
	for _, t := range d.Txs {
		s += util.ToFloat(t.VAAmount)
	}
	return s
}

// remainder — остаток ВА = общая сумма минус введённые тесты.
func remainder(d *docgen.Draft) float64 { return util.ToFloat(d.Get("va_amount")) - testsSum(d) }

func dealSummary(d *docgen.Draft, client *storage.Company) string {
	actType := d.GetOr("act_type", "sell")
	g := func(k string) string { return escape(d.GetOr(k, "—")) }
	docWord := "акта"
	if actType == "invoice" {
		docWord = "счёта"
	}
	clientName := "—"
	if client != nil {
		clientName = escape(client.ShortName)
	}
	vaType := escape(d.Get("va_type"))
	lines := []string{
		"<b>📄 Сводка " + docWord + "</b>\n",
		"<b>Тип:</b> " + directionLabels[actType],
		"<b>Клиент:</b> " + clientName,
		"",
		"<b>Номер заявки:</b> №" + g("deal_number"),
		"<b>Дата заявки:</b> " + g("deal_date"),
		"<b>Дата исполнения:</b> " + g("execution_date"),
		"",
		"<b>Актив:</b> " + g("va_type") + " (" + g("network") + ")",
		"<b>Сумма ВА:</b> " + g("va_amount") + " " + vaType,
		"<b>Сумма RUB:</b> " + g("fiat_amount") + " RUB",
		"<b>Курс:</b> " + g("exchange_rate"),
		"<b>КВВО:</b> " + g("kvvo"),
	}
	// Разбивка на транзакции (тесты + остаток) с рублями по курсу
	var rows []docgen.ComputedTx
	if actType != "invoice" {
		rows = docgen.ComputeTransactions(d.Txs, d.Get("va_amount"), d.Get("fiat_amount"), d.Get("tx_hash"))
	}
	if len(rows) > 1 {
		lines = append(lines, fmt.Sprintf("\n<b>Транзакций: %d</b>", len(rows)))
		for i, t := range rows {
			lines = append(lines, fmt.Sprintf("  %d) %s %s = %s RUB — <code>%s</code>",
				i+1, escape(t.VAAmount), vaType, util.Fmt2(t.FiatAmount), escape(t.TxHash)))
		}
	}
	lines = append(lines, "",
		"<b>Кошелёк клиента:</b>\n<code>"+g("client_wallet")+"</code>",
		"<b>Кошелёк оператора:</b>\n<code>"+g("operator_wallet")+"</code>")
	if len(rows) <= 1 && actType != "invoice" && d.Get("tx_hash") != "" {
		lines = append(lines, "<b>Хэш:</b>\n<code>"+g("tx_hash")+"</code>")
	}
	lines = append(lines, "",
		"<b>Комиссия (RUB):</b> "+escape(d.GetOr("commission_fiat", "0%")),
		"<b>Комиссия (ВА):</b> "+escape(d.GetOr("commission_va", "0%")))
	return strings.Join(lines, "\n")
}

// ─── Вход ─────────────────────────────────────────────────────────────────────

func (a *App) startCreateAct(c *Ctx) {
	c.s.Clear()
	c.s.State = stActSelectType
	c.Reply("📋 <b>Новая сделка</b>\n\nВыберите тип операции.\nДальше: счёт → оплата → выплата USDT → акт — всё в карточке сделки.", actTypeKB())
}

// actCallback — кнопки диалога создания акта (с проверкой шага, как в FSM).
func (a *App) actCallback(c *Ctx, head, rest string) {
	st := c.s.State
	switch {
	case head == "act_type" && st == stActSelectType:
		a.stepType(c, rest)
	case head == "sel_company" && st == stActSelectCompany:
		a.stepCompany(c, rest)

	case head == "skip" && rest == "deal_number" && st == stActDealNumber:
		a.askDealDate(c)
	case head == "skip" && rest == "deal_date" && st == stActDealDate:
		a.askVAType(c)
	case head == "skip" && rest == "exchange_rate" && st == stActExchangeRate:
		a.askKVVO(c)
	case head == "skip" && rest == "client_wallet" && st == stActClientWallet:
		a.askOperatorWallet(c)
	case head == "skip" && rest == "operator_wallet" && st == stActOperatorWallet:
		a.afterOperatorWallet(c)
	case head == "skip" && rest == "commission_fiat" && st == stActCommissionFiat:
		a.askCommissionVA(c)
	case head == "skip" && rest == "commission_va" && st == stActCommissionVA:
		a.askExecutionDate(c)
	case head == "skip" && rest == "execution_date" && st == stActExecutionDate:
		a.showConfirm(c)

	case head == "va_type" && st == stActVAType:
		if rest == "custom" {
			c.EditPlain("Введите тип актива (например: USDT, BTC):", nil)
			return
		}
		c.act().Fields["va_type"] = rest
		a.askNetwork(c)
	case head == "network" && st == stActNetwork:
		if rest == "custom" {
			c.EditPlain("Введите название сети (например: TRC-20, ERC-20):", nil)
			return
		}
		c.act().Fields["network"] = rest
		a.askVAAmount(c)

	case head == "kvvo" && st == stActKVVO:
		switch rest {
		case "custom":
			c.EditPlain("Введите код КВВО (он сохранится в списке):", nil)
		case "manage":
			custom, _ := a.customKVVO(c)
			c.EditPlain("Удалить свой КВВО:", kvvoManageKB(custom))
		case "back":
			custom, _ := a.customKVVO(c)
			c.Edit(kvvoPrompt, kvvoKB(custom))
		default:
			c.act().Fields["kvvo"] = rest
			a.askClientWallet(c)
		}
	case head == "kvvo_del" && st == stActKVVO:
		if err := a.st.DeleteKVVO(c, rest); err != nil {
			c.fail(err)
			return
		}
		custom, _ := a.customKVVO(c)
		if len(custom) > 0 {
			c.EditPlain("Удалить свой КВВО:", kvvoManageKB(custom))
		} else {
			c.Edit(kvvoPrompt, kvvoKB(custom))
		}
		c.Answer("Удалён "+rest, false)

	case head == "tx":
		a.txCallback(c, rest)

	case head == "cw" || head == "ow":
		if st != stActClientWallet && st != stActOperatorWallet && st != stActEditField {
			return
		}
		w, _ := a.st.Wallet(c, mustID(rest))
		if w == nil {
			return
		}
		key := "client_wallet"
		if head == "ow" {
			key = "operator_wallet"
		}
		c.act().Fields[key] = w.Address
		switch {
		case st == stActEditField:
			a.showConfirm(c)
		case head == "cw":
			a.askOperatorWallet(c)
		default:
			a.afterOperatorWallet(c)
		}
	case head == "cb" || head == "ob":
		if st != stActClientBank && st != stActOperatorBank && st != stActEditField {
			return
		}
		key := "cl_bank_id"
		if head == "ob" {
			key = "op_bank_id"
		}
		c.act().Fields[key] = rest
		switch {
		case st == stActEditField:
			a.showConfirm(c)
		case head == "cb":
			a.askOperatorBank(c)
		default:
			a.afterBanks(c)
		}

	case head == "act" && rest == "confirm_show" && (st == stActConfirm || st == stActEditField):
		a.showConfirm(c)
	case head == "act" && rest == "edit_menu" && st == stActConfirm:
		c.s.State = stActEditField
		c.EditPlain("Выберите поле для изменения:", editActFieldsKB())
	case head == "edit_act_field" && st == stActEditField && (rest == "client_bank" || rest == "op_bank"):
		d := c.act()
		if rest == "client_bank" {
			cid, _ := parseID(d.Get("company_id"))
			banks, _ := a.st.BankAccounts(c, storage.OwnerCompany, cid)
			cur, _ := parseID(d.Get("cl_bank_id"))
			c.EditPlain("Реквизиты клиента:", ikb(append(bankRows(banks, "cb", cur), row(btn("◀️ Назад", "act:confirm_show")))...))
		} else {
			banks, _ := a.st.BankAccounts(c, storage.OwnerOperator, 0)
			cur, _ := parseID(d.Get("op_bank_id"))
			c.EditPlain("Наши реквизиты:", ikb(append(bankRows(banks, "ob", cur), row(btn("◀️ Назад", "act:confirm_show")))...))
		}
	case head == "edit_act_field" && st == stActEditField && (rest == "client_wallet" || rest == "operator_wallet"):
		d := c.act()
		d.Fields["editing_field"] = rest
		var rows []kbRow
		if rest == "client_wallet" {
			cid, _ := parseID(d.Get("company_id"))
			rows = a.walletRows(c, storage.OwnerCompany, cid, "cw", d.Get("network"), d.Get(rest))
		} else {
			rows = a.walletRows(c, storage.OwnerOperator, 0, "ow", d.Get("network"), d.Get(rest))
		}
		rows = append(rows, row(btn("◀️ Назад", "act:confirm_show")))
		c.Edit("✏️ <b>"+actFieldLabels[rest]+"</b>\nТекущий: <code>"+escape(d.Get(rest))+"</code>\n\nВыберите или пришлите адрес текстом:", ikb(rows...))
	case head == "edit_act_field" && st == stActEditField:
		label := actFieldLabels[rest]
		if label == "" {
			label = rest
		}
		c.act().Fields["editing_field"] = rest
		c.Edit("✏️ <b>"+escape(label)+"</b>\n\nТекущее: <code>"+escape(c.act().Get(rest))+
			"</code>\n\nВведите новое значение:", nil)
	case head == "act" && rest == "generate" && st == stActConfirm:
		if c.act().Get("standalone") != "" {
			a.generateStandalone(c)
		} else if c.act().Get("editing_deal_id") != "" {
			a.saveDealEdit(c)
		} else {
			a.createDeal(c)
		}
	}
}

func (a *App) txCallback(c *Ctx, rest string) {
	st := c.s.State
	d := c.act()
	switch {
	case rest == "single" && st == stActSplitMode:
		c.s.State = stActTxOneHash
		c.Edit("<b>Хэш транзакции</b>\nВведите tx hash (или <code>-</code> если ещё нет):",
			skipKB("tx:one_skip", "Пропустить (-)"))
	case rest == "one_skip" && st == stActTxOneHash:
		a.finishSingle(c, "-")
		a.afterTxs(c)
	case rest == "split" && st == stActSplitMode:
		d.Txs = nil // сюда копятся тесты
		a.askAddTx(c)
	case rest == "add" && st == stActAddTx:
		c.s.State = stActExtraVA
		c.Edit(fmt.Sprintf("<b>Тест-транзакция — сумма %s</b>\nДолжна быть меньше остатка <b>%s</b>.\n\nВведите сумму:",
			escape(d.GetOr("va_type", "ВА")), util.Fmt2(remainder(d))), nil)
	case rest == "extra_skip_hash" && st == stActExtraTxHash:
		a.appendTest(c, "-")
		a.askAddTx(c)
	case rest == "done" && st == stActAddTx:
		rem := remainder(d)
		if rem <= 0 {
			c.Answer("Остаток нулевой — уберите лишние тесты", true)
			return
		}
		d.Fields["_remainder_va"] = util.Fmt2(rem)
		c.s.State = stActRemainderHash
		c.Edit(fmt.Sprintf("<b>Хэш перевода остатка (%s %s)</b>\nВведите tx hash (или <code>-</code> если ещё нет):",
			util.Fmt2(rem), escape(d.Get("va_type"))), skipKB("tx:rem_skip", "Пропустить (-)"))
	case rest == "rem_skip" && st == stActRemainderHash:
		a.finishSplit(c, "-")
		a.afterTxs(c)
	}
}

// actMessage — текстовый ввод на шагах создания акта.
func (a *App) actMessage(c *Ctx, text string) {
	d := c.act()
	switch c.s.State {
	case stActDealNumber:
		d.Fields["deal_number"] = text
		a.askDealDate(c)
	case stActDealDate:
		if !validDate(text) {
			c.Send("❌ Формат: ДД.ММ.ГГГГ — например, 12.04.2026", nil)
			return
		}
		d.Fields["deal_date"] = text
		d.Fields["execution_date"] = text
		a.askVAType(c)
	case stActVAType:
		d.Fields["va_type"] = strings.ToUpper(text)
		a.askNetwork(c)
	case stActNetwork:
		d.Fields["network"] = text
		a.askVAAmount(c)
	case stActVAAmount:
		d.Fields["va_amount"] = strings.ReplaceAll(text, ",", ".")
		c.s.State = stActFiatAmount
		c.Send("<b>Шаг: Сумма в рублях (RUB)</b>\nВведите сумму (например: 500000):", nil)
	case stActFiatAmount:
		a.inputFiatAmount(c, text)
	case stActExchangeRate:
		d.Fields["exchange_rate"] = text
		a.askKVVO(c)
	case stActKVVO:
		// свой код сохраняется для будущих сделок (длинный не влезет в кнопку)
		if text != "" && !isBuiltinKVVO(text) && len("kvvo_del:"+text) <= 64 {
			if err := a.st.AddKVVO(c, text); err != nil {
				c.fail(err)
				return
			}
		}
		d.Fields["kvvo"] = text
		a.askClientWallet(c)
	case stActClientWallet:
		d.Fields["client_wallet"] = text
		a.askOperatorWallet(c)
	case stActOperatorWallet:
		d.Fields["operator_wallet"] = text
		a.afterOperatorWallet(c)
	case stActTxOneHash:
		a.finishSingle(c, text)
		a.afterTxs(c)
	case stActExtraVA:
		a.inputTestVA(c, text)
	case stActExtraTxHash:
		a.appendTest(c, text)
		a.askAddTx(c)
	case stActRemainderHash:
		a.finishSplit(c, text)
		a.afterTxs(c)
	case stActCommissionFiat:
		d.Fields["commission_fiat"] = text
		a.askCommissionVA(c)
	case stActCommissionVA:
		d.Fields["commission_va"] = text
		a.askExecutionDate(c)
	case stActExecutionDate:
		if !validDate(text) {
			c.Send("❌ Формат: ДД.ММ.ГГГГ — например, 12.04.2026", nil)
			return
		}
		d.Fields["execution_date"] = text
		a.showConfirm(c)
	case stActEditField:
		if key := d.Get("editing_field"); key != "" {
			if (key == "deal_date" || key == "execution_date") && !validDate(text) {
				c.Send("❌ Формат: ДД.ММ.ГГГГ — например, 12.04.2026", nil)
				return
			}
			d.Fields[key] = text
			delete(d.Fields, "editing_field")
		}
		a.showConfirm(c)
	}
}

// ─── Шаги ─────────────────────────────────────────────────────────────────────

func (a *App) stepType(c *Ctx, actType string) {
	standalone := c.act().Get("standalone") != ""
	if actType != "sell" && actType != "buy" && !(standalone && actType == "invoice") {
		return
	}
	c.act().Fields["act_type"] = actType
	c.s.State = stActSelectCompany
	cs, err := a.st.Companies(c)
	if err != nil {
		c.fail(err)
		return
	}
	if len(cs) == 0 {
		c.EditPlain("⚠️ Нет ни одной компании в базе.\nСначала добавьте компанию через раздел 🏢 Компании.",
			backToMainKB())
		return
	}
	c.EditPlain("Выберите компанию-клиента:", companiesListKB(cs, true))
}

func (a *App) stepCompany(c *Ctx, idStr string) {
	id, _ := parseID(idStr)
	client, err := a.st.Company(c, id)
	if err != nil {
		c.fail(err)
		return
	}
	if client == nil {
		c.Answer("Компания не найдена", true)
		return
	}
	op, err := a.st.Settings(c)
	if err != nil {
		c.fail(err)
		return
	}
	d := c.act()
	d.Fields["company_id"] = idStr
	d.Fields["client_wallet"] = client.Wallet
	d.Fields["operator_wallet"] = op["wallet"]

	// Нумерация: у каждого клиента своя (акты и счета отдельно).
	// Если истории по клиенту ещё нет — предлагаем общий счётчик.
	kind := "act"
	if d.Get("act_type") == "invoice" {
		kind = "invoice"
	}
	next, ok, err := a.st.PeekCompanyNumber(c, id, kind)
	if err != nil {
		c.fail(err)
		return
	}
	var hint string
	if ok {
		hint = fmt.Sprintf("Следующий номер для этого клиента: <b>%d</b>", next)
	} else {
		if next, err = a.st.PeekNextNumber(c); err != nil {
			c.fail(err)
			return
		}
		hint = fmt.Sprintf("По этому клиенту истории ещё нет — предлагаю общий номер: <b>%d</b>\n"+
			"<i>Введите правильный номер один раз, дальше бот запомнит нумерацию клиента.</i>", next)
	}
	d.Fields["deal_number"] = strconv.FormatInt(next, 10)
	c.s.State = stActDealNumber
	c.Edit(fmt.Sprintf("✅ Клиент: <b>%s</b>\n\n<b>Шаг: Номер заявки</b>\n%s\n\nВведите номер или нажмите «Использовать»:",
		escape(client.ShortName), hint), skipKB("skip:deal_number", fmt.Sprintf("Использовать №%d", next)))
}

func (a *App) askDealDate(c *Ctx) {
	// счётчики двигаются только при генерации — брошенный черновик номер не сжигает
	t := today()
	c.act().Fields["deal_date"] = t
	c.s.State = stActDealDate
	c.Reply("<b>Шаг: Дата заявки</b>\nСегодня: <b>"+t+"</b>\n\nВведите дату (ДД.ММ.ГГГГ) или нажмите «Сегодня»:",
		skipKB("skip:deal_date", "Сегодня ("+t+")"))
}

func (a *App) askVAType(c *Ctx) {
	c.s.State = stActVAType
	c.Reply("<b>Шаг: Тип виртуального актива</b>\nВыберите или введите свой:", vaTypeKB())
}

func (a *App) askNetwork(c *Ctx) {
	c.s.State = stActNetwork
	c.Reply("<b>Шаг: Сеть (blockchain)</b>\nВыберите или введите свою:", networkKB())
}

func (a *App) askVAAmount(c *Ctx) {
	c.s.State = stActVAAmount
	c.Reply("<b>Шаг: Сумма виртуального актива</b>\nАктив: "+escape(c.act().GetOr("va_type", "ВА"))+
		"\n\nВведите сумму (например: 6287.726):", nil)
}

func (a *App) inputFiatAmount(c *Ctx, text string) {
	d := c.act()
	val := strings.ReplaceAll(strings.ReplaceAll(text, " ", ""), ",", ".")
	d.Fields["fiat_amount"] = val

	// Автокурс, если обе суммы распознаны
	rateStr := ""
	va, err1 := strconv.ParseFloat(strings.ReplaceAll(d.Get("va_amount"), ",", "."), 64)
	rub, err2 := strconv.ParseFloat(val, 64)
	if err1 == nil && err2 == nil {
		if va != 0 {
			rateStr = strings.ReplaceAll(util.PyFloat(util.Round2(rub/va)), ".", ",")
		} else {
			rateStr = "0"
		}
		d.Fields["exchange_rate"] = rateStr
	}
	c.s.State = stActExchangeRate
	hint := ""
	var kb = skipKB("skip:exchange_rate", "Использовать "+rateStr)
	if rateStr != "" {
		hint = "\n<i>Автовычисленный курс: " + rateStr + "</i>"
	} else {
		kb = nil
	}
	c.Send("<b>Шаг: Курс обмена ВА к RUB</b>"+hint+"\n\nВведите курс или нажмите «Использовать» авторасчёт:", kb)
}

const kvvoPrompt = "<b>Шаг: КВВО (код вида валютной операции)</b>\nВыберите или введите свой:"

// customKVVO — сохранённые пользователем коды (без встроенных).
func (a *App) customKVVO(c *Ctx) ([]string, error) {
	codes, err := a.st.KVVOCodes(c)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, code := range codes {
		if !isBuiltinKVVO(code) {
			out = append(out, code)
		}
	}
	return out, nil
}

func (a *App) askKVVO(c *Ctx) {
	c.s.State = stActKVVO
	custom, err := a.customKVVO(c)
	if err != nil {
		c.fail(err)
		return
	}
	c.Reply(kvvoPrompt, kvvoKB(custom))
}

// walletRows — кнопки выбора кошелька; кошельки нужной сети — первыми.
func (a *App) walletRows(c *Ctx, ownerType string, ownerID int64, prefix, network, current string) []kbRow {
	ws, _ := a.st.Wallets(c, ownerType, ownerID)
	net := chain.Normalize(network)
	sort.SliceStable(ws, func(i, j int) bool {
		return chain.Normalize(ws[i].Network) == net && chain.Normalize(ws[j].Network) != net
	})
	var rows []kbRow
	for _, w := range ws {
		mark := "₿"
		if w.Address == current {
			mark = "✅"
		}
		warn := ""
		if w.Network != "" && net != "" && chain.Normalize(w.Network) != net {
			warn = " ⚠️"
		}
		rows = append(rows, row(btn(util.TruncRunes(mark+" "+w.Title()+warn, 60), fmt.Sprintf("%s:%d", prefix, w.ID))))
	}
	return rows
}

func (a *App) askClientWallet(c *Ctx) {
	d := c.act()
	c.s.State = stActClientWallet
	cid, _ := parseID(d.Get("company_id"))
	rows := a.walletRows(c, storage.OwnerCompany, cid, "cw", d.Get("network"), d.Get("client_wallet"))
	text := "<b>Шаг: Кошелёк клиента</b>\n"
	if len(rows) > 0 {
		text += "Выберите кошелёк клиента или пришлите другой адрес текстом:"
	} else {
		text += "В карточке клиента кошельков нет — пришлите адрес текстом:"
	}
	if cur := d.Get("client_wallet"); cur != "" {
		rows = append(rows, row(btn("⏩ "+storage.ShortAddr(cur)+" (основной)", "skip:client_wallet")))
	}
	c.Reply(text, ikb(rows...))
}

func (a *App) askOperatorWallet(c *Ctx) {
	d := c.act()
	c.s.State = stActOperatorWallet
	rows := a.walletRows(c, storage.OwnerOperator, 0, "ow", d.Get("network"), d.Get("operator_wallet"))
	if cur := d.Get("operator_wallet"); cur != "" {
		rows = append(rows, row(btn("⏩ Оставить "+storage.ShortAddr(cur), "skip:operator_wallet")))
	}
	c.Reply("<b>Шаг: Наш кошелёк</b>\nВыберите, на какой/с какого кошелька идёт перевод, или пришлите адрес текстом:", ikb(rows...))
}

func bankRows(banks []*storage.BankAccount, prefix string, current int64) []kbRow {
	var rows []kbRow
	for _, b := range banks {
		mark := "🏦"
		if b.ID == current || (current == 0 && b.IsDefault) {
			mark = "✅"
		}
		rows = append(rows, row(btn(util.TruncRunes(mark+" "+b.Title(), 60), fmt.Sprintf("%s:%d", prefix, b.ID))))
	}
	return rows
}

// afterOperatorWallet → выбор реквизитов (если их несколько) → транзакции.
func (a *App) afterOperatorWallet(c *Ctx) { a.askClientBank(c) }

func (a *App) askClientBank(c *Ctx) {
	d := c.act()
	cid, _ := parseID(d.Get("company_id"))
	banks, _ := a.st.BankAccounts(c, storage.OwnerCompany, cid)
	if len(banks) <= 1 {
		if len(banks) == 1 {
			d.Fields["cl_bank_id"] = fmt.Sprint(banks[0].ID)
		}
		a.askOperatorBank(c)
		return
	}
	c.s.State = stActClientBank
	cur, _ := parseID(d.Get("cl_bank_id"))
	c.Reply("<b>Шаг: Реквизиты клиента</b>\nКакие банковские реквизиты клиента указать в документе?", ikb(bankRows(banks, "cb", cur)...))
}

func (a *App) askOperatorBank(c *Ctx) {
	d := c.act()
	banks, _ := a.st.BankAccounts(c, storage.OwnerOperator, 0)
	if len(banks) <= 1 {
		if len(banks) == 1 {
			d.Fields["op_bank_id"] = fmt.Sprint(banks[0].ID)
		}
		a.afterBanks(c)
		return
	}
	c.s.State = stActOperatorBank
	cur, _ := parseID(d.Get("op_bank_id"))
	c.Reply("<b>Шаг: Наши реквизиты</b>\nНа какой наш счёт / с какого счёта?", ikb(bankRows(banks, "ob", cur)...))
}

// afterBanks: транзакции USDT вводятся позже, на этапе выплаты.
func (a *App) afterBanks(c *Ctx) {
	d := c.act()
	// Документ без сделки: транзакции акта вводятся сразу (как раньше)
	if d.Get("standalone") != "" && d.Get("act_type") != "invoice" {
		c.s.State = stActSplitMode
		c.Reply(fmt.Sprintf("<b>Шаг: Транзакции</b>\nОбщая сумма: <b>%s %s</b>\n\n"+
			"Перевод одной транзакцией или разбить на части (например тест + остаток)?",
			escape(d.GetOr("va_amount", "—")), escape(d.Get("va_type"))), splitModeKB())
		return
	}
	a.askCommission(c)
}

// finishSingle: одна транзакция = вся сумма; хэш — в основном поле (для правок) и списком.
func (a *App) finishSingle(c *Ctx, hash string) {
	d := c.act()
	d.Fields["tx_hash"] = hash
	d.Txs = []storage.Tx{{VAAmount: d.Get("va_amount"), TxHash: hash}}
}

func (a *App) askAddTx(c *Ctx) {
	d := c.act()
	vaType := escape(d.Get("va_type"))
	rem := remainder(d)
	c.s.State = stActAddTx
	lines := []string{"<b>Разбивка транзакций</b>",
		fmt.Sprintf("Всего: %s %s", util.Fmt2(util.ToFloat(d.Get("va_amount"))), vaType)}
	for i, t := range d.Txs {
		lines = append(lines, fmt.Sprintf("  тест %d) %s %s — <code>%s</code>", i+1, escape(t.VAAmount), vaType, escape(t.TxHash)))
	}
	lines = append(lines, fmt.Sprintf("<b>Остаток: %s %s</b>", util.Fmt2(rem), vaType),
		"\nДобавьте тест-транзакцию или завершите — остаток уйдёт отдельной строкой.")
	c.Reply(strings.Join(lines, "\n"), addTxKB(util.Fmt2(rem)+" "+d.Get("va_type")))
}

func (a *App) inputTestVA(c *Ctx, text string) {
	d := c.act()
	rem := remainder(d)
	val := strings.ReplaceAll(strings.ReplaceAll(text, ",", "."), " ", "")
	amount, err := strconv.ParseFloat(val, 64)
	if err != nil {
		c.Send("❌ Введите число, например: 100", nil)
		return
	}
	if amount <= 0 {
		c.Send("❌ Сумма должна быть больше 0.", nil)
		return
	}
	if amount >= rem {
		c.SendPlain(fmt.Sprintf("❌ Сумма теста должна быть меньше остатка (%s) — остаток уйдёт отдельной строкой.",
			util.Fmt2(rem)), nil)
		return
	}
	d.Fields["_extra_va"] = val
	c.s.State = stActExtraTxHash
	c.Send("<b>Тест-транзакция — хэш</b>\nВведите tx hash (или <code>-</code> если ещё нет):",
		skipKB("tx:extra_skip_hash", "Пропустить (-)"))
}

func (a *App) appendTest(c *Ctx, hash string) {
	d := c.act()
	d.Txs = append(d.Txs, storage.Tx{VAAmount: d.Get("_extra_va"), TxHash: hash})
}

func (a *App) finishSplit(c *Ctx, hash string) {
	d := c.act()
	d.Txs = append(d.Txs, storage.Tx{VAAmount: d.Get("_remainder_va"), TxHash: hash}) // остаток последним
}

func (a *App) askCommission(c *Ctx) {
	d := c.act()
	c.s.State = stActCommissionFiat
	d.Fields["commission_fiat"] = "0%"
	d.Fields["commission_va"] = "0%"
	c.Reply("<b>Шаг: Комиссия в RUB (%)</b>\nВведите значение (например: 1.5%) или нажмите «0%»:",
		skipKB("skip:commission_fiat", "0%"))
}

func (a *App) askCommissionVA(c *Ctx) {
	c.s.State = stActCommissionVA
	c.Reply("<b>Шаг: Комиссия в ВА (%)</b>\nВведите значение или нажмите «0%»:",
		skipKB("skip:commission_va", "0%"))
}

func (a *App) askExecutionDate(c *Ctx) {
	d := c.act()
	dd := d.GetOr("deal_date", today())
	d.Fields["execution_date"] = dd
	c.s.State = stActExecutionDate
	c.Reply("<b>Шаг: Дата исполнения акта</b>\nПо умолчанию = дата заявки: <b>"+escape(dd)+
		"</b>\n\nВведите другую дату или нажмите «Использовать»:",
		skipKB("skip:execution_date", "= дата заявки ("+dd+")"))
}

func (a *App) showConfirm(c *Ctx) {
	c.s.State = stActConfirm
	d := c.act()
	id, _ := parseID(d.Get("company_id"))
	client, err := a.st.Company(c, id)
	if err != nil {
		c.fail(err)
		return
	}
	prefix := ""
	kb := confirmActKB()
	if d.Get("editing_deal_id") != "" {
		prefix = "✏️ <b>Изменение сделки</b> — поправьте поля и сохраните. Уже сформированные счёт и акт пересоздадутся.\n\n"
		kb = confirmEditKB()
	}
	if d.Get("standalone") != "" {
		prefix = "📄 <b>Документ без сделки</b> — в журнал, остатки и отчёты не попадёт.\n\n"
		kb = confirmStandaloneKB()
	}
	extra := ""
	if bid, ok := parseID(d.Get("cl_bank_id")); ok {
		if b, _ := a.st.BankAccount(c, bid); b != nil {
			extra += "\n<b>Реквизиты клиента:</b> " + escape(b.Title())
		}
	}
	if bid, ok := parseID(d.Get("op_bank_id")); ok {
		if b, _ := a.st.BankAccount(c, bid); b != nil {
			extra += "\n<b>Наши реквизиты:</b> " + escape(b.Title())
		}
	}
	c.Reply(prefix+dealSummary(d, client)+extra, kb)
}

// ─── Генерация ────────────────────────────────────────────────────────────────

func parseDealNumber(s string) (int64, bool) {
	s = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(s), "№"))
	n, err := strconv.ParseInt(strings.TrimPrefix(s, "+"), 10, 64)
	return n, err == nil
}
