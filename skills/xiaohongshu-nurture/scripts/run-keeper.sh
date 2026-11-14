#!/usr/bin/env bash
# 构建并启动小红书养号引擎。
# 用法: run-keeper.sh [once|serve]   once=跑一场后退出; serve=常驻(调度+后台, 默认)
# 环境变量: PROJECT_DIR(工程目录), GOFLAGS
set -euo pipefail
MODE="${1:-serve}"
PROJECT_DIR="${PROJECT_DIR:-/Users/zhaoruizhe/Desktop/project/P/小红书养号/ref/xiaohongshu-mcp}"
if [[ ! -f "$PROJECT_DIR/go.mod" ]]; then
  echo "找不到工程目录: $PROJECT_DIR (用 PROJECT_DIR 指定含 go.mod 的 ref/xiaohongshu-mcp)" >&2
  exit 1
fi
command -v go >/dev/null 2>&1 || { echo "需要 Go 工具链" >&2; exit 1; }
cd "$PROJECT_DIR"
mkdir -p bin
echo "构建中 ..."
GOFLAGS="${GOFLAGS:--mod=mod}" go build -o bin/keeper ./cmd/keeper
case "$MODE" in
  once)  exec ./bin/keeper -once ;;
  serve) exec ./bin/keeper ;;
  *) echo "未知模式: $MODE (once|serve)" >&2; exit 2 ;;
esac
