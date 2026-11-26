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

## Docker

```bash
docker compose up -d
```

`docker-compose.yml` 把宿主机的 `~/GeneralQuarterData` 挂到容器 `/data`，并通过
`generalquarters.docker.json` 提供配置（`GQ_CONFIG=/etc/gq/config.json`）。要换挂载点，
改那份 JSON 的 `data_dir`/`db_path` 即可，不必重写 compose 的环境变量；临时覆盖仍然可以用环境变量。
