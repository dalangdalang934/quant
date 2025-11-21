//go:build ai
// +build ai

package trader

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"nofx/decision"
	"nofx/learning"
	"nofx/logger"
	"nofx/market"
	"nofx/news"
	"nofx/pool"
	"nofx/strategy"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	colorReset     = "\033[0m"
	colorGreen     = "\033[32m"
	colorRed       = "\033[31m"
	colorBlue      = "\033[34m"
	minPositionUSD = 30.0
)

// AutoTraderConfig 自动交易配置
type AutoTraderConfig struct {
	// Trader标识
	ID       string
	Name     string
	Strategy string

	// 交易平台选择
	Exchange string // "binance", "hyperliquid" 或 "aster"

	// 币安API配置
	BinanceAPIKey    string
	BinanceSecretKey string

	// Hyperliquid配置
	HyperliquidPrivateKey string
	HyperliquidWalletAddr string
	HyperliquidTestnet    bool

	// Aster配置
	AsterUser       string
	AsterSigner     string
	AsterPrivateKey string

	CoinPoolAPIURL string
	MarginType     string // "isolated" or "cross"

	// 扫描配置
	ScanInterval time.Duration

	// 账户配置
	InitialBalance float64

	// 杠杆配置
	BTCETHLeverage  int
	AltcoinLeverage int

	// 风险控制
	MaxDailyLoss    float64
	MaxDrawdown     float64
	StopTradingTime time.Duration

	// 币安 Websocket 流配置
	BinanceStream BinanceStreamOptions

	// 量化策略参数
	QuantConfig strategy.QuantConfig
}

// BinanceStreamOptions 币安 Websocket 流配置（解析后的时间单位）
type BinanceStreamOptions struct {
	MarketEnabled            bool
	UserEnabled              bool
	MarketReconnectDelay     time.Duration
	MarketPingInterval       time.Duration
	UserReconnectDelay       time.Duration
	UserPingInterval         time.Duration
	ListenKeyRefreshInterval time.Duration
}

// AutoTrader 自动交易器
type AutoTrader struct {
	id                    string // Trader唯一标识
	name                  string // Trader显示名称
	strategyLabel         string // 策略标签
	exchange              string // 交易平台名称
	config                AutoTraderConfig
	trader                Trader                 // 使用Trader接口（支持多平台）
	decisionLogger        *logger.DecisionLogger // 决策日志记录器
	initialBalance        float64
	dailyPnL              float64
	lastResetTime         time.Time
	stopUntil             time.Time
	isRunning             bool
	startTime             time.Time        // 系统启动时间
	callCount             int              // AI调用次数
	positionFirstSeenTime map[string]int64 // 持仓首次出现时间 (symbol_side -> timestamp毫秒)
	lastOpenActionTime    time.Time        // 最近一次开仓执行时间
	openActionHistory     []time.Time      // 最近1小时内的开仓时间戳

	// 持仓快照管理
	lastPositionSnapshot  map[string]logger.PositionSnapshot // 上次持仓快照
	positionSnapshotMutex sync.RWMutex                       // 持仓快照互斥锁

	// 交易所成交记录缓存
	exchangeTradesFile  string       // 交易所成交记录文件路径
	exchangeTradesMutex sync.RWMutex // 交易所成交记录互斥锁

	// 学习状态
	learningManager       *learning.Manager
	learningState         *learning.State
	learningMutex         sync.RWMutex
	historyWarmupNotified bool

	// 仓位追踪
	positionTracker *PositionTracker // 仓位追踪器

	// 币安 Websocket 流配置
	binanceStream BinanceStreamOptions

	// 量化策略
	quantConfig strategy.QuantConfig
}

// NewAutoTrader 创建自动交易器
func NewAutoTrader(config AutoTraderConfig) (*AutoTrader, error) {
	// 设置默认值
	if config.ID == "" {
		config.ID = "default_trader"
	}
	if config.Name == "" {
		config.Name = "Default Trader"
	}
	if config.Strategy == "" {
		config.Strategy = "quant"
	}
	config.QuantConfig.Normalize()
	log.Printf("🧮 [%s] 使用量化策略: %s", config.Name, strings.ToUpper(config.Strategy))

	// 初始化币种池API
	if config.CoinPoolAPIURL != "" {
		pool.SetCoinPoolAPI(config.CoinPoolAPIURL)
	}

	// 设置默认交易平台
	if config.Exchange == "" {
		config.Exchange = "binance"
	}

	// 根据配置创建对应的交易器
	var trader Trader
	var err error

	switch config.Exchange {
	case "binance":
		log.Printf("🏦 [%s] 使用币安合约交易", config.Name)
		ft := NewFuturesTrader(config.BinanceAPIKey, config.BinanceSecretKey)
		ft.SetDefaultMarginMode(config.MarginType)
		if config.BinanceStream.UserEnabled {
			ft.ConfigureBinanceUserStream(
				true,
				config.BinanceStream.UserReconnectDelay,
				config.BinanceStream.UserPingInterval,
				config.BinanceStream.ListenKeyRefreshInterval,
			)
		}
		trader = ft
	case "hyperliquid":
		log.Printf("🏦 [%s] 使用Hyperliquid交易", config.Name)
		trader, err = NewHyperliquidTrader(config.HyperliquidPrivateKey, config.HyperliquidWalletAddr, config.HyperliquidTestnet)
		if err != nil {
			return nil, fmt.Errorf("初始化Hyperliquid交易器失败: %w", err)
		}
	case "aster":
		log.Printf("🏦 [%s] 使用Aster交易", config.Name)
		trader, err = NewAsterTrader(config.AsterUser, config.AsterSigner, config.AsterPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("初始化Aster交易器失败: %w", err)
		}
	default:
		return nil, fmt.Errorf("不支持的交易平台: %s", config.Exchange)
	}

	// 验证初始金额配置
	if config.InitialBalance <= 0 {
		return nil, fmt.Errorf("初始金额必须大于0，请在配置中设置InitialBalance")
	}

	// 初始化决策日志记录器（使用trader ID创建独立目录）
	logDir := fmt.Sprintf("decision_logs/%s", config.ID)
	decisionLogger := logger.NewDecisionLogger(logDir)
	currentCycle := decisionLogger.CurrentCycleNumber()
	if currentCycle > 0 {
		log.Printf("📂 [%s] 检测到历史决策记录，将从周期 #%d 继续计数", config.Name, currentCycle)
	}

	// 初始化交易所成交记录文件路径
	exchangeTradesDir := fmt.Sprintf("data/exchange_trades")
	if err := os.MkdirAll(exchangeTradesDir, 0o755); err != nil {
		log.Printf("⚠️ 创建交易所成交记录目录失败: %v", err)
	}
	exchangeTradesFile := filepath.Join(exchangeTradesDir, fmt.Sprintf("%s.json", config.ID))

	// 初始化学习状态管理器
	learningDir := filepath.Join("data", "learning_state")
	learningManager := learning.NewManager(config.ID, learningDir)

	// 初始化仓位追踪器
	positionTracker := NewPositionTracker(config.ID)
	if matcher, ok := trader.(TradeFillMatcher); ok {
		positionTracker.SetTradeFillMatcher(matcher)
	}

	// 设置币安 WSS 默认参数（仅当启用时）
	if config.BinanceStream.UserEnabled {
		if ft, ok := trader.(*FuturesTrader); ok {
			ft.StartBinanceUserStream()
		}
	}

	return &AutoTrader{
		id:                    config.ID,
		name:                  config.Name,
		strategyLabel:         config.Strategy,
		exchange:              config.Exchange,
		config:                config,
		trader:                trader,
		decisionLogger:        decisionLogger,
		initialBalance:        config.InitialBalance,
		lastResetTime:         time.Now(),
		startTime:             time.Now(),
		callCount:             currentCycle,
		isRunning:             false,
		positionFirstSeenTime: make(map[string]int64),
		lastOpenActionTime:    time.Time{},
		openActionHistory:     make([]time.Time, 0, 8),
		lastPositionSnapshot:  make(map[string]logger.PositionSnapshot),
		exchangeTradesFile:    exchangeTradesFile,
		learningManager:       learningManager,
		positionTracker:       positionTracker,
		binanceStream:         config.BinanceStream,
		quantConfig:           config.QuantConfig.Copy(),
	}, nil
}

func (at *AutoTrader) updateLearningState(state *learning.State) {
	if state == nil {
		return
	}
	at.learningMutex.Lock()
	defer at.learningMutex.Unlock()
	at.learningState = state.Copy()
}

func (at *AutoTrader) getLearningState() *learning.State {
	at.learningMutex.RLock()
	defer at.learningMutex.RUnlock()
	if at.learningState == nil {
		return nil
	}
	return at.learningState.Copy()
}

// RestorePositionContext 恢复持仓上下文（程序启动时调用）
func (at *AutoTrader) RestorePositionContext() error {
	log.Printf("🔄 [%s] 恢复持仓上下文...", at.name)

	// 1. 从交易所获取当前实际持仓
	currentPositions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("获取当前持仓失败: %w", err)
	}

	if len(currentPositions) == 0 {
		log.Printf("✓ [%s] 当前没有持仓，无需恢复", at.name)
		at.positionFirstSeenTime = make(map[string]int64)
		at.positionSnapshotMutex.Lock()
		at.lastPositionSnapshot = make(map[string]logger.PositionSnapshot)
		at.positionSnapshotMutex.Unlock()
		return nil
	}

	// 2. 读取最近的日志记录（足够多，确保能覆盖所有可能的开仓）
	records, err := at.decisionLogger.GetLatestRecords(1000)
	if err != nil {
		return fmt.Errorf("读取日志记录失败: %w", err)
	}

	// 3. 分析日志，找出未平仓的持仓
	openPositionsFromLogs := make(map[string]map[string]interface{})

	// 从旧到新遍历记录，收集所有开仓和平仓
	for _, record := range records {
		for _, action := range record.Decisions {
			if !action.Success {
				continue
			}

			symbol := action.Symbol
			side := ""
			if action.Action == "open_long" || action.Action == "close_long" {
				side = "long"
			} else if action.Action == "open_short" || action.Action == "close_short" {
				side = "short"
			}

			if side == "" {
				continue
			}

			posKey := symbol + "_" + side

			switch action.Action {
			case "open_long", "open_short":
				// 记录开仓信息
				openPositionsFromLogs[posKey] = map[string]interface{}{
					"side":      side,
					"openPrice": action.Price,
					"openTime":  action.Timestamp,
					"quantity":  action.Quantity,
					"leverage":  action.Leverage,
					"orderID":   action.OrderID,
				}
			case "close_long", "close_short":
				// 移除已平仓记录
				delete(openPositionsFromLogs, posKey)
			}
		}
	}

	log.Printf("📊 [%s] 从日志分析：发现 %d 个未平仓持仓（从日志记录）", at.name, len(openPositionsFromLogs))

	// 4. 匹配交易所实际持仓与日志记录
	matchedCount := 0
	at.positionFirstSeenTime = make(map[string]int64)
	at.positionSnapshotMutex.Lock()
	at.lastPositionSnapshot = make(map[string]logger.PositionSnapshot)

	for _, pos := range currentPositions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		posKey := symbol + "_" + side

		// 检查日志中是否有对应的开仓记录
		if logPos, exists := openPositionsFromLogs[posKey]; exists {
			// 找到了匹配的持仓！
			openTime := logPos["openTime"].(time.Time)
			openPrice := logPos["openPrice"].(float64)

			// 恢复 positionFirstSeenTime
			at.positionFirstSeenTime[posKey] = openTime.UnixMilli()

			// 构建持仓快照
			entryPrice, _ := pos["entryPrice"].(float64)
			markPrice, _ := pos["markPrice"].(float64)
			posAmt, _ := pos["positionAmt"].(float64)
			if posAmt < 0 {
				posAmt = -posAmt
			}
			leverageFloat, _ := pos["leverage"].(float64)
			liquidationPrice, _ := pos["liquidationPrice"].(float64)
			unrealizedProfit, _ := pos["unRealizedProfit"].(float64)

			at.lastPositionSnapshot[posKey] = logger.PositionSnapshot{
				Symbol:           symbol,
				Side:             side,
				PositionAmt:      posAmt,
				EntryPrice:       entryPrice,
				MarkPrice:        markPrice,
				UnrealizedProfit: unrealizedProfit,
				Leverage:         leverageFloat,
				LiquidationPrice: liquidationPrice,
			}

			matchedCount++
			log.Printf("✓ [%s] 恢复持仓: %s %s (开仓时间: %s, 开仓价: %.4f, 日志记录匹配)",
				at.name, symbol, side, openTime.Format("2006-01-02 15:04:05"), openPrice)
		} else {
			// 日志中没有找到，可能是手动开仓或程序重启前的开仓
			// 仍然记录，但使用当前时间作为首次出现时间
			at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

			entryPrice, _ := pos["entryPrice"].(float64)
			markPrice, _ := pos["markPrice"].(float64)
			posAmt, _ := pos["positionAmt"].(float64)
			if posAmt < 0 {
				posAmt = -posAmt
			}
			leverage, _ := pos["leverage"].(float64)
			liquidationPrice, _ := pos["liquidationPrice"].(float64)
			unrealizedProfit, _ := pos["unRealizedProfit"].(float64)

			at.lastPositionSnapshot[posKey] = logger.PositionSnapshot{
				Symbol:           symbol,
				Side:             side,
				PositionAmt:      posAmt,
				EntryPrice:       entryPrice,
				MarkPrice:        markPrice,
				UnrealizedProfit: unrealizedProfit,
				Leverage:         leverage,
				LiquidationPrice: liquidationPrice,
			}

			log.Printf("⚠️  [%s] 持仓未匹配到日志: %s %s (可能是手动开仓或重启前开仓)", at.name, symbol, side)
		}
	}

	at.positionSnapshotMutex.Unlock()

	// 4. 同步到仓位追踪器，确保活跃仓位也能恢复
	at.positionTracker.SyncWithExchange(currentPositions)
	log.Printf("🔄 [%s] 已同步 %d 个持仓到仓位追踪器", at.name, len(currentPositions))

	log.Printf("✓ [%s] 持仓上下文恢复完成: %d/%d 个持仓匹配到日志记录", at.name, matchedCount, len(currentPositions))

	return nil
}

func (at *AutoTrader) logTradeExecution(decision decision.Decision, action logger.DecisionAction) {
	if at.decisionLogger == nil {
		return
	}

	if action.Action != "open_long" && action.Action != "open_short" &&
		action.Action != "close_long" && action.Action != "close_short" {
		return
	}

	direction := ""
	switch action.Action {
	case "open_long", "close_long":
		direction = "long"
	case "open_short", "close_short":
		direction = "short"
	}

	reasoning := strings.TrimSpace(decision.Reasoning)
	if !action.Success && action.Error != "" {
		if reasoning != "" {
			reasoning += "\n"
		}
		reasoning += fmt.Sprintf("系统提示: %s", action.Error)
	}

	entry := logger.TradeLogEntry{
		Timestamp:       action.Timestamp,
		TraderID:        at.id,
		CycleNumber:     at.decisionLogger.CurrentCycleNumber() + 1,
		Action:          action.Action,
		Symbol:          action.Symbol,
		Direction:       direction,
		Reasoning:       reasoning,
		Confidence:      decision.Confidence,
		RiskUSD:         decision.RiskUSD,
		Leverage:        decision.Leverage,
		PositionSizeUSD: decision.PositionSizeUSD,
		StopLoss:        decision.StopLoss,
		TakeProfit:      decision.TakeProfit,
		OrderPrice:      action.Price,
		Quantity:        action.Quantity,
		OrderID:         action.OrderID,
		PositionID:      action.PositionID,
		RealizedPnL:     action.RealizedPnL,
		Commission:      action.Commission,
		Success:         action.Success,
		Error:           action.Error,
	}

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	if err := at.decisionLogger.AppendTradeLog(entry); err != nil {
		log.Printf("⚠️ 追加交易日志失败: %v", err)
	}
}

// Run 运行自动交易主循环
func (at *AutoTrader) Run() error {
	at.isRunning = true

	var userStreamStarted bool
	if at.binanceStream.UserEnabled {
		if ft, ok := at.trader.(*FuturesTrader); ok {
			ft.StartBinanceUserStream()
			userStreamStarted = true
		}
	}
	defer func() {
		if userStreamStarted {
			if ft, ok := at.trader.(*FuturesTrader); ok {
				ft.StopBinanceUserStream()
			}
		}
	}()

	// 恢复持仓上下文
	if err := at.RestorePositionContext(); err != nil {
		log.Printf("⚠️  [%s] 恢复持仓上下文失败: %v，将使用当前持仓状态", at.name, err)
		// 不阻断程序运行，继续执行
		// 尝试直接从交易所同步持仓到追踪器
		if positions, err := at.trader.GetPositions(); err == nil && len(positions) > 0 {
			at.positionTracker.SyncWithExchange(positions)
			log.Printf("🔄 [%s] 备用方案：已从交易所同步 %d 个持仓到仓位追踪器", at.name, len(positions))
		}
	}

	// 加载交易所历史成交记录
	if err := at.loadExchangeTrades(); err != nil {
		log.Printf("⚠️  [%s] 加载交易所成交记录失败: %v", at.name, err)
	}

	log.Println("🚀 AI驱动自动交易系统启动")
	log.Printf("💰 初始余额: %.2f USDT", at.initialBalance)
	log.Printf("⚙️  扫描间隔: %v", at.config.ScanInterval)
	log.Println("🤖 AI将全权决定杠杆、仓位大小、止损止盈等参数")

	ticker := time.NewTicker(at.config.ScanInterval)
	defer ticker.Stop()

	// 首次立即执行
	if err := at.runCycle(); err != nil {
		log.Printf("❌ 执行失败: %v", err)
	}

	for at.isRunning {
		select {
		case <-ticker.C:
			if err := at.runCycle(); err != nil {
				log.Printf("❌ 执行失败: %v", err)
			}
		}
	}

	return nil
}

// Stop 停止自动交易
func (at *AutoTrader) Stop() {
	at.isRunning = false
	if at.binanceStream.UserEnabled {
		if ft, ok := at.trader.(*FuturesTrader); ok {
			ft.StopBinanceUserStream()
		}
	}
	log.Println("⏹ 自动交易系统停止")
}

// runCycle 运行一个交易周期（使用AI全权决策）
func (at *AutoTrader) runCycle() error {
	at.callCount++

	// 清除持仓缓存，确保每个周期都获取最新数据
	if ft, ok := at.trader.(*FuturesTrader); ok {
		ft.ClearPositionsCache()
	}

	log.Println()
	log.Println(strings.Repeat("=", 70))
	log.Printf("⏰ %s - AI决策周期 #%d", time.Now().Format("2006-01-02 15:04:05"), at.callCount)
	log.Println(strings.Repeat("=", 70))

	// 创建决策记录
	record := &logger.DecisionRecord{
		ExecutionLog: []string{},
		Success:      true,
	}

	// 1. 检查是否需要停止交易
	if time.Now().Before(at.stopUntil) {
		remaining := at.stopUntil.Sub(time.Now())
		log.Printf("⏸ 风险控制：暂停交易中，剩余 %.0f 分钟", remaining.Minutes())
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("风险控制暂停中，剩余 %.0f 分钟", remaining.Minutes())
		at.decisionLogger.LogDecision(record)
		return nil
	}

	// 2. 重置日盈亏（每天重置）
	if time.Since(at.lastResetTime) > 24*time.Hour {
		at.dailyPnL = 0
		at.lastResetTime = time.Now()
		log.Println("📅 日盈亏已重置")
	}

	// 3. 收集交易上下文（先获取当前持仓状态）
	ctx, err := at.buildTradingContext()
	if err != nil {
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("构建交易上下文失败: %v", err)
		at.decisionLogger.LogDecision(record)
		return fmt.Errorf("构建交易上下文失败: %w", err)
	}

	// 4. 检查被动平仓（止盈止损触发）- 必须在获取持仓状态之后
	if err := at.checkAndRecordPassiveCloses(); err != nil {
		log.Printf("⚠️  检查被动平仓失败: %v", err)
		// 不阻断主流程，继续执行
	}

	// 5. 保存账户状态快照
	record.AccountState = logger.AccountSnapshot{
		TotalBalance:          ctx.Account.TotalEquity,
		AvailableBalance:      ctx.Account.AvailableBalance,
		TotalUnrealizedProfit: ctx.Account.TotalPnL,
		PositionCount:         ctx.Account.PositionCount,
		MarginUsedPct:         ctx.Account.MarginUsedPct,
	}

	// 6. 保存持仓快照到决策记录（不更新 lastPositionSnapshot，由 checkAndRecordPassiveCloses 更新）
	for _, pos := range ctx.Positions {
		posSnapshot := logger.PositionSnapshot{
			Symbol:           pos.Symbol,
			Side:             pos.Side,
			PositionAmt:      pos.Quantity,
			EntryPrice:       pos.EntryPrice,
			MarkPrice:        pos.MarkPrice,
			UnrealizedProfit: pos.UnrealizedPnL,
			Leverage:         float64(pos.Leverage),
			LiquidationPrice: pos.LiquidationPrice,
		}
		record.Positions = append(record.Positions, posSnapshot)
	}

	// 保存候选币种列表
	for _, coin := range ctx.CandidateCoins {
		record.CandidateCoins = append(record.CandidateCoins, coin.Symbol)
	}

	log.Printf("📊 账户净值: %.2f USDT | 可用: %.2f USDT | 持仓: %d",
		ctx.Account.TotalEquity, ctx.Account.AvailableBalance, ctx.Account.PositionCount)

	// 4. 调用AI获取完整决策
	log.Println("🤖 正在请求AI分析并决策...")
	decision, err := decision.GetFullDecision(ctx, at.mcpClient)

	// 即使有错误，也保存思维链、决策和输入prompt（用于debug）
	if decision != nil {
		record.InputPrompt = decision.UserPrompt
		record.CoTTrace = decision.CoTTrace
		if len(decision.Decisions) > 0 {
			decisionJSON, _ := json.MarshalIndent(decision.Decisions, "", "  ")
			record.DecisionJSON = string(decisionJSON)
		}
	}

	if err != nil {
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("获取AI决策失败: %v", err)

		// 打印AI思维链（即使有错误）
		if decision != nil && decision.CoTTrace != "" {
			log.Println()
			log.Println(strings.Repeat("-", 70))
			log.Println("💭 AI思维链分析（错误情况）:")
			log.Println(strings.Repeat("-", 70))
			log.Println(decision.CoTTrace)
			log.Println(strings.Repeat("-", 70))
			log.Println()
		}

		at.decisionLogger.LogDecision(record)
		return fmt.Errorf("获取AI决策失败: %w", err)
	}

	// 5. 打印AI思维链
	log.Println()
	log.Println(strings.Repeat("-", 70))
	log.Println("💭 AI思维链分析:")
	log.Println(strings.Repeat("-", 70))
	log.Println(decision.CoTTrace)
	log.Println(strings.Repeat("-", 70))
	log.Println()

	// 6. 应用学习状态约束
	adjustedDecisions, constraintNotes := at.applyLearningConstraints(ctx, decision.Decisions)
	for _, note := range constraintNotes {
		log.Println(note)
		record.ExecutionLog = append(record.ExecutionLog, note)
	}

	// 7. 打印AI决策
	log.Printf("📋 AI决策列表 (%d 个):\n", len(adjustedDecisions))
	for i, d := range adjustedDecisions {
		log.Printf("  [%d] %s: %s - %s", i+1, d.Symbol, d.Action, d.Reasoning)
		if d.Action == "open_long" || d.Action == "open_short" {
			log.Printf("      杠杆: %dx | 仓位: %.2f USDT | 止损: %.4f | 止盈: %.4f",
				d.Leverage, d.PositionSizeUSD, d.StopLoss, d.TakeProfit)
		}
	}
	log.Println()

	// 8. 对决策排序：确保先平仓后开仓（防止仓位叠加超限）
	sortedDecisions := sortDecisionsByPriority(adjustedDecisions)

	log.Println("🔄 执行顺序（已优化）: 先平仓→后开仓")
	for i, d := range sortedDecisions {
		log.Printf("  [%d] %s %s", i+1, d.Symbol, d.Action)
	}
	log.Println()

	// 预处理：标记需要先平仓再开仓的币种
	requiresClose := make(map[string]bool)
	for _, d := range sortedDecisions {
		if d.Action == "close_long" || d.Action == "close_short" {
			requiresClose[d.Symbol] = true
		}
	}

	failedClose := make(map[string]bool)
	successfulClose := make(map[string]bool)

	// 执行决策并记录结果
	for _, d := range sortedDecisions {
		actionRecord := logger.DecisionAction{
			Action:    d.Action,
			Symbol:    d.Symbol,
			Quantity:  0,
			Leverage:  d.Leverage,
			Price:     0,
			Timestamp: time.Now(),
			Success:   false,
		}

		if (d.Action == "open_long" || d.Action == "open_short") && requiresClose[d.Symbol] {
			if failedClose[d.Symbol] {
				msg := fmt.Sprintf("⚠ 跳过 %s %s：前置平仓失败，禁止直接反向开仓", d.Symbol, d.Action)
				log.Println(msg)
				actionRecord.Error = "前置平仓失败，跳过反向开仓"
				record.ExecutionLog = append(record.ExecutionLog, msg)
				record.Decisions = append(record.Decisions, actionRecord)
				at.logTradeExecution(d, actionRecord)
				continue
			}
			if !successfulClose[d.Symbol] {
				msg := fmt.Sprintf("⚠ 跳过 %s %s：等待前置平仓结果", d.Symbol, d.Action)
				log.Println(msg)
				actionRecord.Error = "前置平仓尚未完成，跳过反向开仓"
				record.ExecutionLog = append(record.ExecutionLog, msg)
				record.Decisions = append(record.Decisions, actionRecord)
				at.logTradeExecution(d, actionRecord)
				continue
			}
		}

		if err := at.executeDecisionWithRecord(&d, &actionRecord); err != nil {
			log.Printf("❌ 执行决策失败 (%s %s): %v", d.Symbol, d.Action, err)
			actionRecord.Error = err.Error()
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("❌ %s %s 失败: %v", d.Symbol, d.Action, err))
			if d.Action == "close_long" || d.Action == "close_short" {
				failedClose[d.Symbol] = true
			}
		} else {
			actionRecord.Success = true
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("✓ %s %s 成功", d.Symbol, d.Action))
			// 成功执行后短暂延迟
			time.Sleep(1 * time.Second)
			if d.Action == "close_long" || d.Action == "close_short" {
				successfulClose[d.Symbol] = true
			}
		}

		record.Decisions = append(record.Decisions, actionRecord)
		at.logTradeExecution(
			d,
			actionRecord,
		)
	}

	// 8. 保存决策记录
	if err := at.decisionLogger.LogDecision(record); err != nil {
		log.Printf("⚠ 保存决策记录失败: %v", err)
		// ... existing code ...
	}

	return nil
}

// buildTradingContext 构建交易上下文
func (at *AutoTrader) buildTradingContext() (*decision.Context, error) {
	// 1. 获取账户信息
	balance, err := at.trader.GetBalance()
	if err != nil {
		return nil, fmt.Errorf("获取账户余额失败: %w", err)
	}

	// 获取账户字段
	totalWalletBalance := 0.0
	totalUnrealizedProfit := 0.0
	availableBalance := 0.0

	if wallet, ok := balance["totalWalletBalance"].(float64); ok {
		totalWalletBalance = wallet
	}
	if unrealized, ok := balance["totalUnrealizedProfit"].(float64); ok {
		totalUnrealizedProfit = unrealized
	}
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}

	// Total Equity = 钱包余额 + 未实现盈亏
	totalEquity := totalWalletBalance + totalUnrealizedProfit

	// 2. 获取持仓信息
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}

	// 同步仓位追踪器，确保活跃仓位与交易所一致
	at.positionTracker.SyncWithExchange(positions)

	var positionInfos []decision.PositionInfo
	totalMarginUsed := 0.0

	// 当前持仓的key集合（用于清理已平仓的记录）
	currentPositionKeys := make(map[string]bool)

	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		entryPrice := pos["entryPrice"].(float64)
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity // 空仓数量为负，转为正数
		}
		unrealizedPnl := pos["unRealizedProfit"].(float64)
		liquidationPrice := pos["liquidationPrice"].(float64)

		// 计算占用保证金（估算）
		leverage := 10 // 默认值，实际应该从持仓信息获取
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}
		marginUsed := (quantity * markPrice) / float64(leverage)
		totalMarginUsed += marginUsed

		// 计算盈亏百分比
		pnlPct := 0.0
		priceChangePct := ((markPrice - entryPrice) / entryPrice) * 100
		if side == "long" {
			pnlPct = priceChangePct
		} else {
			pnlPct = -priceChangePct
		}

		// 跟踪持仓首次出现时间
		posKey := symbol + "_" + side
		currentPositionKeys[posKey] = true
		if _, exists := at.positionFirstSeenTime[posKey]; !exists {
			// 新持仓，记录当前时间
			at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()
		}
		updateTime := at.positionFirstSeenTime[posKey]

		// 查找对应的仓位ID
		position := at.positionTracker.GetActivePositionBySymbol(symbol, side)
		positionID := ""
		if position != nil {
			positionID = position.ID
		}

		positionInfos = append(positionInfos, decision.PositionInfo{
			Symbol:           symbol,
			Side:             side,
			EntryPrice:       entryPrice,
			MarkPrice:        markPrice,
			Quantity:         quantity,
			Leverage:         leverage,
			UnrealizedPnL:    unrealizedPnl,
			UnrealizedPnLPct: pnlPct,
			LiquidationPrice: liquidationPrice,
			MarginUsed:       marginUsed,
			UpdateTime:       updateTime,
			PositionID:       positionID,
		})
	}

	// 清理已平仓的持仓记录
	for key := range at.positionFirstSeenTime {
		if !currentPositionKeys[key] {
			delete(at.positionFirstSeenTime, key)
		}
	}

	// 3. 获取合并的候选币种池（AI500 + OI Top，去重）
	// 无论有没有持仓，都分析相同数量的币种（让AI看到所有好机会）
	// AI会根据保证金使用率和现有持仓情况，自己决定是否要换仓
	const ai500Limit = 20 // AI500取前20个评分最高的币种

	// 获取合并后的币种池（AI500 + OI Top）
	mergedPool, err := pool.GetMergedCoinPool(ai500Limit)
	if err != nil {
		return nil, fmt.Errorf("获取合并币种池失败: %w", err)
	}

	// 构建候选币种列表（包含来源信息）
	var candidateCoins []decision.CandidateCoin
	for _, symbol := range mergedPool.AllSymbols {
		sources := mergedPool.SymbolSources[symbol]
		candidateCoins = append(candidateCoins, decision.CandidateCoin{
			Symbol:  symbol,
			Sources: sources, // "ai500" 和/或 "oi_top"
		})
	}

	log.Printf("📋 合并币种池: AI500前%d + OI_Top20 = 总计%d个候选币种",
		ai500Limit, len(candidateCoins))

	// 4. 计算总盈亏
	totalPnL := totalEquity - at.initialBalance
	totalPnLPct := 0.0
	if at.initialBalance > 0 {
		totalPnLPct = (totalPnL / at.initialBalance) * 100
	}

	marginUsedPct := 0.0
	if totalEquity > 0 {
		marginUsedPct = (totalMarginUsed / totalEquity) * 100
	}

	// 5. 分析历史表现（读取尽可能多的记录以获取完整历史）
	// 使用一个足够大的数字（如5000）来覆盖所有历史记录
	// 假设每个扫描周期约2分钟，5000个周期可覆盖一周以上数据
	performance, err := at.decisionLogger.AnalyzePerformance(5000)
	if err != nil {
		log.Printf("⚠️  分析历史表现失败: %v", err)
		// 不影响主流程，继续执行（但设置performance为nil以避免传递错误数据）
		performance = nil
	}

	// 5.1. 获取交易所成交记录并生成快照（仅用于展示，不用于AI学习）
	if exchangeTrades, err := at.GetTradeHistory(1000); err != nil {
		log.Printf("⚠️  获取交易所历史成交失败: %v，将仅使用决策日志数据", err)
	} else {
		var localTrades []logger.TradeOutcome
		if performance != nil {
			localTrades = performance.RecentTrades
		}
		summaries := summarizeExchangeTrades(exchangeTrades, localTrades, at.aiModel, at.config.ScanInterval)
		if err := at.writeExchangeTrades(summaries); err != nil {
			log.Printf("⚠️  写入交易所成交汇总失败: %v", err)
		}
	}

	// 5.2. 保存仓位历史（用于AI学习）
	if performance != nil && len(performance.RecentTrades) > 0 {
		if err := at.writePositionHistory(performance.RecentTrades); err != nil {
			log.Printf("⚠️  保存仓位历史失败: %v", err)
		}
	}

	// 5.3. 更新学习状态缓存
	var learningState *learning.State
	if at.learningManager != nil {
		if performance != nil {
			if state, err := at.learningManager.Update(at.id, performance); err != nil {
				log.Printf("⚠️  生成学习状态失败: %v", err)
				learningState = at.getLearningState()
			} else {
				at.updateLearningState(state)
				learningState = state.Copy()
			}
		} else if cached := at.getLearningState(); cached != nil {
			learningState = cached
		} else if state, err := at.learningManager.Load(at.id); err == nil {
			at.updateLearningState(state)
			learningState = state.Copy()
		}
	}

	historyWarmup := false
	if performance == nil || performance.TotalTrades == 0 {
		historyWarmup = true
	}
	if learningState == nil {
		historyWarmup = true
	}
	if historyWarmup {
		if !at.historyWarmupNotified {
			log.Printf("ℹ️ [%s] 历史日志与学习数据为空，AI 将从头积累数据，当前绩效统计仅供参考", at.name)
			at.historyWarmupNotified = true
		}
	} else {
		if at.historyWarmupNotified {
			at.historyWarmupNotified = false
		}
	}

	if learningState != nil && len(candidateCoins) > 0 {
		avoidSymbols := make(map[string]string)
		for _, directive := range learningState.Symbols {
			if directive.Action == "avoid" {
				avoidSymbols[strings.ToUpper(directive.Symbol)] = directive.Reason
			}
		}
		if len(avoidSymbols) > 0 {
			var filtered []decision.CandidateCoin
			var removed []string
			for _, coin := range candidateCoins {
				symbolUpper := strings.ToUpper(coin.Symbol)
				if reason, blocked := avoidSymbols[symbolUpper]; blocked {
					removed = append(removed, fmt.Sprintf("%s(%s)", symbolUpper, reason))
					continue
				}
				filtered = append(filtered, coin)
			}
			if len(removed) > 0 {
				log.Printf("🎯 学习状态过滤候选币种: %s", strings.Join(removed, ", "))
			}
			candidateCoins = filtered
		}
	}

	// 6. 获取新闻摘要（如果新闻服务可用）
	var newsDigests interface{}
	if newsSvc := news.GetDefaultService(); newsSvc != nil {
		digests := newsSvc.GetDigests()
		if len(digests) > 0 {
			newsDigests = digests
		}
	}

	// 7. 构建上下文
	ctx := &decision.Context{
		CurrentTime:     time.Now().Format("2006-01-02 15:04:05"),
		RuntimeMinutes:  int(time.Since(at.startTime).Minutes()),
		CallCount:       at.callCount,
		BTCETHLeverage:  at.config.BTCETHLeverage,  // 使用配置的杠杆倍数
		AltcoinLeverage: at.config.AltcoinLeverage, // 使用配置的杠杆倍数
		HistoryWarmup:   historyWarmup,
		Account: decision.AccountInfo{
			TotalEquity:      totalEquity,
			AvailableBalance: availableBalance,
			TotalPnL:         totalPnL,
			TotalPnLPct:      totalPnLPct,
			MarginUsed:       totalMarginUsed,
			MarginUsedPct:    marginUsedPct,
			PositionCount:    len(positionInfos),
		},
		Positions:      positionInfos,
		CandidateCoins: candidateCoins,
		Performance:    performance,   // 添加历史表现分析
		NewsDigests:    newsDigests,   // 添加新闻摘要
		LearningState:  learningState, // 学习状态
	}

	return ctx, nil
}

func (at *AutoTrader) applyLearningConstraints(ctx *decision.Context, decisions []decision.Decision) ([]decision.Decision, []string) {
	state, ok := ctx.LearningStateCopy()
	if !ok || state == nil {
		return decisions, nil
	}

	avoid := make(map[string]string)
	for _, directive := range state.Symbols {
		if directive.Action == "avoid" {
			avoid[strings.ToUpper(directive.Symbol)] = directive.Reason
		}
	}

	risk := state.Risk
	execution := state.Execution
	globalNotes := []string{}
	profitPct := ctx.Account.TotalPnLPct

	lock := func(target float64, maxPositions int) {
		if target <= 0 {
			target = 0.25
		}
		if risk.PositionSizeMultiplier <= 0 || risk.PositionSizeMultiplier > target {
			risk.PositionSizeMultiplier = target
		}
		if risk.MaxConcurrentPositions == 0 || risk.MaxConcurrentPositions > maxPositions {
			risk.MaxConcurrentPositions = maxPositions
		}
	}

	switch {
	case profitPct >= 15:
		lock(0.25, 1)
		globalNotes = append(globalNotes, fmt.Sprintf("🔐 超级锁盈模式：净值+%.1f%%，仓位系数≤%.2f，最大持仓≤%d", profitPct, risk.PositionSizeMultiplier, risk.MaxConcurrentPositions))
	case profitPct >= 8:
		lock(0.4, 2)
		globalNotes = append(globalNotes, fmt.Sprintf("🔒 锁盈保护：净值+%.1f%%，仓位系数≤%.2f，最大持仓≤%d", profitPct, risk.PositionSizeMultiplier, risk.MaxConcurrentPositions))
	case profitPct <= -6:
		if risk.PositionSizeMultiplier <= 0 || risk.PositionSizeMultiplier > 0.6 {
			risk.PositionSizeMultiplier = 0.6
		}
		if risk.ConfidenceThreshold < 70 {
			risk.ConfidenceThreshold = 70
		} else if risk.ConfidenceThreshold > 72 {
			risk.ConfidenceThreshold = 72
		}
		globalNotes = append(globalNotes, fmt.Sprintf("🧊 回撤减速器：净值%.1f%%，仓位系数收紧至%.2f", profitPct, risk.PositionSizeMultiplier))
	}

	minConfidence := 72
	if risk.ConfidenceThreshold > minConfidence {
		minConfidence = risk.ConfidenceThreshold
	}

	maxSlots := math.MaxInt32
	if risk.MaxConcurrentPositions > 0 {
		remaining := risk.MaxConcurrentPositions - ctx.Account.PositionCount
		if remaining < 0 {
			remaining = 0
		}
		maxSlots = remaining
	}

	var cooldownUntil time.Time
	if risk.CooldownMinutes > 0 && !at.lastOpenActionTime.IsZero() {
		cooldownUntil = at.lastOpenActionTime.Add(time.Duration(risk.CooldownMinutes) * time.Minute)
	}

	now := time.Now()
	if execution.MaxTradesPerHour > 0 {
		at.trimOpenHistory(now.Add(-1 * time.Hour))
	}
	minHoldDuration := time.Duration(execution.MinHoldMinutes) * time.Minute

	result := make([]decision.Decision, 0, len(decisions))
	var notes []string
	scheduledOpens := 0

	for _, d := range decisions {
		action := d.Action
		isOpen := action == "open_long" || action == "open_short"
		isClose := action == "close_long" || action == "close_short"
		symbolUpper := strings.ToUpper(d.Symbol)

		if isClose && minHoldDuration > 0 {
			side := "long"
			if action == "close_short" {
				side = "short"
			}
			posKey := d.Symbol + "_" + side
			firstSeen := at.positionFirstSeenTime[posKey]
			if firstSeen == 0 {
				for _, pos := range ctx.Positions {
					if strings.EqualFold(pos.Symbol, d.Symbol) && strings.EqualFold(pos.Side, side) {
						firstSeen = pos.UpdateTime
						break
					}
				}
			}
			if firstSeen > 0 {
				held := now.Sub(time.UnixMilli(firstSeen))
				if held < minHoldDuration {
					notes = append(notes, fmt.Sprintf("⚠️ %s %s 被拒绝：持仓仅%.1f分钟，需满足最少 %d 分钟", symbolUpper, action, held.Minutes(), execution.MinHoldMinutes))
					continue
				}
			}
		}

		if isOpen {
			if reason, blocked := avoid[symbolUpper]; blocked {
				notes = append(notes, fmt.Sprintf("⚠️ %s %s 被拒绝：学习状态标记为避开（%s）", symbolUpper, action, reason))
				continue
			}
			if minConfidence > 0 && d.Confidence > 0 && d.Confidence < minConfidence {
				notes = append(notes, fmt.Sprintf("⚠️ %s %s 被拒绝：信心 %d 低于最低要求 %d", symbolUpper, action, d.Confidence, minConfidence))
				continue
			}
			if maxSlots <= 0 {
				notes = append(notes, fmt.Sprintf(
					"⚠️ %s %s 被拒绝：学习状态将最大持仓收紧至 %d（当前持仓 %d）",
					symbolUpper, action, risk.MaxConcurrentPositions, ctx.Account.PositionCount,
				))
				continue
			}
			if !cooldownUntil.IsZero() && now.Before(cooldownUntil) {
				remaining := cooldownUntil.Sub(now).Minutes()
				notes = append(notes, fmt.Sprintf("⚠️ %s %s 被拒绝：冷静期尚未结束（剩余 %.1f 分钟）", symbolUpper, action, remaining))
				continue
			}
			if execution.MaxTradesPerHour > 0 {
				executed := len(at.openActionHistory) + scheduledOpens
				if float64(executed) >= execution.MaxTradesPerHour {
					notes = append(notes, fmt.Sprintf("⚠️ %s %s 被拒绝：每小时最多 %.2f 笔，最近1小时已登记 %d 笔", symbolUpper, action, execution.MaxTradesPerHour, executed))
					continue
				}
			}
			if risk.PositionSizeMultiplier > 0 && d.PositionSizeUSD > 0 {
				adjusted := d.PositionSizeUSD * risk.PositionSizeMultiplier
				if math.Abs(adjusted-d.PositionSizeUSD) > 1e-6 {
					notes = append(notes, fmt.Sprintf("ℹ️ %s 仓位调整: %.2f → %.2f (乘 %.2f)", symbolUpper, d.PositionSizeUSD, adjusted, risk.PositionSizeMultiplier))
					d.PositionSizeUSD = adjusted
				}
			}
			if d.PositionSizeUSD > 0 && d.PositionSizeUSD < minPositionUSD {
				notes = append(notes, fmt.Sprintf("⚠️ %s %s 被拒绝：仓位 %.2f 低于最低开仓金额 %.2f", symbolUpper, action, d.PositionSizeUSD, minPositionUSD))
				continue
			}
			maxSlots--
			scheduledOpens++
			if risk.CooldownMinutes > 0 {
				cooldownUntil = now.Add(time.Duration(risk.CooldownMinutes) * time.Minute)
			}
		}

		result = append(result, d)
	}

	return result, append(globalNotes, notes...)
}

func (at *AutoTrader) trimOpenHistory(cutoff time.Time) {
	if len(at.openActionHistory) == 0 {
		return
	}
	idx := 0
	for _, ts := range at.openActionHistory {
		if ts.After(cutoff) {
			at.openActionHistory[idx] = ts
			idx++
		}
	}
	at.openActionHistory = at.openActionHistory[:idx]
}

func (at *AutoTrader) recordOpenExecution() {
	now := time.Now()
	at.lastOpenActionTime = now
	at.openActionHistory = append(at.openActionHistory, now)
	at.trimOpenHistory(now.Add(-1 * time.Hour))
}

// executeDecisionWithRecord 执行AI决策并记录详细信息
func (at *AutoTrader) executeDecisionWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	switch decision.Action {
	case "open_long":
		return at.executeOpenLongWithRecord(decision, actionRecord)
	case "open_short":
		return at.executeOpenShortWithRecord(decision, actionRecord)
	case "close_long":
		return at.executeCloseLongWithRecord(decision, actionRecord)
	case "close_short":
		return at.executeCloseShortWithRecord(decision, actionRecord)
	case "hold", "wait":
		// 无需执行，仅记录
		return nil
	default:
		return fmt.Errorf("未知的action: %s", decision.Action)
	}
}

// ensurePositionAffordable 根据可用保证金动态缩放仓位，避免交易所拒单
func (at *AutoTrader) ensurePositionAffordable(decision *decision.Decision) error {
	if decision.Leverage <= 0 {
		return fmt.Errorf("杠杆必须大于0: %d", decision.Leverage)
	}
	if decision.PositionSizeUSD <= 0 {
		return fmt.Errorf("仓位价值必须大于0: %.2f", decision.PositionSizeUSD)
	}
	if decision.PositionSizeUSD < minPositionUSD {
		return fmt.Errorf("%s 仓位 %.2f 低于最低开仓金额 %.2f", decision.Symbol, decision.PositionSizeUSD, minPositionUSD)
	}

	balance, err := at.trader.GetBalance()
	if err != nil {
		log.Printf("  ⚠ 获取账户余额失败，跳过保证金校验: %v", err)
		return nil
	}

	avail, _ := balance["availableBalance"].(float64)
	if avail <= 0 {
		return fmt.Errorf("保证金不足: 可用余额 %.2f USDT", avail)
	}

	requiredMargin := decision.PositionSizeUSD / float64(decision.Leverage)
	if requiredMargin <= 0 {
		return fmt.Errorf("保证金计算异常: 仓位 %.2f, 杠杆 %d", decision.PositionSizeUSD, decision.Leverage)
	}

	maxMargin := avail * 0.95 // 预留5%缓冲，避免因行情波动被拒单
	if requiredMargin <= maxMargin {
		return nil
	}

	maxPositionUSD := maxMargin * float64(decision.Leverage)
	if maxPositionUSD < minPositionUSD {
		return fmt.Errorf("保证金不足: 需要 %.2f USDT, 实际仅 %.2f USDT，无法满足最小下单 %.2f USDT", requiredMargin, avail, minPositionUSD)
	}

	log.Printf("  ⚠ 可用保证金 %.2f USDT 不足以支撑 %.2f USDT 仓位，缩减至 %.2f USDT (杠杆 %dx)", avail, decision.PositionSizeUSD, maxPositionUSD, decision.Leverage)
	decision.PositionSizeUSD = maxPositionUSD
	return nil
}

// executeOpenLongWithRecord 执行开多仓并记录详细信息
func (at *AutoTrader) executeOpenLongWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("%s  📈 开多仓: %s%s", colorGreen, decision.Symbol, colorReset)

	// ⚠️ 关键：检查是否已有同币种同方向持仓，如果有则拒绝开仓（防止仓位叠加超限）
	positions, err := at.trader.GetPositions()
	if err == nil {
		for _, pos := range positions {
			if pos["symbol"] == decision.Symbol && pos["side"] == "long" {
				return fmt.Errorf("❌ %s 已有多仓，拒绝开仓以防止仓位叠加超限。如需换仓，请先给出 close_long 决策", decision.Symbol)
			}
		}
	}

	// 根据可用保证金缩放仓位，避免提交到交易所被拒单
	if err := at.ensurePositionAffordable(decision); err != nil {
		return err
	}

	// 获取当前价格
	marketData, err := market.Get(decision.Symbol)
	if err != nil {
		return err
	}

	// 计算数量
	quantity := decision.PositionSizeUSD / marketData.CurrentPrice
	actionRecord.Quantity = quantity
	actionRecord.Price = marketData.CurrentPrice
	actionRecord.Leverage = decision.Leverage

	// 开仓
	order, err := at.trader.OpenLong(decision.Symbol, quantity, decision.Leverage)
	if err != nil {
		return err
	}

	// 记录订单ID
	orderID := int64(0)
	if id, ok := order["orderId"].(int64); ok {
		orderID = id
		actionRecord.OrderID = id
	}

	log.Printf("%s  ✓ 开仓成功，订单ID: %v, 数量: %.4f (杠杆 %dx)%s", colorGreen, orderID, quantity, decision.Leverage, colorReset)

	// 获取实际成交信息
	actualPrice := marketData.CurrentPrice
	actualQuantity := quantity
	if summary, err := at.getOrderFillSummary(decision.Symbol, orderID); err == nil && summary.Quantity > 0 {
		actualPrice = summary.AvgPrice
		actualQuantity = summary.Quantity
		actionRecord.Price = summary.AvgPrice
		actionRecord.Quantity = summary.Quantity
		log.Printf("  📊 实际成交: 价格 %.4f (决策价 %.4f), 数量 %.4f", summary.AvgPrice, marketData.CurrentPrice, summary.Quantity)
	}

	// 创建仓位记录
	position := at.positionTracker.CreatePosition(
		decision.Symbol,
		"long",
		actualPrice,    // 使用实际成交价格
		actualQuantity, // 使用实际成交数量
		decision.Leverage,
		fmt.Sprintf("%d", orderID),
	)

	// 记录仓位ID到决策中（用于后续关联）
	decision.PositionID = position.ID
	actionRecord.PositionID = position.ID

	// 记录开仓时间
	posKey := decision.Symbol + "_long"
	at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

	// 设置止损止盈
	if err := at.trader.SetStopLoss(decision.Symbol, "LONG", actualQuantity, decision.StopLoss); err != nil {
		log.Printf("  ⚠ 设置止损失败: %v", err)
	}
	if err := at.trader.SetTakeProfit(decision.Symbol, "LONG", actualQuantity, decision.TakeProfit); err != nil {
		log.Printf("  ⚠ 设置止盈失败: %v", err)
	}

	at.recordOpenExecution()
	return nil
}

// executeOpenShortWithRecord 执行开空仓并记录详细信息
func (at *AutoTrader) executeOpenShortWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("%s  📉 开空仓: %s%s", colorRed, decision.Symbol, colorReset)

	// ⚠️ 关键：检查是否已有同币种同方向持仓，如果有则拒绝开仓（防止仓位叠加超限）
	positions, err := at.trader.GetPositions()
	if err == nil {
		for _, pos := range positions {
			if pos["symbol"] == decision.Symbol && pos["side"] == "short" {
				return fmt.Errorf("❌ %s 已有空仓，拒绝开仓以防止仓位叠加超限。如需换仓，请先给出 close_short 决策", decision.Symbol)
			}
		}
	}

	// 根据可用保证金缩放仓位
	if err := at.ensurePositionAffordable(decision); err != nil {
		return err
	}

	// 获取当前价格
	marketData, err := market.Get(decision.Symbol)
	if err != nil {
		return err
	}

	// 计算数量
	quantity := decision.PositionSizeUSD / marketData.CurrentPrice
	actionRecord.Quantity = quantity
	actionRecord.Price = marketData.CurrentPrice
	actionRecord.Leverage = decision.Leverage

	// 开仓
	order, err := at.trader.OpenShort(decision.Symbol, quantity, decision.Leverage)
	if err != nil {
		return err
	}

	// 记录订单ID
	orderID := int64(0)
	if id, ok := order["orderId"].(int64); ok {
		orderID = id
		actionRecord.OrderID = id
	}

	log.Printf("%s  ✓ 开仓成功，订单ID: %v, 数量: %.4f (杠杆 %dx)%s", colorGreen, orderID, quantity, decision.Leverage, colorReset)

	// 获取实际成交信息
	actualPrice := marketData.CurrentPrice
	actualQuantity := quantity
	if summary, err := at.getOrderFillSummary(decision.Symbol, orderID); err == nil && summary.Quantity > 0 {
		actualPrice = summary.AvgPrice
		actualQuantity = summary.Quantity
		actionRecord.Price = summary.AvgPrice
		actionRecord.Quantity = summary.Quantity
		log.Printf("  📊 实际成交: 价格 %.4f (决策价 %.4f), 数量 %.4f", summary.AvgPrice, marketData.CurrentPrice, summary.Quantity)
	}

	// 创建仓位记录
	position := at.positionTracker.CreatePosition(
		decision.Symbol,
		"short",
		actualPrice,    // 使用实际成交价格
		actualQuantity, // 使用实际成交数量
		decision.Leverage,
		fmt.Sprintf("%d", orderID),
	)

	// 记录仓位ID到决策中
	decision.PositionID = position.ID
	actionRecord.PositionID = position.ID

	// 记录开仓时间
	posKey := decision.Symbol + "_short"
	at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

	// 设置止损止盈
	if err := at.trader.SetStopLoss(decision.Symbol, "SHORT", actualQuantity, decision.StopLoss); err != nil {
		log.Printf("  ⚠ 设置止损失败: %v", err)
	}
	if err := at.trader.SetTakeProfit(decision.Symbol, "SHORT", actualQuantity, decision.TakeProfit); err != nil {
		log.Printf("  ⚠ 设置止盈失败: %v", err)
	}

	at.recordOpenExecution()
	return nil
}

// executeCloseLongWithRecord 执行平多仓并记录详细信息
func (at *AutoTrader) executeCloseLongWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("%s  🔄 平多仓: %s%s", colorBlue, decision.Symbol, colorReset)

	// 获取当前价格
	marketData, err := market.Get(decision.Symbol)
	if err != nil {
		return err
	}
	actionRecord.Price = marketData.CurrentPrice

	// 查找对应的仓位
	var position *Position
	if decision.PositionID != "" {
		// 如果AI提供了position_id，优先使用
		pos, err := at.positionTracker.GetPosition(decision.PositionID)
		if err != nil {
			log.Printf("  ⚠️ 未找到指定的仓位ID: %s，将尝试按币种查找", decision.PositionID)
		} else {
			position = pos
		}
	}

	// 如果没有找到，按币种和方向查找
	if position == nil {
		position = at.positionTracker.GetActivePositionBySymbol(decision.Symbol, "long")
		if position == nil {
			return fmt.Errorf("未找到 %s 的多仓", decision.Symbol)
		}
	}

	// 记录仓位ID
	actionRecord.PositionID = position.ID

	// 平仓
	order, err := at.trader.CloseLong(decision.Symbol, 0) // 0 = 全部平仓
	if err != nil {
		return err
	}

	// 记录订单ID
	orderID := int64(0)
	if id, ok := order["orderId"].(int64); ok {
		orderID = id
		actionRecord.OrderID = id
	}

	closedQty := position.RemainingQty
	pnl := (marketData.CurrentPrice - position.OpenPrice) * closedQty
	commission := closedQty * marketData.CurrentPrice * 0.0005 // 估算手续费

	if summary, err := at.getOrderFillSummary(decision.Symbol, orderID); err == nil && summary.Quantity > 0 {
		actionRecord.Price = summary.AvgPrice
		actionRecord.Quantity = summary.Quantity
		actionRecord.RealizedPnL = summary.RealizedPnL
		actionRecord.Commission = summary.Commission
		closedQty = math.Min(position.RemainingQty, summary.Quantity)
		pnl = summary.RealizedPnL
		commission = summary.Commission
		log.Printf("  📊 实际成交: 平仓价 %.4f (决策价 %.4f), 数量 %.4f, 盈亏 %.4f", summary.AvgPrice, marketData.CurrentPrice, summary.Quantity, summary.RealizedPnL)
	} else {
		actionRecord.Quantity = closedQty
		actionRecord.RealizedPnL = pnl
		actionRecord.Commission = commission
		log.Printf("  ⚠️ 未能获取实际成交信息，使用市场价 %.4f", marketData.CurrentPrice)
	}

	// 更新仓位追踪器
	err = at.positionTracker.ClosePosition(
		position.ID,
		actionRecord.Price,
		closedQty,
		pnl,
		commission,
		fmt.Sprintf("%d", orderID),
		"用户平仓",
	)
	if err != nil {
		log.Printf("  ⚠️ 更新仓位追踪器失败: %v", err)
	}

	log.Printf("%s  ✓ 平仓成功，仓位ID: %s%s", colorGreen, position.ID, colorReset)

	// 立即保存最新的交易记录到持久化存储
	go at.saveRecentTradesToHistory()

	return nil
}

// executeCloseShortWithRecord 执行平空仓并记录详细信息
func (at *AutoTrader) executeCloseShortWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("%s  🔄 平空仓: %s%s", colorBlue, decision.Symbol, colorReset)

	// 获取当前价格
	marketData, err := market.Get(decision.Symbol)
	if err != nil {
		return err
	}
	actionRecord.Price = marketData.CurrentPrice

	// 查找对应的仓位
	var position *Position
	if decision.PositionID != "" {
		// 如果AI提供了position_id，优先使用
		pos, err := at.positionTracker.GetPosition(decision.PositionID)
		if err != nil {
			log.Printf("  ⚠️ 未找到指定的仓位ID: %s，将尝试按币种查找", decision.PositionID)
		} else {
			position = pos
		}
	}

	// 如果没有找到，按币种和方向查找
	if position == nil {
		position = at.positionTracker.GetActivePositionBySymbol(decision.Symbol, "short")
		if position == nil {
			return fmt.Errorf("未找到 %s 的空仓", decision.Symbol)
		}
	}

	// 记录仓位ID
	actionRecord.PositionID = position.ID

	// 平仓
	order, err := at.trader.CloseShort(decision.Symbol, 0) // 0 = 全部平仓
	if err != nil {
		return err
	}

	// 记录订单ID
	orderID := int64(0)
	if id, ok := order["orderId"].(int64); ok {
		orderID = id
		actionRecord.OrderID = id
	}

	closedQty := position.RemainingQty
	pnl := (position.OpenPrice - marketData.CurrentPrice) * closedQty
	commission := closedQty * marketData.CurrentPrice * 0.0005 // 估算手续费

	if summary, err := at.getOrderFillSummary(decision.Symbol, orderID); err == nil && summary.Quantity > 0 {
		actionRecord.Price = summary.AvgPrice
		actionRecord.Quantity = summary.Quantity
		actionRecord.RealizedPnL = summary.RealizedPnL
		actionRecord.Commission = summary.Commission
		closedQty = math.Min(position.RemainingQty, summary.Quantity)
		pnl = summary.RealizedPnL
		commission = summary.Commission
		log.Printf("  📊 实际成交: 平仓价 %.4f (决策价 %.4f), 数量 %.4f, 盈亏 %.4f", summary.AvgPrice, marketData.CurrentPrice, summary.Quantity, summary.RealizedPnL)
	} else {
		actionRecord.Quantity = closedQty
		actionRecord.RealizedPnL = pnl
		actionRecord.Commission = commission
		log.Printf("  ⚠️ 未能获取实际成交信息，使用市场价 %.4f", marketData.CurrentPrice)
	}

	// 更新仓位追踪器
	err = at.positionTracker.ClosePosition(
		position.ID,
		actionRecord.Price,
		closedQty,
		pnl,
		commission,
		fmt.Sprintf("%d", orderID),
		"用户平仓",
	)
	if err != nil {
		log.Printf("  ⚠️ 更新仓位追踪器失败: %v", err)
	}

	log.Printf("  ✓ 平仓成功，仓位ID: %s", position.ID)

	// 立即保存最新的交易记录到持久化存储
	go at.saveRecentTradesToHistory()

	return nil
}

// GetID 获取trader ID
func (at *AutoTrader) GetID() string {
	return at.id
}

// GetName 获取trader名称
func (at *AutoTrader) GetName() string {
	return at.name
}

// GetAIModel 获取AI模型
func (at *AutoTrader) GetAIModel() string {
	return at.strategyLabel
}

// GetDecisionLogger 获取决策日志记录器
func (at *AutoTrader) GetDecisionLogger() *logger.DecisionLogger {
	return at.decisionLogger
}

// ForceLearningUpdate 强制刷新学习状态
func (at *AutoTrader) ForceLearningUpdate() (*learning.State, error) {
	if at.learningManager == nil {
		return nil, fmt.Errorf("学习状态管理器未初始化")
	}

	performance, err := at.decisionLogger.AnalyzePerformance(5000)
	if err != nil {
		return nil, fmt.Errorf("分析历史表现失败: %w", err)
	}

	state, err := at.learningManager.Update(at.id, performance)
	if err != nil {
		return nil, fmt.Errorf("生成学习状态失败: %w", err)
	}

	at.updateLearningState(state)
	return state.Copy(), nil
}

// GetStatus 获取系统状态（用于API）
func (at *AutoTrader) GetStatus() map[string]interface{} {
	strategyName := strings.ToUpper(at.strategyLabel)

	return map[string]interface{}{
		"trader_id":       at.id,
		"trader_name":     at.name,
		"ai_model":        at.strategyLabel,
		"exchange":        at.exchange,
		"is_running":      at.isRunning,
		"start_time":      at.startTime.Format(time.RFC3339),
		"runtime_minutes": int(time.Since(at.startTime).Minutes()),
		"call_count":      at.callCount,
		"initial_balance": at.initialBalance,
		"scan_interval":   at.config.ScanInterval.String(),
		"stop_until":      at.stopUntil.Format(time.RFC3339),
		"last_reset_time": at.lastResetTime.Format(time.RFC3339),
		"ai_provider":     strategyName,
	}
}

// GetAccountInfo 获取账户信息（用于API）
func (at *AutoTrader) GetAccountInfo() (map[string]interface{}, error) {
	balance, err := at.trader.GetBalance()
	if err != nil {
		return nil, fmt.Errorf("获取余额失败: %w", err)
	}

	// 获取账户字段
	totalWalletBalance := 0.0
	totalUnrealizedProfit := 0.0
	availableBalance := 0.0

	if wallet, ok := balance["totalWalletBalance"].(float64); ok {
		totalWalletBalance = wallet
	}
	if unrealized, ok := balance["totalUnrealizedProfit"].(float64); ok {
		totalUnrealizedProfit = unrealized
	}
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}

	// Total Equity = 钱包余额 + 未实现盈亏
	totalEquity := totalWalletBalance + totalUnrealizedProfit

	// 获取持仓计算总保证金
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}

	totalMarginUsed := 0.0
	totalUnrealizedPnL := 0.0
	for _, pos := range positions {
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity
		}
		unrealizedPnl := pos["unRealizedProfit"].(float64)
		totalUnrealizedPnL += unrealizedPnl

		leverage := 10
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}
		marginUsed := (quantity * markPrice) / float64(leverage)
		totalMarginUsed += marginUsed
	}

	totalPnL := totalEquity - at.initialBalance
	totalPnLPct := 0.0
	if at.initialBalance > 0 {
		totalPnLPct = (totalPnL / at.initialBalance) * 100
	}

	marginUsedPct := 0.0
	if totalEquity > 0 {
		marginUsedPct = (totalMarginUsed / totalEquity) * 100
	}

	return map[string]interface{}{
		// 核心字段
		"total_equity":      totalEquity,           // 账户净值 = wallet + unrealized
		"wallet_balance":    totalWalletBalance,    // 钱包余额（不含未实现盈亏）
		"unrealized_profit": totalUnrealizedProfit, // 未实现盈亏（从API）
		"available_balance": availableBalance,      // 可用余额

		// 盈亏统计
		"total_pnl":            totalPnL,           // 总盈亏 = equity - initial
		"total_pnl_pct":        totalPnLPct,        // 总盈亏百分比
		"total_unrealized_pnl": totalUnrealizedPnL, // 未实现盈亏（从持仓计算）
		"initial_balance":      at.initialBalance,  // 初始余额
		"daily_pnl":            at.dailyPnL,        // 日盈亏

		// 持仓信息
		"position_count":  len(positions),  // 持仓数量
		"margin_used":     totalMarginUsed, // 保证金占用
		"margin_used_pct": marginUsedPct,   // 保证金使用率
	}, nil
}

// GetPositions 获取持仓列表（用于API）
func (at *AutoTrader) GetPositions() ([]map[string]interface{}, error) {
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}

	var result []map[string]interface{}
	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		entryPrice := pos["entryPrice"].(float64)
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity
		}
		unrealizedPnl := pos["unRealizedProfit"].(float64)
		liquidationPrice := pos["liquidationPrice"].(float64)

		leverage := 10
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}

		pnlPct := 0.0
		priceChangePct := ((markPrice - entryPrice) / entryPrice) * 100
		if side == "long" {
			pnlPct = priceChangePct
		} else {
			pnlPct = -priceChangePct
		}

		marginUsed := (quantity * markPrice) / float64(leverage)

		// 获取保证金模式（统一为小写 canonical 值）
		marginType := "isolated" // 默认逐仓
		if mt, ok := pos["marginType"].(string); ok {
			mt = strings.ToLower(strings.TrimSpace(mt))
			switch mt {
			case "cross", "crossed", "cross_margin", "crossed_margin":
				marginType = "cross"
			default:
				marginType = "isolated"
			}
		}

		posMap := map[string]interface{}{
			"symbol":           symbol,
			"side":             side,
			"entryPrice":       entryPrice, // 使用币安标准字段名
			"markPrice":        markPrice,  // 使用币安标准字段名
			"positionAmt":      quantity,   // 使用币安标准字段名
			"leverage":         leverage,
			"unRealizedProfit": unrealizedPnl,    // 使用币安标准字段名
			"liquidationPrice": liquidationPrice, // 使用币安标准字段名
			"marginType":       marginType,
			// 保留前端使用的字段（兼容性）
			"entry_price":        entryPrice,
			"mark_price":         markPrice,
			"quantity":           quantity,
			"unrealized_pnl":     unrealizedPnl,
			"unrealized_pnl_pct": pnlPct,
			"liquidation_price":  liquidationPrice,
			"margin_used":        marginUsed,
			"margin_type":        marginType,
		}

		// 获取该币种的止盈止损订单
		orders, err := at.trader.GetOpenOrders(symbol)
		if err == nil {
			// 将订单信息添加到持仓中
			var stopLoss, takeProfit float64
			for _, order := range orders {
				if orderType, ok := order["orderType"].(string); ok {
					if price, ok := order["stopPrice"].(float64); ok {
						if orderType == "stop_loss" {
							stopLoss = price
						} else if orderType == "take_profit" {
							takeProfit = price
						}
					}
				}
			}
			if stopLoss > 0 {
				posMap["stop_loss"] = stopLoss
			}
			if takeProfit > 0 {
				posMap["take_profit"] = takeProfit
			}
		} else {
			log.Printf("⚠️ 获取 %s 的订单失败: %v", symbol, err)
		}

		result = append(result, posMap)
	}

	return result, nil
}

// GetTradeHistory 获取历史成交记录（从交易所API获取）
func (at *AutoTrader) GetTradeHistory(limit int) ([]map[string]interface{}, error) {
	return at.trader.GetTradeHistory(limit)
}

// sortDecisionsByPriority 对决策排序：先平仓，再开仓，最后hold/wait
// 这样可以避免换仓时仓位叠加超限
func sortDecisionsByPriority(decisions []decision.Decision) []decision.Decision {
	if len(decisions) <= 1 {
		return decisions
	}

	// 定义优先级
	getActionPriority := func(action string) int {
		switch action {
		case "close_long", "close_short":
			return 1 // 最高优先级：先平仓
		case "open_long", "open_short":
			return 2 // 次优先级：后开仓
		case "hold", "wait":
			return 3 // 最低优先级：观望
		default:
			return 999 // 未知动作放最后
		}
	}

	// 复制决策列表
	sorted := make([]decision.Decision, len(decisions))
	copy(sorted, decisions)

	// 按优先级排序
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if getActionPriority(sorted[i].Action) > getActionPriority(sorted[j].Action) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// checkAndRecordPassiveCloses 检查并记录被动平仓（止盈止损触发）
// 注意：此方法应该在 buildTradingContext() 之后调用，以确保使用最新的持仓状态
func (at *AutoTrader) checkAndRecordPassiveCloses() error {
	// 1. 获取当前持仓（从刚构建的上下文中获取，避免重复API调用）
	// 但为了确保准确性，还是从交易所获取
	currentPositions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("获取当前持仓失败: %w", err)
	}

	// 2. 构建当前持仓key集合
	currentKeys := make(map[string]bool)
	currentPosMap := make(map[string]logger.PositionSnapshot)
	for _, pos := range currentPositions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		key := symbol + "_" + side
		currentKeys[key] = true

		entryPrice, _ := pos["entryPrice"].(float64)
		markPrice, _ := pos["markPrice"].(float64)
		quantity, _ := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity
		}
		leverage, _ := pos["leverage"].(float64)
		liquidationPrice, _ := pos["liquidationPrice"].(float64)
		unrealizedProfit, _ := pos["unRealizedProfit"].(float64)

		currentPosMap[key] = logger.PositionSnapshot{
			Symbol:           symbol,
			Side:             side,
			PositionAmt:      quantity,
			EntryPrice:       entryPrice,
			MarkPrice:        markPrice,
			UnrealizedProfit: unrealizedProfit,
			Leverage:         leverage,
			LiquidationPrice: liquidationPrice,
		}
	}

	// 3. 对比上次快照，找出消失的持仓
	at.positionSnapshotMutex.RLock()
	lastSnapshot := make(map[string]logger.PositionSnapshot)
	for k, v := range at.lastPositionSnapshot {
		lastSnapshot[k] = v
	}
	at.positionSnapshotMutex.RUnlock()

	var closedPositions []logger.PositionSnapshot
	for key, pos := range lastSnapshot {
		if !currentKeys[key] {
			// 持仓消失了，可能是被平仓
			closedPositions = append(closedPositions, pos)
		}
	}

	// 4. 对于每个消失的持仓，创建被动平仓记录
	if len(closedPositions) > 0 {
		log.Printf("🔍 [%s] 检测到 %d 个持仓被平仓（可能是止盈止损触发）", at.name, len(closedPositions))

		// 获取最近的交易历史（用于查找实际平仓价格）
		tradeHistory, tradeErr := at.trader.GetTradeHistory(100)

		for _, pos := range closedPositions {
			// 获取当前价格（作为备用平仓价）
			marketData, err := market.Get(pos.Symbol)
			if err != nil {
				log.Printf("⚠️  [%s] 无法获取 %s 的价格，跳过记录", at.name, pos.Symbol)
				continue
			}

			// 尝试从交易历史中找到实际平仓价格
			actualClosePrice := marketData.CurrentPrice // 默认使用当前价格
			actualQuantity := pos.PositionAmt
			actualCommission := 0.0
			actualPnL := 0.0

			if tradeErr == nil {
				// 查找最近的平仓交易（2分钟内）
				cutoffTime := time.Now().Add(-2 * time.Minute).Unix()
				for _, trade := range tradeHistory {
					if symbol, ok := trade["symbol"].(string); ok && symbol == pos.Symbol {
						if timestamp, ok := trade["time"].(int64); ok && timestamp/1000 > cutoffTime {
							// 检查是否是平仓交易
							qty := math.Abs(parseFloatValue(trade["quantity"]))
							side := trade["side"].(string)

							// 判断是否是对应的平仓交易
							isCloseTrade := false
							if pos.Side == "long" && (side == "SELL" || side == "sell") {
								isCloseTrade = true
							} else if pos.Side == "short" && (side == "BUY" || side == "buy") {
								isCloseTrade = true
							}

							if isCloseTrade && math.Abs(qty-pos.PositionAmt) < 0.0001 {
								// 找到匹配的平仓交易
								actualClosePrice = parseFloatValue(trade["price"])
								actualQuantity = qty
								actualCommission = math.Abs(parseFloatValue(trade["commission"]))
								actualPnL = parseFloatValue(trade["realized_pnl"])
								log.Printf("  📊 找到被动平仓的实际成交: %s @ %.4f (市场价 %.4f)", pos.Symbol, actualClosePrice, marketData.CurrentPrice)
								break
							}
						}
					}
				}
			}

			// 获取当前账户状态（用于更准确的记录）
			balance, _ := at.trader.GetBalance()
			totalEquity := 0.0
			availableBalance := 0.0
			if wallet, ok := balance["totalWalletBalance"].(float64); ok {
				if unrealized, ok2 := balance["totalUnrealizedProfit"].(float64); ok2 {
					totalEquity = wallet + unrealized
				}
			}
			if avail, ok := balance["availableBalance"].(float64); ok {
				availableBalance = avail
			}

			// 创建被动平仓记录
			record := &logger.DecisionRecord{
				Timestamp:   time.Now(),
				CycleNumber: at.callCount, // 使用当前周期号
				Success:     true,
				AccountState: logger.AccountSnapshot{
					TotalBalance:          totalEquity,
					AvailableBalance:      availableBalance,
					TotalUnrealizedProfit: 0, // 被动平仓后，未实现盈亏为0
					PositionCount:         len(currentPositions),
					MarginUsedPct:         0, // 简化处理
				},
				ExecutionLog: []string{
					fmt.Sprintf("被动平仓: %s %s (止盈止损触发)", pos.Symbol, pos.Side),
				},
			}

			// 确定平仓动作
			action := "close_long"
			if pos.Side == "short" {
				action = "close_short"
			}

			// 创建平仓动作记录
			actionRecord := logger.DecisionAction{
				Action:      action,
				Symbol:      pos.Symbol,
				Quantity:    actualQuantity,
				Leverage:    int(pos.Leverage),
				Price:       actualClosePrice, // 使用实际成交价格
				Timestamp:   time.Now(),
				Success:     true,
				RealizedPnL: actualPnL,        // 添加实际盈亏
				Commission:  actualCommission, // 添加实际手续费
			}

			record.Decisions = []logger.DecisionAction{actionRecord}

			// 记录到日志
			if err := at.decisionLogger.LogDecision(record); err != nil {
				log.Printf("⚠️  [%s] 记录被动平仓失败: %v", at.name, err)
			} else {
				log.Printf("✓ [%s] 已记录被动平仓: %s %s @ %.4f", at.name, pos.Symbol, pos.Side, actualClosePrice)
			}

			// 取消该币种的所有剩余挂单（止盈/止损）
			if err := at.trader.CancelAllOrders(pos.Symbol); err != nil {
				log.Printf("⚠️  [%s] 取消 %s 剩余挂单失败: %v", at.name, pos.Symbol, err)
			} else {
				log.Printf("✓ [%s] 已取消 %s 的所有剩余挂单（止盈/止损）", at.name, pos.Symbol)
			}
		}
	}

	// 5. 更新持仓快照（使用当前持仓）
	at.positionSnapshotMutex.Lock()
	at.lastPositionSnapshot = currentPosMap
	at.positionSnapshotMutex.Unlock()

	return nil
}

// PositionHistory 仓位历史记录（用于AI学习）
type PositionHistory struct {
	Trades      []logger.TradeOutcome `json:"trades"`
	LastUpdated time.Time             `json:"last_updated"`
}

// ExchangeTrade 交易所成交记录
type ExchangeTrade struct {
	ID                     string  `json:"id"`
	Symbol                 string  `json:"symbol"`
	ModelID                string  `json:"model_id"`
	Side                   string  `json:"side"`
	Quantity               float64 `json:"quantity"`
	OpenQuantity           float64 `json:"open_quantity"`
	Leverage               int     `json:"leverage"`
	EntryPrice             float64 `json:"entry_price"`
	ExitPrice              float64 `json:"exit_price"`
	PositionValue          float64 `json:"position_value"`
	MarginUsed             float64 `json:"margin_used"`
	RealizedGrossPnL       float64 `json:"realized_gross_pnl"`
	RealizedNetPnL         float64 `json:"realized_net_pnl"`
	TotalCommissionDollars float64 `json:"total_commission_dollars"`
	PnLPct                 float64 `json:"pnl_pct"`
	EntryTime              int64   `json:"entry_time"`
	ExitTime               int64   `json:"exit_time"`
	Duration               string  `json:"duration"`
	OrderID                int64   `json:"order_id"`
	Source                 string  `json:"source"`
	WasStopLoss            bool    `json:"was_stop_loss"`
	IsPartialClose         bool    `json:"is_partial_close"`
	CloseNote              string  `json:"close_note,omitempty"`
}

type orderFillSummary struct {
	AvgPrice    float64
	Quantity    float64
	RealizedPnL float64
	Commission  float64
}

type openLot struct {
	price     float64
	remaining float64
	openTime  int64
}

func (at *AutoTrader) getOrderFillSummary(symbol string, orderID int64) (*orderFillSummary, error) {
	if orderID == 0 {
		return nil, fmt.Errorf("orderID 未提供")
	}

	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		trades, err := at.trader.GetTradeHistory(200)
		if err != nil {
			return nil, err
		}

		var totalQty, totalValue, totalPnL, totalCommission float64
		for _, trade := range trades {
			rawOrderID := parseInt64Value(trade["order_id"])
			if rawOrderID != orderID {
				continue
			}
			if symbol != "" && !strings.EqualFold(fmt.Sprint(trade["symbol"]), symbol) {
				continue
			}

			qty := math.Abs(parseFloatValue(trade["quantity"]))
			if qty == 0 {
				continue
			}
			price := parseFloatValue(trade["price"])
			totalQty += qty
			totalValue += qty * price
			totalPnL += parseFloatValue(trade["realized_pnl"])
			totalCommission += math.Abs(parseFloatValue(trade["commission"]))
		}

		if totalQty > 0 {
			return &orderFillSummary{
				AvgPrice:    totalValue / totalQty,
				Quantity:    totalQty,
				RealizedPnL: totalPnL,
				Commission:  totalCommission,
			}, nil
		}

		time.Sleep(300 * time.Millisecond)
	}

	return nil, fmt.Errorf("未找到 %s 的订单 %d 成交记录", symbol, orderID)
}

func parseFloatValue(value interface{}) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f
		}
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return 0
}

func parseInt64Value(value interface{}) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
	case string:
		if i, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return i
		}
	}
	return 0
}

func consumeOpenLots(openQueues map[string][]*openLot, key string, qty float64) (consumedQty float64, consumedValue float64, firstOpenTime int64) {
	if qty <= 0 {
		return 0, 0, 0
	}

	queue := openQueues[key]
	for qty > 0 && len(queue) > 0 {
		current := queue[0]
		take := math.Min(current.remaining, qty)
		consumedQty += take
		consumedValue += take * current.price
		if firstOpenTime == 0 || (current.openTime > 0 && current.openTime < firstOpenTime) {
			firstOpenTime = current.openTime
		}

		current.remaining -= take
		qty -= take

		if current.remaining <= 1e-8 {
			queue = queue[1:]
		} else {
			queue[0] = current
		}
	}

	openQueues[key] = queue
	return consumedQty, consumedValue, firstOpenTime
}

// loadExchangeTrades 加载交易所历史成交记录
func (at *AutoTrader) loadExchangeTrades() error {
	trades, err := at.GetExchangeTrades()
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("📝 [%s] 交易所成交记录文件不存在，将创建新文件", at.name)
			return nil
		}
		return fmt.Errorf("读取交易所成交记录失败: %w", err)
	}

	log.Printf("✓ [%s] 已加载 %d 条交易所成交记录", at.name, len(trades))
	return nil
}

// writeExchangeTrades 将汇总后的交易所成交记录写入快照文件。
func (at *AutoTrader) writeExchangeTrades(trades []ExchangeTrade) error {
	at.exchangeTradesMutex.Lock()
	defer at.exchangeTradesMutex.Unlock()

	data, err := json.MarshalIndent(trades, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化交易所成交记录失败: %w", err)
	}
	if len(trades) == 0 {
		data = []byte("[]\n")
	}
	if err := ioutil.WriteFile(at.exchangeTradesFile, data, 0644); err != nil {
		return fmt.Errorf("写入交易所成交记录失败: %w", err)
	}
	return nil
}

// GetExchangeTrades 返回最新的交易所成交汇总。
func (at *AutoTrader) GetExchangeTrades() ([]ExchangeTrade, error) {
	at.exchangeTradesMutex.RLock()
	defer at.exchangeTradesMutex.RUnlock()

	data, err := ioutil.ReadFile(at.exchangeTradesFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []ExchangeTrade{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return []ExchangeTrade{}, nil
	}

	var trades []ExchangeTrade
	if err := json.Unmarshal(data, &trades); err == nil {
		return trades, nil
	}

	type legacyExchangeTrade struct {
		Symbol        string    `json:"symbol"`
		Side          string    `json:"side"`
		Quantity      float64   `json:"quantity"`
		OpenQuantity  float64   `json:"open_quantity"`
		Leverage      int       `json:"leverage"`
		EntryPrice    float64   `json:"entry_price"`
		ExitPrice     float64   `json:"exit_price"`
		PositionValue float64   `json:"position_value"`
		MarginUsed    float64   `json:"margin_used"`
		PnL           float64   `json:"pn_l"`
		PnLPct        float64   `json:"pn_l_pct"`
		EntryTime     time.Time `json:"entry_time"`
		ExitTime      time.Time `json:"exit_time"`
		Duration      string    `json:"duration"`
		OrderID       int64     `json:"order_id"`
		Source        string    `json:"source"`
		WasStopLoss   bool      `json:"was_stop_loss"`
		IsPartial     bool      `json:"is_partial_close"`
		CloseNote     string    `json:"close_note"`
	}

	var legacy []legacyExchangeTrade
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, err
	}

	converted := make([]ExchangeTrade, 0, len(legacy))
	for _, item := range legacy {
		entryUnix := item.EntryTime.Unix()
		exitUnix := item.ExitTime.Unix()
		if entryUnix == 0 && !item.EntryTime.IsZero() {
			entryUnix = item.EntryTime.Unix()
		}
		if exitUnix == 0 && !item.ExitTime.IsZero() {
			exitUnix = item.ExitTime.Unix()
		}
		tradeID := fmt.Sprintf("%s-%s-%d", at.aiModel, item.Symbol, exitUnix)

		source := item.Source
		if source == "" {
			source = "exchange"
		}

		converted = append(converted, ExchangeTrade{
			ID:                     tradeID,
			Symbol:                 item.Symbol,
			ModelID:                at.aiModel,
			Side:                   item.Side,
			Quantity:               item.Quantity,
			OpenQuantity:           item.OpenQuantity,
			Leverage:               item.Leverage,
			EntryPrice:             item.EntryPrice,
			ExitPrice:              item.ExitPrice,
			PositionValue:          item.PositionValue,
			MarginUsed:             item.MarginUsed,
			RealizedGrossPnL:       item.PnL,
			RealizedNetPnL:         item.PnL,
			TotalCommissionDollars: 0,
			PnLPct:                 item.PnLPct,
			EntryTime:              entryUnix,
			ExitTime:               exitUnix,
			Duration:               item.Duration,
			OrderID:                item.OrderID,
			Source:                 source,
			WasStopLoss:            item.WasStopLoss,
			IsPartialClose:         item.IsPartial,
			CloseNote:              item.CloseNote,
		})
	}

	sort.Slice(converted, func(i, j int) bool {
		return converted[i].ExitTime > converted[j].ExitTime
	})

	return converted, nil
}

// summarizeExchangeTrades 将交易所原始成交合并为标准化的开平仓区间。
func summarizeExchangeTrades(exchangeTrades []map[string]interface{}, localTrades []logger.TradeOutcome, modelID string, scanInterval time.Duration) []ExchangeTrade {
	const epsilon = 1e-8
	if len(exchangeTrades) == 0 {
		return []ExchangeTrade{}
	}
	_ = localTrades // 交易所视图仅依赖交易所数据，忽略本地日志

	// 简化处理：直接使用交易所返回的realized_pnl
	results := make([]ExchangeTrade, 0, len(exchangeTrades))

	parseFloat := func(value interface{}) float64 {
		switch v := value.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case int64:
			return float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
		}
		return 0
	}

	toSeconds := func(raw interface{}) int64 {
		switch v := raw.(type) {
		case int64:
			if v > 1_000_000_000_000 {
				return v / 1000
			}
			return v
		case float64:
			t := int64(v)
			if t > 1_000_000_000_000 {
				return t / 1000
			}
			return t
		case int:
			if v > 1_000_000_000_000 {
				return int64(v) / 1000
			}
			return int64(v)
		case string:
			if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
				if parsed > 1_000_000_000_000 {
					return parsed / 1000
				}
				return parsed
			}
			if parsed, err := time.Parse(time.RFC3339, v); err == nil {
				return parsed.Unix()
			}
		}
		return time.Now().Unix()
	}

	// 先按时间排序
	sort.Slice(exchangeTrades, func(i, j int) bool {
		ti := toSeconds(exchangeTrades[i]["time"])
		tj := toSeconds(exchangeTrades[j]["time"])
		return ti < tj
	})

	// 追踪未平仓位（FIFO）
	openQueues := make(map[string][]*openLot)

	// 用于聚合同一币种在短时间内多次平仓的交易
	type aggregatedTrade struct {
		symbol          string
		side            string
		trades          []map[string]interface{}
		totalQty        float64
		totalPnL        float64
		totalCommission float64
		firstCloseTime  int64
		lastCloseTime   int64
		totalCloseValue float64 // 用于计算加权平均价格
		openTime        int64
		openQty         float64
		openValue       float64
	}

	aggregatedMap := make(map[string]*aggregatedTrade)

	for _, raw := range exchangeTrades {
		action, _ := raw["action"].(string)
		symbol := strings.ToUpper(strings.TrimSpace(fmt.Sprint(raw["symbol"])))
		if symbol == "" {
			continue
		}

		side := "long"
		if strings.Contains(action, "short") {
			side = "short"
		}

		qty := math.Abs(parseFloat(raw["quantity"]))
		price := parseFloat(raw["price"])
		timeUnix := toSeconds(raw["time"])

		key := fmt.Sprintf("%s_%s", symbol, side)

		switch action {
		case "open_long", "open_short":
			// 开仓
			if qty <= epsilon || price <= 0 {
				continue
			}
			lot := &openLot{
				price:     price,
				remaining: qty,
				openTime:  timeUnix,
			}
			openQueues[key] = append(openQueues[key], lot)

		case "close_long", "close_short":
			// 平仓 - 先记录到聚合map中
			// 生成聚合key：使用订单ID（如果有）或时间窗口
			orderID := int64(parseFloat(raw["order_id"]))
			aggKey := ""
			if orderID > 0 {
				// 优先使用订单ID聚合（同一订单的多次平仓）
				aggKey = fmt.Sprintf("%s_%s_order_%d", symbol, side, orderID)
			} else {
				// 否则使用扫描周期时间窗口（更精确的时间窗口）
				intervalSeconds := int64(scanInterval.Seconds())
				if intervalSeconds <= 0 {
					intervalSeconds = 120 // 回退至默认2分钟
				}
				timeWindow := timeUnix / intervalSeconds
				aggKey = fmt.Sprintf("%s_%s_time_%d", symbol, side, timeWindow)
			}

			agg := aggregatedMap[aggKey]
			if agg == nil {
				agg = &aggregatedTrade{
					symbol:         symbol,
					side:           side,
					trades:         []map[string]interface{}{},
					firstCloseTime: timeUnix,
					lastCloseTime:  timeUnix,
				}
				aggregatedMap[aggKey] = agg
			}

			consumedQty, consumedValue, firstOpenTime := consumeOpenLots(openQueues, key, qty)
			if consumedQty > 0 {
				agg.openQty += consumedQty
				agg.openValue += consumedValue
				if agg.openTime == 0 || (firstOpenTime > 0 && firstOpenTime < agg.openTime) {
					agg.openTime = firstOpenTime
				}
			}

			// 累加数据
			agg.trades = append(agg.trades, raw)
			agg.totalQty += qty
			agg.totalPnL += parseFloat(raw["realized_pnl"])
			agg.totalCommission += math.Abs(parseFloat(raw["commission"]))
			agg.totalCloseValue += qty * price // 累加成交金额用于计算加权平均

			if timeUnix < agg.firstCloseTime {
				agg.firstCloseTime = timeUnix
			}
			if timeUnix > agg.lastCloseTime {
				agg.lastCloseTime = timeUnix
			}

		}
	}

	// 处理聚合的交易数据
	for _, agg := range aggregatedMap {
		// 计算加权平均平仓价格
		avgClosePrice := 0.0
		if agg.totalQty > 0 {
			avgClosePrice = agg.totalCloseValue / agg.totalQty
		}

		// 计算持仓时长（精确到秒）
		duration := ""
		holdSeconds := int64(0)
		if agg.openTime > 0 && agg.lastCloseTime > agg.openTime {
			holdSeconds = agg.lastCloseTime - agg.openTime
			d := time.Duration(holdSeconds) * time.Second
			hours := int(d.Hours())
			minutes := int(d.Minutes()) % 60
			seconds := int(d.Seconds()) % 60

			if hours > 0 {
				duration = fmt.Sprintf("%d小时%d分%d秒", hours, minutes, seconds)
			} else if minutes > 0 {
				duration = fmt.Sprintf("%d分%d秒", minutes, seconds)
			} else {
				duration = fmt.Sprintf("%d秒", seconds)
			}
		}

		// 生成详细的关闭说明
		closeNote := ""
		if len(agg.trades) > 1 {
			// 计算平均每笔交易大小
			avgTradeSize := agg.totalQty / float64(len(agg.trades))
			closeNote = fmt.Sprintf("汇总(%d笔,平均%.4f)", len(agg.trades), avgTradeSize)
		} else {
			closeNote = "已平仓"
		}

		// 如果是部分平仓（平仓数量小于开仓数量）
		isPartialClose := false
		if agg.openQty > 0 && agg.totalQty < agg.openQty {
			isPartialClose = true
			closeNote += fmt.Sprintf(" [部分平仓:%.2f%%]", (agg.totalQty/agg.openQty)*100)
		}

		tradeID := fmt.Sprintf("%s-%s-%d", modelID, agg.symbol, agg.lastCloseTime)

		// 计算精确的仓位价值
		positionValue := 0.0
		openPrice := 0.0
		if agg.openQty > 0 {
			openPrice = agg.openValue / agg.openQty
			positionValue = agg.openQty * openPrice
		} else {
			positionValue = agg.totalCloseValue // 如果没有开仓价，使用平仓总价值
		}

		results = append(results, ExchangeTrade{
			ID:                     tradeID,
			Symbol:                 agg.symbol,
			ModelID:                modelID,
			Side:                   agg.side,
			Quantity:               agg.totalQty,
			OpenQuantity:           agg.openQty, // 显示原始开仓量
			Leverage:               0,           // 不显示
			EntryPrice:             openPrice,
			ExitPrice:              avgClosePrice, // 使用加权平均价格
			PositionValue:          positionValue,
			MarginUsed:             0, // 不显示
			RealizedGrossPnL:       agg.totalPnL + agg.totalCommission,
			RealizedNetPnL:         agg.totalPnL,
			TotalCommissionDollars: agg.totalCommission,
			PnLPct:                 0, // 不显示
			EntryTime:              agg.openTime,
			ExitTime:               agg.lastCloseTime,
			Duration:               duration,
			OrderID:                0, // 汇总后没有单一订单ID
			Source:                 "exchange",
			WasStopLoss:            false,
			IsPartialClose:         isPartialClose,
			CloseNote:              closeNote,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ExitTime > results[j].ExitTime
	})

	return results
}

// writePositionHistory 写入仓位历史（用于AI学习）
func (at *AutoTrader) writePositionHistory(trades []logger.TradeOutcome) error {
	at.exchangeTradesMutex.Lock()
	defer at.exchangeTradesMutex.Unlock()

	fileName := fmt.Sprintf("data/position_history/%s.json", at.id)
	dir := filepath.Dir(fileName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 读取现有历史
	var history PositionHistory
	if data, err := os.ReadFile(fileName); err == nil {
		if err := json.Unmarshal(data, &history); err != nil {
			log.Printf("⚠️ 解析仓位历史失败: %v", err)
			history = PositionHistory{}
		}
	}

	// 合并新交易（避免重复）
	existingIDs := make(map[string]bool)
	for _, trade := range history.Trades {
		// 使用更详细的ID：币种_平仓时间_开仓时间_数量
		id := fmt.Sprintf("%s_%d_%d_%.4f", trade.Symbol, trade.CloseTime.Unix(), trade.OpenTime.Unix(), trade.Quantity)
		existingIDs[id] = true
	}

	for _, trade := range trades {
		// 使用更详细的ID：币种_平仓时间_开仓时间_数量
		id := fmt.Sprintf("%s_%d_%d_%.4f", trade.Symbol, trade.CloseTime.Unix(), trade.OpenTime.Unix(), trade.Quantity)
		if !existingIDs[id] {
			history.Trades = append(history.Trades, trade)
		}
	}

	// 按时间倒序排序
	sort.Slice(history.Trades, func(i, j int) bool {
		return history.Trades[i].CloseTime.After(history.Trades[j].CloseTime)
	})

	// 限制最多保存1000条
	if len(history.Trades) > 1000 {
		history.Trades = history.Trades[:1000]
	}

	history.LastUpdated = time.Now()

	// 写入文件
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	if err := os.WriteFile(fileName, data, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	return nil
}

// GetPositionHistory 获取仓位历史（用于AI学习）
func (at *AutoTrader) GetPositionHistory() ([]logger.TradeOutcome, error) {
	at.exchangeTradesMutex.RLock()
	defer at.exchangeTradesMutex.RUnlock()

	fileName := fmt.Sprintf("data/position_history/%s.json", at.id)

	data, err := os.ReadFile(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			return []logger.TradeOutcome{}, nil
		}
		return nil, fmt.Errorf("读取仓位历史失败: %w", err)
	}

	var history PositionHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, fmt.Errorf("解析仓位历史失败: %w", err)
	}

	return history.Trades, nil
}

// GetPositionTracker 获取仓位追踪器
func (at *AutoTrader) GetPositionTracker() *PositionTracker {
	return at.positionTracker
}

// ClearPositionsCache 清除持仓缓存（强制下次获取最新数据）
func (at *AutoTrader) ClearPositionsCache() {
	if ft, ok := at.trader.(*FuturesTrader); ok {
		ft.ClearPositionsCache()
	}
}

// saveRecentTradesToHistory 立即分析并保存最近的交易到持久化存储
func (at *AutoTrader) saveRecentTradesToHistory() {
	// 分析最近的交易
	performance, err := at.decisionLogger.AnalyzePerformance(1000)
	if err != nil {
		log.Printf("⚠️ 分析交易记录失败: %v", err)
		return
	}

	if len(performance.RecentTrades) == 0 {
		return
	}

	// 保存到持久化存储
	if err := at.writePositionHistory(performance.RecentTrades); err != nil {
		log.Printf("⚠️ 保存仓位历史失败: %v", err)
	} else {
		log.Printf("✓ 已保存 %d 条交易记录到仓位历史", len(performance.RecentTrades))
	}
}

func (at *AutoTrader) GetStartTime() time.Time {
	return at.startTime
}
