package bot

import (
	"strconv"
	"strings"

	"hodler-doc-bot/internal/util"
)

// Состояния диалогов.
const (
	stSettingsEditing = "settings:editing"

	stCompanyInput    = "company:input"
	stCompanyResident = "company:resident"
	stCompanyCustom   = "company:custom"

	stActSelectType     = "act:select_type"
	stActSelectCompany  = "act:select_company"
	stActDealNumber     = "act:deal_number"
	stActDealDate       = "act:deal_date"
	stActVAType         = "act:va_type"
	stActNetwork        = "act:network"
	stActVAAmount       = "act:va_amount"
	stActFiatAmount     = "act:fiat_amount"
	stActExchangeRate   = "act:exchange_rate"
	stActKVVO           = "act:kvvo"
	stActClientWallet   = "act:client_wallet"
	stActOperatorWallet = "act:operator_wallet"
	stActSplitMode      = "act:split_mode"
	stActTxOneHash      = "act:tx_one_hash"
	stActAddTx          = "act:add_tx"
	stActExtraVA        = "act:extra_va"
	stActExtraTxHash    = "act:extra_tx_hash"
	stActRemainderHash  = "act:remainder_hash"
	stActCommissionFiat = "act:commission_fiat"
	stActCommissionVA   = "act:commission_va"
	stActExecutionDate  = "act:execution_date"
	stActConfirm        = "act:confirm"
	stActEditField      = "act:edit_field"

	stReportRate    = "report:kgs_rate"
	stTplWaiting    = "tpl:waiting_file"
	stVarKey        = "var:key"
	stVarLabel      = "var:label"
	stSignedWaiting = "signed:waiting_files"
)

const welcomeText = "🏦 <b>Hodler Doc Generator</b>\n\n" +
	"Автоматическое формирование актов и договоров\n" +
	"для сделок с виртуальными активами.\n\n" +
	"Выберите действие:"

func escape(s string) string { return util.Esc(s) }

func parseID(s string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n, err == nil
}

func (a *App) cmdStart(c *Ctx) {
	c.s.Clear()
	c.Send(welcomeText, mainMenuKB())
	c.Send("⬇️ Быстрый доступ:", navKB())
}

// ─── Реквизиты оператора ──────────────────────────────────────────────────────

func settingLabel(key string) string {
	for _, f := range settingsFields {
		if f[0] == key {
			return f[1]
		}
	}
	return key
}

func settingsText(s map[string]string) string {
	g := func(k string) string {
		if v, ok := s[k]; ok {
			return escape(v)
		}
		return "—"
	}
	lines := []string{
		"<b>⚙️ Реквизиты оператора</b>\n",
		"<b>Наименование:</b> " + g("short_name"),
		"<b>Полное:</b> " + g("full_name"),
		"<b>ИНН:</b> " + g("inn") + "  <b>КПП:</b> " + g("kpp"),
		"<b>Адрес:</b> " + g("address"),
		"<b>Лицензия:</b> " + g("license"),
		"<b>КИО:</b> " + g("kio"),
		"<b>ИНН/КПП в РФ:</b> " + g("inn_rf") + " / " + g("kpp_rf"),
		"\n<b>Банк:</b> " + g("bank_name"),
		"<b>Р/счёт:</b> " + g("bank_account"),
		"<b>БИК:</b> " + g("bank_bik"),
		"\n<b>Кошелёк:</b> <code>" + g("wallet") + "</code>",
		"\n<b>Директор:</b> " + g("director_name") + " (" + g("director_title") + ")",
		"\n📈 <b>Прибыль:</b> налог " + g("tax_percent") + "% от ЧКД · комиссия банка " + g("bank_fee_percent") +
			"% · отчёт «" + g("report_title") + "»",
	}
	return strings.Join(lines, "\n")
}

func (a *App) showSettings(c *Ctx, prefix string) {
	s, err := a.st.Settings(c)
	if err != nil {
		c.fail(err)
		return
	}
	c.Reply(prefix+settingsText(s), settingsKB())
}

func (a *App) editSetting(c *Ctx, key string) {
	s, err := a.st.Settings(c)
	if err != nil {
		c.fail(err)
		return
	}
	c.s.Clear()
	c.s.State = stSettingsEditing
	c.s.Data["setting_key"] = key
	c.Edit("✏️ <b>"+escape(settingLabel(key))+"</b>\n\n"+
		"Текущее значение:\n<code>"+escape(s[key])+"</code>\n\n"+
		"Введите новое значение:", nil)
}

func (a *App) saveSetting(c *Ctx, text string) {
	key := c.s.Data["setting_key"]
	c.s.Clear()
	if key == "" {
		return
	}
	if err := a.st.SetSetting(c, key, text); err != nil {
		c.fail(err)
		return
	}
	a.showSettings(c, "✅ Сохранено!\n\n")
}
