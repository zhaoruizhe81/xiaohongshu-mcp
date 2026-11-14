package main

import (
	"context"
	"flag"
	"os"
	"time"

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
	accountID := flag.String("account", "demo", "账号 ID")
	stageFlag := flag.Int("stage", -1, "设置账号阶段 0-4（-1 不改）")
	persona := flag.String("persona", "", "设置语言风格（空=不改）")
	keywords := flag.String("keywords", "", "设置兴趣关键词 JSON（空=不改）")
	minutes := flag.Int("minutes", 0, "单场时长(分钟, 调试用; 0=按节奏随机)")
	behavior := flag.String("behavior", "", "强制单一行为(调试): like/favorite/comment/open_note/browse_feed/search_browse/visit_profile/check_notif/rest")
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

	// 账号（真实场景由后台/配置下发）
	acc, err := st.EnsureAccount(*accountID)
	if err != nil {
		logrus.Fatal(err)
	}
	// 可选：本次启动覆盖账号配置
	if *stageFlag >= 0 {
		_ = st.SetStage(acc.ID, *stageFlag)
	}
	if *persona != "" {
		_ = st.SetPersona(acc.ID, *persona)
	}
	if *keywords != "" {
		_ = st.UpdateKeywords(acc.ID, *keywords)
	}
	if *stageFlag >= 0 || *persona != "" || *keywords != "" {
		if a, e := st.GetAccount(acc.ID); e == nil {
			acc = a
		}
	}
	logrus.Infof("账号 %s 阶段=%s 状态=%s", acc.ID, Stage(acc.Stage), acc.Status)

	eng := NewEngine(st)
	eng.Commenter = buildCommenter(st)
	if *minutes > 0 {
		eng.Duration = time.Duration(*minutes) * time.Minute
	}
	eng.ForceBehavior = *behavior

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
