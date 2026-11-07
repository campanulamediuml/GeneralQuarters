package main

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"generalquarters/api"
	"generalquarters/content"
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
		if _, err := os.Stat(target); err == nil {
			return nil
		}
		source, err := content.Files.Open(path)
		if err != nil {
			return err
		}
		defer source.Close()
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(file, source)
		return err
	})
}

func backupDatabase(dataDir, dbPath string) error {
	if _, err := os.Stat(dbPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "backups"), 0o750); err != nil {
		return err
	}
	source, err := os.Open(dbPath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.Create(filepath.Join(dataDir, "backups", "generalquarters-"+time.Now().UTC().Format("20060102T150405Z")+".db"))
	if err != nil {
		return err
	}
	defer target.Close()
	_, err = io.Copy(target, source)
	return err
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
