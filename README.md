# GeneralQuarters

回合制海战沙盒：Go 服务端（结算、权威状态、WebSocket）+ Vue 前端（海图与算子），数据落在 SQLite。

## 运行

```bash
go test ./...
go run ./cmd/server            # 数据默认写到 ./data，监听 :8080
go run ./cmd/server -config ./generalquarters.json
```

配置优先级：默认值 < 配置文件 < 环境变量。配置文件按 `-config` → `GQ_CONFIG` → 当前目录的
`generalquarters.json` 依次查找，字段见 [`generalquarters.example.json`](generalquarters.example.json)；
`GQ_DATA_DIR`、`GQ_DB_PATH`、`GQ_LISTEN`、`PORT`、`GQ_SECURE_COOKIE`、`GQ_BACKUP_KEEP`、
`GQ_TURN_COMPRESS_INTERVAL`、`GQ_TURN_COMPRESS_BATCH` 可以临时覆盖文件里的值。

数据目录里会有 `generalquarters.db`、启动快照备份 `backups/`（只保留最近 `backup_keep` 份，设为 `0` 就不备份）
和从内嵌资源同步出来的 `content/`。这个目录可以直接挂到卷上。

临时文件（验证脚本、抓图、cookie jar、Chrome profile、临时数据目录）一律放仓库里的 `.gq/tmp/`，
不要写到 `/tmp`。`.gq/` 已经在 `.gitignore` 里：它同时放着本地配置、二进制和会话 cookie，本身就不进库，
所以临时产物随手丢进去即可，用完也不用担心污染提交。

### 前端和服务分开跑（改页面不用重启后端）

默认形态是"单文件"：`web/dist` 用 `go:embed` 编进二进制，部署只要一个文件。改页面就得重新编译，
联调很烦。开发时拆成两个进程，各改各的：

```bash
# 后端：只出 API / WebSocket / 观战页，监听 :8100
./.gq/gq-server -config .gq/config.json     # 配置里 listen = ":8100"

# 前端：vite dev server 占 :8099，热更新，把 /api /ws /watch /content 代理给 :8100
cd web && npm run dev                        # 后端不在 8100 就 GQ_API=http://127.0.0.1:9000 npm run dev
```

浏览器打开 `http://127.0.0.1:8099`：`.vue` 改一行立刻热更新，不用编译、不用重启 Go。
观战页（`internal/server/watch.html`）是独立的静态页，由 Go 直接发——配置文件里给它一个
`watch_html` 路径，服务端就按 mtime 读盘，改完刷新浏览器就生效；`web_dir` 同理，指到
`web/dist` 后 `npm run build` 的产物直接生效，也不用重启。两项留空就回到内嵌版本（发布形态）。

## Docker

```bash
docker compose up -d
```

`docker-compose.yml` 把宿主机的 `~/GeneralQuarterData` 挂到容器 `/data`，并通过
`generalquarters.docker.json` 提供配置（`GQ_CONFIG=/etc/gq/config.json`）。要换挂载点，
改那份 JSON 的 `data_dir`/`db_path` 即可，不必重写 compose 的环境变量；临时覆盖仍然可以用环境变量。
