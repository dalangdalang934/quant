package trader

// Trader 交易器统一接口
// 支持多个交易平台（币安、Hyperliquid等）
type Trader interface {
	// GetBalance 获取账户余额
	GetBalance() (map[string]interface{}, error)

	// GetPositions 获取所有持仓
	GetPositions() ([]map[string]interface{}, error)

	// GetOpenOrders 获取所有挂单（包括止盈止损订单）
	// 返回订单列表，每个订单包含: symbol, side, type, stopPrice, quantity 等字段
	GetOpenOrders(symbol string) ([]map[string]interface{}, error)

	// OpenLong 开多仓
	OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error)

	// OpenShort 开空仓
	OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error)

	// CloseLong 平多仓（quantity=0表示全部平仓）
	CloseLong(symbol string, quantity float64) (map[string]interface{}, error)

	// CloseShort 平空仓（quantity=0表示全部平仓）
	CloseShort(symbol string, quantity float64) (map[string]interface{}, error)

	// SetLeverage 设置杠杆
	SetLeverage(symbol string, leverage int) error

	// GetMarketPrice 获取市场价格
	GetMarketPrice(symbol string) (float64, error)

	// SetStopLoss 设置止损单
	SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error

	// SetTakeProfit 设置止盈单
	SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error

	// CancelAllOrders 取消该币种的所有挂单
	CancelAllOrders(symbol string) error

	// FormatQuantity 格式化数量到正确的精度
	FormatQuantity(symbol string, quantity float64) (string, error)

	// GetTradeHistory 获取历史成交记录（从交易所API获取）
	// limit: 返回最近N条记录，0表示获取所有
	// 返回成交记录列表，每个记录包含: symbol, side, price, quantity, time, realized_pnl 等字段
	GetTradeHistory(limit int) ([]map[string]interface{}, error)
}
