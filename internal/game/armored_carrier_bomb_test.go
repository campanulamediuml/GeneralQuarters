package game

import "testing"

// 装甲航母的机库不是免单：炸弹要先过一次"击穿装甲甲板"的 DCT。
// 损管成功＝装甲甲板扛住，机库飞机不受影响；损管失败＝打穿，机库按普通航母结算。
func TestArmoredDeckPenetrationRequiresDCT(t *testing.T) {
	doctrines := map[string]string{"A": DoctrineUS, "B": DoctrineJapan}
	const runs = 20000
	// 返回每发命中的平均毁机数：击穿才会毁 2 架，所以它约等于 2 × 击穿率。
	lossPerHit := func(dc int) float64 {
		card := factionProbeCard(dc, false, true, 20)
		lost := 0
		for run := 0; run < runs; run++ {
			destroyed, _, _ := factionHangarHit(card, "B", doctrines, 18, true, uint64(run)*2654435761+19)
			lost += destroyed
		}
		return float64(lost) / float64(runs)
	}
	strong := lossPerHit(9) // 损管 9：只有 10% 会被打穿
	weak := lossPerHit(1)   // 损管 1：90% 会被打穿
	t.Logf("装甲航母每发毁机：损管9 = %.2f 架，损管1 = %.2f 架", strong, weak)
	if strong > 0.45 {
		t.Fatalf("高损管的装甲航母应该很难被打穿，实际每发毁 %.2f 架", strong)
	}
	if weak < 1.4 {
		t.Fatalf("低损管的装甲航母应该基本被打穿，实际每发毁 %.2f 架", weak)
	}
	if !(weak > strong*2) {
		t.Fatalf("击穿率应随损管值明显变化：损管1 %.2f / 损管9 %.2f", weak, strong)
	}
}
