package profit

import (
	"sort"

	"hodler-doc-bot/internal/storage"
)

// Treasury — остатки USDT и рублей на текущий момент.
//
// Движения считаются из журнала, поэтому правка/отмена сделки или откупки
// сразу отражается в остатках:
//
//	продажа клиенту:   +₽ от клиента, −USDT клиенту
//	покупка у клиента: −₽ клиенту,     +USDT
//	откупка:           −₽ поставщику, −комиссия банка, +USDT
//	ручные операции:   со своим знаком
type Treasury struct {
	FromISO     string // начало учёта ("" — с начала журнала)
	OpeningUSDT float64
	OpeningRub  float64

	SellCount           int
	SellUSDT, SellRub   float64 // отдано клиентам / получено от клиентов
	BuyCount            int
	BuyUSDT, BuyRub     float64 // получено от клиентов / отдано клиентам
	BBCount             int
	BBUSDT, BBRub, Fees float64 // куплено / переведено поставщикам / комиссии
	ManualUSDT          float64
	ManualRub           float64
	ManualCount         int

	USDT, Rub float64 // итоговые остатки

	OpenCount           int     // продажи без откупки (USDT выдан наперёд)
	OpenUSDT, OpenRub   float64 // сколько USDT выдано наперёд и сколько ₽ под это лежит
	OpenEstimatedBuyUSD float64 // сколько USDT вернёт откупка этих ₽ по последнему курсу
	LastBuyRate         float64

	CKD, Tax, Net float64 // по сделкам с откупкой
	RefAccrued    map[string]float64
	RefPaid       map[string]float64
	TaxPaid       float64
}

// RefOwed — сколько осталось выплатить каждому рефералу (по убыванию).
func (t Treasury) RefOwed() []RefDebt {
	names := map[string]bool{}
	for n := range t.RefAccrued {
		names[n] = true
	}
	for n := range t.RefPaid {
		names[n] = true
	}
	var out []RefDebt
	for n := range names {
		out = append(out, RefDebt{Name: n, Accrued: t.RefAccrued[n], Paid: t.RefPaid[n]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Owed() > out[j].Owed() })
	return out
}

type RefDebt struct {
	Name          string
	Accrued, Paid float64
}

func (r RefDebt) Owed() float64 { return r.Accrued - r.Paid }

// ComputeTreasury считает остатки. deals — активные сделки с начала учёта,
// buybacks — активные откупки с начала учёта, ops — ручные операции.
func ComputeTreasury(opening *storage.TreasuryOp, deals []*storage.Deal, buybacks []*storage.Buyback,
	ops []*storage.TreasuryOp) Treasury {
	t := Treasury{RefAccrued: map[string]float64{}, RefPaid: map[string]float64{}}
	if opening != nil {
		t.FromISO, t.OpeningUSDT, t.OpeningRub = opening.OpDate, opening.USDT, opening.Rub
	}
	lastDate := ""
	for _, b := range buybacks {
		t.BBCount++
		t.BBUSDT += b.USDT()
		t.BBRub += b.RubAmount
		t.Fees += b.FeeRub()
		if b.BuyDateISO >= lastDate {
			lastDate, t.LastBuyRate = b.BuyDateISO, b.Rate
		}
	}
	for _, d := range deals {
		usdt := d.VAAmount
		switch d.ActType {
		case "sell":
			r := Compute(d)
			if usdt <= 0 {
				usdt = r.Volume
			}
			t.SellCount++
			t.SellUSDT += usdt
			t.SellRub += d.FiatAmount
			if r.HasBuyback {
				t.CKD += r2(r.CKD)
				t.Tax += r2(r.Tax)
				t.Net += r2(r.Net)
				if d.ReferrerName != "" && d.ReferralPercent > 0 {
					t.RefAccrued[d.ReferrerName] += r2(r.Referral)
				}
			} else {
				t.OpenCount++
				t.OpenUSDT += usdt
				t.OpenRub += d.FiatAmount
			}
		case "buy":
			t.BuyCount++
			t.BuyUSDT += usdt
			t.BuyRub += d.FiatAmount
		}
	}
	if t.LastBuyRate > 0 {
		t.OpenEstimatedBuyUSD = t.OpenRub / (1 + feeOf(deals)/100) / t.LastBuyRate
	}
	for _, o := range ops {
		t.ManualCount++
		t.ManualUSDT += o.USDT
		t.ManualRub += o.Rub
		switch o.Kind {
		case "referral":
			t.RefPaid[o.ReferrerName] += -o.USDT
		case "tax":
			t.TaxPaid += -o.USDT
		}
	}
	t.USDT = t.OpeningUSDT - t.SellUSDT + t.BuyUSDT + t.BBUSDT + t.ManualUSDT
	t.Rub = t.OpeningRub + t.SellRub - t.BuyRub - t.BBRub - t.Fees + t.ManualRub
	return t
}

func feeOf(deals []*storage.Deal) float64 {
	for _, d := range deals {
		if d.FeePercent > 0 {
			return d.FeePercent
		}
	}
	return 0.05
}

// ─── Сводка «у кого сколько купили» ───────────────────────────────────────────

type SupplierStat struct {
	Name      string
	Count     int
	Rub, USDT float64
}

func (s SupplierStat) AvgRate() float64 {
	if s.USDT == 0 {
		return 0
	}
	return s.Rub / s.USDT
}

type ClientStat struct {
	Name                  string
	Count, Pending        int
	Rub, Volume, CKD, Net float64
}

// Stats — по поставщикам (откупки) и по клиентам (продажи), по убыванию объёма.
func Stats(deals []*storage.Deal, buybacks []*storage.Buyback) ([]SupplierStat, []ClientStat) {
	sup := map[string]*SupplierStat{}
	for _, b := range buybacks {
		s, ok := sup[b.SupplierName]
		if !ok {
			s = &SupplierStat{Name: b.SupplierName}
			sup[b.SupplierName] = s
		}
		s.Count++
		s.Rub += b.RubAmount
		s.USDT += b.USDT()
	}
	cl := map[string]*ClientStat{}
	for _, d := range deals {
		if !Profitable(d) {
			continue
		}
		r := Compute(d)
		c, ok := cl[d.ClientName]
		if !ok {
			c = &ClientStat{Name: d.ClientName}
			cl[d.ClientName] = c
		}
		c.Count++
		c.Rub += r.Rub
		c.Volume += r2(r.Volume)
		if r.HasBuyback {
			c.CKD += r2(r.CKD)
			c.Net += r2(r.Net)
		} else {
			c.Pending++
		}
	}
	var ss []SupplierStat
	for _, s := range sup {
		ss = append(ss, *s)
	}
	sort.Slice(ss, func(i, j int) bool { return ss[i].Rub > ss[j].Rub })
	var cs []ClientStat
	for _, c := range cl {
		cs = append(cs, *c)
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].Rub > cs[j].Rub })
	return ss, cs
}
