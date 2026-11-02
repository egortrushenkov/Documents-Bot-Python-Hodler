// Package report — выгрузка отчёта для Финнадзора КР (XLSX).
//
// Листы повторяют Приложения 4/о («Продажа») и 5/о («Покупка») ежемесячного
// отчёта операторов обмена ВА. Одна строка = одна транзакция (части сделки
// «тест + остаток» идут отдельными строками с рублями по курсу сделки).
package report

import (
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"hodler-doc-bot/internal/docgen"
	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/util"
)

var monthsNom = [...]string{"", "Январь", "Февраль", "Март", "Апрель", "Май", "Июнь",
	"Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь"}

var headers = []string{
	"№",
	"Дата сделки",
	"Наименование и ИНН (для юридического лица) или Ф.И.О. и ID (для физического лица)",
	"Резидент/нерезидент Кыргызской Республики",
	"Наименование виртуального актива",
	"Курс обмена ВА на денежные средства",
	"Объём в денежных средствах (в иностранной валюте)",
	"Объём в денежных средствах (в сомах)",
	"Метод расчета (наличный/безналичный)",
	"Адрес электронного кошелька оператора",
	"Адрес электронного кошелька клиента",
	"Идентификация клиента KYC (да/нет)",
}

var colWidths = []float64{5, 12, 38, 16, 14, 12, 18, 18, 15, 36, 36, 14}

// MonthLabel: '2026-06' → 'Июнь 2026'.
func MonthLabel(month string) string {
	parts := strings.Split(month, "-")
	if len(parts) == 2 {
		if m, err := strconv.Atoi(parts[1]); err == nil && m >= 1 && m <= 12 {
			return monthsNom[m] + " " + parts[0]
		}
	}
	return month
}

type styles struct {
	cell, date, money, header, bold, totalLabel, totalMoney int
}

func makeStyles(f *excelize.File) (*styles, error) {
	font := &excelize.Font{Family: "Times New Roman", Size: 10}
	bold := &excelize.Font{Family: "Times New Roman", Size: 10, Bold: true}
	border := []excelize.Border{
		{Type: "left", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1},
		{Type: "top", Color: "000000", Style: 1}, {Type: "bottom", Color: "000000", Style: 1},
	}
	dateFmt := "DD.MM.YYYY"
	moneyFmt := "#,##0.00"
	defs := []*excelize.Style{
		{Font: font, Border: border},
		{Font: font, Border: border, CustomNumFmt: &dateFmt},
		{Font: font, Border: border, CustomNumFmt: &moneyFmt},
		{Font: bold, Border: border, Alignment: &excelize.Alignment{WrapText: true, Vertical: "center", Horizontal: "center"}},
		{Font: bold},
		{Font: bold, Border: border},
		{Font: bold, Border: border, CustomNumFmt: &moneyFmt},
	}
	ids := make([]int, len(defs))
	for i, d := range defs {
		id, err := f.NewStyle(d)
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return &styles{ids[0], ids[1], ids[2], ids[3], ids[4], ids[5], ids[6]}, nil
}

func cellName(col, row int) string {
	n, _ := excelize.CoordinatesToCellName(col, row)
	return n
}

// dealRows — одна сделка → строки-транзакции листа (без колонки №).
func dealRows(d *storage.Deal, kgsRate float64) [][]any {
	txs := docgen.ComputeTransactions(storage.ParseTxs(d.Transactions),
		util.Num(d.VAAmount), util.Num(d.FiatAmount), "")
	var date any = d.DealDate
	if t, err := time.Parse("2006-01-02", d.DealDateISO); err == nil {
		date = t
	}
	client := strings.TrimSpace(d.ClientName + " " + d.ClientINN)
	resident := d.ClientResident
	if resident == "" {
		resident = "Резидент"
	}
	rate := util.ToFloat(d.ExchangeRate)
	var rows [][]any
	for _, t := range txs {
		var som any = ""
		if kgsRate != 0 {
			som = util.Round2(t.FiatAmount * kgsRate)
		}
		rows = append(rows, []any{
			date, client, resident, d.VAType, rate, t.FiatAmount, som,
			"Безналичный", d.OperatorWallet, d.ClientWallet, "Да",
		})
	}
	return rows
}

func fillSheet(f *excelize.File, st *styles, sheet, title, annex string, deals []*storage.Deal, kgsRate float64) error {
	last := len(headers)
	_ = f.SetCellValue(sheet, cellName(last-1, 1), annex)
	_ = f.SetCellStyle(sheet, cellName(last-1, 1), cellName(last-1, 1), st.bold)
	_ = f.SetCellValue(sheet, "A2", title)
	_ = f.SetCellStyle(sheet, "A2", "A2", st.bold)
	if err := f.MergeCell(sheet, "A2", cellName(last, 2)); err != nil {
		return err
	}
	for i, h := range headers {
		_ = f.SetCellValue(sheet, cellName(i+1, 3), h)
	}
	_ = f.SetCellStyle(sheet, "A3", cellName(last, 3), st.header)

	row, n := 4, 0
	totalFiat, totalSom := 0.0, 0.0
	for _, d := range deals {
		for _, vals := range dealRows(d, kgsRate) {
			n++
			_ = f.SetCellValue(sheet, cellName(1, row), n)
			for i, v := range vals {
				if s, ok := v.(string); ok && s == "" {
					continue
				}
				_ = f.SetCellValue(sheet, cellName(i+2, row), v)
			}
			_ = f.SetCellStyle(sheet, cellName(1, row), cellName(last, row), st.cell)
			if _, isDate := vals[0].(time.Time); isDate {
				_ = f.SetCellStyle(sheet, cellName(2, row), cellName(2, row), st.date)
			}
			_ = f.SetCellStyle(sheet, cellName(7, row), cellName(8, row), st.money)
			totalFiat += vals[5].(float64)
			if som, ok := vals[6].(float64); ok {
				totalSom += som
			}
			row++
		}
	}

	// Итого
	_ = f.SetCellStyle(sheet, cellName(1, row), cellName(last, row), st.cell)
	_ = f.SetCellValue(sheet, cellName(1, row), "Итого")
	_ = f.SetCellStyle(sheet, cellName(1, row), cellName(1, row), st.totalLabel)
	_ = f.SetCellValue(sheet, cellName(7, row), util.Round2(totalFiat))
	_ = f.SetCellValue(sheet, cellName(8, row), util.Round2(totalSom))
	_ = f.SetCellStyle(sheet, cellName(7, row), cellName(8, row), st.totalMoney)

	for i, w := range colWidths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheet, col, col, w)
	}
	return f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 3, TopLeftCell: "A4", ActivePane: "bottomLeft"})
}

// BuildFNReport пишет XLSX: kgsRate — курс RUB→KGS (0 = колонку «в сомах» не заполнять).
func BuildFNReport(path string, deals []*storage.Deal, kgsRate float64) error {
	var sell, buy []*storage.Deal
	for _, d := range deals {
		switch d.ActType {
		case "sell":
			sell = append(sell, d)
		case "buy":
			buy = append(buy, d)
		}
	}
	f := excelize.NewFile()
	defer f.Close()
	st, err := makeStyles(f)
	if err != nil {
		return err
	}
	if err := f.SetSheetName("Sheet1", "Продажа"); err != nil {
		return err
	}
	if err := fillSheet(f, st, "Продажа", "Отчет по продаже виртуальных активов клиентам",
		"Приложение 4/о", sell, kgsRate); err != nil {
		return err
	}
	if _, err := f.NewSheet("Покупка"); err != nil {
		return err
	}
	if err := fillSheet(f, st, "Покупка", "Отчет по покупке виртуальных активов у клиентов",
		"Приложение 5/о", buy, kgsRate); err != nil {
		return err
	}
	f.SetActiveSheet(0)
	return f.SaveAs(path)
}
