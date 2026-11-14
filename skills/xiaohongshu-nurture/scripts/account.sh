#!/usr/bin/env bash
# 无后台时管理养号账号。
# 用法:
#   account.sh list
#   account.sh stage    <id> <0-4>
#   account.sh persona  <id> <text>
#   account.sh keywords <id> '<json数组>'
#   account.sh status   <id> <active|paused>
#   account.sh usage    <id>
# 环境变量: PROJECT_DIR, DB(默认 $PROJECT_DIR/data/keeper.db)
set -euo pipefail
PROJECT_DIR="${PROJECT_DIR:-/Users/zhaoruizhe/Desktop/project/P/小红书养号/ref/xiaohongshu-mcp}"
DB="${DB:-$PROJECT_DIR/data/keeper.db}"
command -v sqlite3 >/dev/null 2>&1 || { echo "需要 sqlite3" >&2; exit 1; }
[[ -f "$DB" ]] || { echo "数据库不存在: $DB（先跑一次 keeper 生成）" >&2; exit 1; }
cmd="${1:-list}"; shift || true
case "$cmd" in
  list)     sqlite3 -header -column "$DB" "SELECT id,stage,status,persona,keywords FROM accounts;" ;;
  stage)    sqlite3 "$DB" "UPDATE accounts SET stage=$2, updated_at=strftime('%s','now') WHERE id='$1';"; echo "ok: $1 stage=$2" ;;
  persona)  sqlite3 "$DB" "UPDATE accounts SET persona='$2', updated_at=strftime('%s','now') WHERE id='$1';"; echo "ok: $1 persona 更新" ;;
  keywords) sqlite3 "$DB" "UPDATE accounts SET keywords='$2', updated_at=strftime('%s','now') WHERE id='$1';"; echo "ok: $1 keywords 更新" ;;
  status)   sqlite3 "$DB" "UPDATE accounts SET status='$2', updated_at=strftime('%s','now') WHERE id='$1';"; echo "ok: $1 status=$2" ;;
  usage)    sqlite3 -header -column "$DB" "SELECT * FROM daily_usage WHERE account_id='$1' ORDER BY day DESC LIMIT 3;" ;;
  *) echo "未知命令: $cmd" >&2; exit 2 ;;
esac
