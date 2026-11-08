package bot

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"hodler-doc-bot/internal/profit"
	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/util"
)

const (
	stBBSelect   = "bb:select"
	stBBSupplier = "bb:supplier"
	stBBRate     = "bb:rate"
	stBBRub      = "bb:rub"
	stBBConfirm  = "bb:confirm"
	stBBEdit     = "bb:edit"
)

var bbFieldLabels = map[string]string{
	"rate": "Курс откупки", "rub_amount": "Сумма перевода ₽", "buy_date": "Дата (ДД.ММ.ГГГГ)",
	"fee_percent": "Комиссия банка, %", "note": "Комментарий", "supplier": "Поставщик",
}

// ─── Выбранные сделки хранятся в сессии строкой «12,15,18» ────────────────────

func selIDs(c *Ctx) []int64 {
	var out []int64
	for _, p := range strings.Split(c.s.Data["bb_sel"], ",") {
		if id, ok := parseID(p); ok {
			out = append(out, id)
		}
	}
	return out
}

func setSel(c *Ctx, ids []int64) {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var parts []string
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	c.s.Data["bb_sel"] = strings.Join(parts, ",")
}

func toggleSel(c *Ctx, id int64) {
	ids := selIDs(c)
	for i, x := range ids {
		if x == id {
			setSel(c, append(ids[:i], ids[i+1:]...))
			return
		}
	}
	setSel(c, append(ids, id))
}

func (a *App) selectedDeals(c *Ctx) ([]*storage.Deal, error) {
	var out []*storage.Deal
	for _, id := range selIDs(c) {
		d, err := a.st.Deal(c, id)
		if err != nil {
			return nil, err
		}
		if d != nil && profit.Profitable(d) {
			out = append(out, d)
		}
	}
	return out, nil
}

func sumRub(deals []*storage.Deal) float64 {
	s := 0.0
	for _, d := range deals {
		s += d.FiatAmount
	}
	return s
}

func (a *App) feePercent(c *Ctx) float64 {
	s, _ := a.st.Settings(c)
	if v, ok := util.ParseFloat(s["bank_fee_percent"]); ok {
		return v
	}
	return 0.05
}

// ─── Меню ─────────────────────────────────────────────────────────────────────

func (a *App) showBuybacksMenu(c *Ctx) {
	open, err := a.st.OpenSellDeals(c, 200)
	if err != nil {
		c.fail(err)
		return
	}
	vol, rub := 0.0, 0.0
	for _, d := range open {
		r := profit.Compute(d)
		vol += r.Volume
		rub += r.Rub
	}
	text := "🔁 <b>Откупки</b>\n\nОткупка — покупка USDT у поставщика за рубли. Одна откупка может закрыть " +
		"несколько заявок клиентов; курс откупки определяет ЧКД каждой из них.\n\n"
	if len(open) > 0 {
		text += fmt.Sprintf("⏳ <b>Без откупки:</b> %d сделок · %s USDT · %s ₽\n<i>Это USDT, выданные наперёд из прибыли или ждущие откупки.</i>",
			len(open), util.RU(vol, 2), util.RUAmount(rub))
	} else {
		text += "✅ Все сделки закрыты откупками."
	}
	c.Reply(text, ikb(
		row(btn("➕ Новая откупка", "bb:new")),
		row(btn("⏳ Сделки без откупки", "bb:open")),
		row(btn("🕐 Последние откупки", "bb:list")),
		row(btn("🏭 Поставщики", "sup:menu")),
		row(btn("🏠 Главное меню", "menu:main")),
	))
}

func (a *App) showSelect(c *Ctx) {
	open, err := a.st.OpenSellDeals(c, 30)
	if err != nil {
		c.fail(err)
		return
	}
	selected := map[int64]bool{}
	for _, id := range selIDs(c) {
		selected[id] = true
	}
	var rows []kbRow
	total := 0.0
	for _, d := range open {
		mark := "☑️"
		if selected[d.ID] {
			mark = "✅"
			total += d.FiatAmount
		}
		rows = append(rows, row(btn(util.TruncRunes(fmt.Sprintf("%s №%s · %s · %s ₽", mark, d.DealNumber,
			profit.ShortName(d.ClientName), util.RUAmount(d.FiatAmount)), 60), fmt.Sprintf("bb:t:%d", d.ID))))
	}
	n := len(selected)
	next := fmt.Sprintf("➡️ Далее (%d сд., %s ₽)", n, util.RUAmount(total))
	if n == 0 {
		next = "➡️ Откупка впрок (без сделок)"
	}
	rows = append(rows, row(btn(next, "bb:next")), row(btn("❌ Отмена", "bb:menu")))
	text := "🔁 <b>Новая откупка — шаг 1/4</b>\n\nОтметьте заявки клиентов, которые закрывает эта откупка:"
	if len(open) == 0 {
		text = "🔁 <b>Новая откупка — шаг 1/4</b>\n\nОткрытых заявок нет — можно сделать откупку впрок."
	}
	c.s.State = stBBSelect
	c.Edit(text, ikb(rows...))
}

func (a *App) askSupplier(c *Ctx) {
	sups, err := a.st.Suppliers(c)
	if err != nil {
		c.fail(err)
		return
	}
	var b []btnT
	for _, s := range sups {
		b = append(b, btn(s.Name, fmt.Sprintf("bb:sup:%d", s.ID)))
	}
	rows := grid(2, b...)
	rows = append(rows, row(btn("❌ Отмена", "bb:menu")))
	c.s.State = stBBSupplier
	c.Reply("🔁 <b>Шаг 2/4 — поставщик</b>\nУ кого откупаем USDT? Выберите или введите название нового:", ikb(rows...))
}

func (a *App) askRate(c *Ctx) {
	c.s.State = stBBRate
	c.Reply("🔁 <b>Шаг 3/4 — курс откупки</b>\nПоставщик: <b>"+escape(c.s.Data["bb_supplier"])+
		"</b>\n\nВведите курс, ₽ за 1 USDT (например: 81,40):", nil)
}

func (a *App) askRub(c *Ctx) {
	deals, err := a.selectedDeals(c)
	if err != nil {
		c.fail(err)
		return
	}
	c.s.State = stBBRub
	fee := a.feePercent(c)
	text := "🔁 <b>Шаг 4/4 — сумма перевода поставщику, ₽</b>\n"
	var rows []kbRow
	if clientRub := sumRub(deals); clientRub > 0 {
		exact, rounded := profit.SuggestRub(clientRub, fee)
		text += fmt.Sprintf("\nОт клиентов: <b>%s ₽</b>\nЧтобы хватило на комиссию банка %s%%, можно перевести до <b>%s ₽</b>.\n",
			util.RUAmount(clientRub), strings.TrimSuffix(util.RU(fee, 2), "0"), util.RUAmount(exact))
		rows = append(rows, row(btn("⏩ "+util.RUAmount(exact)+" ₽ (максимум)", fmt.Sprintf("bb:rub:%.0f", exact))))
		if rounded > 0 && rounded != exact {
			rows = append(rows, row(btn("⏩ "+util.RUAmount(rounded)+" ₽ (округлить до 10 000)", fmt.Sprintf("bb:rub:%.0f", rounded))))
		}
	}
	text += "\nВыберите или введите свою сумму:"
	rows = append(rows, row(btn("❌ Отмена", "bb:menu")))
	c.Reply(text, ikb(rows...))
}

func (a *App) draftBuyback(c *Ctx) *storage.Buyback {
	b := &storage.Buyback{
		BuyDate:      today(),
		SupplierName: c.s.Data["bb_supplier"],
		RubAmount:    util.ToFloat(c.s.Data["bb_rub"]),
		Rate:         util.ToFloat(c.s.Data["bb_rate"]),
		FeePercent:   a.feePercent(c),
		CreatedBy:    c.userName,
	}
	b.BuyDateISO = dateISO(b.BuyDate)
	if id, ok := parseID(c.s.Data["bb_supplier_id"]); ok {
		b.SupplierID = sql.NullInt64{Int64: id, Valid: true}
	}
	return b
}

func (a *App) showBBConfirm(c *Ctx) {
	deals, err := a.selectedDeals(c)
	if err != nil {
		c.fail(err)
		return
	}
	b := a.draftBuyback(c)
	text := strings.Replace(buybackNotifyText(b, deals), "🔁 <b>Откупка №0</b>", "🔁 <b>Проверьте откупку</b>", 1)
	c.s.State = stBBConfirm
	c.Reply(text, ikb(row(btn("✅ Сохранить", "bb:save")), row(btn("❌ Отмена", "bb:menu"))))
}

func (a *App) saveBuyback(c *Ctx) {
	b := a.draftBuyback(c)
	if b.Rate <= 0 || b.RubAmount <= 0 {
		c.Answer("Нет курса или суммы", true)
		return
	}
	ids := selIDs(c)
	if !b.SupplierID.Valid {
		if id, err := a.st.EnsureSupplier(c, b.SupplierName); err == nil {
			b.SupplierID = sql.NullInt64{Int64: id, Valid: true}
		}
	}
	id, err := a.st.AddBuyback(c, b, ids)
	if err != nil {
		c.fail(err)
		return
	}
	c.s.Clear()
	c.Answer("✅ Откупка сохранена", false)
	saved, _ := a.st.Buyback(c, id)
	deals, _ := a.st.DealsOfBuyback(c, id)
	a.notifyNew("buyback", id, buybackNotifyText(saved, deals))
	for _, d := range deals { // в уведомлениях о сделках появится курс откупки и прибыль
		a.refreshDealNotify(d.ID, "")
	}
	a.showBuyback(c, id, "✅ <b>Откупка сохранена</b>\n\n")
}

// ─── Карточка откупки ─────────────────────────────────────────────────────────

func (a *App) showBuyback(c *Ctx, id int64, prefix string) {
	b, err := a.st.Buyback(c, id)
	if err != nil {
		c.fail(err)
		return
	}
	if b == nil {
		c.Answer("Откупка не найдена", true)
		return
	}
	deals, err := a.st.DealsOfBuyback(c, id)
	if err != nil {
		c.fail(err)
		return
	}
	var rows []kbRow
	if !b.Cancelled() {
		rows = append(rows,
			row(btn("➕ Добавить сделки", fmt.Sprintf("bb:add:%d", id)), btn("✏️ Изменить", fmt.Sprintf("bb:edit:%d", id))))
		for _, d := range deals {
			rows = append(rows, row(btn(util.TruncRunes("➖ Отвязать №"+d.DealNumber+" "+profit.ShortName(d.ClientName), 60),
				fmt.Sprintf("bb:unlink:%d:%d", id, d.ID))))
		}
		rows = append(rows, row(btn("❌ Отменить откупку", fmt.Sprintf("bb:cancel:%d", id))))
	} else {
		rows = append(rows, row(btn("↩️ Восстановить", fmt.Sprintf("bb:restore:%d", id))))
	}
	rows = append(rows, row(btn("◀️ К откупкам", "bb:menu")))
	text := buybackNotifyText(b, deals)
	if b.Note != "" {
		text += "\n\n💬 " + escape(b.Note)
	}
	c.Reply(prefix+text, ikb(rows...))
}

// ─── Callback-и ───────────────────────────────────────────────────────────────

func (a *App) buybackCallback(c *Ctx, rest string) {
	parts := strings.Split(rest, ":")
	arg := func(i int) int64 {
		if i < len(parts) {
			n, _ := parseID(parts[i])
			return n
		}
		return 0
	}
	switch parts[0] {
	case "menu":
		c.s.Clear()
		a.showBuybacksMenu(c)
	case "new":
		c.s.Clear()
		a.showSelect(c)
	case "t":
		if c.s.State != stBBSelect {
			return
		}
		toggleSel(c, arg(1))
		a.showSelect(c)
	case "next":
		if c.s.State != stBBSelect {
			return
		}
		if target := c.s.Data["bb_target"]; target != "" { // добавление к существующей откупке
			a.addDealsToBuyback(c, target)
			return
		}
		if c.s.Data["bb_presup"] != "" { // поставщик выбран заранее (из его карточки)
			a.askRate(c)
			return
		}
		a.askSupplier(c)
	case "sup":
		if c.s.State != stBBSupplier {
			return
		}
		name, err := a.st.SupplierName(c, arg(1))
		if err != nil || name == "" {
			c.Answer("Поставщик не найден", true)
			return
		}
		c.s.Data["bb_supplier"], c.s.Data["bb_supplier_id"] = name, parts[1]
		if id := c.s.Data["bb_edit_id"]; id != "" { // смена поставщика у существующей откупки
			a.finishSupplierEdit(c, id)
			return
		}
		a.askRate(c)
	case "rub":
		if c.s.State != stBBRub {
			return
		}
		c.s.Data["bb_rub"] = parts[1]
		a.showBBConfirm(c)
	case "save":
		if c.s.State != stBBConfirm {
			return
		}
		a.saveBuyback(c)
	case "list":
		list, err := a.st.RecentBuybacks(c, 15)
		if err != nil {
			c.fail(err)
			return
		}
		if len(list) == 0 {
			c.Answer("Откупок пока нет", true)
			return
		}
		var rows []kbRow
		for _, b := range list {
			mark := "🔁"
			if b.Cancelled() {
				mark = "❌"
			}
			rows = append(rows, row(btn(util.TruncRunes(fmt.Sprintf("%s №%d · %s · %s · %s ₽ по %s", mark, b.ID, b.BuyDate,
				profit.ShortName(b.SupplierName), util.RUAmount(b.RubAmount), util.RU(b.Rate, 2)), 60), fmt.Sprintf("bb:view:%d", b.ID))))
		}
		rows = append(rows, row(btn("◀️ Назад", "bb:menu")))
		c.Edit("🕐 <b>Последние откупки:</b>", ikb(rows...))
	case "open":
		open, err := a.st.OpenSellDeals(c, 30)
		if err != nil {
			c.fail(err)
			return
		}
		if len(open) == 0 {
			c.Answer("Все сделки закрыты откупками", true)
			return
		}
		c.Edit("⏳ <b>Сделки без откупки</b> (USDT выдан наперёд или откупка ещё не сделана):", dealsListKB(open))
	case "view":
		c.s.Clear()
		a.showBuyback(c, arg(1), "")
	case "add":
		c.s.Clear()
		c.s.Data["bb_target"] = parts[1]
		a.showSelect(c)
	case "unlink":
		bbID, dealID := arg(1), arg(2)
		if err := a.st.LinkDealsToBuyback(c, 0, dealID); err != nil {
			c.fail(err)
			return
		}
		c.Answer("Сделка отвязана", false)
		d, _ := a.st.Deal(c, dealID)
		if d != nil {
			a.refreshBuybackNotify(bbID, fmt.Sprintf("➖ От откупки №%d отвязана сделка №%s %s", bbID, escape(d.DealNumber), by(c.userName)))
		}
		a.refreshDealNotify(dealID, "")
		a.showBuyback(c, bbID, "")
	case "cancel":
		c.EditPlain("⚠️ Отменить откупку? Привязанные сделки снова станут «без откупки», "+
			"их прибыль перестанет считаться.", ikb(row(
			btn("✅ Да, отменить", fmt.Sprintf("bb:cancelyes:%d", arg(1))),
			btn("❌ Нет", fmt.Sprintf("bb:view:%d", arg(1))))))
	case "cancelyes", "restore":
		id := arg(1)
		deals, _ := a.st.DealsOfBuyback(c, id)
		status, verb := "cancelled", "❌ Откупка №%d отменена"
		if parts[0] == "restore" {
			status, verb = "active", "↩️ Откупка №%d восстановлена"
		}
		if err := a.st.SetBuybackStatus(c, id, status); err != nil {
			c.fail(err)
			return
		}
		a.refreshBuybackNotify(id, fmt.Sprintf(verb, id)+by(c.userName))
		for _, d := range deals {
			a.refreshDealNotify(d.ID, "")
		}
		a.showBuyback(c, id, "")
	case "edit":
		id := arg(1)
		var rows []kbRow
		for _, k := range []string{"rate", "rub_amount", "supplier", "buy_date", "fee_percent", "note"} {
			rows = append(rows, row(btn("✏️ "+bbFieldLabels[k], fmt.Sprintf("bb:ef:%d:%s", id, k))))
		}
		rows = append(rows, row(btn("◀️ Назад", fmt.Sprintf("bb:view:%d", id))))
		c.EditPlain("Что изменить в откупке?", ikb(rows...))
	case "ef":
		id, field := parts[1], parts[2]
		c.s.Clear()
		c.s.Data["bb_edit_id"] = id
		if field == "supplier" {
			a.askSupplier(c)
			return
		}
		c.s.State = stBBEdit
		c.s.Data["bb_edit_field"] = field
		c.Edit("✏️ <b>"+bbFieldLabels[field]+"</b>\n\nВведите новое значение:", nil)
	}
}

func (a *App) addDealsToBuyback(c *Ctx, target string) {
	bbID, _ := parseID(target)
	ids := selIDs(c)
	if len(ids) == 0 {
		c.Answer("Отметьте хотя бы одну сделку", true)
		return
	}
	if err := a.st.LinkDealsToBuyback(c, bbID, ids...); err != nil {
		c.fail(err)
		return
	}
	var nums []string
	for _, id := range ids {
		if d, _ := a.st.Deal(c, id); d != nil {
			nums = append(nums, "№"+escape(d.DealNumber))
		}
		a.refreshDealNotify(id, "")
	}
	a.refreshBuybackNotify(bbID, fmt.Sprintf("➕ К откупке №%d добавлены сделки: %s%s", bbID, strings.Join(nums, ", "), by(c.userName)))
	c.s.Clear()
	a.showBuyback(c, bbID, "")
}

func (a *App) finishSupplierEdit(c *Ctx, idStr string) {
	id, _ := parseID(idStr)
	old, _ := a.st.Buyback(c, id)
	supID, _ := parseID(c.s.Data["bb_supplier_id"])
	name := c.s.Data["bb_supplier"]
	if supID == 0 {
		supID, _ = a.st.EnsureSupplier(c, name)
	}
	if err := a.st.SetBuybackSupplier(c, id, supID, name); err != nil {
		c.fail(err)
		return
	}
	c.s.Clear()
	if old != nil {
		a.afterBuybackEdit(id, fmt.Sprintf("Поставщик: %s → <b>%s</b>", escape(old.SupplierName), escape(name)), c.userName)
	}
	a.showBuyback(c, id, "✅ Сохранено\n\n")
}

func (a *App) afterBuybackEdit(id int64, change, who string) {
	a.refreshBuybackNotify(id, fmt.Sprintf("✏️ Откупка №%d изменена%s:\n• %s", id, by(who), change))
	ctx, cancel := a.bgCtx()
	defer cancel()
	deals, _ := a.st.DealsOfBuyback(ctx, id)
	for _, d := range deals {
		a.refreshDealNotify(d.ID, "")
	}
}

// buybackMessage — текстовый ввод в диалогах откупки.
func (a *App) buybackMessage(c *Ctx, text string) {
	switch c.s.State {
	case stBBSupplier:
		c.s.Data["bb_supplier"] = text
		delete(c.s.Data, "bb_supplier_id")
		if id := c.s.Data["bb_edit_id"]; id != "" {
			a.finishSupplierEdit(c, id)
			return
		}
		a.askRate(c)
	case stBBRate:
		rate, ok := util.ParseFloat(text)
		if !ok || rate <= 0 {
			c.SendPlain("❌ Введите курс числом, например: 81,40", nil)
			return
		}
		c.s.Data["bb_rate"] = util.Num(rate)
		a.askRub(c)
	case stBBRub:
		rub, ok := util.ParseFloat(text)
		if !ok || rub <= 0 {
			c.SendPlain("❌ Введите сумму в рублях, например: 1160000", nil)
			return
		}
		c.s.Data["bb_rub"] = util.Num(rub)
		a.showBBConfirm(c)
	case stBBEdit:
		id, _ := parseID(c.s.Data["bb_edit_id"])
		field := c.s.Data["bb_edit_field"]
		old, err := a.st.Buyback(c, id)
		if err != nil || old == nil {
			c.s.Clear()
			c.Send("Откупка не найдена", backToMainKB())
			return
		}
		var value any = text
		var oldStr, newStr string
		switch field {
		case "buy_date":
			if !validDate(text) {
				c.SendPlain("❌ Формат: ДД.ММ.ГГГГ", nil)
				return
			}
			oldStr, newStr = old.BuyDate, text
		case "note":
			oldStr, newStr = old.Note, text
		default:
			f, ok := util.ParseFloat(text)
			if !ok || f < 0 || (field != "fee_percent" && f == 0) {
				c.SendPlain("❌ Введите число", nil)
				return
			}
			value = f
			newStr = util.RU(f, 2)
			switch field {
			case "rate":
				oldStr = util.RU(old.Rate, 2)
			case "rub_amount":
				oldStr, newStr = util.RUAmount(old.RubAmount), util.RUAmount(f)
			case "fee_percent":
				oldStr = util.RU(old.FeePercent, 2)
			}
		}
		if err := a.st.UpdateBuybackField(c, id, field, value); err != nil {
			c.fail(err)
			return
		}
		c.s.Clear()
		a.afterBuybackEdit(id, fmt.Sprintf("%s: %s → <b>%s</b>", bbFieldLabels[field], escape(oldStr), escape(newStr)), c.userName)
		a.showBuyback(c, id, "✅ Сохранено\n\n")
	}
}
