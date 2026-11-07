package bot

import (
	"fmt"
	"strings"

	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/util"
)

const (
	stRefName    = "ref:name"
	stRefPercent = "ref:percent"
	stRefCoPct   = "ref:copct" // свой процент для клиента
)

func (a *App) showReferrers(c *Ctx, prefix string) {
	refs, err := a.st.Referrers(c)
	if err != nil {
		c.fail(err)
		return
	}
	lines := []string{prefix + "👥 <b>Рефералы</b>\n",
		"Реферал закрепляется за клиентом (🏢 Компании → ✏️ Редактировать → 👥 Реферал).",
		"Вознаграждение = процент × (ЧКД − налог). В проведённых сделках имя и процент сохраняются на момент сделки.\n"}
	var rows []kbRow
	if len(refs) == 0 {
		lines = append(lines, "<i>Пока никого нет.</i>")
	}
	for _, r := range refs {
		rows = append(rows, row(btn(fmt.Sprintf("👤 %s — %s%%", r.Name, util.Fmt2(r.Percent)), fmt.Sprintf("ref:view:%d", r.ID))))
	}
	rows = append(rows, row(btn("➕ Добавить реферала", "ref:add")), row(btn("🏠 Главное меню", "menu:main")))
	c.Reply(strings.Join(lines, "\n"), ikb(rows...))
}

func (a *App) referrerCallback(c *Ctx, head, rest string) {
	parts := strings.Split(rest, ":")
	id := func(i int) int64 {
		if i < len(parts) {
			n, _ := parseID(parts[i])
			return n
		}
		return 0
	}
	if head == "coref" { // выбор реферала для клиента: coref:<companyID>:<referrerID>
		cid, rid := id(0), id(1)
		if err := a.st.SetCompanyReferrer(c, cid, rid); err != nil {
			c.fail(err)
			return
		}
		co, _ := a.st.Company(c, cid)
		if co != nil {
			a.sendCompanyCard(c, co, "✅ Реферал обновлён\n\n")
		}
		return
	}
	switch parts[0] {
	case "menu":
		c.s.Clear()
		a.showReferrers(c, "")
	case "add":
		c.s.Clear()
		c.s.State = stRefName
		c.Edit("➕ <b>Новый реферал</b>\n\nВведите имя, как оно будет в отчёте (например: <i>Дима Реферал</i>):", nil)
	case "view":
		r, err := a.st.Referrer(c, id(1))
		if err != nil || r == nil {
			c.Answer("Не найден", true)
			return
		}
		c.Edit(fmt.Sprintf("👤 <b>%s</b>\nПроцент: %s%% от (ЧКД − налог)", escape(r.Name), util.Fmt2(r.Percent)), ikb(
			row(btn("✏️ Имя", fmt.Sprintf("ref:ename:%d", r.ID)), btn("✏️ Процент", fmt.Sprintf("ref:epct:%d", r.ID))),
			row(btn("🗑 Удалить", fmt.Sprintf("ref:del:%d", r.ID))),
			row(btn("◀️ Назад", "ref:menu"))))
	case "ename", "epct":
		c.s.Clear()
		c.s.Data["ref_id"] = parts[1]
		if parts[0] == "ename" {
			c.s.State = stRefName
			c.EditPlain("Введите новое имя:", nil)
		} else {
			c.s.State = stRefPercent
			c.EditPlain("Введите процент (например: 20):", nil)
		}
	case "del":
		if err := a.st.DeleteReferrer(c, id(1)); err != nil {
			c.fail(err)
			return
		}
		c.Answer("Удалён", false)
		a.showReferrers(c, "")
	}
}

func (a *App) referrerMessage(c *Ctx, text string) {
	switch c.s.State {
	case stRefName:
		if rid, ok := parseID(c.s.Data["ref_id"]); ok { // переименование
			r, _ := a.st.Referrer(c, rid)
			if r != nil {
				_ = a.st.UpdateReferrer(c, rid, text, r.Percent)
			}
			c.s.Clear()
			a.showReferrers(c, "✅ Сохранено\n\n")
			return
		}
		c.s.Data["ref_name"] = text
		c.s.State = stRefPercent
		c.Send("Процент вознаграждения от (ЧКД − налог), например <code>20</code>:", nil)
	case stRefPercent:
		pct, ok := util.ParseFloat(strings.TrimSuffix(text, "%"))
		if !ok || pct < 0 || pct > 100 {
			c.SendPlain("❌ Введите число от 0 до 100", nil)
			return
		}
		var err error
		if rid, ok := parseID(c.s.Data["ref_id"]); ok {
			r, _ := a.st.Referrer(c, rid)
			if r != nil {
				err = a.st.UpdateReferrer(c, rid, r.Name, pct)
			}
		} else {
			_, err = a.st.AddReferrer(c, c.s.Data["ref_name"], pct)
		}
		c.s.Clear()
		if err != nil {
			c.fail(err)
			return
		}
		a.showReferrers(c, "✅ Сохранено\n\n")
	case stRefCoPct:
		cid, _ := parseID(c.s.Data["editing_id"])
		var err error
		if text == "-" {
			err = a.st.SetCompanyReferralPercent(c, cid, 0, false)
		} else {
			pct, ok := util.ParseFloat(strings.TrimSuffix(text, "%"))
			if !ok || pct < 0 || pct > 100 {
				c.SendPlain("❌ Введите число от 0 до 100 или - (как у реферала)", nil)
				return
			}
			err = a.st.SetCompanyReferralPercent(c, cid, pct, true)
		}
		c.s.Clear()
		if err != nil {
			c.fail(err)
			return
		}
		if co, _ := a.st.Company(c, cid); co != nil {
			a.sendCompanyCard(c, co, "✅ Поле обновлено!\n\n")
		}
	}
}

// askCompanyReferrer — выбор реферала кнопками для клиента.
func (a *App) askCompanyReferrer(c *Ctx, co *storage.Company) {
	refs, err := a.st.Referrers(c)
	if err != nil {
		c.fail(err)
		return
	}
	var rows []kbRow
	for _, r := range refs {
		mark := "👤"
		if co.ReferrerID.Valid && co.ReferrerID.Int64 == r.ID {
			mark = "✅"
		}
		rows = append(rows, row(btn(fmt.Sprintf("%s %s — %s%%", mark, r.Name, util.Fmt2(r.Percent)), fmt.Sprintf("coref:%d:%d", co.ID, r.ID))))
	}
	rows = append(rows, row(btn("🚫 Без реферала", fmt.Sprintf("coref:%d:0", co.ID))),
		row(btn("➕ Новый реферал", "ref:add")), row(btn("◀️ Назад", fmt.Sprintf("view_company:%d", co.ID))))
	c.Edit("👥 <b>Реферал клиента "+escape(co.ShortName)+"</b>\nВыберите:", ikb(rows...))
}

// companyReferralLine — строка для карточки компании.
func (a *App) companyReferralLine(c *Ctx, co *storage.Company) string {
	name, pct, err := a.st.CompanyReferral(c, co)
	if err != nil || name == "" {
		return "<b>Реферал:</b> —"
	}
	own := ""
	if co.ReferralPercent.Valid {
		own = " (свой процент)"
	}
	return fmt.Sprintf("<b>Реферал:</b> %s, %s%%%s", escape(name), util.Fmt2(pct), own)
}
