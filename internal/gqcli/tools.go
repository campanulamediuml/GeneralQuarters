package gqcli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (c *Client) call(ctx context.Context, request Request) (any, error) {
	switch request.Method {
	case "gq_create":
		var params createParams
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return c.create(ctx, params)
	case "gq_join":
		var params joinParams
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return c.join(ctx, params)
	case "gq_state":
		var params stateParams
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return c.state(ctx, params)
	case "gq_command":
		var params commandParams
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return c.command(ctx, request.ID, request.Params, params)
	case "gq_ready":
		var params readyParams
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return c.ready(ctx, params)
	case "gq_wait":
		var params waitParams
		if err := decodeParams(request.Params, &params); err != nil {
			return nil, err
		}
		return c.wait(ctx, params)
	default:
		return nil, &ToolError{Message: "未知工具：" + request.Method, Kind: "protocol"}
	}
}

func (c *Client) create(ctx context.Context, params createParams) (any, error) {
	var result struct {
		RoomID    string `json:"room_id"`
		JoinToken string `json:"join_token"`
	}
	_, err := c.postJSON(ctx, "/api/v1/rooms", params, &result)
	if err != nil {
		return nil, err
	}
	return map[string]any{"room_id": result.RoomID, "join_token": result.JoinToken}, nil
}

func (c *Client) join(ctx context.Context, params joinParams) (any, error) {
	params.Token = strings.TrimSpace(params.Token)
	params.Faction = strings.ToUpper(strings.TrimSpace(params.Faction))
	if params.Token == "" || (params.Faction != "A" && params.Faction != "B") {
		return nil, &ToolError{Message: "token 必填，faction 必须是 A 或 B", Kind: "validation"}
	}
	c.mu.Lock()
	existing := c.session
	c.mu.Unlock()
	if existing != nil {
		return nil, &ToolError{Message: "此 CLI 已加入房间；如需恢复，请使用原状态目录启动，不要重复 join", Kind: "state"}
	}
	var result struct {
		RoomID   string `json:"room_id"`
		PlayerID string `json:"player_id"`
		Faction  string `json:"faction"`
	}
	cookies, err := c.postJSON(ctx, "/api/v1/rooms/join", params, &result)
	if err != nil {
		return nil, err
	}
	cookie := ""
	for _, item := range cookies {
		if item.Name == "gq_session" {
			cookie = item.Value
			break
		}
	}
	if cookie == "" {
		return nil, fmt.Errorf("加入响应没有 gq_session Cookie")
	}
	session := &Session{
		ServerURL: c.cfg.ServerURL, RoomID: result.RoomID,
		PlayerID: result.PlayerID, Faction: result.Faction, Cookie: cookie,
	}
	c.mu.Lock()
	c.session = session
	c.saveSessionLocked()
	c.mu.Unlock()
	response := map[string]any{
		"joined": true, "room_id": result.RoomID,
		"player_id": result.PlayerID, "faction": result.Faction,
	}
	if err := c.connect(ctx); err != nil {
		response["connected"] = false
		response["synced"] = false
		response["sync_error"] = err.Error()
		response["message"] = "已加入并保存会话，CLI 将在后台继续重连；不要重复 join"
		return response, nil
	}
	c.markDelivered()
	response["connected"] = true
	response["synced"] = true
	return response, nil
}

func (c *Client) command(ctx context.Context, key string, rawParams json.RawMessage, params commandParams) (any, error) {
	if params.Type == "" {
		return nil, &ToolError{Message: "命令 type 不能为空", Kind: "validation"}
	}
	c.commandMu.Lock()
	defer c.commandMu.Unlock()
	if err := c.connect(ctx); err != nil {
		return nil, err
	}
	fingerprint := hashText(string(rawParams))
	c.mu.Lock()
	session := c.session
	turn := 0
	if raw := c.snapshot["turn"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &turn)
	}
	c.mu.Unlock()
	if session == nil {
		return nil, &ToolError{Message: "尚未加入房间", Kind: "state"}
	}
	record := c.commands[key]
	if record != nil && record.Fingerprint != fingerprint {
		return nil, &ToolError{Message: "同一个 stdio 请求 ID 不能用于不同命令", Kind: "protocol"}
	}
	if record != nil && record.Done {
		if record.Error != nil {
			return nil, record.Error
		}
		return decodeResult(record.Response)
	}
	if record == nil {
		params.Type = strings.ToUpper(params.Type)
		commandID := hashText(session.PlayerID + "\x00" + key)[:32]
		body, err := json.Marshal(commandRequest{CommandID: "gq-" + commandID, BaseTurn: turn, commandParams: params})
		if err != nil {
			return nil, err
		}
		record = &commandRecord{Key: key, Fingerprint: fingerprint, Request: body}
		c.commands[key] = record
		c.saveCommand(record)
	}
	c.mu.Lock()
	conn := c.conn
	roomID := c.session.RoomID
	c.mu.Unlock()
	if conn == nil {
		return nil, &ToolError{Message: "WebSocket 尚未连接", Kind: "connection"}
	}
	body, err := conn.rpc(ctx, http.MethodPost, "/api/v1/rooms/"+roomID+"/commands", record.Request)
	if err != nil {
		if domain, ok := err.(*ToolError); ok && domain.Kind == "domain" && domain.Status != http.StatusServiceUnavailable {
			record.Error = domain
			record.Done = true
			c.saveCommand(record)
		}
		return nil, err
	}
	record.Response = append(json.RawMessage(nil), body...)
	record.Done = true
	c.saveCommand(record)
	result, err := decodeResult(body)
	if err != nil {
		return nil, err
	}
	if err := c.refresh(ctx); err != nil {
		if object, ok := result.(map[string]any); ok {
			object["sync_error"] = err.Error()
		}
	}
	return result, nil
}

func (c *Client) ready(ctx context.Context, params readyParams) (any, error) {
	if err := c.connect(ctx); err != nil {
		return nil, err
	}
	ready := true
	if params.Ready != nil {
		ready = *params.Ready
	}
	c.mu.Lock()
	conn := c.conn
	roomID := c.session.RoomID
	turn := 0
	_ = json.Unmarshal(c.snapshot["turn"], &turn)
	c.mu.Unlock()
	if conn == nil {
		return nil, &ToolError{Message: "WebSocket 尚未连接", Kind: "connection"}
	}
	body, err := json.Marshal(readyRequest{BaseTurn: turn, Ready: ready})
	if err != nil {
		return nil, err
	}
	response, err := conn.rpc(ctx, http.MethodPost, "/api/v1/rooms/"+roomID+"/ready", body)
	if err != nil {
		return nil, err
	}
	result, err := decodeResult(response)
	if err != nil {
		return nil, err
	}
	if err := c.refresh(ctx); err != nil {
		if object, ok := result.(map[string]any); ok {
			object["sync_error"] = err.Error()
		}
	}
	return result, nil
}

func decodeResult(data json.RawMessage) (any, error) {
	var result any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("解析命令回执：%w", err)
	}
	return result, nil
}

func hashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
