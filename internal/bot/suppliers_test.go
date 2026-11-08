package bot

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hodler-doc-bot/internal/storage"
)

func TestSuppliers(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.addCompany("Ромашка") // компания id=1
	clientWallet := func() string { co, _ := h.app.st.Company(ctx, 1); return co.Wallet }
	before := clientWallet()

	// Добавить поставщика (id=1, как у компании — проверяем, что данные не путаются)
	h.click("sup:add")
	h.expect(h.text("ООО «БРАЙТУМ»"), "Поставщик добавлен")
	h.click("sup:add")
	h.expect(h.text("ооо «брайтум»"), "уже есть")

	// Кошелёк поставщика
	h.click("wal:add:s1")
	h.click("wal:net:TRC-20")
	h.text("TSupp1erWa11etXXXXXXXXXXXXXXXXXXX")
	h.expect(h.text("Основной"), "Кошелёк добавлен")
	if clientWallet() != before {
		t.Fatalf("кошелёк поставщика затёр кошелёк клиента: %q", clientWallet())
	}
	h.click("acc:add:s1")
	h.text("Основной")
	h.text("Банк поставщика")
	h.text("40702")
	h.text("044")
	if co, _ := h.app.st.Company(ctx, 1); co.BankName == "Банк поставщика" {
		t.Fatal("реквизиты поставщика попали клиенту")
	}
	h.expect(h.click("sup:view:1"), "Кошельки (1)")

	// Откупка из карточки поставщика — без шага выбора поставщика
	h.sellDeal("1", "10", "100", "8000")
	h.click("sup:bb:1")
	h.click("bb:t:1")
	h.expect(h.click("bb:next"), "курс откупки")
	h.text("79,90")
	h.text("7990")
	h.click("bb:save")
	if d, _ := h.app.st.Deal(ctx, 1); d.SupplierName != "ООО «БРАЙТУМ»" {
		t.Fatalf("поставщик в сделке: %q", d.SupplierName)
	}
	h.expect(h.click("sup:view:1"), "Откупок: 1")

	// Переименование — и в откупках
	h.click("sup:ren:1")
	h.text("ООО «Брайтум Трейд»")
	if d, _ := h.app.st.Deal(ctx, 1); d.SupplierName != "ООО «Брайтум Трейд»" {
		t.Fatalf("имя не обновилось в откупке: %q", d.SupplierName)
	}

	// Поступление с кошелька поставщика подписывается
	op := "TXFEYN4C5BnesaxUXJiXJHGS7K12QutZ3r"
	h.app.st.DB().Exec("UPDATE wallets SET watch_since=? WHERE owner_type='operator'", time.Now().Add(-time.Minute).UnixMilli())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"success":true,"data":[{"transaction_id":"s1","from":"TSupp1erWa11etXXXXXXXXXXXXXXXXXXX","to":"%s",
			"value":"100000000","block_timestamp":%d,"token_info":{"decimals":6}}]}`, op, time.Now().UnixMilli())
	}))
	defer srv.Close()
	h.app.chain.TronURL = srv.URL
	h.app.checkWallets(ctx)
	if c := h.tg.lastTo("1", "Поступление +100,00"); c == nil || !strings.Contains(c.Text, "поставщик ООО «Брайтум Трейд»") {
		t.Fatalf("поступление от поставщика не подписано: %+v", c)
	}

	// Удаление: откупка остаётся с названием
	h.click("sup:delyes:1")
	if l, _ := h.app.st.Suppliers(ctx); len(l) != 0 {
		t.Fatal("поставщик не удалён")
	}
	if ws, _ := h.app.st.Wallets(ctx, storage.OwnerSupplier, 1); len(ws) != 0 {
		t.Fatal("кошельки поставщика не удалены")
	}
	if d, _ := h.app.st.Deal(ctx, 1); d.SupplierName != "ООО «Брайтум Трейд»" {
		t.Fatalf("откупка потеряла поставщика: %q", d.SupplierName)
	}
}
