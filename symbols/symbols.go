package symbols

import "strings"

var stableQuotes = []string{"USDT", "USDC"}

var majorAliasMap = map[string][]string{
	"BTC": {"BTCUSDT", "BTCUSDC"},
	"ETH": {"ETHUSDT", "ETHUSDC"},
}

// Normalize 清理符号（去除分隔符并转为大写）
func Normalize(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "/", "")
	return s
}

// HasStableQuote 判断是否以受支持的稳定币结尾（USDT/USDC）
func HasStableQuote(symbol string) bool {
	s := Normalize(symbol)
	for _, quote := range stableQuotes {
		if strings.HasSuffix(s, quote) {
			return true
		}
	}
	return false
}

// StripStableQuote 去除末尾的稳定币后缀，返回基础资产
func StripStableQuote(symbol string) string {
	s := Normalize(symbol)
	for _, quote := range stableQuotes {
		if strings.HasSuffix(s, quote) {
			return strings.TrimSuffix(s, quote)
		}
	}
	return s
}

// Aliases 返回基础资产对应的受支持交易对列表（按优先级排序）
func Aliases(asset string) []string {
	base := StripStableQuote(asset)
	if aliases, ok := majorAliasMap[base]; ok {
		return append([]string(nil), aliases...)
	}

	results := make([]string, 0, len(stableQuotes))
	for _, quote := range stableQuotes {
		results = append(results, base+quote)
	}
	return results
}

// IsAsset 判断 symbol 是否属于指定基础资产（忽略稳定币后缀）
func IsAsset(symbol, asset string) bool {
	return StripStableQuote(symbol) == StripStableQuote(asset)
}
