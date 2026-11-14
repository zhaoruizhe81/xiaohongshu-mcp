package main

import (
	"context"
	"encoding/json"
	"math/rand"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

const homeURL = "https://www.xiaohongshu.com/explore"

// BrowseFeed 真滚动刷首页：每屏停留、刷到 targetPerRound 条新笔记就刷新首页，重复 rounds 轮。
// 只为"像人地刷"，把候选笔记喂进 s.queue；不点开看详情（那由 open_note 行为负责）。
// 关键：刷一段就刷新（reload），不无限下滑——无限下滑是明显的非人特征。
func (s *Session) BrowseFeed(ctx context.Context, targetPerRound, rounds int) error {
	if err := s.gotoHome(ctx); err != nil {
		return err
	}

	for r := 0; r < rounds; r++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		got, attempts := 0, 0
		maxAttempts := targetPerRound*2 + 10
		for got < targetPerRound && attempts < maxAttempts {
			if err := ctx.Err(); err != nil {
				return err
			}
			scrollFeed(s.page) // 滚 1~3 屏
			dwell(ctx, 2, 6)   // 每屏停留 2~6s
			attempts++

			feeds, err := s.readFeedList(ctx)
			if err != nil {
				continue
			}
			for _, f := range feeds {
				if f.ID == "" || s.seen[f.ID] {
					continue
				}
				s.seen[f.ID] = true
				s.queue = append(s.queue, f)
				got++
			}
		}

		logrus.Infof("本轮刷到 %d 条新笔记（累计队列 %d），刷新首页", got, len(s.queue))
		if err := s.gotoHome(ctx); err != nil { // 刷到 N 条 → 刷新
			return err
		}
	}
	return nil
}

// gotoHome 导航/刷新首页并等待就绪。
func (s *Session) gotoHome(ctx context.Context) error {
	p := s.page.Context(ctx).Timeout(60 * time.Second)
	if err := p.Navigate(homeURL); err != nil {
		return err
	}
	_ = p.WaitLoad()
	_ = p.WaitDOMStable(time.Second, 0.1)
	humanize.Delay(ctx, humanize.AfterNavigate)
	return nil
}

// readFeedList 读当前已加载的推荐流笔记（随滚动懒加载会变多）。
func (s *Session) readFeedList(ctx context.Context) ([]xiaohongshu.Feed, error) {
	p := s.page.Context(ctx).Timeout(10 * time.Second)
	res, err := p.Eval(`() => {
		if (window.__INITIAL_STATE__ && window.__INITIAL_STATE__.feed && window.__INITIAL_STATE__.feed.feeds) {
			const feeds = window.__INITIAL_STATE__.feed.feeds;
			const v = feeds.value !== undefined ? feeds.value : feeds._value;
			return v ? JSON.stringify(v) : "";
		}
		return "";
	}`)
	if err != nil {
		return nil, err
	}
	str := res.Value.Str()
	if str == "" {
		return nil, nil
	}
	var feeds []xiaohongshu.Feed
	if err := json.Unmarshal([]byte(str), &feeds); err != nil {
		return nil, err
	}
	// 只要笔记（图文/视频 modelType 都是 note）
	notes := make([]xiaohongshu.Feed, 0, len(feeds))
	for _, f := range feeds {
		if f.ModelType == "note" {
			notes = append(notes, f)
		}
	}
	return notes, nil
}

// scrollFeed 指针移到视口中间后滚 1~3 屏，每屏幅度约 0.6~1.1 视口高。
func scrollFeed(page *rod.Page) {
	w, h := viewport(page)
	if w > 0 && h > 0 {
		_ = page.Mouse.MoveTo(proto.Point{
			X: float64(w)/2 + (rand.Float64()-0.5)*100,
			Y: float64(h)/2 + (rand.Float64()-0.5)*100,
		})
	}

	screens := 1 + rand.Intn(3)
	for i := 0; i < screens; i++ {
		delta := float64(h) * (0.6 + rand.Float64()*0.5)
		_ = page.Mouse.Scroll(0, delta, 10)
		time.Sleep(time.Duration(20+rand.Intn(40)) * time.Millisecond)
	}
}

func viewport(page *rod.Page) (int, int) {
	res, err := page.Eval(`() => JSON.stringify([window.innerWidth, window.innerHeight])`)
	if err != nil {
		return 0, 0
	}
	var wh []int
	if json.Unmarshal([]byte(res.Value.Str()), &wh) != nil || len(wh) != 2 {
		return 0, 0
	}
	return wh[0], wh[1]
}

// dwell 在 [minS,maxS] 秒间随机停顿一会（模拟看卡片）。
func dwell(ctx context.Context, minS, maxS int) {
	d := time.Duration(minS+rand.Intn(maxS-minS+1)) * time.Second
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}
