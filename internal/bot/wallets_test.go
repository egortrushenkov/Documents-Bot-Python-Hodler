package bot

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hodler-doc-bot/internal/storage"
)

func TestMultipleAccountsInAct(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.addCompany("Ромашка") // мастер создаёт первые реквизиты и кошелёк

	// Второй счёт и кошелёк клиента
	h.click("acc:add:c1")
	h.text("Тинькофф")
	h.text("АО Тинькофф Банк")
	h.text("40702810999")
	h.expect(h.text("044525974"), "Реквизиты добавлены")
	h.click("wal:add:c1")
	h.click("wal:net:TRC-20")
	h.text("TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE")
	h.expect(h.text("Binance"), "Кошелёк добавлен")
	h.expect(h.click("view_company:1"), "Реквизиты (2)")

	// Наш второй счёт
	h.click("acc:add:op")
	h.text("Резервный")
	h.text("Банк Б")
	h.text("40807810000")
	h.text("046")

	h.text(navCreateAct)
	h.click("act_type:sell")
	h.click("sel_company:1")
	h.text("900")
	h.click("skip:deal_date")
	h.click("va_type:USDT")
	h.click("network:TRC-20")
	h.text("100")
	h.text("8000")
	h.click("skip:exchange_rate")
	c := h.click("kvvo:99082")
	h.expect(c, "Binance · TRC-20")
	ws, _ := h.app.st.Wallets(ctx, storage.OwnerCompany, 1)
	var binance int64
	for _, w := range ws {
		if w.Label == "Binance" {
			binance = w.ID
		}
	}
	h.click(fmt.Sprintf("cw:%d", binance))
	h.expect(h.click("skip:operator_wallet"), "Реквизиты клиента")
	banks, _ := h.app.st.BankAccounts(ctx, storage.OwnerCompany, 1)
	var tink int64
	for _, b := range banks {
		if b.Label == "Тинькофф" {
			tink = b.ID
		}
	}
	h.expect(h.click(fmt.Sprintf("cb:%d", tink)), "Наши реквизиты")
	ops, _ := h.app.st.BankAccounts(ctx, storage.OwnerOperator, 0)
	h.click(fmt.Sprintf("ob:%d", ops[len(ops)-1].ID))
	h.click("skip:commission_fiat")
	h.click("skip:commission_va")
	h.expect(h.click("skip:execution_date"), "Реквизиты клиента:</b> Тинькофф")
	h.click("act:generate")

	d, _ := h.app.st.Deal(ctx, 1)
	if d == nil || d.ClientWallet != "TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE" || !d.ClientBankID.Valid || d.ClientBankID.Int64 != tink {
		t.Fatalf("в сделке не те реквизиты: %+v", d)
	}
	// Основные реквизиты клиента не изменились
	if co, _ := h.app.st.Company(ctx, 1); co.BankAccount != "408" {
		t.Fatalf("основные реквизиты затронуты: %q", co.BankAccount)
	}
}

func TestWalletIncomingNotification(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.addCompany("Ромашка")
	since := time.Now().Add(-time.Minute).UnixMilli()
	opWallet := "TXFEYN4C5BnesaxUXJiXJHGS7K12QutZ3r"
	clientWallet := "TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE"
	h.app.st.AddWallet(ctx, &storage.Wallet{OwnerType: storage.OwnerCompany, OwnerID: 1, Label: "Binance", Network: "TRC-20", Address: clientWallet})
	// Кошелёк оператора уже создан миграцией из реквизитов — повтор не добавляется
	if _, err := h.app.st.AddWallet(ctx, &storage.Wallet{OwnerType: storage.OwnerOperator, Address: opWallet}); err != storage.ErrDuplicateWallet {
		t.Fatalf("дубль кошелька: %v", err)
	}
	h.app.st.DB().Exec("UPDATE wallets SET watch_since=? WHERE owner_type='operator'", since)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		now := time.Now().UnixMilli()
		fmt.Fprintf(w, `{"success":true,"data":[
			{"transaction_id":"new1","from":"%s","to":"%s","value":"500000000","block_timestamp":%d,"token_info":{"decimals":6}},
			{"transaction_id":"old1","from":"%s","to":"%s","value":"999000000","block_timestamp":%d,"token_info":{"decimals":6}},
			{"transaction_id":"out1","from":"%s","to":"TVaGj4RBvdE5CBS8UTAmEByZR7Zr6E6DV8","value":"100000000","block_timestamp":%d,"token_info":{"decimals":6}}]}`,
			clientWallet, opWallet, now, clientWallet, opWallet, since-3600_000, opWallet, now)
	}))
	defer srv.Close()
	h.app.chain.TronURL = srv.URL

	h.app.checkWallets(ctx)
	h.app.checkWallets(ctx) // повторная проверка — без дублей
	var got []call
	for _, c := range h.tg.callsTo("1") {
		if c.Method == "sendMessage" && strings.Contains(c.Text, "USDT</b> (TRC-20)") {
			got = append(got, c)
		}
	}
	if len(got) != 1 {
		t.Fatalf("ожидалось 1 уведомление, получено %d: %+v", len(got), got)
	}
	for _, want := range []string{"Поступление +500,00 USDT", "Ромашка", "tronscan.org/#/transaction/new1"} {
		if !strings.Contains(got[0].Text, want) {
			t.Fatalf("нет %q в %q", want, got[0].Text)
		}
	}

	// Карточка кошелька: баланс и история (тот же фейковый сервер)
	ws, _ := h.app.st.Wallets(ctx, storage.OwnerOperator, 0)
	card := h.click(fmt.Sprintf("wal:view:%d", ws[0].ID))
	h.expect(card, "Поступления: ✅ вкл")
	h.expect(card, "+500,00")

	// Включаем списания — следующая исходящая придёт
	h.click(fmt.Sprintf("wal:out:%d", ws[0].ID))
	if w, _ := h.app.st.Wallet(ctx, ws[0].ID); !w.WatchOut {
		t.Fatal("списания не включились")
	}
	if calls.Load() < 2 {
		t.Fatal("сервер блокчейна не опрашивался")
	}
}
