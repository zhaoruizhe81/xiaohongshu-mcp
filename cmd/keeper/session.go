package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/go-rod/rod"
	"github.com/xpzouying/headless_browser"
	"github.com/xpzouying/xiaohongshu-mcp/browser"
	"github.com/xpzouying/xiaohongshu-mcp/configs"
	"github.com/xpzouying/xiaohongshu-mcp/cookies"
	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

// Session 一个账号的一次常驻养号会话：整个会话共用一个浏览器 + 一个标签页。
// 关键点：浏览器只在 OpenSession 时开一次，后续所有动作都复用同一个 page，
// 不再像 service.go 那样每个动作 newBrowser()/Close()。
type Session struct {
	b     *headless_browser.Browser
	page  *rod.Page
	queue []xiaohongshu.Feed // 刷 feed 得到的候选笔记，供后续点赞/收藏/评论消费
	seen  map[string]bool    // 已入队去重
}

// OpenSession 打开有头浏览器并校验登录态。整场会话只开这一次浏览器。
func OpenSession() (*Session, error) {
	// 指纹 seed：env > 会话文件 > 新生成写回（同一账号同一指纹）
	seed := configs.ResolveFingerprintSeed(cookies.NewLoadCookie(cookies.GetCookiesFilePath()))

	b := browser.NewBrowser(false, // 有头
		browser.WithFingerprintSeed(seed),
		browser.WithProxy(configs.ProxyFromEnv()),
	)
	page := b.NewPage()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if ok, err := xiaohongshu.NewLogin(page).CheckLoginStatus(ctx); err != nil || !ok {
		b.Close()
		return nil, fmt.Errorf("登录态无效，请先扫码登录: %v", err)
	}
	return &Session{b: b, page: page, seen: map[string]bool{}}, nil
}

func (s *Session) Close() {
	_ = s.page.Close()
	s.b.Close()
}

// Healthy 会话健康检查：命中验证页/登录页返回错误，供上层冷却。
func (s *Session) Healthy() error { return xiaohongshu.CheckPageBlocked(s.page) }

// safe 包住动作：Must* 遇到异常页会 panic，recover 住不让养号循环崩。
func (s *Session) safe(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("动作 panic: %v", r)
		}
	}()
	return fn()
}

// —— 动作：全部复用同一个常驻 page ——

func (s *Session) Feeds(ctx context.Context) ([]xiaohongshu.Feed, error) {
	return xiaohongshu.NewFeedsListAction(s.page).GetFeedsList(ctx)
}

func (s *Session) Detail(ctx context.Context, id, token string) (*xiaohongshu.FeedDetailResponse, error) {
	return xiaohongshu.NewFeedDetailAction(s.page).GetFeedDetail(ctx, id, token, false, xiaohongshu.DefaultCommentLoadConfig())
}

func (s *Session) Like(ctx context.Context, id, token string) error {
	return xiaohongshu.NewLikeAction(s.page).Like(ctx, id, token)
}

func (s *Session) Favorite(ctx context.Context, id, token string) error {
	return xiaohongshu.NewFavoriteAction(s.page).Favorite(ctx, id, token)
}

func (s *Session) Comment(ctx context.Context, id, token, text string) error {
	return xiaohongshu.NewCommentFeedAction(s.page).PostComment(ctx, id, token, text)
}

func (s *Session) Search(ctx context.Context, keyword string) ([]xiaohongshu.Feed, error) {
	return xiaohongshu.NewSearchAction(s.page).Search(ctx, keyword)
}

func (s *Session) Profile(ctx context.Context, userID, token string) (*xiaohongshu.UserProfileResponse, error) {
	tab, _ := xiaohongshu.ParseProfileTab("note")
	return xiaohongshu.NewUserProfileAction(s.page).UserProfile(ctx, userID, token, tab)
}

func (s *Session) NotifUnread(ctx context.Context) (*xiaohongshu.NotificationCount, error) {
	return xiaohongshu.NewNotificationAction(s.page).UnreadCount(ctx)
}

// enqueue 把候选笔记加入队列（按 ID 去重）。
func (s *Session) enqueue(f xiaohongshu.Feed) {
	if f.ID == "" || s.seen[f.ID] {
		return
	}
	s.seen[f.ID] = true
	s.queue = append(s.queue, f)
}

// popNote 从队列随机取一条待处理笔记。
func (s *Session) popNote() (xiaohongshu.Feed, bool) {
	if len(s.queue) == 0 {
		return xiaohongshu.Feed{}, false
	}
	i := rand.Intn(len(s.queue))
	f := s.queue[i]
	s.queue = append(s.queue[:i], s.queue[i+1:]...)
	return f, true
}
