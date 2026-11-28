package gqcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	ID     string     `json:"id"`
	OK     bool       `json:"ok"`
	Result any        `json:"result,omitempty"`
	Error  *ToolError `json:"error,omitempty"`
}

type ToolError struct {
	Message     string            `json:"message"`
	Kind        string            `json:"kind,omitempty"`
	Status      int               `json:"status,omitempty"`
	ErrorCode   string            `json:"error_code,omitempty"`
	ErrorParams map[string]string `json:"error_params,omitempty"`
}

func (e *ToolError) Error() string { return e.Message }

type Session struct {
	ServerURL   string `json:"server_url"`
	RoomID      string `json:"room_id"`
	PlayerID    string `json:"player_id"`
	Faction     string `json:"faction"`
	Cookie      string `json:"cookie"`
	LastEventID int64  `json:"last_event_id,omitempty"`
}

type Player struct {
	ID        string `json:"id"`
	Faction   string `json:"faction"`
	Name      string `json:"name"`
	Ready     bool   `json:"ready"`
	Connected bool   `json:"connected"`
}

type Event struct {
	ID        int64  `json:"id"`
	Turn      int    `json:"turn"`
	Sequence  int    `json:"sequence"`
	Type      string `json:"type"`
	Text      string `json:"text"`
	Audience  string `json:"audience"`
	CreatedAt string `json:"created_at"`
}

type createParams struct {
	ScenarioID string `json:"scenario_id"`
	Solo       bool   `json:"solo"`
}

type joinParams struct {
	Token   string `json:"token"`
	Faction string `json:"faction"`
	Name    string `json:"name"`
}

type stateParams struct {
	Section string `json:"section"`
	ID      string `json:"id"`
	Scope   string `json:"scope"`
	Offset  int    `json:"offset"`
	Limit   int    `json:"limit"`
	Refresh bool   `json:"refresh"`
}

type waitParams struct {
	TimeoutMS int `json:"timeout_ms"`
}

type readyParams struct {
	Ready *bool `json:"ready"`
}

type commandParams struct {
	Type          string            `json:"type"`
	UnitID        string            `json:"unit_id"`
	TargetUnitID  string            `json:"target_unit_id"`
	AircraftID    string            `json:"aircraft_id"`
	AircraftIDs   []string          `json:"aircraft_ids"`
	TaskID        string            `json:"task_id"`
	Coord         string            `json:"coord"`
	TargetFleetID string            `json:"target_fleet_id"`
	Heading       string            `json:"heading"`
	SectorWidth   int               `json:"sector_width"`
	Assignments   map[string]string `json:"assignments"`
	SplitIDs      []string          `json:"split_ids"`
	TaskKind      string            `json:"task_kind"`
	Loadout       string            `json:"loadout"`
	Lane          int               `json:"lane"`
	Disperse      bool              `json:"disperse"`
	Cycle         bool              `json:"cycle"`
	Engagement    string            `json:"engagement"`
}

type commandRequest struct {
	CommandID string `json:"command_id"`
	BaseTurn  int    `json:"base_turn"`
	commandParams
}

type readyRequest struct {
	BaseTurn int  `json:"base_turn"`
	Ready    bool `json:"ready"`
}

type rpcRequest struct {
	Type   string          `json:"type"`
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body,omitempty"`
}

type rpcResponse struct {
	Type   string          `json:"type"`
	ID     string          `json:"id"`
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

type apiError struct {
	Error       string            `json:"error"`
	ErrorCode   string            `json:"error_code"`
	ErrorParams map[string]string `json:"error_params"`
}

type commandRecord struct {
	Key         string          `json:"key"`
	Fingerprint string          `json:"fingerprint"`
	Request     json.RawMessage `json:"request"`
	Response    json.RawMessage `json:"response,omitempty"`
	Error       *ToolError      `json:"error,omitempty"`
	Done        bool            `json:"done"`
}

func decodeParams(data json.RawMessage, target any) error {
	if len(data) == 0 || string(data) == "null" {
		data = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return &ToolError{Message: "工具参数无效或包含未知字段：" + err.Error(), Kind: "protocol"}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return &ToolError{Message: "工具参数必须只有一个 JSON 对象", Kind: "protocol"}
	}
	return nil
}

func domainError(status int, body json.RawMessage, fallback string) *ToolError {
	var payload apiError
	_ = json.Unmarshal(body, &payload)
	if payload.Error == "" {
		payload.Error = fallback
	}
	return &ToolError{
		Message:     fmt.Sprintf("%s（HTTP %d）", payload.Error, status),
		Kind:        "domain",
		Status:      status,
		ErrorCode:   payload.ErrorCode,
		ErrorParams: payload.ErrorParams,
	}
}
