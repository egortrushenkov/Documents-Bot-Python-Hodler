package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
)

var dumpHook func(*fakeTG)

func (f *fakeTG) callsTo(chat string) []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []call
	for _, c := range f.calls {
		if c.ChatID == chat {
			out = append(out, c)
		}
	}
	return out
}

func (h *harness) group(text string, userID int64) {
	h.app.handle(context.Background(), nil, &models.Update{Message: &models.Message{
		ID: 5, Text: text, Chat: models.Chat{ID: -100, Type: models.ChatTypeSupergroup, Title: "Руководство"},
		From: &models.User{ID: userID, FirstName: "Админ"}}})
}

func (h *harness) addCompany(name string) {
	h.click("add_company")
	for _, a := range []string{"ООО " + name, name, "1", "2", "3", "адрес", "-", "-", "-", "Банк", "408", "044", "TW"} {
		h.text(a)
	}
	h.click("resident:Резидент")
}

func (h *harness) sellDeal(companyID, number, va, rub string) {
	h.text(navCreateAct)
	h.click("act_type:sell")
	h.click("sel_company:" + companyID)
	h.text(number)
	h.click("skip:deal_date")
	h.click("va_type:USDT")
	h.click("network:TRC-20")
	h.text(va)
	h.text(rub)
	h.click("skip:exchange_rate")
	h.click("kvvo:99082")
	h.click("skip:client_wallet")
	h.click("skip:operator_wallet")
	h.click("tx:single")
	h.click("tx:one_skip")
	h.click("skip:commission_fiat")
	h.click("skip:commission_va")
	h.click("skip:execution_date")
	h.click("act:generate")
}

func TestBuybackProfitNotifications(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// Чат руководителей: посторонний не может подключить, админ — может
	h.group("/bind", 999)
	if n, _ := h.app.st.NotifyChats(ctx); len(n) != 0 {
		t.Fatal("подключил не-админ")
	}
	h.group("/bind@hodler_bot", 1)
	if n, _ := h.app.st.NotifyChats(ctx); len(n) != 1 || n[0].Title != "Руководство" {
		t.Fatalf("чат не подключён: %+v", n)
	}

	// Реферал, закреплённый за клиентом
	h.click("ref:add")
	h.text("Дима Реферал")
	h.expect(h.text("20"), "Дима Реферал — 20%")
	h.addCompany("Эвридэй Тхай")
	h.click("edit_company_field:1:referrer")
	h.expect(h.click("coref:1:1"), "Дима Реферал, 20%")

	// Две заявки клиентов: 170 000 и 1 000 000 ₽ по 81,72
	h.sellDeal("1", "601", "2080,27", "170000")
	h.sellDeal("1", "602", "12236,92", "1000000")
	g := h.tg.callsTo("-100")
	news := 0
	for _, c := range g {
		if c.Method == "sendMessage" && strings.Contains(c.Text, "Новая сделка") {
			news++
			if !strings.Contains(c.Text, "ещё не сделана") {
				t.Errorf("до откупки прибыль не должна считаться: %s", c.Text)
			}
		}
	}
	if news != 2 {
		t.Fatalf("ожидалось 2 уведомления о новых сделках, есть %d", news)
	}

	// Откупка на обе заявки: ООО «БРАЙТУМ», 81,40, 1 160 000 ₽
	h.expect(h.click("bb:new"), "№601")
	h.click("bb:t:1")
	h.click("bb:t:2")
	h.expect(h.click("bb:next"), "поставщик")
	h.text("ООО «БРАЙТУМ»")
	h.expect(h.text("81,40"), "1 160 000 ₽ (округлить до 10 000)")
	c := h.click("bb:rub:1160000")
	h.expect(c, "Остаток на счёте: 9 420,00 ₽")
	h.expect(c, "Не хватает")
	h.click("bb:save")
	if !h.tg.find("sendMessage", "Откупка №1") {
		t.Fatal("нет уведомления об откупке")
	}
	d1, _ := h.app.st.Deal(ctx, 1)
	if d1.BuyRate != 81.40 || d1.SupplierName != "ООО «БРАЙТУМ»" || d1.ReferrerName != "Дима Реферал" {
		t.Fatalf("сделка не связана с откупкой: %+v", d1)
	}

	// Отчёт за день
	h.click("profit:date")
	rep := h.text(time.Now().Format("02.01.2006"))
	for _, want := range []string{"📊 Классика за", "ООО «БРАЙТУМ» / Эвридэй Тхай", "Курс: 81,40 / 81,72 ₽",
		"Реферал:", "(Дима Реферал 20%)", "Сделок: 2", "👥 Рефералы:"} {
		if !strings.Contains(rep.Text, want) {
			t.Fatalf("в отчёте нет %q:\n%s", want, rep.Text)
		}
	}

	// Переделка заявки: меняем сумму → ответ «было → стало» на исходное уведомление
	h.click("deal:2")
	h.expect(h.click("dealx:edit:2"), "Переделка заявки")
	h.click("act:edit_menu")
	h.click("edit_act_field:fiat_amount")
	h.text("1050000")
	h.click("act:generate")
	found := false
	for _, c := range h.tg.callsTo("-100") {
		if c.Method == "sendMessage" && strings.Contains(c.Text, "Сделка №602 изменена") &&
			strings.Contains(c.Text, "1 000 000 → <b>1 050 000</b>") && c.Reply != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("нет ответа об изменении сделки")
	}
	d2, _ := h.app.st.Deal(ctx, 2)
	if d2.Version != 2 || d2.FiatAmount != 1050000 || !d2.BuybackID.Valid {
		t.Fatalf("после переделки: %+v", d2)
	}

	// Отмена: сделка уходит из отчёта, руководители получают ответ
	h.click("dealx:cancel:1")
	h.click("dealx:cancelyes:1")
	if !h.tg.find("sendMessage", "Сделка №601 отменена") {
		t.Fatal("нет уведомления об отмене")
	}
	if !h.tg.find("editMessageText", "<s>") {
		t.Fatal("исходное уведомление не зачёркнуто")
	}
	if deals, _ := h.app.st.DealsForDate(ctx, time.Now().Format("2006-01-02")); len(deals) != 1 {
		t.Fatalf("отменённая сделка осталась в отчёте: %d", len(deals))
	}

	// Отчёт в группе
	h.group("/today", 555)
	last := h.tg.callsTo("-100")
	if l := last[len(last)-1]; !strings.Contains(l.Text, "Сделок: 1") {
		t.Fatalf("/today в группе: %q", l.Text)
	}

	// XLSX за месяц
	h.click("profit:xlsx:" + time.Now().Format("2006-01"))
	if !h.tg.find("sendDocument", "Прибыль_") {
		t.Fatal("XLSX не отправлен")
	}

	if dumpHook != nil {
		dumpHook(h.tg)
	}
	// Очистка журнала
	nd, ns, err := h.app.st.PurgeDeals(ctx)
	if err != nil || nd != 2 {
		t.Fatalf("purge: %d %d %v", nd, ns, err)
	}
	if co, _ := h.app.st.Companies(ctx); len(co) != 1 {
		t.Fatal("компании не должны удаляться")
	}
	if n, ok, _ := h.app.st.PeekCompanyNumber(ctx, 1, "act"); !ok || n != 603 {
		t.Fatalf("нумерация должна сохраниться: %d", n)
	}
}
