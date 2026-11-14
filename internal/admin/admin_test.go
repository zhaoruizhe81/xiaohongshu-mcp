package admin

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/xpzouying/xiaohongshu-mcp/internal/store"
)

// 验证后台首页模板能正确渲染出账号。
func TestDashboardRenders(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.EnsureAccount("demo"); err != nil {
		t.Fatal(err)
	}

	s := New(st, func(string) {})
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	s.dashboard(c)

	if w.Code != 200 {
		t.Fatalf("状态码 %d", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "demo") || !strings.Contains(body, "养号后台") {
		t.Fatalf("渲染缺内容: %s", body)
	}
}
