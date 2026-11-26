package game

import "encoding/json"

// Terrain 是地形。以前全代码拿字符串判断（== "island"、== "reef"），
// 拼错一个字母编译器不管、运行期默默当水面；现在是枚举。
type Terrain uint8

const (
	// TerrainWater 是零值：没写在地图里的格子都是水。剧本只列岛和浅滩，
	// 所以"查不到"和"是水"本来就是同一件事。
	TerrainWater Terrain = iota
	TerrainIsland
	TerrainShoal
	TerrainReef
)

func (terrain Terrain) String() string {
	switch terrain {
	case TerrainIsland:
		return "island"
	case TerrainShoal:
		return "shoal"
	case TerrainReef:
		return "reef"
	}
	return "water"
}

// ParseTerrain 把内容文件里的地形名转成枚举。认不出来的按水处理，
// 和老代码"扫不到这个坐标就当 water"一致。
func ParseTerrain(name string) Terrain {
	switch name {
	case "island":
		return TerrainIsland
	case "shoal":
		return TerrainShoal
	case "reef":
		return TerrainReef
	}
	return TerrainWater
}

// board 是 MapDefinition 编译出来的地形表：按 Hex 直接索引。
//
// 只缓存**静态**内容（地形）。天气这类每回合会变的字段留在 Cells 里：
// 地图是场景级、跨房间共享的内容，把可变状态塞进来，每个房间、每份历史快照
// 都会跟着重写一遍。
type board struct {
	cells []Terrain // 索引 = int(hex)；行列都在 [1, Rows]×[1, Cols] 内
}

// compileBoard 把稀疏的 Cells 展开成按坐标索引的表。
// 越界或解析不了的格子直接跳过（它们本来也读不到）。
func compileBoard(gameMap MapDefinition) *board {
	if gameMap.Rows < 1 || gameMap.Cols < 1 {
		return nil
	}
	cells := make([]Terrain, (gameMap.Rows+1)*100+gameMap.Cols+1)
	for _, cell := range gameMap.Cells {
		hex, ok := HexOf(cell.Coord)
		if !ok || hex.Row() > gameMap.Rows || hex.Col() > gameMap.Cols {
			continue
		}
		cells[int(hex)] = ParseTerrain(cell.Terrain)
	}
	return &board{cells: cells}
}

// terrain 查一格的地形。
//
// 板子没编译过（测试里手搭的 MapDefinition 字面量）就退回扫 Cells：
// 行为一样，只是慢，正确性不受影响。
func (gameMap MapDefinition) terrain(hex Hex) Terrain {
	if gameMap.board != nil {
		if index := int(hex); index >= 0 && index < len(gameMap.board.cells) {
			return gameMap.board.cells[index]
		}
		return TerrainWater
	}
	// 兜底路径不能给每个格子分配一个 "0102" 字符串：解析成 Hex 再比，
	// 否则一条命令里的几千次查询会顺带几千次分配。
	for _, cell := range gameMap.Cells {
		parsed, ok := HexOf(cell.Coord)
		if ok && parsed == hex {
			return ParseTerrain(cell.Terrain)
		}
	}
	return TerrainWater
}

// UnmarshalJSON 解完地图顺手把地形表编译出来。
//
// 放在这里是因为 State 是整份塞进 state_json 的：房间每次从库里读出来都会
// 解一次地图，编译也就跟着发生一次，不用在 store 里再手动重建一遍缓存。
// 手搭的 MapDefinition 字面量没有这一步，走 terrain 的扫描兜底。
func (gameMap *MapDefinition) UnmarshalJSON(data []byte) error {
	// 用别名避免递归调用自己；别名没有方法，所以 json 不会再进来一次。
	type definition MapDefinition
	var decoded definition
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*gameMap = MapDefinition(decoded)
	gameMap.board = compileBoard(*gameMap)
	return nil
}
