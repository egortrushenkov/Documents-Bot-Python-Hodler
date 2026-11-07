package bot

import (
	"fmt"
	"strings"

	"hodler-doc-bot/internal/storage"
)

// Поля, которые бот спрашивает при добавлении компании: ключ, вопрос, пример.
var companyFields = [][3]string{
	{"full_name", "📋 Полное наименование", "ООО «Ромашка»"},
	{"short_name", "📋 Краткое наименование", "ОсОО «Ромашка»"},
	{"inn", "🔢 ИНН", "1234567890"},
	{"kpp", "🔢 КПП", "123456789"},
	{"reg_number", "📎 Рег. номер (ОГРН/КР)", "1234567890123"},
	{"address", "🏠 Юридический адрес", "г. Бишкек, ул. ..."},
	{"kio", "🌐 КИО (или - если нет)", "-"},
	{"inn_rf", "🇷🇺 ИНН в РФ (или -)", "-"},
	{"kpp_rf", "🇷🇺 КПП в РФ (или -)", "-"},
	{"bank_name", "🏦 Наименование банка", "КБ «Долинск» (АО)"},
	{"bank_account", "💳 Расчётный счёт", "40807810..."},
	{"bank_bik", "🔢 БИК", "046401727"},
	{"wallet", "₿ Крипто-кошелёк", "TXxxx..."},
}

func companyCardText(c *storage.Company, custom map[string]string, defs []storage.CustomField) string {
	resident := c.Resident
	if resident == "" {
		resident = "Резидент"
	}
	or := func(s string) string {
		if s == "" {
			return "—"
		}
		return escape(s)
	}
	lines := []string{
		"<b>🏢 " + escape(c.ShortName) + "</b>\n",
		"<b>Полное наименование:</b> " + escape(c.FullName),
		"<b>ИНН:</b> " + escape(c.INN) + "  <b>КПП:</b> " + escape(c.KPP),
	}
	if c.RegNumber != "" {
		lines = append(lines, "<b>Рег. номер:</b> "+escape(c.RegNumber))
	}
	lines = append(lines, "<b>Адрес:</b> "+escape(c.Address), "<b>Статус в КР:</b> "+escape(resident))
	if c.KIO != "" && c.KIO != "-" {
		lines = append(lines, "<b>КИО:</b> "+escape(c.KIO))
	}
	if c.INNRF != "" && c.INNRF != "-" {
		lines = append(lines, "<b>ИНН/КПП в РФ:</b> "+escape(c.INNRF)+" / "+escape(c.KPPRF))
	}
	lines = append(lines,
		"\n<b>Банк:</b> "+or(c.BankName),
		"<b>Р/счёт:</b> "+or(c.BankAccount),
		"<b>БИК:</b> "+or(c.BankBIK))
	if c.Wallet != "" {
		lines = append(lines, "\n<b>Кошелёк:</b> <code>"+escape(c.Wallet)+"</code>")
	}
	var extra []string
	for _, f := range defs {
		if v := custom[f.Key]; v != "" {
			extra = append(extra, "<b>"+escape(f.Label)+":</b> "+escape(v))
		}
	}
	if len(extra) > 0 {
		lines = append(lines, "")
		lines = append(lines, extra...)
	}
	return strings.Join(lines, "\n")
}

func (a *App) sendCompanyCard(c *Ctx, co *storage.Company, prefix string) {
	defs, err := a.st.CustomFields(c)
	if err != nil {
		c.fail(err)
		return
	}
	custom := map[string]string{}
	if len(defs) > 0 {
		if custom, err = a.st.CompanyCustom(c, co.ID); err != nil {
			c.fail(err)
			return
		}
	}
	c.Reply(prefix+companyCardText(co, custom, defs)+"\n"+a.companyReferralLine(c, co), companyCardKB(co.ID))
}

func (a *App) showCompanies(c *Ctx) {
	cs, err := a.st.Companies(c)
	if err != nil {
		c.fail(err)
		return
	}
	text := "📭 Список компаний пуст.\nДобавьте первую компанию:"
	if len(cs) > 0 {
		text = fmt.Sprintf("<b>🏢 Компании</b> (%d шт.)\n\nВыберите для просмотра или добавьте новую:", len(cs))
	}
	c.Reply(text, companiesListKB(cs, false))
}

func (a *App) companyCallback(c *Ctx, head, rest string) {
	switch head {
	case "view_company":
		c.s.Clear()
		id, _ := parseID(rest)
		co, err := a.st.Company(c, id)
		if err != nil {
			c.fail(err)
			return
		}
		if co == nil {
			c.Answer("Компания не найдена", true)
			return
		}
		a.sendCompanyCard(c, co, "")

	case "del_company":
		id, _ := parseID(rest)
		co, err := a.st.Company(c, id)
		if err != nil || co == nil {
			c.Answer("Компания не найдена", true)
			return
		}
		c.Edit("⚠️ Удалить компанию <b>"+escape(co.ShortName)+"</b>?\nЭто действие нельзя отменить.",
			confirmDeleteKB(id))

	case "confirm_del":
		id, _ := parseID(rest)
		if err := a.st.DeleteCompany(c, id); err != nil {
			c.fail(err)
			return
		}
		c.Answer("✅ Компания удалена", true)
		a.showCompanies(c)

	case "add_company":
		c.s.Clear()
		c.s.State = stCompanyInput
		c.s.Data["field_index"] = "0"
		c.s.CompanyData = map[string]string{}
		f := companyFields[0]
		c.Edit(fmt.Sprintf("➕ <b>Добавление компании</b>\n\nШаг 1/%d — %s:\n<i>Пример: %s</i>",
			len(companyFields), f[1], escape(f[2])), nil)

	case "edit_company":
		id, _ := parseID(rest)
		defs, err := a.st.CustomFields(c)
		if err != nil {
			c.fail(err)
			return
		}
		c.EditPlain("Выберите поле для редактирования:", editCompanyFieldsKB(id, defs))

	case "edit_company_field":
		idStr, key, _ := strings.Cut(rest, ":")
		id, _ := parseID(idStr)
		co, err := a.st.Company(c, id)
		if err != nil || co == nil {
			c.Answer("Компания не найдена", true)
			return
		}
		c.s.Clear()
		c.s.Data["editing_id"] = idStr
		c.s.Data["single_field"] = key

		if key == "referrer" {
			c.s.Clear()
			a.askCompanyReferrer(c, co)
			return
		}
		if key == "referral_percent" {
			c.s.State = stRefCoPct
			c.Edit("👥 <b>Процент реферала для этого клиента</b>\n\nВведите число (например: 15) или <code>-</code>, "+
				"чтобы брать процент самого реферала:", nil)
			return
		}
		if key == "resident" { // резидентство меняется кнопками
			c.s.State = stCompanyResident
			cur := co.Resident
			if cur == "" {
				cur = "Резидент"
			}
			c.Edit("✏️ <b>Резидент/Нерезидент КР</b>\n\nТекущее значение: <code>"+escape(cur)+"</code>",
				residentKB())
			return
		}
		var label, current string
		if ck, ok := strings.CutPrefix(key, "c__"); ok { // своя переменная
			defs, _ := a.st.CustomFields(c)
			label = ck
			for _, f := range defs {
				if f.Key == ck {
					label = f.Label
				}
			}
			custom, _ := a.st.CompanyCustom(c, id)
			current = custom[ck]
		} else {
			label = key
			for _, f := range companyEditFields {
				if f[0] == key {
					label = f[1]
				}
			}
			current = co.Map()[key]
		}
		c.s.State = stCompanyInput
		c.Edit("✏️ <b>"+escape(label)+"</b>\n\nТекущее значение: <code>"+escape(current)+
			"</code>\n\nВведите новое значение:", nil)

	case "resident":
		if c.s.State != stCompanyResident {
			return
		}
		a.setResident(c, rest)
	}
}

func (a *App) companyMessage(c *Ctx, text string) {
	switch c.s.State {
	case stCompanyInput:
		a.companyInput(c, text)
	case stCompanyResident:
		v := "Резидент"
		if strings.HasPrefix(strings.ToLower(text), "нерез") {
			v = "Нерезидент"
		}
		a.setResident(c, v)
	case stCompanyCustom:
		defs := c.s.CustomDefs
		idx := c.s.CustomIndex
		if idx < len(defs) && text != "-" {
			c.s.CustomValues[defs[idx].Key] = text
		}
		c.s.CustomIndex = idx + 1
		a.nextCustomOrSave(c)
	}
}

func (a *App) companyInput(c *Ctx, value string) {
	// Правка одного поля существующей компании
	if key := c.s.Data["single_field"]; key != "" {
		id, _ := parseID(c.s.Data["editing_id"])
		var err error
		if ck, ok := strings.CutPrefix(key, "c__"); ok {
			if value == "-" {
				value = ""
			}
			err = a.st.SetCompanyCustom(c, id, ck, value)
		} else {
			err = a.st.UpdateCompanyField(c, id, key, value)
		}
		c.s.Clear()
		if err != nil {
			c.fail(err)
			return
		}
		co, err := a.st.Company(c, id)
		if err != nil || co == nil {
			c.Send("Компания не найдена", backToMainKB())
			return
		}
		a.sendCompanyCard(c, co, "✅ Поле обновлено!\n\n")
		return
	}

	// Последовательное добавление
	idx := 0
	fmt.Sscan(c.s.Data["field_index"], &idx)
	if idx >= len(companyFields) {
		idx = len(companyFields) - 1
	}
	if c.s.CompanyData == nil {
		c.s.CompanyData = map[string]string{}
	}
	c.s.CompanyData[companyFields[idx][0]] = value
	next := idx + 1
	if next < len(companyFields) {
		c.s.Data["field_index"] = fmt.Sprint(next)
		f := companyFields[next]
		c.Send(fmt.Sprintf("Шаг %d/%d — %s:\n<i>Пример: %s</i>\n(Введите <code>-</code> чтобы пропустить)",
			next+1, len(companyFields), f[1], escape(f[2])), nil)
		return
	}
	// Реквизиты заполнены → резидентство (кнопками)
	c.s.State = stCompanyResident
	c.Send(fmt.Sprintf("Шаг %d — Резидент или нерезидент Кыргызской Республики?\n"+
		"<i>(нужно для отчёта в Финнадзор)</i>", len(companyFields)+1), residentKB())
}

func (a *App) setResident(c *Ctx, value string) {
	if c.s.Data["single_field"] == "resident" && c.s.Data["editing_id"] != "" {
		id, _ := parseID(c.s.Data["editing_id"])
		c.s.Clear()
		if err := a.st.UpdateCompanyField(c, id, "resident", value); err != nil {
			c.fail(err)
			return
		}
		co, err := a.st.Company(c, id)
		if err != nil || co == nil {
			c.Send("Компания не найдена", backToMainKB())
			return
		}
		a.sendCompanyCard(c, co, "✅ Поле обновлено!\n\n")
		return
	}
	if c.s.CompanyData == nil {
		return
	}
	c.s.CompanyData["resident"] = value
	c.Answer(value, false)
	a.nextCustomOrSave(c)
}

// nextCustomOrSave задаёт вопросы по своим переменным, затем сохраняет компанию.
func (a *App) nextCustomOrSave(c *Ctx) {
	if !c.s.CustomDefsLoaded {
		defs, err := a.st.CustomFields(c)
		if err != nil {
			c.fail(err)
			return
		}
		c.s.CustomDefs, c.s.CustomDefsLoaded = defs, true
		c.s.CustomIndex = 0
		c.s.CustomValues = map[string]string{}
	}
	if idx := c.s.CustomIndex; idx < len(c.s.CustomDefs) {
		f := c.s.CustomDefs[idx]
		c.s.State = stCompanyCustom
		c.Reply(fmt.Sprintf("🧩 <b>%s</b>\n<i>(своя переменная {{CL_%s}}; введите <code>-</code> чтобы пропустить)</i>",
			escape(f.Label), strings.ToUpper(f.Key)), nil)
		return
	}
	// Всё собрано — сохраняем
	id, err := a.st.AddCompany(c, c.s.CompanyData)
	if err != nil {
		c.fail(err)
		return
	}
	for k, v := range c.s.CustomValues {
		if err := a.st.SetCompanyCustom(c, id, k, v); err != nil {
			c.fail(err)
			return
		}
	}
	c.s.Clear()
	co, err := a.st.Company(c, id)
	if err != nil || co == nil {
		c.fail(fmt.Errorf("компания не сохранилась"))
		return
	}
	a.sendCompanyCard(c, co, "✅ <b>Компания добавлена!</b>\n\n")
}
