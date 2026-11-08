package bot

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
)

func (h *harness) topic(text string, thread int) {
	h.app.handle(context.Background(), nil, &models.Update{Message: &models.Message{
		ID: 7, Text: text, MessageThreadID: thread, IsTopicMessage: thread != 0,
		Chat: models.Chat{ID: -100, Type: models.ChatTypeSupergroup, Title: "Руководство", IsForum: true},
		From: &models.User{ID: 1, FirstName: "Админ"}}})
}

func (f *fakeTG) lastTo(chat, substr string) *call {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		c := f.calls[i]
		if c.ChatID == chat && c.Method == "sendMessage" && strings.Contains(c.Text, substr) {
			return &c
		}
	}
	return nil
}

func TestForumTopics(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	h.topic("/bind", 42)          // сделки → ветка 42
	h.topic("/bind кошельки", 77) // поступления → ветка 77
	h.topic("/bind финансы", 99)  // выводы, выплаты → ветка 99
	if c := h.tg.lastTo("-100", "Подключено"); c == nil || c.Thread != "42" {
		t.Fatalf("ответ на /bind не в ветке: %+v", c)
	}
	h.topic("/status", 5)
	st := h.tg.lastTo("-100", "Уведомления этого чата")
	if st == nil || st.Thread != "5" || !strings.Contains(st.Text, "ветка №42") || !strings.Contains(st.Text, "ветка №99") {
		t.Fatalf("/status: %+v", st)
	}

	// Новая сделка и её правка — в ветку 42 (правка ответом там же)
	h.addCompany("Ромашка")
	h.sellDeal("1", "100", "100", "8000")
	if c := h.tg.lastTo("-100", "Новая сделка"); c == nil || c.Thread != "42" {
		t.Fatalf("сделка не в ветке 42: %+v", c)
	}
	h.click("dealx:cancelyes:1")
	if c := h.tg.lastTo("-100", "отменена"); c == nil || c.Thread != "42" || c.Reply == "" {
		t.Fatalf("отмена не ответом в ветке 42: %+v", c)
	}

	// Вывод прибыли — в ветку 99
	h.click("tr:op")
	h.click("tr:kind:withdraw")
	h.click("tr:cur:usdt")
	h.text("10")
	h.click("tr:notedone")
	if c := h.tg.lastTo("-100", "Вывод прибыли"); c == nil || c.Thread != "99" {
		t.Fatalf("вывод не в ветке 99: %+v", c)
	}

	// Поступление на кошелёк — в ветку 77 (когда включено «в чат руководителей»)
	h.click("notify:wgrp")
	op := "TXFEYN4C5BnesaxUXJiXJHGS7K12QutZ3r"
	h.app.st.DB().Exec("UPDATE wallets SET watch_since=? WHERE owner_type='operator'", time.Now().Add(-time.Minute).UnixMilli())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"success":true,"data":[{"transaction_id":"t1","from":"TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE","to":"%s",
			"value":"250000000","block_timestamp":%d,"token_info":{"decimals":6}}]}`, op, time.Now().UnixMilli())
	}))
	defer srv.Close()
	h.app.chain.TronURL = srv.URL
	h.app.checkWallets(ctx)
	if c := h.tg.lastTo("-100", "Поступление +250,00"); c == nil || c.Thread != "77" {
		t.Fatalf("поступление не в ветке 77: %+v", c)
	}
	if c := h.tg.lastTo("1", "Поступление +250,00"); c == nil || c.Thread != "" {
		t.Fatalf("в личку — без ветки: %+v", c)
	}

	// Команда в ветке — ответ в ту же ветку
	h.topic("/balance", 13)
	if c := h.tg.lastTo("-100", "Остатки на"); c == nil || c.Thread != "13" {
		t.Fatalf("/balance не в ветке 13: %+v", c)
	}
}

// Сообщение из темы без флага is_topic_message (так бывает в некоторых клиентах)
func TestForumTopicWithoutFlag(t *testing.T) {
	h := newHarness(t)
	h.app.handle(context.Background(), nil, &models.Update{Message: &models.Message{
		ID: 7, Text: "/bind", MessageThreadID: 55,
		Chat: models.Chat{ID: -100, Type: models.ChatTypeSupergroup, Title: "Руководство", IsForum: true},
		From: &models.User{ID: 1, FirstName: "Админ"}}})
	ch, _ := h.app.st.NotifyChat(context.Background(), -100)
	if ch == nil || ch.ThreadID != 55 {
		t.Fatalf("ветка не сохранена: %+v", ch)
	}
}
