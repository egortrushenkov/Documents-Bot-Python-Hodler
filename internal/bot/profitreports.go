package bot

import (
	"fmt"
	"os"
	"time"

	"hodler-doc-bot/internal/profit"
	"hodler-doc-bot/internal/report"
	"hodler-doc-bot/internal/storage"
)

const stProfitDate = "profit:date"

func (a *App) showProfitMenu(c *Ctx) {
	c.Reply("📈 <b>Прибыль</b>\n\nОтчёт для руководства за день — в формате «📊 Классика за ДД.ММ.ГГГГ»: "+
		"курс откупки / курс клиента, объём, ЧКД, налог, рефералы, чистая прибыль.\n\n"+
		"XLSX за месяц: все транзакции, прибыль по сделкам, откупки, поставщики, клиенты.", ikb(
		row(btn("📊 Сегодня", "profit:today"), btn("📊 Вчера", "profit:yesterday")),
		row(btn("📅 Другая дата", "profit:date")),
		row(btn("📥 XLSX за месяц", "profit:months")),
		row(btn("💰 Остатки", "tr:menu"), btn("📦 Поставщики и клиенты", "tr:stats")),
		row(btn("🏠 Главное меню", "menu:main")),
	))
}

func (a *App) profitCallback(c *Ctx, rest string) {
	switch {
	case rest == "menu":
		c.s.Clear()
		a.showProfitMenu(c)
	case rest == "today":
		c.Answer("", false)
		a.sendDailyReport(c, time.Now())
	case rest == "yesterday":
		c.Answer("", false)
		a.sendDailyReport(c, time.Now().AddDate(0, 0, -1))
	case rest == "date":
		c.s.Clear()
		c.s.State = stProfitDate
		c.EditPlain("Введите дату (ДД.ММ.ГГГГ):", nil)
	case rest == "months":
		months, err := a.st.DealMonths(c)
		if err != nil {
			c.fail(err)
			return
		}
		if len(months) == 0 {
			c.Answer("Сделок пока нет", true)
			return
		}
		var rows []kbRow
		for _, m := range months {
			rows = append(rows, row(btn("📅 "+report.MonthLabel(m), "profit:xlsx:"+m)))
		}
		rows = append(rows, row(btn("◀️ Назад", "profit:menu")))
		c.EditPlain("Выберите месяц:", ikb(rows...))
	case len(rest) > 5 && rest[:5] == "xlsx:":
		c.Answer("⏳ Готовлю таблицу...", false)
		a.sendProfitXLSX(c, rest[5:])
	}
}

func (a *App) profitDateInput(c *Ctx, text string) {
	day, err := time.Parse("02.01.2006", text)
	if err != nil {
		c.SendPlain("❌ Формат: ДД.ММ.ГГГГ — например, 03.11.2026", nil)
		return
	}
	c.s.Clear()
	a.sendDailyReport(c, day)
}

// sendDailyReport — текст отчёта за день (без HTML: удобно пересылать и копировать).
func (a *App) sendDailyReport(c *Ctx, day time.Time) {
	deals, err := a.st.DealsForDate(c, day.Format("2006-01-02"))
	if err != nil {
		c.fail(err)
		return
	}
	s, _ := a.st.Settings(c)
	title := s["report_title"]
	if title == "" {
		title = "Классика"
	}
	c.SendPlain(profit.DailyReport(title, day, deals), nil)
}

func (a *App) sendProfitXLSX(c *Ctx, month string) {
	deals, err := a.st.DealsForMonth(c, month)
	if err != nil {
		c.fail(err)
		return
	}
	buybacks, err := a.st.BuybacksForMonth(c, month)
	if err != nil {
		c.fail(err)
		return
	}
	linked := map[int64][]*storage.Deal{}
	for _, b := range buybacks {
		linked[b.ID], _ = a.st.DealsOfBuyback(c, b.ID)
	}
	if err := os.MkdirAll(a.cfg.OutputDir, 0o755); err != nil {
		c.fail(err)
		return
	}
	f, err := os.CreateTemp(a.cfg.OutputDir, "profit_*.xlsx")
	if err != nil {
		c.fail(err)
		return
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)
	if err := report.BuildProfitReport(path, month, deals, buybacks, linked); err != nil {
		c.fail(err)
		return
	}
	name := fmt.Sprintf("Прибыль_%s.xlsx", month)
	if _, err := c.SendFile(path, name, "📥 Транзакции, прибыль, откупки, поставщики и клиенты за "+report.MonthLabel(month), false); err != nil {
		c.fail(err)
	}
}
