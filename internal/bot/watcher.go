package bot

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	tg "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"hodler-doc-bot/internal/chain"
	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/util"
)

// watchLoop периодически проверяет наши кошельки и уведомляет о новых
// поступлениях/списаниях USDT.
func (a *App) watchLoop(ctx context.Context) {
	if a.cfg.WalletPollSeconds <= 0 {
		return
	}
	interval := time.Duration(a.cfg.WalletPollSeconds) * time.Second
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		a.checkWallets(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// checkWallets — один проход по всем отслеживаемым кошелькам.
func (a *App) checkWallets(ctx context.Context) {
	wallets, err := a.st.WatchedWallets(ctx)
	if err != nil {
		slog.Error("кошельки", "err", err)
		return
	}
	for _, w := range wallets {
		if !chain.Supported(w.Network) {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		trs, err := a.chain.Transfers(cctx, w.Network, w.Address, 50)
		cancel()
		if err != nil {
			slog.Warn("не удалось проверить кошелёк", "wallet", w.Short(), "err", err)
			continue
		}
		sort.Slice(trs, func(i, j int) bool { return trs[i].Time.Before(trs[j].Time) }) // старые первыми
		for _, t := range trs {
			in := strings.EqualFold(t.To, w.Address)
			dir, other := "out", t.To
			if in {
				dir, other = "in", t.From
			}
			fresh := t.Time.UnixMilli() >= w.WatchSince
			notify := fresh && ((in && w.WatchIn) || (!in && w.WatchOut)) && t.Amount >= w.MinAmount && t.Amount > 0
			isNew, err := a.st.MarkWalletTx(ctx, &storage.WalletTx{Hash: t.Hash, WalletID: w.ID, Direction: dir,
				Amount: t.Amount, Counterparty: other, TS: t.Time.UnixMilli()}, notify)
			if err != nil || !isNew || !notify {
				continue
			}
			a.notifyWalletTx(ctx, w, t, in)
		}
		select { // бережём лимиты API
		case <-ctx.Done():
			return
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func walletTxText(w *storage.Wallet, t chain.Transfer, in bool, who string) string {
	head := fmt.Sprintf("💰 <b>Поступление +%s USDT</b> (%s)", util.RU(t.Amount, 2), escape(w.Network))
	lines := []string{head, "<b>На:</b> " + escape(w.Title())}
	other := t.From
	if !in {
		head = fmt.Sprintf("💸 <b>Списание −%s USDT</b> (%s)", util.RU(t.Amount, 2), escape(w.Network))
		lines = []string{head, "<b>С:</b> " + escape(w.Title())}
		other = t.To
	}
	label := "<b>От:</b> "
	if !in {
		label = "<b>Кому:</b> "
	}
	cp := "<code>" + escape(other) + "</code>"
	if who != "" {
		cp += " — <b>" + escape(who) + "</b>"
	}
	lines = append(lines, label+cp, "🕐 "+t.Time.Format("02.01.2006 15:04"))
	if u := chain.ExplorerURL(w.Network, t.Hash); u != "" {
		lines = append(lines, `<a href="`+u+`">Транзакция в обозревателе</a>`)
	}
	return strings.Join(lines, "\n")
}

// notifyWalletTx — кому: админам в личку и/или в чаты руководителей (настройки).
func (a *App) notifyWalletTx(ctx context.Context, w *storage.Wallet, t chain.Transfer, in bool) {
	other := t.From
	if !in {
		other = t.To
	}
	text := walletTxText(w, t, in, a.counterpartyName(ctx, other))
	s, _ := a.st.Settings(ctx)
	var targets []int64
	if s["wallet_notify_admins"] != "0" {
		for id := range a.cfg.AdminIDs {
			targets = append(targets, id)
		}
	}
	if s["wallet_notify_groups"] == "1" {
		chats, _ := a.st.NotifyChats(ctx)
		for _, ch := range chats {
			targets = append(targets, ch.ChatID)
		}
	}
	for _, chatID := range targets {
		if _, err := a.b.SendMessage(ctx, &tg.SendMessageParams{ChatID: chatID, Text: text, ParseMode: models.ParseModeHTML,
			LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: tg.True()}}); err != nil {
			slog.Warn("уведомление о транзакции не отправлено", "chat", chatID, "err", err)
		}
	}
}
