package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 观战页模板给了磁盘路径之后，改文件再请求就应该拿到新内容，不用重启进程。
// 这是"页面和服务分开"的核心：改 HTML 和改 Vue 一样，刷新就够。
func TestWatchTemplateReloadsFromDiskWithoutRestart(t *testing.T) {
	template := filepath.Join(t.TempDir(), "watch.html")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(template, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("<html><body>spectator v1</body></html>")
	client, _, _, cleanup := setupTestServer(t, func(app *Server) { app.SetWatchTemplate(template) })
	defer cleanup()

	if body, status := fetchWatchPage(t, client, "/watch/room?token=x"); status != 200 || !strings.Contains(body, "spectator v1") {
		t.Fatalf("首次读取没有用磁盘模板：status=%d body=%s", status, head(body, 200))
	}

	write("<html><body>spectator v2 带红线</body></html>")
	if body, status := fetchWatchPage(t, client, "/watch/room?token=x"); status != 200 || !strings.Contains(body, "spectator v2 带红线") {
		t.Fatalf("改完模板后没有重新读盘：status=%d body=%s", status, head(body, 200))
	}
}

// 模板路径读不到时直接 500：页面不再有内嵌兜底，部署漏拷 watch.html 要当场暴露，
// 而不是悄悄发一份旧页面出去。
func TestWatchTemplateMissingReturnsError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.html")
	client, _, _, cleanup := setupTestServer(t, func(app *Server) { app.SetWatchTemplate(missing) })
	defer cleanup()
	if body, status := fetchWatchPage(t, client, "/watch/room?token=x"); status != 500 {
		t.Fatalf("模板缺失时状态码 %d，期望 500：%s", status, head(body, 200))
	}
}
