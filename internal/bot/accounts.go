package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"hodler-doc-bot/internal/chain"
	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/util"
)

const (
	stAccAdd  = "acc:add"  // пошаговое добавление реквизитов
	stAccEdit = "acc:edit" // правка поля
	stWalAdd  = "wal:add"  // пошаговое добавление кошелька
	stWalEdit = "wal:edit"
)

var bankSteps = [][2]string{
	{"label", "Название (например: Основной, Сбер, Тинькофф)"},
	{"bank_name", "Наименование банка"},
	{"account", "Расчётный счёт"},
	{"bik", "БИК"},
}

var bankLabels = map[string]string{"label": "Название", "bank_name": "Банк", "account": "Р/счёт", "bik": "БИК"}

var networks = []string{"TRC-20", "ERC-20", "BEP-20", "SOL", "TON", "BTC"}

// owner в callback-данных: «op» или «c12».
func ownerCode(ownerType string, id int64) string {
	switch ownerType {
	case storage.OwnerOperator:
		return "op"
	case storage.OwnerSupplier:
		return fmt.Sprintf("s%d", id)
	}
	return fmt.Sprintf("c%d", id)
}

func parseOwner(code string) (string, int64) {
	if code == "op" {
		return storage.OwnerOperator, 0
	}
	if strings.HasPrefix(code, "s") {
		id, _ := parseID(strings.TrimPrefix(code, "s"))
		return storage.OwnerSupplier, id
	}
	id, _ := parseID(strings.TrimPrefix(code, "c"))
	return storage.OwnerCompany, id
}

func (a *App) ownerName(c *Ctx, ownerType string, id int64) string {
	if ownerType == storage.OwnerOperator {
		return "наши"
	}
	if ownerType == storage.OwnerSupplier {
		if sp := a.supplierByID(c, id); sp != nil {
			return sp.Name
		}
		return "поставщик"
	}
	if co, _ := a.st.Company(c, id); co != nil {
		return co.ShortName
	}
	return "клиент"
}

func backToOwner(ownerType string, id int64) btnT {
	if ownerType == storage.OwnerOperator {
		return btn("◀️ К реквизитам", "menu:settings")
	}
	if ownerType == storage.OwnerSupplier {
		return btn("◀️ К поставщику", fmt.Sprintf("sup:view:%d", id))
	}
	return btn("◀️ К компании", fmt.Sprintf("view_company:%d", id))
}

// ─── Банковские реквизиты ─────────────────────────────────────────────────────

func (a *App) showBanks(c *Ctx, ownerType string, id int64, prefix string) {
	list, err := a.st.BankAccounts(c, ownerType, id)
	if err != nil {
		c.fail(err)
		return
	}
	code := ownerCode(ownerType, id)
	var rows []kbRow
	for _, b := range list {
		mark := "🏦"
		if b.IsDefault {
			mark = "⭐"
		}
		rows = append(rows, row(btn(util.TruncRunes(mark+" "+b.Title(), 60), fmt.Sprintf("acc:view:%d", b.ID))))
	}
	rows = append(rows, row(btn("➕ Добавить реквизиты", "acc:add:"+code)), row(backToOwner(ownerType, id)))
	text := fmt.Sprintf("%s🏦 <b>Банковские реквизиты — %s</b>\n⭐ основные подставляются в документы по умолчанию; "+
		"при создании акта можно выбрать другие.", prefix, escape(a.ownerName(c, ownerType, id)))
	if len(list) == 0 {
		text += "\n\n<i>Пока нет ни одних.</i>"
	}
	c.Reply(text, ikb(rows...))
}

func (a *App) showBank(c *Ctx, id int64, prefix string) {
	b, err := a.st.BankAccount(c, id)
	if err != nil || b == nil {
		c.Answer("Не найдены", true)
		return
	}
	sid := fmt.Sprint(id)
	rows := []kbRow{}
	if !b.IsDefault {
		rows = append(rows, row(btn("⭐ Сделать основными", "acc:def:"+sid)))
	}
	rows = append(rows,
		row(btn("✏️ Название", "acc:e:"+sid+":label"), btn("✏️ Банк", "acc:e:"+sid+":bank_name")),
		row(btn("✏️ Р/счёт", "acc:e:"+sid+":account"), btn("✏️ БИК", "acc:e:"+sid+":bik")),
		row(btn("🗑 Удалить", "acc:del:"+sid)),
		row(btn("◀️ К списку", "acc:list:"+ownerCode(b.OwnerType, b.OwnerID))))
	def := ""
	if b.IsDefault {
		def = " ⭐ основные"
	}
	c.Reply(fmt.Sprintf("%s🏦 <b>%s</b>%s\n\n<b>Банк:</b> %s\n<b>Р/счёт:</b> <code>%s</code>\n<b>БИК:</b> <code>%s</code>",
		prefix, escape(b.Label), def, escape(b.BankName), escape(b.Account), escape(b.BIK)), ikb(rows...))
}

func (a *App) askBankStep(c *Ctx) {
	i := 0
	fmt.Sscan(c.s.Data["acc_step"], &i)
	c.s.State = stAccAdd
	c.Reply(fmt.Sprintf("➕ <b>Новые реквизиты — шаг %d/%d</b>\n%s:", i+1, len(bankSteps), bankSteps[i][1]), nil)
}

// ─── Кошельки ─────────────────────────────────────────────────────────────────

func (a *App) showWallets(c *Ctx, ownerType string, id int64, prefix string) {
	list, err := a.st.Wallets(c, ownerType, id)
	if err != nil {
		c.fail(err)
		return
	}
	code := ownerCode(ownerType, id)
	var rows []kbRow
	for _, w := range list {
		mark := "₿"
		if w.IsDefault {
			mark = "⭐"
		}
		if ownerType == storage.OwnerOperator && (w.WatchIn || w.WatchOut) {
			mark += "🔔"
		}
		rows = append(rows, row(btn(util.TruncRunes(mark+" "+w.Title(), 60), fmt.Sprintf("wal:view:%d", w.ID))))
	}
	rows = append(rows, row(btn("➕ Добавить кошелёк", "wal:add:"+code)), row(backToOwner(ownerType, id)))
	text := fmt.Sprintf("%s₿ <b>Кошельки — %s</b>\n⭐ основной подставляется в акт по умолчанию.", prefix, escape(a.ownerName(c, ownerType, id)))
	if ownerType == storage.OwnerOperator {
		text += "\n🔔 — бот присылает уведомления о поступлениях. Откройте кошелёк, чтобы увидеть баланс и последние операции."
	}
	if len(list) == 0 {
		text += "\n\n<i>Пока нет ни одного.</i>"
	}
	c.Reply(text, ikb(rows...))
}

func onOff(b bool) string {
	if b {
		return "✅ вкл"
	}
	return "❌ выкл"
}

// counterpartyName — чей это адрес: клиент из базы или наш кошелёк.
func (a *App) counterpartyName(c context.Context, addr string) string {
	ws, _ := a.st.WalletsByAddress(c, addr)
	for _, w := range ws {
		if w.OwnerType == storage.OwnerOperator {
			return "наш кошелёк «" + w.Label + "»"
		}
		if w.OwnerType == storage.OwnerSupplier {
			if name, _ := a.st.SupplierName(c, w.OwnerID); name != "" {
				return "поставщик " + name
			}
			continue
		}
		if co, _ := a.st.Company(c, w.OwnerID); co != nil {
			return co.ShortName
		}
	}
	return ""
}

func (a *App) showWallet(c *Ctx, id int64, prefix string) {
	w, err := a.st.Wallet(c, id)
	if err != nil || w == nil {
		c.Answer("Не найден", true)
		return
	}
	sid := fmt.Sprint(id)
	def := ""
	if w.IsDefault {
		def = " ⭐ основной"
	}
	lines := []string{fmt.Sprintf("%s₿ <b>%s</b>%s", prefix, escape(w.Label), def),
		"<b>Сеть:</b> " + escape(w.Network), "<b>Адрес:</b> <code>" + escape(w.Address) + "</code>"}
	if u := chain.AddressURL(w.Network, w.Address); u != "" {
		lines = append(lines, `<a href="`+u+`">Открыть в обозревателе</a>`)
	}
	var rows []kbRow
	if w.OwnerType == storage.OwnerOperator {
		lines = append(lines, "", fmt.Sprintf("🔔 Поступления: %s · списания: %s · порог: %s USDT",
			onOff(w.WatchIn), onOff(w.WatchOut), util.RU(w.MinAmount, 2)))
		lines = append(lines, a.walletLive(c, w)...)
		rows = append(rows,
			row(btn("📥 Поступления: "+onOff(w.WatchIn), "wal:in:"+sid), btn("📤 Списания: "+onOff(w.WatchOut), "wal:out:"+sid)),
			row(btn("💵 Порог суммы", "wal:e:"+sid+":min_amount"), btn("🔄 Обновить", "wal:view:"+sid)))
	}
	if !w.IsDefault {
		rows = append(rows, row(btn("⭐ Сделать основным", "wal:def:"+sid)))
	}
	rows = append(rows,
		row(btn("✏️ Название", "wal:e:"+sid+":label"), btn("✏️ Сеть", "wal:e:"+sid+":network"), btn("✏️ Адрес", "wal:e:"+sid+":address")),
		row(btn("🗑 Удалить", "wal:del:"+sid)),
		row(btn("◀️ К кошелькам", "wal:list:"+ownerCode(w.OwnerType, w.OwnerID))))
	c.Reply(strings.Join(lines, "\n"), ikb(rows...))
}

// walletLive — баланс и последние операции из блокчейна.
func (a *App) walletLive(c *Ctx, w *storage.Wallet) []string {
	if !chain.Supported(w.Network) {
		return []string{"", "<i>Баланс и уведомления доступны для TRC-20, ERC-20 и BEP-20.</i>"}
	}
	var lines []string
	b, err := a.chain.GetBalance(c, w.Network, w.Address)
	if err != nil {
		return []string{"", "⚠️ Баланс недоступен: " + escape(err.Error())}
	}
	lines = append(lines, "", fmt.Sprintf("💰 <b>Баланс: %s USDT</b> · %s %s (на комиссии сети)",
		util.RU(b.USDT, 2), util.RU(b.Native, 2), b.Symbol))
	trs, err := a.chain.Transfers(c, w.Network, w.Address, 8)
	if err != nil {
		return append(lines, "⚠️ История недоступна: "+escape(err.Error()))
	}
	if len(trs) == 0 {
		return append(lines, "<i>Операций USDT пока нет.</i>")
	}
	lines = append(lines, "", "<b>Последние операции:</b>")
	for _, t := range trs {
		in := strings.EqualFold(t.To, w.Address)
		arrow, sign, other, dir := "⬇️", "+", t.From, "от"
		if !in {
			arrow, sign, other, dir = "⬆️", "−", t.To, "→"
		}
		who := storage.ShortAddr(other)
		if n := a.counterpartyName(c, other); n != "" {
			who += " (" + escape(n) + ")"
		}
		lines = append(lines, fmt.Sprintf(`%s %s%s · %s · %s %s · <a href="%s">tx</a>`, arrow, sign, util.RU(t.Amount, 2),
			t.Time.Format("02.01 15:04"), dir, who, chain.ExplorerURL(w.Network, t.Hash)))
	}
	return lines
}

func (a *App) askWalletNetwork(c *Ctx) {
	var b []btnT
	for _, n := range networks {
		b = append(b, btn(n, "wal:net:"+n))
	}
	c.s.State = stWalAdd
	c.s.Data["wal_step"] = "network"
	c.Reply("➕ <b>Новый кошелёк — шаг 1/3</b>\nСеть:", ikb(grid(3, b...)...))
}

// ─── Callback-и ───────────────────────────────────────────────────────────────

func (a *App) accountsCallback(c *Ctx, head, rest string) {
	parts := strings.Split(rest, ":")
	arg := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}
	id := mustID(arg(1))
	if head == "acc" {
		switch parts[0] {
		case "list":
			c.s.Clear()
			ot, oid := parseOwner(arg(1))
			a.showBanks(c, ot, oid, "")
		case "view":
			c.s.Clear()
			a.showBank(c, id, "")
		case "add":
			c.s.Clear()
			c.s.Data["acc_owner"] = arg(1)
			c.s.Data["acc_step"] = "0"
			a.askBankStep(c)
		case "def":
			if err := a.st.SetDefaultBank(c, id); err != nil {
				c.fail(err)
				return
			}
			a.showBank(c, id, "✅ Теперь основные\n\n")
		case "e":
			c.s.Clear()
			c.s.State = stAccEdit
			c.s.Data["acc_id"], c.s.Data["acc_field"] = arg(1), arg(2)
			c.EditPlain(bankLabels[arg(2)]+" — новое значение:", nil)
		case "del":
			c.EditPlain("Удалить реквизиты?", ikb(row(btn("✅ Удалить", "acc:delyes:"+arg(1)), btn("❌ Нет", "acc:view:"+arg(1)))))
		case "delyes":
			b, _ := a.st.BankAccount(c, id)
			if err := a.st.DeleteBankAccount(c, id); err != nil {
				c.fail(err)
				return
			}
			if b != nil {
				a.showBanks(c, b.OwnerType, b.OwnerID, "🗑 Удалено\n\n")
			}
		}
		return
	}
	switch parts[0] { // wal
	case "list":
		c.s.Clear()
		ot, oid := parseOwner(arg(1))
		a.showWallets(c, ot, oid, "")
	case "view":
		c.s.Clear()
		a.showWallet(c, id, "")
	case "add":
		c.s.Clear()
		c.s.Data["wal_owner"] = arg(1)
		a.askWalletNetwork(c)
	case "net":
		if c.s.State == stWalEdit { // правка сети
			a.saveWalletField(c, arg(1))
			return
		}
		if c.s.State != stWalAdd {
			return
		}
		c.s.Data["wal_network"] = arg(1)
		c.s.Data["wal_step"] = "address"
		c.Edit("➕ <b>Новый кошелёк — шаг 2/3</b>\nСеть: "+escape(arg(1))+"\n\nАдрес кошелька:", nil)
	case "nolabel":
		if c.s.State == stWalAdd {
			a.saveNewWallet(c, "")
		}
	case "def":
		if err := a.st.SetDefaultWallet(c, id); err != nil {
			c.fail(err)
			return
		}
		a.showWallet(c, id, "✅ Теперь основной\n\n")
	case "in", "out":
		w, _ := a.st.Wallet(c, id)
		if w == nil {
			return
		}
		in, out := w.WatchIn, w.WatchOut
		if parts[0] == "in" {
			in = !in
		} else {
			out = !out
		}
		if (in || out) && !chain.Supported(w.Network) {
			c.Answer("Отслеживание доступно для TRC-20, ERC-20, BEP-20", true)
			return
		}
		if (in || out) && chain.Normalize(w.Network) != "TRC-20" && a.cfg.EtherscanKey == "" {
			c.Answer("Для ERC-20/BEP-20 нужен ETHERSCAN_API_KEY в .env", true)
			return
		}
		if err := a.st.SetWalletWatch(c, id, in, out, time.Now().UnixMilli()); err != nil {
			c.fail(err)
			return
		}
		a.showWallet(c, id, "✅ Сохранено\n\n")
	case "e":
		c.s.Clear()
		c.s.State = stWalEdit
		c.s.Data["wal_id"], c.s.Data["wal_field"] = arg(1), arg(2)
		if arg(2) == "network" {
			var b []btnT
			for _, n := range networks {
				b = append(b, btn(n, "wal:net:"+n))
			}
			c.EditPlain("Сеть:", ikb(grid(3, b...)...))
			return
		}
		prompts := map[string]string{"label": "Название:", "address": "Адрес:",
			"min_amount": "Не уведомлять о суммах меньше, USDT (0 — обо всех):"}
		c.EditPlain(prompts[arg(2)], nil)
	case "del":
		c.EditPlain("Удалить кошелёк?", ikb(row(btn("✅ Удалить", "wal:delyes:"+arg(1)), btn("❌ Нет", "wal:view:"+arg(1)))))
	case "delyes":
		w, _ := a.st.Wallet(c, id)
		if err := a.st.DeleteWallet(c, id); err != nil {
			c.fail(err)
			return
		}
		if w != nil {
			a.showWallets(c, w.OwnerType, w.OwnerID, "🗑 Удалено\n\n")
		}
	}
}

func (a *App) saveWalletField(c *Ctx, value string) {
	id := mustID(c.s.Data["wal_id"])
	field := c.s.Data["wal_field"]
	var v any = value
	if field == "min_amount" {
		f, ok := util.ParseFloat(value)
		if !ok || f < 0 {
			c.SendPlain("❌ Введите число", nil)
			return
		}
		v = f
	}
	if field == "network" {
		v = chain.Normalize(value)
	}
	if err := a.st.UpdateWalletField(c, id, field, v); err != nil {
		c.fail(err)
		return
	}
	c.s.Clear()
	a.showWallet(c, id, "✅ Сохранено\n\n")
}

func (a *App) accountsMessage(c *Ctx, text string) {
	switch c.s.State {
	case stAccAdd:
		i := 0
		fmt.Sscan(c.s.Data["acc_step"], &i)
		c.s.Data["acc_"+bankSteps[i][0]] = text
		if i+1 < len(bankSteps) {
			c.s.Data["acc_step"] = fmt.Sprint(i + 1)
			a.askBankStep(c)
			return
		}
		ot, oid := parseOwner(c.s.Data["acc_owner"])
		id, err := a.st.AddBankAccount(c, &storage.BankAccount{OwnerType: ot, OwnerID: oid, Label: c.s.Data["acc_label"],
			BankName: c.s.Data["acc_bank_name"], Account: c.s.Data["acc_account"], BIK: c.s.Data["acc_bik"]})
		if err != nil {
			c.fail(err)
			return
		}
		c.s.Clear()
		a.showBank(c, id, "✅ Реквизиты добавлены\n\n")
	case stAccEdit:
		id := mustID(c.s.Data["acc_id"])
		if err := a.st.UpdateBankField(c, id, c.s.Data["acc_field"], text); err != nil {
			c.fail(err)
			return
		}
		c.s.Clear()
		a.showBank(c, id, "✅ Сохранено\n\n")
	case stWalAdd:
		switch c.s.Data["wal_step"] {
		case "network": // своя сеть текстом
			c.s.Data["wal_network"] = chain.Normalize(text)
			c.s.Data["wal_step"] = "address"
			c.Send("➕ <b>Новый кошелёк — шаг 2/3</b>\nАдрес кошелька:", nil)
		case "address":
			addr := strings.TrimSpace(text)
			if strings.ContainsAny(addr, " \n") || len(addr) < 20 {
				c.SendPlain("❌ Похоже, это не адрес кошелька — пришлите адрес целиком", nil)
				return
			}
			if g := storage.GuessNetwork(addr); g != "" && chain.Normalize(c.s.Data["wal_network"]) == "TRC-20" && g != "TRC-20" {
				c.SendPlain("⚠️ Адрес не похож на TRC-20 (должен начинаться с T). Проверьте сеть или пришлите другой адрес.", nil)
				return
			}
			c.s.Data["wal_address"] = addr
			c.s.Data["wal_step"] = "label"
			c.Send("➕ <b>Шаг 3/3</b> — название кошелька (например: Основной, Binance, Холодный):", skipKB("wal:nolabel", "Без названия"))
		case "label":
			a.saveNewWallet(c, text)
		}
	case stWalEdit:
		a.saveWalletField(c, text)
	}
}

func (a *App) saveNewWallet(c *Ctx, label string) {
	ot, oid := parseOwner(c.s.Data["wal_owner"])
	net := chain.Normalize(c.s.Data["wal_network"])
	w := &storage.Wallet{OwnerType: ot, OwnerID: oid, Label: label, Network: net, Address: c.s.Data["wal_address"]}
	// Наши TRC-20 кошельки отслеживаются сразу (ERC/BEP — если есть ключ Etherscan)
	if ot == storage.OwnerOperator && chain.Supported(net) && (net == "TRC-20" || a.cfg.EtherscanKey != "") {
		w.WatchIn, w.WatchSince = true, time.Now().UnixMilli()
	}
	id, err := a.st.AddWallet(c, w)
	if err == storage.ErrDuplicateWallet {
		c.s.Clear()
		c.SendPlain("⚠️ Этот кошелёк уже есть в списке.", nil)
		a.showWallets(c, ot, oid, "")
		return
	}
	if err != nil {
		c.fail(err)
		return
	}
	c.s.Clear()
	a.showWallet(c, id, "✅ Кошелёк добавлен\n\n")
}
