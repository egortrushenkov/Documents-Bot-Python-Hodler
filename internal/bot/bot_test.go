package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tg "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"hodler-doc-bot/internal/config"
	"hodler-doc-bot/internal/storage"
)

// fakeTG — минимальный Bot API: запоминает вызовы, отдаёт успешные ответы.
type fakeTG struct {
	mu     sync.Mutex
	calls  []call
	nextID int
	file   string // что отдавать при скачивании
}

type call struct {
	Thread string // message_thread_id
	ChatID string
	Reply  string // reply_parameters
	Method string
	Text   string // text / caption
	Markup string
	File   string // имя загруженного файла
}

func (f *fakeTG) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/file/") {
		http.ServeFile(w, r, f.file)
		return
	}
	_ = r.ParseMultipartForm(64 << 20)
	method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	c := call{Method: method, Text: r.FormValue("text") + r.FormValue("caption"), Markup: r.FormValue("reply_markup"),
		ChatID: r.FormValue("chat_id"), Reply: r.FormValue("reply_parameters"), Thread: r.FormValue("message_thread_id")}
	if r.MultipartForm != nil {
		for _, fhs := range r.MultipartForm.File {
			c.File = fhs[0].Filename
		}
	}
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.nextID++
	id := f.nextID
	f.mu.Unlock()

	var result any = true
	switch method {
	case "sendMessage", "editMessageText", "sendPhoto":
		result = map[string]any{"message_id": id, "date": 0, "chat": map[string]any{"id": 1, "type": "private"}}
	case "sendDocument":
		result = map[string]any{"message_id": id, "date": 0, "chat": map[string]any{"id": 1, "type": "private"},
			"document": map[string]any{"file_id": fmt.Sprintf("FILE_%d", id), "file_unique_id": "u"}}
	case "getFile":
		result = map[string]any{"file_id": r.FormValue("file_id"), "file_unique_id": "u", "file_path": "doc.bin"}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}

func (f *fakeTG) last() call {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i].Method != "answerCallbackQuery" {
			return f.calls[i]
		}
	}
	return call{}
}

func (f *fakeTG) find(method, substr string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c.Method == method && strings.Contains(c.Text+c.File, substr) {
			return true
		}
	}
	return false
}

type harness struct {
	t   *testing.T
	app *App
	tg  *fakeTG
	st  *storage.Store
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	st, err := storage.Open(filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.Migrate(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	tplDir := filepath.Join(dir, "templates")
	os.MkdirAll(tplDir, 0o755)
	for _, name := range config.ActTemplateFiles {
		b, err := os.ReadFile(filepath.Join("../../templates", name))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(tplDir, name), b, 0o644)
	}
	fake := &fakeTG{file: filepath.Join(tplDir, config.TemplateBuy)}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	cfg := &config.Config{
		BotToken: "123:TEST", AdminIDs: map[int64]bool{1: true},
		DBPath: "", TemplatesDir: tplDir, OutputDir: filepath.Join(dir, "out"),
		LibreOfficePath: "false", // PDF «не создаётся» — проверяем ветку предупреждения
		PDFConcurrency:  1,
	}
	app, err := New(cfg, st, tg.WithServerURL(srv.URL), tg.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, app: app, tg: fake, st: st}
}

func (h *harness) text(s string) call {
	h.app.handle(context.Background(), nil, &models.Update{Message: &models.Message{
		ID: 1, Text: s, Chat: models.Chat{ID: 1}, From: &models.User{ID: 1}}})
	return h.tg.last()
}

func (h *harness) doc(fileName string) call {
	h.app.handle(context.Background(), nil, &models.Update{Message: &models.Message{
		ID: 1, Chat: models.Chat{ID: 1}, From: &models.User{ID: 1},
		Document: &models.Document{FileID: "DOC1", FileName: fileName, FileSize: 1000}}})
	return h.tg.last()
}

func (h *harness) click(data string) call {
	h.app.handle(context.Background(), nil, &models.Update{CallbackQuery: &models.CallbackQuery{
		ID: "cb", From: models.User{ID: 1}, Data: data,
		Message: models.MaybeInaccessibleMessage{Type: models.MaybeInaccessibleMessageTypeMessage,
			Message: &models.Message{ID: 99, Chat: models.Chat{ID: 1}}}}})
	return h.tg.last()
}

func (h *harness) expect(c call, substr string) {
	h.t.Helper()
	if !strings.Contains(c.Text+c.Markup+c.File, substr) {
		h.t.Fatalf("ожидалось %q, получено %s: %q / %s", substr, c.Method, c.Text, c.Markup)
	}
}

func TestFullFlow(t *testing.T) {
	h := newHarness(t)

	h.expect(h.text("/start"), "Быстрый доступ")

	// ─── Компания ───
	h.expect(h.click("add_company"), "Шаг 1/13")
	answers := []string{"ООО «Ромашка & Ко»", "ОсОО «Ромашка»", "123", "456", "789", "г. Бишкек",
		"-", "-", "-", "Банк", "4080", "0464", "TCLIENT"}
	for _, a := range answers[:len(answers)-1] {
		h.text(a)
	}
	h.expect(h.text(answers[len(answers)-1]), "Резидент или нерезидент")
	h.expect(h.click("resident:Нерезидент"), "Компания добавлена")
	h.expect(h.click("view_company:1"), "Ромашка &amp; Ко")

	// Правка поля
	h.click("edit_company_field:1:inn")
	h.expect(h.text("999"), "Поле обновлено")
	if c, _ := h.st.Company(context.Background(), 1); c.INN != "999" || c.Resident != "Нерезидент" {
		t.Fatalf("company %+v", c)
	}

	// ─── Сделка: создание → счёт → оплата → USDT (тест + остаток) → акт ───
	h.expect(h.text(navCreateAct), "Выберите тип операции")
	h.expect(h.click("act_type:sell"), "sel_company:1")
	h.expect(h.click("sel_company:1"), "Использовать №500")
	h.expect(h.click("skip:deal_number"), "Дата заявки")
	h.expect(h.text("12.04.2026"), "Тип виртуального актива")
	h.expect(h.click("va_type:USDT"), "Сеть")
	h.expect(h.click("network:TRC-20"), "Сумма виртуального актива")
	h.expect(h.text("6287,726"), "Сумма в рублях")
	h.expect(h.text("500 000"), "79,52")
	h.expect(h.click("skip:exchange_rate"), "КВВО")
	h.expect(h.text("12345"), "Кошелёк клиента") // свой КВВО сохраняется
	h.expect(h.click("skip:client_wallet"), "Наш кошелёк")
	h.expect(h.click("skip:operator_wallet"), "Комиссия в RUB")
	h.click("skip:commission_fiat")
	h.expect(h.click("skip:commission_va"), "Дата исполнения")
	h.expect(h.click("skip:execution_date"), "✅ Создать сделку")
	h.click("act:edit_menu")
	h.click("edit_act_field:commission_fiat")
	h.expect(h.text("1.5%"), "1.5%")
	card := h.click("act:generate")
	h.expect(card, "Сделка создана")
	h.expect(card, "👉 🧾 Счёт на оплату")
	h.expect(card, "st:inv:1")

	// Счёт
	h.click("st:inv:1")
	if !h.tg.find("sendDocument", "invoice_500-") {
		t.Fatal("счёт не отправлен")
	}
	if !h.tg.find("sendMessage", "PDF не создан") {
		t.Fatal("нет предупреждения о PDF")
	}
	h.expect(h.tg.last(), "👉 💰 Оплата ₽ получена")

	// Оплата: другая сумма и дата
	h.click("st:paid:1")
	h.text("499 000")
	h.expect(h.text("12.04.2026"), "Оплата ₽ получена — 499 000 ₽ · 12.04.2026")

	// USDT: тест + остаток
	h.expect(h.click("st:usdt:1"), "Сумма: <b>6 287,73 USDT</b>")
	h.expect(h.click("tx:split"), "Остаток: 6287.73")
	h.click("tx:add")
	h.expect(h.text("7000"), "меньше остатка")
	h.text("100")
	h.expect(h.text("hash_test"), "Остаток: 6187.73")
	h.click("tx:done")
	h.expect(h.text("hash_rest"), "Сегодня")
	h.expect(h.click("st:usdttoday:1"), "👉 📄 Акт")

	// Акт → завершена
	h.click("st:act:1")
	if !h.tg.find("sendDocument", "act_buy_500-") {
		t.Fatal("акт не отправлен")
	}
	h.expect(h.tg.last(), "Сделка завершена")

	ctx := context.Background()
	d, err := h.st.Deal(ctx, 1)
	if err != nil || d == nil {
		t.Fatal("сделка не в журнале", err)
	}
	if d.DealDateISO != "2026-04-12" || d.VAAmount != 6287.726 || d.CommissionFiat != "1.5%" ||
		!strings.Contains(d.Transactions, "hash_rest") || d.DocxFileID == "" || d.InvoiceDocxFileID == "" ||
		!d.Completed() || d.PaidAmount != 499000 || d.PaidAt != "2026-04-12" {
		t.Fatalf("deal %+v", d)
	}
	if n, ok, _ := h.st.PeekCompanyNumber(ctx, 1, "act"); !ok || n != 501 {
		t.Fatalf("номер клиента: %d %v", n, ok)
	}
	if codes, _ := h.st.KVVOCodes(ctx); len(codes) != 1 || codes[0] != "12345" {
		t.Fatalf("kvvo %v", codes)
	}
	files, _ := os.ReadDir(h.app.cfg.OutputDir)
	if len(files) != 0 {
		t.Errorf("локальные файлы не удалены: %v", files)
	}

	// ─── Журнал и подписанные ───
	h.expect(h.click("deal:1"), "Транзакций:</b> 2")
	h.click("deal_attach:1")
	h.expect(h.doc("signed.pdf"), "Сохранено (1 шт.)")
	h.expect(h.click("signed:done"), "прикреплено документов — 1")
	h.click("arch:1")
	if !h.tg.find("sendDocument", "Подписанные_") {
		t.Fatal("ZIP не отправлен")
	}

	// ─── Отчёт ФН ───
	h.expect(h.text(navReport), "report_month:2026-04")
	h.click("report_month:2026-04")
	h.text("1.2072")
	if !h.tg.find("sendDocument", "ФН_Апрель_2026.xlsx") {
		t.Fatal("отчёт не отправлен")
	}
	if s, _ := h.st.Settings(ctx); s["kgs_rate"] != "1.2072" {
		t.Errorf("kgs_rate %q", s["kgs_rate"])
	}

	// ─── Шаблоны и переменные ───
	h.click("vars:add")
	h.text("contract_number")
	h.expect(h.text("Номер договора"), "{{CL_CONTRACT_NUMBER}}")
	h.click("tpl:upload:sell")
	h.expect(h.doc("new.docx"), "Плейсхолдеры")
	backups, _ := os.ReadDir(filepath.Join(h.app.cfg.TemplatesDir, "backups"))
	if len(backups) != 1 {
		t.Errorf("нет резервной копии шаблона: %v", backups)
	}

	// ─── Реквизиты ───
	h.click("setting:wallet")
	h.expect(h.text("TNEWWALLET"), "TNEWWALLET")

	// Посторонний пользователь игнорируется
	before := len(h.tg.calls)
	h.app.handle(ctx, nil, &models.Update{Message: &models.Message{Text: "/start", Chat: models.Chat{ID: 2}, From: &models.User{ID: 2}}})
	// посторонний получает только «нет доступа» со своим ID — без меню и данных
	if n := len(h.tg.calls) - before; n != 1 || !strings.Contains(h.tg.last().Text, "нет доступа") {
		t.Errorf("ответ постороннему: %d сообщений, %q", n, h.tg.last().Text)
	}
}
