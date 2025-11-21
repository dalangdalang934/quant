package pool

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type MergedPool struct {
	AllSymbols    []string
	SymbolSources map[string][]string
}

type OITopPosition struct {
	Symbol            string  `json:"symbol"`
	Rank              int     `json:"rank"`
	OIDeltaPercent    float64 `json:"oi_delta_percent"`
	OIDeltaValue      float64 `json:"oi_delta_value"`
	PriceDeltaPercent float64 `json:"price_change_percent"`
	NetLong           float64 `json:"net_long"`
	NetShort          float64 `json:"net_short"`
}

var (
	mu              sync.RWMutex
	defaultCoins    []string
	useDefaultCoins = true
	coinPoolAPI     string
	oiTopAPI        string
	httpClient      = &http.Client{Timeout: 8 * time.Second}
)

func SetDefaultCoins(coins []string) {
	mu.Lock()
	defer mu.Unlock()
	defaultCoins = normalizeSymbols(coins)
}

func SetUseDefaultCoins(v bool) {
	mu.Lock()
	defer mu.Unlock()
	useDefaultCoins = v
}

func SetCoinPoolAPI(url string) {
	mu.Lock()
	defer mu.Unlock()
	coinPoolAPI = strings.TrimSpace(url)
}

func SetOITopAPI(url string) {
	mu.Lock()
	defer mu.Unlock()
	oiTopAPI = strings.TrimSpace(url)
}

func GetCoinPool() ([]string, error) {
	mu.RLock()
	api := coinPoolAPI
	fallback := append([]string(nil), defaultCoins...)
	allowDefault := useDefaultCoins
	mu.RUnlock()

	symbols, err := fetchSymbols(api)
	if err != nil && api != "" {
		symbols = nil
	}
	if len(symbols) == 0 && allowDefault {
		symbols = fallback
	}
	symbols = normalizeSymbols(symbols)
	if len(symbols) == 0 {
		return nil, errors.New("no symbols available")
	}
	return symbols, nil
}

func GetMergedCoinPool(limit int) (*MergedPool, error) {
	symbols, err := GetCoinPool()
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(symbols) > limit {
		symbols = symbols[:limit]
	}
	sourceMap := make(map[string][]string)
	for _, sym := range symbols {
		sourceMap[sym] = append(sourceMap[sym], "ai500")
	}

	if positions, err := GetOITopPositions(); err == nil {
		for _, pos := range positions {
			sym := strings.ToUpper(pos.Symbol)
			if _, ok := sourceMap[sym]; !ok {
				sourceMap[sym] = []string{}
			}
			if !contains(sourceMap[sym], "oi_top") {
				sourceMap[sym] = append(sourceMap[sym], "oi_top")
			}
		}
	}

	all := make([]string, 0, len(sourceMap))
	for sym := range sourceMap {
		all = append(all, sym)
	}
	sort.Strings(all)

	return &MergedPool{AllSymbols: all, SymbolSources: sourceMap}, nil
}

func GetOITopPositions() ([]OITopPosition, error) {
	mu.RLock()
	url := oiTopAPI
	mu.RUnlock()
	if url == "" {
		return nil, errors.New("oi top api not configured")
	}

	body, err := doGet(url)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	payload, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Positions []OITopPosition `json:"positions"`
	}
	if err := json.Unmarshal(payload, &resp); err == nil && len(resp.Positions) > 0 {
		return resp.Positions, nil
	}

	var arr []OITopPosition
	if err := json.Unmarshal(payload, &arr); err == nil && len(arr) > 0 {
		return arr, nil
	}

	return nil, fmt.Errorf("invalid oi top payload")
}

func fetchSymbols(url string) ([]string, error) {
	if url == "" {
		return nil, nil
	}
	body, err := doGet(url)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	payload, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Symbols []string `json:"symbols"`
		Data    []string `json:"data"`
		Coins   []string `json:"coins"`
	}
	if err := json.Unmarshal(payload, &resp); err == nil {
		switch {
		case len(resp.Symbols) > 0:
			return normalizeSymbols(resp.Symbols), nil
		case len(resp.Data) > 0:
			return normalizeSymbols(resp.Data), nil
		case len(resp.Coins) > 0:
			return normalizeSymbols(resp.Coins), nil
		}
	}

	var arr []string
	if err := json.Unmarshal(payload, &arr); err == nil && len(arr) > 0 {
		return normalizeSymbols(arr), nil
	}

	return nil, fmt.Errorf("invalid coin pool payload")
}

func doGet(url string) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func normalizeSymbols(list []string) []string {
	dedup := make(map[string]struct{})
	for _, sym := range list {
		sym = strings.ToUpper(strings.TrimSpace(sym))
		if sym == "" {
			continue
		}
		dedup[sym] = struct{}{}
	}
	out := make([]string, 0, len(dedup))
	for sym := range dedup {
		out = append(out, sym)
	}
	sort.Strings(out)
	return out
}

func contains(slice []string, target string) bool {
	for _, v := range slice {
		if v == target {
			return true
		}
	}
	return false
}
