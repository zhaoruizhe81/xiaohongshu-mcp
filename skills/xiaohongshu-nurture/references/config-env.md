# 环境变量 · 后台接口 · sqlite 操作

## 环境变量
| 变量 | 作用 | 默认 |
|---|---|---|
| `COOKIES_PATH` | 登录 cookies 文件 | 工程内 `cookies.json` |
| `XHS_FP_SEED` | 固定浏览器指纹 seed（不设则按会话文件/新生成） | 随机 |
| `XHS_PROXY` | 代理（http/https/socks5），一号一 IP 时用 | 无 |
| `ADMIN_ADDR` | 小后台监听地址 | `:18061` |
| `ADMIN_TOKEN` | 后台鉴权 token（设了才校验 Bearer 或 ?token=） | 空=不鉴权 |
| `LLM_BASE_URL` | OpenAI 兼容接口 | `https://api.openai.com/v1` |
| `LLM_MODEL` | 多模态模型名（缺则不评论） | 空 |
| `LLM_API_KEY` | 密钥（缺则不评论） | 空 |
| `LLM_TEMPERATURE` | 生成温度 | `0.85` |

## 小后台接口（Gin）
- `GET /` 首页（账号表：改阶段/persona/关键词/启停/立即跑）。
- `GET /api/accounts` 账号 JSON。
- `POST /accounts/:id/stage`（表单 `stage=0..4`）
- `POST /accounts/:id/persona`（`persona=`）
- `POST /accounts/:id/keywords`（`keywords=["美食","穿搭"]`）
- `POST /accounts/:id/status`（`status=active|paused`）
- `POST /accounts/:id/run`（触发调度器立即开跑）
curl 示例（无 token 时）：
```bash
curl -X POST http://127.0.0.1:18061/accounts/demo/stage -d stage=1
curl -X POST http://127.0.0.1:18061/accounts/demo/run
```

## sqlite 常用（无后台时）
```bash
DB=data/keeper.db
sqlite3 -header -column "$DB" "SELECT id,stage,status,persona,keywords FROM accounts;"
sqlite3 "$DB" "UPDATE accounts SET stage=1 WHERE id='demo';"
sqlite3 "$DB" "UPDATE accounts SET persona='20岁女生，轻松口语化' WHERE id='demo';"
sqlite3 "$DB" "UPDATE accounts SET keywords='[\"美食\",\"穿搭\"]' WHERE id='demo';"
sqlite3 "$DB" "UPDATE accounts SET status='paused' WHERE id='demo';"
# 今日配额/会话
sqlite3 -header -column "$DB" "SELECT * FROM daily_usage ORDER BY day DESC LIMIT 5;"
sqlite3 -header -column "$DB" "SELECT id,started_at,like_cnt,fav_cnt,comment_cnt,outcome FROM sessions ORDER BY id DESC LIMIT 10;"
sqlite3 -header -column "$DB" "SELECT * FROM risk_events ORDER BY id DESC LIMIT 10;"
```

## 登录 / 换号
- 登录：`go run ./cmd/login`（扫码，生成 cookies.json）。
- 换号：备份/替换 `cookies.json`（对应 `COOKIES_PATH`）；多号需给每个号单独 cookie+seed+proxy（`browser.NewBrowser` 需加 `WithCookiesPath`）。
