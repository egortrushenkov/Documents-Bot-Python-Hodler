package bot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hodler-doc-bot/internal/report"
	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/util"
)

func (a *App) showReportMonths(c *Ctx) {
	months, err := a.st.DealMonths(c)
	if err != nil {
		c.fail(err)
		return
	}
	if len(months) == 0 {
		c.Reply("📊 <b>Отчёт ФН</b>\n\nВ журнале пока нет сделок — "+
			"создайте первый акт, и он попадёт в отчёт автоматически.", backToMainKB())
		return
	}
	c.Reply("📊 <b>Отчёт ФН</b>\n\nВыберите месяц для выгрузки:", reportMonthsKB(months, report.MonthLabel))
}

func (a *App) reportCallback(c *Ctx, head, rest string) {
	switch head {
	case "report":
		if rest == "menu" {
			c.s.Clear()
			a.showReportMonths(c)
		}
	case "report_month":
		settings, err := a.st.Settings(c)
		if err != nil {
			c.fail(err)
			return
		}
		c.s.Clear()
		c.s.State = stReportRate
		c.s.Data["report_month"] = rest
		c.Edit("📅 Месяц: <b>"+escape(report.MonthLabel(rest))+"</b>\n\n"+
			"<b>Курс RUB → KGS</b> для колонки «Объём в сомах»\n"+
			"Введите курс (например: 1.2072):", reportRateKB(settings["kgs_rate"]))
	case "report_rate":
		if c.s.State != stReportRate {
			return
		}
		c.Answer("", false)
		switch rest {
		case "saved":
			settings, err := a.st.Settings(c)
			if err != nil {
				c.fail(err)
				return
			}
			a.generateReport(c, util.ToFloat(settings["kgs_rate"]))
		case "skip":
			a.generateReport(c, 0)
		}
	}
}

func (a *App) reportRateInput(c *Ctx, text string) {
	rate := util.ToFloat(text)
	if rate <= 0 {
		c.Send("❌ Введите число, например: 1.2072", nil)
		return
	}
	if err := a.st.SetSetting(c, "kgs_rate", util.PyFloat(rate)); err != nil { // запомним на следующий раз
		c.fail(err)
		return
	}
	a.generateReport(c, rate)
}

func (a *App) generateReport(c *Ctx, rate float64) {
	month := c.s.Data["report_month"]
	c.s.Clear()
	deals, err := a.st.DealsForMonth(c, month)
	if err != nil {
		c.fail(err)
		return
	}
	var reportable []*storage.Deal
	nSell := 0
	for _, d := range deals {
		if d.ActType == "sell" || d.ActType == "buy" {
			reportable = append(reportable, d)
			if d.ActType == "sell" {
				nSell++
			}
		}
	}
	label := report.MonthLabel(month)
	if len(reportable) == 0 {
		c.SendPlain(fmt.Sprintf("За %s нет актов продажи/покупки (счета-заявки в отчёт не входят).", label),
			backToMainKB())
		return
	}
	if err := os.MkdirAll(a.cfg.OutputDir, 0o755); err != nil {
		c.fail(err)
		return
	}
	f, err := os.CreateTemp(a.cfg.OutputDir, "fn_report_"+month+"_*.xlsx")
	if err != nil {
		c.fail(err)
		return
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)
	if err := report.BuildFNReport(path, reportable, rate); err != nil {
		c.fail(err)
		return
	}
	caption := fmt.Sprintf("📊 Отчёт ФН за %s\nПродажа: %d сделок · Покупка: %d сделок", label, nSell, len(reportable)-nSell)
	if rate != 0 {
		caption += "\nКурс RUB→KGS: " + util.PyFloat(rate)
	} else {
		caption += "\nБез пересчёта в сомы"
	}
	name := "ФН_" + strings.ReplaceAll(label, " ", "_") + ".xlsx"
	if _, err := c.SendFile(path, filepath.Base(name), caption, false); err != nil {
		c.fail(err)
	}
}
