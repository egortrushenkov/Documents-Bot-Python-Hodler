// Package docgen — генерация DOCX-документов из шаблонов с плейсхолдерами {{...}}.
package docgen

import (
	"fmt"
	"strings"
	"time"

	"hodler-doc-bot/internal/storage"
	"hodler-doc-bot/internal/util"
)

var monthsGen = [...]string{"", "января", "февраля", "марта", "апреля", "мая", "июня",
	"июля", "августа", "сентября", "октября", "ноября", "декабря"}

// FormatDateFull: 12.04.2026 → «12» апреля 2026 г.
func FormatDateFull(s string) string {
	d, err := time.Parse("02.01.2006", strings.TrimSpace(s))
	if err != nil {
		return s
	}
	return fmt.Sprintf("«%d» %s %d г.", d.Day(), monthsGen[d.Month()], d.Year())
}

// Draft — данные акта/счёта, собранные в диалоге (ключи как в Python-версии).
type Draft struct {
	Fields map[string]string
	Txs    []storage.Tx
}

func (d *Draft) Get(k string) string { return d.Fields[k] }

// GetOr — значение ключа или def, если ключа нет вовсе (пустое значение — валидно).
func (d *Draft) GetOr(k, def string) string {
	if v, ok := d.Fields[k]; ok {
		return v
	}
	return def
}

// ComputedTx — транзакция с рассчитанными по курсу рублями.
type ComputedTx struct {
	VAAmount   string
	FiatAmount float64
	TxHash     string
}

// Transactions — части сделки. Несколько — только если их явно добавили (>1);
// для одной берём текущие поля сделки (так работает ручная правка суммы/хэша).
func Transactions(txs []storage.Tx, vaAmount, txHash string) []storage.Tx {
	if len(txs) > 1 {
		return txs
	}
	return []storage.Tx{{VAAmount: vaAmount, TxHash: txHash}}
}

// ComputeTransactions: рубли каждой части = ВА × курс, последняя часть (остаток)
// добивается так, чтобы сумма рублей точно совпала с общей суммой сделки.
func ComputeTransactions(txs []storage.Tx, vaAmount, fiatAmount, txHash string) []ComputedTx {
	parts := Transactions(txs, vaAmount, txHash)
	totalVA := 0.0
	for _, t := range parts {
		totalVA += util.ToFloat(t.VAAmount)
	}
	totalFiat := util.ToFloat(fiatAmount)
	rate := 0.0
	if totalVA != 0 {
		rate = totalFiat / totalVA
	}
	out := make([]ComputedTx, 0, len(parts))
	acc := 0.0
	for i, t := range parts {
		var fiat float64
		if i < len(parts)-1 {
			fiat = util.Round2(util.ToFloat(t.VAAmount) * rate)
			acc += fiat
		} else {
			fiat = util.Round2(totalFiat - acc)
		}
		out = append(out, ComputedTx{VAAmount: t.VAAmount, FiatAmount: fiat, TxHash: t.TxHash})
	}
	return out
}

// Pair — плейсхолдер и его значение (порядок замены важен и стабилен).
type Pair struct{ Key, Value string }

// BuildReplacements — все плейсхолдеры документа.
func BuildReplacements(d *Draft, op, client, clientCustom map[string]string) []Pair {
	get := func(m map[string]string, k, def string) string {
		if v, ok := m[k]; ok {
			return v
		}
		return def
	}
	dealDate := d.GetOr("deal_date", "")
	vaType := d.GetOr("va_type", "USDT")
	network := d.GetOr("network", "TRC-20")
	vaFmt := util.FormatNumber(d.GetOr("va_amount", "0"))
	fiatFmt := util.FormatNumber(d.GetOr("fiat_amount", "0"))
	operation := "Покупка виртуальных активов"
	if d.GetOr("act_type", "sell") == "buy" { // клиент продаёт нам
		operation = "Продажа виртуальных активов"
	}
	pairs := []Pair{
		{"{{DEAL_NUMBER}}", d.GetOr("deal_number", "")},
		{"{{DEAL_DATE}}", dealDate},
		{"{{DEAL_DATE_FULL}}", FormatDateFull(dealDate)},
		{"{{OPERATION_TYPE}}", operation},
		{"{{KVVO}}", d.GetOr("kvvo", "99082")},
		{"{{VA_TYPE}}", vaType},
		{"{{NETWORK}}", network},
		{"{{VA_TICKER}}", vaType + "_" + strings.ReplaceAll(network, "-", "")},
		{"{{VA_AMOUNT}}", vaFmt},
		{"{{VA_AMOUNT_SHORT}}", vaFmt},
		{"{{FIAT_AMOUNT}}", fiatFmt},
		{"{{FIAT_AMOUNT_SHORT}}", fiatFmt},
		{"{{FIAT_CURRENCY}}", "RUB"},
		{"{{EXCHANGE_RATE}}", strings.ReplaceAll(d.GetOr("exchange_rate", ""), ".", ",")},
		{"{{COMMISSION_FIAT}}", d.GetOr("commission_fiat", "0%")},
		{"{{COMMISSION_VA}}", d.GetOr("commission_va", "0%")},
		{"{{TX_HASH}}", d.GetOr("tx_hash", "")},
		{"{{EXECUTION_DATE}}", d.GetOr("execution_date", dealDate)},
		{"{{CL_WALLET}}", d.GetOr("client_wallet", get(client, "wallet", ""))},
		{"{{OP_WALLET}}", d.GetOr("operator_wallet", get(op, "wallet", ""))},
		// Оператор
		{"{{OP_FULL_NAME}}", get(op, "full_name", "")},
		{"{{OP_SHORT_NAME}}", get(op, "short_name", "")},
		{"{{OP_INN}}", get(op, "inn", "")},
		{"{{OP_KPP}}", get(op, "kpp", "")},
		{"{{OP_ADDRESS}}", get(op, "address", "")},
		{"{{OP_ADDRESS_FULL}}", get(op, "address", "")},
		{"{{OP_LEGAL_ADDRESS}}", get(op, "legal_address", get(op, "address", ""))},
		{"{{OP_LICENSE}}", get(op, "license", "")},
		{"{{OP_KIO}}", get(op, "kio", "")},
		{"{{OP_INN_RF}}", get(op, "inn_rf", get(op, "inn", ""))},
		{"{{OP_KPP_RF}}", get(op, "kpp_rf", get(op, "kpp", ""))},
		{"{{OP_BANK_NAME}}", get(op, "bank_name", "")},
		{"{{OP_BANK_ACCOUNT}}", get(op, "bank_account", "")},
		{"{{OP_BANK_BIK}}", get(op, "bank_bik", "")},
		{"{{OP_DIRECTOR}}", get(op, "director_name", "")},
		// Клиент
		{"{{CL_FULL_NAME}}", get(client, "full_name", "")},
		{"{{CL_SHORT_NAME}}", get(client, "short_name", "")},
		{"{{CL_INN}}", get(client, "inn", "")},
		{"{{CL_KPP}}", get(client, "kpp", "")},
		{"{{CL_REG_NUMBER}}", get(client, "reg_number", "")},
		{"{{CL_ADDRESS}}", get(client, "address", "")},
		{"{{CL_KIO}}", get(client, "kio", "-")},
		{"{{CL_INN_RF}}", get(client, "inn_rf", "-")},
		{"{{CL_KPP_RF}}", get(client, "kpp_rf", "-")},
		{"{{CL_BANK_NAME}}", get(client, "bank_name", "")},
		{"{{CL_BANK_ACCOUNT}}", get(client, "bank_account", "")},
		{"{{CL_BANK_BIK}}", get(client, "bank_bik", "")},
	}
	// Свои переменные: любой ключ реквизитов оператора — {{OP_KEY}},
	// значения пользовательских полей компании — {{CL_KEY}}.
	seen := map[string]bool{}
	for _, p := range pairs {
		seen[p.Key] = true
	}
	add := func(prefix string, m map[string]string) {
		for _, k := range sortedKeys(m) {
			key := "{{" + prefix + strings.ToUpper(k) + "}}"
			if !seen[key] {
				seen[key] = true
				pairs = append(pairs, Pair{key, m[k]})
			}
		}
	}
	add("OP_", op)
	add("CL_", clientCustom)
	return pairs
}

// KnownPlaceholders — плейсхолдеры, которые бот умеет заполнять.
func KnownPlaceholders(op map[string]string, custom []storage.CustomField) map[string]bool {
	out := map[string]bool{}
	for _, p := range BuildReplacements(&Draft{Fields: map[string]string{}}, op, map[string]string{}, nil) {
		out[p.Key] = true
	}
	for _, f := range custom {
		out["{{CL_"+strings.ToUpper(f.Key)+"}}"] = true
	}
	return out
}
