package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"hodler-doc-bot/internal/docgen"
	"hodler-doc-bot/internal/profit"
	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/util"
)

var actTypeNames = map[string]string{"sell": "Продажа клиенту", "buy": "Покупка у клиента", "invoice": "Счёт-заявка"}

type column struct {
	title string
	width float64
	kind  string // text | date | money | rate | int
}

// table пишет лист: заголовок, шапку и строки; totals — строка «Итого» (может быть nil).
func table(f *excelize.File, st *styles, sheet, title string, cols []column, rows [][]any, totals []any) {
	last := len(cols)
	_ = f.SetCellValue(sheet, "A1", title)
	_ = f.SetCellStyle(sheet, "A1", "A1", st.bold)
	for i, c := range cols {
		_ = f.SetCellValue(sheet, cellName(i+1, 2), c.title)
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheet, col, col, c.width)
	}
	_ = f.SetCellStyle(sheet, "A2", cellName(last, 2), st.header)
	write := func(r int, vals []any, bold bool) {
		for i, v := range vals {
			if v == nil {
				continue
			}
			if s, ok := v.(string); ok && s == "" {
				continue
			}
			_ = f.SetCellValue(sheet, cellName(i+1, r), v)
		}
		for i, c := range cols {
			style := st.cell
			switch {
			case bold && (c.kind == "money" || c.kind == "rate"):
				style = st.totalMoney
			case bold:
				style = st.totalLabel
			case c.kind == "date":
				if i < len(vals) {
					if _, ok := vals[i].(time.Time); ok {
						style = st.date
					}
				}
			case c.kind == "money" || c.kind == "rate":
				style = st.money
			}
			_ = f.SetCellStyle(sheet, cellName(i+1, r), cellName(i+1, r), style)
		}
	}
	r := 3
	for _, vals := range rows {
		write(r, vals, false)
		r++
	}
	if totals != nil {
		write(r, totals, true)
	}
	_ = f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 2, TopLeftCell: "A3", ActivePane: "bottomLeft"})
}

func dealDate(d *storage.Deal) any {
	if t, err := time.Parse("2006-01-02", d.DealDateISO); err == nil {
		return t
	}
	return d.DealDate
}

// BuildProfitReport — XLSX за месяц: «Транзакции», «Прибыль», «Откупки».
func BuildProfitReport(path, month string, deals []*storage.Deal, buybacks []*storage.Buyback,
	buybackDeals map[int64][]*storage.Deal) error {
	f := excelize.NewFile()
	defer f.Close()
	st, err := makeStyles(f)
	if err != nil {
		return err
	}
	label := MonthLabel(month)

	// ─── Транзакции ───
	_ = f.SetSheetName("Sheet1", "Транзакции")
	txCols := []column{
		{"Дата", 11, "date"}, {"№ заявки", 9, "text"}, {"Тип", 16, "text"}, {"Клиент", 30, "text"},
		{"Поставщик (откупка)", 26, "text"}, {"Актив", 8, "text"}, {"Сеть", 9, "text"},
		{"Сумма ВА", 14, "money"}, {"Сумма ₽", 16, "money"}, {"Курс клиента", 10, "rate"},
		{"Курс откупки", 10, "rate"}, {"Хэш транзакции", 40, "text"}, {"Кошелёк клиента", 36, "text"},
		{"Кошелёк оператора", 36, "text"}, {"Часть", 7, "text"},
	}
	var txRows [][]any
	var txVA, txRub float64
	for _, d := range deals {
		if d.ActType == "invoice" {
			continue // счёт-заявка — не движение средств
		}
		txs := docgen.ComputeTransactions(storage.ParseTxs(d.Transactions), util.Num(d.VAAmount), util.Num(d.FiatAmount), "")
		for i, t := range txs {
			part := ""
			if len(txs) > 1 {
				part = fmt.Sprintf("%d/%d", i+1, len(txs))
			}
			var buy any = ""
			if d.BuyRate > 0 {
				buy = d.BuyRate
			}
			va := util.ToFloat(t.VAAmount)
			txVA += va
			txRub += t.FiatAmount
			txRows = append(txRows, []any{dealDate(d), d.DealNumber, actTypeNames[d.ActType], d.ClientName,
				d.SupplierName, d.VAType, d.Network, va, t.FiatAmount, util.ToFloat(d.ExchangeRate), buy,
				t.TxHash, d.ClientWallet, d.OperatorWallet, part})
		}
	}
	table(f, st, "Транзакции", "Транзакции за "+label, txCols, txRows,
		[]any{"Итого", "", "", "", "", "", "", util.Round2(txVA), util.Round2(txRub)})

	// ─── Прибыль ───
	if _, err := f.NewSheet("Прибыль"); err != nil {
		return err
	}
	pCols := []column{
		{"Дата", 11, "date"}, {"№ заявки", 9, "text"}, {"Поставщик", 26, "text"}, {"Клиент", 30, "text"},
		{"Курс откупки", 10, "rate"}, {"Курс клиента", 10, "rate"}, {"Объём USDT", 14, "money"},
		{"Сумма ₽", 16, "money"}, {"Сеть", 9, "text"}, {"ЧКД USDT", 12, "money"}, {"Налог USDT", 11, "money"},
		{"Реферал", 22, "text"}, {"% реф.", 7, "rate"}, {"Реферал USDT", 12, "money"},
		{"Чистая прибыль USDT", 14, "money"}, {"Статус", 18, "text"},
	}
	var pRows [][]any
	var results []profit.Result
	for _, d := range deals {
		if !profit.Profitable(d) {
			continue
		}
		r := profit.Compute(d)
		results = append(results, r)
		status := "Откупка"
		var buy, ckd, tax, ref, net any = r.BuyRate, util.Round2(r.CKD), util.Round2(r.Tax), util.Round2(r.Referral), util.Round2(r.Net)
		if !r.HasBuyback {
			status = "Ожидает откупки"
			buy, ckd, tax, ref, net = "", "", "", "", ""
		}
		var refPct any = ""
		if d.ReferrerName != "" {
			refPct = d.ReferralPercent
		}
		pRows = append(pRows, []any{dealDate(d), d.DealNumber, d.SupplierName, d.ClientName, buy, r.ClientRate,
			util.Round2(r.Volume), r.Rub, d.Network, ckd, tax, d.ReferrerName, refPct, ref, net, status})
	}
	t := profit.Sum(results)
	table(f, st, "Прибыль", "Прибыль за "+label+" (по сделкам с откупкой)", pCols, pRows,
		[]any{"Итого", fmt.Sprintf("%d сд.", t.Deals), "", "", "", "", util.Round2(t.Volume), t.Rub, "",
			util.Round2(t.CKD), util.Round2(t.Tax), "", "", util.Round2(t.Ref), util.Round2(t.Net)})

	// ─── Откупки ───
	if _, err := f.NewSheet("Откупки"); err != nil {
		return err
	}
	bCols := []column{
		{"Дата", 11, "date"}, {"№", 6, "int"}, {"Поставщик", 26, "text"}, {"Курс", 10, "rate"},
		{"Переведено ₽", 16, "money"}, {"Куплено USDT", 14, "money"}, {"Комиссия банка ₽", 13, "money"},
		{"Сделки клиентов", 24, "text"}, {"Получено от клиентов ₽", 16, "money"},
		{"Отдать клиентам USDT", 14, "money"}, {"Разница USDT", 12, "money"}, {"Остаток ₽", 13, "money"},
		{"ЧКД USDT", 12, "money"},
	}
	var bRows [][]any
	for _, b := range buybacks {
		linked := buybackDeals[b.ID]
		s := profit.SummarizeBuyback(b, linked)
		var nums []string
		for _, d := range linked {
			nums = append(nums, "№"+d.DealNumber)
		}
		var date any = b.BuyDate
		if tm, err := time.Parse("2006-01-02", b.BuyDateISO); err == nil {
			date = tm
		}
		bRows = append(bRows, []any{date, b.ID, b.SupplierName, b.Rate, b.RubAmount, util.Round2(s.BoughtUSDT),
			util.Round2(s.FeeRub), strings.Join(nums, ", "), s.ClientRub, util.Round2(s.ClientUSDT),
			util.Round2(s.USDTDiff), util.Round2(s.RubLeft), util.Round2(s.CKD)})
	}
	table(f, st, "Откупки", "Откупки за "+label, bCols, bRows, nil)

	f.SetActiveSheet(0)
	return f.SaveAs(path)
}
