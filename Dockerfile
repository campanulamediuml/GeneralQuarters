FROM docker.m.daocloud.io/library/node:20-alpine AS web-build
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM docker.m.daocloud.io/library/golang:1.25-alpine AS go-build
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-build /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go test ./... && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/general-quarters ./cmd/server

FROM docker.m.daocloud.io/library/alpine:3.22
WORKDIR /app
COPY --from=go-build /out/general-quarters /usr/local/bin/general-quarters
# content 和前端产物都不再编进二进制，随镜像放在 /app 下：
# 前端由 web_dir（默认 web/dist）读盘，content 由入口脚本在空卷上铺一份到数据目录。
COPY content /app/content
COPY --from=web-build /app/web/dist /app/web/dist
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
# 环境变量只是"没挂配置文件"时的兜底默认值；正式配置用 -config 或 GQ_CONFIG 指向挂进来的 JSON。
# 注意：改了配置文件里的 listen 端口，也要同步改下面的 HEALTHCHECK 和 compose 的 ports。
ENV GQ_DATA_DIR=/data GQ_DB_PATH=/data/generalquarters.db PORT=8080
USER 1000:1000
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["-config", "/etc/gq/config.json"]
