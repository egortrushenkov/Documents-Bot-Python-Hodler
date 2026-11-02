package docgen

import (
	"path/filepath"
	"strings"
	"testing"

	"hodler-doc-bot/internal/storage"
)

func TestComputeTransactions(t *testing.T) {
	txs := ComputeTransactions([]storage.Tx{{VAAmount: "100", TxHash: "h1"}, {VAAmount: "6187.73", TxHash: "h2"}}, "6287.73", "500000", "")
	if len(txs) != 2 || txs[0].FiatAmount != 7952.0 || txs[1].FiatAmount != 492048.0 {
		t.Fatalf("%+v", txs)
	}
	one := ComputeTransactions(nil, "10", "800", "hash")
	if len(one) != 1 || one[0].FiatAmount != 800 || one[0].TxHash != "hash" {
		t.Fatalf("%+v", one)
	}
}

func TestGenerateAllTemplates(t *testing.T) {
	op := map[string]string{"short_name": "ОП", "inn": "1", "wallet": "TOP", "director_name": "Иванов"}
	client := map[string]string{"short_name": "ОсОО «Ромашка»", "full_name": "Ромашка", "inn": "2", "wallet": "TCL"}
	for _, kind := range []string{"sell", "buy", "invoice"} {
		d := &Draft{Fields: map[string]string{
			"act_type": kind, "deal_number": "512", "deal_date": "12.04.2026", "execution_date": "12.04.2026",
			"va_type": "USDT", "network": "TRC-20", "va_amount": "6287.73", "fiat_amount": "500000",
			"exchange_rate": "79,52", "kvvo": "99082", "client_wallet": "TCL", "operator_wallet": "TOP",
			"commission_fiat": "0%", "commission_va": "0%",
		}}
		if kind != "invoice" {
			d.Txs = []storage.Tx{{VAAmount: "100", TxHash: "hash_test"}, {VAAmount: "6187.73", TxHash: "hash_rest"}}
		}
		out := t.TempDir()
		path, err := Generate("../../templates", out, d, op, client, nil)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if filepath.Dir(path) != out {
			t.Fatalf("path %s", path)
		}
		left, err := ScanPlaceholders(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(left) > 0 {
			t.Errorf("%s: остались плейсхолдеры %v", kind, left)
		}
		pkg, _ := openDocx(path)
		body := string(pkg.data["word/document.xml"])
		if kind != "invoice" {
			for _, s := range []string{"hash_test", "hash_rest", "7 952", "492 048", "500 000"} {
				if !strings.Contains(body, s) {
					t.Errorf("%s: нет %q в документе", kind, s)
				}
			}
		}
	}
}
