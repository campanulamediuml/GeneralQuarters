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
ENV GQ_DATA_DIR=/data GQ_DB_PATH=/data/generalquarters.db PORT=8080
USER 1000:1000
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/general-quarters"]
