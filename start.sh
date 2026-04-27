#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
ENV_FILE="$ROOT_DIR/.env"
FRONTEND_DIR="$ROOT_DIR/frontend"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
ORANGE='\033[0;33m'
NC='\033[0m'

info()  { echo -e "${ORANGE}🍊 $1${NC}"; }
ok()    { echo -e "${GREEN}✓ $1${NC}"; }
warn()  { echo -e "${YELLOW}⚠ $1${NC}"; }
err()   { echo -e "${RED}✗ $1${NC}"; }

check_cmd() {
  if command -v "$1" &>/dev/null; then
    ok "$1 已安装"
    return 0
  else
    err "$1 未安装，请先安装后重试"
    return 1
  fi
}

info "project-helper 一键启动脚本"
echo ""

MISSING=0
check_cmd go || MISSING=1
check_cmd node || MISSING=1
check_cmd npm || MISSING=1

if [ "$MISSING" -eq 1 ]; then
  echo ""
  err "缺少必要依赖，请安装后重新运行"
  exit 1
fi

echo ""

if [ ! -f "$ENV_FILE" ]; then
  warn ".env 文件不存在，正在从 .env-example 创建..."
  cp "$ROOT_DIR/.env-example" "$ENV_FILE"
  warn "请编辑 .env 填入你的 DEEPSEEK_API_KEY 后重新运行"
  echo ""
  echo "  ${YELLOW}vim $ENV_FILE${NC}"
  echo ""
  exit 1
fi

if grep -q 'your_api_key' "$ENV_FILE" 2>/dev/null; then
  warn ".env 中 DEEPSEEK_API_KEY 仍为占位值，请填入真实 Key"
  echo ""
  echo "  ${YELLOW}vim $ENV_FILE${NC}"
  echo ""
  exit 1
fi

ok ".env 配置检查通过"

if [ ! -d "$FRONTEND_DIR/node_modules" ]; then
  info "正在安装前端依赖..."
  (cd "$FRONTEND_DIR" && npm install)
  ok "前端依赖安装完成"
else
  ok "前端依赖已存在，跳过安装"
fi

info "正在启动后端服务..."
(cd "$ROOT_DIR" && go run ./cmd/server) &
BACKEND_PID=$!
ok "后端服务已启动 (PID: $BACKEND_PID)"

info "正在启动前端开发服务器..."
(cd "$FRONTEND_DIR" && npm run dev) &
FRONTEND_PID=$!
ok "前端服务已启动 (PID: $FRONTEND_PID)"

cleanup() {
  echo ""
  info "正在停止服务..."
  kill "$BACKEND_PID" 2>/dev/null || true
  kill "$FRONTEND_PID" 2>/dev/null || true
  ok "服务已停止"
  exit 0
}

trap cleanup SIGINT SIGTERM

echo ""
ok "所有服务已启动！"
echo ""
echo "  后端:  ${GREEN}http://localhost:8080${NC}"
echo "  前端:  ${GREEN}http://localhost:5173${NC}"
echo ""
info "按 Ctrl+C 停止所有服务"
echo ""

wait
