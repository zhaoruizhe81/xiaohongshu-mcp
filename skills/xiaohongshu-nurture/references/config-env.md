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


## LLM 本地配置（.env，推荐）
工程根目录放 `.env`（已 gitignore），keeper 启动自动加载，**真实环境变量优先**。
兼容两套命名：`LLM_*` 优先，回退标准 `OPENAI_BASE_URL / OPENAI_API_KEY / OPENAI_MODEL`。
```
OPENAI_BASE_URL=http://127.0.0.1:3425/v1
OPENAI_API_KEY=sk-xxxx
LLM_MODEL=zhipu/glm-5.3-flash
LLM_TEMPERATURE=0.85
```
诊断连通：`go run ./cmd/keeper -llmcheck`（只调 LLM 后退出，不开浏览器）。
视觉需选**多模态**型号（如 GLM 系）；不支持 image_url 会自动退化纯文本。

## 看评论行为参数（可选，环境变量/.env 覆盖）
| 变量 | 含义 | 默认 |
|---|---|---|
| `XHS_CR_OPEN_PROB` | 看笔记后翻评论的概率 0~1 | 0.7 |
| `XHS_CR_OPEN_SCREENS_MIN/MAX` | 看笔记后翻看屏数范围 | 2 / 5 |
| `XHS_CR_COMMENT_PROB` | 评论前翻评论概率 0~1 | 0.6 |
| `XHS_CR_COMMENT_SCREENS_MIN/MAX` | 评论前翻看屏数范围 | 1 / 3 |
| `XHS_CR_DWELL_MIN/MAX` | 每屏停留秒数范围 | 1 / 4 |

启动时会打印实际生效值。改 `.env` 即可调整"像不像人"的翻评论强度。
