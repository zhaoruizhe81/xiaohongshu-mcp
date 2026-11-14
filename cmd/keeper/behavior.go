package main

import (
	"context"
	"encoding/json"
	"math/rand"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/internal/store"
	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

// Stage 养号阶段。由后台人工推进。
type Stage int

const (
	StageObserve Stage = iota // D1-2 只看不互动
	StageLight                // D3-5 低频点赞
	StageDeepen               // D6-10 收藏+逛主页
	StageSocial               // D11+ 评论+通知
	StageCreator              // 发布（暂不做）
)

func (s Stage) String() string {
	switch s {
	case StageObserve:
		return "观察"
	case StageLight:
		return "轻互动"
	case StageDeepen:
		return "深化"
	case StageSocial:
		return "社交"
	case StageCreator:
		return "创作"
	}
	return "未知"
}

// 各阶段行为权重（抽样时归一化）。
var stageWeights = map[Stage]map[string]float64{
	StageObserve: {"browse_feed": 70, "open_note": 20, "rest": 10},
	StageLight:   {"browse_feed": 45, "open_note": 25, "like": 15, "search_browse": 10, "rest": 5},
	StageDeepen:  {"browse_feed": 35, "open_note": 25, "like": 12, "favorite": 8, "search_browse": 8, "visit_profile": 5, "rest": 7},
	StageSocial:  {"browse_feed": 30, "open_note": 20, "like": 10, "favorite": 5, "comment": 8, "search_browse": 7, "visit_profile": 8, "check_notif": 5, "rest": 7},
}

// 各阶段每日用量上限（硬顶；权重负责让频率更低）。
type dailyCaps struct {
	like, fav, comment, search int
}

var stageCaps = map[Stage]dailyCaps{
	StageObserve: {0, 0, 0, 1},
	StageLight:   {8, 0, 0, 2},
	StageDeepen:  {15, 5, 0, 3},
	StageSocial:  {25, 8, 6, 4},
}

// Commenter 评论生成器（P4 注入多模态 LLM；nil 则不做评论）。
type Commenter interface {
	Generate(ctx context.Context, accountID string, detail *xiaohongshu.FeedDetailResponse, persona string) (string, error)
}

// SessionStats 一场会话的行为计数。
type SessionStats struct {
	Browse, Open, Like, Favorite, Comment, Search int
}

// BehaviorCtx 行为执行上下文。
type BehaviorCtx struct {
	Ctx       context.Context
	S         *Session
	St        *store.Store
	Acc       *store.Account
	Usage     *store.DailyUsage
	Day       string
	Stats     *SessionStats
	Commenter Commenter
	Persona   string
}

func (c *BehaviorCtx) stage() Stage { return Stage(c.Acc.Stage) }

// consume 记一次用量（同时写真库与内存）。
func (c *BehaviorCtx) consume(field string, n int) {
	if err := c.St.IncrUsage(c.Acc.ID, c.Day, field, n); err != nil {
		logrus.Warnf("记用量失败 %s: %v", field, err)
		return
	}
	switch field {
	case "like":
		c.Usage.UsedLike += n
	case "favorite":
		c.Usage.UsedFavorite += n
	case "comment":
		c.Usage.UsedComment += n
	case "search":
		c.Usage.UsedSearch += n
	}
}

// Behavior 一个可抽样的养号微行为。
type Behavior interface {
	Key() string
	Weight(c *BehaviorCtx) float64
	Run(c *BehaviorCtx) error
}

func allBehaviors() []Behavior {
	return []Behavior{
		browseBehavior{}, openBehavior{}, likeBehavior{}, favoriteBehavior{},
		commentBehavior{}, searchBehavior{}, profileBehavior{}, notifBehavior{}, restBehavior{},
	}
}

func baseWeight(c *BehaviorCtx, key string) float64 {
	return stageWeights[c.stage()][key]
}

// ===== 各行为实现 =====

// 刷 feed：真滚动，刷 N(20~50) 条就刷新。
type browseBehavior struct{}

func (browseBehavior) Key() string                   { return "browse_feed" }
func (browseBehavior) Weight(c *BehaviorCtx) float64 { return baseWeight(c, "browse_feed") }
func (browseBehavior) Run(c *BehaviorCtx) error {
	n := 20 + rand.Intn(31)
	if err := c.S.BrowseFeed(c.Ctx, n, 1); err != nil {
		return err
	}
	c.Stats.Browse++
	return nil
}

// 看笔记：按图片数/视频时长停留。
type openBehavior struct{}

func (openBehavior) Key() string                   { return "open_note" }
func (openBehavior) Weight(c *BehaviorCtx) float64 { return baseWeight(c, "open_note") }
func (openBehavior) Run(c *BehaviorCtx) error {
	f, ok := c.S.popNote()
	if !ok {
		return nil
	}
	detail, err := c.S.Detail(c.Ctx, f.ID, f.XsecToken)
	if err != nil {
		return err
	}
	c.Stats.Open++
	noteWatch(c.Ctx, detail)
	// 多数情况会翻一下评论区（模拟看评论）
	if rand.Float64() < 0.7 {
		c.S.ReadComments(c.Ctx, 2+rand.Intn(4)) // 2~5 屏
	}
	return nil
}

// 点赞。
type likeBehavior struct{}

func (likeBehavior) Key() string { return "like" }
func (likeBehavior) Weight(c *BehaviorCtx) float64 {
	cap := stageCaps[c.stage()].like
	if cap == 0 || c.Usage.UsedLike >= cap {
		return 0
	}
	return baseWeight(c, "like")
}
func (likeBehavior) Run(c *BehaviorCtx) error {
	f, ok := c.S.popNote()
	if !ok {
		return nil
	}
	if done, _ := c.St.HasInteraction(c.Acc.ID, f.ID, "like"); done {
		return nil
	}
	if err := c.S.Like(c.Ctx, f.ID, f.XsecToken); err != nil {
		return err
	}
	_, _ = c.St.AddInteraction(c.Acc.ID, f.ID, f.NoteCard.User.UserID, "like", f.NoteCard.DisplayTitle, "", "", "ok")
	c.consume("like", 1)
	c.Stats.Like++
	return nil
}

// 收藏。
type favoriteBehavior struct{}

func (favoriteBehavior) Key() string { return "favorite" }
func (favoriteBehavior) Weight(c *BehaviorCtx) float64 {
	cap := stageCaps[c.stage()].fav
	if cap == 0 || c.Usage.UsedFavorite >= cap {
		return 0
	}
	return baseWeight(c, "favorite")
}
func (favoriteBehavior) Run(c *BehaviorCtx) error {
	f, ok := c.S.popNote()
	if !ok {
		return nil
	}
	if done, _ := c.St.HasInteraction(c.Acc.ID, f.ID, "favorite"); done {
		return nil
	}
	if err := c.S.Favorite(c.Ctx, f.ID, f.XsecToken); err != nil {
		return err
	}
	_, _ = c.St.AddInteraction(c.Acc.ID, f.ID, f.NoteCard.User.UserID, "favorite", f.NoteCard.DisplayTitle, "", "", "ok")
	c.consume("favorite", 1)
	c.Stats.Favorite++
	return nil
}

// 评论（需 Commenter；P4 注入）。
type commentBehavior struct{}

func (commentBehavior) Key() string { return "comment" }
func (commentBehavior) Weight(c *BehaviorCtx) float64 {
	if c.Commenter == nil {
		return 0
	}
	cap := stageCaps[c.stage()].comment
	if cap == 0 || c.Usage.UsedComment >= cap {
		return 0
	}
	return baseWeight(c, "comment")
}
func (commentBehavior) Run(c *BehaviorCtx) error {
	f, ok := c.S.popNote()
	if !ok {
		return nil
	}
	if done, _ := c.St.HasInteraction(c.Acc.ID, f.ID, "comment"); done {
		return nil
	}
	detail, err := c.S.Detail(c.Ctx, f.ID, f.XsecToken)
	if err != nil {
		return err
	}
	noteWatch(c.Ctx, detail) // 先看一会再评论，像人
	// 评论前先翻看评论，更像真人
	if rand.Float64() < 0.6 {
		c.S.ReadComments(c.Ctx, 1+rand.Intn(3))
	}
	text, err := c.Commenter.Generate(c.Ctx, c.Acc.ID, detail, c.Persona)
	if err != nil || text == "" {
		return err
	}
	if err := c.S.Comment(c.Ctx, f.ID, f.XsecToken, text); err != nil {
		return err
	}
	_, _ = c.St.AddInteraction(c.Acc.ID, f.ID, f.NoteCard.User.UserID, "comment", f.NoteCard.DisplayTitle, "", text, "ok")
	c.consume("comment", 1)
	c.Stats.Comment++
	return nil
}

// 搜索浏览（用人设关键词）。
type searchBehavior struct{}

func (searchBehavior) Key() string { return "search_browse" }
func (searchBehavior) Weight(c *BehaviorCtx) float64 {
	cap := stageCaps[c.stage()].search
	if c.Usage.UsedSearch >= cap {
		return 0
	}
	return baseWeight(c, "search_browse")
}
func (searchBehavior) Run(c *BehaviorCtx) error {
	kw := pickKeyword(c.Acc.Keywords)
	if kw == "" {
		return nil
	}
	feeds, err := c.S.Search(c.Ctx, kw)
	if err != nil {
		return err
	}
	for _, f := range feeds {
		c.S.enqueue(f)
	}
	c.consume("search", 1)
	c.Stats.Search++
	dwell(c.Ctx, 3, 8)
	return nil
}

// 逛主页。
type profileBehavior struct{}

func (profileBehavior) Key() string                   { return "visit_profile" }
func (profileBehavior) Weight(c *BehaviorCtx) float64 { return baseWeight(c, "visit_profile") }
func (profileBehavior) Run(c *BehaviorCtx) error {
	f, ok := c.S.popNote()
	if !ok {
		return nil
	}
	uid := f.NoteCard.User.UserID
	if uid == "" {
		return nil
	}
	if _, err := c.S.Profile(c.Ctx, uid, f.XsecToken); err != nil {
		return err
	}
	dwell(c.Ctx, 3, 10)
	return nil
}

// 看通知。
type notifBehavior struct{}

func (notifBehavior) Key() string                   { return "check_notif" }
func (notifBehavior) Weight(c *BehaviorCtx) float64 { return baseWeight(c, "check_notif") }
func (notifBehavior) Run(c *BehaviorCtx) error {
	if _, err := c.S.NotifUnread(c.Ctx); err != nil {
		return err
	}
	dwell(c.Ctx, 2, 6)
	return nil
}

// 休息（模拟走神）。
type restBehavior struct{}

func (restBehavior) Key() string                   { return "rest" }
func (restBehavior) Weight(c *BehaviorCtx) float64 { return baseWeight(c, "rest") }
func (restBehavior) Run(c *BehaviorCtx) error {
	dwell(c.Ctx, 8, 40)
	return nil
}

// noteWatch 看一条笔记的停留：视频按时长比例，图文按图片数。
func noteWatch(ctx context.Context, d *xiaohongshu.FeedDetailResponse) {
	var sec float64
	if d.Note.Video != nil && d.Note.Video.Capa.Duration > 0 {
		sec = float64(d.Note.Video.Capa.Duration) * (0.3 + rand.Float64()*0.5)
	} else {
		sec = 3 + float64(len(d.Note.ImageList))*2 + rand.Float64()*3
	}
	if sec < 4 {
		sec = 4
	}
	if sec > 60 {
		sec = 60
	}
	sleepCtx(ctx, time.Duration(sec*float64(time.Second)))
}

// pickKeyword 从账号关键词 JSON 里随机取一个；空则返回 ""。
func pickKeyword(raw string) string {
	var kws []string
	if err := json.Unmarshal([]byte(raw), &kws); err != nil || len(kws) == 0 {
		return ""
	}
	return kws[rand.Intn(len(kws))]
}
