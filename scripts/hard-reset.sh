#!/usr/bin/env bash

# ============================================================================
# 极限重置脚本
# - 清空指定持久化数据
# - 彻底移除所有 Docker 容器 / 镜像 / 网络 / 数据卷 / 构建缓存
# 使用前请务必确认需要完全清空历史记录与 Docker 资源！
# ============================================================================

set -euo pipefail

# ---------------------------- 可调整目标路径 ----------------------------
REPO_DATA_PATHS=(
  "/Users/dalang/Downloads/jiaoyibot_wss-main/decision_logs"
  "/root/jiaoyibot_wss-main/data/learning_state"
  "/root/jiaoyibot_wss-main/data/position_history/binance_deepseek.json"
)

# ---------------------------- 辅助函数 ----------------------------
info() {
  printf "\n\033[34m[INFO]\033[0m %s\n" "$*"
}

warn() {
  printf "\n\033[33m[WARN]\033[0m %s\n" "$*"
}

die() {
  printf "\n\033[31m[ERROR]\033[0m %s\n" "$*"
  exit 1
}

# ---------------------------- 确认提示 ----------------------------
cat <<'EOF'
=============================================================
 即将执行以下操作：
 1. 删除指定目录 / 文件中的历史数据
 2. 停止并删除所有 Docker 容器
 3. 删除所有 Docker 镜像
 4. 清理自定义 Docker 网络
 5. 删除全部 Docker 数据卷
 6. 清除 Docker 构建缓存与系统残留

 该操作不可逆，请确保已备份重要数据！
=============================================================
EOF

read -rp "确认继续？(yes/NO): " CONFIRM
if [[ "${CONFIRM}" != "yes" ]]; then
  warn "已取消执行。"
  exit 0
fi

# ---------------------------- 删除历史数据 ----------------------------
info "开始清理项目历史数据..."
for target in "${REPO_DATA_PATHS[@]}"; do
  if [[ -e "${target}" ]]; then
    info "删除 ${target}"
    rm -rf "${target}"
  else
    info "跳过（不存在）：${target}"
  fi
done

# ---------------------------- Docker 清理 ----------------------------
info "停止所有容器..."
docker stop "$(docker ps -aq)" 2>/dev/null || info "无运行中的容器。"

info "删除所有容器..."
docker rm "$(docker ps -aq)" 2>/dev/null || info "无容器可删除。"

info "删除所有镜像..."
docker rmi -f "$(docker images -aq)" 2>/dev/null || info "无镜像可删除。"

info "清理自定义网络..."
docker network prune -f

info "清理数据卷..."
docker volume prune -f

info "清理构建缓存..."
docker builder prune -af

info "执行系统级清理（包含未引用的卷）..."
docker system prune -af --volumes

# ---------------------------- 完成 ----------------------------
info "重置完成。"
info "建议重新构建 / 启动所需的 Docker 资源。"

