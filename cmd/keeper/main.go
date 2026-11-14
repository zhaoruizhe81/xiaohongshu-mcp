package main

import (
	"context"
	"flag"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/internal/admin"
	"github.com/xpzouying/xiaohongshu-mcp/internal/commentgen"
	"github.com/xpzouying/xiaohongshu-mcp/internal/llm"
	"github.com/xpzouying/xiaohongshu-mcp/internal/store"
)

func main() {
	loadDotEnv(".env") // 本地 .env 提供默认，真实环境变量优先

	once := flag.Bool("once", false, "立即跑一场后退出（调试用）")
	llmcheck := flag.Bool("llmcheck", false, "只测试 LLM 连通后退出")
	flag.Parse()

	if *llmcheck {
		runLLMCheck()
		return
	}

	if err := os.MkdirAll("data", 0o755); err != nil {
		logrus.Fatal(err)
	}
	st, err := store.Open("data/keeper.db")
	if err != nil {
		logrus.Fatal(err)
	}
	defer st.Close()

	// 示例账号（真实场景由后台/配置下发）
	acc, err := st.EnsureAccount("demo")
	if err != nil {
		logrus.Fatal(err)
	}
	logrus.Infof("账号 %s 阶段=%s 状态=%s", acc.ID, Stage(acc.Stage), acc.Status)

	eng := NewEngine(st)
	eng.Commenter = buildCommenter(st)

	if *once {
		if err := eng.RunSession(acc); err != nil {
			logrus.Fatal(err)
		}
		return
	}

	// 手动触发通道（后台"立即跑一场" → 调度器）
	trigger := make(chan string, 16)

	// 小后台（可用 ADMIN_ADDR 覆盖，默认 :18061）
	addr := os.Getenv("ADMIN_ADDR")
	if addr == "" {
		addr = ":18061"
	}
	go func() {
		logrus.Infof("养号后台: http://127.0.0.1%s", addr)
		if err := admin.New(st, func(id string) {
			select {
			case trigger <- id:
			default:
			}
		}).Start(addr); err != nil {
			logrus.Errorf("后台启动失败: %v", err)
		}
	}()

	// 守护调度（阻塞）
	RunScheduler(eng, st, trigger)
}

// buildCommenter 按环境变量决定是否启用 LLM 评论。
func buildCommenter(st *store.Store) Commenter {
	cfg := llm.FromEnv()
	if !cfg.Enabled() {
		logrus.Info("未配置 LLM（LLM_MODEL / LLM_API_KEY），本次不做评论")
		return nil
	}
	logrus.Infof("已启用多模态评论：%s @ %s", cfg.Model, cfg.BaseURL)
	return commentgen.New(llm.New(cfg), st)
}

// runLLMCheck 只测 LLM 连通（供 -llmcheck）。
func runLLMCheck() {
	cfg := llm.FromEnv()
	if !cfg.Enabled() {
		logrus.Fatal("LLM 未配置：需要 LLM_MODEL(或 OPENAI_MODEL) + API_KEY(LLM_API_KEY / OPENAI_API_KEY)")
	}
	logrus.Infof("测试 LLM：%s @ %s", cfg.Model, cfg.BaseURL)
	out, err := llm.New(cfg).Chat(context.Background(), "你是助手。", "只回复两个字：你好", nil)
	if err != nil {
		logrus.Fatalf("调用失败：%v", err)
	}
	logrus.Infof("LLM 返回：%s", out)
}
