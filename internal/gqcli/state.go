package gqcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"generalquarters/internal/game"
)

var allowedSections = map[string]bool{
	"room_id": true, "status": true, "turn": true, "phase": true, "solo": true,
	"scenario": true, "map": true, "objectives": true, "outcome": true,
	"you": true, "players": true, "orders": true, "events": true,
	"units": true, "fleets": true, "aviation": true, "airfields": true,
	"ground_air": true, "ground_aircraft": true, "ground_tasks": true,
	"enemy_air": true, "air_combats": true, "sub_battles": true,
	"submarines": true, "fleet_stays": true, "fleet_sessions": true,
	"shore_supplies": true, "enemy_shore_supplies": true,
	"radio_used": true, "radio_tasks": true, "weather_cells": true,
	"wind": true, "night": true, "clock": true, "losses": true,
	"scores": true, "score_breakdown": true, "display": true, "deploy_zone": true,
}

func (c *Client) replaceSnapshot(data json.RawMessage, resync bool) error {
	var next map[string]json.RawMessage
	if err := json.Unmarshal(data, &next); err != nil {
		return fmt.Errorf("解析全量状态：%w", err)
	}
	var events []Event
	if raw := next["events"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &events); err != nil {
			return fmt.Errorf("解析事件：%w", err)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	changed := resync || !snapshotsEqual(c.snapshot, next)
	newEvents := make([]Event, 0)
	for _, event := range events {
		if event.ID > c.lastEventID {
			newEvents = append(newEvents, event)
		}
	}
	if len(newEvents) > 0 && c.lastEventID > 0 && len(events) == 100 && events[0].ID > c.lastEventID+1 {
		c.eventUncertain = true
	}
	if changed {
		for key, value := range next {
			if old, ok := c.snapshot[key]; !ok || !bytes.Equal(old, value) {
				c.pendingSections[key] = true
			}
		}
		for key := range c.snapshot {
			if _, ok := next[key]; !ok {
				c.pendingSections[key] = true
			}
		}
		c.revision++
		c.snapshot = next
		c.snapshotRaw = append(c.snapshotRaw[:0], data...)
	}
	c.pendingResync = c.pendingResync || resync
	for _, event := range newEvents {
		if event.ID > c.lastEventID {
			c.lastEventID = event.ID
		}
	}
	if c.session != nil {
		c.session.LastEventID = c.lastEventID
		c.saveSessionLocked()
	}
	if changed {
		if err := atomicWrite(filepath.Join(c.cfg.StateDir, "snapshot.json"), data); err != nil {
			c.logger.Printf("保存快照失败：%v", err)
		}
	}
	if len(newEvents) > 0 {
		c.appendEvents(newEvents)
	}
	c.signalLocked()
	return nil
}

func snapshotsEqual(left, right map[string]json.RawMessage) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		other, ok := right[key]
		if !ok || !bytes.Equal(value, other) {
			return false
		}
	}
	return true
}

func (c *Client) appendEvents(events []Event) {
	file, err := os.OpenFile(filepath.Join(c.cfg.StateDir, "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		c.logger.Printf("保存事件日志失败：%v", err)
		return
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			c.logger.Printf("写入事件日志失败：%v", err)
			return
		}
	}
}

func (c *Client) markDelivered() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deliveredRevision = c.revision
	c.waitEventID = c.lastEventID
	c.waitConnected = c.connected
	c.pendingSections = map[string]bool{}
	c.pendingResync = false
	c.eventUncertain = false
}

func (c *Client) wait(ctx context.Context, params waitParams) (any, error) {
	timeout := time.Duration(params.TimeoutMS) * time.Millisecond
	if params.TimeoutMS == 0 {
		timeout = 30 * time.Second
	}
	if timeout < 0 || timeout > 5*time.Minute {
		return nil, &ToolError{Message: "timeout_ms 必须在 0 到 300000 之间", Kind: "validation"}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		c.mu.Lock()
		if c.shouldDeliverLocked() {
			result := c.waitResultLocked(false)
			c.mu.Unlock()
			return result, nil
		}
		change := c.changeCh
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			c.mu.Lock()
			result := c.connectionResultLocked(true)
			c.mu.Unlock()
			return result, nil
		case <-change:
		case <-c.stopCh:
			return nil, context.Canceled
		}
	}
}

func (c *Client) shouldDeliverLocked() bool {
	return c.revision > c.deliveredRevision || c.lastEventID > c.waitEventID || c.pendingResync || c.connected != c.waitConnected
}

func (c *Client) connectionResultLocked(timedOut bool) map[string]any {
	result := map[string]any{
		"changed":   false,
		"timed_out": timedOut,
		"revision":  c.revision,
		"connected": c.connected,
		"synced":    c.synced,
	}
	c.addCurrentMetaLocked(result)
	return result
}

func (c *Client) waitResultLocked(timedOut bool) map[string]any {
	result := c.connectionResultLocked(timedOut)
	result["changed"] = c.revision > c.deliveredRevision || c.lastEventID > c.waitEventID || c.pendingResync
	result["connection_changed"] = c.connected != c.waitConnected
	result["resynced"] = c.pendingResync
	result["event_history_uncertain"] = c.eventUncertain
	sections := make([]string, 0, len(c.pendingSections))
	for section := range c.pendingSections {
		if allowedSections[section] {
			sections = append(sections, section)
		}
	}
	sort.Strings(sections)
	result["changed_sections"] = sections
	events := make([]Event, 0)
	if c.snapshot != nil {
		var current []Event
		_ = json.Unmarshal(c.snapshot["events"], &current)
		for _, event := range current {
			if event.ID > c.waitEventID {
				events = append(events, event)
			}
		}
	}
	result["events"] = events
	c.deliveredRevision = c.revision
	c.waitEventID = c.lastEventID
	c.waitConnected = c.connected
	c.pendingSections = map[string]bool{}
	c.pendingResync = false
	c.eventUncertain = false
	return result
}

func (c *Client) addCurrentMetaLocked(result map[string]any) {
	if c.snapshot == nil {
		return
	}
	for _, key := range []string{"room_id", "status", "turn", "phase", "you", "players", "outcome"} {
		if raw, ok := c.snapshot[key]; ok {
			result[key] = raw
		}
	}
}

func (c *Client) state(ctx context.Context, params stateParams) (any, error) {
	if params.Refresh {
		if err := c.refresh(ctx); err != nil {
			return nil, err
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.snapshot == nil {
		return nil, &ToolError{Message: "尚无状态快照，请先加入房间并等待同步", Kind: "state"}
	}
	if params.Section == "" || params.Section == "summary" {
		return c.summaryLocked()
	}
	if params.Section == "full" {
		state := map[string]json.RawMessage{}
		for key, value := range c.snapshot {
			if allowedSections[key] {
				state[key] = value
			}
		}
		return map[string]any{"revision": c.revision, "synced": c.synced, "state": state}, nil
	}
	section := params.Section
	switch section {
	case "fleet":
		section = "fleets"
	case "unit":
		section = "units"
	case "air_task":
		section = "aviation"
	case "airfield":
		section = "airfields"
	case "contacts":
		return c.contactsLocked()
	}
	if !allowedSections[section] {
		return nil, &ToolError{Message: "不支持的状态 section：" + section, Kind: "validation"}
	}
	raw, ok := c.snapshot[section]
	if !ok {
		return map[string]any{"revision": c.revision, "synced": c.synced, "section": section, "value": nil}, nil
	}
	if params.Section == "air_task" {
		return c.findAirTaskLocked(params.ID)
	}
	if params.ID != "" {
		return findEntity(c.revision, c.synced, section, params.ID, raw)
	}
	if params.Scope != "" {
		if section != "fleets" && section != "units" {
			return nil, &ToolError{Message: "scope 只支持 fleets 和 units", Kind: "validation"}
		}
		filtered, err := c.filterFactionLocked(raw, params.Scope)
		if err != nil {
			return nil, err
		}
		raw = filtered
	}
	return pageSection(c.revision, c.synced, section, raw, params.Offset, params.Limit)
}

func (c *Client) summaryLocked() (map[string]any, error) {
	result := map[string]any{
		"revision":  c.revision,
		"connected": c.connected,
		"synced":    c.synced,
	}
	for _, key := range []string{"room_id", "status", "turn", "phase", "solo", "scenario", "you", "players", "objectives", "outcome", "orders", "clock", "night", "scores", "radio_used"} {
		if raw, ok := c.snapshot[key]; ok {
			result[key] = raw
		}
	}
	if raw := c.snapshot["map"]; len(raw) > 0 {
		var value struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Rows int    `json:"rows"`
			Cols int    `json:"cols"`
		}
		_ = json.Unmarshal(raw, &value)
		result["map"] = value
	}
	var faction string
	var you Player
	_ = json.Unmarshal(c.snapshot["you"], &you)
	faction = you.Faction
	var fleets []game.FleetGroup
	if err := json.Unmarshal(c.snapshot["fleets"], &fleets); err != nil {
		return nil, fmt.Errorf("解析舰队摘要：%w", err)
	}
	own := make([]map[string]any, 0)
	enemy := make([]map[string]any, 0)
	for _, fleet := range fleets {
		item := fleetSummary(fleet)
		if fleet.Faction == faction {
			own = append(own, item)
		} else {
			enemy = append(enemy, item)
		}
	}
	result["own_fleets"] = own
	result["enemy_contacts"] = enemy
	var aviation game.AviationView
	if err := json.Unmarshal(c.snapshot["aviation"], &aviation); err == nil {
		carriers := make([]map[string]any, 0, len(aviation.Carriers))
		for _, carrier := range aviation.Carriers {
			carriers = append(carriers, map[string]any{
				"unit_id": carrier.UnitID, "name": carrier.Name,
				"hangar_used": carrier.HangarUsed, "hangar_capacity": carrier.HangarCapacity,
				"deck_length": carrier.DeckLength, "operation": carrier.Operation,
				"blocked_reason": carrier.BlockedReason, "aircraft_count": len(carrier.Aircraft),
			})
		}
		result["carriers"] = carriers
		result["air_tasks"] = taskSummaries(aviation.Tasks)
	}
	var groundTasks []game.AirTask
	if err := json.Unmarshal(c.snapshot["ground_tasks"], &groundTasks); err == nil {
		result["ground_tasks"] = taskSummaries(groundTasks)
	}
	var events []Event
	_ = json.Unmarshal(c.snapshot["events"], &events)
	if len(events) > 10 {
		events = events[len(events)-10:]
	}
	result["recent_events"] = events
	return result, nil
}

func fleetSummary(fleet game.FleetGroup) map[string]any {
	return map[string]any{
		"id": fleet.ID, "fleet_id": fleet.FleetID, "fleet_name": fleet.FleetName,
		"faction": fleet.Faction, "coord": fleet.Coord, "heading": fleet.Heading,
		"count": fleet.Count, "strength": fleet.Strength, "max_strength": fleet.MaxStrength,
		"move_cooldown": fleet.MoveCooldown, "move_interval": fleet.MoveInterval,
		"move_knots": fleet.MoveKnots, "confusion": fleet.Confusion,
		"contact_level": fleet.ContactLevel, "contact_stale_turns": fleet.ContactStaleTurns,
		"contact_size": fleet.ContactSize, "contact_capitals": fleet.ContactCapitals,
		"legal_moves": fleet.LegalMoves, "destination": fleet.Destination,
		"engagement": fleet.Engagement,
	}
}

func taskSummaries(tasks []game.AirTask) []map[string]any {
	result := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, map[string]any{
			"id": task.ID, "kind": task.Kind, "phase": task.Phase, "name": task.Name,
			"carrier_id": task.CarrierID, "fleet_id": task.FleetID,
			"aircraft_count": len(task.Aircraft), "airborne_count": task.AirborneCount,
			"origin_coord": task.OriginCoord, "current_coord": task.CurrentCoord,
			"target_coord": task.TargetCoord, "returning": task.Returning,
			"return_turns": task.ReturnTurns, "turns_to_dark": task.TurnsToDark,
			"night_margin": task.NightMargin, "turns_to_target": task.TurnsToTarget,
			"turns_to_landed": task.TurnsToLanded, "lands_at_night": task.LandsAtNight,
		})
	}
	return result
}

func (c *Client) contactsLocked() (map[string]any, error) {
	var you Player
	_ = json.Unmarshal(c.snapshot["you"], &you)
	var fleets []game.FleetGroup
	if err := json.Unmarshal(c.snapshot["fleets"], &fleets); err != nil {
		return nil, err
	}
	contacts := make([]map[string]any, 0)
	for _, fleet := range fleets {
		if fleet.Faction != you.Faction {
			contacts = append(contacts, fleetSummary(fleet))
		}
	}
	return map[string]any{
		"revision": c.revision, "synced": c.synced,
		"fleets": contacts, "enemy_air": c.snapshot["enemy_air"],
	}, nil
}

func (c *Client) findAirTaskLocked(id string) (map[string]any, error) {
	if id == "" {
		return nil, &ToolError{Message: "air_task 需要 id", Kind: "validation"}
	}
	var aviation game.AviationView
	_ = json.Unmarshal(c.snapshot["aviation"], &aviation)
	for _, task := range aviation.Tasks {
		if task.ID == id {
			return map[string]any{"revision": c.revision, "synced": c.synced, "section": "air_task", "value": task}, nil
		}
	}
	var tasks []game.AirTask
	_ = json.Unmarshal(c.snapshot["ground_tasks"], &tasks)
	for _, task := range tasks {
		if task.ID == id {
			return map[string]any{"revision": c.revision, "synced": c.synced, "section": "air_task", "value": task}, nil
		}
	}
	return nil, &ToolError{Message: "找不到航空任务：" + id, Kind: "not_found"}
}

func findEntity(revision uint64, synced bool, section, id string, raw json.RawMessage) (map[string]any, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, &ToolError{Message: section + " 不是实体数组，不能按 id 查询", Kind: "validation"}
	}
	for _, item := range items {
		var entity map[string]any
		if json.Unmarshal(item, &entity) != nil {
			continue
		}
		for _, key := range []string{"id", "unit_id", "task_id", "carrier_id", "fleet_id", "coord"} {
			if fmt.Sprint(entity[key]) == id {
				return map[string]any{"revision": revision, "synced": synced, "section": section, "value": entity}, nil
			}
		}
	}
	return nil, &ToolError{Message: "找不到 " + section + " 实体：" + id, Kind: "not_found"}
}

func (c *Client) filterFactionLocked(raw json.RawMessage, scope string) (json.RawMessage, error) {
	if scope != "own" && scope != "enemy" && scope != "all" {
		return nil, &ToolError{Message: "scope 必须是 own、enemy 或 all", Kind: "validation"}
	}
	if scope == "all" {
		return raw, nil
	}
	var you Player
	_ = json.Unmarshal(c.snapshot["you"], &you)
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	filtered := make([]map[string]any, 0)
	for _, item := range items {
		own := fmt.Sprint(item["faction"]) == you.Faction
		if (scope == "own" && own) || (scope == "enemy" && !own) {
			filtered = append(filtered, item)
		}
	}
	return json.Marshal(filtered)
}

func pageSection(revision uint64, synced bool, section string, raw json.RawMessage, offset, limit int) (map[string]any, error) {
	if offset < 0 || limit < 0 || limit > 500 {
		return nil, &ToolError{Message: "offset 不能为负，limit 必须在 0 到 500 之间", Kind: "validation"}
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err == nil {
		if limit == 0 {
			limit = 100
		}
		total := len(items)
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		return map[string]any{
			"revision": revision, "synced": synced, "section": section,
			"total": total, "offset": offset, "limit": limit,
			"items": items[offset:end],
		}, nil
	}
	if offset != 0 || limit != 0 {
		return nil, &ToolError{Message: "分页只支持数组 section", Kind: "validation"}
	}
	return map[string]any{"revision": revision, "synced": synced, "section": section, "value": raw}, nil
}
