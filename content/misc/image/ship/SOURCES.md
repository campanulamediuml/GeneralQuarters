# 舰船剪影素材

兵牌、地图标记和舰队面板里的舰船剪影，统一来自用户提供的整包识别图剪影
（仓库根目录 `素材/船只剪影/正常`）。原图是黑底透明的侧视轮廓，舰首朝左，
前端用 CSS `mask-image` 只取 alpha 通道、再用 `currentColor` 按阵营和损伤上色，
所以这里的 PNG 只看形状，颜色会被忽略。

`content/units/ships/*.json` 的具名舰和 `content/units/sandbox.json` 的通用舰级卡
都用 `picture` 字段指向本目录，服务端按 `/content/...` 静态下发；
换图、加船的素材都放在 content 里，不需要重新构建前端。

## 当前映射

沿用最早的具名文件名，这样老存档里已经写进卡片的 `picture` 路径不会失效；
文件内容换成了对应舰级的剪影：

| 本目录文件 | 素材文件（素材/船只剪影/正常） | 对应舰级 / 用途 |
|---|---|---|
| north_carolina.png | USA_BB7-1_ship.png | 北卡罗来纳级（美军 BB 卡） |
| usa_bc1.png | USA_BC1_ship.png | 素材内的通用美国战列巡洋舰剪影 |
| northampton.png | USA_CA2_ship.png | 北安普敦级（美军 CA 卡） |
| new_orleans.png | USA_CA4_ship.png | 新奥尔良级 / 威奇塔级 |
| portland.png | USA_CA3_ship.png | 波特兰级 |
| leander.png | USA_CL2_ship.png | 布鲁克林级与利安德级共用（美军 CL 卡、澳大利亚号霍巴特） |
| australia.png | GBR_CA1-2_ship.png | 英国肯特级/伦敦级（澳大利亚号） |
| farragut.png | USA_DD1_ship.png | 美军通用驱逐舰（法拉格特、弗莱彻等） |
| lexington.png | USA_CV1_ship.png | 列克星敦级 |
| yorktown.png | USA_CV3_ship.png | 约克城级 |
| usa_cvl1.png | USA_CVL1_ship.png | 独立级 |
| usa_cve0.png | USA_CVE0_ship.png | 博格 / 卡萨布兰卡 / 长岛 / 桑加蒙级共用 |
| nagato.png | JAP_BB4_ship.png | 长门级（长门/陆奥） |
| fuso.png | JAP_BB1_ship.png | 扶桑级（扶桑/山城） |
| ise.png | JAP_BB2_ship.png | 伊势级常规战列舰形态（伊势/日向；素材里的 JAP_BBV1 是航空战列舰形态，本作未用） |
| kongo.png / haruna.png / hiei.png / kirishima.png | JAP_BC1_ship.png | 金刚级（金刚、榛名、比叡、雾岛同名文件，同一张剪影） |
| akagi.png / kaga.png | JAP_CV1 Akagi_ship.png / JAP_CV1 Kaga_ship.png | 赤城 / 加贺（单舰） |
| shokaku.png | JAP_CV8_ship.png | 翔鹤级（翔鹤、瑞鹤共用） |
| soryu.png | JAP_CV6_ship.png | 苍龙（单舰） |
| hiryu.png | JAP_CV7_ship.png | 飞龙（单舰） |
| shinano.png | JAP_CV5_ship.png | 信浓（单舰，装甲航母） |
| jap_cvl1.png | JAP_CVL1_ship.png | 瑞凤级 |
| jap_cve0.png | JAP_CVE0_ship.png | 大鹰 / 海鹰 / 神鹰共用 |
| myoko.png / furutaka.png | JAP_CA1_ship.png | 古鹰级、青叶级、妙高级共用（日 CA 卡与青叶/古鹰/加古/衣笠/妙高/羽黑） |
| yubari.png | JAP_CL1_ship.png | 舰队通用轻巡剪影（夕张与香取/球磨/长良/川内/天龙级共用） |
| jap_dd1.png | JAP_DD1_ship.png | 日军通用新式驱逐舰（秋月/朝潮/阳炎/岛风/夕云共用） |
| shiratsuyu.png | JAP_DE1_ship.png | 日军通用旧式驱逐舰（白露/时雨/有明/夕暮及吹雪、初春等） |
| cmn_ms1_3.png | CMN_MS1-3_ship.png | 通用运输船（双方 TRS 卡） |
| essex.png | USA_CV5_ship.png | 埃塞克斯级（马里亚纳的 8 艘 CV） |
| south_dakota.png | USA_BB7-2_ship.png | 南达科他级（南达科他/印第安纳/马萨诸塞/阿拉巴马） |
| iowa.png | USA_BB8_ship.png | 衣阿华级（新泽西/依阿华） |
| tennessee.png | USA_BB5_ship.png | 田纳西级 |
| pennsylvania.png | USA_BB3_ship.png | 宾夕法尼亚级（宾夕法尼亚/加利福尼亚） |
| colorado.png | USA_BB6_ship.png | 科罗拉多级（马里兰/科罗拉多） |
| new_mexico.png | USA_BB4_ship.png | 新墨西哥级（新墨西哥/爱达荷/密西西比） |
| baltimore.png | USA_CA5_ship.png | 巴尔的摩级（巴尔的摩/波士顿/芝加哥/昆西/休斯顿） |
| cleveland.png | USA_CL3_ship.png | 克利夫兰级（6 艘 CL） |
| fletcher.png | USA_DD1_ship.png | 弗莱彻级（马里亚纳的 56 艘 DD） |
| usa_de1.png | USA_DE1_ship.png | 美国护航驱逐舰（16 艘 DE） |
| taiho.png | JAP_CV4_ship.png | 大凤级 |
| hiyo.png | JAP_CV2_ship.png | 飞鹰级（隼鹰/飞鹰） |
| unryu.png | JAP_CV3_ship.png | 云龙级（云龙/天城/葛城） |
| ryuho.png | JAP_CVL4_ship.png | 龙凤级 |
| yamato.png | JAP_BB5_ship.png | 大和级（大和/武藏） |
| takao.png | JAP_CA2_ship.png | 高雄级（高雄/爱宕） |
| tone.png | JAP_CA10_ship.png | 利根级（利根/筑摩） |
| mogami.png | JAP_CA3_ship.png | 最上级（最上/铃谷/熊野） |
| agano.png | JAP_CL2_ship.png | 阿贺野级 / 大淀（矢矧/能代/大淀） |

完整的"素材文件 → 舰种 → 舰级"对照在 `素材/船只剪影/ship_classes.csv`。
素材包里还有一批当前剧本没用到（或本作尚未建卡）的舰级图，例如亚特兰大、兰利、
利根 1943 形态等，需要时按同一命名规则抄进本目录并在 JSON 里指过去。

`素材/船只剪影/沉没` 是沉没状态的单独剪影，目前前端没有用到：
沉没舰船是给同一张剪影加倾斜、灰度和半透明来表现（`ShipCounter.vue`），
如果以后要做"残骸图"，可以在卡片上再加一个 `picture_sunk` 字段。

## 新增素材的硬性要求

放进本目录的 PNG **必须是带 alpha 的 RGBA 蒙版**（不透明 = 舰体，透明 = 背景）。
CSS `mask-image` 默认读 alpha 通道：白底 JPG、灰度图或整块不透明的 PNG，
在兵牌上会渲染成一个实心矩形。`TestShipPicturesCarryAlpha` 会检查每张图至少有一个透明像素。

- 舰首朝左，和素材包保持一致，同级舰之间不要混方向。
- 不要给图加边框、水印、尺度标尺；剪影要裁到舰体本身。
- 长宽比保持素材原样（多为 5:1），前端用 `mask-size: contain` 等比缩放，
  在 48×20 的地图标记和 128×72 的兵牌里都不会被拉伸。
