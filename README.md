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
和 `content/`（剧本、单位、地图、图片）。这个目录可以直接挂到卷上。

`content/` 和前端 `web/dist` 都**不再编进二进制**，服务端启动时只检查它们存在：

```bash
mkdir -p data && rsync -a content/ data/content/
cd web && npm run build && cd ..
go run ./cmd/server -config ./generalquarters.json
```

缺 `content/scenarios|units|maps` 会直接拒绝启动并提示从仓库复制；缺前端产物同理。
好处是改剧本 JSON、换兵牌图片都不用重编译后端：图片由 `/content/` 静态目录直接读盘（刷新即可），
JSON 在每次建局/读取时从磁盘加载（新开房间就生效）。

临时文件（验证脚本、抓图、cookie jar、Chrome profile、临时数据目录）一律放仓库里的 `.gq/tmp/`，
不要写到 `/tmp`。`.gq/` 已经在 `.gitignore` 里：它同时放着本地配置、二进制和会话 cookie，本身就不进库，
所以临时产物随手丢进去即可，用完也不用担心污染提交。

### 改页面不用重启服务

前端产物一律从磁盘读（`web_dir`，不填就是 `web/dist`），只有 Go 代码需要重新编译重启。
本地配置一般长这样：

```json
{ "listen": ":8100", "web_dir": "web/dist" }
```

页面相关的改动都不用重启后端：

- 改 Vue/CSS：`cd web && npm run build`（不到 1 秒）→ 刷新浏览器。想连这一步都省，就挂着
  `npm run watch`（`vite build --watch`），存盘即重建，刷新即可见。
- 改观战页：和普通页面一样是 `/watch/:token` 路由（`views/WatchView.vue`），复用海图和兵牌，改完刷新即可。
- 真要改 .vue 还不想刷新，再 `npm run dev`（Vite 在 5173 热更新，把 `/api`、`/ws`、`/watch`、
  `/content` 代理给 8100）。

`web_dir` 留空时按 `web/dist` 处理；目录不存在会拒绝启动。观战链接是 `/watch/{token}`，
页面调 `/api/v1/watch/{token}` 解析房间，之后连 `/ws?mode=spectator` 收双方完整快照。部署/联调统一用 `http://127.0.0.1:8100`。

## 剧本

内置两套剧本，创建房间时用 `scenario_id` 选择：

- `_coral_real`（默认）：珊瑚海 1942 年 5 月，25 海里/hex，双方各一个部署区，适合航母小编队对局。
- `_marianas`：马里亚纳 1944 年 6 月（架空），89×60 图、360 回合上限、`fleet_max_ships` 45；美军 8 艘舰队航母（7 埃塞克斯 + 企业）+ 4 轻母 + 4 个护航航母群 + 第 52 特混舰队 8 艘老战列舰，另有三个登陆运输队要在塞班、天宁、关岛各投送 40 点物资；日军四支航母群（17 艘航母：甲/乙/丙/丁）+ 栗田第一游击部队（10 艘战列舰，含架空复活的陆奥）+ 岸基航空兵，共 70 艘。日军分 `1023`（甲/乙/丙）、`4221`（丁/栗田）两个部署区，04:00 开局、05:30 开放起飞。两个剧本的完整简报、编成与结算规则见 [剧本附录](剧本/README.md)。

剧本、单位与地图分别在 `content/scenarios/`、`content/units/`、`content/maps/`；新增剧本的字段见 [实现注记.md](实现注记.md) 与 [API.md](API.md)。

## 文档

- [规则书.md](规则书.md)：规则本身——判定怎么做、数值怎么取、边界条件是什么。
- [剧本附录](剧本/README.md)：珊瑚海与马里亚纳的简报、胜负结算、舰队与航空编成、机场、部署区和特殊地形。
- [实现注记.md](实现注记.md)：调档记录、规则到代码的对应、剧本字段写法、内容管线。
- [DESIGN.md](DESIGN.md)：联机底座与前端结构；[API.md](API.md)：命令与快照字段；[TESTING.md](TESTING.md)：测试约定。

## DSH 双模型对战

DSH 插件位于 `tools/dsh-generalquarters`，CLI 入口为 `cmd/gqcli`。每个 DSH session 按 `exec.agent.session.id` 启动独立 CLI，分别保存 Cookie、WebSocket、全量私有快照、事件和命令日志；模型只能使用 `gq_create/join/state/command/ready/wait`，不能访问观战接口或任意 RPC。

构建并安装到 DSH Web profile：

```bash
mkdir -p .gq/bin
go build -o .gq/bin/gqcli ./cmd/gqcli
export GQ_CLI_PATH="$PWD/.gq/bin/gqcli"
export GQ_SERVER="http://127.0.0.1:8100"   # 按实际服务端地址调整
dsh plugin --profile web add "$PWD/tools/dsh-generalquarters"
```

重启 DSH 后，A session 调用 `gq_create` 并以阵营 A 执行 `gq_join`，把建房返回的同一个 token 交给 B session，由 B 以阵营 B 加入。不要在两个 session 之间转发 `gq_state` 或 `gq_wait` 的内容。

CLI 收到 `state.patch` 后只把它当作变更通知，再拉取完整 `state` 替换本地快照，不实现补丁合并。`gq_wait` 返回变化 section 和新增战报；`gq_state` 默认返回摘要，可按 section 和实体 ID 查询，避免把完整地图和航空状态反复放进模型上下文。所有模型输出都会移除 `join_token`。

插件在 DSH 工作目录的 `.gq/dsh/<session-id>/` 下保存数据；CLI 独立运行时通过 `-state-dir` 指定目录，恢复身份时必须复用原目录，不能重新 join。`session.json` 含玩家 Cookie，`snapshot.json` 含原始私有视图，均按仅当前用户可读写保存，不应提供给模型或其他 session。

```bash
# CLI 的 stdio 请求/响应均为一行 JSON；stdout 不写日志
printf '%s\n' '{"id":"1","method":"gq_state","params":{}}' | \
  .gq/bin/gqcli -server http://127.0.0.1:8100 -state-dir .gq/dsh/player-a
```

插件桥测试：

```bash
cd tools/dsh-generalquarters && npm test
```

## Docker

```bash
docker compose up -d
```

`docker-compose.yml` 把宿主机的 `~/GeneralQuarterData` 挂到容器 `/data`，并通过
`generalquarters.docker.json` 提供配置（`GQ_CONFIG=/etc/gq/config.json`）。要换挂载点，
改那份 JSON 的 `data_dir`/`db_path` 即可，不必重写 compose 的环境变量；临时覆盖仍然可以用环境变量。
