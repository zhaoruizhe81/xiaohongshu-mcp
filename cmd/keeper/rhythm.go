package main

import (
	"context"
	"math"
	"math/rand"
	"time"
)

// Policy 节奏策略：活跃时段、每日场次、会话时长、动作间隔。
type Policy struct {
	ActiveWindows     [][2]int // 活跃小时区间 [start,end)
	MaxSessionsPerDay int
	SessionMedian     time.Duration
	GapMin, GapMax    time.Duration
}

// DefaultPolicy 默认节奏（后台后续可覆盖）。
var DefaultPolicy = Policy{
	ActiveWindows:     [][2]int{{7, 9}, {12, 14}, {19, 23}},
	MaxSessionsPerDay: 3,
	SessionMedian:     20 * time.Minute,
	GapMin:            8 * time.Second,
	GapMax:            50 * time.Second,
}

// InActiveWindow 当前是否处于活跃时段。
func InActiveWindow(t time.Time) bool {
	h := t.Hour()
	for _, w := range DefaultPolicy.ActiveWindows {
		if h >= w[0] && h < w[1] {
			return true
		}
	}
	return false
}

// SampleSessionDuration 对数正态，中位约 SessionMedian，夹在 10~45min。
func SampleSessionDuration() time.Duration {
	secs := DefaultPolicy.SessionMedian.Seconds() * math.Exp(0.35*rand.NormFloat64())
	d := time.Duration(secs * float64(time.Second))
	if d < 10*time.Minute {
		d = 10 * time.Minute
	}
	if d > 45*time.Minute {
		d = 45 * time.Minute
	}
	return d
}

// SampleGap 两个行为之间的随机间隔。
func SampleGap() time.Duration {
	span := DefaultPolicy.GapMax - DefaultPolicy.GapMin
	if span <= 0 {
		return DefaultPolicy.GapMin
	}
	return DefaultPolicy.GapMin + time.Duration(rand.Int63n(int64(span)))
}

// sleepCtx 可被 ctx 取消的 sleep。
func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}
