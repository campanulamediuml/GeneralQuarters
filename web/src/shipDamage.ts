import { label } from './display'
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


// 实际航速档位由服务端算好下发（进水降档、失去动力都在后端判定）：
// 前端只做"档位 → 文案"的映射，不按进水层数自己退算。
export function effectiveSpeed(unit: Unit): SpeedClass {
  return (unit.effective_speed as SpeedClass) || (unit.card.speed_class as SpeedClass) || 'normal'
}

export function shipDamageMarkers(unit: Unit): DamageMarkers {
  const damage: ShipDamage = unit.damage || {}
  const speed = effectiveSpeed(unit)
  const disabled = speed === 'disabled'
  return {
    fire: !!damage.fire,
    fireLabel: label('damage', damage.fire === 'burning' ? 'fire_burning' : 'fire_small'),
    deck: !!damage.deck_damaged || !!damage.deck_destroyed,
    deckDestroyed: !!damage.deck_destroyed,
    disabled,
    // 慢速标记：慢速/极慢都算"跑得慢"；失去动力单独用橙条，不重复画白条。
    slow: !disabled && (speed === 'slow' || speed === 'very_slow'),
    slowLabel: label('speed_class', speed),
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

// 卡片和敌军损伤清单共用同一套徽标：两处各写一份的结果就是图标上看得见、列表里却没有。
export type DamageBadgeKind = 'fire' | 'deny' | 'disabled' | 'slow'

export interface DamageBadge {
  label: string
  kind: DamageBadgeKind
}

export function shipDamageBadges(unit: Unit): DamageBadge[] {
  const markers = shipDamageMarkers(unit)
  const badges: DamageBadge[] = []
  if (markers.sunk) badges.push({ label: '沉没', kind: 'deny' })
  if (markers.fire) badges.push({ label: markers.fireLabel, kind: 'fire' })
  if (markers.deck) badges.push({ label: markers.deckDestroyed ? '飞行甲板摧毁' : '飞行甲板损伤', kind: 'deny' })
  if (markers.tilt) badges.push({ label: '进水倾斜', kind: 'deny' })
  if (markers.disabled) badges.push({ label: '失去动力', kind: 'disabled' })
  else if (markers.slow) badges.push({ label: markers.slowLabel, kind: 'slow' })
  if (markers.oilLeak) badges.push({ label: '漏油', kind: 'fire' })
  return badges
}

// 徽标只看最要紧的几项；逐条的损伤说明（雷达、声呐、鱼雷管、升降机）用这个。
export function shipDamageDetailLabels(unit: Unit): string[] {
  const damage: ShipDamage = unit.damage || {}
  const labels: string[] = []
  if (damage.fire === 'small') labels.push('小型火灾')
  if (damage.fire === 'burning') labels.push('起火燃烧')
  if (damage.flooding?.length) labels.push(`进水 ${damage.flooding.length} 层`)
  if (damage.disabled) labels.push('失去动力')
  if (damage.tilt) labels.push('倾斜')
  if (damage.oil_leak) labels.push('漏油')
  if (damage.deck_destroyed) labels.push('飞行甲板摧毁')
  else if (damage.deck_damaged) labels.push('飞行甲板损伤')
  if (damage.elevator_damage) labels.push(`升降机损坏 ${damage.elevator_damage} 部`)
  if (damage.torpedo_disabled) labels.push('鱼雷发射器毁坏')
  if (damage.fire_control_radar_disabled) labels.push('火控雷达损坏')
  if (damage.surface_radar_disabled) labels.push('对海搜索雷达损坏')
  if (damage.air_radar_disabled) labels.push('对空雷达损坏')
  if (damage.sonar_disabled) labels.push('声呐损坏')
  return labels
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
