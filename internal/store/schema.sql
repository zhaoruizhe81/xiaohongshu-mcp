-- 养号引擎持久化（SQLite）。时间统一存 Unix 秒。所有表 IF NOT EXISTS，可反复执行。

CREATE TABLE IF NOT EXISTS accounts (
  id             TEXT PRIMARY KEY,
  nickname       TEXT NOT NULL DEFAULT '',
  red_id         TEXT NOT NULL DEFAULT '',
  first_login_at INTEGER NOT NULL DEFAULT 0,   -- 首次登录时间(算养号天数)
  stage          INTEGER NOT NULL DEFAULT 0,    -- 0Observe 1Light 2Deepen 3Social 4Creator
  status         TEXT NOT NULL DEFAULT 'active',-- active/paused/cooldown
  cooldown_until INTEGER NOT NULL DEFAULT 0,
  risk_score     INTEGER NOT NULL DEFAULT 0,
  keywords       TEXT NOT NULL DEFAULT '[]',    -- 人设兴趣词 JSON
  persona        TEXT NOT NULL DEFAULT '',       -- 语言风格(评论用)
  created_at     INTEGER NOT NULL DEFAULT 0,
  updated_at     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS daily_usage (
  account_id    TEXT NOT NULL,
  day           TEXT NOT NULL,                  -- YYYY-MM-DD
  used_like     INTEGER NOT NULL DEFAULT 0,
  used_favorite INTEGER NOT NULL DEFAULT 0,
  used_comment  INTEGER NOT NULL DEFAULT 0,
  used_search   INTEGER NOT NULL DEFAULT 0,
  sessions      INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, day)
);

CREATE TABLE IF NOT EXISTS interactions (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id  TEXT NOT NULL,
  note_id     TEXT NOT NULL,
  author_id   TEXT NOT NULL DEFAULT '',
  action      TEXT NOT NULL,                    -- like/favorite/comment
  note_title  TEXT NOT NULL DEFAULT '',
  note_url    TEXT NOT NULL DEFAULT '',
  content     TEXT NOT NULL DEFAULT '',
  result      TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL DEFAULT 0,
  UNIQUE (account_id, note_id, action)          -- 去重：同号同笔记同动作只一次
);

CREATE TABLE IF NOT EXISTS comment_jobs (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id   TEXT NOT NULL,
  note_id      TEXT NOT NULL,
  note_excerpt TEXT NOT NULL DEFAULT '',
  generated    TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'pending', -- pending/approved/rejected/posted
  created_at   INTEGER NOT NULL DEFAULT 0,
  posted_at    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS risk_events (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id TEXT NOT NULL,
  type       TEXT NOT NULL,                     -- captcha/ratelimit/login_fail/panic
  detail     TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS sessions (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id  TEXT NOT NULL,
  started_at  INTEGER NOT NULL DEFAULT 0,
  ended_at    INTEGER NOT NULL DEFAULT 0,
  browse_cnt  INTEGER NOT NULL DEFAULT 0,
  open_cnt    INTEGER NOT NULL DEFAULT 0,
  like_cnt    INTEGER NOT NULL DEFAULT 0,
  fav_cnt     INTEGER NOT NULL DEFAULT 0,
  comment_cnt INTEGER NOT NULL DEFAULT 0,
  outcome     TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_interactions_acc ON interactions(account_id, created_at);
CREATE INDEX IF NOT EXISTS idx_risk_acc ON risk_events(account_id, created_at);
CREATE INDEX IF NOT EXISTS idx_comment_status ON comment_jobs(status, created_at);
