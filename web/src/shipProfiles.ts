// 舰船侧视图：照 Shipbucket 那类识别图的画法，用参数化的小积木拼出来（船体、干舷、水线下涂装、
// 飞行甲板、舰岛、烟囱、桅杆、炮塔、吊车、甲板机影），每艘船只要给一组数字。
// 视图框固定 0..620 × 0..150，船头朝右，水线在 y=118。

export interface ProfileParts {
  hull: string
  boot: string
  deck?: string
  superstructure: string[]
  funnels: string[]
  masts: string[]
  turrets: string[]
  extras: string[]
}

export interface ProfileSpec {
  /** 船体长度（从艉到艏的 x 跨度） */
  length: number
  /** 干舷高度：水线到主甲板 */
  freeboard: number
  /** 水线下涂装厚度 */
  boot: number
  /** 艉部起点 */
  stern: number
  /** 飞行甲板（航母） */
  flightDeck?: { thickness: number; overhang: number }
  island?: { x: number; w: number; h: number; funnel?: boolean }
  funnels: { x: number; w: number; h: number }[]
  masts: { x: number; h: number; tripod?: boolean }[]
  turrets: { x: number; w: number; h: number; barrels?: number; dir?: 1 | -1 }[]
  cranes?: number[]
  deckAircraft?: number
  /** 甲板室（连续上层建筑） */
  deckhouse?: { x: number; w: number; h: number }[]
}

const WATERLINE = 118
const BOW = 610

function hullPath(spec: ProfileSpec): string {
  const stern = spec.stern
  const bow = stern + spec.length
  const deckY = WATERLINE - spec.freeboard
  const keelY = WATERLINE + spec.boot
  // 艉部略低、艏部略高，舰艏外飘一点
  return [
    `M${stern} ${deckY + 3}`,
    `L${bow - 26} ${deckY}`,
    `Q${bow - 6} ${deckY - 2} ${bow} ${deckY + 6}`,
    `L${bow - 4} ${WATERLINE}`,
    `L${bow - 10} ${keelY}`,
    `L${stern + 12} ${keelY}`,
    `Q${stern + 2} ${keelY - 2} ${stern} ${keelY - 8}`,
    'Z',
  ].join(' ')
}

function bootPath(spec: ProfileSpec): string {
  const stern = spec.stern
  const bow = stern + spec.length
  const keelY = WATERLINE + spec.boot
  return [
    `M${stern} ${WATERLINE}`,
    `L${bow - 4} ${WATERLINE}`,
    `L${bow - 10} ${keelY}`,
    `L${stern + 12} ${keelY}`,
    `L${stern + 6} ${WATERLINE}`,
    'Z',
  ].join(' ')
}

export function buildProfile(spec: ProfileSpec): ProfileParts {
  const stern = spec.stern
  const deckY = WATERLINE - spec.freeboard
  const parts: ProfileParts = {
    hull: hullPath(spec),
    boot: bootPath(spec),
    superstructure: [],
    funnels: [],
    masts: [],
    turrets: [],
    extras: [],
  }

  if (spec.flightDeck) {
    const t = spec.flightDeck.thickness
    const over = spec.flightDeck.overhang
    parts.deck = `M${stern - over} ${deckY} h${spec.length + over * 2} v${t} h${-(spec.length + over * 2)} Z`
  }

  for (const house of spec.deckhouse || []) {
    parts.superstructure.push(`M${house.x} ${deckY - house.h} h${house.w} v${house.h} h${-house.w} Z`)
  }

  if (spec.island) {
    const { x, w, h } = spec.island
    parts.superstructure.push(`M${x} ${deckY - h} h${w} v${h} h${-w} Z`)
    if (spec.island.funnel) {
      const fx = x + w * 0.15
      parts.funnels.push(`M${fx} ${deckY - h - 8} h${w * 0.5} v8 h${-w * 0.5} Z`)
    }
  }

  for (const funnel of spec.funnels) {
    parts.funnels.push(`M${funnel.x} ${deckY - funnel.h} h${funnel.w} v${funnel.h} h${-funnel.w} Z`)
  }

  for (const mast of spec.masts) {
    const top = deckY - mast.h
    parts.masts.push(`M${mast.x} ${top} v${mast.h} h2 v${-mast.h} Z`)
    if (mast.tripod) {
      parts.masts.push(`M${mast.x - 10} ${deckY - 6} L${mast.x + 1} ${top + mast.h * 0.35}`)
      parts.masts.push(`M${mast.x + 12} ${deckY - 6} L${mast.x + 1} ${top + mast.h * 0.35}`)
    }
  }

  for (const turret of spec.turrets) {
    const y = deckY - turret.h
    parts.turrets.push(`M${turret.x} ${y} h${turret.w} v${turret.h} h${-turret.w} Z`)
    const barrels = turret.barrels ?? 2
    const dir = turret.dir ?? 1
    for (let i = 0; i < barrels; i += 1) {
      const by = y + (turret.h / (barrels + 1)) * (i + 1)
      const from = dir > 0 ? turret.x + turret.w : turret.x
      parts.turrets.push(`M${from} ${by} h${dir * 18} v2 h${dir * -18} Z`)
    }
  }

  for (const x of spec.cranes || []) {
    parts.extras.push(`M${x} ${deckY - 34} L${x + 26} ${deckY - 10} h2 L${x + 2} ${deckY - 32} Z`)
    parts.extras.push(`M${x - 2} ${deckY - 34} h6 v4 h-6 Z`)
  }

  if (spec.deckAircraft) {
    const n = spec.deckAircraft
    const spacing = spec.length / (n + 1)
    for (let i = 0; i < n; i += 1) {
      const x = stern + spacing * (i + 1)
      parts.extras.push(`M${x} ${deckY - 9} h14 v2 h-14 Z M${x + 5} ${deckY - 13} h4 v4 h-4 Z`)
    }
  }

  return parts
}

// 每艘船的"数字":照着识别图/线稿量出来的长度、干舷、烟囱与炮塔位置。
export const shipProfileSpecs: Record<string, ProfileSpec> = {
  约克城: {
    length: 560,
    freeboard: 30,
    boot: 14,
    stern: 40,
    flightDeck: { thickness: 6, overhang: 14 },
    island: { x: 330, w: 74, h: 26 },
    funnels: [],
    masts: [{ x: 300, h: 44, tripod: true }],
    turrets: [],
    cranes: [180, 520],
    deckAircraft: 6,
    deckhouse: [{ x: 250, w: 60, h: 10 }],
  },
}
