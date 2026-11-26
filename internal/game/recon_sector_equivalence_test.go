package game

import (
	"sort"
	"strings"
	"testing"
)

// 下面四个是"优化前那份写法"的参考实现：逐格 Sprintf 坐标、再 Atoi 解析回来算距离，
// 每个距离圈都把整张地图重扫一遍。它们只用来做等价性对照，不参与正式判定。

func referenceSectorHexes(gameMap MapDefinition, origin, heading string, width, radius int) []string {
	heading = strings.ToUpper(heading)
	axis, ok := reconHeadings[heading]
	originRow, originCol, err := ParseCoord(origin)
	if !ok || err != nil || (width != 60 && width != 120) || radius < 1 {
		return []string{}
	}
	originX, originY := hexCenter(originRow, originCol)
	hexes := []string{}
	for row := 1; row <= gameMap.Rows; row++ {
		for col := 1; col <= gameMap.Cols; col++ {
			coord := Coord(row, col)
			if coord == origin || Distance(origin, coord) > radius {
				continue
			}
			x, y := hexCenter(row, col)
			if sectorIntersectsHex(sectorPoint{x - originX, y - originY}, axis, float64(width)) {
				hexes = append(hexes, coord)
			}
		}
	}
	return hexes
}

func referenceRingHexes(gameMap MapDefinition, origin, heading string, width, ring int) []string {
	all := referenceSectorHexes(gameMap, origin, heading, width, ring)
	out := make([]string, 0, len(all))
	for _, coord := range all {
		if Distance(origin, coord) == ring {
			out = append(out, coord)
		}
	}
	return out
}

func referenceScanBandHexes(gameMap MapDefinition, origin, heading string, width, radius int) []string {
	if radius < 1 || origin == "" {
		return []string{}
	}
	seen := map[string]bool{}
	out := []string{}
	for ring := 1; ring <= radius; ring++ {
		for _, coord := range referenceRingHexes(gameMap, origin, heading, width, ring) {
			if seen[coord] {
				continue
			}
			seen[coord] = true
			out = append(out, coord)
		}
	}
	sort.Strings(out)
	return out
}

func referenceScannedRingHexes(gameMap MapDefinition, origin, coord, heading string, width, radius int) []string {
	if origin == "" || coord == "" {
		return []string{}
	}
	if radius <= 0 {
		radius = 99
	}
	seen := map[string]bool{}
	out := []string{}
	ring := Distance(origin, coord)
	for _, r := range []int{ring, ring + 1} {
		if r < 1 || r > radius {
			continue
		}
		for _, item := range referenceRingHexes(gameMap, origin, heading, width, r) {
			if seen[item] {
				continue
			}
			seen[item] = true
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

func sameHexes(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

// TestReconSectorGeometryUnchanged 是重写扇面扫描时的守门测试：
// 新实现只在内部换成整数运算，判定出的格子（以及顺序）必须与旧写法逐格一致。
// 顺序也在比，因为战报会截断"前 24 格"，换顺序等于换文案。
func TestReconSectorGeometryUnchanged(t *testing.T) {
	gameMap := MapDefinition{Rows: 21, Cols: 25}
	origins := []string{"0101", "0510", "0624", "1112", "2115", "1025"}
	headings := []string{"E", "SE", "SW", "W", "NW", "NE"}
	for _, origin := range origins {
		for _, heading := range headings {
			for _, width := range []int{60, 120} {
				for radius := 1; radius <= 8; radius++ {
					want := referenceSectorHexes(gameMap, origin, heading, width, radius)
					if got := ReconSectorHexes(gameMap, origin, heading, width, radius); !sameHexes(got, want) {
						t.Fatalf("扇面不一致 origin=%s heading=%s width=%d radius=%d: got %d 格 / want %d 格", origin, heading, width, radius, len(got), len(want))
					}
					if got := ReconRingHexes(gameMap, origin, heading, width, radius); !sameHexes(got, referenceRingHexes(gameMap, origin, heading, width, radius)) {
						t.Fatalf("距离圈不一致 origin=%s heading=%s width=%d ring=%d", origin, heading, width, radius)
					}
					if got := ReconScanBandHexes(gameMap, origin, heading, width, radius); !sameHexes(got, referenceScanBandHexes(gameMap, origin, heading, width, radius)) {
						t.Fatalf("搜索带不一致 origin=%s heading=%s width=%d radius=%d", origin, heading, width, radius)
					}
					for _, coord := range []string{"0101", "0512", "0810", "1112", "1518", "2115"} {
						if got := ReconScannedRingHexes(gameMap, origin, coord, heading, width, radius); !sameHexes(got, referenceScannedRingHexes(gameMap, origin, coord, heading, width, radius)) {
							t.Fatalf("当前扫描带不一致 origin=%s coord=%s heading=%s width=%d radius=%d", origin, coord, heading, width, radius)
						}
					}
				}
			}
		}
	}
}

func TestCoordFormattingPreserved(t *testing.T) {
	cases := [][3]any{{1, 1, "0101"}, {12, 34, "1234"}, {99, 99, "9999"}, {100, 5, "10005"}, {0, 7, "0007"}}
	for _, item := range cases {
		row := item[0].(int)
		col := item[1].(int)
		if got := Coord(row, col); got != item[2].(string) {
			t.Fatalf("Coord(%d,%d)=%q，期望 %q", row, col, got, item[2])
		}
	}
}

// TestReconSectorBoundedScanUnchanged 把对照扩到真实地图尺寸（52×64）和更大的半径：
// 新实现把候选格限定在 origin 的 ±radius 方框内并做角向预剪，这里专门压边界——
// 地图角落、边缘行、radius 大到方框要截断、以及 120° 张角贴轴线的格子。
func TestReconSectorBoundedScanUnchanged(t *testing.T) {
	gameMap := MapDefinition{Rows: 52, Cols: 64}
	origins := []string{"0101", "0120", "2640", "5210", "1450", "3064"}
	for _, origin := range origins {
		for _, heading := range []string{"E", "NW", "SE"} {
			for _, width := range []int{60, 120} {
				for _, radius := range []int{1, 2, 5, 9, 12, 16} {
					want := referenceSectorHexes(gameMap, origin, heading, width, radius)
					if got := ReconSectorHexes(gameMap, origin, heading, width, radius); !sameHexes(got, want) {
						t.Fatalf("大图扇面不一致 origin=%s heading=%s width=%d radius=%d: got %d / want %d", origin, heading, width, radius, len(got), len(want))
					}
					if got := ReconScanBandHexes(gameMap, origin, heading, width, radius); !sameHexes(got, referenceScanBandHexes(gameMap, origin, heading, width, radius)) {
						t.Fatalf("大图搜索带不一致 origin=%s heading=%s width=%d radius=%d", origin, heading, width, radius)
					}
				}
			}
		}
	}
}
