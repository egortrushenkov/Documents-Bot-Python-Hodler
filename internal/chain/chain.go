// Package chain — переводы и балансы USDT на кошельках через публичные API:
// TronGrid (TRC-20, ключ необязателен) и Etherscan v2 (ERC-20 и BEP-20,
// нужен бесплатный ключ ETHERSCAN_API_KEY).
package chain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	usdtTRC20 = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
	usdtERC20 = "0xdAC17F958D2ee523a2206206994597C13D831ec7"
	usdtBEP20 = "0x55d398326f99059fF775485246999027B3197955"
)

// ErrNoKey — для сети нужен API-ключ, а он не задан.
var ErrNoKey = errors.New("для этой сети нужен ETHERSCAN_API_KEY")

// ErrUnsupported — сеть не поддерживается.
var ErrUnsupported = errors.New("сеть не поддерживается (TRC-20, ERC-20, BEP-20)")

type Transfer struct {
	Hash   string
	From   string
	To     string
	Amount float64 // USDT
	Time   time.Time
}

type Balance struct {
	USDT   float64
	Native float64 // TRX / ETH / BNB — для оплаты комиссий сети
	Symbol string
}

type Client struct {
	HTTP     *http.Client
	TronURL  string // https://api.trongrid.io
	TronKey  string
	EtherURL string // https://api.etherscan.io/v2/api
	EtherKey string
}

func New(tronKey, etherKey string) *Client {
	return &Client{
		HTTP:     &http.Client{Timeout: 20 * time.Second},
		TronURL:  "https://api.trongrid.io",
		TronKey:  tronKey,
		EtherURL: "https://api.etherscan.io/v2/api",
		EtherKey: etherKey,
	}
}

// Normalize: «trc20», «TRC 20», «TRC-20» → «TRC-20».
func Normalize(network string) string {
	n := strings.ToUpper(strings.NewReplacer("-", "", " ", "", "_", "").Replace(network))
	switch n {
	case "TRC20", "TRON", "TRX":
		return "TRC-20"
	case "ERC20", "ETH", "ETHEREUM":
		return "ERC-20"
	case "BEP20", "BSC", "BNB":
		return "BEP-20"
	}
	return network
}

// Supported — можно ли отслеживать сеть.
func Supported(network string) bool {
	switch Normalize(network) {
	case "TRC-20", "ERC-20", "BEP-20":
		return true
	}
	return false
}

// ExplorerURL — ссылка на транзакцию в обозревателе.
func ExplorerURL(network, hash string) string {
	switch Normalize(network) {
	case "TRC-20":
		return "https://tronscan.org/#/transaction/" + hash
	case "ERC-20":
		return "https://etherscan.io/tx/" + hash
	case "BEP-20":
		return "https://bscscan.com/tx/" + hash
	}
	return ""
}

// AddressURL — ссылка на кошелёк в обозревателе.
func AddressURL(network, addr string) string {
	switch Normalize(network) {
	case "TRC-20":
		return "https://tronscan.org/#/address/" + addr
	case "ERC-20":
		return "https://etherscan.io/address/" + addr
	case "BEP-20":
		return "https://bscscan.com/address/" + addr
	}
	return ""
}

func (c *Client) get(ctx context.Context, u string, header map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d от %s", resp.StatusCode, req.URL.Host)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// units: строка в минимальных единицах → число с учётом decimals.
func units(v string, decimals int) float64 {
	n, ok := new(big.Float).SetString(strings.TrimSpace(v))
	if !ok {
		return 0
	}
	n.Quo(n, new(big.Float).SetFloat64(math.Pow10(decimals)))
	f, _ := n.Float64()
	return f
}

// Transfers — последние переводы USDT на кошельке (входящие и исходящие).
func (c *Client) Transfers(ctx context.Context, network, address string, limit int) ([]Transfer, error) {
	switch Normalize(network) {
	case "TRC-20":
		return c.tronTransfers(ctx, address, limit)
	case "ERC-20":
		return c.etherTransfers(ctx, 1, usdtERC20, address, limit)
	case "BEP-20":
		return c.etherTransfers(ctx, 56, usdtBEP20, address, limit)
	}
	return nil, ErrUnsupported
}

func (c *Client) tronHeader() map[string]string {
	if c.TronKey == "" {
		return nil
	}
	return map[string]string{"TRON-PRO-API-KEY": c.TronKey}
}

func (c *Client) tronTransfers(ctx context.Context, address string, limit int) ([]Transfer, error) {
	u := fmt.Sprintf("%s/v1/accounts/%s/transactions/trc20?limit=%d&contract_address=%s",
		c.TronURL, url.PathEscape(address), limit, usdtTRC20)
	var resp struct {
		Success bool `json:"success"`
		Data    []struct {
			TxID      string `json:"transaction_id"`
			From      string `json:"from"`
			To        string `json:"to"`
			Value     string `json:"value"`
			Timestamp int64  `json:"block_timestamp"`
			TokenInfo struct {
				Decimals int `json:"decimals"`
			} `json:"token_info"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := c.get(ctx, u, c.tronHeader(), &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("TronGrid: %s", resp.Error)
	}
	var out []Transfer
	for _, d := range resp.Data {
		dec := d.TokenInfo.Decimals
		if dec == 0 {
			dec = 6
		}
		out = append(out, Transfer{Hash: d.TxID, From: d.From, To: d.To, Amount: units(d.Value, dec),
			Time: time.UnixMilli(d.Timestamp)})
	}
	return out, nil
}

type etherResp struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Result  json.RawMessage `json:"result"`
}

func (c *Client) ether(ctx context.Context, chainID int, params url.Values) (json.RawMessage, error) {
	if c.EtherKey == "" {
		return nil, ErrNoKey
	}
	params.Set("chainid", strconv.Itoa(chainID))
	params.Set("apikey", c.EtherKey)
	var r etherResp
	if err := c.get(ctx, c.EtherURL+"?"+params.Encode(), nil, &r); err != nil {
		return nil, err
	}
	if r.Status != "1" {
		// «No transactions found» — не ошибка
		if strings.Contains(strings.ToLower(r.Message), "no transactions") {
			return json.RawMessage("[]"), nil
		}
		var msg string
		_ = json.Unmarshal(r.Result, &msg)
		return nil, fmt.Errorf("Etherscan: %s %s", r.Message, msg)
	}
	return r.Result, nil
}

func (c *Client) etherTransfers(ctx context.Context, chainID int, contract, address string, limit int) ([]Transfer, error) {
	raw, err := c.ether(ctx, chainID, url.Values{
		"module": {"account"}, "action": {"tokentx"}, "contractaddress": {contract}, "address": {address},
		"page": {"1"}, "offset": {strconv.Itoa(limit)}, "sort": {"desc"},
	})
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Hash      string `json:"hash"`
		From      string `json:"from"`
		To        string `json:"to"`
		Value     string `json:"value"`
		Decimal   string `json:"tokenDecimal"`
		TimeStamp string `json:"timeStamp"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	var out []Transfer
	for _, r := range rows {
		dec, _ := strconv.Atoi(r.Decimal)
		ts, _ := strconv.ParseInt(r.TimeStamp, 10, 64)
		out = append(out, Transfer{Hash: r.Hash, From: r.From, To: r.To, Amount: units(r.Value, dec), Time: time.Unix(ts, 0)})
	}
	return out, nil
}

// GetBalance — баланс USDT и монеты сети (для комиссий).
func (c *Client) GetBalance(ctx context.Context, network, address string) (Balance, error) {
	switch Normalize(network) {
	case "TRC-20":
		var resp struct {
			Data []struct {
				Balance int64               `json:"balance"`
				TRC20   []map[string]string `json:"trc20"`
			} `json:"data"`
			Success bool `json:"success"`
		}
		if err := c.get(ctx, fmt.Sprintf("%s/v1/accounts/%s", c.TronURL, url.PathEscape(address)), c.tronHeader(), &resp); err != nil {
			return Balance{}, err
		}
		b := Balance{Symbol: "TRX"}
		if len(resp.Data) == 0 { // кошелёк ещё не активирован
			return b, nil
		}
		b.Native = float64(resp.Data[0].Balance) / 1e6
		for _, m := range resp.Data[0].TRC20 {
			if v, ok := m[usdtTRC20]; ok {
				b.USDT = units(v, 6)
			}
		}
		return b, nil
	case "ERC-20", "BEP-20":
		chainID, contract, sym, dec := 1, usdtERC20, "ETH", 6
		if Normalize(network) == "BEP-20" {
			chainID, contract, sym, dec = 56, usdtBEP20, "BNB", 18
		}
		var b Balance
		b.Symbol = sym
		raw, err := c.ether(ctx, chainID, url.Values{"module": {"account"}, "action": {"tokenbalance"},
			"contractaddress": {contract}, "address": {address}, "tag": {"latest"}})
		if err != nil {
			return b, err
		}
		var s string
		_ = json.Unmarshal(raw, &s)
		b.USDT = units(s, dec)
		raw, err = c.ether(ctx, chainID, url.Values{"module": {"account"}, "action": {"balance"}, "address": {address}, "tag": {"latest"}})
		if err == nil {
			_ = json.Unmarshal(raw, &s)
			b.Native = units(s, 18)
		}
		return b, nil
	}
	return Balance{}, ErrUnsupported
}
