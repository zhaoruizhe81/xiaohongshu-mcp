package commentgen

import (
	"strings"
	"testing"

	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		text string
		ok   bool
	}{
		{"这个配色我真的爱了，求个色号", true},
		{"好棒", false},                    // 太短
		{"加我微信拿链接", false},               // 联系方式
		{"https://x.com 看看", false},      // 链接
		{"这个要 199 元吗", false},            // 价格
		{strings.Repeat("字", 50), false}, // 太长
	}
	for _, c := range cases {
		if got := validate(c.text, nil); got != c.ok {
			t.Errorf("validate(%q)=%v want %v", c.text, got, c.ok)
		}
	}
	if validate("这个配色我真的爱了", []string{"这个配色我真的爱了"}) {
		t.Error("与历史重复的评论应被拦")
	}
}

func TestClean(t *testing.T) {
	if got := clean(`"评论：这个也太好用了吧"`); got != "这个也太好用了吧" {
		t.Errorf("clean=%q", got)
	}
}

func TestBuildUserText(t *testing.T) {
	d := &xiaohongshu.FeedDetailResponse{}
	d.Note.Title = "标题X"
	d.Note.Desc = "正文Y"
	s := buildUserText(d, "活泼", "共情作者")
	if !strings.Contains(s, "标题X") || !strings.Contains(s, "活泼") || !strings.Contains(s, "共情作者") {
		t.Errorf("buildUserText 缺字段: %s", s)
	}
}
