package profit

import (
	"testing"

	"hodler-doc-bot/internal/storage"
)

func TestTreasury(t *testing.T) {
	opening := &storage.TreasuryOp{OpDate: "2026-11-01", USDT: 1000, Rub: 50000}
	d1 := &storage.Deal{ActType: "sell", Status: "active", ExchangeRate: "81,72", FiatAmount: 170000, VAAmount: 2080.27,
		TaxPercent: 5, FeePercent: 0.05, BuyRate: 81.40, SupplierName: "БРАЙТУМ", ReferrerName: "Дима", ReferralPercent: 20, ClientName: "А", PaidAt: "2026-11-02", USDTAt: "2026-11-02"}
	d2 := &storage.Deal{ActType: "sell", Status: "active", ExchangeRate: "81,72", FiatAmount: 1000000, VAAmount: 12236.91,
		TaxPercent: 5, FeePercent: 0.05, BuyRate: 81.40, SupplierName: "БРАЙТУМ", ReferrerName: "Дима", ReferralPercent: 20, ClientName: "Б", PaidAt: "2026-11-02", USDTAt: "2026-11-02"}
	open := &storage.Deal{ActType: "sell", Status: "active", ExchangeRate: "82", FiatAmount: 82000, VAAmount: 1000,
		TaxPercent: 5, FeePercent: 0.05, ClientName: "В", PaidAt: "2026-11-02", USDTAt: "2026-11-02"}
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
	// Сделка в работе: оплата не пришла, USDT не отправлен — на остатки не влияет
	pending := &storage.Deal{ActType: "sell", Status: "active", ExchangeRate: "80", FiatAmount: 80000, VAAmount: 1000}
	tr2 := ComputeTreasury(opening, []*storage.Deal{d1, d2, open, pending}, []*storage.Buyback{bb}, ops)
	if tr2.USDT != tr.USDT || tr2.Rub != tr.Rub || tr2.AwaitRub != 80000 || tr2.ToSendUSDT != 1000 {
		t.Fatalf("сделка в работе изменила остатки: %+v", tr2)
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
