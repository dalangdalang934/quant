package market

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"strings"
)

// Data 市场数据结构
type Data struct {
	Symbol            string
	CurrentPrice      float64
	PriceChange1h     float64 // 1小时价格变化百分比
	PriceChange4h     float64 // 4小时价格变化百分比
	CurrentEMA20      float64
	CurrentMACD       float64
	CurrentRSI7       float64
	CurrentBBUpper    float64 // 当前布林带上轨
	CurrentBBLower    float64 // 当前布林带下轨
	CurrentBBWidth    float64 // 当前布林带宽度
	CurrentVWAP       float64 // 当前VWAP
	OpenInterest      *OIData
	FundingRate       float64
	IntradaySeries    *IntradayData
	MediumTermContext *MediumTermData
	LongerTermContext *LongerTermData
}

// OIData Open Interest数据
type OIData struct {
	Latest  float64
	Average float64
}

// IntradayData 日内数据(3分钟间隔)
type IntradayData struct {
	MidPrices     []float64
	EMA20Values   []float64
	MACDValues    []float64
	RSI7Values    []float64
	RSI14Values   []float64
	BBUpperValues []float64 // 布林带上轨序列
	BBLowerValues []float64 // 布林带下轨序列
	BBWidthValues []float64 // 布林带宽度序列（波动率）
	VWAPValues    []float64 // VWAP序列
	OBVValues     []float64 // OBV序列（能量潮）
}

// MediumTermData 中期数据(1小时时间框架)
type MediumTermData struct {
	EMA20         float64
	EMA50         float64
	MACD          float64
	RSI14         float64
	ATR14         float64
	BBUpper       float64
	BBLower       float64
	BBWidth       float64
	VWAP          float64
	CurrentVolume float64
	AverageVolume float64
}

// LongerTermData 长期数据(4小时时间框架)
type LongerTermData struct {
	EMA20         float64
	EMA50         float64
	EMA100        float64 // 中期趋势
	EMA200        float64 // 长期趋势
	ATR3          float64
	ATR14         float64
	CurrentVolume float64
	AverageVolume float64
	MACDValues    []float64
	RSI14Values   []float64
	ADX           float64    // 平均趋向指数（趋势强度）
	PlusDI        float64    // 正向动向指数
	MinusDI       float64    // 负向动向指数
	VWAP          float64    // 当前VWAP
	Fibonacci     *FibLevels // 斐波那契回撤/扩展
}

// FibLevels 斐波那契回撤和扩展水平
type FibLevels struct {
	Retracement236 float64 // 23.6% 回撤
	Retracement382 float64 // 38.2% 回撤
	Retracement500 float64 // 50% 回撤
	Retracement618 float64 // 61.8% 回撤
	Extension1272  float64 // 127.2% 扩展
	Extension1618  float64 // 161.8% 扩展
	Extension2000  float64 // 200% 扩展
	High           float64 // 计算区间最高价
	Low            float64 // 计算区间最低价
}

// Kline K线数据
type Kline struct {
	OpenTime  int64
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	CloseTime int64
}

// Get 获取指定代币的市场数据
func Get(symbol string) (*Data, error) {
	// 标准化symbol
	symbol = Normalize(symbol)

	// 获取3分钟K线数据 (最近10个)
	klines3m, err := getKlines(symbol, "3m", 40) // 多获取一些用于计算
	if err != nil {
		return nil, fmt.Errorf("获取3分钟K线失败: %v", err)
	}

	// 获取1小时K线数据
	klines1h, err := getKlines(symbol, "1h", 120)
	if err != nil {
		return nil, fmt.Errorf("获取1小时K线失败: %v", err)
	}

	// 获取4小时K线数据 (最近10个)
	klines4h, err := getKlines(symbol, "4h", 60) // 多获取用于计算指标
	if err != nil {
		return nil, fmt.Errorf("获取4小时K线失败: %v", err)
	}

	// 计算当前指标 (基于3分钟最新数据)
	currentPrice := klines3m[len(klines3m)-1].Close
	currentEMA20 := calculateEMA(klines3m, 20)
	currentMACD := calculateMACD(klines3m)
	currentRSI7 := calculateRSI(klines3m, 7)

	// 计算当前布林带
	bbUpper, bbLower, bbWidth := calculateBollingerBands(klines3m, 20, 2.0)

	// 计算当前VWAP
	currentVWAP := calculateVWAP(klines3m)

	// 计算价格变化百分比
	// 1小时价格变化 = 1根1小时K线前的价格
	priceChange1h := 0.0
	if len(klines1h) >= 2 {
		price1hAgo := klines1h[len(klines1h)-2].Close
		if price1hAgo > 0 {
			priceChange1h = ((currentPrice - price1hAgo) / price1hAgo) * 100
		}
	}

	// 4小时价格变化 = 1个4小时K线前的价格
	priceChange4h := 0.0
	if len(klines4h) >= 2 {
		price4hAgo := klines4h[len(klines4h)-2].Close
		if price4hAgo > 0 {
			priceChange4h = ((currentPrice - price4hAgo) / price4hAgo) * 100
		}
	}

	// 获取OI数据
	oiData, err := getOpenInterestData(symbol)
	if err != nil {
		// OI失败不影响整体,使用默认值
		oiData = &OIData{Latest: 0, Average: 0}
	}

	// 获取Funding Rate
	fundingRate, _ := getFundingRate(symbol)

	// 计算日内系列数据
	intradayData := calculateIntradaySeries(klines3m)

	// 计算中期和长期数据
	mediumTermData := calculateMediumTermData(klines1h)
	longerTermData := calculateLongerTermData(klines4h)

	return &Data{
		Symbol:            symbol,
		CurrentPrice:      currentPrice,
		PriceChange1h:     priceChange1h,
		PriceChange4h:     priceChange4h,
		CurrentEMA20:      currentEMA20,
		CurrentMACD:       currentMACD,
		CurrentRSI7:       currentRSI7,
		CurrentBBUpper:    bbUpper,
		CurrentBBLower:    bbLower,
		CurrentBBWidth:    bbWidth,
		CurrentVWAP:       currentVWAP,
		OpenInterest:      oiData,
		FundingRate:       fundingRate,
		IntradaySeries:    intradayData,
		MediumTermContext: mediumTermData,
		LongerTermContext: longerTermData,
	}, nil
}

// getKlines 获取K线数据（优先从WSS缓存，失败时回落REST）
func getKlines(symbol, interval string, limit int) ([]Kline, error) {
	if stream := getBinanceMarketStream(); stream != nil && stream.IsEnabled() {
		if klines, ok := stream.GetKlines(symbol, interval, limit); ok {
			return klines, nil
		}
		if stream.EnsureSymbol(symbol) {
			if klines, ok := stream.GetKlines(symbol, interval, limit); ok {
				return klines, nil
			}
		}
	}
	return fetchKlinesREST(symbol, interval, limit)
}

// fetchKlinesREST 从Binance REST接口获取K线数据
func fetchKlinesREST(symbol, interval string, limit int) ([]Kline, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/klines?symbol=%s&interval=%s&limit=%d",
		symbol, interval, limit)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rawData [][]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, err
	}

	klines := make([]Kline, len(rawData))
	for i, item := range rawData {
		openTime := int64(item[0].(float64))
		open, _ := parseFloat(item[1])
		high, _ := parseFloat(item[2])
		low, _ := parseFloat(item[3])
		close, _ := parseFloat(item[4])
		volume, _ := parseFloat(item[5])
		closeTime := int64(item[6].(float64))

		klines[i] = Kline{
			OpenTime:  openTime,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    volume,
			CloseTime: closeTime,
		}
	}

	return klines, nil
}

// calculateEMA 计算EMA
func calculateEMA(klines []Kline, period int) float64 {
	if len(klines) < period {
		return 0
	}

	// 计算SMA作为初始EMA
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += klines[i].Close
	}
	ema := sum / float64(period)

	// 计算EMA
	multiplier := 2.0 / float64(period+1)
	for i := period; i < len(klines); i++ {
		ema = (klines[i].Close-ema)*multiplier + ema
	}

	return ema
}

// calculateMACD 计算MACD
func calculateMACD(klines []Kline) float64 {
	if len(klines) < 26 {
		return 0
	}

	// 计算12期和26期EMA
	ema12 := calculateEMA(klines, 12)
	ema26 := calculateEMA(klines, 26)

	// MACD = EMA12 - EMA26
	return ema12 - ema26
}

// calculateRSI 计算RSI
func calculateRSI(klines []Kline, period int) float64 {
	if len(klines) <= period {
		return 0
	}

	gains := 0.0
	losses := 0.0

	// 计算初始平均涨跌幅
	for i := 1; i <= period; i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			gains += change
		} else {
			losses += -change
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	// 使用Wilder平滑方法计算后续RSI
	for i := period + 1; i < len(klines); i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			avgGain = (avgGain*float64(period-1) + change) / float64(period)
			avgLoss = (avgLoss * float64(period-1)) / float64(period)
		} else {
			avgGain = (avgGain * float64(period-1)) / float64(period)
			avgLoss = (avgLoss*float64(period-1) + (-change)) / float64(period)
		}
	}

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))

	return rsi
}

// calculateATR 计算ATR
func calculateATR(klines []Kline, period int) float64 {
	if len(klines) <= period {
		return 0
	}

	trs := make([]float64, len(klines))
	for i := 1; i < len(klines); i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close

		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)

		trs[i] = math.Max(tr1, math.Max(tr2, tr3))
	}

	// 计算初始ATR
	sum := 0.0
	for i := 1; i <= period; i++ {
		sum += trs[i]
	}
	atr := sum / float64(period)

	// Wilder平滑
	for i := period + 1; i < len(klines); i++ {
		atr = (atr*float64(period-1) + trs[i]) / float64(period)
	}

	return atr
}

// calculateIntradaySeries 计算日内系列数据
func calculateIntradaySeries(klines []Kline) *IntradayData {
	data := &IntradayData{
		MidPrices:     make([]float64, 0, 10),
		EMA20Values:   make([]float64, 0, 10),
		MACDValues:    make([]float64, 0, 10),
		RSI7Values:    make([]float64, 0, 10),
		RSI14Values:   make([]float64, 0, 10),
		BBUpperValues: make([]float64, 0, 10),
		BBLowerValues: make([]float64, 0, 10),
		BBWidthValues: make([]float64, 0, 10),
		VWAPValues:    make([]float64, 0, 10),
		OBVValues:     make([]float64, 0, 10),
	}

	// 获取最近10个数据点
	start := len(klines) - 10
	if start < 0 {
		start = 0
	}

	// 计算OBV（需要从开始计算）
	obv := 0.0
	for i := 1; i < start; i++ {
		if klines[i].Close > klines[i-1].Close {
			obv += klines[i].Volume
		} else if klines[i].Close < klines[i-1].Close {
			obv -= klines[i].Volume
		}
	}

	for i := start; i < len(klines); i++ {
		data.MidPrices = append(data.MidPrices, klines[i].Close)

		// 计算每个点的EMA20
		if i >= 19 {
			ema20 := calculateEMA(klines[:i+1], 20)
			data.EMA20Values = append(data.EMA20Values, ema20)
		}

		// 计算每个点的MACD
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}

		// 计算每个点的RSI
		if i >= 7 {
			rsi7 := calculateRSI(klines[:i+1], 7)
			data.RSI7Values = append(data.RSI7Values, rsi7)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}

		// 计算每个点的布林带
		if i >= 19 {
			bbUpper, bbLower, bbWidth := calculateBollingerBands(klines[:i+1], 20, 2.0)
			data.BBUpperValues = append(data.BBUpperValues, bbUpper)
			data.BBLowerValues = append(data.BBLowerValues, bbLower)
			data.BBWidthValues = append(data.BBWidthValues, bbWidth)
		}

		// 计算每个点的VWAP
		if i >= 0 {
			vwap := calculateVWAP(klines[:i+1])
			data.VWAPValues = append(data.VWAPValues, vwap)
		}

		// 计算每个点的OBV
		if i > 0 {
			if klines[i].Close > klines[i-1].Close {
				obv += klines[i].Volume
			} else if klines[i].Close < klines[i-1].Close {
				obv -= klines[i].Volume
			}
			data.OBVValues = append(data.OBVValues, obv)
		} else {
			data.OBVValues = append(data.OBVValues, obv)
		}
	}

	return data
}

// calculateMediumTermData 计算中期数据
func calculateMediumTermData(klines []Kline) *MediumTermData {
	if len(klines) == 0 {
		return nil
	}

	data := &MediumTermData{}

	data.EMA20 = calculateEMA(klines, 20)
	data.EMA50 = calculateEMA(klines, 50)
	data.MACD = calculateMACD(klines)
	data.RSI14 = calculateRSI(klines, 14)
	data.ATR14 = calculateATR(klines, 14)

	upper, lower, width := calculateBollingerBands(klines, 20, 2.0)
	data.BBUpper = upper
	data.BBLower = lower
	data.BBWidth = width

	data.VWAP = calculateVWAP(klines)

	data.CurrentVolume = klines[len(klines)-1].Volume
	sumVolume := 0.0
	for _, k := range klines {
		sumVolume += k.Volume
	}
	if len(klines) > 0 {
		data.AverageVolume = sumVolume / float64(len(klines))
	}

	return data
}

// calculateLongerTermData 计算长期数据
func calculateLongerTermData(klines []Kline) *LongerTermData {
	data := &LongerTermData{
		MACDValues:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
	}

	// 计算EMA
	data.EMA20 = calculateEMA(klines, 20)
	data.EMA50 = calculateEMA(klines, 50)
	if len(klines) >= 100 {
		data.EMA100 = calculateEMA(klines, 100)
	}
	if len(klines) >= 200 {
		data.EMA200 = calculateEMA(klines, 200)
	}

	// 计算ATR
	data.ATR3 = calculateATR(klines, 3)
	data.ATR14 = calculateATR(klines, 14)

	// 计算成交量
	if len(klines) > 0 {
		data.CurrentVolume = klines[len(klines)-1].Volume
		// 计算平均成交量
		sum := 0.0
		for _, k := range klines {
			sum += k.Volume
		}
		data.AverageVolume = sum / float64(len(klines))
	}

	// 计算VWAP
	data.VWAP = calculateVWAP(klines)

	// 计算ADX
	if len(klines) >= 14 {
		adx, plusDI, minusDI := calculateADX(klines, 14)
		data.ADX = adx
		data.PlusDI = plusDI
		data.MinusDI = minusDI
	}

	// 计算斐波那契回撤/扩展
	if len(klines) >= 20 {
		data.Fibonacci = calculateFibonacci(klines)
	}

	// 计算MACD和RSI序列
	start := len(klines) - 10
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}

	return data
}

// getOpenInterestData 获取OI数据
func getOpenInterestData(symbol string) (*OIData, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/openInterest?symbol=%s", symbol)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		OpenInterest string `json:"openInterest"`
		Symbol       string `json:"symbol"`
		Time         int64  `json:"time"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	oi, _ := strconv.ParseFloat(result.OpenInterest, 64)

	return &OIData{
		Latest:  oi,
		Average: oi * 0.999, // 近似平均值
	}, nil
}

// getFundingRate 获取资金费率
func getFundingRate(symbol string) (float64, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/premiumIndex?symbol=%s", symbol)

	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		Symbol          string `json:"symbol"`
		MarkPrice       string `json:"markPrice"`
		IndexPrice      string `json:"indexPrice"`
		LastFundingRate string `json:"lastFundingRate"`
		NextFundingTime int64  `json:"nextFundingTime"`
		InterestRate    string `json:"interestRate"`
		Time            int64  `json:"time"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	rate, _ := strconv.ParseFloat(result.LastFundingRate, 64)
	return rate, nil
}

// calculateBollingerBands 计算布林带
func calculateBollingerBands(klines []Kline, period int, numStdDev float64) (upper, lower, width float64) {
	if len(klines) < period {
		return 0, 0, 0
	}

	// 计算SMA
	sum := 0.0
	for i := len(klines) - period; i < len(klines); i++ {
		sum += klines[i].Close
	}
	sma := sum / float64(period)

	// 计算标准差
	variance := 0.0
	for i := len(klines) - period; i < len(klines); i++ {
		diff := klines[i].Close - sma
		variance += diff * diff
	}
	varianceVal := variance / float64(period)

	// 安全检查：确保方差非负
	if varianceVal < 0 {
		varianceVal = 0
	}

	stdDev := math.Sqrt(varianceVal)

	// 添加NaN/Inf检查
	if math.IsNaN(stdDev) || math.IsInf(stdDev, 0) {
		stdDev = 0
	}

	upper = sma + (numStdDev * stdDev)
	lower = sma - (numStdDev * stdDev)
	width = upper - lower

	// 最终安全检查
	if math.IsNaN(upper) || math.IsInf(upper, 0) {
		upper = sma
	}
	if math.IsNaN(lower) || math.IsInf(lower, 0) {
		lower = sma
	}
	if math.IsNaN(width) || math.IsInf(width, 0) {
		width = 0
	}

	return upper, lower, width
}

// calculateVWAP 计算成交量加权平均价
func calculateVWAP(klines []Kline) float64 {
	if len(klines) == 0 {
		return 0
	}

	totalPV := 0.0 // Price * Volume
	totalVolume := 0.0

	for _, k := range klines {
		// 使用典型价格 (High + Low + Close) / 3
		typicalPrice := (k.High + k.Low + k.Close) / 3.0
		totalPV += typicalPrice * k.Volume
		totalVolume += k.Volume
	}

	if totalVolume == 0 {
		return 0
	}

	vwap := totalPV / totalVolume

	// 添加NaN/Inf检查
	if math.IsNaN(vwap) || math.IsInf(vwap, 0) {
		// 如果VWAP计算失败，返回最后一个收盘价作为fallback
		if len(klines) > 0 {
			return klines[len(klines)-1].Close
		}
		return 0
	}

	return vwap
}

// calculateADX 计算ADX（平均趋向指数）
func calculateADX(klines []Kline, period int) (adx, plusDI, minusDI float64) {
	if len(klines) < period+1 {
		return 0, 0, 0
	}

	// 计算True Range和Directional Movement
	trs := make([]float64, len(klines))
	plusDMs := make([]float64, len(klines))
	minusDMs := make([]float64, len(klines))

	for i := 1; i < len(klines); i++ {
		// True Range
		tr1 := klines[i].High - klines[i].Low
		tr2 := math.Abs(klines[i].High - klines[i-1].Close)
		tr3 := math.Abs(klines[i].Low - klines[i-1].Close)
		trs[i] = math.Max(tr1, math.Max(tr2, tr3))

		// Directional Movement
		upMove := klines[i].High - klines[i-1].High
		downMove := klines[i-1].Low - klines[i].Low

		if upMove > downMove && upMove > 0 {
			plusDMs[i] = upMove
		} else {
			plusDMs[i] = 0
		}

		if downMove > upMove && downMove > 0 {
			minusDMs[i] = downMove
		} else {
			minusDMs[i] = 0
		}
	}

	// 计算初始平滑值
	trSum := 0.0
	plusDMSum := 0.0
	minusDMSum := 0.0

	start := len(klines) - period
	if start < 1 {
		start = 1
	}

	// 修复：确保实际累加的周期数等于period
	actualPeriod := 0
	for i := start; i < len(klines) && actualPeriod < period; i++ {
		trSum += trs[i]
		plusDMSum += plusDMs[i]
		minusDMSum += minusDMs[i]
		actualPeriod++
	}

	if actualPeriod == 0 {
		return 0, 0, 0
	}

	atr := trSum / float64(actualPeriod)
	if atr == 0 {
		return 0, 0, 0
	}

	plusDI = (plusDMSum / atr) * 100
	minusDI = (minusDMSum / atr) * 100

	// 计算DX
	diDiff := math.Abs(plusDI - minusDI)
	diSum := plusDI + minusDI
	if diSum == 0 {
		adx = 0
	} else {
		dx := (diDiff / diSum) * 100
		// ADX是DX的平滑值（简化版，使用当前DX）
		// 注意：标准ADX应该使用Wilder平滑，这里简化处理
		adx = dx
	}

	// 添加NaN/Inf检查
	if math.IsNaN(adx) || math.IsInf(adx, 0) {
		adx = 0
	}
	if math.IsNaN(plusDI) || math.IsInf(plusDI, 0) {
		plusDI = 0
	}
	if math.IsNaN(minusDI) || math.IsInf(minusDI, 0) {
		minusDI = 0
	}

	return adx, plusDI, minusDI
}

// calculateFibonacci 计算斐波那契回撤和扩展水平
func calculateFibonacci(klines []Kline) *FibLevels {
	if len(klines) < 20 {
		return nil
	}

	// 找到最近20个K线的最高价和最低价
	high := klines[0].High
	low := klines[0].Low

	start := len(klines) - 20
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		if klines[i].High > high {
			high = klines[i].High
		}
		if klines[i].Low < low {
			low = klines[i].Low
		}
	}

	diff := high - low
	if diff == 0 {
		return nil
	}

	// 判断趋势方向（使用最新价格）
	currentPrice := klines[len(klines)-1].Close
	midPoint := (high + low) / 2.0

	isUptrend := currentPrice > midPoint

	var fibLevels FibLevels
	fibLevels.High = high
	fibLevels.Low = low

	if isUptrend {
		// 上涨趋势：从低点到高点计算回撤和扩展
		fibLevels.Retracement236 = high - (diff * 0.236)
		fibLevels.Retracement382 = high - (diff * 0.382)
		fibLevels.Retracement500 = high - (diff * 0.500)
		fibLevels.Retracement618 = high - (diff * 0.618)
		fibLevels.Extension1272 = high + (diff * 1.272) // 修复：127.2%扩展 = 1.272倍
		fibLevels.Extension1618 = high + (diff * 1.618) // 161.8%扩展 = 1.618倍
		fibLevels.Extension2000 = high + diff           // 200%扩展 = 2.0倍
	} else {
		// 下跌趋势：从高点到低点计算回撤和扩展
		fibLevels.Retracement236 = low + (diff * 0.236)
		fibLevels.Retracement382 = low + (diff * 0.382)
		fibLevels.Retracement500 = low + (diff * 0.500)
		fibLevels.Retracement618 = low + (diff * 0.618)
		fibLevels.Extension1272 = low - (diff * 1.272) // 修复：127.2%扩展 = 1.272倍
		fibLevels.Extension1618 = low - (diff * 1.618) // 161.8%扩展 = 1.618倍
		fibLevels.Extension2000 = low - diff           // 200%扩展 = 2.0倍
	}

	return &fibLevels
}

// Format 格式化输出市场数据
func Format(data *Data) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("current_price = %.2f, current_ema20 = %.3f, current_macd = %.3f, current_rsi (7 period) = %.3f\n",
		data.CurrentPrice, data.CurrentEMA20, data.CurrentMACD, data.CurrentRSI7))
	sb.WriteString(fmt.Sprintf("Bollinger Bands: Upper = %.3f, Lower = %.3f, Width = %.3f\n",
		data.CurrentBBUpper, data.CurrentBBLower, data.CurrentBBWidth))
	sb.WriteString(fmt.Sprintf("VWAP = %.3f\n\n", data.CurrentVWAP))

	sb.WriteString(fmt.Sprintf("In addition, here is the latest %s open interest and funding rate for perps:\n\n",
		data.Symbol))

	if data.OpenInterest != nil {
		sb.WriteString(fmt.Sprintf("Open Interest: Latest: %.2f Average: %.2f\n\n",
			data.OpenInterest.Latest, data.OpenInterest.Average))
	}

	sb.WriteString(fmt.Sprintf("Funding Rate: %.2e\n\n", data.FundingRate))

	if data.IntradaySeries != nil {
		sb.WriteString("Intraday series (3‑minute intervals, oldest → latest):\n\n")

		if len(data.IntradaySeries.MidPrices) > 0 {
			sb.WriteString(fmt.Sprintf("Mid prices: %s\n\n", formatFloatSlice(data.IntradaySeries.MidPrices)))
		}

		if len(data.IntradaySeries.EMA20Values) > 0 {
			sb.WriteString(fmt.Sprintf("EMA indicators (20‑period): %s\n\n", formatFloatSlice(data.IntradaySeries.EMA20Values)))
		}

		if len(data.IntradaySeries.MACDValues) > 0 {
			sb.WriteString(fmt.Sprintf("MACD indicators: %s\n\n", formatFloatSlice(data.IntradaySeries.MACDValues)))
		}

		if len(data.IntradaySeries.RSI7Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI indicators (7‑Period): %s\n\n", formatFloatSlice(data.IntradaySeries.RSI7Values)))
		}

		if len(data.IntradaySeries.RSI14Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI indicators (14‑Period): %s\n\n", formatFloatSlice(data.IntradaySeries.RSI14Values)))
		}

		if len(data.IntradaySeries.BBUpperValues) > 0 {
			sb.WriteString(fmt.Sprintf("Bollinger Bands Upper: %s\n\n", formatFloatSlice(data.IntradaySeries.BBUpperValues)))
		}

		if len(data.IntradaySeries.BBLowerValues) > 0 {
			sb.WriteString(fmt.Sprintf("Bollinger Bands Lower: %s\n\n", formatFloatSlice(data.IntradaySeries.BBLowerValues)))
		}

		if len(data.IntradaySeries.BBWidthValues) > 0 {
			sb.WriteString(fmt.Sprintf("Bollinger Bands Width (volatility): %s\n\n", formatFloatSlice(data.IntradaySeries.BBWidthValues)))
		}

		if len(data.IntradaySeries.VWAPValues) > 0 {
			sb.WriteString(fmt.Sprintf("VWAP values: %s\n\n", formatFloatSlice(data.IntradaySeries.VWAPValues)))
		}

		if len(data.IntradaySeries.OBVValues) > 0 {
			sb.WriteString(fmt.Sprintf("OBV (On-Balance Volume): %s\n\n", formatFloatSlice(data.IntradaySeries.OBVValues)))
		}
	}

	if data.MediumTermContext != nil {
		sb.WriteString("Medium-term context (1-hour timeframe):\n\n")
		sb.WriteString(fmt.Sprintf("EMA: 20 = %.3f, 50 = %.3f\n",
			data.MediumTermContext.EMA20, data.MediumTermContext.EMA50))
		sb.WriteString(fmt.Sprintf("MACD: %.3f | RSI(14): %.3f | ATR(14): %.3f\n",
			data.MediumTermContext.MACD, data.MediumTermContext.RSI14, data.MediumTermContext.ATR14))
		sb.WriteString(fmt.Sprintf("Bollinger Bands: upper = %.3f, lower = %.3f, width = %.3f\n",
			data.MediumTermContext.BBUpper, data.MediumTermContext.BBLower, data.MediumTermContext.BBWidth))
		if data.MediumTermContext.VWAP > 0 {
			sb.WriteString(fmt.Sprintf("VWAP: %.3f\n", data.MediumTermContext.VWAP))
		}
		sb.WriteString(fmt.Sprintf("Volume: current = %.3f, average = %.3f\n\n",
			data.MediumTermContext.CurrentVolume, data.MediumTermContext.AverageVolume))
	}

	if data.LongerTermContext != nil {
		sb.WriteString("Longer‑term context (4‑hour timeframe):\n\n")

		sb.WriteString(fmt.Sprintf("EMA: 20 = %.3f, 50 = %.3f",
			data.LongerTermContext.EMA20, data.LongerTermContext.EMA50))
		if data.LongerTermContext.EMA100 > 0 {
			sb.WriteString(fmt.Sprintf(", 100 = %.3f", data.LongerTermContext.EMA100))
		}
		if data.LongerTermContext.EMA200 > 0 {
			sb.WriteString(fmt.Sprintf(", 200 = %.3f", data.LongerTermContext.EMA200))
		}
		sb.WriteString("\n\n")

		sb.WriteString(fmt.Sprintf("3‑Period ATR: %.3f vs. 14‑Period ATR: %.3f\n\n",
			data.LongerTermContext.ATR3, data.LongerTermContext.ATR14))

		sb.WriteString(fmt.Sprintf("Current Volume: %.3f vs. Average Volume: %.3f\n\n",
			data.LongerTermContext.CurrentVolume, data.LongerTermContext.AverageVolume))

		if data.LongerTermContext.VWAP > 0 {
			sb.WriteString(fmt.Sprintf("VWAP: %.3f\n\n", data.LongerTermContext.VWAP))
		}

		if data.LongerTermContext.ADX > 0 {
			sb.WriteString(fmt.Sprintf("ADX (trend strength): %.2f | +DI: %.2f | -DI: %.2f\n",
				data.LongerTermContext.ADX, data.LongerTermContext.PlusDI, data.LongerTermContext.MinusDI))
			if data.LongerTermContext.ADX > 25 {
				sb.WriteString("  (ADX > 25 indicates strong trend)\n")
			} else if data.LongerTermContext.ADX < 20 {
				sb.WriteString("  (ADX < 20 indicates ranging market)\n")
			}
			sb.WriteString("\n")
		}

		if data.LongerTermContext.Fibonacci != nil {
			fib := data.LongerTermContext.Fibonacci
			sb.WriteString(fmt.Sprintf("Fibonacci Levels (High: %.3f, Low: %.3f):\n",
				fib.High, fib.Low))
			sb.WriteString(fmt.Sprintf("  Retracements: 23.6%% = %.3f, 38.2%% = %.3f, 50%% = %.3f, 61.8%% = %.3f\n",
				fib.Retracement236, fib.Retracement382, fib.Retracement500, fib.Retracement618))
			sb.WriteString(fmt.Sprintf("  Extensions: 127.2%% = %.3f, 161.8%% = %.3f, 200%% = %.3f\n\n",
				fib.Extension1272, fib.Extension1618, fib.Extension2000))
		}

		if len(data.LongerTermContext.MACDValues) > 0 {
			sb.WriteString(fmt.Sprintf("MACD indicators: %s\n\n", formatFloatSlice(data.LongerTermContext.MACDValues)))
		}

		if len(data.LongerTermContext.RSI14Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI indicators (14‑Period): %s\n\n", formatFloatSlice(data.LongerTermContext.RSI14Values)))
		}
	}

	return sb.String()
}

// formatFloatSlice 格式化float64切片为字符串
func formatFloatSlice(values []float64) string {
	strValues := make([]string, len(values))
	for i, v := range values {
		strValues[i] = fmt.Sprintf("%.3f", v)
	}
	return "[" + strings.Join(strValues, ", ") + "]"
}

// Normalize 标准化symbol,确保是USDT交易对
func Normalize(symbol string) string {
	symbol = strings.ToUpper(symbol)
	if strings.HasSuffix(symbol, "USDT") {
		return symbol
	}
	return symbol + "USDT"
}

// parseFloat 解析float值
func parseFloat(v interface{}) (float64, error) {
	switch val := v.(type) {
	case string:
		return strconv.ParseFloat(val, 64)
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("unsupported type: %T", v)
	}
}
