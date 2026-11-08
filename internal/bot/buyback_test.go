package bot

import (
	"context"
	"fmt"
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
	h.click("skip:commission_fiat")
	h.click("skip:commission_va")
	h.click("skip:execution_date")
	h.click("act:generate")
	// все этапы: без счёта → оплата → USDT → акт
	id := h.lastDealID()
	sid := fmt.Sprint(id)
	h.click("st:invskip:" + sid)
	h.click("st:paid:" + sid)
	h.click("st:paidamt:" + sid)
	h.click("st:paidtoday:" + sid)
	h.click("st:usdt:" + sid)
	h.click("tx:single")
	h.click("tx:one_skip")
	h.click("st:usdttoday:" + sid)
	h.click("st:act:" + sid)
}

func (h *harness) lastDealID() int64 {
	var id int64
	h.app.st.DB().QueryRow("SELECT MAX(id) FROM deals").Scan(&id)
	return id
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
	h.expect(h.click("dealx:edit:2"), "Изменение сделки")
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

func TestTreasuryFlow(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.group("/bind", 1)
	h.addCompany("Клиент")

	// Начальный остаток: 1000 USDT, 50 000 ₽ с сегодняшнего дня
	h.click("tr:opening")
	h.text("1000")
	h.text("50000")
	h.expect(h.click("tr:openday"), "USDT:</b> 1 000,00")

	// Продажа без откупки → USDT уходит, рубли приходят, «выдано наперёд»
	h.sellDeal("1", "700", "1000", "82000")
	c := h.text(navTreasury)
	h.expect(c, "<b>USDT:</b> 0,00")
	h.expect(c, "₽ на счёте:</b> 132 000,00")
	h.expect(c, "Выдано наперёд без откупки:</b> 1 сд.")

	// Вывод прибыли: уменьшает USDT и приходит уведомлением в группу
	h.click("tr:op")
	h.click("tr:kind:withdraw")
	h.click("tr:cur:usdt")
	h.text("10")
	h.expect(h.text("на карту"), "USDT:</b> -10,00 ⚠️")
	if !h.tg.find("sendMessage", "Вывод прибыли") {
		t.Fatal("нет уведомления о выводе")
	}

	// В группе: /balance и /stats
	h.group("/balance", 555)
	g := h.tg.callsTo("-100")
	if !strings.Contains(g[len(g)-1].Text, "Остатки на") {
		t.Fatalf("/balance: %q", g[len(g)-1].Text)
	}
	h.group("/stats сегодня", 555)
	g = h.tg.callsTo("-100")
	if l := g[len(g)-1].Text; !strings.Contains(l, "Кому продавали") || !strings.Contains(l, "1 без откупки") {
		t.Fatalf("/stats: %q", l)
	}

	// Отмена сделки возвращает остатки
	h.click("dealx:cancelyes:1")
	tr, _ := h.app.treasury(&Ctx{Context: ctx, app: h.app})
	if r := tr.Rub; r != 50000 || tr.USDT != 990 {
		t.Fatalf("после отмены: %v ₽, %v USDT", r, tr.USDT)
	}
}

func TestReferralPayouts(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.group("/bind", 1)
	h.click("ref:add")
	h.text("Дима Реферал")
	h.text("20")
	h.addCompany("Иксхаб")
	h.click("edit_company_field:1:referrer")
	h.click("coref:1:1")
	h.sellDeal("1", "800", "22629,97", "1850000")
	h.click("bb:new")
	h.click("bb:t:1")
	h.click("bb:next")
	h.text("БРАЙТУМ")
	h.text("81,60")
	h.text("1850000")
	h.click("bb:save")

	// Долг до бота — ручное начисление
	h.click("ref:adj:1")
	h.text("100")
	h.click("ref:paytoday")
	h.expect(h.click("ref:paynonote"), "Накоплено к выплате: 105,75 USDT")

	// Расчёт за сегодня → кнопка «Выплатить» с точной суммой
	day := time.Now().Format("20060102")
	st := h.click("ref:st:1:" + day + ":" + day)
	h.expect(st, "Иксхаб — 1 850 000 ₽, ЧКД 30,28 → <b>5,75</b>")
	h.expect(st, "К выплате на")
	h.expect(st, "ref:payq:1:10575:")

	// Выплата 50 за период, с датой и комментарием
	h.click("ref:payq:1:5000:" + day + ":" + day)
	h.click("ref:paytoday")
	h.expect(h.text("наличными"), "Накоплено к выплате: 55,75 USDT")
	if !h.tg.find("sendMessage", "Выплата рефералу Дима Реферал: 50,00 USDT") {
		t.Fatal("нет уведомления о выплате")
	}

	// Правка суммы выплаты → долг пересчитан, руководителям ответ «изменена»
	ops, _ := h.app.st.ReferrerOps(ctx, "Дима Реферал")
	var payID int64
	for _, o := range ops {
		if o.Kind == "referral" {
			payID = o.ID
		}
	}
	h.click(fmt.Sprintf("ref:oe:%d:amount", payID))
	h.text("60")
	if !h.tg.find("sendMessage", "Выплата изменена") {
		t.Fatal("нет уведомления об изменении выплаты")
	}
	h.expect(h.click("ref:view:1"), "Накоплено к выплате: 45,75 USDT")

	// Остатки USDT учитывают выплату, но не ручное начисление
	tr, _ := h.app.treasury(&Ctx{Context: ctx, app: h.app})
	if r := tr.ManualUSDT; r != -60 {
		t.Fatalf("ручные операции в остатках: %v", r)
	}

	// Переименование реферала сохраняет историю
	h.click("ref:ename:1")
	h.text("Дмитрий")
	h.expect(h.click("ref:view:1"), "Накоплено к выплате: 45,75 USDT")

	// Удаление выплаты
	h.click(fmt.Sprintf("ref:odyes:%d", payID))
	h.expect(h.click("ref:view:1"), "Накоплено к выплате: 105,75 USDT")

	// /ref в группе
	h.group("/ref", 555)
	g := h.tg.callsTo("-100")
	if l := g[len(g)-1].Text; !strings.Contains(l, "Дмитрий") || !strings.Contains(l, "105,75 USDT") {
		t.Fatalf("/ref: %q", l)
	}
}
