package decision

import (
	"nofx/market"
	"nofx/symbols"
	"strings"
)

func assetAliases(asset string) []string {
	return symbols.Aliases(asset)
}

func marketDataForAsset(ctx *Context, asset string) (*market.Data, string, bool) {
	for _, alias := range assetAliases(asset) {
		if data, ok := ctx.MarketDataMap[alias]; ok && data != nil {
			return data, alias, true
		}
	}
	return nil, "", false
}

func findPositionByAsset(ctx *Context, asset, side string) *PositionInfo {
	for _, alias := range assetAliases(asset) {
		if pos := findPositionInfo(ctx, alias, side); pos != nil {
			return pos
		}
	}
	return nil
}

func isMajorSymbol(symbol string) bool {
	return symbols.IsAsset(symbol, "BTC") || symbols.IsAsset(symbol, "ETH")
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
