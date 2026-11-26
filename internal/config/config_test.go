package config

import (
	"os"
	"path/filepath"
	"testing"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"GQ_CONFIG", "GQ_DATA_DIR", "GQ_DB_PATH", "GQ_LISTEN", "PORT", "GQ_SECURE_COOKIE", "GQ_BACKUP_KEEP", "GQ_TURN_COMPRESS_INTERVAL", "GQ_TURN_COMPRESS_BATCH"} {
		t.Setenv(key, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	// 在临时目录里跑，避免仓库根目录的 generalquarters.json 影响默认值测试。
	chdirTemp(t)
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != "./data" || cfg.DBPath != "data/generalquarters.db" {
		t.Fatalf("默认数据目录错误：%+v", cfg)
	}
	if cfg.Listen != ":8080" || cfg.BackupKeep != 3 {
		t.Fatalf("默认值错误：%+v", cfg)
	}
	if cfg.ContentDir() != filepath.Join("./data", "content") {
		t.Fatalf("content 目录错误：%s", cfg.ContentDir())
	}
}

func TestLoadFileThenEnvOverrides(t *testing.T) {
	clearEnv(t)
	dir := chdirTemp(t)
	path := filepath.Join(dir, "gq.json")
	writeFile(t, path, `{"data_dir":"/srv/gq","listen":"127.0.0.1:9000","backup_keep":5,"secure_cookie":true}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != "/srv/gq" || cfg.Listen != "127.0.0.1:9000" || cfg.BackupKeep != 5 || !cfg.SecureCookie {
		t.Fatalf("配置文件没有生效：%+v", cfg)
	}
	if cfg.DBPath != filepath.Join("/srv/gq", "generalquarters.db") {
		t.Fatalf("db_path 没有按 data_dir 派生：%s", cfg.DBPath)
	}
	if cfg.Source != path {
		t.Fatalf("没有记录配置来源：%q", cfg.Source)
	}

	// 环境变量覆盖文件。
	t.Setenv("GQ_DATA_DIR", "/var/gq")
	t.Setenv("PORT", "7070")
	t.Setenv("GQ_BACKUP_KEEP", "2")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != "/var/gq" {
		t.Fatalf("环境变量没有覆盖 data_dir：%s", cfg.DataDir)
	}
	// 数据目录换了、db_path 还指着旧目录的话就成了两份库，必须重新派生。
	if cfg.DBPath != filepath.Join("/var/gq", "generalquarters.db") {
		t.Fatalf("换 data_dir 后 db_path 没有跟着走：%s", cfg.DBPath)
	}
	if cfg.Listen != ":7070" {
		t.Fatalf("PORT 没有覆盖 listen：%s", cfg.Listen)
	}
	if cfg.BackupKeep != 2 {
		t.Fatalf("备份保留份数没有被覆盖：%d", cfg.BackupKeep)
	}
}

func TestLoadExplicitDBPathWins(t *testing.T) {
	clearEnv(t)
	chdirTemp(t)
	t.Setenv("GQ_DATA_DIR", "/var/gq")
	t.Setenv("GQ_DB_PATH", "/mnt/ssd/gq.db")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBPath != "/mnt/ssd/gq.db" {
		t.Fatalf("显式 db_path 被覆盖了：%s", cfg.DBPath)
	}
}

func TestLoadAutoDiscoversDefaultFile(t *testing.T) {
	clearEnv(t)
	dir := chdirTemp(t)
	writeFile(t, filepath.Join(dir, "generalquarters.json"), `{"data_dir":"/data"}`)
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != "/data" {
		t.Fatalf("没有自动读到 generalquarters.json：%+v", cfg)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	clearEnv(t)
	dir := chdirTemp(t)
	path := filepath.Join(dir, "gq.json")
	writeFile(t, path, `{"data_dirr":"/srv"}`)
	if _, err := Load(path); err == nil {
		t.Fatal("拼错的字段名没有报错")
	}
}

func TestLoadRejectsMissingFile(t *testing.T) {
	clearEnv(t)
	chdirTemp(t)
	if _, err := Load(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("配置文件不存在时没有报错")
	}
}

func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDataDirEnvMovesDBPath(t *testing.T) {
	clearEnv(t)
	dir := chdirTemp(t)
	path := filepath.Join(dir, "gq.json")
	writeFile(t, path, `{"data_dir":"/srv/gq","db_path":"/srv/gq/custom.db"}`)
	t.Setenv("GQ_DATA_DIR", "/var/gq")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBPath != filepath.Join("/var/gq", "generalquarters.db") {
		t.Fatalf("数据目录换了，库还指着旧目录：%s", cfg.DBPath)
	}
	t.Setenv("GQ_DB_PATH", "/mnt/ssd/gq.db")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBPath != "/mnt/ssd/gq.db" {
		t.Fatalf("显式 GQ_DB_PATH 没有优先：%s", cfg.DBPath)
	}
}

func TestLoadBackupKeepZeroDisablesBackups(t *testing.T) {
	clearEnv(t)
	chdirTemp(t)
	t.Setenv("GQ_BACKUP_KEEP", "0")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BackupKeep != 0 {
		t.Fatalf("backup_keep=0 应当保留为 0（不备份），实际 %d", cfg.BackupKeep)
	}
}
