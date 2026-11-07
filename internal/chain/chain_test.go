package chain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTronAndEther(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/transactions/trc20"):
			w.Write([]byte(`{"success":true,"data":[{"transaction_id":"abc","from":"TFROM","to":"TME","value":"500000000",
				"block_timestamp":1780067016000,"token_info":{"decimals":6}}]}`))
		case strings.HasPrefix(r.URL.Path, "/v1/accounts/"):
			w.Write([]byte(`{"success":true,"data":[{"balance":12500000,"trc20":[{"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t":"1234560000"}]}]}`))
		case r.URL.Query().Get("action") == "tokentx":
			if r.URL.Query().Get("chainid") != "56" || r.URL.Query().Get("apikey") != "KEY" {
				t.Errorf("параметры: %v", r.URL.Query())
			}
			w.Write([]byte(`{"status":"1","message":"OK","result":[{"hash":"0xh","from":"0xa","to":"0xb",
				"value":"2500000000000000000000","tokenDecimal":"18","timeStamp":"1780067016"}]}`))
		}
	}))
	defer srv.Close()
	c := New("", "KEY")
	c.TronURL, c.EtherURL = srv.URL, srv.URL+"/v2/api"
	ctx := context.Background()

	tr, err := c.Transfers(ctx, "trc20", "TME", 10)
	if err != nil || len(tr) != 1 || tr[0].Amount != 500 || tr[0].From != "TFROM" {
		t.Fatalf("tron: %+v %v", tr, err)
	}
	b, err := c.GetBalance(ctx, "TRC-20", "TME")
	if err != nil || b.USDT != 1234.56 || b.Native != 12.5 {
		t.Fatalf("balance: %+v %v", b, err)
	}
	bsc, err := c.Transfers(ctx, "BEP-20", "0xb", 10)
	if err != nil || len(bsc) != 1 || bsc[0].Amount != 2500 {
		t.Fatalf("bsc: %+v %v", bsc, err)
	}
	if _, err := New("", "").Transfers(ctx, "ERC-20", "0xb", 1); err != ErrNoKey {
		t.Fatalf("без ключа: %v", err)
	}
}
