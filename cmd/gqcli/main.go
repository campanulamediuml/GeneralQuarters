package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"generalquarters/internal/gqcli"
)

func main() {
	serverURL := flag.String("server", os.Getenv("GQ_SERVER"), "GQ 服务端 HTTP 地址，默认 http://127.0.0.1:8080")
	stateDir := flag.String("state-dir", os.Getenv("GQ_CLI_STATE_DIR"), "此玩家独立的状态目录；恢复会话时必须复用原目录")
	flag.Parse()
	if *stateDir == "" {
		*stateDir = filepath.Join(".gq", "dsh", fmt.Sprintf("cli-%d", os.Getpid()))
	}
	logger := log.New(os.Stderr, "gqcli: ", log.LstdFlags)
	client, err := gqcli.NewClient(gqcli.Config{ServerURL: *serverURL, StateDir: *stateDir, Logger: logger})
	if err != nil {
		logger.Fatal(err)
	}
	defer client.Close()
	logger.Printf("状态目录：%s", *stateDir)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go func() {
		<-ctx.Done()
		_ = os.Stdin.Close()
	}()
	if err := gqcli.Serve(ctx, client, os.Stdin, os.Stdout); err != nil && ctx.Err() == nil {
		logger.Fatal(err)
	}
}
