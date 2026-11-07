// Package profit — расчёт чистого комиссионного дохода (ЧКД) по сделкам
// продажи USDT клиентам и ежедневный отчёт для руководства.
//
//	Объём  = ₽ / курс клиента                       (USDT, отданные клиенту)
//	ЧКД    = ₽ / курс откупки − Объём − комиссия% × Объём
//	Налог  = налог% × ЧКД
//	Реферал = реферал% × (ЧКД − Налог)
//	Чистая = ЧКД − Налог − Реферал
//
// Сумма самой откупки на ЧКД не влияет — она определяет только, сколько
// USDT куплено и сколько рублей осталось на счёте (см. BuybackSummary).
package profit

import (
	"fmt"
	"math"
	"strings"
	"time"

	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/util"
)

// Result — расчёт по одной сделке. Значения не округлены.
type Result struct {
	Deal       *storage.Deal
	Rub        float64
	ClientRate float64
	BuyRate    float64 // 0 — откупки ещё нет
	Volume     float64 // USDT клиенту
	CKD        float64
	Tax        float64
	Referral   float64
	Net        float64
	HasBuyback bool
}

// Profitable — сделка, по которой считается прибыль (продажа USDT клиенту).
func Profitable(d *storage.Deal) bool { return d.ActType == "sell" && !d.Cancelled() }

// Compute считает прибыль по сделке (для сделок без откупки — только объём).
func Compute(d *storage.Deal) Result {
	r := Result{Deal: d, Rub: d.FiatAmount, ClientRate: util.ToFloat(d.ExchangeRate), BuyRate: d.BuyRate}
	if r.ClientRate > 0 && r.Rub > 0 {
		r.Volume = r.Rub / r.ClientRate
	} else {
		r.Volume = d.VAAmount
	}
	if r.BuyRate <= 0 || r.Rub <= 0 {
		return r
	}
	r.HasBuyback = true
	r.CKD = r.Rub/r.BuyRate - r.Volume - d.FeePercent/100*r.Volume
	r.Tax = d.TaxPercent / 100 * r.CKD
	r.Referral = d.ReferralPercent / 100 * (r.CKD - r.Tax)
	r.Net = r.CKD - r.Tax - r.Referral
	return r
}

func r2(f float64) float64 { return math.Round(f*100) / 100 }

// ShortName убирает организационно-правовую форму и кавычки:
// «ОсОО «Алтынкопрю»» → «Алтынкопрю».
func ShortName(name string) string {
	s := strings.TrimSpace(name)
	for _, form := range []string{"ОсОО", "ООО", "ЗАО", "ОАО", "ПАО", "АО", "ИП", "LLC", "Ltd", "LTD", "FZE", "FZCO", "Inc"} {
		if strings.HasPrefix(s, form+" ") {
			s = strings.TrimSpace(strings.TrimPrefix(s, form+" "))
			break
		}
	}
	s = strings.NewReplacer("«", "", "»", "", "\"", "", "“", "", "”", "").Replace(s)
	if s == "" {
		return name
	}
	return s
}

var keycaps = []string{"1️⃣", "2️⃣", "3️⃣", "4️⃣", "5️⃣", "6️⃣", "7️⃣", "8️⃣", "9️⃣", "🔟"}

func num(i int) string {
	if i < len(keycaps) {
		return keycaps[i]
	}
	return fmt.Sprintf("%d.", i+1)
}

func pct(p float64) string { return strings.TrimSuffix(strings.TrimSuffix(util.RU(p, 2), "0"), ",0") }

// Header — «ООО «БРАЙТУМ» / Иксхаб» (поставщик / клиент).
func Header(r Result) string {
	if r.Deal.SupplierName != "" {
		return r.Deal.SupplierName + " / " + r.Deal.ClientName
	}
	return r.Deal.ClientName
}

// DealBlock — блок одной сделки в отчёте (без номера).
func DealBlock(r Result) []string {
	d := r.Deal
	volume := fmt.Sprintf("Объём: %s USDT (%s ₽)", util.RU(r.Volume, 2), util.RUAmount(r.Rub))
	if n := strings.TrimSpace(d.Network); n != "" && !strings.EqualFold(strings.ReplaceAll(n, "-", ""), "TRC20") {
		volume += ", " + n
	}
	if !r.HasBuyback {
		return []string{
			fmt.Sprintf("Курс: — / %s ₽", util.RU(r.ClientRate, 2)),
			volume,
			"⏳ Ожидает откупки — прибыль не посчитана",
		}
	}
	lines := []string{
		fmt.Sprintf("Курс: %s / %s ₽", util.RU(r.BuyRate, 2), util.RU(r.ClientRate, 2)),
		volume,
		fmt.Sprintf("ЧКД: %s USDT", util.RU(r.CKD, 2)),
		fmt.Sprintf("Налог (%s%%): %s USDT", pct(d.TaxPercent), util.RU(r.Tax, 2)),
	}
	if d.ReferrerName != "" && d.ReferralPercent > 0 {
		lines = append(lines, fmt.Sprintf("Реферал: %s USDT (%s %s%%)", util.RU(r.Referral, 2), d.ReferrerName, pct(d.ReferralPercent)))
	}
	return append(lines, fmt.Sprintf("Чистая прибыль: %s USDT", util.RU(r.Net, 2)))
}

// Totals — итоги (сумма округлённых значений по сделкам, как в ручном отчёте).
type Totals struct {
	Deals, Pending                  int
	Volume, Rub, CKD, Tax, Ref, Net float64
	PendingVolume, PendingRub       float64
	TaxPercent                      float64
	RefParts                        map[string][]refPart
	RefOrder                        []string
}

type refPart struct {
	Amount float64
	Client string
}

func Sum(results []Result) Totals {
	t := Totals{RefParts: map[string][]refPart{}}
	for _, r := range results {
		if !r.HasBuyback {
			t.Pending++
			t.PendingVolume += r2(r.Volume)
			t.PendingRub += r.Rub
			continue
		}
		t.Deals++
		t.Volume += r2(r.Volume)
		t.Rub += r.Rub
		t.CKD += r2(r.CKD)
		t.Tax += r2(r.Tax)
		t.Ref += r2(r.Referral)
		t.Net += r2(r.Net)
		t.TaxPercent = r.Deal.TaxPercent
		if name := r.Deal.ReferrerName; name != "" && r.Deal.ReferralPercent > 0 {
			if _, ok := t.RefParts[name]; !ok {
				t.RefOrder = append(t.RefOrder, name)
			}
			t.RefParts[name] = append(t.RefParts[name], refPart{r2(r.Referral), ShortName(r.Deal.ClientName)})
		}
	}
	return t
}

// DailyReport — текст отчёта за день в формате «📊 Классика за 03.11.2026».
func DailyReport(title string, day time.Time, deals []*storage.Deal) string {
	var results []Result
	for _, d := range deals {
		if Profitable(d) {
			results = append(results, Compute(d))
		}
	}
	date := day.Format("02.01.2006")
	if len(results) == 0 {
		return fmt.Sprintf("📊 %s за %s\n\nСделок нет.", title, date)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "📊 %s за %s\n", title, date)
	for i, r := range results {
		fmt.Fprintf(&b, "\n%s %s\n%s\n", num(i), Header(r), strings.Join(DealBlock(r), "\n"))
	}
	t := Sum(results)
	fmt.Fprintf(&b, "\nИтого за %s:\n", day.Format("02.01"))
	fmt.Fprintf(&b, "Сделок: %d\n", t.Deals)
	fmt.Fprintf(&b, "Объём: %s USDT (%s ₽)\n", util.RU(t.Volume, 2), util.RUAmount(t.Rub))
	fmt.Fprintf(&b, "ЧКД: %s USDT\n", util.RU(t.CKD, 2))
	fmt.Fprintf(&b, "Налог (%s%%): %s USDT\n", pct(t.TaxPercent), util.RU(t.Tax, 2))
	fmt.Fprintf(&b, "Рефералы: %s USDT\n", util.RU(t.Ref, 2))
	fmt.Fprintf(&b, "Чистая прибыль: %s USDT\n", util.RU(t.Net, 2))
	if t.Pending > 0 {
		fmt.Fprintf(&b, "\n⏳ Ожидают откупки: %d (%s USDT, %s ₽) — в итоги не вошли\n",
			t.Pending, util.RU(t.PendingVolume, 2), util.RUAmount(t.PendingRub))
	}
	if len(t.RefOrder) > 0 {
		b.WriteString("\n👥 Рефералы:\n")
		for _, name := range t.RefOrder {
			parts := t.RefParts[name]
			var items []string
			sum := 0.0
			for _, p := range parts {
				items = append(items, fmt.Sprintf("%s (%s)", util.RU(p.Amount, 2), p.Client))
				sum += p.Amount
			}
			if len(items) == 1 {
				fmt.Fprintf(&b, "%s: %s USDT\n", name, items[0])
			} else {
				fmt.Fprintf(&b, "%s: %s = %s USDT\n", name, strings.Join(items, " + "), util.RU(sum, 2))
			}
		}
		fmt.Fprintf(&b, "Итого рефералам: %s USDT\n", util.RU(t.Ref, 2))
	}
	return strings.TrimRight(b.String(), "\n")
}

// BuybackSummary — баланс откупки: хватает ли купленных USDT на клиентов
// и сколько рублей осталось после перевода и комиссии.
type BuybackSummary struct {
	ClientRub  float64 // получено от клиентов по связанным сделкам
	ClientUSDT float64 // надо отдать клиентам
	BoughtUSDT float64 // куплено
	FeeRub     float64 // комиссия банка
	RubLeft    float64 // остаток рублей = ClientRub − откупка − комиссия
	USDTDiff   float64 // куплено − надо отдать (минус — доплата из прибыли)
	CKD, Net   float64 // по связанным сделкам (сумма округлённых)
}

func SummarizeBuyback(b *storage.Buyback, deals []*storage.Deal) BuybackSummary {
	s := BuybackSummary{BoughtUSDT: b.USDT(), FeeRub: b.FeeRub()}
	for _, d := range deals {
		dd := *d
		dd.BuyRate, dd.SupplierName = b.Rate, b.SupplierName
		r := Compute(&dd)
		s.ClientRub += r.Rub
		s.ClientUSDT += r.Volume
		s.CKD += r2(r.CKD)
		s.Net += r2(r.Net)
	}
	s.RubLeft = s.ClientRub - b.RubAmount - s.FeeRub
	s.USDTDiff = s.BoughtUSDT - s.ClientUSDT
	return s
}

// SuggestRub — сколько рублей можно перевести поставщику, чтобы хватило на
// комиссию банка: точная сумма и округлённая вниз до 10 000.
func SuggestRub(clientRub, feePercent float64) (exact, rounded float64) {
	exact = math.Floor(clientRub / (1 + feePercent/100))
	rounded = math.Floor(exact/10000) * 10000
	return exact, rounded
}
