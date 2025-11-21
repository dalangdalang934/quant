package trader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/adshao/go-binance/v2/futures"
	"github.com/gorilla/websocket"
)

const (
	tradeQuantityTolerance = 1e-6
	defaultTradeFillWindow = 2 * time.Minute
)

// FuturesTrader 币安合约交易器
type FuturesTrader struct {
	client *futures.Client

	// 余额缓存
	cachedBalance     map[string]interface{}
	balanceCacheTime  time.Time
	balanceCacheMutex sync.RWMutex

	// 持仓缓存
	cachedPositions     []map[string]interface{}
	positionsCacheTime  time.Time
	positionsCacheMutex sync.RWMutex

	// 缓存有效期（15秒）
	cacheDuration     time.Duration
	defaultMarginType futures.MarginType

	// 用户数据流
	userStream struct {
		enabled        bool
		reconnectDelay time.Duration
		pingInterval   time.Duration
		listenRefresh  time.Duration

		mu         sync.RWMutex
		listenKey  string
		conn       *websocket.Conn
		cancel     context.CancelFunc
		refreshing bool
		lastSync   time.Time
	}

	tradeCache struct {
		mu       sync.RWMutex
		fills    []userTradeFill
		maxAge   time.Duration
		maxItems int
	}
}

type userTradeFill struct {
	symbol      string
	side        string
	positionSide string
	orderID     int64
	execType    string
	price       float64
	quantity    float64
	realizedPnL float64
	commission  float64
	feeAsset    string
	time        time.Time
}

// TradeFill 对外暴露的成交信息（用于仓位追踪器）
type TradeFill struct {
	Symbol      string
	Side        string
	PositionSide string
	Price       float64
	Quantity    float64
	RealizedPnL float64
	Commission  float64
	FeeAsset    string
	Time        time.Time
}

// NewFuturesTrader 创建合约交易器
func NewFuturesTrader(apiKey, secretKey string) *FuturesTrader {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DialContext:         (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
			TLSHandshakeTimeout: 15 * time.Second,
			IdleConnTimeout:     120 * time.Second,
			MaxIdleConnsPerHost: 10,
		},
	}

	client := futures.NewClient(apiKey, secretKey)
	client.HTTPClient = httpClient

	return &FuturesTrader{
		client:            client,
		cacheDuration:     15 * time.Second, // 15秒缓存
		defaultMarginType: futures.MarginTypeIsolated,
		tradeCache: struct {
			mu       sync.RWMutex
			fills    []userTradeFill
			maxAge   time.Duration
			maxItems int
		}{
			fills:    make([]userTradeFill, 0, 128),
			maxAge:   defaultTradeFillWindow,
			maxItems: 256,
		},
	}
}

// ConfigureBinanceUserStream 配置币安用户数据流参数
func (t *FuturesTrader) ConfigureBinanceUserStream(enabled bool, reconnectDelay, pingInterval, refreshInterval time.Duration) {
	if !enabled {
		return
	}

	if reconnectDelay <= 0 {
		reconnectDelay = 5 * time.Second
	}
	if pingInterval <= 0 {
		pingInterval = 30 * time.Second
	}
	if refreshInterval <= 0 {
		refreshInterval = 25 * time.Minute
	}

	t.userStream.mu.Lock()
	t.userStream.enabled = true
	t.userStream.reconnectDelay = reconnectDelay
	t.userStream.pingInterval = pingInterval
	t.userStream.listenRefresh = refreshInterval
	t.userStream.mu.Unlock()
}

// SetDefaultMarginMode 设置默认保证金模式（每次开仓前使用）
func (t *FuturesTrader) SetDefaultMarginMode(mode string) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "cross", "crossed", "cross_margin", "crossed_margin":
		t.defaultMarginType = futures.MarginTypeCrossed
		log.Printf("⚙️  默认保证金模式已设置为 CROSS")
	default:
		t.defaultMarginType = futures.MarginTypeIsolated
		log.Printf("⚙️  默认保证金模式已设置为 ISOLATED")
	}
}

// GetBalance 获取账户余额（带缓存）
func (t *FuturesTrader) GetBalance() (map[string]interface{}, error) {
	// 先检查缓存是否有效
	t.balanceCacheMutex.RLock()
	if t.cachedBalance != nil && time.Since(t.balanceCacheTime) < t.cacheDuration {
		cacheAge := time.Since(t.balanceCacheTime)
		t.balanceCacheMutex.RUnlock()
		log.Printf("✓ 使用缓存的账户余额（缓存时间: %.1f秒前）", cacheAge.Seconds())
		return t.cachedBalance, nil
	}
	t.balanceCacheMutex.RUnlock()

	log.Printf("🔄 缓存过期，正在调用币安API获取账户余额...")
	balance, err := t.fetchBalanceFromAPI()
	if err != nil {
		return nil, err
	}

	t.balanceCacheMutex.Lock()
	t.cachedBalance = balance
	t.balanceCacheTime = time.Now()
	t.balanceCacheMutex.Unlock()

	return balance, nil
}

func (t *FuturesTrader) fetchBalanceFromAPI() (map[string]interface{}, error) {
	var account *futures.Account
	err := withRetries("获取账户信息", 3, func() error {
		svc := t.client.NewGetAccountService()
		acct, err := svc.Do(context.Background())
		if err != nil {
			return err
		}
		account = acct
		return nil
	})
	if err != nil {
		log.Printf("❌ 币安API调用失败: %v", err)
		return nil, fmt.Errorf("获取账户信息失败: %w", err)
	}

	result := make(map[string]interface{})
	result["totalWalletBalance"], _ = strconv.ParseFloat(account.TotalWalletBalance, 64)
	result["availableBalance"], _ = strconv.ParseFloat(account.AvailableBalance, 64)
	result["totalUnrealizedProfit"], _ = strconv.ParseFloat(account.TotalUnrealizedProfit, 64)

	log.Printf("✓ 币安API返回: 总余额=%s, 可用=%s, 未实现盈亏=%s",
		account.TotalWalletBalance,
		account.AvailableBalance,
		account.TotalUnrealizedProfit)

	return result, nil
}

// StartBinanceUserStream 启动币安用户数据流监听（带自动重连和心跳）
func (t *FuturesTrader) StartBinanceUserStream() {
	t.userStream.mu.Lock()
	if !t.userStream.enabled {
		t.userStream.mu.Unlock()
		return
	}
	if t.userStream.cancel != nil {
		t.userStream.mu.Unlock()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.userStream.cancel = cancel
	t.userStream.mu.Unlock()

	go t.runUserStream(ctx)
}

// StopBinanceUserStream 停止币安用户数据流
func (t *FuturesTrader) StopBinanceUserStream() {
	t.userStream.mu.Lock()
	cancel := t.userStream.cancel
	t.userStream.cancel = nil
	t.userStream.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (t *FuturesTrader) runUserStream(ctx context.Context) {
	for {
		if err := t.connectUserStream(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("⚠️  [UserStream] 连接异常: %v", err)
		}

		// 检查是否需要继续重连
		t.userStream.mu.RLock()
		enabled := t.userStream.enabled
		retryDelay := t.userStream.reconnectDelay
		t.userStream.mu.RUnlock()

		if !enabled || ctx.Err() != nil {
			return
		}

		if retryDelay <= 0 {
			retryDelay = 5 * time.Second
		}

		log.Printf("⏳ [UserStream] %v 后重试连接", retryDelay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(retryDelay):
		}
	}
}

func (t *FuturesTrader) connectUserStream(ctx context.Context) error {
	listenKey, err := t.createListenKey()
	if err != nil {
		return err
	}

	wsURL := fmt.Sprintf("wss://fstream.binance.com/ws/%s", listenKey)
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	dialer.EnableCompression = true
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("连接用户数据流失败: %w", err)
	}

	t.userStream.mu.Lock()
	t.userStream.listenKey = listenKey
	t.userStream.conn = conn
	t.userStream.mu.Unlock()

	log.Printf("✅ [UserStream] 连接成功，listenKey=%s", listenKey)

	defer func() {
		conn.Close()
		t.userStream.mu.Lock()
		if t.userStream.conn == conn {
			t.userStream.conn = nil
		}
		if t.userStream.listenKey == listenKey {
			t.userStream.listenKey = ""
		}
		t.userStream.mu.Unlock()
	}()

	t.userStream.mu.RLock()
	pingInterval := t.userStream.pingInterval
	refreshInterval := t.userStream.listenRefresh
	t.userStream.mu.RUnlock()

	if pingInterval <= 0 {
		pingInterval = 30 * time.Second
	}
	if refreshInterval <= 0 {
		refreshInterval = 25 * time.Minute
	}

	pingTicker := time.NewTicker(pingInterval)
	refreshTicker := time.NewTicker(refreshInterval)
	defer pingTicker.Stop()
	defer refreshTicker.Stop()

	errCh := make(chan error, 2)

	go t.userStreamPinger(ctx, conn, pingTicker, errCh)
	go t.userStreamRefresher(ctx, refreshTicker, errCh)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		default:
		}

		if err := conn.SetReadDeadline(time.Now().Add(90 * time.Second)); err != nil {
			return err
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		t.handleUserStreamMessage(message)
	}
}

func (t *FuturesTrader) userStreamPinger(ctx context.Context, conn *websocket.Conn, ticker *time.Ticker, errCh chan<- error) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deadline := time.Now().Add(10 * time.Second)
			if err := conn.WriteControl(websocket.PingMessage, []byte("ping"), deadline); err != nil {
				errCh <- fmt.Errorf("发送Ping失败: %w", err)
				return
			}
			if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err == nil {
				if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
					errCh <- fmt.Errorf("Ping心跳失败: %w", err)
					return
				}
			}
		}
	}
}

func (t *FuturesTrader) userStreamRefresher(ctx context.Context, ticker *time.Ticker, errCh chan<- error) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.userStream.mu.RLock()
			listenKey := t.userStream.listenKey
			t.userStream.mu.RUnlock()

			if listenKey == "" {
				continue
			}

			if err := t.keepAliveListenKey(listenKey); err != nil {
				errCh <- fmt.Errorf("刷新listenKey失败: %w", err)
				return
			}
		}
	}
}

func (t *FuturesTrader) createListenKey() (string, error) {
	resp, err := t.client.NewStartUserStreamService().Do(context.Background())
	if err != nil {
		return "", fmt.Errorf("申请listenKey失败: %w", err)
	}
	if resp == "" {
		return "", errors.New("listenKey为空")
	}
	return resp, nil
}

func (t *FuturesTrader) keepAliveListenKey(listenKey string) error {
	return t.client.NewKeepaliveUserStreamService().ListenKey(listenKey).Do(context.Background())
}

func withRetries(name string, attempts int, fn func() error) error {
	var err error
	if attempts <= 0 {
		attempts = 1
	}
	for i := 0; i < attempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		delay := time.Duration(i+1) * 300 * time.Millisecond
		log.Printf("⚠️  %s尝试失败(%d/%d): %v，%v后重试", name, i+1, attempts, err, delay)
		time.Sleep(delay)
	}
	return err
}

func (t *FuturesTrader) handleUserStreamMessage(payload []byte) {
	var base struct {
		Event string `json:"e"`
	}
	if err := json.Unmarshal(payload, &base); err != nil {
		log.Printf("⚠️  [UserStream] 解析事件失败: %v", err)
		return
	}

	switch base.Event {
	case "ACCOUNT_UPDATE":
		t.handleAccountUpdate(payload)
	case "ORDER_TRADE_UPDATE":
		t.handleOrderTradeUpdate(payload)
	default:
		// 其他事件暂不处理
	}
}

func (t *FuturesTrader) handleAccountUpdate(payload []byte) {
	var evt struct {
		Balances []struct {
			Asset     string `json:"a"`
			Balance   string `json:"wb"`
			Available string `json:"cw"`
		} `json:"B"`
		Positions []struct {
			Symbol           string `json:"s"`
			UnrealizedProfit string `json:"up"`
		} `json:"P"`
	}

	if err := json.Unmarshal(payload, &evt); err != nil {
		log.Printf("⚠️  [UserStream] 解析ACCOUNT_UPDATE失败: %v", err)
		return
	}

	var totalWallet, available, unrealized float64
	for _, bal := range evt.Balances {
		if strings.ToUpper(bal.Asset) != "USDT" {
			continue
		}
		totalWallet, _ = strconv.ParseFloat(bal.Balance, 64)
		available, _ = strconv.ParseFloat(bal.Available, 64)
	}
	for _, pos := range evt.Positions {
		val, _ := strconv.ParseFloat(pos.UnrealizedProfit, 64)
		unrealized += val
	}

	balance := map[string]interface{}{
		"totalWalletBalance":      totalWallet,
		"availableBalance":        available,
		"totalUnrealizedProfit":   unrealized,
	}

	t.balanceCacheMutex.Lock()
	t.cachedBalance = balance
	t.balanceCacheTime = time.Now()
	t.balanceCacheMutex.Unlock()

	// 更新持仓缓存以获取最新逐仓/杠杆信息
	t.refreshPositionsFromAPI()
}

func (t *FuturesTrader) handleOrderTradeUpdate(payload []byte) {
	var evt struct {
		EventTime int64 `json:"E"`
		Order    struct {
			Symbol          string `json:"s"`
			Side            string `json:"S"`
			PositionSide    string `json:"ps"`
			OrderID         int64  `json:"i"`
			ExecType        string `json:"x"`
			OrderStatus     string `json:"X"`
			LastFilledQty   string `json:"l"`
			LastFilledPrice string `json:"L"`
			AveragePrice    string `json:"ap"`
			RealizedPnL     string `json:"rp"`
			Commission      string `json:"n"`
			CommissionAsset string `json:"N"`
			TradeID         int64  `json:"t"`
			TradeTime       int64  `json:"T"`
		} `json:"o"`
	}

	if err := json.Unmarshal(payload, &evt); err != nil {
		log.Printf("⚠️  [UserStream] 解析ORDER_TRADE_UPDATE失败: %v", err)
		return
	}

	ord := evt.Order
	if !strings.EqualFold(ord.ExecType, "trade") {
		return
	}

	qty, err := strconv.ParseFloat(ord.LastFilledQty, 64)
	if err != nil {
		log.Printf("⚠️  [UserStream] 解析成交数量失败: %v", err)
		return
	}
	qty = math.Abs(qty)
	if qty <= 0 {
		return
	}

	price, _ := strconv.ParseFloat(ord.LastFilledPrice, 64)
	if price <= 0 {
		price, _ = strconv.ParseFloat(ord.AveragePrice, 64)
	}
	realizedPnL, _ := strconv.ParseFloat(ord.RealizedPnL, 64)
	commission, _ := strconv.ParseFloat(ord.Commission, 64)
	commission = math.Abs(commission)

	tradeTime := ord.TradeTime
	if tradeTime == 0 {
		tradeTime = evt.EventTime
	}
	fillTime := time.UnixMilli(tradeTime)
	if fillTime.IsZero() {
		fillTime = time.Now()
	}

	positionSide := strings.ToLower(ord.PositionSide)
	if positionSide == "" || positionSide == "both" {
		positionSide = strings.ToLower(ord.Side)
	}

	fill := userTradeFill{
		symbol:       strings.ToUpper(ord.Symbol),
		side:         strings.ToLower(ord.Side),
		positionSide: positionSide,
		orderID:      ord.OrderID,
		execType:     strings.ToUpper(ord.ExecType),
		price:        price,
		quantity:     qty,
		realizedPnL:  realizedPnL,
		commission:   commission,
		feeAsset:     strings.ToUpper(ord.CommissionAsset),
		time:         fillTime,
	}

	t.addTradeFill(fill)
	log.Printf("📝 [UserStream] 成交: %s %s qty=%.6f price=%.6f pnl=%.6f fee=%.6f %s",
		fill.symbol, fill.positionSide, fill.quantity, fill.price, fill.realizedPnL, fill.commission, fill.feeAsset)

	status := strings.ToLower(ord.OrderStatus)
	if status == "filled" || status == "partially_filled" {
		go t.refreshPositionsFromAPI()
	}
}

func (t *FuturesTrader) refreshPositionsFromAPI() {
	positions, err := t.fetchPositionsFromAPI(true)
	if err != nil {
		log.Printf("⚠️  [UserStream] 刷新持仓失败: %v", err)
		return
	}

	t.positionsCacheMutex.Lock()
	t.cachedPositions = positions
	t.positionsCacheTime = time.Now()
	t.positionsCacheMutex.Unlock()

	t.userStream.mu.Lock()
	t.userStream.lastSync = time.Now()
	t.userStream.mu.Unlock()
}

func (t *FuturesTrader) refreshStateFromAPI() {
	if balance, err := t.fetchBalanceFromAPI(); err == nil {
		t.balanceCacheMutex.Lock()
		t.cachedBalance = balance
		t.balanceCacheTime = time.Now()
		t.balanceCacheMutex.Unlock()
	} else {
		log.Printf("⚠️  [UserStream] 刷新账户余额失败: %v", err)
	}

	if positions, err := t.fetchPositionsFromAPI(true); err == nil {
		t.positionsCacheMutex.Lock()
		t.cachedPositions = positions
		t.positionsCacheTime = time.Now()
		t.positionsCacheMutex.Unlock()
	} else {
		log.Printf("⚠️  [UserStream] 刷新持仓失败: %v", err)
	}

	t.userStream.mu.Lock()
	t.userStream.lastSync = time.Now()
	t.userStream.mu.Unlock()
}

// GetPositions 获取所有持仓（带缓存）
func (t *FuturesTrader) GetPositions() ([]map[string]interface{}, error) {
	// 先检查缓存是否有效
	t.positionsCacheMutex.RLock()
	if t.cachedPositions != nil && time.Since(t.positionsCacheTime) < t.cacheDuration {
		cacheAge := time.Since(t.positionsCacheTime)
		t.positionsCacheMutex.RUnlock()
		log.Printf("✓ 使用缓存的持仓信息（缓存时间: %.1f秒前，持仓数: %d）", cacheAge.Seconds(), len(t.cachedPositions))
		return t.cachedPositions, nil
	}
	t.positionsCacheMutex.RUnlock()

	// 缓存过期或不存在，调用API
	positions, err := t.fetchPositionsFromAPI(true)
	if err != nil {
		return nil, err
	}

	t.positionsCacheMutex.Lock()
	t.cachedPositions = positions
	t.positionsCacheTime = time.Now()
	t.positionsCacheMutex.Unlock()

	return positions, nil
}

func (t *FuturesTrader) fetchPositionsFromAPI(filterZero bool) ([]map[string]interface{}, error) {
	var positions []*futures.PositionRisk
	err := withRetries("获取持仓", 3, func() error {
		svc := t.client.NewGetPositionRiskService()
		pos, err := svc.Do(context.Background())
		if err != nil {
			return err
		}
		positions = pos
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}

	log.Printf("📊 币安API返回 %d 条持仓记录", len(positions))

	var result []map[string]interface{}
	skipped := 0
	for _, pos := range positions {
		posAmt, parseErr := strconv.ParseFloat(pos.PositionAmt, 64)
		
		if filterZero && posAmt == 0 {
			skipped++
			continue // 跳过无持仓的
		}
		
		// 只记录有效持仓
		if parseErr != nil {
			log.Printf("  ⚠️  持仓数据解析错误: %s, PositionAmt='%s', err=%v", pos.Symbol, pos.PositionAmt, parseErr)
		}
		log.Printf("  ✅ 有效持仓: %s, 数量=%.4f, 开仓价=%s, 标记价=%s, 未实现盈亏=%s", 
			pos.Symbol, posAmt, pos.EntryPrice, pos.MarkPrice, pos.UnRealizedProfit)

		posMap := make(map[string]interface{})
		posMap["symbol"] = pos.Symbol
		posMap["positionAmt"], _ = strconv.ParseFloat(pos.PositionAmt, 64)
		posMap["entryPrice"], _ = strconv.ParseFloat(pos.EntryPrice, 64)
		posMap["markPrice"], _ = strconv.ParseFloat(pos.MarkPrice, 64)
		posMap["unRealizedProfit"], _ = strconv.ParseFloat(pos.UnRealizedProfit, 64)
		posMap["leverage"], _ = strconv.ParseFloat(pos.Leverage, 64)
		posMap["liquidationPrice"], _ = strconv.ParseFloat(pos.LiquidationPrice, 64)

		// 解析保证金模式（确保前端能区分逐仓/全仓）
		marginType := "isolated"
		if mt := strings.ToLower(strings.TrimSpace(pos.MarginType)); mt != "" {
			switch mt {
			case "cross", "crossed", "cross_margin", "crossed_margin":
				marginType = "cross"
			default:
				marginType = "isolated"
			}
		}
		posMap["marginType"] = marginType

		if isoMargin, err := strconv.ParseFloat(pos.IsolatedMargin, 64); err == nil {
			posMap["isolatedMargin"] = isoMargin
		}

		// 判断方向
		if posAmt > 0 {
			posMap["side"] = "long"
		} else {
			posMap["side"] = "short"
		}

		result = append(result, posMap)
	}

	// 更新缓存
	log.Printf("✅ 币安API过滤后返回 %d 个有效持仓（跳过 %d 个零持仓）", len(result), skipped)
	return result, nil
}

func (t *FuturesTrader) addTradeFill(fill userTradeFill) {
	if fill.time.IsZero() {
		fill.time = time.Now()
	}
	if fill.positionSide == "" {
		fill.positionSide = fill.side
	}

	t.tradeCache.mu.Lock()
	defer t.tradeCache.mu.Unlock()

	t.tradeCache.fills = append(t.tradeCache.fills, fill)
	t.pruneTradeCacheLocked(time.Now())
}

func (t *FuturesTrader) pruneTradeCacheLocked(now time.Time) {
	if maxAge := t.tradeCache.maxAge; maxAge > 0 {
		cutoff := now.Add(-maxAge)
		n := 0
		for n < len(t.tradeCache.fills) && t.tradeCache.fills[n].time.Before(cutoff) {
			n++
		}
		if n > 0 {
			t.tradeCache.fills = append([]userTradeFill(nil), t.tradeCache.fills[n:]...)
		}
	}

	if maxItems := t.tradeCache.maxItems; maxItems > 0 && len(t.tradeCache.fills) > maxItems {
		t.tradeCache.fills = append([]userTradeFill(nil), t.tradeCache.fills[len(t.tradeCache.fills)-maxItems:]...)
	}
}

func closingSide(positionSide string) string {
	switch strings.ToLower(positionSide) {
	case "long":
		return "sell"
	case "short":
		return "buy"
	default:
		return ""
	}
}

// MatchTradeFill 尝试匹配真实成交，优先缓存，失败则回退REST
func (t *FuturesTrader) MatchTradeFill(symbol, positionSide string, quantity float64, window time.Duration) (*TradeFill, bool) {
	if quantity <= 0 {
		return nil, false
	}
	if window <= 0 {
		window = defaultTradeFillWindow
	}

	symbol = strings.ToUpper(symbol)
	positionSide = strings.ToLower(positionSide)
	closeSide := closingSide(positionSide)

	if fill := t.matchTradeFillFromCache(symbol, positionSide, closeSide, quantity, window); fill != nil {
		return fill, true
	}

	if fill := t.matchTradeFillFromAPI(symbol, positionSide, closeSide, quantity, window); fill != nil {
		return fill, true
	}

	return nil, false
}

func (t *FuturesTrader) matchTradeFillFromCache(symbol, positionSide, closeSide string, quantity float64, window time.Duration) *TradeFill {
	now := time.Now()
	cutoff := time.Time{}
	if window > 0 {
		cutoff = now.Add(-window)
	}

	t.tradeCache.mu.RLock()
	defer t.tradeCache.mu.RUnlock()

	if len(t.tradeCache.fills) == 0 {
		return nil
	}

	var selected []userTradeFill
	accQty := 0.0
	for i := len(t.tradeCache.fills) - 1; i >= 0; i-- {
		fill := t.tradeCache.fills[i]
		if window > 0 && fill.time.Before(cutoff) {
			break
		}
		if !strings.EqualFold(fill.symbol, symbol) {
			continue
		}
		if positionSide != "" && !strings.EqualFold(fill.positionSide, positionSide) {
			continue
		}
		if closeSide != "" && !strings.EqualFold(fill.side, closeSide) {
			continue
		}

		selected = append(selected, fill)
		accQty += fill.quantity
		if math.Abs(accQty-quantity) <= tradeQuantityTolerance {
			return aggregateTradeFills(symbol, selected)
		}
		if accQty > quantity+tradeQuantityTolerance {
			return nil
		}
	}

	return nil
}

func (t *FuturesTrader) matchTradeFillFromAPI(symbol, positionSide, closeSide string, quantity float64, window time.Duration) *TradeFill {
	trades, err := t.GetTradeHistory(200)
	if err != nil {
		log.Printf("⚠️  [TradeMatch] 获取REST成交失败: %v", err)
		return nil
	}

	cutoff := time.Time{}
	if window > 0 {
		cutoff = time.Now().Add(-window)
	}

	var selected []userTradeFill
	accQty := 0.0
	for _, trade := range trades {
		if !strings.EqualFold(fmt.Sprint(trade["symbol"]), symbol) {
			continue
		}

		side := strings.ToLower(fmt.Sprint(trade["side"]))
		if closeSide != "" && side != closeSide {
			continue
		}

		qty := math.Abs(parseFloatValue(trade["quantity"]))
		if qty == 0 {
			continue
		}

		timestamp := parseInt64Value(trade["time_millis"])
		if timestamp == 0 {
			timestamp = parseInt64Value(trade["time"]) * 1000
		}
		tradeTime := time.UnixMilli(timestamp)
		if window > 0 && tradeTime.Before(cutoff) {
			continue
		}

		fill := userTradeFill{
			symbol:       symbol,
			side:         side,
			positionSide: positionSide,
			orderID:      parseInt64Value(trade["order_id"]),
			price:        parseFloatValue(trade["price"]),
			quantity:     qty,
			realizedPnL:  parseFloatValue(trade["realized_pnl"]),
			commission:   math.Abs(parseFloatValue(trade["commission"])),
			feeAsset:     strings.ToUpper(fmt.Sprint(trade["commission_asset"])),
			time:         tradeTime,
		}

		selected = append(selected, fill)
		accQty += qty
		if math.Abs(accQty-quantity) <= tradeQuantityTolerance {
			return aggregateTradeFills(symbol, selected)
		}
		if accQty > quantity+tradeQuantityTolerance {
			return nil
		}
	}

	return nil
}

func aggregateTradeFills(symbol string, fills []userTradeFill) *TradeFill {
	if len(fills) == 0 {
		return nil
	}

	var totalQty, totalValue, totalPnL, totalFee float64
	latest := fills[0].time
	side := fills[0].side
	positionSide := fills[0].positionSide
	feeAsset := fills[0].feeAsset

	for _, f := range fills {
		totalQty += f.quantity
		totalValue += f.price * f.quantity
		totalPnL += f.realizedPnL
		totalFee += f.commission
		if f.time.After(latest) {
			latest = f.time
		}
		if feeAsset == "" {
			feeAsset = f.feeAsset
		}
	}

	avgPrice := 0.0
	if totalQty > 0 {
		avgPrice = totalValue / totalQty
	}

	return &TradeFill{
		Symbol:       symbol,
		Side:         side,
		PositionSide: positionSide,
		Price:        avgPrice,
		Quantity:     totalQty,
		RealizedPnL:  totalPnL,
		Commission:   totalFee,
		FeeAsset:     feeAsset,
		Time:         latest,
	}
}

// ClearPositionsCache 清除持仓缓存
func (t *FuturesTrader) ClearPositionsCache() {
	t.positionsCacheMutex.Lock()
	defer t.positionsCacheMutex.Unlock()
	t.cachedPositions = nil
	t.positionsCacheTime = time.Time{}
	log.Printf("🗑️  已清除持仓缓存，下次将重新从API获取")
}

// SetLeverage 设置杠杆（智能判断+冷却期）
func (t *FuturesTrader) SetLeverage(symbol string, leverage int) error {
	// 先尝试获取当前杠杆（从持仓信息）
	currentLeverage := 0
	positions, err := t.GetPositions()
	if err == nil {
		for _, pos := range positions {
			if pos["symbol"] == symbol {
				if lev, ok := pos["leverage"].(float64); ok {
					currentLeverage = int(lev)
					break
				}
			}
		}
	}

	// 如果当前杠杆已经是目标杠杆，跳过
	if currentLeverage == leverage && currentLeverage > 0 {
		log.Printf("  ✓ %s 杠杆已是 %dx，无需切换", symbol, leverage)
		return nil
	}

	// 切换杠杆
	_, err = t.client.NewChangeLeverageService().
		Symbol(symbol).
		Leverage(leverage).
		Do(context.Background())

	if err != nil {
		// 如果错误信息包含"No need to change"，说明杠杆已经是目标值
		if contains(err.Error(), "No need to change") {
			log.Printf("  ✓ %s 杠杆已是 %dx", symbol, leverage)
			return nil
		}
		return fmt.Errorf("设置杠杆失败: %w", err)
	}

	log.Printf("  ✓ %s 杠杆已切换为 %dx", symbol, leverage)

	// 切换杠杆后等待5秒（避免冷却期错误）
	log.Printf("  ⏱ 等待5秒冷却期...")
	time.Sleep(5 * time.Second)

	return nil
}

// SetMarginType 设置保证金模式
func (t *FuturesTrader) SetMarginType(symbol string, marginType futures.MarginType) error {
	err := t.client.NewChangeMarginTypeService().
		Symbol(symbol).
		MarginType(marginType).
		Do(context.Background())

	if err != nil {
		// 如果已经是该模式，不算错误
		if contains(err.Error(), "No need to change") {
			log.Printf("  ✓ %s 保证金模式已是 %s", symbol, marginType)
			return nil
		}
		return fmt.Errorf("设置保证金模式失败: %w", err)
	}

	log.Printf("  ✓ %s 保证金模式已切换为 %s", symbol, marginType)

	// 切换保证金模式后等待3秒（避免冷却期错误）
	log.Printf("  ⏱ 等待3秒冷却期...")
	time.Sleep(3 * time.Second)

	return nil
}

// OpenLong 开多仓
func (t *FuturesTrader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	// 先取消该币种的所有委托单（清理旧的止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消旧委托单失败（可能没有委托单）: %v", err)
	}

	// 设置杠杆
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}

	// 设置保证金模式（逐仓/全仓）
	if err := t.SetMarginType(symbol, t.defaultMarginType); err != nil {
		return nil, err
	}

	// 格式化数量到正确精度
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 创建市价买入订单
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeBuy).
		PositionSide(futures.PositionSideTypeLong).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("开多仓失败: %w", err)
	}

	log.Printf("✓ 开多仓成功: %s 数量: %s", symbol, quantityStr)
	log.Printf("  订单ID: %d", order.OrderID)

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// OpenShort 开空仓
func (t *FuturesTrader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	// 先取消该币种的所有委托单（清理旧的止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消旧委托单失败（可能没有委托单）: %v", err)
	}

	// 设置杠杆
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}

	// 设置保证金模式（逐仓/全仓）
	if err := t.SetMarginType(symbol, t.defaultMarginType); err != nil {
		return nil, err
	}

	// 格式化数量到正确精度
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 创建市价卖出订单
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeSell).
		PositionSide(futures.PositionSideTypeShort).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("开空仓失败: %w", err)
	}

	log.Printf("✓ 开空仓成功: %s 数量: %s", symbol, quantityStr)
	log.Printf("  订单ID: %d", order.OrderID)

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// CloseLong 平多仓
func (t *FuturesTrader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	// 如果数量为0，获取当前持仓数量
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}

		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "long" {
				quantity = pos["positionAmt"].(float64)
				break
			}
		}

		if quantity == 0 {
			return nil, fmt.Errorf("没有找到 %s 的多仓", symbol)
		}
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 创建市价卖出订单（平多）
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeSell).
		PositionSide(futures.PositionSideTypeLong).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("平多仓失败: %w", err)
	}

	log.Printf("✓ 平多仓成功: %s 数量: %s", symbol, quantityStr)

	// 平仓后取消该币种的所有挂单（止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消挂单失败: %v", err)
	}

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// CloseShort 平空仓
func (t *FuturesTrader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	// 如果数量为0，获取当前持仓数量
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}

		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "short" {
				quantity = -pos["positionAmt"].(float64) // 空仓数量是负的，取绝对值
				break
			}
		}

		if quantity == 0 {
			return nil, fmt.Errorf("没有找到 %s 的空仓", symbol)
		}
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 创建市价买入订单（平空）
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeBuy).
		PositionSide(futures.PositionSideTypeShort).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("平空仓失败: %w", err)
	}

	log.Printf("✓ 平空仓成功: %s 数量: %s", symbol, quantityStr)

	// 平仓后取消该币种的所有挂单（止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消挂单失败: %v", err)
	}

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// CancelAllOrders 取消该币种的所有挂单
func (t *FuturesTrader) CancelAllOrders(symbol string) error {
	err := t.client.NewCancelAllOpenOrdersService().
		Symbol(symbol).
		Do(context.Background())

	if err != nil {
		return fmt.Errorf("取消挂单失败: %w", err)
	}

	log.Printf("  ✓ 已取消 %s 的所有挂单", symbol)
	return nil
}

// GetMarketPrice 获取市场价格
func (t *FuturesTrader) GetMarketPrice(symbol string) (float64, error) {
	prices, err := t.client.NewListPricesService().Symbol(symbol).Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("获取价格失败: %w", err)
	}

	if len(prices) == 0 {
		return 0, fmt.Errorf("未找到价格")
	}

	price, err := strconv.ParseFloat(prices[0].Price, 64)
	if err != nil {
		return 0, err
	}

	return price, nil
}

// CalculatePositionSize 计算仓位大小
func (t *FuturesTrader) CalculatePositionSize(balance, riskPercent, price float64, leverage int) float64 {
	riskAmount := balance * (riskPercent / 100.0)
	positionValue := riskAmount * float64(leverage)
	quantity := positionValue / price
	return quantity
}

// SetStopLoss 设置止损单
func (t *FuturesTrader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error {
	var side futures.SideType
	var posSide futures.PositionSideType

	if positionSide == "LONG" {
		side = futures.SideTypeSell
		posSide = futures.PositionSideTypeLong
	} else {
		side = futures.SideTypeBuy
		posSide = futures.PositionSideTypeShort
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return err
	}

	_, err = t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(side).
		PositionSide(posSide).
		Type(futures.OrderTypeStopMarket).
		StopPrice(fmt.Sprintf("%.8f", stopPrice)).
		Quantity(quantityStr).
		WorkingType(futures.WorkingTypeContractPrice).
		ClosePosition(true).
		Do(context.Background())

	if err != nil {
		return fmt.Errorf("设置止损失败: %w", err)
	}

	log.Printf("  止损价设置: %.4f", stopPrice)
	return nil
}

// SetTakeProfit 设置止盈单
func (t *FuturesTrader) SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error {
	var side futures.SideType
	var posSide futures.PositionSideType

	if positionSide == "LONG" {
		side = futures.SideTypeSell
		posSide = futures.PositionSideTypeLong
	} else {
		side = futures.SideTypeBuy
		posSide = futures.PositionSideTypeShort
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return err
	}

	_, err = t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(side).
		PositionSide(posSide).
		Type(futures.OrderTypeTakeProfitMarket).
		StopPrice(fmt.Sprintf("%.8f", takeProfitPrice)).
		Quantity(quantityStr).
		WorkingType(futures.WorkingTypeContractPrice).
		ClosePosition(true).
		Do(context.Background())

	if err != nil {
		return fmt.Errorf("设置止盈失败: %w", err)
	}

	log.Printf("  止盈价设置: %.4f", takeProfitPrice)
	return nil
}

// GetOpenOrders 获取指定币种的挂单（包括止盈止损订单）
func (t *FuturesTrader) GetOpenOrders(symbol string) ([]map[string]interface{}, error) {
	orders, err := t.client.NewListOpenOrdersService().
		Symbol(symbol).
		Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("获取挂单失败: %w", err)
	}

	var result []map[string]interface{}
	for _, order := range orders {
		// 只关注止盈止损订单
		orderType := string(order.Type)
		if orderType != "STOP_MARKET" && orderType != "TAKE_PROFIT_MARKET" {
			continue
		}

		stopPrice, _ := strconv.ParseFloat(order.StopPrice, 64)
		quantity, _ := strconv.ParseFloat(order.OrigQuantity, 64)

		orderMap := map[string]interface{}{
			"symbol":    order.Symbol,
			"orderId":   order.OrderID,
			"side":      string(order.Side),
			"type":      orderType,
			"stopPrice": stopPrice,
			"quantity":  quantity,
		}

		// 如果是止盈单
		if orderType == "TAKE_PROFIT_MARKET" {
			orderMap["orderType"] = "take_profit"
		} else if orderType == "STOP_MARKET" {
			orderMap["orderType"] = "stop_loss"
		}

		result = append(result, orderMap)
	}

	return result, nil
}

// GetSymbolPrecision 获取交易对的数量精度
func (t *FuturesTrader) GetSymbolPrecision(symbol string) (int, error) {
	exchangeInfo, err := t.client.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("获取交易规则失败: %w", err)
	}

	for _, s := range exchangeInfo.Symbols {
		if s.Symbol == symbol {
			// 从LOT_SIZE filter获取精度
			for _, filter := range s.Filters {
				if filter["filterType"] == "LOT_SIZE" {
					stepSize := filter["stepSize"].(string)
					precision := calculatePrecision(stepSize)
					log.Printf("  %s 数量精度: %d (stepSize: %s)", symbol, precision, stepSize)
					return precision, nil
				}
			}
		}
	}

	log.Printf("  ⚠ %s 未找到精度信息，使用默认精度3", symbol)
	return 3, nil // 默认精度为3
}

// calculatePrecision 从stepSize计算精度
func calculatePrecision(stepSize string) int {
	// 去除尾部的0
	stepSize = trimTrailingZeros(stepSize)

	// 查找小数点
	dotIndex := -1
	for i := 0; i < len(stepSize); i++ {
		if stepSize[i] == '.' {
			dotIndex = i
			break
		}
	}

	// 如果没有小数点或小数点在最后，精度为0
	if dotIndex == -1 || dotIndex == len(stepSize)-1 {
		return 0
	}

	// 返回小数点后的位数
	return len(stepSize) - dotIndex - 1
}

// trimTrailingZeros 去除尾部的0
func trimTrailingZeros(s string) string {
	// 如果没有小数点，直接返回
	if !stringContains(s, ".") {
		return s
	}

	// 从后向前遍历，去除尾部的0
	for len(s) > 0 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}

	// 如果最后一位是小数点，也去掉
	if len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}

	return s
}

// FormatQuantity 格式化数量到正确的精度
func (t *FuturesTrader) FormatQuantity(symbol string, quantity float64) (string, error) {
	precision, err := t.GetSymbolPrecision(symbol)
	if err != nil {
		// 如果获取失败，使用默认格式
		return fmt.Sprintf("%.3f", quantity), nil
	}

	format := fmt.Sprintf("%%.%df", precision)
	return fmt.Sprintf(format, quantity), nil
}

// 辅助函数
func contains(s, substr string) bool {
	return len(s) >= len(substr) && stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// GetTradeHistory 获取历史成交记录（从币安API获取）
// 改进：支持分页获取，尽可能获取更多历史数据
func (t *FuturesTrader) GetTradeHistory(limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 1000 // 默认最多1000条
	}
	if limit > 1000 {
		limit = 1000 // 币安API单次请求限制最多1000条
	}

	// 币安API限制：最大时间间隔为7天，最多1000条/次
	// 策略：分多个7天时间段获取，每次最多1000条
	endTime := time.Now().Unix() * 1000
	// 获取最近30天的数据（分多个7天时间段）
	daysToFetch := 30
	maxDaysPerRequest := 7 // 币安API限制：最大7天间隔

	var allTrades []*futures.AccountTrade
	currentEndTime := endTime
	maxRequests := 20 // 最多请求20次（30天 ÷ 7天 ≈ 5次，留有余量）

	for i := 0; i < maxRequests && len(allTrades) < limit; i++ {
		// 计算当前请求的时间范围（最多7天）
		currentStartTime := currentEndTime - int64(maxDaysPerRequest*24*3600*1000)
		if currentStartTime < 0 {
			currentStartTime = 0
		}

		// 使用币安合约账户成交API获取历史成交
		trades, err := t.client.NewListAccountTradeService().
			StartTime(currentStartTime).
			EndTime(currentEndTime).
			Limit(1000). // 每次最多1000条
			Do(context.Background())
		if err != nil {
			if i == 0 {
				return nil, fmt.Errorf("获取历史成交失败: %w", err)
			}
			// 如果后续请求失败，返回已获取的数据
			log.Printf("⚠️  获取历史成交（%d-%d）失败: %v，返回已获取的数据", currentStartTime, currentEndTime, err)
			break
		}

		if len(trades) == 0 {
			// 没有更多数据，向前移动时间窗口
			currentEndTime = currentStartTime - 1
			if currentEndTime <= 0 {
				break // 已经到达最早时间
			}
			continue
		}

		// 将新获取的数据添加到结果中（注意：币安API返回的是从旧到新，需要合并）
		allTrades = append(allTrades, trades...)

		// 如果返回的数据少于1000条，说明这个时间段已经获取完所有数据
		if len(trades) < 1000 {
			// 向前移动时间窗口，继续获取更早的数据
			currentEndTime = currentStartTime - 1
			if currentEndTime <= 0 {
				break // 已经到达最早时间
			}
			continue
		}

		// 如果返回了1000条，可能还有更多数据，需要在这个时间段内继续分页
		// 但币安API限制时间间隔最大7天，所以我们需要按时间分页
		// 找到这个时间段内的最早交易时间，作为下次请求的结束时间
		if len(trades) > 0 {
			earliestTradeTime := trades[0].Time // 第一条是最早的
			if earliestTradeTime <= currentStartTime {
				// 已经获取完这个时间段的所有数据，向前移动时间窗口
				currentEndTime = currentStartTime - 1
				if currentEndTime <= 0 {
					break
				}
			} else {
				// 在这个时间段内继续分页
				currentEndTime = earliestTradeTime - 1
			}
		}

		// 如果已经获取足够的数据，停止
		if len(allTrades) >= limit {
			allTrades = allTrades[:limit]
			break
		}

		// 检查是否已经获取了足够天数的数据
		earliestTime := endTime
		for _, t := range allTrades {
			if t.Time < earliestTime {
				earliestTime = t.Time
			}
		}
		daysFetched := (endTime - earliestTime) / (24 * 3600 * 1000)
		if daysFetched >= int64(daysToFetch) {
			break // 已经获取了足够天数的数据
		}
	}

	trades := allTrades

	result := make([]map[string]interface{}, 0, len(trades))
	for _, trade := range trades {
		// 解析价格和数量
		price, _ := strconv.ParseFloat(trade.Price, 64)
		qty, _ := strconv.ParseFloat(trade.Quantity, 64)
		realizedPnl, _ := strconv.ParseFloat(trade.RealizedPnl, 64)
		commission, _ := strconv.ParseFloat(trade.Commission, 64)

		// 判断方向：买单为开多/平空，卖单为开空/平多
		side := "long"
		action := "open_long"
		if trade.Side == futures.SideTypeSell {
			side = "short"
			action = "open_short"
		}

		// 如果有realized_pnl，说明是平仓
		if realizedPnl != 0 {
			if trade.Side == futures.SideTypeBuy {
				action = "close_short"
			} else {
				action = "close_long"
			}
		}

		// 解析时间戳
		timeUnix := trade.Time / 1000 // 转换为秒

		result = append(result, map[string]interface{}{
			"order_id":     trade.OrderID,
			"symbol":       trade.Symbol,
			"side":         side,
			"action":       action,
			"price":        price,
			"quantity":     qty,
			"realized_pnl": realizedPnl,
			"commission":   commission,
			"time":         timeUnix,
			"time_millis":  trade.Time,
			"source":       "exchange", // 标记为交易所数据
		})
	}

	// 按时间倒序排列（最新的在前）
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return result, nil
}
