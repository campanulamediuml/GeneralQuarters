package game

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestEveryCatalogPictureFileExists 校验 units/*.json 里写的 picture 都能在 content/ 下找到，
// 避免改名/换图之后 JSON 指向一个 404 路径（前端拿不到图只会静默留白）。
func TestEveryCatalogPictureFileExists(t *testing.T) {
	_, _, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	root := "../../content"
	check := func(label, picture string) {
		if picture == "" {
			t.Errorf("%s 没有 picture：舰船/机型换图时不要漏字段", label)
			return
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(picture))); err != nil {
			t.Errorf("%s picture=%q 找不到文件：%v", label, picture, err)
		}
	}
	for _, unit := range catalog.Units {
		check("舰船 "+unit.ID, unit.Picture)
	}
	for _, aircraft := range catalog.Air {
		check("飞机 "+aircraft.Faction+"-"+aircraft.ID, aircraft.Picture)
	}
}

// TestShipPicturesCarryAlpha 校验舰船图片是带 alpha 的蒙版：前端用 CSS mask-image 取 alpha 上色，
// 如果整张图没有透明像素（白底 JPG、纯色块），兵牌上就会是一个实心矩形。
func TestShipPicturesCarryAlpha(t *testing.T) {
	_, _, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	root := "../../content"
	seen := map[string]bool{}
	for _, unit := range catalog.Units {
		if unit.Picture == "" || seen[unit.Picture] {
			continue
		}
		seen[unit.Picture] = true
		if filepath.Ext(unit.Picture) != ".png" {
			continue
		}
		file, err := os.Open(filepath.Join(root, filepath.FromSlash(unit.Picture)))
		if err != nil {
			t.Fatalf("%s 打不开：%v", unit.Picture, err)
		}
		img, err := png.Decode(file)
		file.Close()
		if err != nil {
			t.Fatalf("%s 不是合法 PNG：%v", unit.Picture, err)
		}
		bounds := img.Bounds()
		transparent, opaque := 0, 0
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				_, _, _, alpha := img.At(x, y).RGBA()
				if alpha == 0 {
					transparent++
				} else {
					opaque++
				}
			}
		}
		if transparent == 0 {
			t.Errorf("%s 没有透明像素：mask-image 会渲染成实心方块，请先去掉白底", unit.Picture)
		}
		if opaque == 0 {
			t.Errorf("%s 全透明：画不出舰体", unit.Picture)
		}
	}
}
