package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"nofx/strategy"
	"os"
	"strings"
	"time"
)

// TraderConfig 单个trader的配置
type TraderConfig struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`           // 是否启用该trader
	Strategy string `json:"strategy,omitempty"` // 机制名称（如 "quant"）
	AIModel  string `json:"ai_model,omitempty"`

	// 交易平台选择（二选一）
	Exchange   string `json:"exchange"`              // "binance" or "hyperliquid"
	MarginType string `json:"margin_type,omitempty"` // 保证金模式：isolated(逐仓) / cross(全仓)

	// 币安配置
	BinanceAPIKey    string `json:"binance_api_key,omitempty"`
	BinanceSecretKey string `json:"binance_secret_key,omitempty"`

	// Hyperliquid配置
	HyperliquidPrivateKey string `json:"hyperliquid_private_key,omitempty"`
	HyperliquidWalletAddr string `json:"hyperliquid_wallet_addr,omitempty"`
	HyperliquidTestnet    bool   `json:"hyperliquid_testnet,omitempty"`

	// Aster配置
	AsterUser       string `json:"aster_user,omitempty"`        // Aster主钱包地址
	AsterSigner     string `json:"aster_signer,omitempty"`      // Aster API钱包地址
	AsterPrivateKey string `json:"aster_private_key,omitempty"` // Aster API钱包私钥

	InitialBalance      float64 `json:"initial_balance"`
	ScanIntervalMinutes int     `json:"scan_interval_minutes"`

	// 可选的币安 Websocket 流配置
	BinanceStream BinanceStreamConfig `json:"binance_stream,omitempty"`

	// 量化策略配置
	Quant strategy.QuantConfig `json:"quant_config"`
}

// BinanceStreamConfig 币安 Websocket 流配置
type BinanceStreamConfig struct {
	MarketEnabled              bool   `json:"market_enabled,omitempty"`
	UserEnabled                bool   `json:"user_enabled,omitempty"`
	MarketReconnectDelay       string `json:"market_reconnect_delay,omitempty"`
	MarketPingInterval         string `json:"market_ping_interval,omitempty"`
	UserReconnectDelay         string `json:"user_reconnect_delay,omitempty"`
	UserPingInterval           string `json:"user_ping_interval,omitempty"`
	ListenKeyRefreshInterval   string `json:"listen_key_refresh_interval,omitempty"`
}

// LeverageConfig 杠杆配置
type LeverageConfig struct {
	BTCETHLeverage  int `json:"btc_eth_leverage"` // BTC和ETH的杠杆倍数（主账户建议5-50，子账户≤5）
	AltcoinLeverage int `json:"altcoin_leverage"` // 山寨币的杠杆倍数（主账户建议5-20，子账户≤5）
}

// Config 总配置
type Config struct {
	Traders            []TraderConfig `json:"traders"`
	UseDefaultCoins    bool           `json:"use_default_coins"` // 是否使用默认主流币种列表
	DefaultCoins       []string       `json:"default_coins"`     // 默认主流币种池
	AdditionalCoins    []string       `json:"additional_coins"`  // 额外加入的币种（会并入默认池）
	CoinPoolAPIURL     string         `json:"coin_pool_api_url"`
	OITopAPIURL        string         `json:"oi_top_api_url"`
	APIServerPort      int            `json:"api_server_port"`
	MaxDailyLoss       float64        `json:"max_daily_loss"`
	MaxDrawdown        float64        `json:"max_drawdown"`
	StopTradingMinutes int            `json:"stop_trading_minutes"`
	Leverage           LeverageConfig `json:"leverage"` // 杠杆配置

	// 新闻服务配置
	NewsWebsocketURL    string `json:"news_websocket_url,omitempty"`
	NewsRSSURL          string `json:"news_rss_url,omitempty"`
	NewsStorageDir      string `json:"news_storage_dir,omitempty"`
	NewsMaxAge          string `json:"news_max_age,omitempty"`          // 例如 "2h"
	NewsPersistCooldown string `json:"news_persist_cooldown,omitempty"` // 例如 "5s"
	NewsReconnectDelay  string `json:"news_reconnect_delay,omitempty"`  // 例如 "10s"
	NewsPingInterval    string `json:"news_ping_interval,omitempty"`    // 例如 "40s"
}

// LoadConfig 从文件加载配置
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 设置默认值：如果use_default_coins未设置（为false）且没有配置coin_pool_api_url，则默认使用默认币种列表
	if !config.UseDefaultCoins && config.CoinPoolAPIURL == "" {
		config.UseDefaultCoins = true
	}

	// 设置默认币种池
	if len(config.DefaultCoins) == 0 {
		config.DefaultCoins = []string{
			"BTCUSDT",
			"ZECUSDT",
			"ETHUSDT",
			"SOLUSDT",
			"BNBUSDT",
			"XRPUSDT",
			"DOGEUSDT",
			"ADAUSDT",
			"AVAXUSDT",
			"LINKUSDT",
			"DOTUSDT",
			"ATOMUSDT",
			"LTCUSDT",
			"ARBUSDT",
			"SUIUSDT",
			"APTUSDT",
			"NEARUSDT",
			"FILUSDT",
			"UNIUSDT",
			"AAVEUSDT",
			"JUPUSDT",
			"INJUSDT",
			"AIAUSDT",
			"OPUSDT",
			"HYPEUSDT",
		}
	}

	config.DefaultCoins = mergeCoinLists(config.DefaultCoins, config.AdditionalCoins)

	// 规范化策略配置
	for i := range config.Traders {
		trader := &config.Traders[i]
		if trader.Strategy == "" {
			trader.Strategy = strings.ToLower(strings.TrimSpace(trader.AIModel))
		}
		if trader.Strategy == "" {
			trader.Strategy = "quant"
		}
		trader.AIModel = trader.Strategy
		trader.Quant.Normalize()
	}

	if err := ensureFuturesSymbolsExist(config.DefaultCoins); err != nil {
		return nil, err
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return &config, nil
}

// Validate 验证配置有效性
func (c *Config) Validate() error {
	if len(c.Traders) == 0 {
		return fmt.Errorf("至少需要配置一个trader")
	}

	traderIDs := make(map[string]bool)
	for i := range c.Traders {
		trader := &c.Traders[i]
		if trader.ID == "" {
			return fmt.Errorf("trader[%d]: ID不能为空", i)
		}
		if traderIDs[trader.ID] {
			return fmt.Errorf("trader[%d]: ID '%s' 重复", i, trader.ID)
		}
		traderIDs[trader.ID] = true

		if trader.Name == "" {
			return fmt.Errorf("trader[%d]: Name不能为空", i)
		}
		if trader.Strategy == "" {
			trader.Strategy = "quant"
		}
		if trader.Strategy != "quant" {
			return fmt.Errorf("trader[%d]: strategy必须为 'quant'", i)
		}
		trader.AIModel = trader.Strategy

		// 验证交易平台配置
		if trader.Exchange == "" {
			trader.Exchange = "binance" // 默认使用币安
		}
		if trader.Exchange != "binance" && trader.Exchange != "hyperliquid" && trader.Exchange != "aster" {
			return fmt.Errorf("trader[%d]: exchange必须是 'binance', 'hyperliquid' 或 'aster'", i)
		}

		// 规范化保证金模式（仅对币安合约生效，默认逐仓）
		marginType := strings.ToLower(strings.TrimSpace(trader.MarginType))
		switch marginType {
		case "", "isolated":
			trader.MarginType = "isolated"
		case "cross", "crossed", "cross_margin", "crossed_margin":
			trader.MarginType = "cross"
		default:
			return fmt.Errorf("trader[%d]: margin_type必须是 'isolated' 或 'cross'", i)
		}

		// 根据平台验证对应的密钥
		if trader.Exchange == "binance" {
			if trader.BinanceAPIKey == "" || trader.BinanceSecretKey == "" {
				return fmt.Errorf("trader[%d]: 使用币安时必须配置binance_api_key和binance_secret_key", i)
			}
		} else if trader.Exchange == "hyperliquid" {
			if trader.HyperliquidPrivateKey == "" {
				return fmt.Errorf("trader[%d]: 使用Hyperliquid时必须配置hyperliquid_private_key", i)
			}
		} else if trader.Exchange == "aster" {
			if trader.AsterUser == "" || trader.AsterSigner == "" || trader.AsterPrivateKey == "" {
				return fmt.Errorf("trader[%d]: 使用Aster时必须配置aster_user, aster_signer和aster_private_key", i)
			}
		}

		if trader.InitialBalance <= 0 {
			return fmt.Errorf("trader[%d]: initial_balance必须大于0", i)
		}
		if trader.ScanIntervalMinutes <= 0 {
			trader.ScanIntervalMinutes = 2 // 默认2分钟
		}
	}

	if c.APIServerPort <= 0 {
		c.APIServerPort = 8080 // 默认8080端口
	}

	// 设置杠杆默认值（适配币安子账户限制，最大5倍）
	if c.Leverage.BTCETHLeverage <= 0 {
		c.Leverage.BTCETHLeverage = 5 // 默认5倍（安全值，适配子账户）
	}
	if c.Leverage.BTCETHLeverage > 5 {
		fmt.Printf("⚠️  警告: BTC/ETH杠杆设置为%dx，如果使用子账户可能会失败（子账户限制≤5x）\n", c.Leverage.BTCETHLeverage)
	}
	if c.Leverage.AltcoinLeverage <= 0 {
		c.Leverage.AltcoinLeverage = 5 // 默认5倍（安全值，适配子账户）
	}
	if c.Leverage.AltcoinLeverage > 5 {
		fmt.Printf("⚠️  警告: 山寨币杠杆设置为%dx，如果使用子账户可能会失败（子账户限制≤5x）\n", c.Leverage.AltcoinLeverage)
	}

	return nil
}

// GetScanInterval 获取扫描间隔
func (tc *TraderConfig) GetScanInterval() time.Duration {
	return time.Duration(tc.ScanIntervalMinutes) * time.Minute
}

func mergeCoinLists(base, extra []string) []string {
	seen := make(map[string]bool)
	var merged []string

	appendCoin := func(symbol string) {
		normalized := normalizeCoinSymbol(symbol)
		if normalized == "" {
			return
		}
		if !seen[normalized] {
			seen[normalized] = true
			merged = append(merged, normalized)
		}
	}

	for _, coin := range base {
		appendCoin(coin)
	}
	for _, coin := range extra {
		appendCoin(coin)
	}

	return merged
}

func normalizeCoinSymbol(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	if strings.Contains(s, "/") {
		s = strings.ReplaceAll(s, "/", "")
	}
	return s
}

func ensureFuturesSymbolsExist(symbols []string) error {
	list, err := fetchBinanceFuturesSymbols()
	if err != nil {
		return fmt.Errorf("验证币种时获取交易对清单失败: %w", err)
	}

	var missing []string
	for _, raw := range symbols {
		symbol := normalizeCoinSymbol(raw)
		if symbol == "" {
			continue
		}
		if !list[symbol] {
			missing = append(missing, symbol)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("以下币种在币安永续合约中不可用或已下架: %s", strings.Join(missing, ", "))
	}

	return nil
}

func fetchBinanceFuturesSymbols() (map[string]bool, error) {
    const maxAttempts = 3
    var lastErr error
    for attempt := 1; attempt <= maxAttempts; attempt++ {
        result, err := fetchBinanceFuturesSymbolsOnce()
        if err == nil {
            return result, nil
        }
        lastErr = err
        if attempt < maxAttempts {
            time.Sleep(time.Duration(attempt) * 2 * time.Second)
        }
    }
    return nil, fmt.Errorf("获取币安交易对清单失败: %w", lastErr)
}

func fetchBinanceFuturesSymbolsOnce() (map[string]bool, error) {
    client := &http.Client{Timeout: 25 * time.Second}
    resp, err := client.Get("https://fapi.binance.com/fapi/v1/exchangeInfo")
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, string(body))
    }

    var payload struct {
        Symbols []struct {
            Symbol       string `json:"symbol"`
            ContractType string `json:"contractType"`
            Status       string `json:"status"`
        } `json:"symbols"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
        return nil, err
    }

    result := make(map[string]bool, len(payload.Symbols))
    for _, item := range payload.Symbols {
        if !strings.EqualFold(item.ContractType, "PERPETUAL") {
            continue
        }
        if !strings.EqualFold(item.Status, "TRADING") {
            continue
        }
        result[strings.ToUpper(item.Symbol)] = true
    }

    return result, nil
}
