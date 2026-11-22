package game

import (
	"math"
	"testing"
)

// TestReconHeadingStepsToNeighbor 校验侦察航向每一步都落在相邻 hex 上、并且朝向正确。
// 之前奇偶行的偏移表写反了，侦察机会一次跨两列、跳过中间距离圈，看起来像"没找到敌人"。
func TestReconHeadingStepsToNeighbor(t *testing.T) {
	gameMap := MapDefinition{Rows: 7, Cols: 9}
	for _, origin := range []string{"0202", "0303", "0602", "0506"} {
		originX, originY := hexCenter(ParseRowCol(t, origin))
		for heading, want := range reconHeadings {
			next := nextReconCoord(gameMap, origin, heading)
			if next == "" {
				continue
			}
			if Distance(origin, next) != 1 {
				t.Fatalf("%s 沿 %s 走到了非相邻格 %s", origin, heading, next)
			}
			nextX, nextY := hexCenter(ParseRowCol(t, next))
			angle := math.Atan2(nextY-originY, nextX-originX) * 180 / math.Pi
			if diff := math.Mod(angle-want+540, 360) - 180; math.Abs(diff) > 31 {
				t.Fatalf("%s 沿 %s 实际朝向 %.0f°，偏差 %.0f°", origin, heading, angle, diff)
			}
		}
	}
}

// ParseRowCol 把 ABCD 坐标拆成 hexCenter 需要的行列。
func ParseRowCol(t *testing.T, coord string) (int, int) {
	t.Helper()
	row, col, err := ParseCoord(coord)
	if err != nil {
		t.Fatal(err)
	}
	return row, col
}

// TestHangarRefillKeepsFuelLedger 校验机库补油不会把副油箱并进机内油：
// 副油箱里程和备用油都必须保留，否则抛弃副油箱和备用油透支判定都失效。
func TestHangarRefillKeepsFuelLedger(t *testing.T) {
	aircraft := &AirUnit{BaseRange: 14, MaxRange: 18, Loadout: LoadoutBombTank}
	aircraft.SetRange(18) // 模拟旧逻辑留下的扁平账本
	refillFuelForLoadout(aircraft)
	if aircraft.Fuel.Tank != 4 || aircraft.Fuel.Internal != 14 || aircraft.Fuel.Reserve != ReserveRange {
		t.Fatalf("机库补油后燃料账本错误：%+v", aircraft.Fuel)
	}
	if aircraft.Range() != 18 {
		t.Fatalf("补油后可用航程=%d，期望18", aircraft.Range())
	}
	if wasted := dropTank(aircraft); wasted != 4 {
		t.Fatalf("抛弃副油箱浪费=%d，期望4", wasted)
	}
	if aircraft.Range() != 14 {
		t.Fatalf("抛弃副油箱后航程=%d，期望14", aircraft.Range())
	}
}

// TestLandingFailureByReserveDebt 校验备用油透支与着舰失败率的对应关系。
func TestLandingFailureByReserveDebt(t *testing.T) {
	cases := map[int]float64{4: 0, 0: 0, -1: 10, -2: 30, -3: 50, -4: 70, -5: 100, -6: 100}
	for reserve, want := range cases {
		if got := landingFailureChance(reserve); got != want {
			t.Fatalf("备用油%d → 失败率%v，期望%v", reserve, got, want)
		}
	}
}
