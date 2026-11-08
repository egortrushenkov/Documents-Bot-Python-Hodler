package bot

import (
	"context"
	"testing"
)

func TestDealStagesBuyAndUndo(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.group("/bind", 1)
	h.addCompany("Ромашка")

	// Покупаем USDT у клиента: создана → USDT получен → ₽ выплачены → акт
	h.text(navCreateAct)
	h.click("act_type:buy")
	h.click("sel_company:1")
	h.text("300")
	h.click("skip:deal_date")
	h.click("va_type:USDT")
	h.click("network:TRC-20")
	h.text("1000")
	h.text("80000")
	h.click("skip:exchange_rate")
	h.click("kvvo:99082")
	h.click("skip:client_wallet")
	h.click("skip:operator_wallet")
	h.click("skip:commission_fiat")
	h.click("skip:commission_va")
	h.click("skip:execution_date")
	card := h.click("act:generate")
	h.expect(card, "👉 📥 USDT получен от клиента")
	h.expect(card, "st:usdt:1")

	// В работе
	h.expect(h.click("deals:open"), "№300")

	// Остатки: пока ничего не двигалось
	tr, _ := h.app.treasury(&Ctx{Context: ctx, app: h.app})
	if tr.USDT != 0 || tr.Rub != 0 || tr.ToPayRub != 80000 {
		t.Fatalf("до этапов: %+v", tr)
	}

	h.click("st:usdt:1")
	h.click("tx:single")
	h.text("hashBUY")
	h.expect(h.click("st:usdttoday:1"), "👉 💸 Рубли выплачены клиенту")
	if !h.tg.find("sendMessage", "USDT получен от клиента") {
		t.Fatal("нет уведомления об этапе")
	}

	h.click("st:paid:1")
	h.click("st:paidamt:1")
	h.expect(h.click("st:paidtoday:1"), "👉 📄 Акт")
	tr, _ = h.app.treasury(&Ctx{Context: ctx, app: h.app})
	if tr.USDT != 1000 || tr.Rub != -80000 {
		t.Fatalf("после этапов: USDT %v, ₽ %v", tr.USDT, tr.Rub)
	}

	// Снять отметку оплаты — снова «выплатить рубли», уведомление
	h.click("st:more:1")
	h.click("st:undo:1:payment")
	if d, _ := h.app.st.Deal(ctx, 1); d.PaidAt != "" || d.Stage() != "payment" {
		t.Fatalf("отметка не снята: %+v", d)
	}
	if !h.tg.find("sendMessage", "снята отметка — оплата") {
		t.Fatal("нет уведомления о снятии отметки")
	}

	// Акт без оплаты — предупреждение, затем «всё равно»
	h.expect(h.click("st:act:1"), "Ещё не отмечено: оплата ₽")
	h.click("st:act:1:force")
	d, _ := h.app.st.Deal(ctx, 1)
	if !d.Completed() || d.DocxFileID == "" {
		t.Fatalf("акт не сформирован: %+v", d)
	}
	if !h.tg.find("editMessageText", "✅ <b>Завершена — сделка №300") {
		t.Fatal("исходное уведомление не стало «завершена»")
	}
	if l, _ := h.app.st.DealsInProgress(ctx, 10); len(l) != 0 {
		t.Fatal("завершённая сделка осталась в работе")
	}
}

func TestCancelShowsTreasuryAndBuyback(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.addCompany("Ромашка")
	h.click("tr:opening")
	h.text("1000")
	h.text("50000")
	h.click("tr:openday")
	h.sellDeal("1", "1", "100", "8000")
	h.click("bb:new")
	h.click("bb:t:1")
	h.click("bb:next")
	h.text("БРАЙТУМ")
	h.text("79")
	h.text("7900")
	h.click("bb:save")

	// Предупреждение об откупке и вариант «вместе с откупкой»
	c := h.click("dealx:cancel:1")
	h.expect(c, "закрыта откупкой №1")
	h.expect(c, "dealx:cancelboth:1")
	c = h.click("dealx:cancelboth:1")
	h.expect(c, "💰 Остатки: USDT")
	tr, _ := h.app.treasury(&Ctx{Context: ctx, app: h.app})
	if tr.USDT != 1000 || tr.Rub != 50000 {
		t.Fatalf("остатки не вернулись: %v USDT, %v ₽", tr.USDT, tr.Rub)
	}
	if b, _ := h.app.st.Buyback(ctx, 1); !b.Cancelled() {
		t.Fatal("откупка не отменена")
	}
	// Восстановление сделки — остатки снова меняются, видно дельту
	h.expect(h.click("dealx:restore:1"), "→")
}
