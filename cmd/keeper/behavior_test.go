package main

import (
	"context"
	"testing"
	"time"

	"github.com/xpzouying/xiaohongshu-mcp/internal/store"
	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

type stubCommenter struct{}

func (stubCommenter) Generate(context.Context, string, *xiaohongshu.FeedDetailResponse, string) (string, error) {
	return "好", nil
}

// 阶段权重与配额门槛：观察期不互动、社交期才评论、配额满则归零。
func TestWeightsAndCaps(t *testing.T) {
	obs := &BehaviorCtx{Acc: &store.Account{Stage: int(StageObserve)}, Usage: &store.DailyUsage{}}
	if w := (likeBehavior{}).Weight(obs); w != 0 {
		t.Errorf("观察期点赞权重应=0，got %v", w)
	}
	if w := (commentBehavior{}).Weight(obs); w != 0 {
		t.Errorf("观察期评论权重应=0，got %v", w)
	}

	soc := &BehaviorCtx{Acc: &store.Account{Stage: int(StageSocial)}, Usage: &store.DailyUsage{}}
	if w := (likeBehavior{}).Weight(soc); w <= 0 {
		t.Errorf("社交期点赞权重应>0，got %v", w)
	}
	// 没有 Commenter 时评论权重为 0
	if w := (commentBehavior{}).Weight(soc); w != 0 {
		t.Errorf("无 Commenter 评论权重应=0，got %v", w)
	}
	soc.Commenter = stubCommenter{}
	if w := (commentBehavior{}).Weight(soc); w <= 0 {
		t.Errorf("有 Commenter 评论权重应>0，got %v", w)
	}
	// 配额打满后点赞权重归零
	soc.Usage.UsedLike = 999
	if w := (likeBehavior{}).Weight(soc); w != 0 {
		t.Errorf("配额满点赞权重应=0，got %v", w)
	}
}

func TestPickKeyword(t *testing.T) {
	if got := pickKeyword(""); got != "" {
		t.Errorf("空关键词应返回空，got %q", got)
	}
	if got := pickKeyword(`["美食","穿搭"]`); got != "美食" && got != "穿搭" {
		t.Errorf("关键词取值异常: %q", got)
	}
	if got := pickKeyword("not-json"); got != "" {
		t.Errorf("非法JSON应返回空，got %q", got)
	}
}

func TestRhythm(t *testing.T) {
	if !InActiveWindow(time.Date(2026, 11, 14, 8, 0, 0, 0, time.Local)) {
		t.Error("8点应在活跃时段")
	}
	if InActiveWindow(time.Date(2026, 11, 14, 15, 0, 0, 0, time.Local)) {
		t.Error("15点不应在活跃时段")
	}
	for i := 0; i < 200; i++ {
		if d := SampleSessionDuration(); d < 10*time.Minute || d > 45*time.Minute {
			t.Fatalf("会话时长越界: %v", d)
		}
	}
}
