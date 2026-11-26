package game

import (
	"encoding/json"
	"testing"
)

func TestHexOfParsesAndRejects(t *testing.T) {
	valid := map[string]Hex{"0101": 101, "0102": 102, "5264": 5264, "9999": 9999}
	for coord, want := range valid {
		got, ok := HexOf(coord)
		if !ok || got != want {
			t.Fatalf("%s 解析失败：得 %d，期望 %d", coord, got, want)
		}
		if got.Row() != int(want)/100 || got.Col() != int(want)%100 {
			t.Fatalf("%s 行列拆错了：%d,%d", coord, got.Row(), got.Col())
		}
		if got.String() != coord {
			t.Fatalf("%s 编码回去变成 %s", coord, got.String())
		}
	}
	for _, bad := range []string{"", "101", "01011", "0001", "0100", "01a1", " 101", "-101"} {
		if _, ok := HexOf(bad); ok {
			t.Fatalf("非法坐标被接受：%q", bad)
		}
	}
}

func TestHexJSONStaysFourCharCoord(t *testing.T) {
	encoded, err := json.Marshal(Hex(102))
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `"0102"` {
		t.Fatalf("hex 的 JSON 变味了：%s", encoded)
	}
	var decoded Hex
	if err := json.Unmarshal(encoded, &decoded); err != nil || decoded != Hex(102) {
		t.Fatalf("hex 解码失败：%d %v", decoded, err)
	}
	if err := json.Unmarshal([]byte(`"01x2"`), &decoded); err == nil {
		t.Fatal("非法坐标没有被拒绝")
	}
}

func TestMapDefinitionJSONCompilesBoard(t *testing.T) {
	raw := []byte(`{"id":"t","rows":3,"cols":3,"cells":[
        {"coord":"0102","terrain":"island"},
        {"coord":"0203","terrain":"shoal"},
        {"coord":"0301","terrain":"reef"}]}`)
	var gameMap MapDefinition
	if err := json.Unmarshal(raw, &gameMap); err != nil {
		t.Fatal(err)
	}
	if gameMap.board == nil {
		t.Fatal("解完之后没有编译地形表")
	}
	want := map[string]Terrain{
		"0102": TerrainIsland,
		"0203": TerrainShoal,
		"0301": TerrainReef,
		"0101": TerrainWater,
		"0303": TerrainWater,
	}
	for coord, terrain := range want {
		if got := terrainAt(gameMap, coord); got != terrain {
			t.Fatalf("%s 地形错了：得 %d，期望 %d", coord, got, terrain)
		}
	}
	// 编码回去不能把编译出来的表漏进 JSON，其它字段也要稳定。
	encoded, err := json.Marshal(gameMap)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if _, exists := roundTrip["board"]; exists {
		t.Fatalf("MapDefinition 的 JSON 漏出了内部字段：%v", roundTrip)
	}
	// 解回来再编一次必须是同一份字节：结构体字段顺序稳定，能证明编解码闭合。
	var again MapDefinition
	if err := json.Unmarshal(encoded, &again); err != nil {
		t.Fatal(err)
	}
	encodedAgain, err := json.Marshal(again)
	if err != nil {
		t.Fatal(err)
	}
	if string(encodedAgain) != string(encoded) {
		t.Fatalf("MapDefinition 编解码不稳定：%s -> %s", encoded, encodedAgain)
	}
	if again.board == nil {
		t.Fatal("解回来之后没有重新编译地形表")
	}
}

func TestTerrainBoardMatchesCellScan(t *testing.T) {
	// 编译过的板子必须和"扫 Cells"（测试里手搭字面量走的那条路）在每一格上一致：
	// 包括默认水、边界外、以及内容里写错的地形名。
	cells := []MapCell{
		{Coord: "0101", Terrain: "island"},
		{Coord: "0103", Terrain: "shoal"},
		{Coord: "0202", Terrain: "reef"},
		{Coord: "0203", Terrain: "sunken"}, // 认不出来的地形名按水处理
	}
	literal := MapDefinition{Rows: 3, Cols: 3, Cells: cells}
	compiled := MapDefinition{Rows: 3, Cols: 3, Cells: cells}
	compiled.board = compileBoard(compiled)
	if compiled.board == nil {
		t.Fatal("编译失败")
	}
	for row := 0; row <= 5; row++ {
		for col := 0; col <= 5; col++ {
			coord := Coord(row, col)
			if got, want := terrainAt(compiled, coord), terrainAt(literal, coord); got != want {
				t.Fatalf("%s 编译版和扫描版不一致：%d vs %d", coord, got, want)
			}
		}
	}
	if got := terrainAt(compiled, "0203"); got != TerrainWater {
		t.Fatalf("未知地形应当按水处理，得 %d", got)
	}
	// 越界的格子载入校验会直接拒绝；这里只保证编译时把它丢掉，
	// 不让一条脏数据把地形表撑大。
	beyond := MapDefinition{Rows: 3, Cols: 3, Cells: []MapCell{{Coord: "0404", Terrain: "island"}}}
	beyond.board = compileBoard(beyond)
	if got := beyond.terrain(Hex(404)); got != TerrainWater {
		t.Fatalf("越界格子没有在编译时丢掉：%d", got)
	}
}

// benchMap 造一张和珊瑚海同量级的图：52×64、438 个陆地格。
func benchMap() MapDefinition {
	gameMap := MapDefinition{ID: "bench", Rows: 52, Cols: 64}
	for index := 0; index < 438; index++ {
		row := index/8%52 + 1
		col := index%8*7 + 1
		gameMap.Cells = append(gameMap.Cells, MapCell{Coord: Coord(row, col), Terrain: "island"})
	}
	return gameMap
}

func BenchmarkTerrainLookupCompiled(b *testing.B) {
	gameMap := benchMap()
	gameMap.board = compileBoard(gameMap)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		terrainAt(gameMap, "2607")
	}
}

func BenchmarkTerrainLookupScan(b *testing.B) {
	gameMap := benchMap()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		terrainAt(gameMap, "2607")
	}
}
