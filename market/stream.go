package market

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/url"
    "strings"
    "sync"
    "time"

    "github.com/gorilla/websocket"
)

// BinanceMarketStreamInit 市场流初始化选项
// Endpoint 默认使用 wss://fstream.binance.com
// ReconnectDelay/PingInterval 可由配置覆盖
// 如果 Enabled=false，则不会初始化流。
type BinanceMarketStreamInit struct {
    Enabled        bool
    Endpoint       string
    ReconnectDelay time.Duration
    PingInterval   time.Duration
}

var (
    marketStreamMu sync.RWMutex
    marketStream   *binanceMarketStream
)

func ConfigureBinanceMarketStream(opts BinanceMarketStreamInit) {
    if !opts.Enabled {
        return
    }

    if opts.Endpoint == "" {
        opts.Endpoint = "wss://fstream.binance.com"
    }
    if opts.ReconnectDelay <= 0 {
        opts.ReconnectDelay = 5 * time.Second
    }
    if opts.PingInterval <= 0 {
        opts.PingInterval = 30 * time.Second
    }

    marketStreamMu.Lock()
    defer marketStreamMu.Unlock()

    if marketStream == nil {
        marketStream = newBinanceMarketStream(opts)
    } else {
        marketStream.updateOptions(opts)
    }
}

func getBinanceMarketStream() *binanceMarketStream {
    marketStreamMu.RLock()
    defer marketStreamMu.RUnlock()
    return marketStream
}

type binanceMarketStream struct {
    endpoint       string
    reconnectDelay time.Duration
    pingInterval   time.Duration

    mu             sync.RWMutex
    symbolStreams  map[string]*symbolStream
    enabled        bool
}

func newBinanceMarketStream(opts BinanceMarketStreamInit) *binanceMarketStream {
    stream := &binanceMarketStream{
        endpoint:       opts.Endpoint,
        reconnectDelay: opts.ReconnectDelay,
        pingInterval:   opts.PingInterval,
        symbolStreams:  make(map[string]*symbolStream),
        enabled:        opts.Enabled,
    }
    return stream
}

func (s *binanceMarketStream) updateOptions(opts BinanceMarketStreamInit) {
    s.mu.Lock()
    defer s.mu.Unlock()
    if opts.Endpoint != "" {
        s.endpoint = opts.Endpoint
    }
    if opts.ReconnectDelay > 0 {
        s.reconnectDelay = opts.ReconnectDelay
    }
    if opts.PingInterval > 0 {
        s.pingInterval = opts.PingInterval
    }
    if opts.Enabled {
        s.enabled = true
    }
}

func (s *binanceMarketStream) IsEnabled() bool {
    return s != nil && s.enabled
}

func (s *binanceMarketStream) EnsureSymbol(symbol string) bool {
    if !s.IsEnabled() {
        return false
    }

    canonical := Normalize(symbol)
    s.mu.Lock()
    defer s.mu.Unlock()

    if stream, ok := s.symbolStreams[canonical]; ok {
        return stream.ensureRunning()
    }

    stream := newSymbolStream(s, canonical)
    s.symbolStreams[canonical] = stream
    return stream.ensureRunning()
}

func (s *binanceMarketStream) GetKlines(symbol, interval string, limit int) ([]Kline, bool) {
    if !s.IsEnabled() {
        return nil, false
    }
    canonical := Normalize(symbol)

    s.mu.RLock()
    stream, ok := s.symbolStreams[canonical]
    s.mu.RUnlock()
    if !ok {
        return nil, false
    }
    return stream.getKlines(interval, limit)
}

type symbolStream struct {
    parent         *binanceMarketStream
    symbol         string
    intervals      []string

    ctxCancel      context.CancelFunc
    mu             sync.RWMutex
    klineCache     map[string][]Kline
    ready          bool
}

func newSymbolStream(parent *binanceMarketStream, symbol string) *symbolStream {
    return &symbolStream{
        parent:     parent,
        symbol:     symbol,
        intervals:  []string{"3m", "1h", "4h"},
        klineCache: make(map[string][]Kline),
    }
}

func (ss *symbolStream) ensureRunning() bool {
    ss.mu.Lock()
    defer ss.mu.Unlock()

    if ss.ready {
        return true
    }

    ctx, cancel := context.WithCancel(context.Background())
    ss.ctxCancel = cancel
    go ss.run(ctx)
    ss.ready = true
    return true
}

func (ss *symbolStream) run(ctx context.Context) {
    // 先预热缓存
    ss.seedInitialData()

    for {
        if err := ss.consume(ctx); err != nil {
            select {
            case <-ctx.Done():
                return
            default:
            }
            log.Printf("⚠️  [MarketStream] %s 连接异常: %v，%s 后重试", ss.symbol, err, ss.parent.reconnectDelay)
            time.Sleep(ss.parent.reconnectDelay)
            continue
        }
        return
    }
}

func (ss *symbolStream) consume(ctx context.Context) error {
    streams := make([]string, 0, len(ss.intervals))
    lowerSymbol := strings.ToLower(ss.symbol)
    for _, interval := range ss.intervals {
        streams = append(streams, fmt.Sprintf("%s@kline_%s", lowerSymbol, interval))
    }

    u, err := url.Parse(ss.parent.endpoint)
    if err != nil {
        return fmt.Errorf("解析WebSocket端点失败: %w", err)
    }

    base := strings.TrimSuffix(u.String(), "/")
    wsURL := fmt.Sprintf("%s/stream?streams=%s", base, strings.Join(streams, "/"))

    dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
    conn, _, err := dialer.DialContext(ctx, wsURL, nil)
    if err != nil {
        return fmt.Errorf("连接Binance WebSocket失败: %w", err)
    }
    defer conn.Close()

    pingTicker := time.NewTicker(ss.parent.pingInterval)
    defer pingTicker.Stop()

    errCh := make(chan error, 1)

    go func() {
        for {
            select {
            case <-ctx.Done():
                return
            case <-pingTicker.C:
                deadline := time.Now().Add(10 * time.Second)
                if err := conn.WriteControl(websocket.PingMessage, []byte("ping"), deadline); err != nil {
                    errCh <- fmt.Errorf("发送Ping失败: %w", err)
                    return
                }
            }
        }
    }()

    for {
        select {
        case <-ctx.Done():
            return context.Canceled
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
        ss.handleMessage(message)
    }
}

type streamEnvelope struct {
    Stream string          `json:"stream"`
    Data   json.RawMessage `json:"data"`
}

type klineEvent struct {
    K klinePayload `json:"k"`
}

type klinePayload struct {
    StartTime int64  `json:"t"`
    CloseTime int64  `json:"T"`
    Symbol    string `json:"s"`
    Interval  string `json:"i"`
    Open      string `json:"o"`
    Close     string `json:"c"`
    High      string `json:"h"`
    Low       string `json:"l"`
    Volume    string `json:"v"`
    IsFinal   bool   `json:"x"`
}

func (ss *symbolStream) handleMessage(payload []byte) {
    var envelope streamEnvelope
    if err := json.Unmarshal(payload, &envelope); err != nil {
        log.Printf("⚠️  [MarketStream] 解析消息失败: %v", err)
        return
    }

    if len(envelope.Data) == 0 {
        return
    }

    var event klineEvent
    if err := json.Unmarshal(envelope.Data, &event); err != nil {
        log.Printf("⚠️  [MarketStream] 解析K线数据失败: %v", err)
        return
    }

    kl := event.K
    if !kl.IsFinal {
        return
    }

    open, err1 := parseFloat(kl.Open)
    closePrice, err2 := parseFloat(kl.Close)
    high, err3 := parseFloat(kl.High)
    low, err4 := parseFloat(kl.Low)
    volume, err5 := parseFloat(kl.Volume)
    if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil {
        return
    }

    ss.mu.Lock()
    defer ss.mu.Unlock()

    cache := ss.klineCache[kl.Interval]
    cache = append(cache, Kline{
        OpenTime:  kl.StartTime,
        Open:      open,
        High:      high,
        Low:       low,
        Close:     closePrice,
        Volume:    volume,
        CloseTime: kl.CloseTime,
    })
    if len(cache) > 600 {
        cache = cache[len(cache)-600:]
    }
    ss.klineCache[kl.Interval] = cache
}

func (ss *symbolStream) getKlines(interval string, limit int) ([]Kline, bool) {
    ss.mu.RLock()
    defer ss.mu.RUnlock()

    data := ss.klineCache[interval]
    if len(data) == 0 {
        return nil, false
    }

    if limit <= 0 || limit > len(data) {
        limit = len(data)
    }

    out := make([]Kline, limit)
    copy(out, data[len(data)-limit:])
    return out, true
}

func (ss *symbolStream) seedInitialData() {
    for _, interval := range ss.intervals {
        if klines, err := fetchKlinesREST(ss.symbol, interval, 600); err == nil {
            ss.mu.Lock()
            ss.klineCache[interval] = klines
            ss.mu.Unlock()
        }
    }
}
