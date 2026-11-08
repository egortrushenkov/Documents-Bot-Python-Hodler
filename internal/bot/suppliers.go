package bot

import (
	"fmt"
	"strings"

	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/util"
)

const (
	stSupName   = "sup:name"
	stSupRename = "sup:rename"
)

func (a *App) showSuppliers(c *Ctx, prefix string) {
	list, err := a.st.Suppliers(c)
	if err != nil {
		c.fail(err)
		return
	}
	var rows []kbRow
	for _, s := range list {
		rows = append(rows, row(btn("🏭 "+s.Name, fmt.Sprintf("sup:view:%d", s.ID))))
	}
	rows = append(rows, row(btn("➕ Добавить поставщика", "sup:add")), row(btn("◀️ К откупкам", "bb:menu")))
	text := prefix + "🏭 <b>Поставщики</b> — у кого откупаем USDT.\n" +
		"Их реквизиты и кошельки хранятся в карточке; если USDT придёт с кошелька поставщика, " +
		"в уведомлении о поступлении будет его название."
	if len(list) == 0 {
		text += "\n\n<i>Пока нет ни одного. Поставщик также создаётся сам, если ввести новое название при откупке.</i>"
	}
	c.Reply(text, ikb(rows...))
}

func (a *App) supplierByID(c *Ctx, id int64) *storage.Supplier {
	list, _ := a.st.Suppliers(c)
	for _, s := range list {
		if s.ID == id {
			return &s
		}
	}
	return nil
}

func (a *App) showSupplier(c *Ctx, id int64, prefix string) {
	sp := a.supplierByID(c, id)
	if sp == nil {
		c.Answer("Поставщик не найден", true)
		return
	}
	bbs, _ := a.st.SupplierBuybacks(c, sp.ID, sp.Name)
	var rub, usdt float64
	last := ""
	for _, b := range bbs {
		rub += b.RubAmount
		usdt += b.USDT()
		if last == "" {
			last = b.BuyDate
		}
	}
	banks, _ := a.st.BankAccounts(c, storage.OwnerSupplier, id)
	wallets, _ := a.st.Wallets(c, storage.OwnerSupplier, id)
	lines := []string{prefix + "🏭 <b>" + escape(sp.Name) + "</b>", ""}
	if len(bbs) == 0 {
		lines = append(lines, "<i>Откупок у поставщика пока нет.</i>")
	} else {
		avg := 0.0
		if usdt > 0 {
			avg = rub / usdt
		}
		lines = append(lines, fmt.Sprintf("🔁 Откупок: %d · последняя %s", len(bbs), escape(last)),
			fmt.Sprintf("Переведено: %s ₽ → куплено %s USDT", util.RUAmount(rub), util.RU(usdt, 2)),
			fmt.Sprintf("Средний курс: %s", util.RU(avg, 2)))
	}
	for _, b := range banks {
		if b.IsDefault {
			lines = append(lines, "", "🏦 "+escape(b.Title()))
		}
	}
	for _, w := range wallets {
		if w.IsDefault {
			lines = append(lines, "₿ "+escape(w.Network)+" <code>"+escape(w.Address)+"</code>")
		}
	}
	sid := fmt.Sprint(id)
	c.Reply(strings.Join(lines, "\n"), ikb(
		row(btn("➕ Откупка у этого поставщика", "sup:bb:"+sid)),
		row(btn(fmt.Sprintf("🏦 Реквизиты (%d)", len(banks)), "acc:list:s"+sid), btn(fmt.Sprintf("₿ Кошельки (%d)", len(wallets)), "wal:list:s"+sid)),
		row(btn("✏️ Переименовать", "sup:ren:"+sid), btn("🗑 Удалить", "sup:del:"+sid)),
		row(btn("◀️ К поставщикам", "sup:menu")),
	))
}

func (a *App) suppliersCallback(c *Ctx, rest string) {
	parts := strings.Split(rest, ":")
	arg := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}
	id := mustID(arg(1))
	switch parts[0] {
	case "menu":
		c.s.Clear()
		a.showSuppliers(c, "")
	case "view":
		c.s.Clear()
		a.showSupplier(c, id, "")
	case "add":
		c.s.Clear()
		c.s.State = stSupName
		c.Edit("➕ <b>Новый поставщик</b>\n\nНазвание, как оно будет в отчётах (например: <i>ООО «БРАЙТУМ»</i>):", nil)
	case "ren":
		c.s.Clear()
		c.s.State = stSupRename
		c.s.Data["sup_id"] = arg(1)
		c.EditPlain("Новое название (обновится и во всех откупках этого поставщика):", nil)
	case "del":
		c.EditPlain("Удалить поставщика? Откупки останутся с его названием, реквизиты и кошельки поставщика удалятся.",
			ikb(row(btn("✅ Удалить", "sup:delyes:"+arg(1)), btn("❌ Нет", "sup:view:"+arg(1)))))
	case "delyes":
		if err := a.st.DeleteSupplier(c, id); err != nil {
			c.fail(err)
			return
		}
		a.showSuppliers(c, "🗑 Удалено\n\n")
	case "bb": // новая откупка с уже выбранным поставщиком
		sp := a.supplierByID(c, id)
		if sp == nil {
			c.Answer("Поставщик не найден", true)
			return
		}
		c.s.Clear()
		c.s.Data["bb_supplier"], c.s.Data["bb_supplier_id"], c.s.Data["bb_presup"] = sp.Name, arg(1), "1"
		a.showSelect(c)
	}
}

func (a *App) suppliersMessage(c *Ctx, text string) {
	name := strings.TrimSpace(text)
	if name == "" {
		return
	}
	switch c.s.State {
	case stSupName:
		for _, s := range func() []storage.Supplier { l, _ := a.st.Suppliers(c); return l }() {
			if strings.EqualFold(s.Name, name) {
				c.s.Clear()
				a.showSupplier(c, s.ID, "ℹ️ Такой поставщик уже есть\n\n")
				return
			}
		}
		id, err := a.st.EnsureSupplier(c, name)
		if err != nil {
			c.fail(err)
			return
		}
		c.s.Clear()
		a.showSupplier(c, id, "✅ Поставщик добавлен\n\n")
	case stSupRename:
		id := mustID(c.s.Data["sup_id"])
		if err := a.st.RenameSupplier(c, id, name); err != nil {
			c.fail(err)
			return
		}
		c.s.Clear()
		a.showSupplier(c, id, "✅ Переименован\n\n")
	}
}
