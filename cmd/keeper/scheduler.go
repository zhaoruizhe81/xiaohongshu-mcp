package main

import (
	"time"

	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/internal/store"
)

// RunScheduler 守护调度：活跃时段内给启用账号自动开场；也接受 admin 的手动触发。
// 阻塞运行（常驻）。单号顺序执行；多号后续再并发。
func RunScheduler(e *Engine, st *store.Store, trigger <-chan string) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	logrus.Info("调度器启动：每 5 分钟检查一次，活跃时段内自动开场")
	for {
		select {
		case id := <-trigger: // 后台手动"立即跑一场"
			acc, err := st.GetAccount(id)
			if err != nil || acc == nil {
				logrus.Warnf("手动触发失败，账号不存在: %s", id)
				continue
			}
			_ = e.RunSession(acc)

		case <-ticker.C:
			if !InActiveWindow(time.Now()) {
				continue
			}
			accs, err := st.ListAccounts()
			if err != nil {
				logrus.Warnf("读账号列表失败: %v", err)
				continue
			}
			day := time.Now().Format("2006-01-02")
			for _, acc := range accs {
				if acc.Status != "active" {
					continue
				}
				if time.Now().Before(acc.CooldownUntil) {
					continue
				}
				usage, err := st.GetDaily(acc.ID, day)
				if err != nil {
					continue
				}
				if usage.Sessions >= DefaultPolicy.MaxSessionsPerDay {
					continue
				}
				if err := e.RunSession(acc); err != nil {
					logrus.Warnf("账号 %s 会话失败: %v", acc.ID, err)
				}
			}
		}
	}
}
