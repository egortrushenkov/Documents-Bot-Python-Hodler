package bot

import (
	"context"
	"testing"
)

func TestStandaloneDocuments(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.group("/bind", 1)
	h.addCompany("Ромашка")
	groupBefore := len(h.tg.callsTo("-100"))

	// Акт с разбивкой тест + остаток
	h.expect(h.click("menu:standalone"), "Документ без сделки")
	h.click("act_type:sell")
	h.click("sel_company:1")
	h.text("777")
	h.click("skip:deal_date")
	h.click("va_type:USDT")
	h.click("network:TRC-20")
	h.text("1000")
	h.text("80000")
	h.click("skip:exchange_rate")
	h.click("kvvo:99082")
	h.click("skip:client_wallet")
	h.expect(h.click("skip:operator_wallet"), "Транзакции")
	h.click("tx:split")
	h.click("tx:add")
	h.text("100")
	h.text("hashT")
	h.click("tx:done")
	h.expect(h.text("hashR"), "Комиссия в RUB")
	h.click("skip:commission_fiat")
	h.click("skip:commission_va")
	h.expect(h.click("skip:execution_date"), "📄 Сформировать документ")
	h.click("act:generate")
	if !h.tg.find("sendDocument", "act_buy_777-") {
		t.Fatal("акт не отправлен")
	}
	h.expect(h.tg.last(), "не сохранён в журнал")

	// Счёт на оплату без сделки
	h.click("menu:standalone")
	h.click("act_type:invoice")
	h.click("sel_company:1")
	h.text("778")
	h.click("skip:deal_date")
	h.click("va_type:USDT")
	h.click("network:TRC-20")
	h.text("500")
	h.text("40000")
	h.click("skip:exchange_rate")
	h.click("kvvo:99082")
	h.click("skip:client_wallet")
	h.expect(h.click("skip:operator_wallet"), "Комиссия в RUB") // у счёта транзакций нет
	h.click("skip:commission_fiat")
	h.click("skip:commission_va")
	h.click("skip:execution_date")
	h.click("act:generate")
	if !h.tg.find("sendDocument", "invoice_778-") {
		t.Fatal("счёт не отправлен")
	}

	// Ничего не сохранено и не отправлено руководителям
	if d, _ := h.app.st.RecentDeals(ctx, 10, 0); len(d) != 0 {
		t.Fatalf("документ попал в журнал: %d", len(d))
	}
	if _, ok, _ := h.app.st.PeekCompanyNumber(ctx, 1, "act"); ok {
		t.Fatal("нумерация сдвинулась")
	}
	if len(h.tg.callsTo("-100")) != groupBefore {
		t.Fatal("ушло уведомление руководителям")
	}
	// Без флага «без сделки» счёт как тип сделки по-прежнему недоступен
	h.text(navCreateAct)
	h.click("act_type:invoice")
	if h.app.session(1, 1).State != stActSelectType {
		t.Fatal("в обычной сделке тип «счёт» принят")
	}
}
