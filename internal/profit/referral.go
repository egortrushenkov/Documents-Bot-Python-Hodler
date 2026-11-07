package profit

import (
	"sort"
	"time"

	"hodler-doc-bot/internal/storage"
)

// RefLine — начисление по сделке.
type RefLine struct {
	Deal    *storage.Deal
	Result  Result
	Amount  float64 // округлено до центов
	Pending bool    // откупки ещё нет — реферал не начислен
}

// RefDay — день расчёта: начисления по сделкам, ручные начисления и выплаты.
type RefDay struct {
	DateISO     string
	Lines       []RefLine
	Adjustments []*storage.TreasuryOp
	Payouts     []*storage.TreasuryOp
}

func (d RefDay) Accrued() float64 {
	s := 0.0
	for _, l := range d.Lines {
		s += l.Amount
	}
	for _, a := range d.Adjustments {
		s += a.USDT
	}
	return s
}

func (d RefDay) Paid() float64 {
	s := 0.0
	for _, p := range d.Payouts {
		s += -p.USDT
	}
	return s
}

// RefStatement — расчёт с рефералом за период [From, To].
type RefStatement struct {
	Name     string
	From, To string // YYYY-MM-DD ("" — без границы)

	OpeningOwed float64 // долг на начало периода
	Accrued     float64 // начислено за период
	Paid        float64 // выплачено за период
	ClosingOwed float64 // к выплате на конец периода

	TotalAccrued, TotalPaid float64 // за всё время
	PendingCount            int     // сделки периода без откупки
	PendingRub              float64
	Days                    []RefDay
}

// TotalOwed — накоплено и не выплачено на сегодня.
func (s RefStatement) TotalOwed() float64 { return s.TotalAccrued - s.TotalPaid }

func inPeriod(date, from, to string) bool {
	return (from == "" || date >= from) && (to == "" || date <= to)
}

// BuildRefStatement строит расчёт. deals — все продажи реферала, ops — все его
// выплаты (kind=referral, usdt<0) и ручные начисления (kind=referral_adj).
func BuildRefStatement(name string, deals []*storage.Deal, ops []*storage.TreasuryOp, from, to string) RefStatement {
	st := RefStatement{Name: name, From: from, To: to}
	days := map[string]*RefDay{}
	day := func(date string) *RefDay {
		d, ok := days[date]
		if !ok {
			d = &RefDay{DateISO: date}
			days[date] = d
		}
		return d
	}
	for _, d := range deals {
		r := Compute(d)
		amount := 0.0
		if r.HasBuyback {
			amount = r2(r.Referral)
		}
		st.TotalAccrued += amount
		date := d.DealDateISO
		switch {
		case from != "" && date < from:
			st.OpeningOwed += amount
		case inPeriod(date, from, to):
			st.Accrued += amount
			if !r.HasBuyback {
				st.PendingCount++
				st.PendingRub += r.Rub
			}
			day(date).Lines = append(day(date).Lines, RefLine{Deal: d, Result: r, Amount: amount, Pending: !r.HasBuyback})
		}
	}
	for _, o := range ops {
		var accrued, paid float64
		if o.Kind == "referral_adj" {
			accrued = o.USDT
		} else {
			paid = -o.USDT
		}
		st.TotalAccrued += accrued
		st.TotalPaid += paid
		switch {
		case from != "" && o.OpDate < from:
			st.OpeningOwed += accrued - paid
		case inPeriod(o.OpDate, from, to):
			st.Accrued += accrued
			st.Paid += paid
			if o.Kind == "referral_adj" {
				day(o.OpDate).Adjustments = append(day(o.OpDate).Adjustments, o)
			} else {
				day(o.OpDate).Payouts = append(day(o.OpDate).Payouts, o)
			}
		}
	}
	st.ClosingOwed = st.OpeningOwed + st.Accrued - st.Paid
	for _, d := range days {
		st.Days = append(st.Days, *d)
	}
	sort.Slice(st.Days, func(i, j int) bool { return st.Days[i].DateISO < st.Days[j].DateISO })
	return st
}

// Period — готовые периоды: week, prevweek, month, prevmonth, all.
func Period(code string, now time.Time) (from, to string, title string) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	f := func(t time.Time) string { return t.Format("2006-01-02") }
	wd := (int(day.Weekday()) + 6) % 7 // понедельник = 0
	monday := day.AddDate(0, 0, -wd)
	switch code {
	case "week":
		return f(monday), f(day), "эта неделя"
	case "prevweek":
		return f(monday.AddDate(0, 0, -7)), f(monday.AddDate(0, 0, -1)), "прошлая неделя"
	case "month":
		return f(time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location())), f(day), "этот месяц"
	case "prevmonth":
		first := time.Date(day.Year(), day.Month()-1, 1, 0, 0, 0, 0, day.Location())
		return f(first), f(first.AddDate(0, 1, -1)), "прошлый месяц"
	case "today":
		return f(day), f(day), "сегодня"
	}
	return "", "", "всё время"
}
