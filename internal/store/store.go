package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动，无需 cgo
)

//go:embed schema.sql
var schemaSQL string

// Store 养号持久化入口（一个 SQLite 文件）。
type Store struct {
	db *sql.DB
}

// Open 打开/创建数据库并建表。
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开 sqlite 失败: %w", err)
	}
	// 单连接，避免 SQLite 写锁竞争（养号是单/少量并发，够用）
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("设置 pragma 失败: %w", err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("建表失败: %w", err)
	}
	// 旧库迁移：缺 persona 列则补上（忽略"已存在"错误）
	_, _ = db.Exec(`ALTER TABLE accounts ADD COLUMN persona TEXT NOT NULL DEFAULT ''`)
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func nowUnix() int64 { return time.Now().Unix() }

// ============ 账号 ============

type Account struct {
	ID            string
	Nickname      string
	RedID         string
	FirstLoginAt  time.Time
	Stage         int
	Status        string
	CooldownUntil time.Time
	RiskScore     int
	Keywords      string // JSON 字符串
	Persona       string // 语言风格(评论用)
}

// EnsureAccount 不存在则建一条默认账号并返回。
func (s *Store) EnsureAccount(id string) (*Account, error) {
	acc, err := s.GetAccount(id)
	if err == nil {
		return acc, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	now := nowUnix()
	_, err = s.db.Exec(
		`INSERT INTO accounts(id,nickname,first_login_at,status,created_at,updated_at)
		 VALUES(?,?,?,?,?,?)`,
		id, "", now, "active", now, now,
	)
	if err != nil {
		return nil, err
	}
	return s.GetAccount(id)
}

func (s *Store) GetAccount(id string) (*Account, error) {
	row := s.db.QueryRow(
		`SELECT id,nickname,red_id,first_login_at,stage,status,cooldown_until,risk_score,keywords,persona
		 FROM accounts WHERE id=?`, id)
	return scanAccount(row)
}

func (s *Store) ListAccounts() ([]*Account, error) {
	rows, err := s.db.Query(
		`SELECT id,nickname,red_id,first_login_at,stage,status,cooldown_until,risk_score,keywords,persona
		 FROM accounts ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type rowScanner interface{ Scan(...any) error }

func scanAccount(r rowScanner) (*Account, error) {
	var a Account
	var first, cd int64
	if err := r.Scan(&a.ID, &a.Nickname, &a.RedID, &first, &a.Stage, &a.Status, &cd, &a.RiskScore, &a.Keywords, &a.Persona); err != nil {
		return nil, err
	}
	a.FirstLoginAt = time.Unix(first, 0)
	a.CooldownUntil = time.Unix(cd, 0)
	return &a, nil
}

func (s *Store) SetStage(id string, stage int) error {
	_, err := s.db.Exec(`UPDATE accounts SET stage=?,updated_at=? WHERE id=?`, stage, nowUnix(), id)
	return err
}

// SetPersona 设置账号语言风格（评论用）。
func (s *Store) SetPersona(id, persona string) error {
	_, err := s.db.Exec(`UPDATE accounts SET persona=?,updated_at=? WHERE id=?`, persona, nowUnix(), id)
	return err
}

// UpdateKeywords 设置账号兴趣关键词（JSON 字符串）。
func (s *Store) UpdateKeywords(id, keywords string) error {
	_, err := s.db.Exec(`UPDATE accounts SET keywords=?,updated_at=? WHERE id=?`, keywords, nowUnix(), id)
	return err
}

func (s *Store) SetStatus(id, status string) error {
	_, err := s.db.Exec(`UPDATE accounts SET status=?,updated_at=? WHERE id=?`, status, nowUnix(), id)
	return err
}

// SetCooldown 设置冷却截止时间（unix 秒，0 表示无冷却）。
func (s *Store) SetCooldown(id string, until time.Time) error {
	_, err := s.db.Exec(`UPDATE accounts SET cooldown_until=?,updated_at=? WHERE id=?`, until.Unix(), nowUnix(), id)
	return err
}

// AddRiskScore 累加风险分并返回新值。
func (s *Store) AddRiskScore(id string, delta int) (int, error) {
	if _, err := s.db.Exec(`UPDATE accounts SET risk_score=risk_score+?,updated_at=? WHERE id=?`, delta, nowUnix(), id); err != nil {
		return 0, err
	}
	var v int
	err := s.db.QueryRow(`SELECT risk_score FROM accounts WHERE id=?`, id).Scan(&v)
	return v, err
}

// ============ 每日配额 ============

type DailyUsage struct {
	AccountID    string
	Day          string
	UsedLike     int
	UsedFavorite int
	UsedComment  int
	UsedSearch   int
	Sessions     int
}

// GetDaily 取某天配额（没有则返回全 0 的行，不落库）。
func (s *Store) GetDaily(accountID, day string) (*DailyUsage, error) {
	u := &DailyUsage{AccountID: accountID, Day: day}
	row := s.db.QueryRow(
		`SELECT used_like,used_favorite,used_comment,used_search,sessions
		 FROM daily_usage WHERE account_id=? AND day=?`, accountID, day)
	err := row.Scan(&u.UsedLike, &u.UsedFavorite, &u.UsedComment, &u.UsedSearch, &u.Sessions)
	if err == sql.ErrNoRows {
		return u, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// IncrUsage 今日某类用量 +n（field 仅限白名单列名，内部调用已固定）。
func (s *Store) IncrUsage(accountID, day, field string, n int) error {
	col, ok := usageColumns[field]
	if !ok {
		return fmt.Errorf("非法用量字段: %s", field)
	}
	_, err := s.db.Exec(
		`INSERT INTO daily_usage(account_id,day,`+col+`) VALUES(?,?,?)
		 ON CONFLICT(account_id,day) DO UPDATE SET `+col+`=`+col+`+excluded.`+col,
		accountID, day, n,
	)
	return err
}

var usageColumns = map[string]string{
	"like": "used_like", "favorite": "used_favorite",
	"comment": "used_comment", "search": "used_search", "sessions": "sessions",
}

// ============ 互动（去重审计）============

// AddInteraction 记录一次互动；已存在(同号同笔记同动作)返回 false。
func (s *Store) AddInteraction(accountID, noteID, authorID, action, title, url, content, result string) (bool, error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO interactions(account_id,note_id,author_id,action,note_title,note_url,content,result,created_at)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		accountID, noteID, authorID, action, title, url, content, result, nowUnix(),
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// HasInteraction 判断是否已做过该动作（去重用）。
func (s *Store) HasInteraction(accountID, noteID, action string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(1) FROM interactions WHERE account_id=? AND note_id=? AND action=?`,
		accountID, noteID, action).Scan(&n)
	return n > 0, err
}

// RecentComments 取该号最近的若干条评论内容（用于生成时去重）。
func (s *Store) RecentComments(accountID string, limit int) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT content FROM interactions
		 WHERE account_id=? AND action='comment' AND content<>''
		 ORDER BY id DESC LIMIT ?`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ============ 风险事件 ============

func (s *Store) AddRiskEvent(accountID, typ, detail string) error {
	_, err := s.db.Exec(
		`INSERT INTO risk_events(account_id,type,detail,created_at) VALUES(?,?,?,?)`,
		accountID, typ, detail, nowUnix())
	return err
}

// ============ 会话记录 ============

// StartSession 建一条会话记录，返回 id。
func (s *Store) StartSession(accountID string) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO sessions(account_id,started_at) VALUES(?,?)`, accountID, nowUnix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// EndSession 回填会话结果（计数与结局）。
func (s *Store) EndSession(id int64, browse, open, like, fav, comment int, outcome string) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET ended_at=?,browse_cnt=?,open_cnt=?,like_cnt=?,fav_cnt=?,comment_cnt=?,outcome=? WHERE id=?`,
		nowUnix(), browse, open, like, fav, comment, outcome, id)
	return err
}
