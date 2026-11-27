package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
	"syscall"
	"time"

	"generalquarters/api"
	"generalquarters/internal/config"
	"generalquarters/internal/server"
	"generalquarters/internal/store"
)

func main() {
	configPath := flag.String("config", "", "配置文件路径（默认读 GQ_CONFIG，其次工作目录下的 generalquarters.json）")
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if cfg.Source != "" {
		log.Printf("使用配置文件 %s", cfg.Source)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		log.Fatal(err)
	}
	if err := verifyContent(cfg.DataDir); err != nil {
		log.Fatal(err)
	}
	if cfg.BackupKeep > 0 {
		if err := backupDatabase(cfg.DataDir, cfg.DBPath, cfg.BackupKeep); err != nil {
			log.Printf("数据库备份跳过：%v", err)
		}
	}
	database, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	mux := http.NewServeMux()
	webDir := cfg.WebDir
	if webDir == "" {
		webDir = "web/dist"
	}
	if info, err := os.Stat(webDir); err != nil || !info.IsDir() {
		log.Fatalf("前端产物目录不存在：%s（先 cd web && ./node_modules/.bin/vite build，或在配置里写 web_dir）", webDir)
	}
	// 观战页是 web/public/watch.html，构建后落在 dist 根目录；和 Vue 页面一样从磁盘读。
	watchPath := cfg.WatchHTML
	if watchPath == "" {
		watchPath = filepath.Join(webDir, "watch.html")
	}
	if _, err := os.Stat(watchPath); err != nil {
		log.Fatalf("观战页模板不存在：%s（前端构建会把 web/public/watch.html 复制进 dist）", watchPath)
	}
	app := server.New(database, cfg.ContentDir(), cfg.SecureCookie)
	app.SetWatchTemplate(watchPath)
	log.Printf("观战页模板从磁盘读：%s", watchPath)
	app.RegisterRoutes(mux)
	// 重启后双方 ready 标记还在库里、却没人再发 ready，回合结算会永远等不到触发。
	runCtx, stopRun := context.WithCancel(context.Background())
	defer stopRun()
	app.StartTurnReconciler(runCtx)
	// 闲置房间超过 12 小时没人来就整间回收：占的是内存和整段战报历史。
	app.StartRoomJanitor(runCtx)
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
	registerSPA(mux, webDir)

	httpServer := &http.Server{Addr: cfg.Listen, Handler: securityHeaders(mux), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
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

// verifyContent 只检查数据目录里的 content 齐不齐，不再从二进制里铺一份。
// 内容（剧本、单位、地图、图片）是纯数据：改 JSON、换图都只动这个目录
// （由仓库 rsync 过来），后端不用重新编译。缺目录时直接拒绝启动，
// 免得起服务之后每个建局请求都 500。
func verifyContent(dataDir string) error {
	contentDir := filepath.Join(dataDir, "content")
	for _, name := range []string{"scenarios", "units", "maps"} {
		info, err := os.Stat(filepath.Join(contentDir, name))
		if err != nil || !info.IsDir() {
			return fmt.Errorf("数据目录里缺少 content/%s：先 rsync -a content/ %s/content/", name, dataDir)
		}
	}
	return nil
}

// backupDatabase 在打开数据库之前留一份一致的快照。
//
// 以前是直接 io.Copy 那个 .db 文件：WAL 里还没 checkpoint 的提交会被漏掉，
// 备份其实是"几分钟前的数据库"；而且每次启动都复制一份、从不清理——
// 开发期反复重启，5.9 GB 的旧备份就这么堆在 data/backups 里。
// 现在用 VACUUM INTO 生成一致快照（包含 WAL 里的内容），只保留最近 backupKeep 份；
// 库文件自上次备份后没被改动过就直接跳过，不白复制一份 45 MB。
func backupDatabase(dataDir, dbPath string, keep int) error {
	info, err := os.Stat(dbPath)
	if err != nil {
		return err
	}
	dir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if newest, ok := newestBackup(dir); ok && !newest.ModTime().Before(info.ModTime()) {
		return pruneBackups(dir, keep)
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
	return pruneBackups(dir, keep)
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

// registerSPA 把前端产物挂到 "/" 上。前端是纯静态资源，一律从磁盘读
// （web_dir，默认 web/dist）：改完 npm run build 刷新即可，不用重新编译后端。
// 目录不存在就直接拒绝启动——比起来之后首页 404 更容易发现问题。
func registerSPA(mux *http.ServeMux, webDir string) {
	if webDir == "" {
		webDir = "web/dist"
	}
	if info, err := os.Stat(webDir); err != nil || !info.IsDir() {
		log.Fatalf("前端产物目录不存在：%s（先 cd web && ./node_modules/.bin/vite build，或在配置里写 web_dir）", webDir)
	}
	source := os.DirFS(webDir)
	log.Printf("前端产物从磁盘读：%s（改完前端刷新即可，不用重启）", webDir)
	files := http.FileServer(http.FS(source))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if _, err := fs.Stat(source, path); err == nil {
				if strings.HasSuffix(path, ".webmanifest") {
					w.Header().Set("Content-Type", "application/manifest+json")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(source, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// index.html 引用的都是带哈希的资源，本身不能缓存，否则改完前端要清浏览器缓存。
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
