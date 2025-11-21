package decision

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"nofx/market"
	"nofx/pool"
	"nofx/strategy"
	"sort"
	"strings"
	"time"
)

// PositionInfo 持仓信息
type PositionInfo struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"` // "long" or "short"
	EntryPrice       float64 `json:"entry_price"`
	MarkPrice        float64 `json:"mark_price"`
	Quantity         float64 `json:"quantity"`
	Leverage         int     `json:"leverage"`
	UnrealizedPnL    float64 `json:"unrealized_pnl"`
	UnrealizedPnLPct float64 `json:"unrealized_pnl_pct"`
	LiquidationPrice float64 `json:"liquidation_price"`
	MarginUsed       float64 `json:"margin_used"`
	UpdateTime       int64   `json:"update_time"`           // 持仓更新时间戳（毫秒）
	PositionID       string  `json:"position_id,omitempty"` // 关联的仓位ID
}

// AccountInfo 账户信息
type AccountInfo struct {
	TotalEquity      float64 `json:"total_equity"`      // 账户净值
	AvailableBalance float64 `json:"available_balance"` // 可用余额
	TotalPnL         float64 `json:"total_pnl"`         // 总盈亏
	TotalPnLPct      float64 `json:"total_pnl_pct"`     // 总盈亏百分比
	MarginUsed       float64 `json:"margin_used"`       // 已用保证金
	MarginUsedPct    float64 `json:"margin_used_pct"`   // 保证金使用率
	PositionCount    int     `json:"position_count"`    // 持仓数量
}

// CandidateCoin 候选币种（来自币种池）
type CandidateCoin struct {
	Symbol  string   `json:"symbol"`
	Sources []string `json:"sources"` // 来源: "ai500" 和/或 "oi_top"
}

// OITopData 持仓量增长Top数据（用于决策参考）
type OITopData struct {
	Rank              int     // OI Top排名
	OIDeltaPercent    float64 // 持仓量变化百分比（1小时）
	OIDeltaValue      float64 // 持仓量变化价值
	PriceDeltaPercent float64 // 价格变化百分比
	NetLong           float64 // 净多仓
	NetShort          float64 // 净空仓
}

// Context 交易上下文（传递给量化引擎的完整信息）
type Context struct {
	CurrentTime     string                  `json:"current_time"`
	RuntimeMinutes  int                     `json:"runtime_minutes"`
	CallCount       int                     `json:"call_count"`
	Account         AccountInfo             `json:"account"`
	Positions       []PositionInfo          `json:"positions"`
	CandidateCoins  []CandidateCoin         `json:"candidate_coins"`
	MarketDataMap   map[string]*market.Data `json:"-"` // 不序列化，但内部使用
	OITopDataMap    map[string]*OITopData   `json:"-"` // OI Top数据映射
	Performance     interface{}             `json:"-"` // 历史表现分析（logger.PerformanceAnalysis）
	BTCETHLeverage  int                     `json:"-"` // BTC/ETH杠杆倍数（从配置读取）
	AltcoinLeverage int                     `json:"-"` // 山寨币杠杆倍数（从配置读取）
	Strategy        strategy.QuantConfig    `json:"-"`
	PairState       *PairState              `json:"-"`
}

// Decision 量化策略生成的交易指令
type Decision struct {
	Symbol          string  `json:"symbol"`
	Action          string  `json:"action"` // "open_long", "open_short", "close_long", "close_short", "hold", "wait"
	Leverage        int     `json:"leverage,omitempty"`
	PositionSizeUSD float64 `json:"position_size_usd,omitempty"`
	StopLoss        float64 `json:"stop_loss,omitempty"`
	TakeProfit      float64 `json:"take_profit,omitempty"`
	Confidence      int     `json:"confidence,omitempty"` // 信心度 (0-100)
	RiskUSD         float64 `json:"risk_usd,omitempty"`   // 最大美元风险
	Reasoning       string  `json:"reasoning"`
	PositionID      string  `json:"position_id,omitempty"` // 关联的仓位ID
	StrategyTag     string  `json:"strategy_tag,omitempty"`
}

// FullDecision 完整的策略输出（包含解释）
type FullDecision struct {
	UserPrompt string     `json:"user_prompt"` // 给策略的输入概览
	CoTTrace   string     `json:"cot_trace"`   // 决策过程说明
	Decisions  []Decision `json:"decisions"`   // 具体决策列表
	Timestamp  time.Time  `json:"timestamp"`
}

// GetFullDecision 生成量化因子驱动的完整交易决策
func GetFullDecision(ctx *Context) (*FullDecision, error) {
	if ctx == nil {
		return nil, fmt.Errorf("上下文为空")
	}

	// 1. 拉取最新市场数据
	if err := fetchMarketDataForContext(ctx); err != nil {
		return nil, fmt.Errorf("获取市场数据失败: %w", err)
	}

	// 2. 规范化策略参数
	ctx.Strategy.Normalize()

	// 3. 评估候选币种的多空得分
	scores := buildQuantScores(ctx)
	if len(scores) == 0 {
		return nil, fmt.Errorf("缺少可分析的市场数据")
	}

	// 4. 根据得分与仓位状况生成交易计划
	decisions := buildQuantDecisions(ctx, scores)
	if pairDecisions := buildEthBtcPairDecisions(ctx); len(pairDecisions) > 0 {
		decisions = append(decisions, pairDecisions...)
	}

	// 5. 构建可读的分析报告
	narrative := buildQuantNarrative(ctx, scores, decisions)

	return &FullDecision{
		UserPrompt: narrative.Overview,
		CoTTrace:   narrative.Detail,
		Decisions:  decisions,
		Timestamp:  time.Now(),
	}, nil
}

type quantScore struct {
	Symbol     string
	Price      float64
	ATR        float64
	LongScore  float64
	ShortScore float64
	LongNotes  []string
	ShortNotes []string
	Order      int
	Data       *market.Data
}

type quantNarrative struct {
	Overview string
	Detail   string
}

func buildQuantScores(ctx *Context) []quantScore {
	order := make(map[string]int)
	for idx, coin := range ctx.CandidateCoins {
		order[strings.ToUpper(coin.Symbol)] = idx
	}

	scores := make([]quantScore, 0, len(ctx.MarketDataMap))
	for symbol, data := range ctx.MarketDataMap {
		score := quantScore{
			Symbol: symbol,
			Price:  data.CurrentPrice,
			Data:   data,
			Order:  order[strings.ToUpper(symbol)],
		}
		score.ATR = estimateATR(data)
		evaluateQuantSignals(&score)
		scores = append(scores, score)
	}

	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].Order != scores[j].Order {
			return scores[i].Order < scores[j].Order
		}
		if math.Abs(scores[i].LongScore-scores[j].LongScore) > 1e-6 {
			return scores[i].LongScore > scores[j].LongScore
		}
		return scores[i].ShortScore > scores[j].ShortScore
	})

	return scores
}

func evaluateQuantSignals(score *quantScore) {
	data := score.Data
	price := data.CurrentPrice
	if price <= 0 {
		return
	}

	pushLong := func(val float64, note string) {
		score.LongScore += val
		score.LongNotes = append(score.LongNotes, note)
	}
	pushShort := func(val float64, note string) {
		score.ShortScore += val
		score.ShortNotes = append(score.ShortNotes, note)
	}

	if data.CurrentEMA20 > 0 {
		if price > data.CurrentEMA20 {
			pushLong(1.0, "价格站上EMA20(+1.0)")
		} else {
			pushShort(1.0, "价格跌破EMA20(+1.0)")
		}
	}

	if ctx := data.MediumTermContext; ctx != nil {
		if ctx.EMA50 > 0 {
			if price > ctx.EMA50 {
				pushLong(0.8, "高于EMA50(+0.8)")
			} else {
				pushShort(0.8, "低于EMA50(+0.8)")
			}
		}
		if ctx.RSI14 > 70 {
			pushShort(0.6, "RSI14>70 过热(+0.6空)")
		} else if ctx.RSI14 < 35 {
			pushLong(0.6, "RSI14<35 超卖(+0.6多)")
		}
	}

	if ctx := data.LongerTermContext; ctx != nil {
		if ctx.EMA100 > 0 && price > ctx.EMA100 {
			pushLong(0.4, "高于EMA100(+0.4)")
		}
		if ctx.EMA200 > 0 && price < ctx.EMA200 {
			pushShort(0.5, "低于EMA200(+0.5)")
		}
		if ctx.ADX > 25 {
			pushLong(0.2, "ADX>25 趋势清晰(+0.2)")
		}
	}

	if data.PriceChange1h > 0.25 {
		pushLong(0.4, fmt.Sprintf("1h涨幅%.2f%%(+0.4)", data.PriceChange1h))
	} else if data.PriceChange1h < -0.25 {
		pushShort(0.4, fmt.Sprintf("1h跌幅%.2f%%(+0.4)", -data.PriceChange1h))
	}

	if data.PriceChange4h > 0.8 {
		pushLong(0.4, fmt.Sprintf("4h趋势%.2f%%(+0.4)", data.PriceChange4h))
	} else if data.PriceChange4h < -0.8 {
		pushShort(0.4, fmt.Sprintf("4h回落%.2f%%(+0.4)", -data.PriceChange4h))
	}

	if data.CurrentMACD > 0 {
		pushLong(0.35, "MACD>0(+0.35)")
	} else if data.CurrentMACD < 0 {
		pushShort(0.35, "MACD<0(+0.35)")
	}

	if data.CurrentRSI7 > 60 && data.CurrentRSI7 < 75 {
		pushLong(0.25, "RSI7强势(+0.25)")
	} else if data.CurrentRSI7 < 40 {
		pushShort(0.25, "RSI7走弱(+0.25)")
	}

	if data.CurrentVWAP > 0 {
		if price > data.CurrentVWAP {
			pushLong(0.2, "价格高于VWAP(+0.2)")
		} else {
			pushShort(0.2, "价格低于VWAP(+0.2)")
		}
	}

	if data.OpenInterest != nil && data.OpenInterest.Average > 0 {
		ratio := data.OpenInterest.Latest / data.OpenInterest.Average
		if ratio > 1.03 {
			pushLong(0.3, "OI上升(+0.3)")
		} else if ratio < 0.97 {
			pushShort(0.3, "OI回落(+0.3)")
		}
	}

	if math.Abs(data.FundingRate) > 0.0004 {
		if data.FundingRate > 0 {
			pushShort(0.2, "正资金费率偏高(+0.2空)")
		} else {
			pushLong(0.2, "负资金费率(+0.2多)")
		}
	}

	if len(score.LongNotes) == 0 && len(score.ShortNotes) == 0 {
		pushLong(0.1, "信号稳定(+0.1)")
		pushShort(0.1, "信号稳定(+0.1)")
	}
}

func estimateATR(data *market.Data) float64 {
	if data.MediumTermContext != nil && data.MediumTermContext.ATR14 > 0 {
		return data.MediumTermContext.ATR14
	}
	if data.LongerTermContext != nil && data.LongerTermContext.ATR14 > 0 {
		return data.LongerTermContext.ATR14
	}
	return data.CurrentPrice * 0.01
}

func buildQuantDecisions(ctx *Context, scores []quantScore) []Decision {
	accountEquity := ctx.Account.TotalEquity
	if accountEquity <= 0 {
		accountEquity = ctx.Account.AvailableBalance
	}

	symbolScores := make(map[string]quantScore, len(scores))
	for _, sc := range scores {
		symbolScores[strings.ToUpper(sc.Symbol)] = sc
	}

	strategy := ctx.Strategy
	defaultSize := accountEquity * strategy.PositionSizePct
	if defaultSize <= 0 {
		return nil
	}

	closeThreshold := math.Max(strategy.MinSignalScore*0.6, 1.2)
	minHold := time.Duration(strategy.MinHoldMinutes) * time.Minute
	now := time.Now()

	currentLong, currentShort := 0, 0
	plannedLongClose, plannedShortClose := 0, 0
	activeLong := make(map[string]bool)
	activeShort := make(map[string]bool)

	var decisions []Decision

	for _, pos := range ctx.Positions {
		symbolKey := strings.ToUpper(pos.Symbol)
		score, hasScore := symbolScores[symbolKey]
		if strings.EqualFold(pos.Side, "long") {
			currentLong++
			activeLong[symbolKey] = true
			if hasScore && shouldCloseLong(pos, score, closeThreshold, minHold, now) {
				reason := fmt.Sprintf("多头得分降至%.2f，空头信号%.2f", score.LongScore, score.ShortScore)
				decisions = append(decisions, Decision{
					Symbol:     pos.Symbol,
					Action:     "close_long",
					Reasoning:  reason,
					PositionID: pos.PositionID,
				})
				plannedLongClose++
				activeLong[symbolKey] = false
			}
		} else {
			currentShort++
			activeShort[symbolKey] = true
			if !strategy.AllowShort {
				decisions = append(decisions, Decision{
					Symbol:     pos.Symbol,
					Action:     "close_short",
					Reasoning:  "策略禁用做空，强制平仓",
					PositionID: pos.PositionID,
				})
				plannedShortClose++
				activeShort[symbolKey] = false
				continue
			}
			if hasScore && shouldCloseShort(pos, score, closeThreshold, minHold, now) {
				reason := fmt.Sprintf("空头得分降至%.2f，多头信号%.2f", score.ShortScore, score.LongScore)
				decisions = append(decisions, Decision{
					Symbol:     pos.Symbol,
					Action:     "close_short",
					Reasoning:  reason,
					PositionID: pos.PositionID,
				})
				plannedShortClose++
				activeShort[symbolKey] = false
			}
		}
	}

	availableLong := strategy.MaxLongPositions - (currentLong - plannedLongClose)
	if availableLong < 0 {
		availableLong = 0
	}
	availableShort := strategy.MaxShortPositions - (currentShort - plannedShortClose)
	if availableShort < 0 {
		availableShort = 0
	}

	for _, score := range scores {
		if availableLong == 0 {
			break
		}
		symbolKey := strings.ToUpper(score.Symbol)
		if activeLong[symbolKey] || score.LongScore < strategy.MinSignalScore {
			continue
		}

		decision := Decision{
			Symbol:          score.Symbol,
			Action:          "open_long",
			Leverage:        leverageForSymbol(score.Symbol, ctx),
			PositionSizeUSD: defaultSize,
			Reasoning:       buildReasoning(score.LongNotes, "多头"),
			Confidence:      confidenceFromScore(score.LongScore, strategy.MinSignalScore),
		}
		decision.StopLoss, decision.TakeProfit = computeStops("long", score.Price, score.ATR, strategy)
		decision.RiskUSD = decision.PositionSizeUSD / float64(decision.Leverage)

		if err := validateDecision(&decision, ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage); err != nil {
			log.Printf("⚠️  跳过 %s 多头信号: %v", decision.Symbol, err)
			continue
		}

		decisions = append(decisions, decision)
		activeLong[symbolKey] = true
		availableLong--
	}

	if strategy.AllowShort {
		for _, score := range scores {
			if availableShort == 0 {
				break
			}
			symbolKey := strings.ToUpper(score.Symbol)
			if activeShort[symbolKey] || score.ShortScore < strategy.MinSignalScore {
				continue
			}

			decision := Decision{
				Symbol:          score.Symbol,
				Action:          "open_short",
				Leverage:        leverageForSymbol(score.Symbol, ctx),
				PositionSizeUSD: defaultSize,
				Reasoning:       buildReasoning(score.ShortNotes, "空头"),
				Confidence:      confidenceFromScore(score.ShortScore, strategy.MinSignalScore),
			}
			decision.StopLoss, decision.TakeProfit = computeStops("short", score.Price, score.ATR, strategy)
			decision.RiskUSD = decision.PositionSizeUSD / float64(decision.Leverage)

			if err := validateDecision(&decision, ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage); err != nil {
				log.Printf("⚠️  跳过 %s 空头信号: %v", decision.Symbol, err)
				continue
			}

			decisions = append(decisions, decision)
			activeShort[symbolKey] = true
			availableShort--
		}
	}

	return decisions
}

func shouldCloseLong(pos PositionInfo, score quantScore, threshold float64, minHold time.Duration, now time.Time) bool {
	if score.LongScore >= threshold && score.ShortScore <= score.LongScore {
		return false
	}
	if !holdSatisfied(pos, minHold, now) && score.ShortScore < score.LongScore {
		return false
	}
	if score.LongScore < threshold {
		return true
	}
	return score.ShortScore > score.LongScore+0.2
}

func shouldCloseShort(pos PositionInfo, score quantScore, threshold float64, minHold time.Duration, now time.Time) bool {
	if score.ShortScore >= threshold && score.LongScore <= score.ShortScore {
		return false
	}
	if !holdSatisfied(pos, minHold, now) && score.LongScore < score.ShortScore {
		return false
	}
	if score.ShortScore < threshold {
		return true
	}
	return score.LongScore > score.ShortScore+0.2
}

func holdSatisfied(pos PositionInfo, minHold time.Duration, now time.Time) bool {
	if minHold == 0 || pos.UpdateTime == 0 {
		return true
	}
	held := now.Sub(time.UnixMilli(pos.UpdateTime))
	return held >= minHold
}

func leverageForSymbol(symbol string, ctx *Context) int {
	symbol = strings.ToUpper(symbol)
	if strings.HasPrefix(symbol, "BTC") || strings.HasPrefix(symbol, "ETH") {
		return ctx.BTCETHLeverage
	}
	return ctx.AltcoinLeverage
}

func computeStops(side string, price, atr float64, cfg strategy.QuantConfig) (float64, float64) {
	if atr <= 0 {
		atr = price * 0.01
	}
	switch side {
	case "long":
		stop := price - atr*cfg.StopLossATRMultiplier
		target := price + (price-stop)*cfg.RiskReward
		if cfg.TakeProfitATRMultiplier > 0 {
			targetAlt := price + atr*cfg.TakeProfitATRMultiplier
			if target == 0 || targetAlt < target {
				target = targetAlt
			}
		}
		return stop, target
	case "short":
		stop := price + atr*cfg.StopLossATRMultiplier
		target := price - (stop-price)*cfg.RiskReward
		if cfg.TakeProfitATRMultiplier > 0 {
			targetAlt := price - atr*cfg.TakeProfitATRMultiplier
			if target == 0 || targetAlt > target {
				target = targetAlt
			}
		}
		return stop, target
	default:
		return price * 0.98, price * 1.02
	}
}

func buildReasoning(notes []string, prefix string) string {
	if len(notes) == 0 {
		return prefix + "信号满足阈值"
	}
	if len(notes) > 3 {
		notes = notes[:3]
	}
	return prefix + ": " + strings.Join(notes, "；")
}

func confidenceFromScore(score, minScore float64) int {
	if minScore <= 0 {
		minScore = 1
	}
	ratio := score / minScore
	conf := int(60 + ratio*20)
	if conf > 98 {
		conf = 98
	}
	if conf < 55 {
		conf = 55
	}
	return conf
}

func buildQuantNarrative(ctx *Context, scores []quantScore, decisions []Decision) quantNarrative {
	strategyName := "QUANT"
	overview := fmt.Sprintf(
		"时间: %s | 策略: %s | 净值 %.2f | 可用 %.2f | 持仓 %d | 阈值 %.1f",
		ctx.CurrentTime,
		strategyName,
		ctx.Account.TotalEquity,
		ctx.Account.AvailableBalance,
		ctx.Account.PositionCount,
		ctx.Strategy.MinSignalScore,
	)

	var detail strings.Builder
	detail.WriteString("顶级多头信号:\n")
	detail.WriteString(formatTopScores(scores, true, 3))
	detail.WriteString("\n顶级空头信号:\n")
	detail.WriteString(formatTopScores(scores, false, 3))

	if len(decisions) > 0 {
		detail.WriteString("\n执行计划:\n")
		for i, d := range decisions {
			detail.WriteString(fmt.Sprintf("%d. %s %s | %s\n", i+1, d.Symbol, strings.ToUpper(d.Action), d.Reasoning))
		}
	} else {
		detail.WriteString("\n执行计划: 暂无调仓，维持现有结构\n")
	}

	return quantNarrative{
		Overview: overview,
		Detail:   detail.String(),
	}
}

func formatTopScores(scores []quantScore, isLong bool, limit int) string {
	var filtered []quantScore
	for _, sc := range scores {
		if isLong && sc.LongScore > 0 {
			filtered = append(filtered, sc)
		}
		if !isLong && sc.ShortScore > 0 {
			filtered = append(filtered, sc)
		}
	}

	if len(filtered) == 0 {
		return "无\n"
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if isLong {
			return filtered[i].LongScore > filtered[j].LongScore
		}
		return filtered[i].ShortScore > filtered[j].ShortScore
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	var sb strings.Builder
	for _, sc := range filtered {
		scoreVal := sc.LongScore
		if !isLong {
			scoreVal = sc.ShortScore
		}
		sb.WriteString(fmt.Sprintf("- %s 得分 %.2f @ %.4f\n", sc.Symbol, scoreVal, sc.Price))
	}
	return sb.String()
}

// fetchMarketDataForContext 为上下文中的所有币种获取市场数据和OI数据
func fetchMarketDataForContext(ctx *Context) error {
	ctx.MarketDataMap = make(map[string]*market.Data)
	ctx.OITopDataMap = make(map[string]*OITopData)

	// 收集所有需要获取数据的币种
	symbolSet := make(map[string]bool)

	// 1. 优先获取持仓币种的数据（这是必须的）
	for _, pos := range ctx.Positions {
		symbolSet[pos.Symbol] = true
	}

	// 2. 候选币种数量根据账户状态动态调整
	maxCandidates := calculateMaxCandidates(ctx)
	for i, coin := range ctx.CandidateCoins {
		if i >= maxCandidates {
			break
		}
		symbolSet[coin.Symbol] = true
	}

	// ETH/BTC 配对策略依赖的基础腿，确保行情一定可用
	for _, alias := range assetAliases("ETH") {
		symbolSet[alias] = true
	}
	for _, alias := range assetAliases("BTC") {
		symbolSet[alias] = true
	}

	// 并发获取市场数据
	// 持仓币种集合（用于判断是否跳过OI检查）
	positionSymbols := make(map[string]bool)
	for _, pos := range ctx.Positions {
		positionSymbols[pos.Symbol] = true
	}

	for symbol := range symbolSet {
		data, err := market.Get(symbol)
		if err != nil {
			// 单个币种失败不影响整体，只记录错误
			continue
		}

		// ⚠️ 流动性过滤：持仓价值低于15M USD的币种不做（多空都不做）
		// 持仓价值 = 持仓量 × 当前价格
		// 但现有持仓必须保留（需要决策是否平仓）
		isExistingPosition := positionSymbols[symbol]
		if !isExistingPosition && data.OpenInterest != nil && data.CurrentPrice > 0 {
			// 计算持仓价值（USD）= 持仓量 × 当前价格
			oiValue := data.OpenInterest.Latest * data.CurrentPrice
			oiValueInMillions := oiValue / 1_000_000 // 转换为百万美元单位
			if oiValueInMillions < 15 {
				log.Printf("⚠️  %s 持仓价值过低(%.2fM USD < 15M)，跳过此币种 [持仓量:%.0f × 价格:%.4f]",
					symbol, oiValueInMillions, data.OpenInterest.Latest, data.CurrentPrice)
				continue
			}
		}

		ctx.MarketDataMap[symbol] = data
	}

	// 加载OI Top数据（不影响主流程）
	oiPositions, err := pool.GetOITopPositions()
	if err == nil {
		for _, pos := range oiPositions {
			// 标准化符号匹配
			symbol := pos.Symbol
			ctx.OITopDataMap[symbol] = &OITopData{
				Rank:              pos.Rank,
				OIDeltaPercent:    pos.OIDeltaPercent,
				OIDeltaValue:      pos.OIDeltaValue,
				PriceDeltaPercent: pos.PriceDeltaPercent,
				NetLong:           pos.NetLong,
				NetShort:          pos.NetShort,
			}
		}
	}

	return nil
}

// calculateMaxCandidates 根据账户状态计算需要分析的候选币种数量
func calculateMaxCandidates(ctx *Context) int {
	// 直接返回候选池的全部币种数量
	// 因为候选池已经在 auto_trader.go 中筛选过了
	// 固定分析前20个评分最高的币种（来自AI500）
	return len(ctx.CandidateCoins)
}

// buildUserPrompt 构建 User Prompt（动态数据）
func buildUserPrompt(ctx *Context) string {
	var sb strings.Builder

	// 系统状态
	sb.WriteString(fmt.Sprintf("**时间**: %s | **周期**: #%d | **运行**: %d分钟\n\n",
		ctx.CurrentTime, ctx.CallCount, ctx.RuntimeMinutes))

	// BTC 市场
	if btcData, _, hasBTC := marketDataForAsset(ctx, "BTC"); hasBTC {
		sb.WriteString(fmt.Sprintf("**BTC**: %.2f (1h: %+.2f%%, 4h: %+.2f%%) | MACD: %.4f | RSI: %.2f\n\n",
			btcData.CurrentPrice, btcData.PriceChange1h, btcData.PriceChange4h,
			btcData.CurrentMACD, btcData.CurrentRSI7))
	}

	// 账户
	sb.WriteString(fmt.Sprintf("**账户**: 净值%.2f | 余额%.2f (%.1f%%) | 盈亏%+.2f%% | 保证金%.1f%% | 持仓%d个\n\n",
		ctx.Account.TotalEquity,
		ctx.Account.AvailableBalance,
		(ctx.Account.AvailableBalance/ctx.Account.TotalEquity)*100,
		ctx.Account.TotalPnLPct,
		ctx.Account.MarginUsedPct,
		ctx.Account.PositionCount))

	profitPct := ctx.Account.TotalPnLPct
	switch {
	case profitPct >= 15:
		sb.WriteString(fmt.Sprintf("🚨 **超级锁盈模式**：净值已领先 %.1f%%。所有新仓位≤原计划的25%%，如无多重共振信号，请优先 `hold/wait`，让收益冷却。任何做空前必须再次确认大周期与宏观环境。\n\n", profitPct))
	case profitPct >= 8:
		sb.WriteString(fmt.Sprintf("🔒 **锁盈提醒**：累计收益 %.1f%%。进入防守状态：新增仓位≤40%%，最多2仓，同方向不连击，优先考虑减仓或观望，避免情绪化反击。\n\n", profitPct))
	case profitPct <= -6:
		sb.WriteString(fmt.Sprintf("🧊 **止血提示**：当前回撤 %.1f%%。缩减仓位并回顾失误；当风险回报重新≥1:2.8 且信心≥70 时，可尝试小仓重启，并在思维链记录风险补偿理由。\n\n", profitPct))
	}

	// 持仓（完整市场数据）
	if len(ctx.Positions) > 0 {
		sb.WriteString("## 当前持仓\n")
		for i, pos := range ctx.Positions {
			// 计算价格涨跌幅（未乘杠杆）
			priceChangePct := 0.0
			if pos.EntryPrice > 0 && pos.MarkPrice > 0 {
				change := ((pos.MarkPrice - pos.EntryPrice) / pos.EntryPrice) * 100
				if strings.ToLower(pos.Side) == "short" {
					change = -change
				}
				priceChangePct = change
			}
			// 计算持仓时长
			holdingDuration := ""
			if pos.UpdateTime > 0 {
				durationMs := time.Now().UnixMilli() - pos.UpdateTime
				durationMin := durationMs / (1000 * 60) // 转换为分钟
				if durationMin < 60 {
					holdingDuration = fmt.Sprintf(" | 持仓时长%d分钟", durationMin)
				} else {
					durationHour := durationMin / 60
					durationMinRemainder := durationMin % 60
					holdingDuration = fmt.Sprintf(" | 持仓时长%d小时%d分钟", durationHour, durationMinRemainder)
				}
			}

			sb.WriteString(fmt.Sprintf("%d. %s %s | 入场价%.4f 当前价%.4f | 盈亏%+.2f%% | 杠杆%dx | 保证金%.0f | 强平价%.4f%s\n\n",
				i+1, pos.Symbol, strings.ToUpper(pos.Side),
				pos.EntryPrice, pos.MarkPrice, priceChangePct,
				pos.Leverage, pos.MarginUsed, pos.LiquidationPrice, holdingDuration))

			// 使用FormatMarketData输出完整市场数据
			if marketData, ok := ctx.MarketDataMap[pos.Symbol]; ok {
				sb.WriteString(market.Format(marketData))
				sb.WriteString("\n")
			}
		}
	} else {
		sb.WriteString("**当前持仓**: 无\n\n")
	}

	// 候选币种（完整市场数据）
	sb.WriteString(fmt.Sprintf("## 候选币种 (%d个)\n\n", len(ctx.MarketDataMap)))
	displayedCount := 0
	for _, coin := range ctx.CandidateCoins {
		marketData, hasData := ctx.MarketDataMap[coin.Symbol]
		if !hasData {
			continue
		}
		displayedCount++

		sourceTags := ""
		if len(coin.Sources) > 1 {
			sourceTags = " (AI500+OI_Top双重信号)"
		} else if len(coin.Sources) == 1 && coin.Sources[0] == "oi_top" {
			sourceTags = " (OI_Top持仓增长)"
		}

		// 使用FormatMarketData输出完整市场数据
		sb.WriteString(fmt.Sprintf("### %d. %s%s\n\n", displayedCount, coin.Symbol, sourceTags))
		sb.WriteString(market.Format(marketData))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// 历史表现摘要（精简版，但确保夏普比率始终显示）
	var sharpeRatio float64
	var hasFullData bool

	if ctx.Performance != nil {
		type PerformanceData struct {
			SharpeRatio   float64                `json:"sharpe_ratio"`
			TotalTrades   int                    `json:"total_trades"`
			WinningTrades int                    `json:"winning_trades"`
			LosingTrades  int                    `json:"losing_trades"`
			WinRate       float64                `json:"win_rate"`
			ProfitFactor  float64                `json:"profit_factor"`
			AvgWin        float64                `json:"avg_win"`
			AvgLoss       float64                `json:"avg_loss"`
			SymbolStats   map[string]interface{} `json:"symbol_stats"` // 改为interface{}以兼容*SymbolPerformance
		}
		var perfData PerformanceData
		if jsonData, err := json.Marshal(ctx.Performance); err == nil {
			if err := json.Unmarshal(jsonData, &perfData); err == nil {
				sharpeRatio = perfData.SharpeRatio
				hasFullData = perfData.TotalTrades > 0

				if hasFullData {
					sb.WriteString("## 📊 历史表现摘要\n\n")
					sb.WriteString(fmt.Sprintf("**夏普比率**: %.2f | **总交易**: %d | **胜/负**: %dW/%dL\n", perfData.SharpeRatio, perfData.TotalTrades, perfData.WinningTrades, perfData.LosingTrades))
					if perfData.WinRate > 0 {
						sb.WriteString(fmt.Sprintf("**胜率**: %.1f%% | **盈亏比**: %.2f\n", perfData.WinRate*100, perfData.ProfitFactor))
					}
					if perfData.AvgWin > 0 || perfData.AvgLoss < 0 {
						sb.WriteString(fmt.Sprintf("**平均盈利**: $%.2f | **平均亏损**: $%.2f\n", perfData.AvgWin, perfData.AvgLoss))
					}
					sb.WriteString("\n")

					// 显示各币种表现统计，辅助人工复盘
					if perfData.SymbolStats != nil && len(perfData.SymbolStats) > 0 {
						sb.WriteString("### 📈 各币种表现统计\n\n")
						// 按总盈亏排序，显示前10个币种
						type SymbolStat struct {
							Symbol        string
							TotalTrades   int
							WinningTrades int
							LosingTrades  int
							WinRate       float64
							TotalPnL      float64
							AvgPnL        float64
						}
						var symbolList []SymbolStat
						for symbol, stats := range perfData.SymbolStats {
							// SymbolStats 可能是 *SymbolPerformance 或 map[string]interface{}
							// 通过JSON序列化/反序列化统一处理
							statsJSON, _ := json.Marshal(stats)
							var statsMap map[string]interface{}
							if err := json.Unmarshal(statsJSON, &statsMap); err == nil {
								totalTrades, _ := statsMap["total_trades"].(float64)
								winningTrades, _ := statsMap["winning_trades"].(float64)
								losingTrades, _ := statsMap["losing_trades"].(float64)
								winRate, _ := statsMap["win_rate"].(float64)
								totalPnL, _ := statsMap["total_pn_l"].(float64)
								avgPnL, _ := statsMap["avg_pn_l"].(float64)
								symbolList = append(symbolList, SymbolStat{
									Symbol:        symbol,
									TotalTrades:   int(totalTrades),
									WinningTrades: int(winningTrades),
									LosingTrades:  int(losingTrades),
									WinRate:       winRate,
									TotalPnL:      totalPnL,
									AvgPnL:        avgPnL,
								})
							}
						}
						// 按总盈亏排序
						for i := 0; i < len(symbolList)-1; i++ {
							for j := i + 1; j < len(symbolList); j++ {
								if symbolList[i].TotalPnL < symbolList[j].TotalPnL {
									symbolList[i], symbolList[j] = symbolList[j], symbolList[i]
								}
							}
						}
						// 显示前10个
						displayCount := 10
						if len(symbolList) < displayCount {
							displayCount = len(symbolList)
						}
						for i := 0; i < displayCount; i++ {
							stat := symbolList[i]
							pnlSign := "+"
							if stat.TotalPnL < 0 {
								pnlSign = ""
							}
							sb.WriteString(fmt.Sprintf("- **%s**: %d笔 (%dW/%dL, 胜率%.1f%%) | 总盈亏: %s$%.2f | 平均: $%.2f\n",
								stat.Symbol, stat.TotalTrades, stat.WinningTrades, stat.LosingTrades,
								stat.WinRate, pnlSign, stat.TotalPnL, stat.AvgPnL))
						}
						sb.WriteString("\n")
					}
				} else {
					sb.WriteString(fmt.Sprintf("## 📊 夏普比率: %.2f\n\n", sharpeRatio))
				}
			}
		}
	}

	// 如果只有部分数据或数据获取失败，至少显示夏普比率
	if ctx.Performance != nil && !hasFullData {
		if sharpeRatio != 0 {
			sb.WriteString(fmt.Sprintf("## 📊 夏普比率: %.2f\n\n", sharpeRatio))
		}
	}

	// 显示最近交易明细，辅助策略复盘
	if ctx.Performance != nil {
		type PerformanceDataFull struct {
			RecentTrades []map[string]interface{} `json:"recent_trades"`
		}
		var perfDataFull PerformanceDataFull
		if jsonData, err := json.Marshal(ctx.Performance); err == nil {
			if err := json.Unmarshal(jsonData, &perfDataFull); err == nil {
				if len(perfDataFull.RecentTrades) > 0 {
					sb.WriteString("## 📋 最近交易明细（最近20笔，用于深度学习和反思）\n\n")
					sb.WriteString("**格式**: 币种 | 方向 | 开仓价→平仓价 | 数量 | 盈亏 | 持仓时长 | 盈亏%%\n\n")

					// 显示最近20笔交易
					displayCount := 20
					if len(perfDataFull.RecentTrades) < displayCount {
						displayCount = len(perfDataFull.RecentTrades)
					}

					for i := 0; i < displayCount; i++ {
						trade := perfDataFull.RecentTrades[i]
						symbol, _ := trade["symbol"].(string)
						side, _ := trade["side"].(string)
						openPrice, _ := trade["open_price"].(float64)
						closePrice, _ := trade["close_price"].(float64)
						quantity, _ := trade["quantity"].(float64)
						pnl, _ := trade["pn_l"].(float64)
						duration, _ := trade["duration"].(string)
						pnlPct, _ := trade["pn_l_pct"].(float64)

						pnlSign := "+"
						pnlColor := "🟢"
						if pnl < 0 {
							pnlSign = ""
							pnlColor = "🔴"
						}

						sideText := "做多"
						if side == "short" {
							sideText = "做空"
						}

						sb.WriteString(fmt.Sprintf("%d. %s **%s** %s | %.4f→%.4f | %.4f | %s%.2f | %s | %.2f%%\n",
							i+1, pnlColor, symbol, sideText, openPrice, closePrice, quantity,
							pnlSign, pnl, duration, pnlPct))
					}
					sb.WriteString("\n")
					sb.WriteString("**反思要点**:\n")
					sb.WriteString("- 哪些交易盈利/亏损？原因是什么？\n")
					sb.WriteString("- 持仓时间是否合理？（<30分钟可能过早平仓）\n")
					sb.WriteString("- 哪些币种表现好/差？应该重点关注哪些币种？\n")
					sb.WriteString("- 是否存在重复的错误模式？（如频繁交易、过早平仓等）\n\n")
				}
			}
		}
	}

	sb.WriteString("---\n\n")
	sb.WriteString("现在请分析并输出决策（思维链 + JSON）\n")

	return sb.String()
}

// validateDecisions 验证所有决策（需要账户信息和杠杆配置）
func validateDecisions(decisions []Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int) error {
	for i, decision := range decisions {
		if err := validateDecision(&decision, accountEquity, btcEthLeverage, altcoinLeverage); err != nil {
			return fmt.Errorf("决策 #%d 验证失败: %w", i+1, err)
		}
	}
	return nil
}

// findMatchingBracket 查找匹配的右括号
func findMatchingBracket(s string, start int) int {
	if start >= len(s) || s[start] != '[' {
		return -1
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		case '`':
			// 忽略 Markdown 代码块内的括号：消耗成对的 ```
			if i+2 < len(s) && s[i+1] == '`' && s[i+2] == '`' {
				i += 2
				for j := i + 1; j+2 < len(s); j++ {
					if s[j] == '`' && s[j+1] == '`' && s[j+2] == '`' {
						i = j + 2
						break
					}
				}
			}
		}
	}

	return -1
}

// validateDecision 验证单个决策的有效性
func validateDecision(d *Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int) error {
	// 验证action
	validActions := map[string]bool{
		"open_long":   true,
		"open_short":  true,
		"close_long":  true,
		"close_short": true,
		"hold":        true,
		"wait":        true,
	}

	if !validActions[d.Action] {
		return fmt.Errorf("无效的action: %s", d.Action)
	}

	// 开仓操作必须提供完整参数
	if d.Action == "open_long" || d.Action == "open_short" {
		// 根据币种使用配置的杠杆上限
		maxLeverage := altcoinLeverage          // 山寨币使用配置的杠杆
		maxPositionValue := accountEquity * 1.5 // 山寨币最多1.5倍账户净值
		if isMajorSymbol(d.Symbol) {
			maxLeverage = btcEthLeverage          // BTC和ETH使用配置的杠杆
			maxPositionValue = accountEquity * 10 // BTC/ETH最多10倍账户净值
		}

		if d.Leverage <= 0 || d.Leverage > maxLeverage {
			return fmt.Errorf("杠杆必须在1-%d之间（%s，当前配置上限%d倍）: %d", maxLeverage, d.Symbol, maxLeverage, d.Leverage)
		}
		if d.PositionSizeUSD <= 0 {
			return fmt.Errorf("仓位大小必须大于0: %.2f", d.PositionSizeUSD)
		}
		// 验证仓位价值上限（加1%容差以避免浮点数精度问题）
		tolerance := maxPositionValue * 0.01 // 1%容差
		if d.PositionSizeUSD > maxPositionValue+tolerance {
			if isMajorSymbol(d.Symbol) {
				return fmt.Errorf("BTC/ETH 单币种仓位价值不能超过 %.0f USDT（10倍账户净值），实际: %.0f（%s）",
					maxPositionValue, d.PositionSizeUSD, d.Symbol)
			} else {
				return fmt.Errorf("山寨币单币种仓位价值不能超过%.0f USDT（1.5倍账户净值），实际: %.0f", maxPositionValue, d.PositionSizeUSD)
			}
		}
		if d.StopLoss <= 0 || d.TakeProfit <= 0 {
			return fmt.Errorf("止损和止盈必须大于0")
		}

		// 验证止损止盈的合理性
		if d.Action == "open_long" {
			if d.StopLoss >= d.TakeProfit {
				return fmt.Errorf("做多时止损价必须小于止盈价")
			}
		} else {
			if d.StopLoss <= d.TakeProfit {
				return fmt.Errorf("做空时止损价必须大于止盈价")
			}
		}

		// 验证风险回报比（必须≥1:3）
		// 计算入场价（假设当前市价）
		var entryPrice float64
		if d.Action == "open_long" {
			// 做多：入场价在止损和止盈之间
			entryPrice = d.StopLoss + (d.TakeProfit-d.StopLoss)*0.2 // 假设在20%位置入场
		} else {
			// 做空：入场价在止损和止盈之间
			entryPrice = d.StopLoss - (d.StopLoss-d.TakeProfit)*0.2 // 假设在20%位置入场
		}

		var riskPercent, rewardPercent, riskRewardRatio float64
		if d.Action == "open_long" {
			riskPercent = (entryPrice - d.StopLoss) / entryPrice * 100
			rewardPercent = (d.TakeProfit - entryPrice) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		} else {
			riskPercent = (d.StopLoss - entryPrice) / entryPrice * 100
			rewardPercent = (entryPrice - d.TakeProfit) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		}

		// 硬约束：风险回报比必须≥3.0
		if riskRewardRatio < 2.8 {
			return fmt.Errorf("风险回报比过低(%.2f:1)，必须≥2.8:1 [风险:%.2f%% 收益:%.2f%%] [止损:%.2f 止盈:%.2f]",
				riskRewardRatio, riskPercent, rewardPercent, d.StopLoss, d.TakeProfit)
		}
	}

	return nil
}
