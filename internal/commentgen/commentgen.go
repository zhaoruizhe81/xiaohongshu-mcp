package commentgen

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/xpzouying/xiaohongshu-mcp/internal/llm"
	"github.com/xpzouying/xiaohongshu-mcp/internal/store"
	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

const systemPrompt = `你是真人小红书用户，正在刷笔记，给这篇笔记留一条自然评论。
铁律：只针对该笔记具体内容说一句，别泛泛而谈；长度 8~40 字、1~2 短句、最多 1 个 emoji；
不提价格、不承诺效果、不涉医疗法律财务、不给也不索要链接或联系方式、不引战不阴阳；
不暴露 AI、不出现"作为…/希望有帮助"之类客套；语气贴合【语言风格】，少客服腔、少感叹号。
只输出评论正文，不要解释、不要引号、不要"评论："前缀。`

var angles = []string{"共情作者", "提一个真实疑问", "补充自己的经验", "夸一个具体细节", "轻微调侃", "表达认同并延伸"}

// Generator 基于笔记内容(文字+图片)生成自然评论。
type Generator struct {
	client *llm.Client
	st     *store.Store
	hc     *http.Client
}

func New(client *llm.Client, st *store.Store) *Generator {
	return &Generator{client: client, st: st, hc: &http.Client{Timeout: 20 * time.Second}}
}

// Generate 生成一条评论；两次都不合格则返回空串（调用方跳过不发）。
func (g *Generator) Generate(ctx context.Context, accountID string, d *xiaohongshu.FeedDetailResponse, persona string) (string, error) {
	if persona == "" {
		persona = "轻松、口语化、像普通年轻人的表达"
	}
	angle := angles[rand.Intn(len(angles))]
	userText := buildUserText(d, persona, angle)
	imgs := g.collectImages(d)

	recent, _ := g.st.RecentComments(accountID, 30)

	for attempt := 0; attempt < 2; attempt++ {
		text, err := g.client.Chat(ctx, systemPrompt, userText, imgs)
		if err != nil {
			return "", err
		}
		text = clean(text)
		if validate(text, recent) {
			return text, nil
		}
		userText += "\n（上一条不合格：太笼统/太长或含联系方式，请更具体、更短、纯评论一句）"
	}
	return "", nil
}

func buildUserText(d *xiaohongshu.FeedDetailResponse, persona, angle string) string {
	title := strings.TrimSpace(d.Note.Title)
	desc := strings.TrimSpace(d.Note.Desc)
	if len([]rune(desc)) > 120 {
		desc = string([]rune(desc)[:120]) + "…"
	}
	var b strings.Builder
	b.WriteString("【帖子内容】\n")
	fmt.Fprintf(&b, "标题：%s\n正文：%s\n作者：%s\n", title, desc, d.Note.User.Nickname)
	b.WriteString("【语言风格】\n" + persona + "\n")
	b.WriteString("【本条角度】\n" + angle + "\n写这条评论：")
	return b.String()
}

// collectImages 取前 1~3 张图转 data URL（图文；视频帖子若带封面图也复用）。
func (g *Generator) collectImages(d *xiaohongshu.FeedDetailResponse) []string {
	var out []string
	for i, img := range d.Note.ImageList {
		if i >= 3 {
			break
		}
		u := img.URLDefault
		if u == "" {
			u = img.URLPre
		}
		if u == "" {
			continue
		}
		if data, err := g.fetchAsDataURL(u); err == nil {
			out = append(out, data)
		}
	}
	return out
}

func (g *Generator) fetchAsDataURL(u string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	// 小红书图 CDN 通常校验 Referer
	req.Header.Set("Referer", "https://www.xiaohongshu.com")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := g.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("图片下载 HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return "", err
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = "image/jpeg"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func clean(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'“”「」")
	s = strings.TrimPrefix(s, "评论：")
	s = strings.TrimPrefix(s, "评论:")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

var (
	bannedRe = regexp.MustCompile(`(?i)(https?://|www\.|微信|weixin|v信|加我|私信|链接|二维码|\bqq\b)`)
	phoneRe  = regexp.MustCompile(`\d{11}`)
	priceRe  = regexp.MustCompile(`\d+\s*元`)
)

// validate 校验：长度 8~40、无联系方式/链接/价格、不与历史评论雷同。
func validate(text string, recent []string) bool {
	n := len([]rune(text))
	if n < 8 || n > 40 {
		return false
	}
	if bannedRe.MatchString(text) || phoneRe.MatchString(text) || priceRe.MatchString(text) {
		return false
	}
	for _, r := range recent {
		if similar(r, text) {
			return false
		}
	}
	return true
}

// similar 用字符集合 Jaccard 粗判雷同（>0.7 视为重复）。
func similar(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	sa, sb := runeSet(a), runeSet(b)
	inter := 0
	for k := range sa {
		if sb[k] {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return false
	}
	return float64(inter)/float64(union) > 0.7
}

func runeSet(s string) map[rune]bool {
	m := map[rune]bool{}
	for _, r := range s {
		m[r] = true
	}
	return m
}
