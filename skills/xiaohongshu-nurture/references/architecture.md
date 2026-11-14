# 架构与数据流

## 分层
```
cmd/keeper  养号主程序：调度循环 + 小后台(goroutine) + LLM 注入
 ├ session.go   Session：常驻 browser+page + 动作封装(Like/Favorite/Comment/Search/Detail/Profile/NotifUnread) + safe/Healthy + queue/seen
 ├ browse.go    BrowseFeed：真滚动，刷 N 条就 reload
 ├ behavior.go  Stage/权重/日配额 + 9 个 Behavior + noteWatch + pickKeyword
 ├ rhythm.go    活跃时段 / 会话时长 / 动作间隔采样
 ├ engine.go    RunSession：抽样执行行为 + 熔断(block) + 记账
 ├ scheduler.go 守护调度：活跃时段自动开场 + 手动触发通道
 └ main.go      组装；-once 单场；否则常驻
internal/
 ├ store/   SQLite schema + DAO（账号/配额/互动/评论/风险/会话）
 ├ llm/     OpenAI 兼容 chat 客户端（多模态）
 ├ commentgen/ 评论生成：取图→prompt→校验闸→去重
 └ admin/   Gin 小后台（阶段/persona/关键词/启停/立即跑）
```
底层复用 `ref/xiaohongshu-mcp/xiaohongshu`（动作层）、`humanize`（拟人时序/鼠标/点击/输入）、`browser`（内置指纹 Chromium）。

## 一次会话的数据流
```
RunSession(acc)
  → OpenSession()  读 cookies/指纹seed/代理，开有头浏览器，校验登录
  → load daily_usage
  → loop(until deadline):
       Healthy() 若命中验证/登录页 → block() 冷却并结束
       pick(按阶段权重抽样, 配额满的权重=0)
       safe(b.Run)  动作出错软失败→risk_score+1；硬失败→block
       sleep(随机间隔)
  → EndSession 记账；sessions/risk_events 落库
```

## SQLite 表（data/keeper.db）
- `accounts(id,nickname,red_id,first_login_at,stage,status,cooldown_until,risk_score,keywords,persona,...)`
- `daily_usage(account_id,day,used_like,used_favorite,used_comment,used_search,sessions)`
- `interactions(...action,note_id,...) UNIQUE(account_id,note_id,action)` 去重审计
- `comment_jobs(... status pending/approved/rejected/posted)`（审核模式备用）
- `risk_events(account_id,type,detail,created_at)`
- `sessions(account_id,started_at,ended_at,browse_cnt,open_cnt,like_cnt,fav_cnt,comment_cnt,outcome)`

## 关键文件（排障入口）
- 浏览器/指纹/代理注入：`browser/browser.go`、`configs/seed.go`、`configs/browser.go`
- 动作与选择器：`xiaohongshu/*.go`（like_favorite / comment_feed / feed_detail / search / login / page_check）
- 拟人时序表：`humanize/provider.go`；鼠标/点击：`humanize/mouse.go`、`humanize/input.go`
- 本 skill 的养号逻辑：`cmd/keeper/*.go`、`internal/*`
