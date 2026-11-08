// Package bot — Telegram-интерфейс: FSM-диалоги, меню, обработчики.
package bot

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	tg "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"hodler-doc-bot/internal/chain"
	"hodler-doc-bot/internal/config"
	"hodler-doc-bot/internal/docgen"
	"hodler-doc-bot/internal/pdf"
	"hodler-doc-bot/internal/storage"
)

type App struct {
	cfg   *config.Config
	st    *storage.Store
	pdf   *pdf.Converter
	b     *tg.Bot
	chain *chain.Client

	mu       sync.Mutex
	sessions map[string]*Session

	http *http.Client
}

// Session — состояние диалога одного пользователя (аналог FSMContext).
type Session struct {
	mu    sync.Mutex
	State string
	Data  map[string]string // мелкие временные значения
	Act   *docgen.Draft     // черновик акта

	CompanyData      map[string]string
	CustomDefs       []storage.CustomField
	CustomDefsLoaded bool
	CustomIndex      int
	CustomValues     map[string]string
}

func (s *Session) Clear() {
	s.State = ""
	s.Data = map[string]string{}
	s.Act = nil
	s.CompanyData = nil
	s.CustomDefs = nil
	s.CustomDefsLoaded = false
	s.CustomIndex = 0
	s.CustomValues = nil
}

func New(cfg *config.Config, st *storage.Store, opts ...tg.Option) (*App, error) {
	a := &App{
		cfg:      cfg,
		st:       st,
		pdf:      pdf.New(cfg.LibreOfficePath, cfg.PDFConcurrency),
		chain:    chain.New(cfg.TronGridKey, cfg.EtherscanKey),
		sessions: map[string]*Session{},
		http:     &http.Client{Timeout: 5 * time.Minute},
	}
	options := append([]tg.Option{
		tg.WithDefaultHandler(a.handle),
		tg.WithAllowedUpdates(tg.AllowedUpdates{"message", "callback_query"}),
		tg.WithErrorsHandler(func(err error) { slog.Error("telegram", "err", err) }),
		tg.WithHTTPClient(time.Minute, &http.Client{Timeout: 3 * time.Minute}),
	}, opts...)
	b, err := tg.New(cfg.BotToken, options...)
	if err != nil {
		return nil, err
	}
	a.b = b
	return a, nil
}

// Run запускает long polling до отмены ctx.
func (a *App) Run(ctx context.Context) {
	go a.watchLoop(ctx)
	a.b.Start(ctx)
}

func (a *App) session(chatID, userID int64) *Session {
	key := fmt.Sprintf("%d:%d", chatID, userID)
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[key]
	if !ok {
		s = &Session{Data: map[string]string{}}
		a.sessions[key] = s
	}
	return s
}

// Ctx — контекст обработки одного апдейта.
type Ctx struct {
	context.Context
	app      *App
	s        *Session
	chatID   int64
	userID   int64
	msg      *models.Message       // входящее сообщение
	cb       *models.CallbackQuery // нажатая кнопка
	cbMsgID  int
	answered bool
	userName string // для «кто создал сделку»
	chatType models.ChatType
	thread   int // ветка (тема) чата, откуда пришло сообщение
}

func (a *App) handle(ctx context.Context, _ *tg.Bot, upd *models.Update) {
	var chatID, userID int64
	var c *Ctx
	switch {
	case upd.Message != nil && upd.Message.From != nil:
		chatID, userID = upd.Message.Chat.ID, upd.Message.From.ID
		c = &Ctx{msg: upd.Message, chatType: upd.Message.Chat.Type, userName: displayName(upd.Message.From)}
		// Ветка форума: берём номер, если сообщение из темы или чат — форум
		// (в некоторых клиентах is_topic_message не приходит)
		if m := upd.Message; m.MessageThreadID != 0 && (m.IsTopicMessage || m.Chat.IsForum) {
			c.thread = m.MessageThreadID
		}
	case upd.CallbackQuery != nil:
		userID = upd.CallbackQuery.From.ID
		if m := upd.CallbackQuery.Message.Message; m != nil {
			chatID = m.Chat.ID
			c = &Ctx{cb: upd.CallbackQuery, cbMsgID: m.ID, chatType: m.Chat.Type, userName: displayName(&upd.CallbackQuery.From)}
		} else if im := upd.CallbackQuery.Message.InaccessibleMessage; im != nil {
			chatID = im.Chat.ID
			c = &Ctx{cb: upd.CallbackQuery, cbMsgID: im.MessageID}
		} else {
			return
		}
	default:
		return
	}
	// Группы (чат руководителей): только команды /bind, /unbind, /today, /report
	if c.chatType != "" && c.chatType != models.ChatTypePrivate {
		if c.msg != nil {
			c.Context, c.app, c.chatID, c.userID = ctx, a, chatID, userID
			c.s = &Session{Data: map[string]string{}}
			a.onGroupMessage(c, len(a.cfg.AdminIDs) == 0 || a.cfg.AdminIDs[userID])
		}
		return
	}
	// Если задан ADMIN_IDS — бот отвечает только этим пользователям (остальных тихо игнорируем).
	if len(a.cfg.AdminIDs) > 0 && !a.cfg.AdminIDs[userID] {
		return
	}

	s := a.session(chatID, userID)
	s.mu.Lock() // апдейты одного пользователя — строго по очереди
	defer s.mu.Unlock()

	c.Context, c.app, c.s, c.chatID, c.userID = ctx, a, s, chatID, userID
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic в обработчике", "err", r, "stack", string(debug.Stack()))
			c.Send("❌ Внутренняя ошибка. Попробуйте ещё раз или /menu", nil)
		}
		if c.cb != nil && !c.answered {
			c.Answer("", false)
		}
	}()
	if c.msg != nil {
		a.onMessage(c)
	} else {
		a.onCallback(c)
	}
}

func displayName(u *models.User) string {
	if u == nil {
		return ""
	}
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if u.Username != "" {
		if name == "" {
			return "@" + u.Username
		}
		return name + " (@" + u.Username + ")"
	}
	return name
}

// ─── Отправка сообщений ───────────────────────────────────────────────────────

func markup(kb models.ReplyMarkup) models.ReplyMarkup {
	switch v := kb.(type) {
	case *models.InlineKeyboardMarkup:
		if v == nil {
			return nil
		}
	case *models.ReplyKeyboardMarkup:
		if v == nil {
			return nil
		}
	}
	return kb
}

func (c *Ctx) send(text string, kb models.ReplyMarkup, html bool) *models.Message {
	p := &tg.SendMessageParams{ChatID: c.chatID, Text: text, ReplyMarkup: markup(kb), MessageThreadID: c.thread}
	if html {
		p.ParseMode = models.ParseModeHTML
	}
	m, err := c.app.b.SendMessage(c, p)
	if err != nil {
		slog.Error("sendMessage", "err", err)
	}
	return m
}

// Send — новое сообщение (HTML).
func (c *Ctx) Send(text string, kb models.ReplyMarkup) *models.Message { return c.send(text, kb, true) }

// SendPlain — новое сообщение без разметки.
func (c *Ctx) SendPlain(text string, kb models.ReplyMarkup) *models.Message {
	return c.send(text, kb, false)
}

func (c *Ctx) edit(text string, kb models.ReplyMarkup, html bool) {
	if c.cb == nil {
		c.send(text, kb, html)
		return
	}
	p := &tg.EditMessageTextParams{ChatID: c.chatID, MessageID: c.cbMsgID, Text: text, ReplyMarkup: markup(kb)}
	if html {
		p.ParseMode = models.ParseModeHTML
	}
	if _, err := c.app.b.EditMessageText(c, p); err != nil {
		if strings.Contains(err.Error(), "message is not modified") {
			return
		}
		// например, сообщение-документ или слишком старое — отправляем новое
		c.send(text, kb, html)
	}
}

// Edit — изменить сообщение с нажатой кнопкой (HTML); вне callback — отправить новое.
func (c *Ctx) Edit(text string, kb models.ReplyMarkup) { c.edit(text, kb, true) }

// EditPlain — как Edit, без разметки.
func (c *Ctx) EditPlain(text string, kb models.ReplyMarkup) { c.edit(text, kb, false) }

// Reply — Edit для кнопок, Send для сообщений (аналог answer_fn в Python-версии).
func (c *Ctx) Reply(text string, kb models.ReplyMarkup) {
	if c.cb != nil {
		c.Edit(text, kb)
	} else {
		c.Send(text, kb)
	}
}

func (c *Ctx) ReplyPlain(text string, kb models.ReplyMarkup) {
	if c.cb != nil {
		c.EditPlain(text, kb)
	} else {
		c.SendPlain(text, kb)
	}
}

// Answer отвечает на callback (один раз).
func (c *Ctx) Answer(text string, alert bool) {
	if c.cb == nil || c.answered {
		return
	}
	c.answered = true
	if _, err := c.app.b.AnswerCallbackQuery(c, &tg.AnswerCallbackQueryParams{
		CallbackQueryID: c.cb.ID, Text: text, ShowAlert: alert,
	}); err != nil {
		slog.Debug("answerCallbackQuery", "err", err)
	}
}

func (c *Ctx) Delete(msgID int) {
	_, _ = c.app.b.DeleteMessage(c, &tg.DeleteMessageParams{ChatID: c.chatID, MessageID: msgID})
}

// SendFile загружает локальный файл документом.
func (c *Ctx) SendFile(path, filename, caption string, html bool) (*models.Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	p := &tg.SendDocumentParams{
		ChatID:   c.chatID,
		Document: &models.InputFileUpload{Filename: filename, Data: f},
		Caption:  caption,
	}
	if html {
		p.ParseMode = models.ParseModeHTML
	}
	return c.app.b.SendDocument(c, p)
}

// SendFileID пересылает документ по file_id (хранится в Telegram).
func (c *Ctx) SendFileID(fileID, caption string) error {
	_, err := c.app.b.SendDocument(c, &tg.SendDocumentParams{
		ChatID: c.chatID, Document: &models.InputFileString{Data: fileID}, Caption: caption,
	})
	return err
}

func (c *Ctx) SendPhotoID(fileID, caption string) error {
	_, err := c.app.b.SendPhoto(c, &tg.SendPhotoParams{
		ChatID: c.chatID, Photo: &models.InputFileString{Data: fileID}, Caption: caption,
	})
	return err
}

// Download скачивает файл Telegram по file_id в dst.
func (c *Ctx) Download(fileID, dst string) error {
	f, err := c.app.b.GetFile(c, &tg.GetFileParams{FileID: fileID})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(c, http.MethodGet, c.app.b.FileDownloadLink(f), nil)
	if err != nil {
		return err
	}
	resp, err := c.app.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("скачивание файла: HTTP %d", resp.StatusCode)
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

// fail — логирует ошибку и сообщает пользователю.
func (c *Ctx) fail(err error) {
	slog.Error("ошибка обработчика", "err", err)
	c.Send("❌ Ошибка: <code>"+escape(err.Error())+"</code>", backToMainKB())
}

// ─── Маршрутизация ────────────────────────────────────────────────────────────

func (a *App) onMessage(c *Ctx) {
	text := strings.TrimSpace(c.msg.Text)

	// Команды и кнопки навигации работают из любого состояния.
	if strings.HasPrefix(text, "/") {
		cmd := strings.SplitN(strings.Fields(text)[0], "@", 2)[0]
		switch cmd {
		case "/start", "/menu", "/new":
			a.cmdStart(c)
			return
		}
	}
	switch text {
	case navMenu:
		a.cmdStart(c)
		return
	case navCreateAct, navCreateOld:
		a.startCreateAct(c)
		return
	case navDeals:
		c.s.Clear()
		a.showDealsMenu(c)
		return
	case navReport:
		c.s.Clear()
		a.showReportMonths(c)
		return
	case navCompanies:
		c.s.Clear()
		a.showCompanies(c)
		return
	case navSettings:
		c.s.Clear()
		a.showSettings(c, "")
		return
	case navBuybacks:
		c.s.Clear()
		a.showBuybacksMenu(c)
		return
	case navProfit:
		c.s.Clear()
		a.showProfitMenu(c)
		return
	case navTreasury:
		c.s.Clear()
		a.showTreasury(c, "")
		return
	}

	state := c.s.State
	switch {
	case state == stSignedWaiting:
		a.signedInput(c)
	case state == stTplWaiting:
		a.tplUploadFile(c)
	case state == "":
		// вне диалога — подскажем, где меню
		if text != "" {
			c.Send("Выберите действие в меню 👇 (или /menu)", nil)
		}
	case c.msg.Text == "":
		c.Send("Пришлите, пожалуйста, текстом.", nil)
	case state == stSettingsEditing:
		a.saveSetting(c, text)
	case strings.HasPrefix(state, "company:"):
		a.companyMessage(c, text)
	case strings.HasPrefix(state, "act:"):
		a.actMessage(c, text)
	case state == stReportRate:
		a.reportRateInput(c, text)
	case state == stVarKey:
		a.varsKeyInput(c, text)
	case state == stVarLabel:
		a.varsLabelInput(c, text)
	case strings.HasPrefix(state, "bb:"):
		a.buybackMessage(c, text)
	case strings.HasPrefix(state, "deal:"):
		a.stageMessage(c, text)
	case strings.HasPrefix(state, "acc:"), strings.HasPrefix(state, "wal:"):
		a.accountsMessage(c, text)
	case strings.HasPrefix(state, "tr:"):
		a.treasuryMessage(c, text)
	case strings.HasPrefix(state, "ref:"):
		a.referrerMessage(c, text)
	case state == stProfitDate:
		a.profitDateInput(c, text)
	}
}

func (a *App) onCallback(c *Ctx) {
	data := c.cb.Data
	head, rest, _ := strings.Cut(data, ":")
	switch head {
	case "menu":
		switch rest {
		case "main":
			c.s.Clear()
			c.Edit(welcomeText, mainMenuKB())
		case "create_act":
			a.startCreateAct(c)
		case "companies":
			c.s.Clear()
			a.showCompanies(c)
		case "settings":
			c.s.Clear()
			a.showSettings(c, "")
		}
	case "setting":
		a.editSetting(c, rest)
	case "add_company", "view_company", "del_company", "confirm_del", "edit_company",
		"edit_company_field", "resident":
		a.companyCallback(c, head, rest)
	case "act_type", "sel_company", "skip", "va_type", "network", "kvvo", "kvvo_del",
		"tx", "act", "edit_act_field", "cw", "ow", "cb", "ob":
		a.actCallback(c, head, rest)
	case "deals", "deals_c", "deal", "deal_docs", "deal_signed", "deal_del", "deal_del_yes",
		"deal_attach", "signed", "arch":
		a.dealsCallback(c, head, rest)
	case "report", "report_month", "report_rate":
		a.reportCallback(c, head, rest)
	case "tpl", "vars":
		a.templatesCallback(c, head, rest)
	case "bb":
		a.buybackCallback(c, rest)
	case "ref", "coref":
		a.referrerCallback(c, head, rest)
	case "profit":
		a.profitCallback(c, rest)
	case "tr":
		a.treasuryCallback(c, rest)
	case "acc", "wal":
		a.accountsCallback(c, head, rest)
	case "st":
		a.stageCallback(c, rest)
	case "notify":
		a.notifyCallback(c, rest)
	case "dealx":
		a.dealExtraCallback(c, rest)
	}
}
