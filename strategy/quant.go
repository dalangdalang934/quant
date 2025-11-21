package strategy

// QuantConfig 定义了基础量化策略参数
type QuantConfig struct {
	AllowShort              bool    `json:"allow_short"`
	MaxLongPositions        int     `json:"max_long_positions"`
	MaxShortPositions       int     `json:"max_short_positions"`
	PositionSizePct         float64 `json:"position_size_pct"`
	MinSignalScore          float64 `json:"min_signal_score"`
	StopLossATRMultiplier   float64 `json:"stop_loss_atr_multiplier"`
	TakeProfitATRMultiplier float64 `json:"take_profit_atr_multiplier"`
	RiskReward              float64 `json:"risk_reward"`
	MinHoldMinutes          int     `json:"min_hold_minutes"`
	CooldownMinutes         int     `json:"cooldown_minutes"`
}

// Normalize 确保配置落在合理区间
func (c *QuantConfig) Normalize() {
	if c.MaxLongPositions <= 0 {
		c.MaxLongPositions = 2
	}
	if c.MaxShortPositions < 0 {
		c.MaxShortPositions = 0
	}
	if c.PositionSizePct <= 0 || c.PositionSizePct > 0.5 {
		c.PositionSizePct = 0.1
	}
	if c.MinSignalScore <= 0 {
		c.MinSignalScore = 3.5
	}
	if c.StopLossATRMultiplier <= 0 {
		c.StopLossATRMultiplier = 1.2
	}
	if c.TakeProfitATRMultiplier <= 0 {
		c.TakeProfitATRMultiplier = 3.0
	}
	if c.RiskReward <= 0 {
		c.RiskReward = 3.0
	}
	if c.MinHoldMinutes < 0 {
		c.MinHoldMinutes = 0
	}
	if c.CooldownMinutes < 0 {
		c.CooldownMinutes = 0
	}
}

// Copy 返回副本，避免共享指针
func (c *QuantConfig) Copy() QuantConfig {
	if c == nil {
		return QuantConfig{}
	}
	cp := *c
	return cp
}
