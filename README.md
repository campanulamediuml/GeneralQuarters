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
`GQ_WEB_DIR`、`GQ_WATCH_HTML`、`GQ_TURN_COMPRESS_INTERVAL`、`GQ_TURN_COMPRESS_BATCH` 可以临时覆盖文件里的值。

数据目录里会有 `generalquarters.db`、启动快照备份 `backups/`（只保留最近 `backup_keep` 份，设为 `0` 就不备份）
和从内嵌资源同步出来的 `content/`。这个目录可以直接挂到卷上。

临时文件（验证脚本、抓图、cookie jar、Chrome profile、临时数据目录）一律放仓库里的 `.gq/tmp/`，
不要写到 `/tmp`。`.gq/` 已经在 `.gitignore` 里：它同时放着本地配置、二进制和会话 cookie，本身就不进库，
所以临时产物随手丢进去即可，用完也不用担心污染提交。

### 改页面不用重启服务

默认形态是"单文件"：`web/dist` 用 `go:embed` 编进二进制，部署只要一个文件，代价是改一行 CSS 都得重新编译。
本地开发时在配置里给两个路径，服务端就改成读盘：

```json
{ "listen": ":8100", "web_dir": "web/dist", "watch_html": "internal/server/watch.html" }
```

于是只有 Go 代码需要重新编译重启，页面都不需要：

- 改 Vue/CSS：`cd web && npm run build`（不到 1 秒）→ 刷新浏览器。想连这一步都省，就挂着
  `npm run watch`（`vite build --watch`），存盘即重建，刷新即可见。
- 改观战页 `internal/server/watch.html`：服务端按 mtime 读盘，保存后刷新就有，连 build 都不用。
- 真要改 .vue 还不想刷新，再 `npm run dev`（Vite 在 5173 热更新，把 `/api`、`/ws`、`/watch`、
  `/content` 代理给 8100）。

两个字段留空就回到内嵌版本（发布形态）。部署/联调统一用 `http://127.0.0.1:8100`。

## 剧本

内置两套剧本，创建房间时用 `scenario_id` 选择：

- `_coral_real`（默认）：珊瑚海 1942 年 5 月，25 海里/hex，双方各一个部署区，适合航母小编队对局。
- `_marianas`：马里亚纳 1944 年 6 月（架空），60×60 图、360 回合上限、`fleet_max_ships` 45；美军 8 艘舰队航母（7 埃塞克斯 + 企业）+ 4 轻母 + 4 个护航航母群 + 第 52 特混舰队 8 艘老战列舰，另有三个登陆运输队要在塞班、天宁、关岛各投送 40 点物资；日军三支机动部队（9 艘航母）+ 栗田第一游击部队 + 岸基航空兵。日军分 `3717`、`1226` 两个部署区，04:00 开局、05:30 开放起飞。

剧本、单位与地图分别在 `content/scenarios/`、`content/units/`、`content/maps/`；新增剧本的字段见 [API.md](API.md) 与 [DESIGN.md](DESIGN.md)。

## Docker

```bash
docker compose up -d
```

`docker-compose.yml` 把宿主机的 `~/GeneralQuarterData` 挂到容器 `/data`，并通过
`generalquarters.docker.json` 提供配置（`GQ_CONFIG=/etc/gq/config.json`）。要换挂载点，
改那份 JSON 的 `data_dir`/`db_path` 即可，不必重写 compose 的环境变量；临时覆盖仍然可以用环境变量。
