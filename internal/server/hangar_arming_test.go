package server

import (
	"net/http"
	"testing"

	"generalquarters/internal/game"
)

// 机库选挂载 → 拖进甲板栏这条链路要能走通 HTTP 命令通道：
// SET_LOADOUT 立刻开始整备，LIFT_TO_DECK 记下提升请求。
func TestHangarArmingAndLiftOverHTTP(t *testing.T) {
	a, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := a.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"scenario_id": "_fixture", "solo": true}, &created); status != http.StatusCreated {
		t.Fatal("创建房间失败")
	}
	send := func(command map[string]any) int {
		var reply map[string]any
		return a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", command, &reply)
	}
	aircraftID := "a2-cv-01-fighter-x1"
	arm := map[string]any{"command_id": "arm-hangar", "type": "SET_LOADOUT", "aircraft_id": aircraftID, "aircraft_ids": []string{aircraftID}, "loadout": "air+tank"}
	if status := send(arm); status != http.StatusOK {
		t.Fatalf("机库选挂载被拒：%d", status)
	}
	lift := map[string]any{"command_id": "lift-deck", "type": "LIFT_TO_DECK", "aircraft_id": aircraftID, "aircraft_ids": []string{aircraftID}}
	if status := send(lift); status != http.StatusOK {
		t.Fatalf("拖到甲板栏被拒：%d", status)
	}
	var view testView
	if status := a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &view); status != http.StatusOK {
		t.Fatalf("读取房间状态失败：%d", status)
	}
	var plane *game.AirUnit
	for index := range view.Aviation.Carriers {
		for _, aircraft := range view.Aviation.Carriers[index].Aircraft {
			if aircraft.ID == aircraftID {
				candidate := aircraft
				plane = &candidate
			}
		}
	}
	if plane == nil {
		t.Fatal("状态里找不到这架算子")
	}
	if plane.Status != "service" || plane.DeckService {
		t.Fatalf("机库选挂载后应在机库整备：status=%s deck_service=%v", plane.Status, plane.DeckService)
	}
	if !plane.LiftRequested {
		t.Fatal("拖进甲板栏之后应记下提升请求")
	}
	if plane.Loadout != game.LoadoutAirTank || !plane.LoadoutManual {
		t.Fatalf("挂载应被手动选中：%s manual=%v", plane.Loadout, plane.LoadoutManual)
	}
	// 甲板上的算子拖进任务栏仍然是"直接出击"，不受整备拖累。
	deck := map[string]any{"command_id": "lift-again", "type": "LIFT_TO_DECK", "aircraft_id": aircraftID, "aircraft_ids": []string{aircraftID}}
	if status := send(deck); status != http.StatusOK {
		t.Fatalf("重复提升请求应被允许（幂等）：%d", status)
	}
}
