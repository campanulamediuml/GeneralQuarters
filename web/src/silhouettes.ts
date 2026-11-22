// 二战舰艇 / 舰载机 / 水侦共用的剪影几何。
// 全部按「船头朝右、甲板线统一落在 y=27」绘制，便于兵牌、地图算子、角色卡复用同一套造型。
// 地图算子按 2/3 缩放放进原来的 52×36 卡面，因此同一绘制在 3 个尺寸下都保持比例一致。
// 造型取自二战舰种特征：平甲板航母、背负式炮塔战列舰、三烟囱四联装重巡、迁就航速的驱逐舰。

import { a6m2, b5n2, d3a1, e13a, e13aFloats } from './aircraft-silhouettes'

export type ShipSilhouetteKind = 'CV' | 'CVL' | 'CVE' | 'BB' | 'BC' | 'CA' | 'CL' | 'DD' | 'TRS'
export type AircraftSilhouetteKind = 'fighter' | 'dive_bomber' | 'torpedo_bomber' | 'recon' | 'seaplane'

export interface ShipSilhouettePart {
  // 部件使用的配色：舰体、上层建筑、飞行甲板、机库、炮塔、烟囱
  tone: 'hull' | 'super' | 'deck' | 'hangar' | 'turret' | 'funnel'
  d: string
}

export interface ShipSilhouette {
  hull: string
  parts: ShipSilhouettePart[]
}

// 九座炮塔的 BB：二号炮塔之上再叠一座，三号为背负面朝后的三联装；矩形炮弹散布表示单舰（原「炮塔 + 烟囱」造型是旧的占位画法）。
function turret(x: number, y: number, w: number, h: number): ShipSilhouettePart {
  return { tone: 'turret', d: `M${x} ${y}h${w}v${h}h${-w}z` }
}

// 约克城级/埃塞克斯级那种「方正海鸥翼」舰体，右舷舰岛、双层机库、前部一部升降机。不是现代航母的斜角甲板造型。
const fleetCV: ShipSilhouette = {
  hull: 'M3 28h110l-9 10H14z',
  parts: [
    { tone: 'hangar', d: 'M12 22h94v6H10z' },
    { tone: 'hangar', d: 'M18 16h84v6H16z' },
    { tone: 'deck', d: 'M2 18h116v9H2z' },
    { tone: 'super', d: 'M78 7h16v11H78z' },
    { tone: 'super', d: 'M81 2h3v5h-3z' },
    { tone: 'funnel', d: 'M84 10h7v4h-7z' },
    turret(8, 13, 6, 5),
    turret(101, 13, 5, 5),
    { tone: 'deck', d: 'M58 18h8v1.6h-8z' },
  ],
}

// 独立级轻母：飞行甲板长而平直，舰岛极矮靠后，右舷一体式烟囱，艉部有水上飞机吊车。
const lightCVL: ShipSilhouette = {
  hull: 'M5 28h107l-9 9H16z',
  parts: [
    { tone: 'hangar', d: 'M14 22h88v6H12z' },
    { tone: 'deck', d: 'M3 19h113v9H3z' },
    { tone: 'super', d: 'M80 12h13v7H80z' },
    { tone: 'funnel', d: 'M84 7h6v5h-6z' },
    turret(10, 14, 5, 5),
    turret(103, 14, 5, 5),
    { tone: 'super', d: 'M20 26h12v1.6H20z' },
  ],
}

// 卡萨布兰卡级护航航母：商船改造，方尾、高干舷，甲板比舰体短一截，小舰岛居中偏右。
const escortCVE: ShipSilhouette = {
  hull: 'M6 28h104l-6 10H20z',
  parts: [
    { tone: 'hangar', d: 'M16 22h76v6H14z' },
    { tone: 'deck', d: 'M6 18h104v9H6z' },
    { tone: 'super', d: 'M70 10h11v8H70z' },
    { tone: 'funnel', d: 'M73 6h5v4h-5z' },
    turret(12, 12, 5, 5),
    turret(99, 12, 5, 5),
  ],
}

// 大和/衣阿华一类战列舰：宽阔舰体、前部两座背负式三联装、后部一座，两组烟囱与高大舰桥。
const battleship: ShipSilhouette = {
  hull: 'M2 27h116l-10 11H14z',
  parts: [
    { tone: 'hull', d: 'M10 23h100v4H10z' },
    { tone: 'super', d: 'M46 14h24v9H46z' },
    { tone: 'super', d: 'M52 8h13v6H52z' },
    { tone: 'super', d: 'M57 2h3v6h-3z' },
    { tone: 'funnel', d: 'M49 10h5v3h-5z' },
    { tone: 'funnel', d: 'M73 12h6v3h-6z' },
    turret(20, 15, 11, 7),
    turret(33, 19, 11, 7),
    turret(88, 19, 11, 7),
    turret(101, 14, 8, 6),
    { tone: 'super', d: 'M60 16h2v2h-2z' },
  ],
}

// 战列巡洋舰：比战列舰修长，主炮三座三联装，后方烟囱更靠后。
const battlecruiser: ShipSilhouette = {
  hull: 'M4 27h112l-11 10H16z',
  parts: [
    { tone: 'super', d: 'M49 14h21v9H49z' },
    { tone: 'super', d: 'M54 8h11v6H54z' },
    { tone: 'super', d: 'M58 2h3v6h-3z' },
    { tone: 'funnel', d: 'M51 10h5v3h-5z' },
    { tone: 'funnel', d: 'M76 12h5v3h-5z' },
    turret(24, 16, 10, 7),
    turret(36, 19, 10, 7),
    turret(90, 19, 10, 7),
  ],
}

// 重巡洋舰：高耸塔式舰桥、三座双联装重炮、舷侧鱼雷发射管。
const heavyCruiser: ShipSilhouette = {
  hull: 'M4 27h112l-12 9H18z',
  parts: [
    { tone: 'super', d: 'M50 15h19v8H50z' },
    { tone: 'super', d: 'M54 9h11v6H54z' },
    { tone: 'super', d: 'M57 3h4v6h-4z' },
    { tone: 'funnel', d: 'M53 11h5v3h-5z' },
    { tone: 'funnel', d: 'M71 12h5v3h-5z' },
    turret(25, 17, 8, 6),
    turret(37, 20, 8, 6),
    turret(89, 19, 8, 6),
    { tone: 'super', d: 'M58 23h14v1.6H58z' },
  ],
}

// 轻巡洋舰：比重巡短一线，炮塔更小更密，舰尾带水上飞机弹射器。
const lightCruiser: ShipSilhouette = {
  hull: 'M6 27h109l-13 9H20z',
  parts: [
    { tone: 'super', d: 'M52 15h14v8H52z' },
    { tone: 'super', d: 'M55 9h8v6H55z' },
    { tone: 'super', d: 'M57 4h4v5h-4z' },
    { tone: 'funnel', d: 'M54 11h4v3h-4z' },
    { tone: 'funnel', d: 'M68 12h4v3h-4z' },
    turret(31, 18, 7, 6),
    turret(42, 20, 7, 6),
    turret(84, 19, 7, 6),
    { tone: 'super', d: 'M92 18h10v2.4H92zM95 15h5v3h-5z' },
  ],
}

// 驱逐舰：低干舷，四联装鱼雷发射管、后部深弹导轨和吊车，舰桥紧凑。
const destroyer: ShipSilhouette = {
  hull: 'M8 27h105l-14 8H24z',
  parts: [
    { tone: 'super', d: 'M53 16h14v7H53z' },
    { tone: 'super', d: 'M56 11h8v5H56z' },
    { tone: 'funnel', d: 'M55 13h4v3h-4z' },
    { tone: 'funnel', d: 'M68 14h4v3h-4z' },
    turret(33, 20, 8, 6),
    turret(82, 20, 7, 6),
    { tone: 'super', d: 'M60 23h14v2H60z' },
    { tone: 'super', d: 'M14 25h13v1.6H14z' },
  ],
}

// 运输船：高干舷、方尾、舱盖与单烟囱，船鼻桅靠右。
const transport: ShipSilhouette = {
  hull: 'M4 26h112l-5 12H22z',
  parts: [
    { tone: 'hangar', d: 'M18 20h28v5H18zM54 20h24v5H54zM86 20h18v5H86z' },
    { tone: 'super', d: 'M28 12h15v8H28z' },
    { tone: 'funnel', d: 'M62 10h7v10h-7z' },
    { tone: 'super', d: 'M92 14h12v6H92z' },
    { tone: 'super', d: 'M56 5h3v5h-3z' },
  ],
}

export const shipSilhouettes: Record<ShipSilhouetteKind, ShipSilhouette> = {
  CV: fleetCV,
  CVL: lightCVL,
  CVE: escortCVE,
  BB: battleship,
  BC: battlecruiser,
  CA: heavyCruiser,
  CL: lightCruiser,
  DD: destroyer,
  TRS: transport,
}

// —— 舰载机 / 水上飞机：俯视剪影，机头朝上 ——

// F4U 海盗 / F6F 地狱猫一类舰载战斗机：平直翼，机翼后缘有折线。
const fighter: string = 'M31 1c1.6 0 2.6 1.4 2.6 3.2V11l2.2 1.8V18l19 1.2v3.4l-19 .8v10.2l4.3 2.8v2.4l-9.7-1.4-9.7 1.4v-2.4l4.3-2.8V23.4l-19-.8v-3.4L24.6 18V12.8l2.2-1.8V4.2C26.8 2.4 29.4 1 31 1Z'

// SBD 无畏 / SB2C 地狱俯冲者：机身粗短，机翼外段收窄（俯视高度收起的折翼效果）。
const diveBomber: string = 'M31 2c1.8 0 3 1.5 3 3.4V13l2 1.6V20l18 1v3.6l-17 .9v9.2l4.6 3v2.6l-10.6-1.5-10.6 1.5v-2.6l4.6-3v-9.2l-17-.9V20l18-1v-5.4l2-1.6V5.4C28 3.5 29.2 2 31 2Z'

// TBF 复仇者：翼展最宽，机背炮塔舱明显，垂尾宽大。
const torpedoBomber: string = 'M31 2c2 0 3.4 1.6 3.4 3.6V12l2 1.6V20l25 1.3v3.8l-24 1v8.6l5 3.2v2.8l-12.4-1.6L17.6 40.9v-2.8l5-3.2V26l-24-1v-3.8L23.6 20v-6.4L27.6 12V5.6C27.6 3.6 29 2 31 2Z'

// F6F-5P 侦察型：同战斗机，机头两侧加摄像机整流罩。
const recon: string = 'M31 1c1.6 0 2.6 1.4 2.6 3.2V11l2.2 1.8V18l19 1.2v3.4l-19 .8v10.2l4.3 2.8v2.4l-9.7-1.4-9.7 1.4v-2.4l4.3-2.8V23.4l-19-.8v-3.4L24.6 18V12.8l2.2-1.8V4.2C26.8 2.4 29.4 1 31 1Z'

// SOC 海鸥：平直上单翼、敞开座舱，挂一个水上浮筒。
const seaplane: string = 'M31 3c1.4 0 2.4 1.2 2.4 2.8V12l2 1.4V19l17.6 1v3.2l-17.6 .9v9l4 2.6v2.2l-8.4-1.3-8.4 1.3v-2.2l4-2.6v-9l-17.6-.9V20L26.6 19v-5.6l2-1.4V5.8C28.6 4.2 29.6 3 31 3Z'

// 浮筒：海鸥的浮筒画在机翼下方两侧，与机体同色。
export const seaplaneFloats: string = 'M18.4 30.5h8.6l-2.4 7h-5.2zM35 30.5h8.6l-2.4 7H36z'

export const aircraftSilhouettes: Record<AircraftSilhouetteKind, string> = {
  fighter,
  dive_bomber: diveBomber,
  torpedo_bomber: torpedoBomber,
  recon,
  seaplane,
}

// 机型 → 剪影：美军 4 种沿用原造型（本来就是照 F6F/SB2C/TBF/SOC 画的），日军 4 种单独画。
export const aircraftByModel: Record<string, string> = {
  F6F: fighter,
  SB2C: diveBomber,
  SBD: diveBomber,
  TBF: torpedoBomber,
  TBD: torpedoBomber,
  SOC: seaplane,
	A6M2: a6m2,
	D3A1: d3a1,
	B5N2: b5n2,
	E13A: e13a,
}

// 浮筒：水机才有，按机型选对应的一套。
export function aircraftFloats(kind?: string, model?: string): string {
  const text = (model || '').toUpperCase()
  if (text.startsWith('E13A')) return e13aFloats
  if (kind === 'seaplane' || text.startsWith('SOC')) return seaplaneFloats
  return ''
}


// 舰船按舰种给一个通用造型：只在舰船卡没有 picture（老存档、临时单位）时兜底。
// 真实剪影一律来自 content/misc/image/ship/*.png，由卡片的 picture 字段指路。
export function shipSilhouette(type?: string): ShipSilhouette {
  return shipSilhouettes[(type as ShipSilhouetteKind) || 'DD'] || shipSilhouettes.DD
}

// 机型按型号前缀匹配：'F6F 地狱猫' / 'A6M2 零战' 都能命中。
export function aircraftSilhouette(kind?: string, model?: string): string {
  const text = (model || '').toUpperCase()
  for (const [prefix, path] of Object.entries(aircraftByModel)) {
    if (text.startsWith(prefix)) return path
  }
  return aircraftSilhouettes[(kind as AircraftSilhouetteKind) || 'fighter'] || aircraftSilhouettes.fighter
}
