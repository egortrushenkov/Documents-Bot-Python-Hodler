package profit

import (
	"testing"

	"hodler-doc-bot/internal/storage"
)

func TestTreasury(t *testing.T) {
	opening := &storage.TreasuryOp{OpDate: "2026-11-01", USDT: 1000, Rub: 50000}
	d1 := &storage.Deal{ActType: "sell", Status: "active", ExchangeRate: "81,72", FiatAmount: 170000, VAAmount: 2080.27,
		TaxPercent: 5, FeePercent: 0.05, BuyRate: 81.40, SupplierName: "БРАЙТУМ", ReferrerName: "Дима", ReferralPercent: 20, ClientName: "А"}
	d2 := &storage.Deal{ActType: "sell", Status: "active", ExchangeRate: "81,72", FiatAmount: 1000000, VAAmount: 12236.91,
		TaxPercent: 5, FeePercent: 0.05, BuyRate: 81.40, SupplierName: "БРАЙТУМ", ReferrerName: "Дима", ReferralPercent: 20, ClientName: "Б"}
	open := &storage.Deal{ActType: "sell", Status: "active", ExchangeRate: "82", FiatAmount: 82000, VAAmount: 1000,
		TaxPercent: 5, FeePercent: 0.05, ClientName: "В"}
	bb := &storage.Buyback{RubAmount: 1160000, Rate: 81.40, FeePercent: 0.05, SupplierName: "БРАЙТУМ"}
	ops := []*storage.TreasuryOp{
		{Kind: "withdraw", USDT: -100},
		{Kind: "referral", USDT: -5, ReferrerName: "Дима"},
	}
	tr := ComputeTreasury(opening, []*storage.Deal{d1, d2, open}, []*storage.Buyback{bb}, ops)

	wantUSDT := 1000 - (2080.27 + 12236.91 + 1000) + 1160000/81.40 - 105
	wantRub := 50000.0 + 170000 + 1000000 + 82000 - 1160000 - 580
	if r2(tr.USDT) != r2(wantUSDT) || r2(tr.Rub) != r2(wantRub) {
		t.Fatalf("остатки %v %v, ожидалось %v %v", tr.USDT, tr.Rub, wantUSDT, wantRub)
	}
	if tr.OpenCount != 1 || tr.OpenUSDT != 1000 || tr.OpenRub != 82000 || tr.OpenEstimatedBuyUSD <= 0 {
		t.Fatalf("выдано наперёд: %+v", tr)
	}
	debts := tr.RefOwed()
	if len(debts) != 1 || debts[0].Paid != 5 || debts[0].Accrued <= 5 {
		t.Fatalf("рефералы: %+v", debts)
	}

	sups, clients := Stats([]*storage.Deal{d1, d2, open}, []*storage.Buyback{bb})
	if len(sups) != 1 || sups[0].Rub != 1160000 || r2(sups[0].AvgRate()) != 81.40 {
		t.Fatalf("поставщики: %+v", sups)
	}
	if len(clients) != 3 || clients[0].Name != "Б" || clients[2].Pending != 1 {
		t.Fatalf("клиенты: %+v", clients)
	}
}
