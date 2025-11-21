#!/usr/bin/env bash
set -euo pipefail

# 用法：scripts/archive-logs.sh <trader_id>
# 功能：
#   1. 将指定 Trader 的决策日志与学习/成交数据打包到 archives/
#   2. 打包成功后删除原始文件，方便系统从零重新积累历史

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TRADER_ID="${1:-}"

if [[ -z "${TRADER_ID}" ]]; then
  echo "用法: $0 <trader_id>" >&2
  exit 1
fi

TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
ARCHIVE_DIR="${ROOT_DIR}/archives"
mkdir -p "${ARCHIVE_DIR}"

log_dir="${ROOT_DIR}/decision_logs/${TRADER_ID}"
learning_file="${ROOT_DIR}/data/learning_state/${TRADER_ID}.json"
exchange_file="${ROOT_DIR}/data/exchange_trades/${TRADER_ID}.json"
position_file="${ROOT_DIR}/data/position_history/${TRADER_ID}.json"
trade_dir="${ROOT_DIR}/data/trade_logs/${TRADER_ID}"

if [[ ! -d "${log_dir}" && ! -f "${learning_file}" && ! -f "${exchange_file}" && ! -f "${position_file}" && ! -d "${trade_dir}" ]]; then
  echo "未找到任何与 ${TRADER_ID} 相关的日志或学习数据" >&2
  exit 2
fi

ARCHIVE_PATH="${ARCHIVE_DIR}/${TRADER_ID}_${TIMESTAMP}.tar.gz"

pushd "${ROOT_DIR}" >/dev/null

tar -czf "${ARCHIVE_PATH}" \
  $( [[ -d "decision_logs/${TRADER_ID}" ]] && echo "decision_logs/${TRADER_ID}" ) \
  $( [[ -f "data/learning_state/${TRADER_ID}.json" ]] && echo "data/learning_state/${TRADER_ID}.json" ) \
  $( [[ -f "data/exchange_trades/${TRADER_ID}.json" ]] && echo "data/exchange_trades/${TRADER_ID}.json" ) \
  $( [[ -f "data/position_history/${TRADER_ID}.json" ]] && echo "data/position_history/${TRADER_ID}.json" ) \
  $( [[ -d "data/trade_logs/${TRADER_ID}" ]] && echo "data/trade_logs/${TRADER_ID}" )

popd >/dev/null

echo "已归档至 ${ARCHIVE_PATH}"

# 删除原始文件，保留空目录防止后续写入失败
if [[ -d "${log_dir}" ]]; then
  rm -rf "${log_dir}"
  mkdir -p "${log_dir}"
fi

rm -f "${learning_file}" "${exchange_file}" "${position_file}"

if [[ -d "${trade_dir}" ]]; then
  rm -rf "${trade_dir}"
fi

echo "原始日志与学习数据已清理。请重启后端让策略重新积累历史。"

