package main

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/sirupsen/logrus"
	xerrs "github.com/xpzouying/xiaohongshu-mcp/errors"
	"github.com/xpzouying/xiaohongshu-mcp/internal/store"
)

// Engine 养号执行引擎：驱动一场会话的行为循环。
type Engine struct {
	st            *store.Store
	Commenter     Commenter          // P4 注入；nil 则不做评论
	Duration      time.Duration      // >0 时覆盖单场时长（调试用；0=按节奏随机）
	ForceBehavior string             // 非空时只跑该行为（调试/编排用）
	Read          ReadCommentsConfig // 看评论参数
}

func NewEngine(st *store.Store) *Engine {
	return &Engine{st: st, Read: loadReadCommentsConfig()}
}

// RunSession 跑一场养号会话：开常驻浏览器 → 按阶段权重抽样执行行为 → 记录。
func (e *Engine) RunSession(acc *store.Account) error {
	now := time.Now()
	if now.Before(acc.CooldownUntil) {
		logrus.Infof("账号 %s 冷却中(至 %s)，跳过", acc.ID, acc.CooldownUntil.Format("15:04"))
		return nil
	}

	s, err := OpenSession()
	if err != nil {
		return err
	}
	defer s.Close()

	day := now.Format("2006-01-02")
	usage, err := e.st.GetDaily(acc.ID, day)
	if err != nil {
		return err
	}
	stats := &SessionStats{}
	bctx := &BehaviorCtx{
		Ctx: context.Background(), S: s, St: e.st, Acc: acc, Usage: usage,
		Day: day, Stats: stats, Commenter: e.Commenter, Persona: acc.Persona, Read: e.Read,
	}

	sid, _ := e.st.StartSession(acc.ID)
	_ = e.st.IncrUsage(acc.ID, day, "sessions", 1) // 计入当日场次
	dur := SampleSessionDuration()
	if e.Duration > 0 {
		dur = e.Duration
	}
	deadline := now.Add(dur)
	outcome := "done"
	logrus.Infof("账号 %s 开始会话(阶段=%s 时长≈%s)", acc.ID, Stage(acc.Stage), time.Until(deadline).Round(time.Minute))

	// 强制行为模式：先刷少量填队列，再只跑指定行为
	if e.ForceBehavior != "" {
		logrus.Infof("强制行为模式: %s（先刷少量填队列）", e.ForceBehavior)
		_ = s.safe(func() error { return s.BrowseFeed(bctx.Ctx, 12, 1) })
		logrus.Infof("预填完成，队列 %d 条", len(s.queue))
	}

	for time.Now().Before(deadline) {
		// 每步先做健康检查，命中验证页/登录页立即熔断
		if err := s.Healthy(); err != nil {
			e.block(acc, err)
			outcome = "blocked"
			break
		}

		b := e.pick(bctx)
		if b == nil {
			break
		}
		if err := s.safe(func() error { return b.Run(bctx) }); err != nil {
			if isHardBlock(err) {
				e.block(acc, err)
				outcome = "blocked"
				break
			}
			logrus.Warnf("行为 %s 出错(软): %v", b.Key(), err)
			_, _ = e.st.AddRiskScore(acc.ID, 1)
		}
		sleepCtx(bctx.Ctx, SampleGap())
	}

	_ = e.st.EndSession(sid, stats.Browse, stats.Open, stats.Like, stats.Favorite, stats.Comment, outcome)
	logrus.Infof("会话结束: browse=%d open=%d like=%d fav=%d comment=%d outcome=%s",
		stats.Browse, stats.Open, stats.Like, stats.Favorite, stats.Comment, outcome)
	return nil
}

// pick 按行为权重抽样；权重全 0 返回 nil（会话提前结束）。
func (e *Engine) pick(c *BehaviorCtx) Behavior {
	if e.ForceBehavior != "" {
		for _, b := range allBehaviors() {
			if b.Key() == e.ForceBehavior && b.Weight(c) > 0 {
				return b
			}
		}
		return nil
	}
	behaviors := allBehaviors()
	weights := make([]float64, len(behaviors))
	total := 0.0
	for i, b := range behaviors {
		w := b.Weight(c)
		weights[i] = w
		total += w
	}
	if total <= 0 {
		return nil
	}
	r := rand.Float64() * total
	for i, w := range weights {
		if r < w {
			return behaviors[i]
		}
		r -= w
	}
	return behaviors[len(behaviors)-1]
}

// block 熔断：按错误类型决定冷却/暂停。
func (e *Engine) block(acc *store.Account, err error) {
	_ = e.st.AddRiskEvent(acc.ID, "block", err.Error())
	switch {
	case errors.Is(err, xerrs.ErrVerificationRequired):
		_ = e.st.SetCooldown(acc.ID, time.Now().Add(24*time.Hour))
		_ = e.st.SetStatus(acc.ID, "cooldown")
		logrus.Warnf("账号 %s 命中安全验证，冷却 24h", acc.ID)
	case errors.Is(err, xerrs.ErrLoginRequired):
		_ = e.st.SetStatus(acc.ID, "paused")
		logrus.Warnf("账号 %s 登录态失效，暂停待重登", acc.ID)
	default:
		_ = e.st.SetCooldown(acc.ID, time.Now().Add(2*time.Hour))
		logrus.Warnf("账号 %s 风控/异常，冷却 2h: %v", acc.ID, err)
	}
}

func isHardBlock(err error) bool {
	return errors.Is(err, xerrs.ErrVerificationRequired) || errors.Is(err, xerrs.ErrLoginRequired)
}
