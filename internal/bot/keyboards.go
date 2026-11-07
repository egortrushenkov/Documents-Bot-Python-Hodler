package bot

import (
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"

	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/util"
)

type kbRow = []models.InlineKeyboardButton

type btnT = models.InlineKeyboardButton

func btn(text, data string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: text, CallbackData: data}
}

func row(b ...models.InlineKeyboardButton) kbRow { return b }

func ikb(rows ...kbRow) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// grid раскладывает кнопки по n в ряд (как builder.adjust(n)).
func grid(n int, btns ...models.InlineKeyboardButton) []kbRow {
	var rows []kbRow
	for i := 0; i < len(btns); i += n {
		end := min(i+n, len(btns))
		rows = append(rows, append(kbRow{}, btns[i:end]...))
	}
	return rows
}

// ─── Постоянная клавиатура навигации ──────────────────────────────────────────

const (
	navCreateAct = "📋 Создать акт"
	navDeals     = "🗂 Сделки"
	navReport    = "📊 Отчёт ФН"
	navCompanies = "🏢 Компании"
	navSettings  = "⚙️ Реквизиты"
	navMenu      = "🏠 Меню"
	navBuybacks  = "🔁 Откупки"
	navProfit    = "📈 Прибыль"
	navTreasury  = "💰 Остатки"
)

func navKB() *models.ReplyKeyboardMarkup {
	k := func(t string) models.KeyboardButton { return models.KeyboardButton{Text: t} }
	return &models.ReplyKeyboardMarkup{
		Keyboard: [][]models.KeyboardButton{
			{k(navCreateAct), k(navDeals)},
			{k(navBuybacks), k(navProfit), k(navTreasury)},
			{k(navReport), k(navCompanies)},
			{k(navSettings), k(navMenu)},
		},
		ResizeKeyboard: true,
		IsPersistent:   true,
	}
}

// ─── Главное меню ─────────────────────────────────────────────────────────────

func mainMenuKB() *models.InlineKeyboardMarkup {
	return ikb(
		row(btn("📋 Создать акт", "menu:create_act"), btn("🏢 Компании", "menu:companies")),
		row(btn("🗂 Сделки", "deals:menu"), btn("🔁 Откупки", "bb:menu")),
		row(btn("📈 Прибыль", "profit:menu"), btn("💰 Остатки", "tr:menu")),
		row(btn("📊 Отчёт ФН", "report:menu")),
		row(btn("👥 Рефералы", "ref:menu"), btn("🔔 Уведомления", "notify:menu")),
		row(btn("📄 Шаблоны", "tpl:menu"), btn("⚙️ Реквизиты", "menu:settings")),
	)
}

func backToMainKB() *models.InlineKeyboardMarkup {
	return ikb(row(btn("🏠 Главное меню", "menu:main")))
}

// ─── Компании ─────────────────────────────────────────────────────────────────

func companiesListKB(cs []*storage.Company, selectMode bool) *models.InlineKeyboardMarkup {
	var rows []kbRow
	for _, c := range cs {
		data := fmt.Sprintf("view_company:%d", c.ID)
		if selectMode {
			data = fmt.Sprintf("sel_company:%d", c.ID)
		}
		rows = append(rows, row(btn("🏢 "+c.ShortName, data)))
	}
	rows = append(rows, row(btn("➕ Добавить компанию", "add_company")))
	rows = append(rows, row(btn("🏠 Главное меню", "menu:main")))
	return ikb(rows...)
}

func companyCardKB(id int64) *models.InlineKeyboardMarkup {
	return ikb(
		row(btn("✏️ Редактировать", fmt.Sprintf("edit_company:%d", id)),
			btn("🗑 Удалить", fmt.Sprintf("del_company:%d", id))),
		row(btn("◀️ Назад к списку", "menu:companies")),
	)
}

var companyEditFields = [][2]string{
	{"full_name", "Полное наименование"},
	{"short_name", "Краткое наименование"},
	{"inn", "ИНН"},
	{"kpp", "КПП"},
	{"reg_number", "Рег. номер (ОГРН/КР)"},
	{"address", "Юр. адрес"},
	{"kio", "КИО"},
	{"inn_rf", "ИНН в РФ"},
	{"kpp_rf", "КПП в РФ"},
	{"bank_name", "Банк"},
	{"bank_account", "Р/счёт"},
	{"bank_bik", "БИК"},
	{"wallet", "Кошелёк (крипто)"},
	{"resident", "Резидент/Нерезидент КР"},
	{"referrer", "👥 Реферал"},
	{"referral_percent", "👥 % реферала для клиента"},
}

func editCompanyFieldsKB(id int64, custom []storage.CustomField) *models.InlineKeyboardMarkup {
	var rows []kbRow
	for _, f := range companyEditFields {
		rows = append(rows, row(btn("✏️ "+f[1], fmt.Sprintf("edit_company_field:%d:%s", id, f[0]))))
	}
	for _, f := range custom {
		rows = append(rows, row(btn("🧩 "+f.Label, fmt.Sprintf("edit_company_field:%d:c__%s", id, f.Key))))
	}
	rows = append(rows, row(btn("◀️ Назад", fmt.Sprintf("view_company:%d", id))))
	return ikb(rows...)
}

func confirmDeleteKB(id int64) *models.InlineKeyboardMarkup {
	return ikb(row(
		btn("✅ Да, удалить", fmt.Sprintf("confirm_del:%d", id)),
		btn("❌ Отмена", fmt.Sprintf("view_company:%d", id)),
	))
}

func residentKB() *models.InlineKeyboardMarkup {
	return ikb(row(
		btn("🇰🇬 Резидент", "resident:Резидент"),
		btn("🌍 Нерезидент", "resident:Нерезидент"),
	))
}

// ─── Создание акта ────────────────────────────────────────────────────────────

func actTypeKB() *models.InlineKeyboardMarkup {
	return ikb(
		row(btn("📤 Продаём ВА клиенту (покупка у нас)", "act_type:sell")),
		row(btn("📥 Покупаем ВА у клиента (продажа нам)", "act_type:buy")),
		row(btn("🧾 Счёт-заявка на покупку клиентом", "act_type:invoice")),
		row(btn("❌ Отмена", "menu:main")),
	)
}

func splitModeKB() *models.InlineKeyboardMarkup {
	return ikb(
		row(btn("📦 Одной транзакцией", "tx:single")),
		row(btn("✂️ Разбить (тест + остаток)", "tx:split")),
	)
}

func addTxKB(remainder string) *models.InlineKeyboardMarkup {
	return ikb(
		row(btn("➕ Добавить тест-транзакцию", "tx:add")),
		row(btn(fmt.Sprintf("✅ Завершить (остаток %s)", remainder), "tx:done")),
	)
}

func vaTypeKB() *models.InlineKeyboardMarkup {
	var b []models.InlineKeyboardButton
	for _, v := range []string{"USDT", "BTC", "ETH", "USDC", "BNB", "TRX"} {
		b = append(b, btn(v, "va_type:"+v))
	}
	b = append(b, btn("✏️ Другой", "va_type:custom"))
	return ikb(grid(3, b...)...)
}

func networkKB() *models.InlineKeyboardMarkup {
	var b []models.InlineKeyboardButton
	for _, v := range []string{"TRC-20", "ERC-20", "BEP-20", "SOL", "TON", "BTC"} {
		b = append(b, btn(v, "network:"+v))
	}
	b = append(b, btn("✏️ Другая", "network:custom"))
	return ikb(grid(3, b...)...)
}

var builtinKVVO = [][2]string{
	{"99082", "99082 — прочие операции с ВА"},
	{"99081", "99081 — обмен ВА"},
	{"10100", "10100 — покупка ин. валюты"},
	{"20100", "20100 — продажа ин. валюты"},
}

func isBuiltinKVVO(code string) bool {
	for _, k := range builtinKVVO {
		if k[0] == code {
			return true
		}
	}
	return false
}

func kvvoKB(custom []string) *models.InlineKeyboardMarkup {
	var rows []kbRow
	for _, k := range builtinKVVO {
		rows = append(rows, row(btn(k[1], "kvvo:"+k[0])))
	}
	for _, c := range custom {
		rows = append(rows, row(btn(c, "kvvo:"+c)))
	}
	rows = append(rows, row(btn("✏️ Ввести другой", "kvvo:custom")))
	if len(custom) > 0 {
		rows = append(rows, row(btn("🗑 Удалить свой КВВО", "kvvo:manage")))
	}
	return ikb(rows...)
}

func kvvoManageKB(custom []string) *models.InlineKeyboardMarkup {
	var rows []kbRow
	for _, c := range custom {
		rows = append(rows, row(btn("🗑 "+c, "kvvo_del:"+c)))
	}
	rows = append(rows, row(btn("◀️ Назад к выбору", "kvvo:back")))
	return ikb(rows...)
}

func skipKB(data, label string) *models.InlineKeyboardMarkup {
	return ikb(row(btn("⏩ "+label, data)))
}

func confirmActKB() *models.InlineKeyboardMarkup {
	return ikb(
		row(btn("✅ Генерировать документы", "act:generate")),
		row(btn("✏️ Редактировать поля", "act:edit_menu")),
		row(btn("❌ Отмена", "menu:main")),
	)
}

var actEditFields = [][2]string{
	{"deal_number", "Номер заявки"},
	{"deal_date", "Дата заявки"},
	{"va_type", "Тип ВА"},
	{"network", "Сеть"},
	{"va_amount", "Сумма ВА"},
	{"fiat_amount", "Сумма RUB"},
	{"exchange_rate", "Курс"},
	{"kvvo", "КВВО"},
	{"client_wallet", "Кошелёк клиента"},
	{"operator_wallet", "Кошелёк оператора"},
	{"tx_hash", "Хэш транзакции"},
	{"commission_fiat", "Комиссия в RUB"},
	{"commission_va", "Комиссия в ВА"},
	{"execution_date", "Дата исполнения"},
}

func editActFieldsKB() *models.InlineKeyboardMarkup {
	var rows []kbRow
	for _, f := range actEditFields {
		rows = append(rows, row(btn("✏️ "+f[1], "edit_act_field:"+f[0])))
	}
	rows = append(rows, row(btn("◀️ Назад к подтверждению", "act:confirm_show")))
	return ikb(rows...)
}

func afterGenerateKB(dealID int64) *models.InlineKeyboardMarkup {
	return ikb(
		row(btn("📎 Прикрепить подписанный акт", fmt.Sprintf("deal_attach:%d", dealID))),
		row(btn("📋 Новый акт", "menu:create_act")),
		row(btn("🏠 Главное меню", "menu:main")),
	)
}

// ─── Реквизиты оператора ──────────────────────────────────────────────────────

var settingsFields = [][2]string{
	{"full_name", "Полное наименование"},
	{"short_name", "Краткое наименование"},
	{"inn", "ИНН"},
	{"kpp", "КПП"},
	{"address", "Юр. адрес (КР)"},
	{"legal_address", "Адрес для документов"},
	{"license", "Лицензия"},
	{"kio", "КИО"},
	{"inn_rf", "ИНН в РФ"},
	{"kpp_rf", "КПП в РФ"},
	{"bank_name", "Банк"},
	{"bank_account", "Р/счёт"},
	{"bank_bik", "БИК"},
	{"wallet", "Кошелёк оператора"},
	{"director_name", "ФИО директора"},
	{"director_title", "Должность директора"},
	{"tax_percent", "📈 Налог, % от ЧКД"},
	{"bank_fee_percent", "📈 Комиссия банка, %"},
	{"report_title", "📈 Название отчёта"},
}

func settingsKB() *models.InlineKeyboardMarkup {
	var rows []kbRow
	for _, f := range settingsFields {
		rows = append(rows, row(btn("✏️ "+f[1], "setting:"+f[0])))
	}
	rows = append(rows, row(btn("🏠 Главное меню", "menu:main")))
	return ikb(rows...)
}

// ─── Журнал сделок ────────────────────────────────────────────────────────────

func dealsMenuKB() *models.InlineKeyboardMarkup {
	return ikb(
		row(btn("🕐 Последние сделки", "deals:recent")),
		row(btn("🏢 Сделки по клиенту", "deals:by_company")),
		row(btn("📦 Архив подписанных (ZIP)", "arch:menu")),
		row(btn("🏠 Главное меню", "menu:main")),
	)
}

var actIcons = map[string]string{"sell": "📤", "buy": "📥", "invoice": "🧾"}

func dealsListKB(deals []*storage.Deal) *models.InlineKeyboardMarkup {
	var rows []kbRow
	for _, d := range deals {
		icon := actIcons[d.ActType]
		if icon == "" {
			icon = "📄"
		}
		if d.Cancelled() {
			icon = "❌"
		}
		text := fmt.Sprintf("%s №%s · %s · %s", icon, d.DealNumber, d.DealDate, d.ClientName)
		rows = append(rows, row(btn(util.TruncRunes(text, 60), fmt.Sprintf("deal:%d", d.ID))))
	}
	rows = append(rows, row(btn("◀️ Назад", "deals:menu")))
	return ikb(rows...)
}

// dealCardKB2 — карточка сделки с откупкой, переделкой и отменой.
func dealCardKB2(d *storage.Deal, hasFiles bool, signed int) *models.InlineKeyboardMarkup {
	id := d.ID
	var rows []kbRow
	if hasFiles {
		rows = append(rows, row(btn("📄 Прислать документы", fmt.Sprintf("deal_docs:%d", id))))
	}
	if !d.Cancelled() {
		if d.ActType == "sell" {
			if d.BuybackID.Valid && d.BuyRate > 0 {
				rows = append(rows, row(btn(fmt.Sprintf("🔁 Откупка №%d", d.BuybackID.Int64), fmt.Sprintf("bb:view:%d", d.BuybackID.Int64))))
			} else {
				rows = append(rows, row(btn("🔁 Закрыть откупкой", fmt.Sprintf("dealx:bb:%d", id))))
			}
		}
		rows = append(rows, row(btn("✏️ Переделать заявку", fmt.Sprintf("dealx:edit:%d", id))))
		label := "📎 Прикрепить подписанный"
		if signed > 0 {
			label = fmt.Sprintf("📎 Прикрепить подписанный (%d)", signed)
		}
		rows = append(rows, row(btn(label, fmt.Sprintf("deal_attach:%d", id))))
	}
	if signed > 0 {
		rows = append(rows, row(btn("📥 Прислать подписанные", fmt.Sprintf("deal_signed:%d", id))))
	}
	if d.Cancelled() {
		rows = append(rows, row(btn("↩️ Восстановить", fmt.Sprintf("dealx:restore:%d", id))),
			row(btn("🗑 Удалить из журнала", fmt.Sprintf("deal_del:%d", id))))
	} else {
		rows = append(rows, row(btn("❌ Отменить сделку", fmt.Sprintf("dealx:cancel:%d", id))))
	}
	rows = append(rows, row(btn("◀️ Назад", "deals:menu")))
	return ikb(rows...)
}

func confirmDealDeleteKB(id int64) *models.InlineKeyboardMarkup {
	return ikb(row(
		btn("✅ Да, удалить", fmt.Sprintf("deal_del_yes:%d", id)),
		btn("❌ Отмена", fmt.Sprintf("deal:%d", id)),
	))
}

func companyPickKB(cs []*storage.Company, prefix, back string) *models.InlineKeyboardMarkup {
	var rows []kbRow
	for _, c := range cs {
		rows = append(rows, row(btn("🏢 "+c.ShortName, fmt.Sprintf("%s:%d", prefix, c.ID))))
	}
	rows = append(rows, row(btn("◀️ Назад", back)))
	return ikb(rows...)
}

func signedDoneKB() *models.InlineKeyboardMarkup {
	return ikb(row(btn("✅ Готово", "signed:done")))
}

// ─── Отчёт ФН ─────────────────────────────────────────────────────────────────

func reportMonthsKB(months []string, label func(string) string) *models.InlineKeyboardMarkup {
	var rows []kbRow
	for _, m := range months {
		rows = append(rows, row(btn("📅 "+label(m), "report_month:"+m)))
	}
	rows = append(rows, row(btn("🏠 Главное меню", "menu:main")))
	return ikb(rows...)
}

func reportRateKB(saved string) *models.InlineKeyboardMarkup {
	var rows []kbRow
	if saved != "" {
		rows = append(rows, row(btn("⏩ Использовать "+saved, "report_rate:saved")))
	}
	rows = append(rows, row(btn("⏭ Без пересчёта в сомы", "report_rate:skip")))
	return ikb(rows...)
}

// ─── Шаблоны и переменные ─────────────────────────────────────────────────────

var templateKinds = [][2]string{
	{"sell", "📤 Акт: продаём ВА клиенту"},
	{"buy", "📥 Акт: покупаем ВА у клиента"},
	{"invoice", "🧾 Счёт-заявка на покупку"},
}

func templateKindLabel(kind string) string {
	for _, k := range templateKinds {
		if k[0] == kind {
			return k[1]
		}
	}
	return kind
}

func templatesMenuKB() *models.InlineKeyboardMarkup {
	var rows []kbRow
	for _, k := range templateKinds {
		rows = append(rows, row(btn(k[1], "tpl:view:"+k[0])))
	}
	rows = append(rows, row(btn("🧩 Свои переменные", "vars:menu")))
	rows = append(rows, row(btn("🏠 Главное меню", "menu:main")))
	return ikb(rows...)
}

func templateCardKB(kind string) *models.InlineKeyboardMarkup {
	return ikb(
		row(btn("⬆️ Загрузить новый .docx", "tpl:upload:"+kind)),
		row(btn("📄 Скачать текущий", "tpl:download:"+kind)),
		row(btn("◀️ Назад", "tpl:menu")),
	)
}

func varsMenuKB(fields []storage.CustomField) *models.InlineKeyboardMarkup {
	var rows []kbRow
	for _, f := range fields {
		text := fmt.Sprintf("🗑 {{CL_%s}} — %s", strings.ToUpper(f.Key), f.Label)
		rows = append(rows, row(btn(util.TruncRunes(text, 60), "vars:del:"+f.Key)))
	}
	rows = append(rows, row(btn("➕ Добавить переменную", "vars:add")))
	rows = append(rows, row(btn("◀️ Назад к шаблонам", "tpl:menu")))
	return ikb(rows...)
}
