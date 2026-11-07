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
	stRefRange     = "ref:range"
	stRefPayAmount = "ref:payamount"
	stRefPayDate   = "ref:paydate"
	stRefPayNote   = "ref:paynote"
	stRefOpEdit    = "ref:opedit"
)

// compact: 2026-11-02 → 20261102 (короткие callback-данные); "" → "-".
func compact(iso string) string {
	if iso == "" {
		return "-"
	}
	return strings.ReplaceAll(iso, "-", "")
}

func uncompact(s string) string {
	if len(s) != 8 {
		return ""
	}
	return s[0:4] + "-" + s[4:6] + "-" + s[6:8]
}

func ddmm(iso string) string {
	if t, err := time.Parse("2006-01-02", iso); err == nil {
		return t.Format("02.01")
	}
	return iso
}

func periodTitle(from, to string) string {
	switch {
	case from == "" && to == "":
		return "за всё время"
	case from == to:
		return "за " + isoToRU(from)
	case from == "":
		return "по " + isoToRU(to)
	}
	return fmt.Sprintf("за %s – %s", ddmm(from), isoToRU(to))
}

func (a *App) refStatement(c *Ctx, r *storage.Referrer, from, to string) (profit.RefStatement, error) {
	deals, err := a.st.ReferrerDeals(c, r.Name)
	if err != nil {
		return profit.RefStatement{}, err
	}
	ops, err := a.st.ReferrerOps(c, r.Name)
	if err != nil {
		return profit.RefStatement{}, err
	}
	return profit.BuildRefStatement(r.Name, deals, ops, from, to), nil
}

func payLine(o *storage.TreasuryOp) string {
	s := fmt.Sprintf("💸 Выплата %s: −%s USDT", ddmm(o.OpDate), util.RU(-o.USDT, 2))
	if o.PeriodFrom != "" {
		s += " (" + periodTitle(o.PeriodFrom, o.PeriodTo) + ")"
	}
	if o.Note != "" {
		s += " — " + escape(o.Note)
	}
	return s
}

func adjLine(o *storage.TreasuryOp) string {
	s := fmt.Sprintf("✍️ Ручное начисление: %s USDT", signed(o.USDT, ""))
	s = strings.TrimSuffix(s, " ")
	if o.Note != "" {
		s += " — " + escape(o.Note)
	}
	return s
}

// refStatementText — расчёт с рефералом по дням (HTML).
func refStatementText(r *storage.Referrer, st profit.RefStatement) string {
	lines := []string{
		fmt.Sprintf("👤 <b>%s — расчёт %s</b>", escape(r.Name), periodTitle(st.From, st.To)),
		fmt.Sprintf("Процент: %s%% от (ЧКД − налог)", util.Fmt2(r.Percent)),
	}
	if st.From != "" {
		lines = append(lines, "", fmt.Sprintf("Долг на начало периода: <b>%s USDT</b>", util.RU(st.OpeningOwed, 2)))
	}
	if len(st.Days) == 0 {
		lines = append(lines, "", "<i>За период нет сделок и выплат.</i>")
	}
	for _, d := range st.Days {
		lines = append(lines, "", "📅 <b>"+isoToRU(d.DateISO)+"</b>")
		for _, l := range d.Lines {
			if l.Pending {
				lines = append(lines, fmt.Sprintf("• №%s %s — %s ₽ · ⏳ ждёт откупки",
					escape(l.Deal.DealNumber), escape(profit.ShortName(l.Deal.ClientName)), util.RUAmount(l.Result.Rub)))
				continue
			}
			pct := ""
			if l.Deal.ReferralPercent != r.Percent {
				pct = fmt.Sprintf(", %s%%", util.Fmt2(l.Deal.ReferralPercent))
			}
			lines = append(lines, fmt.Sprintf("• №%s %s — %s ₽, ЧКД %s%s → <b>%s</b>",
				escape(l.Deal.DealNumber), escape(profit.ShortName(l.Deal.ClientName)), util.RUAmount(l.Result.Rub),
				util.RU(l.Result.CKD, 2), pct, util.RU(l.Amount, 2)))
		}
		for _, o := range d.Adjustments {
			lines = append(lines, adjLine(o))
		}
		if len(d.Lines)+len(d.Adjustments) > 1 {
			lines = append(lines, fmt.Sprintf("Начислено за день: %s USDT", util.RU(d.Accrued(), 2)))
		}
		for _, o := range d.Payouts {
			lines = append(lines, payLine(o))
		}
	}
	lines = append(lines, "",
		fmt.Sprintf("Начислено за период: %s USDT", util.RU(st.Accrued, 2)),
		fmt.Sprintf("Выплачено за период: %s USDT", util.RU(st.Paid, 2)),
		fmt.Sprintf("<b>К выплате на %s: %s USDT</b>", periodEnd(st.To), util.RU(st.ClosingOwed, 2)))
	if st.PendingCount > 0 {
		lines = append(lines, fmt.Sprintf("⏳ Ещё %d сд. (%s ₽) без откупки — реферал по ним начислится после откупки",
			st.PendingCount, util.RUAmount(st.PendingRub)))
	}
	if st.From != "" || st.To != "" {
		lines = append(lines, "", fmt.Sprintf("Всего накоплено: начислено %s · выплачено %s · <b>долг на сегодня %s USDT</b>",
			util.RU(st.TotalAccrued, 2), util.RU(st.TotalPaid, 2), util.RU(st.TotalOwed(), 2)))
	}
	return strings.Join(lines, "\n")
}

func periodEnd(to string) string {
	if to == "" {
		return "сегодня"
	}
	return isoToRU(to)
}

// SendLong — длинный текст несколькими сообщениями (лимит Telegram 4096), клавиатура — у последнего.
func (c *Ctx) SendLong(text string, kb any) {
	const limit = 3800
	parts := []string{}
	cur := ""
	for _, line := range strings.Split(text, "\n") {
		if len(cur)+len(line)+1 > limit && cur != "" {
			parts = append(parts, cur)
			cur = ""
		}
		if cur != "" {
			cur += "\n"
		}
		cur += line
	}
	parts = append(parts, cur)
	for i, p := range parts {
		if i == len(parts)-1 {
			c.Send(p, kb)
		} else {
			c.Send(p, nil)
		}
	}
}

// ─── Карточка реферала ────────────────────────────────────────────────────────

func (a *App) showReferrerCard(c *Ctx, id int64, prefix string) {
	r, err := a.st.Referrer(c, id)
	if err != nil || r == nil {
		c.Answer("Не найден", true)
		return
	}
	st, err := a.refStatement(c, r, "", "")
	if err != nil {
		c.fail(err)
		return
	}
	text := fmt.Sprintf("%s👤 <b>%s</b>\nПроцент: %s%% от (ЧКД − налог)\n\n"+
		"Начислено всего: %s USDT\nВыплачено всего: %s USDT\n<b>Накоплено к выплате: %s USDT</b>",
		prefix, escape(r.Name), util.Fmt2(r.Percent), util.RU(st.TotalAccrued, 2), util.RU(st.TotalPaid, 2), util.RU(st.TotalOwed(), 2))
	if st.PendingCount > 0 {
		text += fmt.Sprintf("\n⏳ %d сд. без откупки — начислится позже", st.PendingCount)
	}
	sid := fmt.Sprint(id)
	c.Reply(text, ikb(
		row(btn("📄 Расчёт за период", "ref:calc:"+sid)),
		row(btn("💸 Записать выплату", "ref:pay:"+sid), btn("🕐 Выплаты", "ref:ops:"+sid)),
		row(btn("✍️ Ручное начисление / долг", "ref:adj:"+sid)),
		row(btn("✏️ Имя", "ref:ename:"+sid), btn("✏️ Процент", "ref:epct:"+sid), btn("🗑", "ref:del:"+sid)),
		row(btn("◀️ К рефералам", "ref:menu")),
	))
}

func (a *App) showRefPeriods(c *Ctx, id string) {
	p := func(label, code string) btnT {
		from, to, _ := profit.Period(code, time.Now())
		return btn(label, fmt.Sprintf("ref:st:%s:%s:%s", id, compact(from), compact(to)))
	}
	c.EditPlain("📄 Расчёт за какой период?", ikb(
		row(p("Эта неделя", "week"), p("Прошлая неделя", "prevweek")),
		row(p("Этот месяц", "month"), p("Прошлый месяц", "prevmonth")),
		row(p("Сегодня", "today"), p("Всё время", "all")),
		row(btn("📅 Свои даты", "ref:range:"+id)),
		row(btn("◀️ Назад", "ref:view:"+id)),
	))
}

func (a *App) sendRefStatement(c *Ctx, id int64, from, to string) {
	r, err := a.st.Referrer(c, id)
	if err != nil || r == nil {
		c.Answer("Не найден", true)
		return
	}
	st, err := a.refStatement(c, r, from, to)
	if err != nil {
		c.fail(err)
		return
	}
	var rows []kbRow
	if st.ClosingOwed > 0.005 {
		rows = append(rows, row(btn(fmt.Sprintf("💸 Выплатить %s USDT (%s)", util.RU(st.ClosingOwed, 2), periodTitle(from, to)),
			fmt.Sprintf("ref:payq:%d:%d:%s:%s", id, int64(st.ClosingOwed*100+0.5), compact(from), compact(to)))))
	}
	rows = append(rows, row(btn("💸 Другая сумма", fmt.Sprintf("ref:payp:%d:%s:%s", id, compact(from), compact(to)))),
		row(btn("📄 Другой период", fmt.Sprintf("ref:calc:%d", id)), btn("◀️ Реферал", fmt.Sprintf("ref:view:%d", id))))
	c.SendLong(refStatementText(r, st), ikb(rows...))
}

// ─── Выплата / ручное начисление ──────────────────────────────────────────────

func (a *App) startPayout(c *Ctx, id, from, to, kind string, amount string) {
	r, _ := a.st.Referrer(c, mustID(id))
	if r == nil {
		c.Answer("Не найден", true)
		return
	}
	c.s.Clear()
	c.s.Data["pay_ref"] = id
	c.s.Data["pay_kind"] = kind
	c.s.Data["pay_from"], c.s.Data["pay_to"] = from, to
	if amount != "" {
		c.s.Data["pay_amount"] = amount
		a.askPayDate(c)
		return
	}
	c.s.State = stRefPayAmount
	if kind == "referral_adj" {
		c.Edit("✍️ <b>Ручное начисление — "+escape(r.Name)+"</b>\n\nСумма в USDT. Плюс — увеличить долг перед рефералом "+
			"(например, долг до запуска бота), минус — уменьшить. На остатки USDT не влияет.\nНапример: <code>120,50</code> или <code>-10</code>", nil)
		return
	}
	st, _ := a.refStatement(c, r, "", "")
	c.Edit(fmt.Sprintf("💸 <b>Выплата — %s</b>\nНакоплено к выплате: %s USDT\n\nВведите сумму выплаты, USDT:",
		escape(r.Name), util.RU(st.TotalOwed(), 2)), nil)
}

func mustID(s string) int64 { n, _ := parseID(s); return n }

func (a *App) askPayDate(c *Ctx) {
	c.s.State = stRefPayDate
	c.Reply(fmt.Sprintf("Сумма: <b>%s USDT</b>\n\nДата операции (ДД.ММ.ГГГГ) или «Сегодня»:",
		util.RU(util.ToFloat(c.s.Data["pay_amount"]), 2)), skipKB("ref:paytoday", "Сегодня ("+today()+")"))
}

func (a *App) askPayNote(c *Ctx) {
	c.s.State = stRefPayNote
	c.Reply("Комментарий (например: «наличными», «на карту Сбер») или нажмите «Без комментария»:",
		skipKB("ref:paynonote", "Без комментария"))
}

func (a *App) savePayout(c *Ctx, note string) {
	id := mustID(c.s.Data["pay_ref"])
	r, _ := a.st.Referrer(c, id)
	if r == nil {
		c.s.Clear()
		return
	}
	amount := util.ToFloat(c.s.Data["pay_amount"])
	kind := c.s.Data["pay_kind"]
	o := &storage.TreasuryOp{OpDate: c.s.Data["pay_date"], Kind: kind, ReferrerName: r.Name, Note: note,
		CreatedBy: c.userName, PeriodFrom: c.s.Data["pay_from"], PeriodTo: c.s.Data["pay_to"]}
	if kind == "referral_adj" {
		o.USDT = amount
	} else {
		o.USDT = -amount
	}
	opID, err := a.st.AddTreasuryOp(c, o)
	if err != nil {
		c.fail(err)
		return
	}
	c.s.Clear()
	st, _ := a.refStatement(c, r, "", "")
	if kind == "referral" {
		a.notifyNew("treasury", opID, fmt.Sprintf("💸 <b>Выплата рефералу %s: %s USDT</b> · %s%s\nОсталось к выплате: %s USDT%s",
			escape(r.Name), util.RU(amount, 2), isoToRU(o.OpDate), periodSuffix(o), util.RU(st.TotalOwed(), 2), by(c.userName)))
	}
	a.showReferrerCard(c, id, "✅ Сохранено\n\n")
}

func periodSuffix(o *storage.TreasuryOp) string {
	if o.PeriodFrom == "" {
		return ""
	}
	return " · " + periodTitle(o.PeriodFrom, o.PeriodTo)
}

// ─── Список выплат и их правка ────────────────────────────────────────────────

func (a *App) showRefOps(c *Ctx, id int64) {
	r, _ := a.st.Referrer(c, id)
	if r == nil {
		c.Answer("Не найден", true)
		return
	}
	ops, err := a.st.ReferrerOps(c, r.Name)
	if err != nil {
		c.fail(err)
		return
	}
	var rows []kbRow
	for i := len(ops) - 1; i >= 0 && len(rows) < 20; i-- { // новые сверху
		o := ops[i]
		label := fmt.Sprintf("💸 %s · −%s USDT", isoToRU(o.OpDate), util.RU(-o.USDT, 2))
		if o.Kind == "referral_adj" {
			label = fmt.Sprintf("✍️ %s · %s USDT", isoToRU(o.OpDate), signed(o.USDT, ""))
		}
		if o.Note != "" {
			label += " · " + o.Note
		}
		rows = append(rows, row(btn(util.TruncRunes(label, 60), fmt.Sprintf("ref:op:%d", o.ID))))
	}
	rows = append(rows, row(btn("◀️ Реферал", fmt.Sprintf("ref:view:%d", id))))
	text := "🕐 <b>Выплаты и начисления — " + escape(r.Name) + "</b>\nНажмите, чтобы изменить или удалить:"
	if len(ops) == 0 {
		text = "🕐 <b>" + escape(r.Name) + "</b>\nВыплат и ручных начислений пока нет."
	}
	c.Edit(text, ikb(rows...))
}

func (a *App) refIDByName(c *Ctx, name string) int64 {
	refs, _ := a.st.Referrers(c)
	for _, r := range refs {
		if r.Name == name {
			return r.ID
		}
	}
	return 0
}

func (a *App) showRefOp(c *Ctx, opID int64, prefix string) {
	o, err := a.st.TreasuryOp(c, opID)
	if err != nil || o == nil {
		c.Answer("Операция не найдена", true)
		return
	}
	line := payLine(o)
	if o.Kind == "referral_adj" {
		line = adjLine(o) + " · " + isoToRU(o.OpDate)
	}
	sid := fmt.Sprint(opID)
	rows := []kbRow{
		row(btn("✏️ Сумма", "ref:oe:"+sid+":amount"), btn("✏️ Дата", "ref:oe:"+sid+":date")),
		row(btn("✏️ Комментарий", "ref:oe:"+sid+":note"), btn("✏️ За период", "ref:oe:"+sid+":period")),
		row(btn("🗑 Удалить", "ref:od:"+sid)),
		row(btn("◀️ К выплатам", fmt.Sprintf("ref:ops:%d", a.refIDByName(c, o.ReferrerName)))),
	}
	c.Reply(prefix+"<b>"+escape(o.ReferrerName)+"</b>\n"+line, ikb(rows...))
}

func parseRange(text string) (from, to string, ok bool) {
	text = strings.NewReplacer("—", "-", "–", "-", " ", "").Replace(text)
	parts := strings.Split(text, "-")
	switch len(parts) {
	case 1:
		if validDate(parts[0]) {
			d := dateISO(parts[0])
			return d, d, true
		}
	case 2:
		if validDate(parts[0]) && validDate(parts[1]) {
			f, t := dateISO(parts[0]), dateISO(parts[1])
			if f > t {
				f, t = t, f
			}
			return f, t, true
		}
	}
	return "", "", false
}

// refCallback2 — расчёты, выплаты и их правка (ref:calc, ref:st, ref:pay...).
func (a *App) refCallback2(c *Ctx, parts []string) bool {
	arg := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}
	switch parts[0] {
	case "view":
		c.s.Clear()
		a.showReferrerCard(c, mustID(arg(1)), "")
	case "calc":
		a.showRefPeriods(c, arg(1))
	case "st":
		c.Answer("", false)
		a.sendRefStatement(c, mustID(arg(1)), uncompact(arg(2)), uncompact(arg(3)))
	case "range":
		c.s.Clear()
		c.s.State = stRefRange
		c.s.Data["ref_id"] = arg(1)
		c.EditPlain("Введите период: 02.11.2026-08.11.2026 (или одну дату):", nil)
	case "pay":
		a.startPayout(c, arg(1), "", "", "referral", "")
	case "payp":
		a.startPayout(c, arg(1), uncompact(arg(2)), uncompact(arg(3)), "referral", "")
	case "payq":
		cents, _ := parseID(arg(2))
		a.startPayout(c, arg(1), uncompact(arg(3)), uncompact(arg(4)), "referral", util.Num(float64(cents)/100))
	case "adj":
		a.startPayout(c, arg(1), "", "", "referral_adj", "")
	case "paytoday":
		if c.s.State != stRefPayDate {
			return true
		}
		c.s.Data["pay_date"] = time.Now().Format("2006-01-02")
		a.askPayNote(c)
	case "paynonote":
		if c.s.State != stRefPayNote {
			return true
		}
		a.savePayout(c, "")
	case "ops":
		c.s.Clear()
		a.showRefOps(c, mustID(arg(1)))
	case "op":
		c.s.Clear()
		a.showRefOp(c, mustID(arg(1)), "")
	case "oe":
		c.s.Clear()
		c.s.State = stRefOpEdit
		c.s.Data["op_id"], c.s.Data["op_field"] = arg(1), arg(2)
		prompts := map[string]string{
			"amount": "Новая сумма, USDT (для ручного начисления можно со знаком минус):",
			"date":   "Новая дата (ДД.ММ.ГГГГ):",
			"note":   "Новый комментарий (или - чтобы убрать):",
			"period": "За какой период выплата: 02.11.2026-08.11.2026, одна дата или - чтобы убрать:",
		}
		c.EditPlain(prompts[arg(2)], nil)
	case "od":
		c.EditPlain("Удалить эту операцию? Долг перед рефералом и остатки пересчитаются.", ikb(row(
			btn("✅ Удалить", "ref:odyes:"+arg(1)), btn("❌ Нет", "ref:op:"+arg(1)))))
	case "odyes":
		id := mustID(arg(1))
		o, _ := a.st.TreasuryOp(c, id)
		if o == nil {
			c.Answer("Не найдена", true)
			return true
		}
		if err := a.st.DeleteTreasuryOp(c, id); err != nil {
			c.fail(err)
			return true
		}
		if o.Kind == "referral" {
			a.notifyChange("treasury", id, "<s>"+payLine(o)+"</s>", "🗑 Выплата рефералу "+escape(o.ReferrerName)+" удалена"+by(c.userName))
		}
		a.showRefOps(c, a.refIDByName(c, o.ReferrerName))
	default:
		return false
	}
	return true
}

func (a *App) refMessage2(c *Ctx, text string) bool {
	switch c.s.State {
	case stRefRange:
		from, to, ok := parseRange(text)
		if !ok {
			c.SendPlain("❌ Формат: 02.11.2026-08.11.2026 или 03.11.2026", nil)
			return true
		}
		id := mustID(c.s.Data["ref_id"])
		c.s.Clear()
		a.sendRefStatement(c, id, from, to)
	case stRefPayAmount:
		v, ok := util.ParseFloat(text)
		if !ok || v == 0 || (c.s.Data["pay_kind"] == "referral" && v < 0) {
			c.SendPlain("❌ Введите сумму числом", nil)
			return true
		}
		c.s.Data["pay_amount"] = util.Num(v)
		a.askPayDate(c)
	case stRefPayDate:
		if !validDate(text) {
			c.SendPlain("❌ Формат: ДД.ММ.ГГГГ", nil)
			return true
		}
		c.s.Data["pay_date"] = dateISO(text)
		a.askPayNote(c)
	case stRefPayNote:
		a.savePayout(c, text)
	case stRefOpEdit:
		id := mustID(c.s.Data["op_id"])
		o, _ := a.st.TreasuryOp(c, id)
		if o == nil {
			c.s.Clear()
			return true
		}
		old := payLine(o)
		switch c.s.Data["op_field"] {
		case "amount":
			v, ok := util.ParseFloat(text)
			if !ok || v == 0 || (o.Kind == "referral" && v < 0) {
				c.SendPlain("❌ Введите сумму числом", nil)
				return true
			}
			if o.Kind == "referral" {
				o.USDT = -v
			} else {
				o.USDT = v
			}
		case "date":
			if !validDate(text) {
				c.SendPlain("❌ Формат: ДД.ММ.ГГГГ", nil)
				return true
			}
			o.OpDate = dateISO(text)
		case "note":
			if text == "-" {
				text = ""
			}
			o.Note = text
		case "period":
			if text == "-" {
				o.PeriodFrom, o.PeriodTo = "", ""
			} else {
				f, t, ok := parseRange(text)
				if !ok {
					c.SendPlain("❌ Формат: 02.11.2026-08.11.2026", nil)
					return true
				}
				o.PeriodFrom, o.PeriodTo = f, t
			}
		}
		if err := a.st.UpdateTreasuryOp(c, o); err != nil {
			c.fail(err)
			return true
		}
		c.s.Clear()
		if o.Kind == "referral" {
			a.notifyChange("treasury", id, "💸 <b>Выплата рефералу "+escape(o.ReferrerName)+"</b>\n"+payLine(o),
				"✏️ Выплата изменена"+by(c.userName)+":\n"+old+" → "+payLine(o))
		}
		a.showRefOp(c, id, "✅ Сохранено\n\n")
	default:
		return false
	}
	return true
}
