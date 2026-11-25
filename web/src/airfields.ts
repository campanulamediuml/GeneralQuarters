import { contentImageURL } from './silhouette-masks'
import type { AirfieldContact, GroundAirStatus } from './types'

// 机场规模就是 small / medium / large 三档，外加一类水上飞机基地：
//   小型 = 只能起降战斗机；中型 = 除战略轰炸机外全部；大型 = 全部；水机基地只操作水上飞机。
// 三档各有一张跑道符号素材（content/misc/image/airfield/{small,medium,large}.png）：
// 小型一条跑道、中型两条交叉、大型三条；水机基地是一枚圆形徽章。
// 服务端按机场 type 给出 picture，剧本也能用 airfields[].picture 单点覆盖。
// 旧存档里的 field / heavy / major 由服务端归一成新名字，这里再兜一层。
export function normalizeAirfieldType(type?: string): string {
  const value = (type || '').trim().toLowerCase()
  if (value === 'field' || value === 'small') return 'small'
  if (value === 'heavy' || value === 'medium') return 'medium'
  if (value === 'major' || value === 'large') return 'large'
  if (value === 'seaplane') return 'seaplane'
  return value
}

export function airfieldTypeLabel(type?: string): string {
  switch (normalizeAirfieldType(type)) {
    case 'seaplane': return '水上飞机基地'
    case 'small': return '小型机场'
    case 'medium': return '中型机场'
    default: return '大型机场'
  }
}

// 悬停提示写清楚规模背后的规则含义，免得"小型/中型"变成纯美术标签。
export function airfieldTypeHint(type?: string): string {
  switch (normalizeAirfieldType(type)) {
    case 'seaplane': return '水上飞机基地：只能操作水上飞机'
    case 'small': return '小型机场：只能起降战斗机'
    case 'medium': return '中型机场：除战略轰炸机外全部可起降'
    default: return '大型机场：所有机型都能起降'
  }
}

export function airfieldPictureURL(picture?: string): string | undefined {
  return contentImageURL(picture)
}

// 地图上的机场标签只写地名：'拉包尔机场' → '拉包尔'、'图拉吉水上飞机基地' → '图拉吉'。
// 规模由图标本身表达（一条/两条/三条跑道或水机徽章），跑道数和情报等级放进悬停提示。
export function airfieldShortName(name?: string): string {
  const text = (name || '').trim()
  if (!text) return ''
  return text.replace(/(水上飞机基地|水机基地|机场)$/, '').trim() || text
}

// 地图上的机场卡片和敌我双方的机场面板共用同一个投影：情报里的名字/类型/图片 +
// 实时的生命值、可用跑道、防空和机库占用。
export function airfieldContactStatus(airfield: AirfieldContact): GroundAirStatus {
  return {
    unit_id: airfield.id,
    faction: airfield.faction,
    name: airfield.name,
    type: airfield.type,
    picture: airfield.picture,
    coord: airfield.coord,
    strength: airfield.strength ?? 0,
    max_strength: airfield.max_strength ?? 0,
    max_runways: airfield.max_runways ?? airfield.runways,
    runways: airfield.runways,
    radar: Boolean(airfield.radar),
    heavy_aa: airfield.heavy_aa ?? 0,
    light_aa: airfield.light_aa ?? 0,
    coastal: airfield.coastal ?? 0,
    hangar_used: airfield.hangar_used ?? 0,
    aircraft: [],
  }
}
