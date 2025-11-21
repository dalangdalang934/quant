package logger

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DecisionRecord 决策记录
type DecisionRecord struct {
	Timestamp      time.Time          `json:"timestamp"`       // 决策时间
	CycleNumber    int                `json:"cycle_number"`    // 周期编号
	InputPrompt    string             `json:"input_prompt"`    // 策略输入上下文
	CoTTrace       string             `json:"cot_trace"`       // 决策分析轨迹
	DecisionJSON   string             `json:"decision_json"`   // 决策JSON
	AccountState   AccountSnapshot    `json:"account_state"`   // 账户状态快照
	Positions      []PositionSnapshot `json:"positions"`       // 持仓快照
	CandidateCoins []string           `json:"candidate_coins"` // 候选币种列表
	Decisions      []DecisionAction   `json:"decisions"`       // 执行的决策
	ExecutionLog   []string           `json:"execution_log"`   // 执行日志
	Success        bool               `json:"success"`         // 是否成功
	ErrorMessage   string             `json:"error_message"`   // 错误信息（如果有）
}

// AccountSnapshot 账户状态快照
type AccountSnapshot struct {
	TotalBalance          float64 `json:"total_balance"`
	AvailableBalance      float64 `json:"available_balance"`
	TotalUnrealizedProfit float64 `json:"total_unrealized_profit"`
	PositionCount         int     `json:"position_count"`
	MarginUsedPct         float64 `json:"margin_used_pct"`
}

// PositionSnapshot 持仓快照
type PositionSnapshot struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"`
	PositionAmt      float64 `json:"position_amt"`
	EntryPrice       float64 `json:"entry_price"`
	MarkPrice        float64 `json:"mark_price"`
	UnrealizedProfit float64 `json:"unrealized_profit"`
	Leverage         float64 `json:"leverage"`
	LiquidationPrice float64 `json:"liquidation_price"`
}

// DecisionAction 决策动作
type DecisionAction struct {
	Action      string    `json:"action"`                 // open_long, open_short, close_long, close_short
	Symbol      string    `json:"symbol"`                 // 币种
	Quantity    float64   `json:"quantity"`               // 数量
	Leverage    int       `json:"leverage"`               // 杠杆（开仓时）
	Price       float64   `json:"price"`                  // 执行价格
	OrderID     int64     `json:"order_id"`               // 订单ID
	Timestamp   time.Time `json:"timestamp"`              // 执行时间
	Success     bool      `json:"success"`                // 是否成功
	Error       string    `json:"error"`                  // 错误信息
	PositionID  string    `json:"position_id"`            // 关联的仓位ID
	RealizedPnL float64   `json:"realized_pnl,omitempty"` // 实际平仓盈亏
	Commission  float64   `json:"commission,omitempty"`   // 实际手续费
}

// TradeLogEntry 单笔开/平仓日志（便于用户复盘）
type TradeLogEntry struct {
	Timestamp       time.Time `json:"timestamp"`
	TraderID        string    `json:"trader_id"`
	CycleNumber     int       `json:"cycle_number"`
	Action          string    `json:"action"`
	Symbol          string    `json:"symbol"`
	Direction       string    `json:"direction"`
	Reasoning       string    `json:"reasoning"`
	Confidence      int       `json:"confidence,omitempty"`
	RiskUSD         float64   `json:"risk_usd,omitempty"`
	Leverage        int       `json:"leverage,omitempty"`
	PositionSizeUSD float64   `json:"position_size_usd,omitempty"`
	StopLoss        float64   `json:"stop_loss,omitempty"`
	TakeProfit      float64   `json:"take_profit,omitempty"`
	OrderPrice      float64   `json:"order_price,omitempty"`
	Quantity        float64   `json:"quantity,omitempty"`
	OrderID         int64     `json:"order_id,omitempty"`
	PositionID      string    `json:"position_id,omitempty"`
	RealizedPnL     float64   `json:"realized_pnl,omitempty"`
	Commission      float64   `json:"commission,omitempty"`
	Success         bool      `json:"success"`
	Error           string    `json:"error,omitempty"`
}

const (
	decisionFilePrefix = "decision_"
	decisionFileSuffix = ".json"
	metaFileName       = "logmeta.json"
	metaTimeFormat     = time.RFC3339
	decisionTimeLayout = "20060102_150405"
)

type logMeta struct {
	CycleNumber     int    `json:"cycle_number"`
	LatestFile      string `json:"latest_file"`
	LatestTimestamp string `json:"latest_timestamp"`
	PeriodStart     string `json:"period_start"`
	PeriodSource    string `json:"period_source"`
	UpdatedAt       string `json:"updated_at"`
}

// DecisionLogger 决策日志记录器
type DecisionLogger struct {
	logDir      string
	cycleNumber int
	metaPath    string
	meta        logMeta
	metaMu      sync.RWMutex
	tradeLogDir string
	tradeLogMu  sync.Mutex
}

// NewDecisionLogger 创建决策日志记录器
func NewDecisionLogger(logDir string) *DecisionLogger {
	if logDir == "" {
		logDir = "decision_logs"
	}

	// 确保日志目录存在
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Printf("⚠ 创建日志目录失败: %v\n", err)
	}

	l := &DecisionLogger{
		logDir:      logDir,
		cycleNumber: 0,
		metaPath:    filepath.Join(logDir, metaFileName),
		tradeLogDir: filepath.Join("data", "trade_logs"),
	}

	if err := os.MkdirAll(l.tradeLogDir, 0755); err != nil {
		fmt.Printf("⚠ 创建交易日志目录失败: %v\n", err)
	}

	if err := l.loadMeta(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("⚠️ 读取日志元数据失败: %v", err)
		}
		if rebuildErr := l.rebuildMeta(); rebuildErr != nil {
			log.Printf("⚠️ 重建日志元数据失败: %v", rebuildErr)
		}
	}
	return l
}

// AppendTradeLog 记录开/平仓执行日志（JSONL），支持后续脚本归档
func (l *DecisionLogger) AppendTradeLog(entry TradeLogEntry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	fileName := fmt.Sprintf("trade_%s.jsonl", entry.Timestamp.Format("20060102"))
	tradeDir := l.tradeLogDir
	if entry.TraderID != "" {
		tradeDir = filepath.Join(tradeDir, entry.TraderID)
	}

	if err := os.MkdirAll(tradeDir, 0755); err != nil {
		return fmt.Errorf("创建交易日志目录失败: %w", err)
	}

	filePath := filepath.Join(tradeDir, fileName)

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化交易日志失败: %w", err)
	}

	l.tradeLogMu.Lock()
	defer l.tradeLogMu.Unlock()

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("打开交易日志文件失败: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("写入交易日志失败: %w", err)
	}

	return nil
}

// LogDecision 记录决策
func (l *DecisionLogger) LogDecision(record *DecisionRecord) error {
	l.cycleNumber++
	record.CycleNumber = l.cycleNumber
	record.Timestamp = time.Now().UTC()

	// 生成文件名：decision_YYYYMMDD_HHMMSS_cycleN.json
	filename := fmt.Sprintf("%s%s_cycle%d%s",
		decisionFilePrefix,
		record.Timestamp.Format(decisionTimeLayout),
		record.CycleNumber,
		decisionFileSuffix)

	filePath := filepath.Join(l.logDir, filename)

	// 序列化为JSON（带缩进，方便阅读）
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化决策记录失败: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("写入决策记录失败: %w", err)
	}

	l.updateMetaAfterWrite(filename, record.Timestamp, record.CycleNumber)

	fmt.Printf("📝 决策记录已保存: %s\n", filename)
	return nil
}

func (l *DecisionLogger) updateMetaAfterWrite(filename string, ts time.Time, cycle int) {
	l.metaMu.Lock()
	defer l.metaMu.Unlock()

	l.cycleNumber = cycle
	if cycle > l.meta.CycleNumber {
		l.meta.CycleNumber = cycle
	}
	l.meta.LatestFile = filename
	l.meta.LatestTimestamp = formatMetaTime(ts)

	existingStart, err := parseMetaTime(l.meta.PeriodStart)
	if err != nil {
		log.Printf("⚠️ 解析日志元数据的 period_start 失败: %v", err)
		existingStart = time.Time{}
	}
	if existingStart.IsZero() || ts.Before(existingStart) {
		l.meta.PeriodStart = formatMetaTime(ts)
		l.meta.PeriodSource = "decision_log"
	}

	l.meta.UpdatedAt = formatMetaTime(time.Now().UTC())
	if err := l.saveMetaLocked(); err != nil {
		log.Printf("⚠️ 保存日志元数据失败: %v", err)
	}
}

func (l *DecisionLogger) loadMeta() error {
	l.metaMu.Lock()
	defer l.metaMu.Unlock()

	data, err := os.ReadFile(l.metaPath)
	if err != nil {
		return err
	}

	var meta logMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("解析日志元数据失败: %w", err)
	}

	l.meta = meta
	if meta.CycleNumber > 0 {
		l.cycleNumber = meta.CycleNumber
	}

	return nil
}

func (l *DecisionLogger) rebuildMeta() error {
	l.metaMu.Lock()
	defer l.metaMu.Unlock()

	entries, err := os.ReadDir(l.logDir)
	if err != nil {
		return fmt.Errorf("读取日志目录失败: %w", err)
	}

	var maxCycle int
	var latestFile string
	var latestTs time.Time
	var periodStart time.Time

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == metaFileName {
			continue
		}

		ts, cycle, ok := parseDecisionFilename(name)
		if !ok {
			continue
		}

		if cycle > maxCycle {
			maxCycle = cycle
		}
		if periodStart.IsZero() || ts.Before(periodStart) {
			periodStart = ts
		}
		if latestTs.IsZero() || ts.After(latestTs) {
			latestTs = ts
			latestFile = name
		}
	}

	periodSource := "runtime_start"
	if !periodStart.IsZero() {
		periodSource = "decision_log"
	}

	l.cycleNumber = maxCycle
	l.meta = logMeta{
		CycleNumber:     maxCycle,
		LatestFile:      latestFile,
		LatestTimestamp: formatMetaTime(latestTs),
		PeriodStart:     formatMetaTime(periodStart),
		PeriodSource:    periodSource,
		UpdatedAt:       formatMetaTime(time.Now().UTC()),
	}

	if err := l.saveMetaLocked(); err != nil {
		return err
	}

	return nil
}

func (l *DecisionLogger) saveMetaLocked() error {
	data, err := json.MarshalIndent(l.meta, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化日志元数据失败: %w", err)
	}
	if err := os.MkdirAll(l.logDir, 0o755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}
	if err := os.WriteFile(l.metaPath, data, 0o644); err != nil {
		return fmt.Errorf("写入日志元数据失败: %w", err)
	}
	return nil
}

func formatMetaTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(metaTimeFormat)
}

func parseMetaTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(metaTimeFormat, value)
}

func parseDecisionFilename(name string) (time.Time, int, bool) {
	if !strings.HasPrefix(name, decisionFilePrefix) || !strings.HasSuffix(name, decisionFileSuffix) {
		return time.Time{}, 0, false
	}
	trimmed := strings.TrimSuffix(strings.TrimPrefix(name, decisionFilePrefix), decisionFileSuffix)
	parts := strings.Split(trimmed, "_cycle")
	if len(parts) != 2 {
		return time.Time{}, 0, false
	}

	ts, err := time.ParseInLocation(decisionTimeLayout, parts[0], time.UTC)
	if err != nil {
		return time.Time{}, 0, false
	}

	cycle, err := strconv.Atoi(parts[1])
	if err != nil {
		return time.Time{}, 0, false
	}

	return ts, cycle, true
}

func (l *DecisionLogger) listDecisionFiles() ([]string, error) {
	entries, err := os.ReadDir(l.logDir)
	if err != nil {
		return nil, fmt.Errorf("读取日志目录失败: %w", err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == metaFileName {
			continue
		}
		if !strings.HasPrefix(name, decisionFilePrefix) || !strings.HasSuffix(name, decisionFileSuffix) {
			continue
		}
		files = append(files, name)
	}

	sort.Strings(files)
	return files, nil
}

func (l *DecisionLogger) findEarliestRecordTime() (time.Time, bool) {
	files, err := l.listDecisionFiles()
	if err != nil {
		log.Printf("⚠️ 枚举决策日志失败: %v", err)
		return time.Time{}, false
	}
	for _, name := range files {
		if ts, _, ok := parseDecisionFilename(name); ok {
			return ts, true
		}
	}
	return time.Time{}, false
}

func (l *DecisionLogger) findFirstRecordNotBefore(target time.Time) (time.Time, bool) {
	files, err := l.listDecisionFiles()
	if err != nil {
		log.Printf("⚠️ 枚举决策日志失败: %v", err)
		return time.Time{}, false
	}
	for _, name := range files {
		ts, _, ok := parseDecisionFilename(name)
		if !ok {
			continue
		}
		if !ts.Before(target) {
			return ts, true
		}
	}
	return time.Time{}, false
}

func (l *DecisionLogger) updateMetaPeriodStart(start time.Time, source string) {
	if start.IsZero() {
		return
	}
	l.metaMu.Lock()
	defer l.metaMu.Unlock()

	l.meta.PeriodStart = formatMetaTime(start)
	if source != "" {
		l.meta.PeriodSource = source
	}
	l.meta.UpdatedAt = formatMetaTime(time.Now().UTC())
	if err := l.saveMetaLocked(); err != nil {
		log.Printf("⚠️ 保存日志元数据失败: %v", err)
	}
}

// DetectPeriodStart 根据日志记录与运行时间判定周期起点
func (l *DecisionLogger) DetectPeriodStart(runtimeStart time.Time) (time.Time, string) {
	l.metaMu.RLock()
	metaCopy := l.meta
	l.metaMu.RUnlock()

	periodStart, err := parseMetaTime(metaCopy.PeriodStart)
	if err != nil {
		log.Printf("⚠️ 解析日志元数据的 period_start 失败: %v", err)
		periodStart = time.Time{}
	}
	latestTs, err := parseMetaTime(metaCopy.LatestTimestamp)
	if err != nil {
		log.Printf("⚠️ 解析日志元数据的 latest_timestamp 失败: %v", err)
		latestTs = time.Time{}
	}

	if !runtimeStart.IsZero() {
		runtimeStart = runtimeStart.UTC()
	}

	if periodStart.IsZero() {
		if ts, ok := l.findEarliestRecordTime(); ok {
			periodStart = ts
			l.updateMetaPeriodStart(ts, "decision_log")
		}
	}

	if runtimeStart.IsZero() {
		if !periodStart.IsZero() {
			return periodStart, defaultSource(metaCopy.PeriodSource)
		}
		return time.Time{}, "runtime_start"
	}

	if periodStart.IsZero() {
		l.updateMetaPeriodStart(runtimeStart, "runtime_start")
		return runtimeStart, "runtime_start"
	}

	if runtimeStart.Before(periodStart) || runtimeStart.Equal(periodStart) {
		return periodStart, defaultSource(metaCopy.PeriodSource)
	}

	if !latestTs.IsZero() && runtimeStart.After(latestTs) {
		l.updateMetaPeriodStart(runtimeStart, "runtime_start")
		return runtimeStart, "runtime_start"
	}

	if ts, ok := l.findFirstRecordNotBefore(runtimeStart); ok {
		l.updateMetaPeriodStart(ts, "decision_log")
		return ts, "decision_log"
	}

	l.updateMetaPeriodStart(runtimeStart, "runtime_start")
	return runtimeStart, "runtime_start"
}

func defaultSource(src string) string {
	if src == "" {
		return "decision_log"
	}
	return src
}

// CurrentCycleNumber 返回当前累计的周期编号
func (l *DecisionLogger) CurrentCycleNumber() int {
	return l.cycleNumber
}

// GetLatestRecords 获取最近N条记录（按时间正序：从旧到新）
// 改进：读取所有文件，按记录时间戳排序，确保不遗漏任何记录
func (l *DecisionLogger) GetLatestRecords(n int) ([]*DecisionRecord, error) {
	entries, err := os.ReadDir(l.logDir)
	if err != nil {
		return nil, fmt.Errorf("读取日志目录失败: %w", err)
	}

	// 读取所有文件，按记录的时间戳排序（而不是文件修改时间）
	var allRecords []*DecisionRecord
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if name == metaFileName {
			continue
		}

		fullpath := filepath.Join(l.logDir, name)
		data, err := os.ReadFile(fullpath)
		if err != nil {
			log.Printf("⚠️ 读取决策日志失败 %s: %v", fullpath, err)
			continue
		}

		var record DecisionRecord
		if err := json.Unmarshal(data, &record); err != nil {
			log.Printf("⚠️ 解析决策日志失败 %s: %v", fullpath, err)
			continue
		}

		allRecords = append(allRecords, &record)
	}

	// 按时间戳排序（从旧到新）
	sort.Slice(allRecords, func(i, j int) bool {
		return allRecords[i].Timestamp.Before(allRecords[j].Timestamp)
	})

	// 如果指定了n，只返回最近的N条
	if n > 0 && len(allRecords) > n {
		allRecords = allRecords[len(allRecords)-n:]
	}

	return allRecords, nil
}

// GetRecordByDate 获取指定日期的所有记录
func (l *DecisionLogger) GetRecordByDate(date time.Time) ([]*DecisionRecord, error) {
	dateStr := date.Format("20060102")
	pattern := filepath.Join(l.logDir, fmt.Sprintf("decision_%s_*.json", dateStr))

	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("查找日志文件失败: %w", err)
	}

	var records []*DecisionRecord
	for _, filepath := range files {
		data, err := os.ReadFile(filepath)
		if err != nil {
			log.Printf("⚠️ 读取决策日志失败 %s: %v", filepath, err)
			continue
		}

		var record DecisionRecord
		if err := json.Unmarshal(data, &record); err != nil {
			log.Printf("⚠️ 解析决策日志失败 %s: %v", filepath, err)
			continue
		}

		records = append(records, &record)
	}

	return records, nil
}

// CleanOldRecords 清理N天前的旧记录
func (l *DecisionLogger) CleanOldRecords(days int) error {
	cutoffTime := time.Now().AddDate(0, 0, -days)

	entries, err := os.ReadDir(l.logDir)
	if err != nil {
		return fmt.Errorf("读取日志目录失败: %w", err)
	}

	removedCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if entry.Name() == metaFileName {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			log.Printf("⚠️ 读取文件信息失败 %s: %v", entry.Name(), err)
			continue
		}

		if info.ModTime().Before(cutoffTime) {
			filepath := filepath.Join(l.logDir, entry.Name())
			if err := os.Remove(filepath); err != nil {
				fmt.Printf("⚠ 删除旧记录失败 %s: %v\n", entry.Name(), err)
				continue
			}
			removedCount++
		}
	}

	if removedCount > 0 {
		fmt.Printf("🗑️ 已清理 %d 条旧记录（%d天前）\n", removedCount, days)
	}

	return nil
}

// GetStatistics 获取统计信息
func (l *DecisionLogger) GetStatistics() (*Statistics, error) {
	entries, err := os.ReadDir(l.logDir)
	if err != nil {
		return nil, fmt.Errorf("读取日志目录失败: %w", err)
	}

	stats := &Statistics{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if entry.Name() == metaFileName {
			continue
		}

		filepath := filepath.Join(l.logDir, entry.Name())
		data, err := os.ReadFile(filepath)
		if err != nil {
			log.Printf("⚠️ 读取决策日志失败 %s: %v", filepath, err)
			continue
		}

		var record DecisionRecord
		if err := json.Unmarshal(data, &record); err != nil {
			log.Printf("⚠️ 解析决策日志失败 %s: %v", filepath, err)
			continue
		}

		stats.TotalCycles++

		for _, action := range record.Decisions {
			if action.Success {
				switch action.Action {
				case "open_long", "open_short":
					stats.TotalOpenPositions++
				case "close_long", "close_short":
					stats.TotalClosePositions++
				}
			}
		}

		if record.Success {
			stats.SuccessfulCycles++
		} else {
			stats.FailedCycles++
		}
	}

	return stats, nil
}

// Statistics 统计信息
type Statistics struct {
	TotalCycles         int `json:"total_cycles"`
	SuccessfulCycles    int `json:"successful_cycles"`
	FailedCycles        int `json:"failed_cycles"`
	TotalOpenPositions  int `json:"total_open_positions"`
	TotalClosePositions int `json:"total_close_positions"`
}

// TradeOutcome 单笔交易结果
type TradeOutcome struct {
	Symbol         string    `json:"symbol"`           // 币种
	Side           string    `json:"side"`             // long/short
	Quantity       float64   `json:"quantity"`         // 仓位数量（实际平仓数量）
	Leverage       int       `json:"leverage"`         // 杠杆倍数
	OpenPrice      float64   `json:"open_price"`       // 开仓价
	ClosePrice     float64   `json:"close_price"`      // 平仓价（加权平均）
	PositionValue  float64   `json:"position_value"`   // 仓位价值（quantity × openPrice）
	MarginUsed     float64   `json:"margin_used"`      // 保证金使用（positionValue / leverage）
	PnL            float64   `json:"pn_l"`             // 盈亏（USDT，累计）
	PnLPct         float64   `json:"pn_l_pct"`         // 盈亏百分比（相对保证金）
	Duration       string    `json:"duration"`         // 持仓时长（从开仓到最后一次平仓）
	OpenTime       time.Time `json:"open_time"`        // 开仓时间
	CloseTime      time.Time `json:"close_time"`       // 平仓时间（最后一次平仓时间）
	WasStopLoss    bool      `json:"was_stop_loss"`    // 是否止损
	IsPartialClose bool      `json:"is_partial_close"` // 是否部分平仓（true=部分平仓，false=全部平仓）
	OpenQuantity   float64   `json:"open_quantity"`    // 原始开仓数量（用于判断是否全部平仓）
	CloseNote      string    `json:"close_note"`
	Commission     float64   `json:"commission"` // 本次平仓手续费（USDT）
}

// PerformanceAnalysis 交易表现分析
type PerformanceAnalysis struct {
	TotalTrades   int                           `json:"total_trades"`   // 总交易数
	WinningTrades int                           `json:"winning_trades"` // 盈利交易数
	LosingTrades  int                           `json:"losing_trades"`  // 亏损交易数
	WinRate       float64                       `json:"win_rate"`       // 胜率
	AvgWin        float64                       `json:"avg_win"`        // 平均盈利
	AvgLoss       float64                       `json:"avg_loss"`       // 平均亏损
	ProfitFactor  float64                       `json:"profit_factor"`  // 盈亏比
	PeriodStart   time.Time                     `json:"period_start"`   // 本周期起始时间（双验证）
	PeriodSource  string                        `json:"period_source"`  // 起始时间来源：decision_log/runtime_start
	SharpeRatio   float64                       `json:"sharpe_ratio"`   // 夏普比率（风险调整后收益）
	RecentTrades  []TradeOutcome                `json:"recent_trades"`  // 最近N笔交易
	SymbolStats   map[string]*SymbolPerformance `json:"symbol_stats"`   // 各币种表现
	BestSymbol    string                        `json:"best_symbol"`    // 表现最好的币种
	WorstSymbol   string                        `json:"worst_symbol"`   // 表现最差的币种
}

// SymbolPerformance 币种表现统计
type SymbolPerformance struct {
	Symbol        string  `json:"symbol"`         // 币种
	TotalTrades   int     `json:"total_trades"`   // 交易次数
	WinningTrades int     `json:"winning_trades"` // 盈利次数
	LosingTrades  int     `json:"losing_trades"`  // 亏损次数
	WinRate       float64 `json:"win_rate"`       // 胜率
	TotalPnL      float64 `json:"total_pn_l"`     // 总盈亏
	AvgPnL        float64 `json:"avg_pn_l"`       // 平均盈亏
}

// AnalyzePerformance 分析最近N个周期的交易表现
func (l *DecisionLogger) AnalyzePerformance(lookbackCycles int) (*PerformanceAnalysis, error) {
	records, err := l.GetLatestRecords(lookbackCycles)
	if err != nil {
		return nil, fmt.Errorf("读取历史记录失败: %w", err)
	}

	if len(records) == 0 {
		return &PerformanceAnalysis{
			RecentTrades: []TradeOutcome{},
			SymbolStats:  make(map[string]*SymbolPerformance),
		}, nil
	}

	analysis := &PerformanceAnalysis{
		RecentTrades: []TradeOutcome{},
		SymbolStats:  make(map[string]*SymbolPerformance),
	}

	// 追踪持仓状态：symbol_side -> []{side, openPrice, openTime, quantity, leverage, remainingQuantity}
	// 使用队列（FIFO）支持同一币种多次开仓，不覆盖
	// 支持部分平仓：remainingQuantity 记录剩余未平数量
	type OpenPosition struct {
		side              string
		openPrice         float64
		openTime          time.Time
		quantity          float64 // 原始开仓数量
		remainingQuantity float64 // 剩余未平数量（支持部分平仓）
		leverage          int
		positionID        string // 仓位ID
	}
	openPositions := make(map[string][]OpenPosition)
	positionMap := make(map[string]*OpenPosition) // positionID -> position（用于快速查找）

	// 为了避免开仓记录在窗口外导致匹配失败，需要先从所有历史记录中找出未平仓的持仓
	// 获取更多历史记录来构建完整的持仓状态（使用更大的窗口）
	allRecords, err := l.GetLatestRecords(lookbackCycles * 3) // 扩大3倍窗口
	if err == nil && len(allRecords) > len(records) {
		// 先从扩大的窗口中收集所有开仓记录
		for _, record := range allRecords {
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
				posKey := symbol + "_" + side

				switch action.Action {
				case "open_long", "open_short":
					// 添加到队列（FIFO），不覆盖
					openPositions[posKey] = append(openPositions[posKey], OpenPosition{
						side:              side,
						openPrice:         action.Price,
						openTime:          action.Timestamp,
						quantity:          action.Quantity,
						remainingQuantity: action.Quantity, // 初始剩余数量等于开仓数量
						leverage:          action.Leverage,
					})
				case "close_long", "close_short":
					// 部分平仓支持：从最早的开仓记录开始平仓，如果平仓数量小于开仓数量，则只减少剩余数量
					closeQuantity := action.Quantity
					if closeQuantity == 0 {
						// 如果平仓数量为0，通常表示全部平仓，从第一个开仓记录开始全部平掉
						if len(openPositions[posKey]) > 0 {
							openPositions[posKey] = openPositions[posKey][1:]
						}
					} else {
						// 有具体平仓数量，按FIFO匹配
						for len(openPositions[posKey]) > 0 && closeQuantity > 0 {
							openPos := &openPositions[posKey][0]
							if openPos.remainingQuantity <= closeQuantity {
								// 当前开仓记录的剩余数量 <= 平仓数量，全部平掉
								closeQuantity -= openPos.remainingQuantity
								openPositions[posKey] = openPositions[posKey][1:]
							} else {
								// 当前开仓记录的剩余数量 > 平仓数量，只减少剩余数量
								openPos.remainingQuantity -= closeQuantity
								closeQuantity = 0
							}
						}
					}
				}
			}
		}
	}

	// 遍历分析窗口内的记录，生成交易结果
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
			posKey := symbol + "_" + side // 使用symbol_side作为key，区分多空持仓

			switch action.Action {
			case "open_long", "open_short":
				// 创建开仓记录
				openPos := OpenPosition{
					side:              side,
					openPrice:         action.Price,
					openTime:          action.Timestamp,
					quantity:          action.Quantity,
					remainingQuantity: action.Quantity, // 初始剩余数量等于开仓数量
					leverage:          action.Leverage,
					positionID:        action.PositionID,
				}
				// 添加到队列（FIFO）
				openPositions[posKey] = append(openPositions[posKey], openPos)
				// 如果有positionID，加入快速查找map
				if action.PositionID != "" {
					positionMap[action.PositionID] = &openPositions[posKey][len(openPositions[posKey])-1]
				}

			case "close_long", "close_short":
				// 优先通过positionID查找
				var matchedPositions []*OpenPosition

				if action.PositionID != "" && positionMap[action.PositionID] != nil {
					// 使用positionID精确匹配
					matchedPositions = append(matchedPositions, positionMap[action.PositionID])
				} else {
					// 没有positionID，使用FIFO匹配
					for i := range openPositions[posKey] {
						if openPositions[posKey][i].remainingQuantity > 0 {
							matchedPositions = append(matchedPositions, &openPositions[posKey][i])
						}
					}
				}

				// 查找对应的开仓记录（支持部分平仓）
				closeQuantity := action.Quantity
				if closeQuantity == 0 && len(matchedPositions) > 0 {
					// 如果平仓数量为0，通常表示全部平仓
					closeQuantity = matchedPositions[0].remainingQuantity
				}

				if closeQuantity <= 0 && len(matchedPositions) > 0 {
					for _, pos := range matchedPositions {
						closeQuantity += pos.remainingQuantity
					}
				}

				realizedPerUnit := 0.0
				commissionPerUnit := 0.0
				if closeQuantity > 0 {
					if math.Abs(action.RealizedPnL) > 0 {
						realizedPerUnit = action.RealizedPnL / closeQuantity
					}
					if math.Abs(action.Commission) > 0 {
						commissionPerUnit = action.Commission / closeQuantity
					}
				}

				// 匹配平仓
				remainingCloseQty := closeQuantity
				for _, openPos := range matchedPositions {
					if remainingCloseQty <= 0 {
						break
					}

					// 本次平仓数量 = min(剩余平仓数量, 当前开仓记录的剩余数量)
					actualCloseQty := remainingCloseQty
					if openPos.remainingQuantity < actualCloseQty {
						actualCloseQty = openPos.remainingQuantity
					}

					openPrice := openPos.openPrice
					openTime := openPos.openTime
					side := openPos.side
					leverage := openPos.leverage

					// 计算实际盈亏（USDT）
					var pnl float64
					if realizedPerUnit != 0 {
						pnl = actualCloseQty * realizedPerUnit
					} else {
						if side == "long" {
							pnl = actualCloseQty * (action.Price - openPrice)
						} else {
							pnl = actualCloseQty * (openPrice - action.Price)
						}
					}

					// 计算盈亏百分比（相对保证金）
					positionValue := actualCloseQty * openPrice
					marginUsed := 0.0
					pnlPct := 0.0

					// 防止杠杆为0导致除零错误
					if leverage > 0 {
						marginUsed = positionValue / float64(leverage)
					}
					if marginUsed > 0 {
						pnlPct = (pnl / marginUsed) * 100
					}

					// 判断是否部分平仓
					isPartialClose := false
					closeNote := ""
					originalOpenQty := openPos.quantity // 原始开仓数量

					// 计算这次平仓后的剩余数量
					remainingAfterClose := openPos.remainingQuantity - actualCloseQty

					if remainingAfterClose > 0.00000001 { // 浮点数精度处理
						// 平仓后还有剩余，是部分平仓
						isPartialClose = true
						// 计算部分平仓的百分比
						closePercent := (actualCloseQty / originalOpenQty) * 100
						closeNote = fmt.Sprintf("[部分平仓:%.2f%%]", closePercent)
					} else {
						// 已经全部平仓
						isPartialClose = false
						if actualCloseQty < originalOpenQty {
							// 这是多次平仓的最后一笔
							closeNote = "分批成交"
						} else {
							// 一次性全部平仓
							closeNote = ""
						}
					}

					commission := actualCloseQty * commissionPerUnit
					if commission != 0 {
						pnl -= commission
					}

					// 记录交易结果
					outcome := TradeOutcome{
						Symbol:         symbol,
						Side:           side,
						Quantity:       actualCloseQty,  // 实际平仓数量
						OpenQuantity:   originalOpenQty, // 原始开仓数量
						Leverage:       leverage,
						OpenPrice:      openPrice,
						ClosePrice:     action.Price,
						PositionValue:  positionValue,
						MarginUsed:     marginUsed,
						PnL:            pnl,
						PnLPct:         pnlPct,
						Duration:       action.Timestamp.Sub(openTime).String(),
						OpenTime:       openTime,
						CloseTime:      action.Timestamp,
						IsPartialClose: isPartialClose,
						CloseNote:      closeNote,
						Commission:     commission,
					}

					analysis.RecentTrades = append(analysis.RecentTrades, outcome)
					analysis.TotalTrades++

					// 分类交易：盈利、亏损、持平（避免将pnl=0算入亏损）
					if pnl > 0 {
						analysis.WinningTrades++
						analysis.AvgWin += pnl
					} else if pnl < 0 {
						analysis.LosingTrades++
						analysis.AvgLoss += pnl
					}
					// pnl == 0 的交易不计入盈利也不计入亏损，但计入总交易数

					// 更新币种统计
					if _, exists := analysis.SymbolStats[symbol]; !exists {
						analysis.SymbolStats[symbol] = &SymbolPerformance{
							Symbol: symbol,
						}
					}
					stats := analysis.SymbolStats[symbol]
					stats.TotalTrades++
					stats.TotalPnL += pnl
					if pnl > 0 {
						stats.WinningTrades++
					} else if pnl < 0 {
						stats.LosingTrades++
					}

					// 更新剩余数量或移除已全部平仓的记录
					openPos.remainingQuantity -= actualCloseQty
					remainingCloseQty -= actualCloseQty

					// 如果当前开仓记录已全部平仓，标记移除
					if openPos.remainingQuantity <= 0.00000001 { // 浮点数精度处理
						// 从positionMap中移除
						if openPos.positionID != "" {
							delete(positionMap, openPos.positionID)
						}
					}
				}
			}
		}
	}

	// 清理已完全平仓的记录
	for key, positions := range openPositions {
		var remaining []OpenPosition
		for _, pos := range positions {
			if pos.remainingQuantity > 0.00000001 {
				remaining = append(remaining, pos)
			}
		}
		openPositions[key] = remaining
	}

	// 计算统计指标
	if analysis.TotalTrades > 0 {
		analysis.WinRate = (float64(analysis.WinningTrades) / float64(analysis.TotalTrades)) * 100

		// 计算总盈利和总亏损
		totalWinAmount := analysis.AvgWin   // 当前是累加的总和
		totalLossAmount := analysis.AvgLoss // 当前是累加的总和（负数）

		if analysis.WinningTrades > 0 {
			analysis.AvgWin /= float64(analysis.WinningTrades)
		}
		if analysis.LosingTrades > 0 {
			analysis.AvgLoss /= float64(analysis.LosingTrades)
		}

		// Profit Factor = 总盈利 / 总亏损（绝对值）
		// 注意：totalLossAmount 是负数，所以取负号得到绝对值
		if totalLossAmount != 0 {
			analysis.ProfitFactor = totalWinAmount / (-totalLossAmount)
		} else if totalWinAmount > 0 {
			// 只有盈利没有亏损的情况，设置为一个很大的值表示完美策略
			analysis.ProfitFactor = 999.0
		}
	}

	// 计算各币种胜率和平均盈亏
	bestPnL := -999999.0
	worstPnL := 999999.0
	for symbol, stats := range analysis.SymbolStats {
		if stats.TotalTrades > 0 {
			stats.WinRate = (float64(stats.WinningTrades) / float64(stats.TotalTrades)) * 100
			stats.AvgPnL = stats.TotalPnL / float64(stats.TotalTrades)

			if stats.TotalPnL > bestPnL {
				bestPnL = stats.TotalPnL
				analysis.BestSymbol = symbol
			}
			if stats.TotalPnL < worstPnL {
				worstPnL = stats.TotalPnL
				analysis.WorstSymbol = symbol
			}
		}
	}

	// 只保留最近的交易（倒序：最新的在前）
	// 增加到100笔以匹配前端显示"最近100笔成交"
	if len(analysis.RecentTrades) > 100 {
		// 反转数组，让最新的在前
		for i, j := 0, len(analysis.RecentTrades)-1; i < j; i, j = i+1, j-1 {
			analysis.RecentTrades[i], analysis.RecentTrades[j] = analysis.RecentTrades[j], analysis.RecentTrades[i]
		}
		analysis.RecentTrades = analysis.RecentTrades[:100]
	} else if len(analysis.RecentTrades) > 0 {
		// 反转数组
		for i, j := 0, len(analysis.RecentTrades)-1; i < j; i, j = i+1, j-1 {
			analysis.RecentTrades[i], analysis.RecentTrades[j] = analysis.RecentTrades[j], analysis.RecentTrades[i]
		}
	}

	// 计算夏普比率（需要至少2个数据点）
	analysis.SharpeRatio = l.calculateSharpeRatio(records)

	return analysis, nil
}

// calculateSharpeRatio 计算夏普比率
// 基于账户净值的变化计算风险调整后收益
func (l *DecisionLogger) calculateSharpeRatio(records []*DecisionRecord) float64 {
	if len(records) < 2 {
		return 0.0
	}

	// 提取每个周期的账户净值
	// 注意：TotalBalance字段实际存储的是TotalEquity（账户总净值）
	// TotalUnrealizedProfit字段实际存储的是TotalPnL（相对初始余额的盈亏）
	var equities []float64
	for _, record := range records {
		// 直接使用TotalBalance，因为它已经是完整的账户净值
		equity := record.AccountState.TotalBalance
		if equity > 0 {
			equities = append(equities, equity)
		}
	}

	if len(equities) < 2 {
		return 0.0
	}

	// 计算周期收益率（period returns）
	var returns []float64
	for i := 1; i < len(equities); i++ {
		if equities[i-1] > 0 {
			periodReturn := (equities[i] - equities[i-1]) / equities[i-1]
			returns = append(returns, periodReturn)
		}
	}

	if len(returns) == 0 {
		return 0.0
	}

	// 计算平均收益率
	sumReturns := 0.0
	for _, r := range returns {
		sumReturns += r
	}
	meanReturn := sumReturns / float64(len(returns))

	// 计算收益率标准差
	sumSquaredDiff := 0.0
	for _, r := range returns {
		diff := r - meanReturn
		sumSquaredDiff += diff * diff
	}
	variance := sumSquaredDiff / float64(len(returns))
	stdDev := math.Sqrt(variance)

	// 避免除以零
	if stdDev == 0 {
		if meanReturn > 0 {
			return 999.0 // 无波动的正收益
		} else if meanReturn < 0 {
			return -999.0 // 无波动的负收益
		}
		return 0.0
	}

	// 计算夏普比率（假设无风险利率为0）
	// 注：直接返回周期级别的夏普比率（非年化），正常范围 -2 到 +2
	sharpeRatio := meanReturn / stdDev
	return sharpeRatio
}
