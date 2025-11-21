# 🤖 AI 交易竞赛系统

> **结合 NOFX 和 NOF0 的完整 AI 交易竞技场解决方案**

一个基于 Go 后端 + Next.js 前端的多 AI 模型加密货币交易竞赛平台，支持实盘交易、实时监控和性能分析。支持原生部署和 Docker 容器化部署。

## 📋 项目简介

本项目是一个**多 AI 模型交易竞赛系统**，允许你配置多个 AI 交易员（如 DeepSeek、Qwen 等）在真实加密货币市场中进行实盘交易竞赛。系统提供完整的交易执行、风险控制、决策记录和实时监控功能。

### 核心特性

- ✅ **多 AI 模型支持**：DeepSeek、Qwen、自定义 OpenAI 兼容 API
- ✅ **多交易所支持**：Binance、Hyperliquid、Aster DEX
- ✅ **实盘交易**：真实资金交易，非回测模拟
- ✅ **实时监控**：Web 界面实时展示账户、持仓、盈亏（10秒自动刷新）
- ✅ **持仓详情**：显示开仓价格、标记价格、强平价格、止盈止损、保证金模式等详细信息
- ✅ **决策记录**：完整记录 AI 的思考过程和交易决策
- ✅ **风险控制**：止损止盈、杠杆限制、仓位管理
- ✅ **性能分析**：胜率、盈亏比、夏普比率等指标
- ✅ **原生部署**：支持直接运行和 PM2 守护进程管理
- ✅ **Docker 部署**：支持 Docker 容器化部署，一键启动前后端

## 🏗️ 技术架构

### 后端技术栈

| 组件 | 技术选型 | 说明 |
|------|---------|------|
| 语言 | Go 1.25+ | 高性能并发处理 |
| Web 框架 | Gin | RESTful API 服务器 |
| 交易所 SDK | go-binance, go-hyperliquid | 多交易所统一接口 |
| 区块链 | go-ethereum | Hyperliquid 和 Aster 支持 |

### 前端技术栈

| 组件 | 技术选型 | 说明 |
|------|---------|------|
| 框架 | Next.js 16 + React 19 | 全栈 React 框架 |
| 语言 | TypeScript | 类型安全 |
| 样式 | Tailwind CSS 4 | 实用优先的 CSS 框架 |
| 数据获取 | SWR | 数据获取和缓存，支持自动刷新 |
| 状态管理 | Zustand | 轻量级状态管理 |
| 图表库 | Recharts | 数据可视化 |
| Markdown | react-markdown | AI 决策内容渲染 |

## 📦 项目结构

```
jiaoyibot/
├── main.go                    # 程序入口
├── config.json.example        # 配置文件模板
├── docker-compose.yml         # Docker Compose 配置
├── Dockerfile.backend         # 后端 Dockerfile
│
├── api/                       # API 服务器
│   └── server.go              # REST API 端点
│
├── config/                    # 配置管理
│   └── config.go              # 配置加载和验证
│
├── trader/                    # 交易核心
│   ├── auto_trader.go         # 自动交易主控制器
│   ├── binance_futures.go     # 币安合约接口
│   ├── hyperliquid_trader.go  # Hyperliquid 接口
│   ├── aster_trader.go        # Aster DEX 接口
│   └── interface.go           # 交易接口定义
│
├── manager/                   # 多交易员管理
│   └── trader_manager.go       # 交易员生命周期管理
│
├── decision/                  # 决策引擎
│   └── engine.go              # AI 决策流程和 Prompt 构建
│
├── market/                    # 市场数据
│   └── data.go                # K线数据和技术指标
│
├── pool/                      # 币种池
│   └── coin_pool.go           # 币种列表管理
│
├── logger/                    # 日志记录
│   └── decision_logger.go     # 决策日志和性能分析
│
├── mcp/                       # AI 通信
│   └── client.go              # AI API 客户端
│
└── web/                       # 前端应用
    ├── Dockerfile.frontend    # 前端 Dockerfile
    ├── src/
    │   ├── app/               # Next.js 应用路由
    │   ├── components/        # React 组件
    │   ├── lib/               # 工具库
    │   └── store/             # 状态管理
    └── package.json
```

## 🚀 快速开始

### 环境要求

- **Go 1.25+**
- **Node.js 18+**
- **TA-Lib**（技术指标计算库）

#### 安装 TA-Lib

**macOS:**
```bash
brew install ta-lib
```

**Ubuntu/Debian:**
```bash
sudo apt-get install libta-lib0-dev
```

**其他系统**: 参考 [TA-Lib 官方文档](https://github.com/markcheno/go-talib)

### 方式一：原生部署

#### 1. 克隆项目

```bash
git clone <repository-url>
cd jiaoyibot
```

#### 2. 安装依赖

**后端:**
```bash
go mod download
```

**前端:**
```bash
cd web
npm install
cd ..
```

#### 3. 配置系统

复制配置模板并编辑：

```bash
cp config.json.example config.json
nano config.json  # 或使用任何编辑器
```

**配置文件说明：**

```json
{
  "traders": [
    {
      "id": "my_trader",
      "name": "我的AI交易员",
      "enabled": true,
      "ai_model": "deepseek",          // "deepseek", "qwen", "custom"
      "exchange": "binance",            // "binance", "hyperliquid", "aster"
      "binance_api_key": "YOUR_KEY",
      "binance_secret_key": "YOUR_SECRET",
      "deepseek_key": "sk-xxxxxxxx",
      "initial_balance": 1000.0,
      "scan_interval_minutes": 2
    }
  ],
  "leverage": {
    "btc_eth_leverage": 5,
    "altcoin_leverage": 5
  },
  "use_default_coins": true,
  "default_coins": ["BTCUSDT", "ETHUSDT", "SOLUSDT"],
  "api_server_port": 8080
}
```

#### 4. 启动后端

```bash
# 编译
go build -o nofx

# 运行
./nofx
```

后端会在 `http://localhost:8080` 启动 API 服务。

#### 5. 启动前端

打开新的终端窗口：

```bash
cd web
npm run dev
```

前端会在 `http://localhost:3000` 启动。

### 方式二：Docker 部署（推荐）

使用 Docker Compose 一键启动前后端服务。

#### 1. 配置环境变量

创建 `.env` 文件（可选，用于覆盖默认配置）：

```bash
# 映射的配置文件（绝对或相对路径）
CONFIG_FILE=./config.prod.json
# 容器暴露的端口
BACKEND_PORT=8080
FRONTEND_PORT=3000
# 前端 Server 端访问后端 API 的地址
NOF1_API_BASE_URL=http://backend:8080
# 浏览器调用的公开 API 前缀
NEXT_PUBLIC_NOF1_API_BASE_URL=/api/nof1
# 行情 Ticker
NEXT_PUBLIC_TICKER_PAIRS=BTCUSDT,ETHUSDT,SOLUSDT
```

#### 2. 配置交易系统

编辑 `config.json` 文件（与原生部署相同）。

#### 3. 启动服务

```bash
docker-compose up -d
```

#### 4. 查看日志

```bash
# 查看所有服务日志
docker-compose logs -f

# 查看后端日志
docker-compose logs -f backend

# 查看前端日志
docker-compose logs -f frontend
```

#### 5. 停止服务

```bash
docker-compose down
```

详细 Docker 部署说明请参考 [DOCKER_DEPLOY.md](./DOCKER_DEPLOY.md)。

### 6. 访问监控界面

在浏览器中打开 `http://localhost:3000`，即可看到：

- 📊 **资产总览**：账户净值曲线
- 📈 **持仓情况**：当前持仓和未实现盈亏（包含详细的持仓信息：开仓价格、标记价格、强平价格、止盈止损、保证金模式等）
- 💰 **成交记录**：历史交易明细
- 🤖 **模型对话**：AI 决策的完整思考过程
- 📉 **AI学习与反思**：胜率统计、币种表现分析

## 🔧 使用 PM2 管理（推荐生产环境）

PM2 可以让服务在后台运行，自动重启，并支持开机自启。

### 安装 PM2

```bash
npm install -g pm2
```

### 启动服务

```bash
./pm2.sh start
```

### 其他命令

```bash
./pm2.sh status      # 查看状态
./pm2.sh logs        # 查看日志
./pm2.sh stop        # 停止服务
./pm2.sh restart     # 重启服务
./pm2.sh rebuild     # 重新编译后端并重启
```

详细说明请参考 `pm2.sh` 脚本。

## 💡 核心功能

### 1. 多 AI 模型支持

系统支持多种 AI 模型：

- **DeepSeek**：性价比高的中文 AI 模型
- **Qwen（通义千问）**：阿里云 AI 模型
- **自定义 API**：支持任何 OpenAI 兼容的 API（GPT-4、Claude 等）

每个交易员可以配置不同的 AI 模型，同台竞技。

### 2. 多交易所支持

- **Binance**：全球最大的加密货币交易所
- **Hyperliquid**：去中心化永续合约交易所
- **Aster DEX**：兼容 Binance API 的去中心化交易所

### 3. 智能决策流程

每个交易周期（默认 3 分钟），系统会：

1. **分析历史表现**：计算胜率、盈亏比、最佳/最差币种
2. **获取账户状态**：净值、可用余额、持仓情况
3. **分析持仓**：现有持仓的技术指标和持有时长
4. **评估新机会**：候选币种的市场数据和技术指标
5. **AI 综合决策**：基于完整上下文做出交易决策
6. **执行交易**：自动下单、设置止损止盈
7. **记录日志**：保存完整的决策过程和执行结果

### 4. 风险控制

- **止损止盈**：AI 自动设置，风险回报比 ≥ 1:3，从交易所实时获取实际订单
- **杠杆限制**：BTC/ETH 最高 5x，山寨币最高 5x（可配置）
- **仓位管理**：最多同时持仓 3 个币种
- **保证金控制**：总使用率 ≤ 90%
- **保证金模式**：支持逐仓（Isolated）和全仓（Cross）模式
- **日亏损限制**：达到阈值自动停止交易
- **最大回撤限制**：保护账户资金

### 5. 实时监控

Web 界面提供实时数据更新（10秒自动刷新）：

- 账户净值曲线（支持多模型对比）
- 当前持仓列表（方向、币种、杠杆、盈亏）
- **持仓详情**：
  - 数量、开仓价格、标记价格
  - 损益两平价、强平价格
  - 保证金模式和保证金金额
  - 止盈止损价格（从交易所实时获取）
- 成交历史（开仓、平仓、盈亏）
- AI 决策日志（输入提示、思考过程、输出结果）
- 性能统计（胜率、总盈亏、平均盈亏）

### 6. 币种选择

支持两种模式：

- **默认币种**：预定义的主流币种列表（BTC、ETH、SOL 等）
- **动态币种池**：通过 API 获取 AI500 和 OI Top 币种列表

## 📊 API 接口

后端提供以下 REST API 端点：

| 端点 | 说明 |
|------|------|
| `GET /health` | 健康检查 |
| `GET /api/competition` | 竞赛概览 |
| `GET /api/account` | 账户信息 |
| `GET /api/positions` | 持仓列表（包含详细持仓信息） |
| `GET /api/trades` | 成交记录 |
| `GET /api/decisions` | 决策日志 |
| `GET /api/performance` | 性能统计 |
| `GET /api/equity-history` | 净值历史 |
| `GET /api/status` | 交易员状态 |
| `GET /api/statistics` | 统计数据 |

详细 API 文档请参考代码中的 `api/server.go`。

## 🔐 安全提示

⚠️ **重要安全提示**：

1. **API 密钥安全**：
   - `config.json` 包含敏感信息，已被 `.gitignore` 忽略
   - 不要将真实的 API 密钥提交到 Git 仓库
   - 建议使用交易所的子账户 API，限制权限为仅合约交易

2. **资金安全**：
   - 建议使用小额资金进行测试
   - 设置合理的止损和日亏损限制
   - 定期检查账户状态和交易记录

3. **网络安全**：
   - API 服务器默认仅监听本地（`localhost:8080`）
   - 如需外网访问，请配置反向代理（如 Nginx）并启用 HTTPS

## 🛠️ 开发指南

### 修改配置

修改 `config.json` 后需要重启后端服务。

### 查看日志

后端日志：
- 标准输出：实时显示交易决策和状态
- 文件日志：`logs/backend-out.log` 和 `logs/backend-error.log`

决策日志：
- JSON 格式存储在 `decision_logs/<trader_id>/` 目录

### 调试技巧

1. **测试配置**：使用 `config.mock.json` 进行测试（不进行真实交易）
2. **单交易员模式**：只启用一个交易员进行调试
3. **增加扫描间隔**：将 `scan_interval_minutes` 调大（默认2分钟），减少决策频率
4. **查看决策日志**：检查 `decision_logs/` 目录中的 JSON 文件

## 📈 性能优化

### 后端优化

- 使用连接池复用交易所 API 连接
- 批量获取市场数据，减少 API 调用
- 缓存技术指标计算结果

### 前端优化

- SWR 自动缓存和重新验证
- 代码分割和懒加载
- 虚拟滚动（大数据列表）
- 10秒自动刷新机制，确保数据实时性

## ❓ 常见问题

### 币安持仓模式错误 (code=-4061)

**错误信息**：`Order's position side does not match user's setting`

**原因**：系统需要使用双向持仓模式，但您的币安账户设置为单向持仓。

**解决方法**：

1. 登录币安合约交易平台
2. 点击右上角的 ⚙️ 偏好设置
3. 选择 **持仓模式**
4. 切换为 **双向持仓 (Hedge Mode)**
5. 确认切换

> ⚠️ **注意**：切换前必须先平掉所有持仓。

## 🤝 贡献指南

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License

---

## 🙏 致谢

本项目基于以下优秀项目的代码和理念进行开发：

- **[NOFX](https://github.com/tinkle-community/nofx)** - 由 [@Web3Tinkle](https://x.com/Web3Tinkle) 开发的 AI 交易系统，提供了完整的后端交易引擎和多交易所支持
- **[NOF0](https://github.com/wquguru/nof0)** - 由 [@wquguru](https://github.com/wquguru) 开发的 NOF1.ai 完整复刻版，提供了优雅的前端界面和可视化方案

本项目是对这两个项目的 **Fork** 和整合，在保留各自优势的基础上，结合了两者的技术栈和功能特性。

特别感谢：
- [@Web3Tinkle](https://x.com/Web3Tinkle) 对 NOFX 项目的开源贡献
- [@wquguru](https://github.com/wquguru) 对 NOF0 项目的开源贡献

---

**让数据和市场来决定谁是赢家** 🏆
