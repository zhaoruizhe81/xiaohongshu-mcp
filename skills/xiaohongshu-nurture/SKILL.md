---
name: xiaohongshu-nurture
description: 操作并编排「小红书养号引擎」——本仓库 ref/xiaohongshu-mcp 里的 Go keeper（有头浏览器拟人 + 阶段行为 + 多模态 LLM 评论 + SQLite + 小后台 + 调度）。当用户要：跑养号/编排养号流程、扫码登录、设置账号阶段(观察/轻互动/深化/社交)、填人设与关键词、跑一场或常驻自动养、查看养号状态与当日配额、配置 LLM 评论、真机校准、排查限流/验证/封号 时使用。
---

# 小红书养号引擎（操作手册）

本 skill 让 agent 操作已搭好的养号引擎：一个 Go 程序，用**有头 Chromium** 模拟真人，在小红书网页端按**阶段**做浏览/点赞/收藏/评论，带 SQLite 记账、小 Web 后台、守护调度、多模态 LLM 评论。

## 0. 定位工程
引擎在仓库 `ref/xiaohongshu-mcp`（默认 `/Users/zhaoruizhe/Desktop/project/P/小红书养号/ref/xiaohongshu-mcp`）。先确认存在 `go.mod` 与 `cmd/keeper/`；找不到就问用户路径。所有命令都在该目录下执行。

## 1. 前置
- Go 1.27+（`go version`）。
- 有头环境（本机 macOS 会弹 Chromium 窗口；Linux 服务器需 Xvfb）。
- 已扫码登录生成 `cookies.json`（`go run ./cmd/login`，或原 MCP 的 `/api/v1/login/qrcode`）。

## 2. 运行
```bash
cd <工程目录>
go run ./cmd/keeper -once    # 单场调试：跑完即退
go run ./cmd/keeper          # 常驻：活跃时段自动开场 + 小后台
```
单场编排参数（配合 `-once`）：
```bash
go run ./cmd/keeper -once -account demo -stage 3 \
  -persona "20岁女生，轻松口语化" -keywords '["美食","穿搭"]' -minutes 6
```
- `-stage 0..4` 本次设置阶段；`-minutes N` 调试用限时（0=按节奏随机）。
- `-behavior like|comment|open_note|browse_feed|...` 只跑单个行为（调试/验收用，会先刷少量填队列）。
- `-llmcheck` 只测 LLM 连通后退出。
- 后台默认 `http://127.0.0.1:18061`（`ADMIN_ADDR` 可改）。
- 仅做只读采集才 `-headless=true`；养号务必有头。

## 3. 账号配置（人管阶段）
- 有后台：浏览器打开后台改 **阶段 / 语言风格 persona / 关键词JSON / 启用暂停 / 立即跑一场**。
- 无后台：用 `scripts/account.sh` 或 sqlite 直接改（见 references/config-env.md）。

## 4. 阶段模型
`0 观察`(D1-2 只看不互动) → `1 轻互动`(D3-5 点赞) → `2 深化`(D6-10 收藏/逛主页) → `3 社交`(D11+ 评论/通知) → `4 创作`(发布，未实现)。
阶段由**人**推进；权重/日配额见 references/stage-behavior.md。

## 5. LLM 评论（多模态，默认直发）
```bash
export LLM_BASE_URL=https://api.openai.com/v1   # 或 DeepSeek/通义/豆包兼容地址
export LLM_MODEL=<多模态模型名>
export LLM_API_KEY=sk-...
```
配置齐全才评论（仅社交阶段）；未配置自动跳过、其余照跑。图文取图/视频取首帧 + 语言风格 + 随机角度 + 校验闸(长度/无联系方式/去重)。

## 6. 监控与熔断
- 每步先查风控页：安全验证→冷却24h；登录失效→暂停；其它异常→冷却2h。
- 查状态：`scripts/account.sh list`；审计看库表 `sessions / interactions / risk_events`。

## 7. 真机首次校准清单
- 首页懒加载是否随滚轮触发；feed 卡片选择器（若要"点开卡片"）。
- 图片 CDN 带 `Referer` 能否取到；所用模型是否接受 `image_url`。
- `-once` 跑一遍观察：有头窗口正常、刷 feed/停留/点赞符合预期。

## 8. 排障
- 登录态无效 → 重扫码，确认 `cookies.json`。
- 频繁超时/验证 → 降频、换住宅IP、冷却后再跑。
- LLM 报错 → 查 `LLM_BASE_URL/MODEL/API_KEY`；失败会退化纯文本，不会崩。

## 参考文件
- `references/architecture.md` — 模块地图、数据流、DB 表、关键文件。
- `references/stage-behavior.md` — 阶段权重/日配额/节奏/行为清单/刷feed算法。
- `references/config-env.md` — 全部环境变量、后台接口、sqlite 常用命令。
- `scripts/run-keeper.sh` — 一键构建并运行（once/serve）。
- `scripts/account.sh` — 无后台时改账号阶段/风格/关键词/状态。
