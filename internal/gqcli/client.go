package gqcli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type Config struct {
	ServerURL string
	StateDir  string
	Logger    *log.Logger
}

type Client struct {
	cfg      Config
	logger   *log.Logger
	http     *http.Client
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup

	mu                sync.Mutex
	session           *Session
	snapshot          map[string]json.RawMessage
	snapshotRaw       json.RawMessage
	revision          uint64
	deliveredRevision uint64
	lastEventID       int64
	waitEventID       int64
	waitConnected     bool
	connected         bool
	synced            bool
	conn              *wsConn
	changeCh          chan struct{}
	pendingSections   map[string]bool
	pendingResync     bool
	eventUncertain    bool
	commands          map[string]*commandRecord

	connectMu   sync.Mutex
	refreshMu   sync.Mutex
	commandMu   sync.Mutex
	refreshCh   chan struct{}
	reconnectCh chan struct{}
}

type wsConn struct {
	conn      *websocket.Conn
	writeMu   sync.Mutex
	pendingMu sync.Mutex
	pending   map[string]chan rpcResult
	done      chan struct{}
	closeOnce sync.Once
	nextID    atomic.Uint64
}

type rpcResult struct {
	response rpcResponse
	err      error
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.ServerURL == "" {
		cfg.ServerURL = "http://127.0.0.1:8080"
	}
	parsed, err := url.Parse(cfg.ServerURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("无效服务端地址 %q", cfg.ServerURL)
	}
	cfg.ServerURL = strings.TrimRight(cfg.ServerURL, "/")
	if cfg.StateDir == "" {
		return nil, fmt.Errorf("状态目录不能为空")
	}
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建状态目录：%w", err)
	}
	if cfg.Logger == nil {
		cfg.Logger = log.New(os.Stderr, "gqcli: ", log.LstdFlags)
	}
	client := &Client{
		cfg:             cfg,
		logger:          cfg.Logger,
		http:            &http.Client{Timeout: 15 * time.Second},
		stopCh:          make(chan struct{}),
		changeCh:        make(chan struct{}),
		pendingSections: map[string]bool{},
		commands:        map[string]*commandRecord{},
		refreshCh:       make(chan struct{}, 1),
		reconnectCh:     make(chan struct{}, 1),
	}
	client.loadSession()
	client.loadSnapshot()
	client.loadCommands()
	client.wg.Add(2)
	go client.maintainConnection()
	go client.refreshLoop()
	return client, nil
}

func (c *Client) Close() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn != nil {
			conn.close()
		}
	})
	c.wg.Wait()
}

func (c *Client) stopped() bool {
	select {
	case <-c.stopCh:
		return true
	default:
		return false
	}
}

func (c *Client) signalLocked() {
	close(c.changeCh)
	c.changeCh = make(chan struct{})
}

func (c *Client) kickReconnect() {
	select {
	case c.reconnectCh <- struct{}{}:
	default:
	}
}

func (c *Client) requestRefresh() {
	select {
	case c.refreshCh <- struct{}{}:
	default:
	}
}

func (c *Client) maintainConnection() {
	defer c.wg.Done()
	backoff := time.Second
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		c.mu.Lock()
		hasSession := c.session != nil
		connected := c.conn != nil
		c.mu.Unlock()
		if hasSession && !connected && !c.stopped() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			err := c.connect(ctx)
			cancel()
			if err != nil && !c.stopped() {
				c.logger.Printf("连接失败，%s 后重试：%v", backoff, err)
				select {
				case <-time.After(backoff):
				case <-c.stopCh:
					return
				}
				if backoff < 15*time.Second {
					backoff *= 2
					if backoff > 15*time.Second {
						backoff = 15 * time.Second
					}
				}
				continue
			}
			backoff = time.Second
		}
		select {
		case <-c.stopCh:
			return
		case <-c.reconnectCh:
		case <-ticker.C:
		}
	}
}

func (c *Client) refreshLoop() {
	defer c.wg.Done()
	for {
		select {
		case <-c.stopCh:
			return
		case <-c.refreshCh:
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			if err := c.refresh(ctx); err != nil && !c.stopped() {
				c.logger.Printf("全量状态刷新失败：%v", err)
				c.kickReconnect()
			}
			cancel()
		}
	}
}

func (c *Client) connect(ctx context.Context) error {
	c.connectMu.Lock()
	defer c.connectMu.Unlock()
	c.mu.Lock()
	if c.conn != nil && c.synced {
		c.mu.Unlock()
		return nil
	}
	session := c.session
	c.mu.Unlock()
	if session == nil {
		return &ToolError{Message: "尚未加入房间", Kind: "state"}
	}
	if c.stopped() {
		return context.Canceled
	}
	wsURL, err := c.webSocketURL(session)
	if err != nil {
		return err
	}
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	header := http.Header{}
	header.Set("Cookie", "gq_session="+session.Cookie)
	raw, response, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		if response != nil {
			return fmt.Errorf("WebSocket 握手 HTTP %d：%w", response.StatusCode, err)
		}
		return err
	}
	conn := &wsConn{conn: raw, pending: map[string]chan rpcResult{}, done: make(chan struct{})}
	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.synced = false
	c.signalLocked()
	c.mu.Unlock()
	c.wg.Add(1)
	go c.readLoop(conn)
	body, err := conn.rpc(ctx, http.MethodGet, "/api/v1/rooms/"+session.RoomID+"/state", nil)
	if err != nil {
		conn.close()
		return err
	}
	if err := c.replaceSnapshot(body, true); err != nil {
		conn.close()
		return err
	}
	c.mu.Lock()
	if c.conn == conn {
		c.synced = true
	}
	c.signalLocked()
	c.mu.Unlock()
	return nil
}

func (c *Client) webSocketURL(session *Session) (string, error) {
	parsed, err := url.Parse(session.ServerURL)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "http" {
		parsed.Scheme = "ws"
	} else {
		parsed.Scheme = "wss"
	}
	parsed.Path = "/ws"
	parsed.RawQuery = url.Values{"room_id": {session.RoomID}}.Encode()
	return parsed.String(), nil
}

func (c *Client) readLoop(conn *wsConn) {
	defer c.wg.Done()
	defer c.handleDisconnect(conn)
	for {
		_, data, err := conn.conn.ReadMessage()
		if err != nil {
			return
		}
		var envelope struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			continue
		}
		switch envelope.Type {
		case "rpc_response":
			var response rpcResponse
			if err := json.Unmarshal(data, &response); err != nil {
				continue
			}
			conn.pendingMu.Lock()
			waiter := conn.pending[response.ID]
			delete(conn.pending, response.ID)
			conn.pendingMu.Unlock()
			if waiter != nil {
				waiter <- rpcResult{response: response}
			}
		case "state":
			c.requestRefresh()
		case "error":
			c.logger.Printf("服务端 WebSocket 错误")
		}
	}
}

func (c *Client) handleDisconnect(conn *wsConn) {
	conn.close()
	conn.pendingMu.Lock()
	pending := conn.pending
	conn.pending = map[string]chan rpcResult{}
	conn.pendingMu.Unlock()
	for _, waiter := range pending {
		waiter <- rpcResult{err: io.EOF}
	}
	c.mu.Lock()
	if c.conn == conn {
		c.conn = nil
		c.connected = false
		c.synced = false
	}
	c.signalLocked()
	c.mu.Unlock()
	if !c.stopped() {
		c.kickReconnect()
	}
}

func (c *Client) refresh(ctx context.Context) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	if err := c.connect(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	conn := c.conn
	roomID := ""
	if c.session != nil {
		roomID = c.session.RoomID
	}
	c.mu.Unlock()
	if conn == nil || roomID == "" {
		return &ToolError{Message: "当前没有可用连接", Kind: "connection"}
	}
	body, err := conn.rpc(ctx, http.MethodGet, "/api/v1/rooms/"+roomID+"/state", nil)
	if err != nil {
		return err
	}
	return c.replaceSnapshot(body, false)
}

func (c *wsConn) rpc(ctx context.Context, method, path string, body json.RawMessage) (json.RawMessage, error) {
	id := fmt.Sprintf("gqcli-%d", c.nextID.Add(1))
	waiter := make(chan rpcResult, 1)
	c.pendingMu.Lock()
	c.pending[id] = waiter
	c.pendingMu.Unlock()
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()
	request := rpcRequest{Type: "rpc_request", ID: id, Method: method, Path: path, Body: body}
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	c.writeMu.Lock()
	err = c.conn.WriteMessage(websocket.TextMessage, data)
	c.writeMu.Unlock()
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, io.EOF
	case result := <-waiter:
		if result.err != nil {
			return nil, result.err
		}
		if result.response.Status < 200 || result.response.Status >= 300 {
			return nil, domainError(result.response.Status, result.response.Body, "WebSocket RPC 失败")
		}
		return result.response.Body, nil
	}
}

func (c *wsConn) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

func (c *Client) postJSON(ctx context.Context, path string, request any, target any) ([]*http.Cookie, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.ServerURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		return nil, &ToolError{Message: "连接服务端失败：" + err.Error(), Kind: "connection"}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, domainError(response.StatusCode, body, "HTTP 请求失败")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return nil, fmt.Errorf("解析服务端响应：%w", err)
	}
	return response.Cookies(), nil
}

func (c *Client) loadSession() {
	data, err := os.ReadFile(filepath.Join(c.cfg.StateDir, "session.json"))
	if err != nil {
		return
	}
	var session Session
	if json.Unmarshal(data, &session) == nil && session.RoomID != "" && session.Cookie != "" {
		c.session = &session
		c.lastEventID = session.LastEventID
		c.waitEventID = session.LastEventID
	}
}

func (c *Client) saveSessionLocked() {
	if c.session == nil {
		return
	}
	data, err := json.Marshal(c.session)
	if err == nil {
		_ = atomicWrite(filepath.Join(c.cfg.StateDir, "session.json"), data)
	}
}

func (c *Client) loadSnapshot() {
	data, err := os.ReadFile(filepath.Join(c.cfg.StateDir, "snapshot.json"))
	if err != nil {
		return
	}
	var snapshot map[string]json.RawMessage
	if json.Unmarshal(data, &snapshot) == nil {
		c.snapshot = snapshot
		c.snapshotRaw = append(json.RawMessage(nil), data...)
	}
}

func (c *Client) loadCommands() {
	file, err := os.Open(filepath.Join(c.cfg.StateDir, "commands.jsonl"))
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2<<20)
	for scanner.Scan() {
		var record commandRecord
		if json.Unmarshal(scanner.Bytes(), &record) == nil && record.Key != "" {
			c.commands[record.Key] = &record
		}
	}
}

func (c *Client) saveCommand(record *commandRecord) {
	file, err := os.OpenFile(filepath.Join(c.cfg.StateDir, "commands.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		c.logger.Printf("保存命令日志失败：%v", err)
		return
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(record); err != nil {
		c.logger.Printf("写入命令日志失败：%v", err)
	}
}

func atomicWrite(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func isCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
