#!/usr/bin/env bash

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${PROJECT_ROOT}"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

info() {
  echo -e "${CYAN}[信息]${NC} $1"
}

success() {
  echo -e "${GREEN}[成功]${NC} $1"
}

warn() {
  echo -e "${YELLOW}[警告]${NC} $1"
}

error() {
  echo -e "${RED}[错误]${NC} $1" >&2
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

ensure_docker() {
  if ! command_exists docker; then
    error "未检测到 docker，请先安装 Docker Desktop 或 docker-ce。"
    exit 1
  fi
  if ! docker info >/dev/null 2>&1; then
    error "docker 守护进程不可用，请确认已启动且当前用户有权限。"
    exit 1
  fi
}

ensure_compose() {
  if docker compose version >/dev/null 2>&1; then
    DOCKER_COMPOSE_CMD=(docker compose)
    return
  fi
  if command_exists docker-compose; then
    DOCKER_COMPOSE_CMD=(docker-compose)
    return
  fi
  error "未找到 docker compose，请安装 Docker Compose v2（推荐）或 docker-compose。"
  exit 1
}

prepare_project() {
  info "检测项目目录与配置..."

  mkdir -p logs decision_logs data
  chmod 755 logs decision_logs data

  if [[ ! -f config.json ]]; then
    if [[ -f config.json.example ]]; then
      warn "首次运行：复制 config.json.example -> config.json"
      cp config.json.example config.json
      warn "⚠️  请编辑 config.json，填入真实的 API Key/策略参数。"
    else
      error "缺少 config.json，请先准备配置。"
      exit 1
    fi
  fi

  if [[ ! -s config.json ]]; then
    error "config.json 为空，请先写入内容。"
    exit 1
  fi

  if command_exists python3; then
    if ! python3 -m json.tool config.json >/dev/null 2>&1; then
      error "config.json JSON 校验失败，请检查语法。"
      exit 1
    fi
  elif command_exists jq; then
    if ! jq empty config.json >/dev/null 2>&1; then
      error "config.json JSON 校验失败，请检查语法。"
      exit 1
    fi
  fi
}

load_env_if_exists() {
  if [[ -f .env ]]; then
    info "加载 .env"
    set -a
    # shellcheck disable=SC1091
    source .env
    set +a
  fi
  export NEXT_PUBLIC_TICKER_PAIRS="${NEXT_PUBLIC_TICKER_PAIRS:-BTCUSDT,BTCUSDC,ETHUSDT,ETHUSDC,SOLUSDT}"
}

compose_up() {
  info "启动 Docker 服务..."
  "${DOCKER_COMPOSE_CMD[@]}" pull || warn "拉取镜像失败，使用本地缓存"
  "${DOCKER_COMPOSE_CMD[@]}" build --pull
  "${DOCKER_COMPOSE_CMD[@]}" up -d --remove-orphans
  success "容器启动完成。"
  "${DOCKER_COMPOSE_CMD[@]}" ps
}

compose_down() {
  info "停止并移除容器..."
  "${DOCKER_COMPOSE_CMD[@]}" down
  success "容器已停止。"
}

compose_restart() {
  info "重启容器..."
  "${DOCKER_COMPOSE_CMD[@]}" down
  "${DOCKER_COMPOSE_CMD[@]}" up -d --remove-orphans
  success "容器已重启。"
}

compose_logs() {
  info "实时日志 (Ctrl+C 退出)..."
  "${DOCKER_COMPOSE_CMD[@]}" logs -f
}

compose_ps() {
  "${DOCKER_COMPOSE_CMD[@]}" ps
}

usage() {
  cat <<'EOF'
用法: ./startsd.sh [命令]

可选命令：
  up        构建并启动服务（默认）
  down      停止并移除容器
  restart   重启容器
  logs      跟随输出日志
  ps        查看容器状态
  help      显示本帮助
EOF
}

main() {
  ensure_docker
  ensure_compose
  prepare_project
  load_env_if_exists

  local cmd=${1:-up}
  case "$cmd" in
    up)
      compose_up
      ;;
    down)
      compose_down
      ;;
    restart)
      compose_restart
      ;;
    logs)
      compose_logs
      ;;
    ps|status)
      compose_ps
      ;;
    -h|--help|help)
      usage
      ;;
    *)
      error "未知命令: $cmd"
      usage
      exit 1
      ;;
  esac
}

main "$@"
