#!/usr/bin/env bash

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${PROJECT_ROOT}"

# 彩色输出
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

ensure_root() {
  if [[ "${EUID}" -ne 0 ]]; then
    error "请使用 root 权限运行：sudo ${BASH_SOURCE[0]}"
    exit 1
  fi
}

install_docker() {
  info "检测 Docker 环境..."
  if command_exists docker; then
    success "Docker 已安装"
    return
  fi

  warn "Docker 未安装，开始自动安装..."
  curl -fsSL https://get.docker.com -o /tmp/get-docker.sh
  sh /tmp/get-docker.sh
  rm -f /tmp/get-docker.sh

  systemctl enable docker >/dev/null 2>&1 || true
  systemctl start docker >/dev/null 2>&1 || true
  success "Docker 安装完成"
}

ensure_docker_compose() {
  if docker compose version >/dev/null 2>&1; then
    DOCKER_COMPOSE_CMD=(docker compose)
    success "检测到 docker compose 插件"
  elif command_exists docker-compose; then
    DOCKER_COMPOSE_CMD=(docker-compose)
    success "检测到 docker-compose 可执行文件"
  else
    warn "未检测到 docker compose，正在安装..."
    mkdir -p /usr/local/lib/docker/cli-plugins
    curl -SL "https://github.com/docker/compose/releases/download/v2.24.6/docker-compose-linux-$(uname -m)" \
      -o /usr/local/lib/docker/cli-plugins/docker-compose
    chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
    DOCKER_COMPOSE_CMD=(docker compose)
    success "docker compose 安装完成"
  fi
}

prepare_project() {
  info "准备项目文件和目录..."

  mkdir -p logs decision_logs data
  chmod 755 logs decision_logs data

  if [[ ! -f config.json ]]; then
    if [[ -f config.json.example ]]; then
      warn "未找到 config.json，从示例配置生成..."
      cp config.json.example config.json
      warn "⚠️  请编辑 config.json 配置你的 API 密钥！"
    else
      error "未找到 config.json 或 config.json.example"
      exit 1
    fi
  fi

  # 验证配置文件格式
  if command_exists python3; then
    if ! python3 -m json.tool config.json >/dev/null 2>&1; then
      error "config.json 格式错误，请检查 JSON 语法"
      exit 1
    fi
  elif command_exists jq; then
    if ! jq empty config.json >/dev/null 2>&1; then
      error "config.json 格式错误，请检查 JSON 语法"
      exit 1
    fi
  fi

  # 检查配置文件是否为空
  if [[ ! -s config.json ]]; then
    error "config.json 文件为空！"
    exit 1
  fi

  success "项目目录准备完成"
}

compose_up() {
  info "开始构建并启动容器..."

  "${DOCKER_COMPOSE_CMD[@]}" pull || warn "跳过镜像拉取（使用本地构建）"
  "${DOCKER_COMPOSE_CMD[@]}" build --pull
  "${DOCKER_COMPOSE_CMD[@]}" up -d --remove-orphans

  success "容器启动完成"
  echo
  info "等待服务就绪（5秒）..."
  sleep 5
  
  info "当前服务状态："
  "${DOCKER_COMPOSE_CMD[@]}" ps
  
  echo
  info "查看日志："
  echo "  ${CYAN}sudo ./starts.sh logs${NC}"
  echo
  info "检查后端健康："
  echo "  ${CYAN}curl http://localhost:8080/health${NC}"
}

display_usage() {
  cat <<'EOF'
用法: sudo ./starts.sh [命令]

可选命令：
  up        安装依赖并启动 Docker 服务（默认）
  down      停止并移除容器
  restart   重启所有容器
  logs      查看前后端日志（跟随模式）
  ps        查看容器状态
EOF
}

handle_down() {
  info "停止并移除容器..."
  "${DOCKER_COMPOSE_CMD[@]}" down
  success "容器已停止"
}

handle_restart() {
  info "重启容器..."
  "${DOCKER_COMPOSE_CMD[@]}" down
  "${DOCKER_COMPOSE_CMD[@]}" up -d --remove-orphans
  success "容器已重启"
}

handle_logs() {
  info "展示容器日志（Ctrl+C 退出）..."
  "${DOCKER_COMPOSE_CMD[@]}" logs -f
}

handle_status() {
  "${DOCKER_COMPOSE_CMD[@]}" ps
}

main() {
  ensure_root
  install_docker
  ensure_docker_compose
  prepare_project

  local cmd=${1:-up}
  case "$cmd" in
    up)
      compose_up
      ;;
    down)
      handle_down
      ;;
    restart)
      handle_restart
      ;;
    logs)
      handle_logs
      ;;
    ps)
      handle_status
      ;;
    -h|--help|help)
      display_usage
      ;;
    *)
      error "未知命令: $cmd"
      display_usage
      exit 1
      ;;
  esac
}

main "$@"
