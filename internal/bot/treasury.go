package bot

import (
	"fmt"
	"strings"
	"time"

	"hodler-doc-bot/internal/profit"
	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/util"
)

const (
	stTrOpenUSDT = "tr:open_usdt"
	stTrOpenRub  = "tr:open_rub"
	stTrOpenDate = "tr:open_date"
	stTrAmount   = "tr:amount"
	stTrNote     = "tr:note"
)

var opKinds = [][3]string{ // ключ, название, знак (+/−/±)
	{"deposit", "➕ Пополнение", "+"},
	{"withdraw", "💸 Вывод прибыли", "-"},
	{"referral", "👥 Выплата рефералу", "-"},
	{"tax", "🧾 Оплата налога", "-"},
	{"correction", "🛠 Корректировка", "±"},
}

func opKind(k string) (label, sign string) {
	for _, o := range opKinds {
		if o[0] == k {
			return o[1], o[2]
		}
	}
	return k, "±"
}

func signed(f float64, unit string) string {
	switch {
	case f > 0.005:
		return "+" + util.RU(f, 2) + " " + unit
	case f < -0.005:
		return "−" + util.RU(-f, 2) + " " + unit
	}
	return "0 " + unit
}

// treasury считает остатки на текущий момент.
func (a *App) treasury(c *Ctx) (profit.Treasury, error) {
	opening, err := a.st.Opening(c)
	if err != nil {
		return profit.Treasury{}, err
	}
	from := ""
	if opening != nil {
		from = opening.OpDate
	}
	deals, err := a.st.DealsSince(c, from)
	if err != nil {
		return profit.Treasury{}, err
	}
	bbs, err := a.st.BuybacksSince(c, from)
	if err != nil {
		return profit.Treasury{}, err
	}
	ops, err := a.st.TreasuryOps(c, from)
	if err != nil {
		return profit.Treasury{}, err
	}
	return profit.ComputeTreasury(opening, deals, bbs, ops), nil
}

func isoToRU(iso string) string {
	if t, err := time.Parse("2006-01-02", iso); err == nil {
		return t.Format("02.01.2006")
	}
	return iso
}

// treasuryText — текст остатков (HTML), одинаковый в боте и в группе.
func treasuryText(t profit.Treasury) string {
	lines := []string{
		"💰 <b>Остатки на " + time.Now().Format("02.01.2006 15:04") + "</b>",
		fmt.Sprintf("<b>USDT:</b> %s%s", util.RU(t.USDT, 2), minusWarn(t.USDT)),
		fmt.Sprintf("<b>₽ на счёте:</b> %s%s", util.RU(t.Rub, 2), minusWarn(t.Rub)),
		"",
	}
	if t.FromISO != "" {
		lines = append(lines, fmt.Sprintf("<b>С начала учёта (%s):</b>", isoToRU(t.FromISO)),
			fmt.Sprintf("Начальный остаток: %s USDT · %s ₽", util.RU(t.OpeningUSDT, 2), util.RU(t.OpeningRub, 2)))
	} else {
		lines = append(lines, "<i>Начальный остаток не задан — считаю с нуля от начала журнала.</i>")
	}
	lines = append(lines, fmt.Sprintf("📤 Продажи клиентам (%d): %s · %s", t.SellCount, signed(-t.SellUSDT, "USDT"), signed(t.SellRub, "₽")),
		fmt.Sprintf("🔁 Откупки (%d): %s · %s, комиссии банка %s ₽", t.BBCount, signed(t.BBUSDT, "USDT"),
			signed(-t.BBRub, "₽"), util.RU(t.Fees, 2)))
	if t.BuyCount > 0 {
		lines = append(lines, fmt.Sprintf("📥 Покупки у клиентов (%d): %s · %s", t.BuyCount, signed(t.BuyUSDT, "USDT"), signed(-t.BuyRub, "₽")))
	}
	if t.ManualCount > 0 {
		lines = append(lines, fmt.Sprintf("✍️ Ручные операции (%d): %s · %s", t.ManualCount, signed(t.ManualUSDT, "USDT"), signed(t.ManualRub, "₽")))
	}
	if t.OpenCount > 0 {
		s := fmt.Sprintf("\n⏳ <b>Выдано наперёд без откупки:</b> %d сд. · %s USDT\nРубли от этих клиентов (%s ₽) уже на счёте",
			t.OpenCount, util.RU(t.OpenUSDT, 2), util.RUAmount(t.OpenRub))
		if t.LastBuyRate > 0 {
			s += fmt.Sprintf("; откупка по последнему курсу %s вернёт ≈ %s USDT", util.RU(t.LastBuyRate, 2), util.RU(t.OpenEstimatedBuyUSD, 2))
		}
		lines = append(lines, s)
	}
	lines = append(lines, "", fmt.Sprintf("📈 <b>Прибыль с начала учёта:</b> ЧКД %s · налог %s · чистая %s USDT",
		util.RU(t.CKD, 2), util.RU(t.Tax, 2), util.RU(t.Net, 2)))
	if debts := t.RefOwed(); len(debts) > 0 {
		lines = append(lines, "👥 <b>Рефералы</b> (начислено / выплачено / к выплате):")
		total := 0.0
		for _, d := range debts {
			lines = append(lines, fmt.Sprintf("• %s: %s / %s / <b>%s</b> USDT", escape(d.Name),
				util.RU(d.Accrued, 2), util.RU(d.Paid, 2), util.RU(d.Owed(), 2)))
			total += d.Owed()
		}
		lines = append(lines, fmt.Sprintf("Итого к выплате: <b>%s USDT</b>", util.RU(total, 2)))
	}
	if t.Tax > 0 || t.TaxPaid > 0 {
		lines = append(lines, fmt.Sprintf("🧾 Налог: начислено %s · оплачено %s · осталось %s USDT",
			util.RU(t.Tax, 2), util.RU(t.TaxPaid, 2), util.RU(t.Tax-t.TaxPaid, 2)))
	}
	return strings.Join(lines, "\n")
}

func minusWarn(f float64) string {
	if f < -0.005 {
		return " ⚠️ меньше нуля — проверьте начальный остаток и операции"
	}
	return ""
}

func (a *App) showTreasury(c *Ctx, prefix string) {
	t, err := a.treasury(c)
	if err != nil {
		c.fail(err)
		return
	}
	c.Reply(prefix+treasuryText(t), ikb(
		row(btn("➕ Операция", "tr:op"), btn("🕐 Операции", "tr:list")),
		row(btn("📥 Начальный остаток", "tr:opening")),
		row(btn("📦 Поставщики и клиенты", "tr:stats")),
		row(btn("🏠 Главное меню", "menu:main")),
	))
}

func (a *App) treasuryCallback(c *Ctx, rest string) {
	parts := strings.Split(rest, ":")
	switch parts[0] {
	case "menu":
		c.s.Clear()
		a.showTreasury(c, "")
	case "opening":
		c.s.Clear()
		c.s.State = stTrOpenUSDT
		cur := ""
		if o, _ := a.st.Opening(c); o != nil {
			cur = fmt.Sprintf("\nСейчас: %s USDT · %s ₽ на %s.", util.RU(o.USDT, 2), util.RU(o.Rub, 2), isoToRU(o.OpDate))
		}
		c.Edit("📥 <b>Начальный остаток</b>"+cur+"\n\nС этой даты бот считает остатки: всё, что раньше, в учёт не входит.\n\n"+
			"Сколько <b>USDT</b> у вас сейчас (вся прибыль в тезере)?", nil)
	case "openday":
		if c.s.State != stTrOpenDate {
			return
		}
		a.saveOpening(c, time.Now().Format("2006-01-02"))
	case "op":
		c.s.Clear()
		var rows []kbRow
		for _, k := range opKinds {
			rows = append(rows, row(btn(k[1], "tr:kind:"+k[0])))
		}
		rows = append(rows, row(btn("◀️ Назад", "tr:menu")))
		c.EditPlain("Какая операция? (продажи, покупки и откупки учитываются сами — их вносить не надо)", ikb(rows...))
	case "kind":
		k := parts[1]
		c.s.Clear()
		c.s.Data["tr_kind"] = k
		if k == "referral" {
			a.askReferralPayee(c)
			return
		}
		if k == "tax" {
			c.s.Data["tr_cur"] = "usdt"
			a.askOpAmount(c)
			return
		}
		label, _ := opKind(k)
		c.EditPlain(label+" — в какой валюте?", ikb(row(btn("USDT", "tr:cur:usdt"), btn("₽", "tr:cur:rub")), row(btn("◀️ Назад", "tr:op"))))
	case "cur":
		c.s.Data["tr_cur"] = parts[1]
		a.askOpAmount(c)
	case "payee":
		id, _ := parseID(parts[1])
		r, _ := a.st.Referrer(c, id)
		if r == nil {
			c.Answer("Реферал не найден", true)
			return
		}
		c.s.Data["tr_payee"] = r.Name
		c.s.Data["tr_cur"] = "usdt"
		a.askOpAmount(c)
	case "notedone":
		if c.s.State != stTrNote {
			return
		}
		a.saveOp(c, "")
	case "list":
		ops, err := a.st.RecentTreasuryOps(c, 15)
		if err != nil {
			c.fail(err)
			return
		}
		if len(ops) == 0 {
			c.Answer("Ручных операций нет", true)
			return
		}
		var rows []kbRow
		for _, o := range ops {
			rows = append(rows, row(btn(util.TruncRunes("🗑 "+opLine(o), 60), fmt.Sprintf("tr:del:%d", o.ID))))
		}
		rows = append(rows, row(btn("◀️ Назад", "tr:menu")))
		c.EditPlain("🕐 Ручные операции (нажмите, чтобы удалить):", ikb(rows...))
	case "del":
		id, _ := parseID(parts[1])
		o, _ := a.st.TreasuryOp(c, id)
		if o == nil {
			c.Answer("Не найдена", true)
			return
		}
		c.EditPlain("Удалить операцию?\n"+opLine(o), ikb(row(btn("✅ Удалить", "tr:delyes:"+parts[1]), btn("❌ Нет", "tr:list"))))
	case "delyes":
		id, _ := parseID(parts[1])
		o, _ := a.st.TreasuryOp(c, id)
		if err := a.st.DeleteTreasuryOp(c, id); err != nil {
			c.fail(err)
			return
		}
		if o != nil {
			a.notifyChange("treasury", id, "<s>"+escape(opLine(o))+"</s>", "🗑 Операция удалена"+by(c.userName))
		}
		a.showTreasury(c, "🗑 Удалено\n\n")
	case "stats":
		c.EditPlain("📦 Сводка по поставщикам и клиентам — за какой период?", ikb(
			row(btn("Сегодня", "tr:st:today"), btn("Этот месяц", "tr:st:month")),
			row(btn("Прошлый месяц", "tr:st:prev"), btn("С начала учёта", "tr:st:all")),
			row(btn("◀️ Назад", "tr:menu"))))
	case "st":
		c.Answer("", false)
		a.sendStats(c, parts[1])
	}
}

func opLine(o *storage.TreasuryOp) string {
	label, _ := opKind(o.Kind)
	var amounts []string
	if o.USDT != 0 {
		amounts = append(amounts, signed(o.USDT, "USDT"))
	}
	if o.Rub != 0 {
		amounts = append(amounts, signed(o.Rub, "₽"))
	}
	s := fmt.Sprintf("%s · %s · %s", isoToRU(o.OpDate), label, strings.Join(amounts, " · "))
	if o.ReferrerName != "" {
		s += " · " + o.ReferrerName
	}
	if o.Note != "" {
		s += " · " + o.Note
	}
	return s
}

func (a *App) askReferralPayee(c *Ctx) {
	t, err := a.treasury(c)
	if err != nil {
		c.fail(err)
		return
	}
	refs, _ := a.st.Referrers(c)
	owed := map[string]float64{}
	for _, d := range t.RefOwed() {
		owed[d.Name] = d.Owed()
	}
	var rows []kbRow
	for _, r := range refs {
		rows = append(rows, row(btn(util.TruncRunes(fmt.Sprintf("👤 %s — к выплате %s", r.Name, util.RU(owed[r.Name], 2)), 60), fmt.Sprintf("tr:payee:%d", r.ID))))
	}
	if len(rows) == 0 {
		c.Answer("Сначала добавьте рефералов (👥 Рефералы)", true)
		return
	}
	rows = append(rows, row(btn("◀️ Назад", "tr:op")))
	c.EditPlain("Кому выплата?", ikb(rows...))
}

func (a *App) askOpAmount(c *Ctx) {
	label, sign := opKind(c.s.Data["tr_kind"])
	cur := "USDT"
	if c.s.Data["tr_cur"] == "rub" {
		cur = "₽"
	}
	hint := "Введите сумму, " + cur + ":"
	if sign == "±" {
		hint = "Введите сумму, " + cur + " — со знаком минус, если остаток уменьшается (например: -12,5):"
	}
	if p := c.s.Data["tr_payee"]; p != "" {
		label += " — " + p
	}
	c.s.State = stTrAmount
	c.Edit("<b>"+escape(label)+"</b>\n\n"+hint, nil)
}

func (a *App) saveOpening(c *Ctx, dateISO string) {
	o := &storage.TreasuryOp{OpDate: dateISO, USDT: util.ToFloat(c.s.Data["tr_usdt"]),
		Rub: util.ToFloat(c.s.Data["tr_rub"]), CreatedBy: c.userName, Note: "начальный остаток"}
	if _, err := a.st.SetOpening(c, o); err != nil {
		c.fail(err)
		return
	}
	c.s.Clear()
	a.showTreasury(c, "✅ Начальный остаток сохранён\n\n")
}

func (a *App) saveOp(c *Ctx, note string) {
	kind := c.s.Data["tr_kind"]
	_, sign := opKind(kind)
	amount := util.ToFloat(c.s.Data["tr_amount"])
	if sign == "-" {
		amount = -amount
	}
	o := &storage.TreasuryOp{OpDate: time.Now().Format("2006-01-02"), Kind: kind,
		ReferrerName: c.s.Data["tr_payee"], Note: note, CreatedBy: c.userName}
	if c.s.Data["tr_cur"] == "rub" {
		o.Rub = amount
	} else {
		o.USDT = amount
	}
	id, err := a.st.AddTreasuryOp(c, o)
	if err != nil {
		c.fail(err)
		return
	}
	c.s.Clear()
	a.notifyNew("treasury", id, "💰 <b>"+escape(opLine(o))+"</b>"+by(c.userName))
	a.showTreasury(c, "✅ Операция сохранена\n\n")
}

func (a *App) treasuryMessage(c *Ctx, text string) {
	switch c.s.State {
	case stTrOpenUSDT, stTrOpenRub:
		v, ok := util.ParseFloat(text)
		if !ok {
			c.SendPlain("❌ Введите число, например: 12500,50", nil)
			return
		}
		if c.s.State == stTrOpenUSDT {
			c.s.Data["tr_usdt"] = util.Num(v)
			c.s.State = stTrOpenRub
			c.Send("Сколько <b>рублей</b> на счёте сейчас?", nil)
			return
		}
		c.s.Data["tr_rub"] = util.Num(v)
		c.s.State = stTrOpenDate
		c.Send("С какой даты вести учёт? Сделки и откупки с этой даты будут менять остатки.\n"+
			"Введите ДД.ММ.ГГГГ или нажмите «Сегодня»:", skipKB("tr:openday", "Сегодня ("+today()+")"))
	case stTrOpenDate:
		if !validDate(text) {
			c.SendPlain("❌ Формат: ДД.ММ.ГГГГ", nil)
			return
		}
		a.saveOpening(c, dateISO(text))
	case stTrAmount:
		v, ok := util.ParseFloat(text)
		_, sign := opKind(c.s.Data["tr_kind"])
		if !ok || v == 0 || (sign != "±" && v < 0) {
			c.SendPlain("❌ Введите сумму числом", nil)
			return
		}
		c.s.Data["tr_amount"] = util.Num(v)
		c.s.State = stTrNote
		c.Send("Комментарий (например: «вывод на карту») или нажмите «Без комментария»:", skipKB("tr:notedone", "Без комментария"))
	case stTrNote:
		a.saveOp(c, text)
	}
}

// ─── Сводка по поставщикам и клиентам ─────────────────────────────────────────

func statsPeriod(code string, opening string) (from, to time.Time, title string) {
	now := time.Now()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch code {
	case "today":
		return day, day, "за " + day.Format("02.01.2006")
	case "prev":
		first := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location())
		return first, first.AddDate(0, 1, -1), "за " + first.Format("01.2006")
	case "all":
		from := time.Date(2000, 1, 1, 0, 0, 0, 0, now.Location())
		if t, err := time.Parse("2006-01-02", opening); err == nil {
			from = t
		}
		return from, day, "с начала учёта"
	default:
		first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return first, day, "за " + first.Format("01.2006")
	}
}

func (a *App) statsText(c *Ctx, code string) (string, error) {
	opening, _ := a.st.Opening(c)
	openISO := ""
	if opening != nil {
		openISO = opening.OpDate
	}
	from, to, title := statsPeriod(code, openISO)
	f, t := from.Format("2006-01-02"), to.Format("2006-01-02")
	deals, err := a.st.DealsBetween(c, f, t)
	if err != nil {
		return "", err
	}
	bbs, err := a.st.BuybacksBetween(c, f, t)
	if err != nil {
		return "", err
	}
	sups, clients := profit.Stats(deals, bbs)
	lines := []string{"📦 <b>Поставщики и клиенты " + title + "</b>", "", "🔁 <b>У кого откупали:</b>"}
	if len(sups) == 0 {
		lines = append(lines, "<i>откупок нет</i>")
	}
	var sr, su float64
	for _, s := range sups {
		lines = append(lines, fmt.Sprintf("• %s: %d откуп. · %s ₽ → %s USDT · средний курс %s",
			escape(s.Name), s.Count, util.RUAmount(s.Rub), util.RU(s.USDT, 2), util.RU(s.AvgRate(), 2)))
		sr += s.Rub
		su += s.USDT
	}
	if len(sups) > 1 {
		lines = append(lines, fmt.Sprintf("Итого: %s ₽ → %s USDT", util.RUAmount(sr), util.RU(su, 2)))
	}
	lines = append(lines, "", "📤 <b>Кому продавали:</b>")
	if len(clients) == 0 {
		lines = append(lines, "<i>продаж нет</i>")
	}
	var cr, cv, cn float64
	for _, cl := range clients {
		s := fmt.Sprintf("• %s: %d сд. · %s USDT (%s ₽) · ЧКД %s · чистая %s",
			escape(cl.Name), cl.Count, util.RU(cl.Volume, 2), util.RUAmount(cl.Rub), util.RU(cl.CKD, 2), util.RU(cl.Net, 2))
		if cl.Pending > 0 {
			s += fmt.Sprintf(" · ⏳ %d без откупки", cl.Pending)
		}
		lines = append(lines, s)
		cr += cl.Rub
		cv += cl.Volume
		cn += cl.Net
	}
	if len(clients) > 1 {
		lines = append(lines, fmt.Sprintf("Итого: %s USDT (%s ₽) · чистая %s USDT", util.RU(cv, 2), util.RUAmount(cr), util.RU(cn, 2)))
	}
	return strings.Join(lines, "\n"), nil
}

func (a *App) sendStats(c *Ctx, code string) {
	text, err := a.statsText(c, code)
	if err != nil {
		c.fail(err)
		return
	}
	c.Send(text, nil)
}
