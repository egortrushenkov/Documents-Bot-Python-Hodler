package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tg "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"hodler-doc-bot/internal/profit"
	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/util"
)

// ─── Тексты ───────────────────────────────────────────────────────────────────

// profitLines — блок прибыли по сделке (HTML).
func profitLines(d *storage.Deal) string {
	r := profit.Compute(d)
	if !r.HasBuyback {
		return "⏳ <b>Откупка:</b> ещё не сделана — USDT выдан наперёд"
	}
	return "🔁 <b>" + escape(profit.Header(r)) + "</b>\n" + escape(strings.Join(profit.DealBlock(r), "\n"))
}

// notifyProfitLines — как profitLines, но без строки объёма (она уже есть в шапке).
func notifyProfitLines(d *storage.Deal) string {
	r := profit.Compute(d)
	if !r.HasBuyback {
		return "⏳ <b>Откупка:</b> ещё не сделана — USDT выдан наперёд"
	}
	var block []string
	for _, l := range profit.DealBlock(r) {
		if !strings.HasPrefix(l, "Объём:") {
			block = append(block, l)
		}
	}
	return "🔁 <b>" + escape(profit.Header(r)) + "</b>\n" + escape(strings.Join(block, "\n"))
}

// by — « (Имя)» для подписи действия; пусто, если имени нет.
func by(name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}
	return " (" + escape(name) + ")"
}

var notifyTitles = map[string]string{"sell": "📤 Продажа USDT клиенту", "buy": "📥 Покупка USDT у клиента"}

func dealNotifyText(d *storage.Deal) string {
	head := "🆕 <b>Новая сделка"
	switch {
	case d.Cancelled():
		head = "❌ <b>ОТМЕНЕНА — сделка"
	case d.Version > 1:
		head = fmt.Sprintf("✏️ <b>Изменена (версия %d) — сделка", d.Version)
	}
	r := profit.Compute(d)
	lines := []string{
		fmt.Sprintf("%s №%s</b> · %s", head, escape(d.DealNumber), escape(d.DealDate)),
		notifyTitles[d.ActType],
		"<b>Клиент:</b> " + escape(d.ClientName),
		fmt.Sprintf("<b>Объём:</b> %s %s (%s ₽)%s", util.RU(r.Volume, 2), escape(d.VAType),
			util.RUAmount(d.FiatAmount), networkSuffix(d.Network)),
	}
	if d.ActType == "sell" {
		if !r.HasBuyback {
			lines = append(lines, "<b>Курс клиента:</b> "+escape(d.ExchangeRate)+" ₽")
		}
		lines = append(lines, "", notifyProfitLines(d))
	} else {
		lines = append(lines, "<b>Курс:</b> "+escape(d.ExchangeRate)+" ₽")
	}
	if d.CreatedBy != "" {
		lines = append(lines, "", "<i>"+escape(d.CreatedBy)+"</i>")
	}
	text := strings.Join(lines, "\n")
	if d.Cancelled() {
		text = "<s>" + text + "</s>"
	}
	return text
}

func networkSuffix(n string) string {
	n = strings.TrimSpace(n)
	if n == "" || strings.EqualFold(strings.ReplaceAll(n, "-", ""), "TRC20") {
		return ""
	}
	return ", " + escape(n)
}

func buybackNotifyText(b *storage.Buyback, deals []*storage.Deal) string {
	s := profit.SummarizeBuyback(b, deals)
	head := "🔁 <b>Откупка №%d</b> · %s"
	if b.Cancelled() {
		head = "❌ <b>ОТМЕНЕНА — откупка №%d</b> · %s"
	}
	lines := []string{
		fmt.Sprintf(head, b.ID, escape(b.BuyDate)),
		"<b>Поставщик:</b> " + escape(b.SupplierName),
		fmt.Sprintf("<b>Куплено:</b> %s USDT за %s ₽ по %s", util.RU(s.BoughtUSDT, 2), util.RUAmount(b.RubAmount), util.RU(b.Rate, 2)),
	}
	if len(deals) > 0 {
		lines = append(lines, "", "<b>Закрывает сделки:</b>")
		for _, d := range deals {
			lines = append(lines, fmt.Sprintf("• №%s %s — %s ₽", escape(d.DealNumber), escape(d.ClientName), util.RUAmount(d.FiatAmount)))
		}
		lines = append(lines, "", buybackBalanceText(b, s))
	} else {
		lines = append(lines, "", "<i>Откупка впрок — сделки ещё не привязаны</i>")
	}
	if b.CreatedBy != "" {
		lines = append(lines, "", "<i>"+escape(b.CreatedBy)+"</i>")
	}
	text := strings.Join(lines, "\n")
	if b.Cancelled() {
		text = "<s>" + text + "</s>"
	}
	return text
}

// buybackBalanceText — хватает ли USDT и сколько рублей осталось.
func buybackBalanceText(b *storage.Buyback, s profit.BuybackSummary) string {
	lines := []string{
		fmt.Sprintf("От клиентов: %s ₽ → отдать %s USDT", util.RUAmount(s.ClientRub), util.RU(s.ClientUSDT, 2)),
		fmt.Sprintf("Комиссия банка (%s%%): %s ₽", strings.TrimSuffix(util.RU(b.FeePercent, 2), "0"), util.RU(s.FeeRub, 2)),
	}
	if s.RubLeft >= 0 {
		lines = append(lines, fmt.Sprintf("Остаток на счёте: %s ₽", util.RU(s.RubLeft, 2)))
	} else {
		lines = append(lines, fmt.Sprintf("⚠️ Рублей от клиентов не хватило: %s ₽", util.RU(-s.RubLeft, 2)))
	}
	switch {
	case s.USDTDiff < -0.005:
		lines = append(lines, fmt.Sprintf("⚠️ Не хватает %s USDT — доплата из прибыли", util.RU(-s.USDTDiff, 2)))
	case s.USDTDiff > 0.005:
		lines = append(lines, fmt.Sprintf("✅ Излишек %s USDT", util.RU(s.USDTDiff, 2)))
	default:
		lines = append(lines, "✅ USDT хватает ровно")
	}
	lines = append(lines, fmt.Sprintf("<b>ЧКД по сделкам: %s USDT · чистая: %s USDT</b>", util.RU(s.CKD, 2), util.RU(s.Net, 2)))
	return strings.Join(lines, "\n")
}

// dealDiff — «было → стало» для правки заявки.
func dealDiff(old, new *storage.Deal) []string {
	type f struct{ label, a, b string }
	fields := []f{
		{"Клиент", old.ClientName, new.ClientName},
		{"№ заявки", old.DealNumber, new.DealNumber},
		{"Дата", old.DealDate, new.DealDate},
		{"Дата исполнения", old.ExecutionDate, new.ExecutionDate},
		{"Актив", old.VAType + " " + old.Network, new.VAType + " " + new.Network},
		{"Сумма ВА", util.Num(old.VAAmount), util.Num(new.VAAmount)},
		{"Сумма ₽", util.RUAmount(old.FiatAmount), util.RUAmount(new.FiatAmount)},
		{"Курс клиента", old.ExchangeRate, new.ExchangeRate},
		{"Кошелёк клиента", old.ClientWallet, new.ClientWallet},
		{"Комиссия", old.CommissionFiat + " / " + old.CommissionVA, new.CommissionFiat + " / " + new.CommissionVA},
		{"Транзакции", txSummary(old), txSummary(new)},
	}
	var out []string
	for _, x := range fields {
		if x.a != x.b {
			out = append(out, fmt.Sprintf("• %s: %s → <b>%s</b>", x.label, escape(x.a), escape(x.b)))
		}
	}
	return out
}

func txSummary(d *storage.Deal) string {
	var parts []string
	for _, t := range storage.ParseTxs(d.Transactions) {
		parts = append(parts, t.VAAmount+" ("+t.TxHash+")")
	}
	return strings.Join(parts, " + ")
}

// ─── Отправка ─────────────────────────────────────────────────────────────────

func (a *App) bgCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

func (a *App) sendNotify(ctx context.Context, chatID int64, text string, replyTo int) (*models.Message, error) {
	p := &tg.SendMessageParams{ChatID: chatID, Text: text, ParseMode: models.ParseModeHTML}
	if replyTo != 0 {
		p.ReplyParameters = &models.ReplyParameters{MessageID: replyTo, AllowSendingWithoutReply: true}
	}
	return a.b.SendMessage(ctx, p)
}

// notifyNew — новое уведомление во все чаты руководителей.
func (a *App) notifyNew(entity string, id int64, text string) {
	ctx, cancel := a.bgCtx()
	defer cancel()
	chats, err := a.st.NotifyChats(ctx)
	if err != nil {
		slog.Error("notify chats", "err", err)
		return
	}
	for _, ch := range chats {
		m, err := a.sendNotify(ctx, ch.ChatID, text, 0)
		if err != nil {
			slog.Error("уведомление не отправлено", "chat", ch.ChatID, "err", err)
			continue
		}
		_ = a.st.SaveNotification(ctx, entity, id, ch.ChatID, m.ID)
	}
}

// notifyChange обновляет исходное уведомление (чтобы оно отражало текущее
// состояние) и, если reply не пустой, отвечает на него — правку сообщения в
// Telegram никто не замечает, а ответ приходит уведомлением.
func (a *App) notifyChange(entity string, id int64, text, reply string) {
	ctx, cancel := a.bgCtx()
	defer cancel()
	sent, err := a.st.Notifications(ctx, entity, id)
	if err != nil {
		slog.Error("notifications", "err", err)
		return
	}
	done := map[int64]bool{}
	for _, n := range sent {
		done[n.ChatID] = true
		if _, err := a.b.EditMessageText(ctx, &tg.EditMessageTextParams{
			ChatID: n.ChatID, MessageID: n.MessageID, Text: text, ParseMode: models.ParseModeHTML,
		}); err != nil && !strings.Contains(err.Error(), "not modified") {
			slog.Warn("не удалось обновить уведомление", "chat", n.ChatID, "err", err)
		}
		if reply != "" {
			if _, err := a.sendNotify(ctx, n.ChatID, reply, n.MessageID); err != nil {
				slog.Error("ответ на уведомление не отправлен", "chat", n.ChatID, "err", err)
			}
		}
	}
	// Чаты, подключённые позже: шлём текущее состояние как новое уведомление
	chats, _ := a.st.NotifyChats(ctx)
	for _, ch := range chats {
		if done[ch.ChatID] || reply == "" {
			continue
		}
		if m, err := a.sendNotify(ctx, ch.ChatID, text+"\n\n"+reply, 0); err == nil {
			_ = a.st.SaveNotification(ctx, entity, id, ch.ChatID, m.ID)
		}
	}
}

func (a *App) refreshDealNotify(dealID int64, reply string) {
	ctx, cancel := a.bgCtx()
	defer cancel()
	d, err := a.st.Deal(ctx, dealID)
	if err != nil || d == nil {
		return
	}
	a.notifyChange("deal", dealID, dealNotifyText(d), reply)
}

func (a *App) refreshBuybackNotify(id int64, reply string) {
	ctx, cancel := a.bgCtx()
	defer cancel()
	b, err := a.st.Buyback(ctx, id)
	if err != nil || b == nil {
		return
	}
	deals, _ := a.st.DealsOfBuyback(ctx, id)
	a.notifyChange("buyback", id, buybackNotifyText(b, deals), reply)
}

// ─── Чат руководителей: команды в группе ──────────────────────────────────────

func (a *App) onGroupMessage(c *Ctx, isAdmin bool) {
	text := strings.TrimSpace(c.msg.Text)
	if !strings.HasPrefix(text, "/") {
		return
	}
	fields := strings.Fields(text)
	cmd := strings.SplitN(fields[0], "@", 2)[0]
	bound, _ := a.st.IsNotifyChat(c, c.chatID)
	switch cmd {
	case "/bind":
		if !isAdmin {
			return
		}
		title := ""
		if c.msg.Chat.Title != "" {
			title = c.msg.Chat.Title
		}
		if err := a.st.AddNotifyChat(c, c.chatID, title, c.userName); err != nil {
			c.fail(err)
			return
		}
		c.Send("✅ Чат подключён к уведомлениям о сделках.\n\n"+
			"Сюда будут приходить новые сделки, откупки, правки и отмены.\n"+
			"Отчёт за день: /today, за дату: /report 03.11.2026\nОтключить: /unbind", nil)
	case "/unbind":
		if !isAdmin {
			return
		}
		_ = a.st.RemoveNotifyChat(c, c.chatID)
		c.Send("🔕 Уведомления в этот чат отключены.", nil)
	case "/today", "/report":
		if !bound && !isAdmin {
			return
		}
		day := time.Now()
		if len(fields) > 1 {
			t, err := time.Parse("02.01.2006", fields[1])
			if err != nil {
				c.SendPlain("Формат: /report 03.11.2026", nil)
				return
			}
			day = t
		}
		a.sendDailyReport(c, day)
	}
}

// ─── Меню «🔔 Уведомления» ────────────────────────────────────────────────────

func (a *App) notifyCallback(c *Ctx, rest string) {
	if id, ok := strings.CutPrefix(rest, "del:"); ok {
		chatID, _ := parseID(id)
		_ = a.st.RemoveNotifyChat(c, chatID)
		c.Answer("Чат отключён", false)
	}
	chats, err := a.st.NotifyChats(c)
	if err != nil {
		c.fail(err)
		return
	}
	lines := []string{
		"🔔 <b>Уведомления руководителям</b>\n",
		"Как подключить чат:",
		"1. Добавьте бота в группу с руководителями.",
		"2. Напишите в группе <code>/bind</code> (с аккаунта из ADMIN_IDS).",
		"",
		"В чат приходят: 🆕 новые сделки, 🔁 откупки, ✏️ правки (ответом «было → стало»), ❌ отмены. " +
			"Исходное сообщение тоже обновляется, чтобы показывать актуальное состояние.",
		"В группе доступны <code>/today</code> и <code>/report ДД.ММ.ГГГГ</code> — отчёт о прибыли.",
		"",
	}
	var rows []kbRow
	if len(chats) == 0 {
		lines = append(lines, "<i>Пока ни одного чата.</i>")
	} else {
		lines = append(lines, "<b>Подключены (нажмите, чтобы отключить):</b>")
		for _, ch := range chats {
			title := ch.Title
			if title == "" {
				title = fmt.Sprint(ch.ChatID)
			}
			rows = append(rows, row(btn("🔕 "+title, fmt.Sprintf("notify:del:%d", ch.ChatID))))
		}
	}
	rows = append(rows, row(btn("🏠 Главное меню", "menu:main")))
	c.Edit(strings.Join(lines, "\n"), ikb(rows...))
}
