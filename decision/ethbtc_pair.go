package decision

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	pairFastPeriod     = 21
	pairSlowPeriod     = 55
	pairStopLossPct    = 0.03
	pairTakeProfitPct  = 0.06
	pairKlineInterval  = "30m"
	pairKlineFetchSize = 120
)

func buildEthBtcPairDecisions(ctx *Context) []Decision {
	defaultSize := ctx.Account.TotalEquity * ctx.Strategy.PositionSizePct
	if defaultSize <= 0 {
		return nil
	}

	closes, err := fetchEthBtcCloses(pairKlineFetchSize)
	if err != nil {
		return nil
	}
	if len(closes) < pairSlowPeriod+1 {
		return nil
	}

	fastNow, ok := sma(closes, pairFastPeriod, 0)
	if !ok {
		return nil
	}
	fastPrev, ok := sma(closes, pairFastPeriod, 1)
	if !ok {
		return nil
	}
	slowNow, ok := sma(closes, pairSlowPeriod, 0)
	if !ok {
		return nil
	}
	slowPrev, ok := sma(closes, pairSlowPeriod, 1)
	if !ok {
		return nil
	}

	fastAboveNow := fastNow > slowNow
	fastAbovePrev := fastPrev > slowPrev
	pairDirection := detectPairDirection(ctx)

	var decisions []Decision
	if exitReason := pairExitReason(ctx, pairDirection, fastAboveNow); exitReason != "" {
		decisions = append(decisions, buildPairCloseDecisions(ctx, pairDirection, exitReason)...)
		// 避免同一周期内立即反手，等待下一轮处理
		return decisions
	}

	if pairDirection != "" {
		return decisions
	}

	// 未持有配对仓位，寻找入场
	// 若策略整体禁止做空，则无法执行配对（两种方向均需要一条腿做空）
	if !ctx.Strategy.AllowShort {
		return decisions
	}

	switch {
	case fastAboveNow && !fastAbovePrev:
		if d, ok := buildPairOpenDecision(ctx, "ETHUSDT", "open_long", "ETH/BTC 21/55 金叉，多头对冲启动", defaultSize); ok {
			decisions = append(decisions, d)
		}
		if d, ok := buildPairOpenDecision(ctx, "BTCUSDT", "open_short", "ETH/BTC 21/55 金叉，多头对冲启动", defaultSize); ok {
			decisions = append(decisions, d)
		}
	case !fastAboveNow && fastAbovePrev:
		if d, ok := buildPairOpenDecision(ctx, "ETHUSDT", "open_short", "ETH/BTC 21/55 死叉，空头对冲启动", defaultSize); ok {
			decisions = append(decisions, d)
		}
		if d, ok := buildPairOpenDecision(ctx, "BTCUSDT", "open_long", "ETH/BTC 21/55 死叉，空头对冲启动", defaultSize); ok {
			decisions = append(decisions, d)
		}
	}

	return decisions
}

func pairExitReason(ctx *Context, direction string, fastAboveNow bool) string {
	if direction == "" {
		return ""
	}

	if ctx.PairState != nil && ctx.PairState.Active && ctx.PairState.EntryEquity > 0 {
		pnlPct := (ctx.Account.TotalEquity - ctx.PairState.EntryEquity) / ctx.PairState.EntryEquity
		if pnlPct <= -pairStopLossPct {
			return fmt.Sprintf("ETH/BTC 配对组合净值回撤 %.2f%% 触发 %.0f%% 止损", pnlPct*100, pairStopLossPct*100)
		}
		if pnlPct >= pairTakeProfitPct {
			return fmt.Sprintf("ETH/BTC 配对组合净值上涨 %.2f%% 达到 %.0f%% 止盈", pnlPct*100, pairTakeProfitPct*100)
		}
	}

	switch direction {
	case PairDirectionLong:
		if !fastAboveNow {
			return "ETH/BTC 均线下穿，结束多头对冲"
		}
	case PairDirectionShort:
		if fastAboveNow {
			return "ETH/BTC 均线上穿，结束空头对冲"
		}
	}
	return ""
}

func buildPairCloseDecisions(ctx *Context, direction, reason string) []Decision {
	if direction == "" {
		return nil
	}
	var decisions []Decision
	switch direction {
	case PairDirectionLong:
		if pos := findPositionInfo(ctx, "ETHUSDT", "long"); pos != nil {
			decisions = append(decisions, Decision{
				Symbol:      pos.Symbol,
				Action:      "close_long",
				Reasoning:   reason,
				PositionID:  pos.PositionID,
				StrategyTag: StrategyTagEthBtcPair,
			})
		}
		if pos := findPositionInfo(ctx, "BTCUSDT", "short"); pos != nil {
			decisions = append(decisions, Decision{
				Symbol:      pos.Symbol,
				Action:      "close_short",
				Reasoning:   reason,
				PositionID:  pos.PositionID,
				StrategyTag: StrategyTagEthBtcPair,
			})
		}
	case PairDirectionShort:
		if pos := findPositionInfo(ctx, "ETHUSDT", "short"); pos != nil {
			decisions = append(decisions, Decision{
				Symbol:      pos.Symbol,
				Action:      "close_short",
				Reasoning:   reason,
				PositionID:  pos.PositionID,
				StrategyTag: StrategyTagEthBtcPair,
			})
		}
		if pos := findPositionInfo(ctx, "BTCUSDT", "long"); pos != nil {
			decisions = append(decisions, Decision{
				Symbol:      pos.Symbol,
				Action:      "close_long",
				Reasoning:   reason,
				PositionID:  pos.PositionID,
				StrategyTag: StrategyTagEthBtcPair,
			})
		}
	}
	return decisions
}

func buildPairOpenDecision(ctx *Context, symbol, action, reason string, defaultSize float64) (Decision, bool) {
	data, ok := ctx.MarketDataMap[symbol]
	if !ok || data == nil || data.CurrentPrice <= 0 {
		return Decision{}, false
	}

	decision := Decision{
		Symbol:          symbol,
		Action:          action,
		Reasoning:       reason,
		StrategyTag:     StrategyTagEthBtcPair,
		Leverage:        leverageForSymbol(symbol, ctx),
		PositionSizeUSD: defaultSize,
		Confidence:      72,
	}

	side := "long"
	if action == "open_short" {
		side = "short"
	}
	atr := estimateATR(data)
	decision.StopLoss, decision.TakeProfit = computeStops(side, data.CurrentPrice, atr, ctx.Strategy)
	decision.RiskUSD = decision.PositionSizeUSD / float64(decision.Leverage)
	if math.IsNaN(decision.RiskUSD) || math.IsInf(decision.RiskUSD, 0) {
		return Decision{}, false
	}
	return decision, true
}

func detectPairDirection(ctx *Context) string {
	hasEthLong := findPositionInfo(ctx, "ETHUSDT", "long") != nil
	hasEthShort := findPositionInfo(ctx, "ETHUSDT", "short") != nil
	hasBtcLong := findPositionInfo(ctx, "BTCUSDT", "long") != nil
	hasBtcShort := findPositionInfo(ctx, "BTCUSDT", "short") != nil

	switch {
	case hasEthLong && hasBtcShort:
		return PairDirectionLong
	case hasEthShort && hasBtcLong:
		return PairDirectionShort
	default:
		return ""
	}
}

func findPositionInfo(ctx *Context, symbol, side string) *PositionInfo {
	for i := range ctx.Positions {
		pos := &ctx.Positions[i]
		if !strings.EqualFold(pos.Symbol, symbol) {
			continue
		}
		if !strings.EqualFold(pos.Side, side) {
			continue
		}
		if pos.Quantity <= 0 {
			continue
		}
		return pos
	}
	return nil
}

func fetchEthBtcCloses(limit int) ([]float64, error) {
	url := fmt.Sprintf("https://data.binance.com/api/v3/klines?symbol=ETHBTC&interval=%s&limit=%d", pairKlineInterval, limit)
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch ETHBTC klines failed: %s %s", resp.Status, string(body))
	}

	var raw [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	closes := make([]float64, 0, len(raw))
	for _, item := range raw {
		if len(item) < 5 {
			continue
		}
		closes = append(closes, parseFloat(item[4]))
	}
	return closes, nil
}

func sma(values []float64, period int, shift int) (float64, bool) {
	if period <= 0 || shift < 0 {
		return 0, false
	}
	end := len(values) - shift
	start := end - period
	if start < 0 || end > len(values) {
		return 0, false
	}
	sum := 0.0
	for i := start; i < end; i++ {
		sum += values[i]
	}
	return sum / float64(period), true
}

func parseFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case string:
		num, _ := strconv.ParseFloat(val, 64)
		return num
	default:
		return 0
	}
}
