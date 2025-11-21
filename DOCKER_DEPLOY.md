# Docker 部署指南

本指南帮助你使用 Docker Compose 同时运行 Go 后端与 Next.js 前端。

## 1. 准备工作

1. 安装 Docker 与 Docker Compose（Docker Desktop 已自带 Compose）。
2. 在项目根目录准备好生产配置 `config.json`，其中包含交易所密钥、AI 模型等敏感信息。此文件会在运行时以只读方式挂载到容器中。
3. 如需自定义端口、配置文件或公开变量，可在项目根目录创建 `.env`，例如：
   ```bash
   # 后端
   CONFIG_FILE=./config.prod.json
   BACKEND_PORT=8080
   TZ=Asia/Shanghai

   # 前端
   FRONTEND_PORT=3000
   NOF1_API_BASE_URL=http://backend:8080
   NEXT_PUBLIC_NOF1_API_BASE_URL=/api/nof1
   NEXT_PUBLIC_TICKER_PAIRS=BTCUSDT,ETHUSDT,SOLUSDT
   ```
   Compose 会自动加载 `.env`，同时仍可在启动命令前 `export` 临时覆盖。

## 2. 构建镜像

在项目根目录执行：
```bash
docker compose build
```
这会分别构建：
- `jiaoyibot-backend`：基于 Go 1.22 的无状态二进制镜像。
- `jiaoyibot-frontend`：基于 Node 20 的 Next.js 16 应用（standalone 打包）。

## 3. 启动服务

```bash
docker compose up -d
```
默认会开放：
- 后端 API：`http://localhost:8080`
- 前端界面：`http://localhost:3000`

前端容器通过环境变量 `NOF1_API_BASE_URL=http://backend:8080` 调用后端；浏览器端请求统一走 `/api/nof1/*`，由 Next.js 服务端聚合并代理。

## 4. 持久化与日志

Compose 文件默认把宿主机的 `decision_logs/` 和 `logs/` 映射到容器内，以便保留策略输出与运行日志。根据需要可以修改或添加卷挂载。

## 5. 停止与重启

```bash
docker compose down        # 停止并移除容器

# 或者只重启单个服务
docker compose restart frontend
```

## 6. 自定义

- **配置文件路径**：如需使用不同名称，可在 `docker-compose.yml` 的 `volumes` 中调整挂载路径，或在启动前设置 `CONFIG_PATH`。
- **环境变量**：可以在 `docker-compose.yml` 中新增/覆盖变量，例如 Binance 行情条的 `NEXT_PUBLIC_TICKER_PAIRS`。
- **反向代理**：若要通过 Nginx/Traefik 暴露 80/443 端口，只需把代理指向前端容器 `3000` 端口，同时继续保留 `/api/nof1/*` 路径。

## 7. 常见问题

| 问题 | 解决方案 |
| ---- | -------- |
| 前端页面加载但无数据 | 确认后端 `config.json` 填写无误且策略正常启动；检查 `docker compose logs backend`。 |
| 浏览器请求返回 502 | 多数为后端未启动或 `NOF1_API_BASE_URL` 配置错误，确认 Compose 环境变量。 |
| 构建缓慢 | 首次构建会安装所有 Node/Golang 依赖；后续修改代码可使用 `docker compose build frontend` 或 `backend` 单独重建。 |
