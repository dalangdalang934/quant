package decision

import "time"

const (
	// PairDirectionLong 表示做多 ETH / 做空 BTC 的配对
	PairDirectionLong = "long"
	// PairDirectionShort 表示做空 ETH / 做多 BTC 的配对
	PairDirectionShort = "short"

	// StrategyTagEthBtcPair 用于标记 ETH/BTC 配对策略的决策
	StrategyTagEthBtcPair = "ETHBTC_PAIR"
)

// PairState 追踪 ETH/BTC 配对仓位的状态
type PairState struct {
	Active      bool
	Direction   string
	EntryEquity float64
	EntryTime   time.Time
	EntryPrice  float64
}

// Copy 返回副本，避免跨 goroutine 共用指针
func (ps *PairState) Copy() *PairState {
	if ps == nil {
		return nil
	}
	cp := *ps
	return &cp
}
