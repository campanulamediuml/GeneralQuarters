# 航空算子图片

航空算子和舰船用同一套配置：`content/units/*.json` 的 `aircraft[].picture` 指向本目录的图片，
服务端按 `/content/misc/image/aircraft/...` 静态下发。换图、加机型只改数据目录，不需要重新构建前端；
`picture` 为空或图片 404 时，`AircraftCounter.vue` 回退到 `web/src/silhouettes.ts` 里的内置矢量剪影。

当前素材是用户提供的**俯视朝上彩色图**（120×120 RGBA，透明背景，机头朝上），
直接原样渲染，所以 `picture_mode` 留空或写 `image`。
只有显式写 `"picture_mode": "mask"` 时，前端才会走 CSS `mask-image`，只取 alpha 通道、
用兵牌当前颜色染成单色剪影——那是给纯剪影素材用的，不适合彩图。

素材来源是 `素材/飞机算子/俯视` 里的 `<机型>Top@2x.png`（统一俯视、机头朝下），
入库时旋转 180° 变成机头朝上。没有直接用 `素材/飞机算子/俯视朝上` 整包，是因为那个包里
B5N/TBD/G3M/H6K 是侧视图、D3A/SBD/G4M/B25 是斜视（3/4 视角），只有 A6M/F4F/P40 等
少数是真正的俯视图；那几张和 `俯视` 包旋转后的结果是同一张画，所以统一走 `俯视` 包更一致。

| 文件 | 素材文件（素材/飞机算子/俯视） | 对应机型 |
|---|---|---|
| f4f.png | F4FTop@2x.png（旋转 180°） | F4F 野猫（美军战斗机卡） |
| sbd.png | SBDTop@2x.png（旋转 180°） | SBD 无畏（美军俯冲轰炸机卡） |
| tbd.png | TBDTop@2x.png（旋转 180°） | TBD 蹂躏者（美军鱼雷机卡） |
| soc.png | SOCTop@2x.png（旋转 180°） | SOC 海鸥（美军水上侦察机卡） |
| p40.png | P40Top@2x.png（旋转 180°） | P-40 战鹰（陆基战斗机） |
| b25.png | B25Top@2x.png（旋转 180°） | B-25 米切尔（陆基中型轰炸机） |
| b26.png | B26Top@2x.png（旋转 180°） | B-26 劫掠者（陆基中型轰炸机，可挂鱼雷） |
| a6m2.png | A6MTop@2x.png（旋转 180°） | A6M2 零战（日军战斗机卡） |
| a5m.png | A5MTop@2x.png（旋转 180°） | A5M 九六舰战（日军旧式战斗机） |
| d3a1.png | D3ATop@2x.png（旋转 180°） | D3A1 九九舰爆 |
| b5n2.png | B5NTop@2x.png（旋转 180°） | B5N2 九七舰攻 |
| e13a.png | E13ATop@2x.png（旋转 180°） | E13A 零式水侦 |
| h6k.png | H6KTop@2x.png（旋转 180°） | H6K 九七式飞行艇 |
| g4m.png | G4MTop@2x.png（旋转 180°） | G4M 一式陆攻 |
| b17.png | B17Top@2x.png（旋转 180°） | B-17 空中堡垒（陆基战略轰炸机） |
| pby.png | PBYTop@2x.png（旋转 180°，60×60 放大到 120×120） | PBY 卡特里娜（水上侦察机） |
| g3m.png | G3MTop@2x.png（旋转 180°） | G3M 九六陆攻 |
| f6f.png | F6FTop@2x.png（旋转 180°） | F6F-5 地狱猫（1944 马里亚纳剧本） |
| sb2c.png | SB2CTop@2x.png（旋转 180°） | SB2C-4 地狱俯冲者 |
| tbf.png | TBFTop@2x.png（旋转 180°） | TBF/TBM 复仇者（TBF 与 TBM 是同一机型的两家厂产） |
| a6m5.png | A6M3Top@2x.png（旋转 180°） | A6M5 零战52（素材包没有 A6M5，暂借 A6M3 的俯视图） |
| d4y.png | D4YTop@2x.png（旋转 180°） | D4Y2 彗星 |
| b6n.png | B6NTop@2x.png（旋转 180°） | B6N2 天山 |
| p1y.png | P1YTop@2x.png（旋转 180°） | P1Y1 银河 |
| h8k.png | H8KTop@2x.png（旋转 180°） | H8K2 二式大艇 |

素材包里还有大量本作尚未建卡的机型（F4U、B-24、B-29、N1K、Ki-84、PBM、PB4Y……），
文件名去掉 `@2x` 就是机型代号；加机型时把对应 PNG 抄进本目录、在 `units/*.json` 的
`aircraft` 里补一条并写上 `picture` 即可。`素材/飞机算子/俯视` 是侧视（机头朝右）版本，
`通用标记` 是棋盘游戏里的通用/染色棋子，两者都没有被游戏引用。

## 素材要求

- 透明背景。彩图如果带白底/黑底，会直接显示成一块矩形；mask 模式下不透明图会变成实心方块。
- 机头朝上（俯视），和现有素材一致；前端用 `object-fit: contain` 等比缩放，不会裁切。
- 120×120 的画布本身带留白，保持这个比例时不同机型的翼展大小关系不会失真。
- 彩图在 84px 和任务栏 52px 两种尺寸下都会显示，细节别做得太密，必要时单独出一版简化图。
