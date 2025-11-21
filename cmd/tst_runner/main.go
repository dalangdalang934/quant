package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"nofx/trader"

	"github.com/adshao/go-binance/v2/futures"
)

// StrategyConfig 映射 tst.pine 的可调参数
type StrategyConfig struct {
	Symbol        string
	Interval      string
	FastLength    int
	SlowLength    int
	ATRLength     int
	UseATRFilter  bool
	MinVolatility float64
	PositionPct   float64
	Leverage      int
	PollInterval  time.Duration
	MarginMode    string
}

type candle struct {
	OpenTime  int64
	CloseTime int64
	Open      float64
	High      float64
	Low       float64
	Close     float64
}

type strategySignal struct {
	LongEntry    bool
	ShortEntry   bool
	LongExit     bool
	ShortExit    bool
	VolatilityOK bool
	FastNow      float64
	SlowNow      float64
	ATRRation    float64
}

type runner struct {
	cfg          StrategyConfig
	trader       *trader.FuturesTrader
	marketClient *futures.Client
	lastClose    int64
}

func main() {
	var (
		symbol       = flag.String("symbol", "ETHUSDT", "交易对（必须是币安USDT永续支持的符号）")
		interval     = flag.String("interval", "5m", "K线周期，例如 1m/5m/15m")
		fastLen      = flag.Int("fast", 12, "快速均线周期")
		slowLen      = flag.Int("slow", 36, "慢速均线周期")
		atrLen       = flag.Int("atr", 14, "ATR 周期")
		useATR       = flag.Bool("use_atr", true, "启用ATR波动率过滤")
		minVol       = flag.Float64("min_vol", 0.0005, "ATR/Close 最小比例（0.0005=0.05%）")
		positionPct  = flag.Float64("position_pct", 1.0, "单笔仓位占账户净值比例(0-1)")
		leverage     = flag.Int("leverage", 3, "使用的杠杆倍数")
		pollInterval = flag.Duration("poll_interval", 15*time.Second, "轮询频率（建议15s~30s）")
		marginMode   = flag.String("margin_mode", "isolated", "默认保证金模式 isolated/cross")
		useTestnet   = flag.Bool("testnet", true, "是否连接币安合约测试网")
	)
	flag.Parse()

	apiKey := strings.TrimSpace(os.Getenv("BINANCE_API_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("BINANCE_SECRET_KEY"))
	if apiKey == "" || secretKey == "" {
		log.Fatalf("必须通过环境变量 BINANCE_API_KEY / BINANCE_SECRET_KEY 提供密钥（测试网也需要单独申请）")
	}

	if *fastLen <= 1 || *slowLen <= *fastLen {
		log.Fatalf("参数错误: fast(%d) 必须大于1且小于 slow(%d)", *fastLen, *slowLen)
	}
	if *atrLen <= 1 {
		log.Fatalf("参数错误: atr(%d) 必须大于1", *atrLen)
	}
	if *positionPct <= 0 || *positionPct > 1.5 {
		log.Fatalf("参数错误: position_pct=%.2f, 建议范围 0-1", *positionPct)
	}
	if *leverage <= 0 {
		log.Fatalf("参数错误: leverage 必须大于0")
	}

	if *useTestnet {
		futures.UseTestnet = true
		log.Printf("🌐 已启用币安永续合约测试网环境")
	}

	cfg := StrategyConfig{
		Symbol:        strings.ToUpper(strings.TrimSpace(*symbol)),
		Interval:      *interval,
		FastLength:    *fastLen,
		SlowLength:    *slowLen,
		ATRLength:     *atrLen,
		UseATRFilter:  *useATR,
		MinVolatility: *minVol,
		PositionPct:   *positionPct,
		Leverage:      *leverage,
		PollInterval:  *pollInterval,
		MarginMode:    *marginMode,
	}

	ft := trader.NewFuturesTrader(apiKey, secretKey)
	ft.SetDefaultMarginMode(cfg.MarginMode)

	marketClient := futures.NewClient("", "")
	if *useTestnet {
		// 部分测试网端点需要显式密钥
		marketClient = futures.NewClient(apiKey, secretKey)
	}

	run := &runner{
		cfg:          cfg,
		trader:       ft,
		marketClient: marketClient,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("🚀 启动 tst.pine 等价策略: %s @ %s | fast=%d slow=%d atr=%d position=%.0f%% leverage=%dx useATR=%v",
		cfg.Symbol, cfg.Interval, cfg.FastLength, cfg.SlowLength, cfg.ATRLength, cfg.PositionPct*100, cfg.Leverage, cfg.UseATRFilter)

	if err := run.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("策略运行异常: %v", err)
	}
}

func (r *runner) Run(ctx context.Context) error {
	// 立即执行一次，随后按轮询间隔执行
	if err := r.tick(ctx); err != nil {
		log.Printf("⚠️ 首次执行失败: %v", err)
	}

	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := r.tick(ctx); err != nil {
				log.Printf("⚠️ tick 执行失败: %v", err)
			}
		}
	}
}

func (r *runner) tick(ctx context.Context) error {
	candles, err := r.fetchRecentCandles(ctx)
	if err != nil {
		return err
	}
	if len(candles) < r.cfg.SlowLength+2 {
		return fmt.Errorf("K线数量不足以计算指标: got=%d need>=%d", len(candles), r.cfg.SlowLength+2)
	}

	last := candles[len(candles)-1]
	if last.CloseTime <= r.lastClose {
		return nil // 没有新K线
	}
	r.lastClose = last.CloseTime

	signal, err := evaluateSignal(candles, r.cfg)
	if err != nil {
		return err
	}

	closeTime := time.UnixMilli(last.CloseTime).UTC()
	log.Printf("🕒 %s | close=%.6f fast=%.6f slow=%.6f ATR%%=%.4f volOK=%v",
		closeTime.Format("2006-01-02 15:04:05"),
		last.Close,
		signal.FastNow,
		signal.SlowNow,
		signal.ATRRation*100,
		signal.VolatilityOK,
	)

	side, qty, err := r.currentPosition()
	if err != nil {
		return err
	}
	log.Printf("📌 当前持仓: %s qty=%.4f", side, qty)

	if side == "long" && signal.LongExit {
		if err := r.closeLong(); err != nil {
			return err
		}
		side = "none"
	}
	if side == "short" && signal.ShortExit {
		if err := r.closeShort(); err != nil {
			return err
		}
		side = "none"
	}

	if !signal.VolatilityOK {
		log.Printf("⚠️ 波动率条件未满足，跳过开仓")
		return nil
	}

	if side == "none" {
		switch {
		case signal.LongEntry:
			if err := r.openLong(last.Close); err != nil {
				return err
			}
		case signal.ShortEntry:
			if err := r.openShort(last.Close); err != nil {
				return err
			}
		default:
			log.Printf("⏸ 没有新的交易信号")
		}
	} else {
		log.Printf("⏸ 保持现有持仓: %s", side)
	}

	return nil
}

func (r *runner) fetchRecentCandles(ctx context.Context) ([]candle, error) {
	window := maxInt(maxInt(r.cfg.SlowLength, r.cfg.ATRLength)+5, 100)
	raw, err := r.marketClient.NewKlinesService().
		Symbol(r.cfg.Symbol).
		Interval(r.cfg.Interval).
		Limit(window).
		Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取K线失败: %w", err)
	}

	candles := make([]candle, 0, len(raw))
	for _, k := range raw {
		open, err := parseFloat(k.Open)
		if err != nil {
			return nil, fmt.Errorf("解析open失败: %w", err)
		}
		high, err := parseFloat(k.High)
		if err != nil {
			return nil, fmt.Errorf("解析high失败: %w", err)
		}
		low, err := parseFloat(k.Low)
		if err != nil {
			return nil, fmt.Errorf("解析low失败: %w", err)
		}
		closePrice, err := parseFloat(k.Close)
		if err != nil {
			return nil, fmt.Errorf("解析close失败: %w", err)
		}

		candles = append(candles, candle{
			OpenTime:  k.OpenTime,
			CloseTime: k.CloseTime,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     closePrice,
		})
	}

	return candles, nil
}

func evaluateSignal(candles []candle, cfg StrategyConfig) (strategySignal, error) {
	closes := make([]float64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
	}

	lastIdx := len(closes) - 1
	prevIdx := lastIdx - 1

	fastNow := sma(closes, cfg.FastLength, lastIdx)
	fastPrev := sma(closes, cfg.FastLength, prevIdx)
	slowNow := sma(closes, cfg.SlowLength, lastIdx)
	slowPrev := sma(closes, cfg.SlowLength, prevIdx)

	if math.IsNaN(fastNow) || math.IsNaN(slowNow) || math.IsNaN(fastPrev) || math.IsNaN(slowPrev) {
		return strategySignal{}, fmt.Errorf("无法计算均线 (fastLen=%d slowLen=%d)", cfg.FastLength, cfg.SlowLength)
	}

	longEntry := fastPrev <= slowPrev && fastNow > slowNow
	shortEntry := fastPrev >= slowPrev && fastNow < slowNow
	longExit := fastPrev >= slowPrev && fastNow < slowNow
	shortExit := fastPrev <= slowPrev && fastNow > slowNow

	var atrRatio float64
	if cfg.UseATRFilter {
		atr := calcATR(candles, cfg.ATRLength)
		if math.IsNaN(atr) {
			return strategySignal{}, fmt.Errorf("无法计算ATR (len=%d)", cfg.ATRLength)
		}
		atrRatio = atr / candles[lastIdx].Close
	} else {
		atrRatio = math.MaxFloat64
	}

	volOK := !cfg.UseATRFilter || atrRatio > cfg.MinVolatility

	return strategySignal{
		LongEntry:    longEntry,
		ShortEntry:   shortEntry,
		LongExit:     longExit,
		ShortExit:    shortExit,
		VolatilityOK: volOK,
		FastNow:      fastNow,
		SlowNow:      slowNow,
		ATRRation:    atrRatio,
	}, nil
}

func (r *runner) currentPosition() (string, float64, error) {
	positions, err := r.trader.GetPositions()
	if err != nil {
		return "", 0, fmt.Errorf("获取持仓失败: %w", err)
	}

	for _, pos := range positions {
		symbol, _ := pos["symbol"].(string)
		if !strings.EqualFold(symbol, r.cfg.Symbol) {
			continue
		}

		side, _ := pos["side"].(string)
		qty, _ := pos["positionAmt"].(float64)
		if qty == 0 {
			continue
		}
		if side == "short" && qty < 0 {
			qty = -qty
		}
		return side, qty, nil
	}

	return "none", 0, nil
}

func (r *runner) openLong(price float64) error {
	quantity, err := r.calculateQuantity(price)
	if err != nil {
		return err
	}
	log.Printf("📈 开多 %s qty=%.4f (price=%.4f)", r.cfg.Symbol, quantity, price)
	if _, err := r.trader.OpenLong(r.cfg.Symbol, quantity, r.cfg.Leverage); err != nil {
		return err
	}
	r.trader.ClearPositionsCache()
	return nil
}

func (r *runner) openShort(price float64) error {
	quantity, err := r.calculateQuantity(price)
	if err != nil {
		return err
	}
	log.Printf("📉 开空 %s qty=%.4f (price=%.4f)", r.cfg.Symbol, quantity, price)
	if _, err := r.trader.OpenShort(r.cfg.Symbol, quantity, r.cfg.Leverage); err != nil {
		return err
	}
	r.trader.ClearPositionsCache()
	return nil
}

func (r *runner) closeLong() error {
	log.Printf("🔄 平多 %s", r.cfg.Symbol)
	if _, err := r.trader.CloseLong(r.cfg.Symbol, 0); err != nil {
		return err
	}
	r.trader.ClearPositionsCache()
	return nil
}

func (r *runner) closeShort() error {
	log.Printf("🔄 平空 %s", r.cfg.Symbol)
	if _, err := r.trader.CloseShort(r.cfg.Symbol, 0); err != nil {
		return err
	}
	r.trader.ClearPositionsCache()
	return nil
}

func (r *runner) calculateQuantity(price float64) (float64, error) {
	balance, err := r.trader.GetBalance()
	if err != nil {
		return 0, fmt.Errorf("获取账户余额失败: %w", err)
	}

	totalWallet := toFloat(balance["totalWalletBalance"])
	unrealized := toFloat(balance["totalUnrealizedProfit"])
	equity := totalWallet + unrealized
	if equity <= 0 {
		return 0, fmt.Errorf("账户净值为0，无法开仓")
	}

	positionValue := equity * r.cfg.PositionPct
	if positionValue <= 0 {
		return 0, fmt.Errorf("计算得到的仓位价值<=0")
	}

	quantity := positionValue / price
	if quantity <= 0 {
		return 0, fmt.Errorf("计算得到的下单数量<=0")
	}

	return quantity, nil
}

func sma(values []float64, length, endIdx int) float64 {
	if endIdx < 0 || endIdx >= len(values) {
		return math.NaN()
	}
	start := endIdx - length + 1
	if start < 0 {
		return math.NaN()
	}
	sum := 0.0
	for i := start; i <= endIdx; i++ {
		sum += values[i]
	}
	return sum / float64(length)
}

func calcATR(candles []candle, length int) float64 {
	if length <= 0 || len(candles) < length+1 {
		return math.NaN()
	}

	start := len(candles) - length
	prevClose := candles[start-1].Close
	total := 0.0
	for i := start; i < len(candles); i++ {
		current := candles[i]
		tr1 := current.High - current.Low
		tr2 := math.Abs(current.High - prevClose)
		tr3 := math.Abs(current.Low - prevClose)
		tr := math.Max(tr1, math.Max(tr2, tr3))
		total += tr
		prevClose = current.Close
	}
	return total / float64(length)
}

func parseFloat(val string) (float64, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
	if err != nil {
		return 0, err
	}
	return f, nil
}

func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(val), 64)
		return f
	default:
		return 0
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
