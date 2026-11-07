package profit

import (
	"strings"
	"testing"
	"time"

	"hodler-doc-bot/internal/storage"
)

func deal(supplier, client, buy, sell string, rub float64, ref, network string) *storage.Deal {
	buyRate := parse(buy)
	return &storage.Deal{ActType: "sell", Status: "active", SupplierName: supplier, ClientName: client,
		BuyRate: buyRate, ExchangeRate: sell, FiatAmount: rub, Network: network,
		TaxPercent: 5, FeePercent: 0.05, ReferrerName: ref, ReferralPercent: 20}
}

func parse(s string) float64 {
	s = strings.ReplaceAll(s, ",", ".")
	var whole, frac float64
	var div float64 = 1
	dot := false
	for _, c := range s {
		if c == '.' {
			dot = true
			continue
		}
		if dot {
			div *= 10
			frac = frac*10 + float64(c-'0')
		} else {
			whole = whole*10 + float64(c-'0')
		}
	}
	return whole + frac/div
}

// Отчёт руководству за 03.11.2026 — рассчитан вручную; бот должен выдать то же.
const expected = `📊 Классика за 03.11.2026

1️⃣ ООО «БРАЙТУМ» / Иксхаб Пэймент Сервис
Курс: 81,60 / 81,75 ₽
Объём: 22 629,97 USDT (1 850 000 ₽)
ЧКД: 30,28 USDT
Налог (5%): 1,51 USDT
Реферал: 5,75 USDT (Дима Реферал 20%)
Чистая прибыль: 23,02 USDT

2️⃣ ООО «БРАЙТУМ» / ООО «Эвридэй Тхай»
Курс: 81,40 / 82,58 ₽
Объём: 3 027,37 USDT (250 000 ₽)
ЧКД: 42,37 USDT
Налог (5%): 2,12 USDT
Реферал: 8,05 USDT (Дима Реферал 20%)
Чистая прибыль: 32,20 USDT

3️⃣ ООО «БРАЙТУМ» / ОсОО «Алтынкопрю»
Курс: 81,50 / 81,69 ₽
Объём: 81 527,73 USDT (6 660 000 ₽)
ЧКД: 149,30 USDT
Налог (5%): 7,47 USDT
Реферал: 28,37 USDT (Дима Реферал 20%)
Чистая прибыль: 113,47 USDT

4️⃣ ООО «БРАЙТУМ» / ЗАО «Онли Файненс»
Курс: 81,40 / 81,72 ₽
Объём: 116 250,61 USDT (9 500 000 ₽)
ЧКД: 398,88 USDT
Налог (5%): 19,94 USDT
Реферал: 75,79 USDT (Игорь Мексика Реферал 20%)
Чистая прибыль: 303,15 USDT

5️⃣ ЗАО Пиксель Интернет / Digitalpay FZE
Курс: 81,54 / 81,70 ₽
Объём: 538 555,69 USDT (44 000 000 ₽)
ЧКД: 787,49 USDT
Налог (5%): 39,37 USDT
Реферал: 149,62 USDT (Саша Булах Реферал 20%)
Чистая прибыль: 598,49 USDT

6️⃣ ООО «БРАЙТУМ» / ОсОО «Интернешенл Эдукейшен»
Курс: 81,60 / 81,75 ₽
Объём: 73 394,50 USDT (6 000 000 ₽), ERC-20
ЧКД: 98,22 USDT
Налог (5%): 4,91 USDT
Реферал: 18,66 USDT (Партнёр Delirio 20%)
Чистая прибыль: 74,65 USDT

7️⃣ ООО «БРАЙТУМ» / ООО «Эвридэй Тхай»
Курс: 81,70 / 82,58 ₽
Объём: 3 572,29 USDT (295 000 ₽)
ЧКД: 36,69 USDT
Налог (5%): 1,83 USDT
Реферал: 6,97 USDT (Дима Реферал 20%)
Чистая прибыль: 27,89 USDT

Итого за 03.11:
Сделок: 7
Объём: 838 958,16 USDT (68 555 000 ₽)
ЧКД: 1 543,23 USDT
Налог (5%): 77,15 USDT
Рефералы: 293,21 USDT
Чистая прибыль: 1 172,87 USDT`

func TestDailyReportMatchesManual(t *testing.T) {
	b := "ООО «БРАЙТУМ»"
	deals := []*storage.Deal{
		deal(b, "Иксхаб Пэймент Сервис", "81,60", "81,75", 1850000, "Дима Реферал", "TRC-20"),
		deal(b, "ООО «Эвридэй Тхай»", "81,40", "82,58", 250000, "Дима Реферал", "TRC-20"),
		deal(b, "ОсОО «Алтынкопрю»", "81,50", "81,69", 6660000, "Дима Реферал", "TRC-20"),
		deal(b, "ЗАО «Онли Файненс»", "81,40", "81,72", 9500000, "Игорь Мексика Реферал", "TRC-20"),
		deal("ЗАО Пиксель Интернет", "Digitalpay FZE", "81,54", "81,70", 44000000, "Саша Булах Реферал", "TRC-20"),
		deal(b, "ОсОО «Интернешенл Эдукейшен»", "81,60", "81,75", 6000000, "Партнёр Delirio", "ERC-20"),
		deal(b, "ООО «Эвридэй Тхай»", "81,70", "82,58", 295000, "Дима Реферал", "TRC-20"),
	}
	got := DailyReport("Классика", time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC), deals)
	main, refs, _ := strings.Cut(got, "\n\n👥 Рефералы:\n")
	if main != expected {
		t.Fatalf("отчёт не совпал:\n%s", main)
	}
	want := "Дима Реферал: 5,75 (Иксхаб Пэймент Сервис) + 8,05 (Эвридэй Тхай) + 28,37 (Алтынкопрю) + 6,97 (Эвридэй Тхай) = 49,14 USDT\n" +
		"Игорь Мексика Реферал: 75,79 (Онли Файненс) USDT\n" +
		"Саша Булах Реферал: 149,62 (Digitalpay FZE) USDT\n" +
		"Партнёр Delirio: 18,66 (Интернешенл Эдукейшен) USDT\n" +
		"Итого рефералам: 293,21 USDT"
	if refs != want {
		t.Fatalf("рефералы:\n%s", refs)
	}
}

func TestBuybackSummary(t *testing.T) {
	// 170 000 + 1 000 000 ₽ по 81,72 закрыты откупкой 1 160 000 ₽ по 81,40
	d1 := &storage.Deal{ActType: "sell", Status: "active", ExchangeRate: "81,72", FiatAmount: 170000, TaxPercent: 5, FeePercent: 0.05}
	d2 := &storage.Deal{ActType: "sell", Status: "active", ExchangeRate: "81,72", FiatAmount: 1000000, TaxPercent: 5, FeePercent: 0.05}
	bb := &storage.Buyback{RubAmount: 1160000, Rate: 81.40, FeePercent: 0.05}
	s := SummarizeBuyback(bb, []*storage.Deal{d1, d2})
	if r2(s.RubLeft) != 9420 || r2(s.USDTDiff) >= 0 || s.CKD <= 0 {
		t.Fatalf("%+v", s)
	}
	exact, rounded := SuggestRub(1170000, 0.05)
	if exact != 1169415 || rounded != 1160000 {
		t.Fatalf("%v %v", exact, rounded)
	}
}
