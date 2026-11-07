package profit

import (
	"testing"
	"time"

	"hodler-doc-bot/internal/storage"
)

func refDeal(date, client string, rub float64, buy float64) *storage.Deal {
	return &storage.Deal{ActType: "sell", Status: "active", DealDateISO: date, ClientName: client,
		ExchangeRate: "81,75", FiatAmount: rub, BuyRate: buy, TaxPercent: 5, FeePercent: 0.05,
		ReferrerName: "Дима Реферал", ReferralPercent: 20}
}

func TestRefStatementWeek(t *testing.T) {
	deals := []*storage.Deal{
		refDeal("2026-10-30", "Старый", 1850000, 81.60), // прошлая неделя: 5,75 → долг на начало
		refDeal("2026-11-03", "Иксхаб", 1850000, 81.60), // 5,75
		refDeal("2026-11-04", "Иксхаб", 1850000, 81.60), // 5,75
		refDeal("2026-11-05", "Без откупки", 500000, 0), // ждёт откупки
		refDeal("2026-11-10", "Следующая", 1850000, 81.60),
	}
	ops := []*storage.TreasuryOp{
		{Kind: "referral_adj", OpDate: "2026-10-01", USDT: 100, ReferrerName: "Дима Реферал"}, // долг до бота
		{Kind: "referral", OpDate: "2026-11-03", USDT: -50, ReferrerName: "Дима Реферал"},     // выплата в среду
	}
	st := BuildRefStatement("Дима Реферал", deals, ops, "2026-11-02", "2026-11-08")
	if r2(st.OpeningOwed) != 105.75 {
		t.Fatalf("долг на начало: %v", st.OpeningOwed)
	}
	if r2(st.Accrued) != 11.50 || st.Paid != 50 || r2(st.ClosingOwed) != 67.25 {
		t.Fatalf("за период: начислено %v, выплачено %v, к выплате %v", st.Accrued, st.Paid, st.ClosingOwed)
	}
	if st.PendingCount != 1 || len(st.Days) != 3 || len(st.Days[0].Payouts) != 1 {
		t.Fatalf("дни: %+v", st.Days)
	}
	if r2(st.TotalOwed()) != 73.00 { // + сделка 10.11
		t.Fatalf("накоплено: %v", st.TotalOwed())
	}

	from, to, _ := Period("prevweek", time.Date(2026, 11, 11, 15, 0, 0, 0, time.UTC)) // среда
	if from != "2026-11-02" || to != "2026-11-08" {
		t.Fatalf("прошлая неделя: %s – %s", from, to)
	}
}
