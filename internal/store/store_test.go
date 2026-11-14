package store

import (
	"path/filepath"
	"testing"
)

// 验证 store 建表与核心读写：账号/阶段/配额累加/互动去重/会话记录。
func TestStoreBasics(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	acc, err := st.EnsureAccount("a1")
	if err != nil || acc.ID != "a1" || acc.Status != "active" {
		t.Fatalf("ensure account: %+v %v", acc, err)
	}

	if err := st.SetStage("a1", 1); err != nil {
		t.Fatal(err)
	}
	if a, _ := st.GetAccount("a1"); a.Stage != 1 {
		t.Fatalf("stage want 1 got %d", a.Stage)
	}

	// 配额累加（同一行 upsert 累加）
	_ = st.IncrUsage("a1", "2026-11-14", "like", 2)
	_ = st.IncrUsage("a1", "2026-11-14", "like", 3)
	if u, _ := st.GetDaily("a1", "2026-11-14"); u.UsedLike != 5 {
		t.Fatalf("like want 5 got %d", u.UsedLike)
	}

	// 互动去重：第二次同号同笔记同动作应插入失败
	ins1, err := st.AddInteraction("a1", "note1", "auth", "like", "标题", "url", "", "ok")
	if err != nil || !ins1 {
		t.Fatalf("first insert: %v %v", ins1, err)
	}
	ins2, _ := st.AddInteraction("a1", "note1", "auth", "like", "标题", "url", "", "ok")
	if ins2 {
		t.Fatal("dedup 失效：第二次仍插入成功")
	}

	// 会话
	sid, err := st.StartSession("a1")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EndSession(sid, 1, 0, 1, 0, 0, "done"); err != nil {
		t.Fatal(err)
	}
}
