package admin

import (
	"html/template"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xpzouying/xiaohongshu-mcp/internal/store"
)

// Server 养号小后台。只读写 DB + 通过 trigger 通知调度器立即开跑。
type Server struct {
	st      *store.Store
	trigger func(accountID string)
	token   string
}

func New(st *store.Store, trigger func(string)) *Server {
	return &Server{st: st, trigger: trigger, token: os.Getenv("ADMIN_TOKEN")}
}

// Start 阻塞启动 HTTP 服务。
func (s *Server) Start(addr string) error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(s.auth())

	r.GET("/", s.dashboard)
	r.GET("/api/accounts", s.apiAccounts)

	g := r.Group("/accounts/:id")
	g.POST("/stage", s.setStage)
	g.POST("/persona", s.setPersona)
	g.POST("/keywords", s.setKeywords)
	g.POST("/status", s.setStatus)
	g.POST("/run", s.run)

	return r.Run(addr)
}

// auth 简单鉴权：ADMIN_TOKEN 非空时校验 Bearer 或 ?token=。
func (s *Server) auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.token == "" {
			c.Next()
			return
		}
		tok := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if tok == "" {
			tok = c.Query("token")
		}
		if tok != s.token {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}

type stageOpt struct {
	Val      int
	Name     string
	Selected bool
}

var stageList = []struct {
	Val  int
	Name string
}{
	{0, "观察"}, {1, "轻互动"}, {2, "深化"}, {3, "社交"}, {4, "创作"},
}

type row struct {
	Acc     *store.Account
	Usage   *store.DailyUsage
	Options []stageOpt
}

func (s *Server) dashboard(c *gin.Context) {
	accs, err := s.st.ListAccounts()
	if err != nil {
		c.String(http.StatusInternalServerError, "读账号失败: %v", err)
		return
	}
	day := time.Now().Format("2006-01-02")
	rows := make([]row, 0, len(accs))
	for _, a := range accs {
		u, _ := s.st.GetDaily(a.ID, day)
		opts := make([]stageOpt, 0, len(stageList))
		for _, o := range stageList {
			opts = append(opts, stageOpt{o.Val, o.Name, o.Val == a.Stage})
		}
		rows = append(rows, row{a, u, opts})
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := dashboardTmpl.Execute(c.Writer, gin.H{"Rows": rows, "Day": day}); err != nil {
		c.String(http.StatusInternalServerError, "渲染失败: %v", err)
	}
}

func (s *Server) apiAccounts(c *gin.Context) {
	accs, _ := s.st.ListAccounts()
	c.JSON(http.StatusOK, accs)
}

func (s *Server) setStage(c *gin.Context) {
	v, _ := strconv.Atoi(c.PostForm("stage"))
	if v < 0 || v > 4 {
		c.String(http.StatusBadRequest, "stage 非法")
		return
	}
	_ = s.st.SetStage(c.Param("id"), v)
	c.Redirect(http.StatusSeeOther, "/")
}

func (s *Server) setPersona(c *gin.Context) {
	_ = s.st.SetPersona(c.Param("id"), c.PostForm("persona"))
	c.Redirect(http.StatusSeeOther, "/")
}

func (s *Server) setKeywords(c *gin.Context) {
	// 关键词直接存 JSON 字符串（前端填 ["美食","穿搭"]）
	_ = s.st.UpdateKeywords(c.Param("id"), c.PostForm("keywords"))
	c.Redirect(http.StatusSeeOther, "/")
}

func (s *Server) setStatus(c *gin.Context) {
	status := c.PostForm("status")
	if status != "active" && status != "paused" {
		c.String(http.StatusBadRequest, "status 非法")
		return
	}
	_ = s.st.SetStatus(c.Param("id"), status)
	c.Redirect(http.StatusSeeOther, "/")
}

func (s *Server) run(c *gin.Context) {
	id := c.Param("id")
	s.trigger(id)
	c.Redirect(http.StatusSeeOther, "/")
}

var dashboardTmpl = template.Must(template.New("dashboard").Parse(`<!doctype html>
<html lang="zh"><head><meta charset="utf-8"><title>养号后台</title>
<style>
body{font-family:-apple-system,"PingFang SC",sans-serif;margin:24px;color:#222}
table{border-collapse:collapse;width:100%}
th,td{border:1px solid #ddd;padding:6px 8px;font-size:13px;vertical-align:top}
th{background:#f5f5f5;text-align:left}
input{padding:3px 6px;font-size:13px}
button{padding:3px 8px;font-size:12px;cursor:pointer}
form{display:inline-block;margin:0}
.small{color:#888;font-size:12px}
</style></head><body>
<h2>小红书养号后台</h2>
<div class="small">今日：{{.Day}} ｜ 阶段：0观察 1轻互动 2深化 3社交 4创作</div>
<table>
<thead><tr>
<th>账号</th><th>阶段</th><th>语言风格(persona)</th><th>关键词(JSON)</th><th>状态</th><th>今日 赞/藏/评/搜/场</th><th>操作</th>
</tr></thead>
<tbody>
{{range .Rows}}
<tr>
<td><b>{{.Acc.ID}}</b><div class="small">{{.Acc.Nickname}}</div></td>
<td><form method="post" action="/accounts/{{.Acc.ID}}/stage">
<select name="stage">{{range .Options}}<option value="{{.Val}}" {{if .Selected}}selected{{end}}>{{.Name}}</option>{{end}}</select>
<button>存</button></form></td>
<td><form method="post" action="/accounts/{{.Acc.ID}}/persona">
<input name="persona" value="{{.Acc.Persona}}" size="22" placeholder="如：20岁女生，轻松口语化">
<button>存</button></form></td>
<td><form method="post" action="/accounts/{{.Acc.ID}}/keywords">
<input name="keywords" value="{{.Acc.Keywords}}" size="24" placeholder='["美食","穿搭"]'>
<button>存</button></form></td>
<td>{{.Acc.Status}}</td>
<td>{{.Usage.UsedLike}}/{{.Usage.UsedFavorite}}/{{.Usage.UsedComment}}/{{.Usage.UsedSearch}}/{{.Usage.Sessions}}</td>
<td>
<form method="post" action="/accounts/{{.Acc.ID}}/status"><button name="status" value="active">启用</button></form>
<form method="post" action="/accounts/{{.Acc.ID}}/status"><button name="status" value="paused">暂停</button></form>
<form method="post" action="/accounts/{{.Acc.ID}}/run"><button>立即跑一场</button></form>
</td>
</tr>
{{end}}
</tbody></table>
</body></html>
`))
