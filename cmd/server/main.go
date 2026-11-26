package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"generalquarters/api"
	"generalquarters/content"
	_ "modernc.org/sqlite"
	"generalquarters/internal/server"
	"generalquarters/internal/store"
	webui "generalquarters/web"
)

func main() {
	dataDir := env("GQ_DATA_DIR", "./data")
	dbPath := env("GQ_DB_PATH", filepath.Join(dataDir, "generalquarters.db"))
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		log.Fatal(err)
	}
	if err := ensureContent(dataDir); err != nil {
		log.Fatal(err)
	}
	if err := backupDatabase(dataDir, dbPath); err != nil {
		log.Printf("数据库备份跳过：%v", err)
	}
	database, err := store.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	mux := http.NewServeMux()
	app := server.New(database, filepath.Join(dataDir, "content"), os.Getenv("GQ_SECURE_COOKIE") == "true")
	app.RegisterRoutes(mux)
	// 重启后双方 ready 标记还在库里、却没人再发 ready，回合结算会永远等不到触发。
	runCtx, stopRun := context.WithCancel(context.Background())
	defer stopRun()
	app.StartTurnReconciler(runCtx)
	// 回放用的回合快照在后台压缩：写入路径只写原样 JSON，worker 负责把已结束的回合压成 gzip。
	database.StartTurnCompressor(runCtx, 0, 0)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /api/v1/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(api.OpenAPI)
	})
	mux.HandleFunc("GET /api/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(api.Docs)
	})
	registerSPA(mux)

	httpServer := &http.Server{Addr: ":" + env("PORT", "8080"), Handler: securityHeaders(mux), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	go func() {
		log.Printf("General Quarters listening on %s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	stopRun()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func ensureContent(dataDir string) error {
	return fs.WalkDir(content.Files, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dataDir, "content", path)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		// 仓库里的 content 是唯一真相：数据目录里的副本与内嵌内容不一致就覆盖，
		// 这样改完 units/scenarios 的 JSON 重启即可生效，不必删库重建房间。
		if current, err := os.ReadFile(target); err == nil {
			if embedded, readErr := content.Files.ReadFile(path); readErr == nil && bytes.Equal(current, embedded) {
				return nil
			}
		}
		source, err := content.Files.Open(path)
		if err != nil {
			return err
		}
		defer source.Close()
		// 已存在且内容不同的文件要就地覆盖，所以不能用 O_EXCL。
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o640)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(file, source)
		return err
	})
}

const backupKeep = 3

// backupDatabase 在打开数据库之前留一份一致的快照。
//
// 以前是直接 io.Copy 那个 .db 文件：WAL 里还没 checkpoint 的提交会被漏掉，
// 备份其实是"几分钟前的数据库"；而且每次启动都复制一份、从不清理——
// 开发期反复重启，5.9 GB 的旧备份就这么堆在 data/backups 里。
// 现在用 VACUUM INTO 生成一致快照（包含 WAL 里的内容），只保留最近 backupKeep 份；
// 库文件自上次备份后没被改动过就直接跳过，不白复制一份 45 MB。
func backupDatabase(dataDir, dbPath string) error {
	info, err := os.Stat(dbPath)
	if err != nil {
		return err
	}
	dir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if newest, ok := newestBackup(dir); ok && !newest.ModTime().Before(info.ModTime()) {
		return pruneBackups(dir, backupKeep)
	}
	target := uniqueBackupPath(dir)
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", dbPath))
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec("VACUUM INTO " + quoteSQLString(target)); err != nil {
		_ = os.Remove(target)
		return err
	}
	return pruneBackups(dir, backupKeep)
}

func quoteSQLString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func uniqueBackupPath(dir string) string {
	base := "generalquarters-" + time.Now().UTC().Format("20060102T150405Z")
	path := filepath.Join(dir, base+".db")
	for index := 1; ; index++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%d.db", base, index))
	}
}

// newestBackup 返回目录里最新的一份备份（按文件名里的时间戳排序，秒级）。
func newestBackup(dir string) (os.FileInfo, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	var newest os.FileInfo
	for _, entry := range entries {
		if entry.IsDir() || !backupName(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if newest == nil || info.ModTime().After(newest.ModTime()) {
			newest = info
		}
	}
	return newest, newest != nil
}

func backupName(name string) bool {
	return strings.HasPrefix(name, "generalquarters-") && strings.HasSuffix(name, ".db")
}

// pruneBackups 只留最近 keep 份，避免备份目录无限膨胀。
func pruneBackups(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	files := make([]os.FileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !backupName(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, info)
	}
	if len(files) <= keep {
		return nil
	}
	sort.Slice(files, func(a, b int) bool { return files[a].ModTime().After(files[b].ModTime()) })
	for _, info := range files[keep:] {
		if err := os.Remove(filepath.Join(dir, info.Name())); err != nil {
			return err
		}
	}
	return nil
}

func registerSPA(mux *http.ServeMux) {
	dist, err := fs.Sub(webui.Dist, "dist")
	if err != nil {
		log.Fatal(err)
	}
	files := http.FileServer(http.FS(dist))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if _, err := fs.Stat(dist, path); err == nil {
				if strings.HasSuffix(path, ".webmanifest") {
					w.Header().Set("Content-Type", "application/manifest+json")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		index, _ := fs.ReadFile(dist, "index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
