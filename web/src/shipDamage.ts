import type { ShipDamage, Unit } from './types'

// 兵牌/卡片上的损伤标记。视觉语言照实体卡牌：
// 红圈横杠 = 甲板损伤或进水倾斜，橙色横条 = 失去动力，白色横条 = 慢速，烟焰 = 起火燃烧。
// 白条表示的是"现在跑得慢"，不是某种损伤名：天生慢速舰和被打慢的快舰都画它。
export type SpeedClass = 'fast' | 'normal' | 'slow' | 'very_slow' | 'disabled'

export interface DamageMarkers {
  fire: boolean
  fireLabel: string
  deck: boolean
  deckDestroyed: boolean
  disabled: boolean
  slow: boolean
  slowLabel: string
  tilt: boolean
  oilLeak: boolean
  sunk: boolean
}

const speedNames: Record<string, string> = { fast: '快速', normal: '常速', slow: '慢速', very_slow: '极慢', disabled: '失去动力' }

// effectiveSpeed 优先用服务端下发的实际档位（已算进水降档），缺失时按卡面 + 进水层数退算。
export function effectiveSpeed(unit: Unit): SpeedClass {
  if (unit.effective_speed) return unit.effective_speed
  const flooding = unit.damage?.flooding?.length || 0
  if (unit.damage?.disabled || flooding > unit.card.damage_control) return 'disabled'
  const order: SpeedClass[] = ['very_slow', 'slow', 'normal', 'fast']
  const base = order.indexOf((unit.card.speed_class as SpeedClass) || 'normal')
  if (base < 0) return 'normal'
  return order[Math.max(0, base - flooding)]
}

export function shipDamageMarkers(unit: Unit): DamageMarkers {
  const damage: ShipDamage = unit.damage || {}
  const speed = effectiveSpeed(unit)
  const disabled = speed === 'disabled'
  return {
    fire: !!damage.fire,
    fireLabel: damage.fire === 'burning' ? '起火燃烧' : '小型火灾',
    deck: !!damage.deck_damaged || !!damage.deck_destroyed,
    deckDestroyed: !!damage.deck_destroyed,
    disabled,
    // 慢速标记：慢速/极慢都算"跑得慢"；失去动力单独用橙条，不重复画白条。
    slow: !disabled && (speed === 'slow' || speed === 'very_slow'),
    slowLabel: speedNames[speed] || '慢速',
    tilt: !!damage.tilt,
    oilLeak: !!damage.oil_leak,
    sunk: unit.strength <= 0,
  }
}

// 兵牌上用的短标签，和实体卡牌一致：只标最要紧的那一两项。
export function damageTitles(unit: Unit): string[] {
  const markers = shipDamageMarkers(unit)
  const damage: ShipDamage = unit.damage || {}
  const flooded = damage.flooding?.length || 0
  const titles: string[] = []
  if (markers.fire) titles.push(markers.fireLabel)
  if (markers.deck) titles.push(markers.deckDestroyed ? '飞行甲板摧毁' : '飞行甲板损伤')
  if (markers.disabled) titles.push('失去动力')
  else if (markers.slow) {
    const native = unit.card.speed_class === 'slow' || unit.card.speed_class === 'very_slow'
    titles.push(native && flooded === 0
      ? `${markers.slowLabel}（本舰航速档位）`
      : `${markers.slowLabel}（进水 ${flooded} 层，降 ${native ? 0 : flooded} 档）`)
  }
  if (markers.tilt) titles.push(damage.flooding?.length ? `进水倾斜（进水 ${damage.flooding.length} 层）` : '舰体倾斜')
  if (markers.oilLeak) titles.push('漏油')
  // 兵牌上不画雷达图标，但悬停时要能看出来：两条雷达损坏互相独立。
  if (damage.fire_control_radar_disabled) titles.push('火控雷达损坏（炮击无 +10%）')
  if (damage.surface_radar_disabled) titles.push('对海搜索雷达损坏（无突袭加成）')
  if (damage.air_radar_disabled) titles.push('对空搜索雷达损坏')
  if (markers.sunk) titles.push('沉没')
  return titles
}

// 兵牌 CSS 类名：给 counter 加整体底色/边框变化。
export function counterDamageClass(unit: Unit): Record<string, boolean> {
  const markers = shipDamageMarkers(unit)
  return {
    'damaged-fire': markers.fire,
    'damaged-burning': unit.damage?.fire === 'burning',
    'damaged-deck': markers.deck,
    'damaged-deck-destroyed': markers.deckDestroyed,
    'damaged-disabled': markers.disabled,
    'damaged-slow': markers.slow,
    'damaged-tilt': markers.tilt,
    'damaged-sunk': markers.sunk,
  }
}
