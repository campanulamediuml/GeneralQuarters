package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// 前端产物只从磁盘读：测试里搭一个最小的 dist 目录。
func distDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	write("index.html", "<!doctype html><title>gq</title>")
	write("manifest.webmanifest", "{}")
	write("assets/app.js", "console.log(1)")
	return dir
}

func TestManifestContentType(t *testing.T) {
	mux := http.NewServeMux()
	registerSPA(mux, distDir(t))
	request := httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("manifest状态码错误：%d", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/manifest+json" {
		t.Fatalf("manifest Content-Type错误：%s", contentType)
	}
}

// 前端路由（/room/xxx 这类）要回落到 index.html，静态资源则原样返回。
func TestSPAFallsBackToIndex(t *testing.T) {
	mux := http.NewServeMux()
	registerSPA(mux, distDir(t))

	page := httptest.NewRecorder()
	mux.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/room/abc", nil))
	if page.Code != http.StatusOK || page.Body.String() != "<!doctype html><title>gq</title>" {
		t.Fatalf("前端路由没有回落 index：%d %q", page.Code, page.Body.String())
	}

	asset := httptest.NewRecorder()
	mux.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if asset.Code != http.StatusOK || asset.Body.String() != "console.log(1)" {
		t.Fatalf("静态资源没读到：%d %q", asset.Code, asset.Body.String())
	}
}
