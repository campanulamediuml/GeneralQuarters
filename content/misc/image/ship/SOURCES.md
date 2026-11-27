# 舰船剪影素材

兵牌、地图标记和舰队面板里的舰船剪影，直接用用户提供的整包识别图
（仓库根目录 `素材/船只剪影/正常`），**整包复制**到 `content/misc/image/ship/library/`，
保留素材原文件名，`content/units/*.json` 的 `picture` 直接引用这些原图：

```json
"picture": "misc/image/ship/library/JAP_BB1_ship.png"
```

原图是黑底透明的侧视轮廓，舰首朝左，前端用 CSS `mask-image` 只取 alpha 通道、
再用 `currentColor` 按阵营和损伤上色，所以这里的 PNG 只看形状，颜色会被忽略。
新增舰船时先看 `素材/船只剪影/正常` 里有没有对应舰级，有就直接写 `library/<原名>`，
不用再另存一份别名文件。

同步素材用 `scripts/import-assets.sh`：舰船剪影整包 rsync（保留原名），
航空算子按下面的映射旋转 180° 生成。手工换图时只要保证是「`library/` 里已有的文件 + JSON 指向它」即可。

## 素材原图 → 舰级对照（常用部分）

完整对照见 `素材/船只剪影/ship_classes.csv`。

| 素材文件 | 舰级 / 用途 |
|---|---|
| JAP_BB1_ship.png | 扶桑级（扶桑、山城） |
| JAP_BB2_ship.png | 伊势级常规战列舰形态（伊势、日向） |
| JAP_BBV1_ship.png | 伊势级航空战列舰形态（本作未用） |
| JAP_BB4_ship.png | 长门级（长门、陆奥） |
| JAP_BB5_ship.png | 大和级（大和、武藏） |
| JAP_BC1_ship.png | 金刚级（金刚、榛名、比叡、雾岛） |
| JAP_CV1 Akagi_ship.png / JAP_CV1 Kaga_ship.png | 赤城 / 加贺（单舰） |
| JAP_CV2_ship.png | 飞鹰级（隼鹰、飞鹰） |
| JAP_CV3_ship.png | 云龙级（云龙、天城、葛城） |
| JAP_CV4_ship.png | 大凤级 |
| JAP_CV5_ship.png | 信浓（单舰，装甲航母） |
| JAP_CV6_ship.png / JAP_CV7_ship.png | 苍龙 / 飞龙（两张不同剪影） |
| JAP_CV8_ship.png | 翔鹤级（翔鹤、瑞鹤） |
| JAP_CVL1_ship.png | 瑞凤级（瑞凤、千岁、千代田） |
| JAP_CVL4_ship.png | 瑞凤级与龙凤级共用（龙凤） |
| JAP_CA1_ship.png | 古鹰级、青叶级、妙高级共用 |
| JAP_CA2_ship.png | 高雄级（高雄、爱宕、摩耶、鸟海） |
| JAP_CA3_ship.png | 最上级（最上、铃谷、熊野） |
| JAP_CA10_ship.png | 利根级（利根、筑摩） |
| JAP_CL1_ship.png | 通用轻巡（川内、神通等） |
| JAP_CL2_ship.png | 阿贺野级与大淀级（矢矧、能代、阿贺野、大淀） |
| JAP_DD1_ship.png | 日军通用新式驱逐舰 |
| JAP_DE1_ship.png | 日军通用旧式驱逐舰（白露级等具名舰） |
| USA_* | 美军舰级，见 `ship_classes.csv` |
| CMN_MS1-3_ship.png | 通用运输船（双方 TRS） |

## 旧版别名文件（只为老存档保留）

`content/misc/image/ship/` 根目录下的一批 `nagato.png`、`kongo.png` 这类别名图是早期
"按舰级各存一份"的产物，内容与 `library/` 里对应原图逐字节相同。现在没有 JSON 再引用它们，
保留只是为了老房间存档里写死的老路径还能出图。新建/改剧本一律用 `library/` 原图。

## 新增素材的硬性要求

放进本目录的 PNG **必须是带 alpha 的 RGBA 蒙版**（不透明 = 舰体，透明 = 背景）。
CSS `mask-image` 默认读 alpha 通道：白底 JPG、灰度图或整块不透明的 PNG，
在兵牌上会渲染成一个实心矩形。`TestShipPicturesCarryAlpha` 会检查每张图至少有一个透明像素。

- 舰首朝左，和素材包保持一致，同级舰之间不要混方向。
- 不要给图加边框、水印、尺度标尺；剪影要裁到舰体本身。
- 长宽比保持素材原样（多为 5:1），前端用 `mask-size: contain` 等比缩放，
  在 48×20 的地图标记和 128×72 的兵牌里都不会被拉伸。
- 文件名里可以有空格（素材原样），前端会做 URL 逐段转义；但 JSON 里要写完整原名。
