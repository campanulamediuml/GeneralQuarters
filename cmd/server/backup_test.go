package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestPruneBackupsKeepsNewest 覆盖备份保留策略：开发期反复重启时，
// 以前每启动一次就复制一份整个数据库、从不清理（5.9 GB 的旧备份就是这么堆的）。
func TestPruneBackupsKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	for index := 0; index < 6; index++ {
		name := filepath.Join(dir, "generalquarters-20260101T00000"+string(rune('0'+index))+"Z.db")
		if err := os.WriteFile(name, []byte("x"), 0o640); err != nil {
			t.Fatal(err)
		}
		stamp := base.Add(time.Duration(index) * time.Minute)
		if err := os.Chtimes(name, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	// 非备份文件不能被误删。
	keep := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := pruneBackups(dir, 3); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	backups := 0
	for _, entry := range entries {
		if backupName(entry.Name()) {
			backups++
		}
	}
	if backups != 3 {
		t.Fatalf("保留份数错误：%d", backups)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("非备份文件被删除了")
	}
}

// TestBackupDatabaseSkipsUnchangedAndSnapshotsChanges 验证两件事：
// 库没改动时不重复复制；有改动时新备份能读出一致内容（VACUUM INTO 而不是裸复制 .db）。
func TestBackupDatabaseSkipsUnchangedAndSnapshotsChanges(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "generalquarters.db")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE rooms (id TEXT PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO rooms (id, name) VALUES ('r1', '珊瑚海')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := backupDatabase(dir, dbPath, 3); err != nil {
		t.Fatal(err)
	}
	count := func() int {
		matches, err := filepath.Glob(filepath.Join(dir, "backups", "generalquarters-*.db"))
		if err != nil {
			t.Fatal(err)
		}
		return len(matches)
	}
	if got := count(); got != 1 {
		t.Fatalf("第一次备份没有生成：%d", got)
	}
	// 没有新写入：再来一次不应产生新备份。
	if err := backupDatabase(dir, dbPath, 3); err != nil {
		t.Fatal(err)
	}
	if got := count(); got != 1 {
		t.Fatalf("库没改动却又备份了一次：%d", got)
	}
	// 写入之后应当再备份一份，而且快照里能读到新行。
	db, err = sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO rooms (id, name) VALUES ('r2', '马里亚纳')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := backupDatabase(dir, dbPath, 3); err != nil {
		t.Fatal(err)
	}
	if got := count(); got != 2 {
		t.Fatalf("改动后没有新增备份：%d", got)
	}
	newest, ok := newestBackup(filepath.Join(dir, "backups"))
	if !ok {
		t.Fatal("找不到最新备份")
	}
	snapshot, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "backups", newest.Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	var rows int
	if err := snapshot.QueryRow(`SELECT count(*) FROM rooms`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("备份内容不是最新：%d 行", rows)
	}
}
