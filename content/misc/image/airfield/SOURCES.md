# 机场标识图片

规则书 92 行：机场算子中间一行是"机场标识图片"。图片按机场规模分档，运行时由服务端
`AirfieldPicture(type, override)` 按 `type` 选图，剧本可以在 `airfields[].picture` 里单独覆盖。

| 文件 | 用途 | 对应机场 type | 原素材 |
|---|---|---|---|
| `small.png` | 小型机场：一条跑道 | `small`（只能起降战斗机） | `素材/机场/AirfieldSmall.png` |
| `medium.png` | 中型机场：两条跑道交叉 | `medium`（除战略轰炸机外全部可起降） | `素材/机场/AirfieldMedium.png` |
| `large.png` | 大型机场：三条跑道 | `large`（所有机型） | `素材/机场/AirfieldLarge.png` |
| `seaplane.png` | 水上飞机基地徽章 | `seaplane`（只能操作水上飞机） | `素材/机场/SeaplanesBase.png` |

- 三张跑道符号是透明底黑色图形，画在地图上的浅色方块里；水机基地是一枚圆形彩色徽章，不走方块。
- 早期剧本/存档写过 `field` / `heavy` / `major`，服务端读取时按 `NormalizeAirfieldType` 归一成 small/medium/large（"重型"是错别字，规则里就是中型）。
- 珊瑚海剧本：莫尔兹比港 = 中型，拉包尔 = 中型（0323，2 条跑道），凯恩斯 = 大型，图拉吉与维拉 = 水机基地。
- 换图只要覆盖同名文件并重启（服务端启动时把内嵌 content 同步到数据目录）。
