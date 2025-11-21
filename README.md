# 📈 量化交易执行与监控系统

> 单一量化策略、实盘执行、可视化监控 —— 面向 Binance/Hyperliquid/Aster 等永续交易所的轻量级自动交易栈。

## 功能亮点

- **自动化量化策略**：内置基于 EMA/MACD/RSI/OI 的动量策略，按固定频率扫描行情、自动下单与撤单。
- **交易所抽象层**：当前实现 Binance Futures，代码同时保留 Hyperliquid/Aster 适配器，可扩展到其他永续交易接口。
- **实盘级风控**：按交易对区分杠杆上限、动态缩放仓位、最小下单金额校验、暂停开仓冷静期等。
- **决策与日志**：每个周期都会记录账户快照、候选币种、执行动作及成交明细，方便审计和复盘。
- **实时监控 UI**：Next.js 前端提供盈亏曲线、当前持仓、历史订单三个核心面板，默认 10 秒刷新。
- **Docker/PM2 支持**：可直接编译运行，也可通过 Docker Compose 或 PM2 守护进程部署。

## 技术栈

| 层 | 技术 | 说明 |
| --- | --- | --- |
| 后端 | Go 1.22 + Gin | REST API / 任务调度 |
| 交易所 | go-binance/v2 | 下单、账户、行情接口 | 
| 前端 | Next.js 16 + React 19 + SWR | 实时看板、数据聚合代理 |
| 数据 | JSON 日志 + 本地文件 | 决策记录、持仓快照、交易汇总 |

## 目录结构（精简）

```
jiaoyibot/
├── main.go                 # 后端入口
├── config/                 # 配置解析 & 默认值
├── trader/                 # 量化交易执行引擎（Binance/Hyperliquid/Aster 实现）
├── decision/               # 行情打分、策略决策生成
├── market/                 # 行情聚合与技术指标
├── logger/                 # 决策 & 交易日志工具
├── api/                    # Gin API（/api/*）
├── web/                    # Next.js 前端（positions / equity / trades）
├── docker-compose.yml      # 可选部署
└── scripts/                # 日志归档等辅助脚本
```

## 快速开始

### 1. 准备配置

复制示例:

```bash
cp config.json.example config.json
```

`config.json` 示例：

```json
{
  "traders": [
    {
      "id": "binance_quant",
      "name": "Binance Quant",
      "enabled": true,
      "exchange": "binance",
      "margin_type": "isolated",
      "binance_api_key": "your_key",
      "binance_secret_key": "your_secret",
      "initial_balance": 200,
      "scan_interval_minutes": 2,
      "quant_config": {
        "allow_short": true,
        "max_long_positions": 2,
        "max_short_positions": 1,
        "position_size_pct": 0.12,
        "min_signal_score": 4.0,
        "stop_loss_atr_multiplier": 1.3,
        "take_profit_atr_multiplier": 3.0,
        "risk_reward": 3.0,
        "min_hold_minutes": 10,
        "cooldown_minutes": 2
      }
    }
  ],
  "leverage": {
    "btc_eth_leverage": 5,
    "altcoin_leverage": 5
  },
  "use_default_coins": true,
  "default_coins": ["BTCUSDT", "ETHUSDT", "SOLUSDT"],
  "coin_pool_api_url": "",
  "oi_top_api_url": "",
  "api_server_port": 8080,
  "max_daily_loss": 10,
  "max_drawdown": 25,
  "stop_trading_minutes": 60
}
```

### 2. 启动后端

```bash
go build -o quantd
./quantd config.json
```

### 3. 启动前端

```bash
cd web
pnpm install
pnpm dev --port 3000
```

浏览器访问 `http://localhost:3000`，即可看到盈亏曲线 / 当前持仓 / 历史订单三个面板。

## 关键 API

| Endpoint | 说明 |
| --- | --- |
| `GET /api/competition` | 当前唯一策略的账户汇总、盈亏、保证金占用 |
| `GET /api/account?trader_id=` | 钱包余额、可用保证金、未实现盈亏 |
| `GET /api/positions?trader_id=` | 交易所实时持仓及止盈/止损挂单 |
| `GET /api/exchange-trades?trader_id=` | 原始成交记录与聚合后的开平仓区间 |
| `GET /api/decisions/latest?trader_id=` | 最近若干决策（用于审计与调试） |
| `POST /diagnostics/sync-positions` | 重新匹配日志与交易所持仓，常用于重启后恢复上下文 |

前端的 `/api/nof1/*` 路由对上述接口做了聚合与缓存，可直接复用。

## 前端视图

- **盈亏曲线**：调用 `equity-history`，展示净值、保证金使用率等指标。
- **当前持仓**：包含入场价、标记价、杠杆、保证金、持仓时长、止盈止损等。
- **历史订单**：汇总最近 100 笔交易，含盈亏、持仓时长、平仓备注，可跳转到详情。

## 附：ETH/BTC 双资产动量策略脚本

仓库新增 `strategy/dual_asset_strategy.py`，把常用的 ETH/USDT ↔ BTC/USDT 配对逻辑标准化，方便在本地快速回测：

- **策略要点**：以 ETH/BTC 价差的 21/55 均线金叉/死叉作为方向判断，持仓时同时做多一条腿、做空另一条腿，默认 3% 止损、6% 止盈、10 倍杠杆名义。
- **依赖**：`pip install backtrader ccxt pandas`
- **运行示例**：

```bash
python strategy/dual_asset_strategy.py --days 400 --timeframe 30m --fast 21 --slow 55 --leverage 8 --stop 0.025 --take 0.05
```

命令行参数可以覆盖周期、杠杆、止盈止损以及抽样天数，输出包含净值、收益率、夏普、回撤与 Profit Factor，方便与主策略对比或作为提示词素材。

## 常见问题

1. **Binance API 报错 `-2019 margin is insufficient`**：调小 `position_size_pct` 或者在策略里启用 `cooldown_minutes`，确保同一周期不会重复开仓。
2. **多策略需求**：当前版本仅运行第一个启用的 `trader`，其余配置会被忽略；如果要扩展请在 `main.go` 中修改 `addedTrader` 限制。
3. **如何追加候选币种**：在 `config.json` 的 `additional_coins` 添加符号，或配置 `coin_pool_api_url`/`oi_top_api_url` 以 HTTP 方式拉取列表。
4. **日志位置**：决策日志位于 `decision_logs/<trader_id>/`，交易所成交汇总位于 `data/exchange_trades/<trader_id>.json`。

## 致谢

本项目初版基于社区开源项目 [NOFX](https://github.com/tinkle-community/nofx) 与 [NOF0](https://github.com/wquguru/nof0) 的理念与实现，现已重构为单一量化策略形态，保留原有的交易所适配与前端视觉风格，感谢原作者的开源贡献。
