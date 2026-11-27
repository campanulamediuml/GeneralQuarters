import type { DistanceReference, Order, Unit } from './types'

export interface FleetGroup extends Unit {
  fleet_id: string
  members: Unit[]
  count: number
  move_interval: number
  move_knots?: number
  move_cycle?: number[]
  move_cooldown?: number
  confusion?: number
  contact_level?: 1 | 2 | 3 | 4
  contact_size?: 'small' | 'medium' | 'large'
  /** 情报 3 级起能看出的主力舰类别：carrier / battleship / heavy_cruiser。 */
  contact_capitals?: string[]
  distance_reference?: DistanceReference
  /** 自动航行目的地与交战设置（规则书 280–283、362–370 行） */
  destination?: string
  engagement?: 'aggressive' | 'advantage' | 'avoid'
}

export function fleetID(unit: Unit) {
  return unit.fleet_id || unit.id
}

const CONTACT_SIZE_LABEL: Record<string, string> = { small: '小型舰队', medium: '中型舰队', large: '大型舰队' }
const CAPITAL_LABEL: Record<string, string> = { carrier: '航母', battleship: '战列舰', heavy_cruiser: '重巡' }

// 敌军标签按情报等级说人话：
// 1 级只知道"这里有敌人"；2 级知道规模；3 级知道有没有航母/战列舰/重巡；4 级才是完整编成。
export function contactSizeLabel(size?: string) {
  return size ? CONTACT_SIZE_LABEL[size] || '' : ''
}

export function contactCapitalsLabel(capitals?: string[]) {
  return (capitals || []).map((key) => CAPITAL_LABEL[key] || key).join('/')
}

/** 地图标签 / 敌军面板标题共用的一行情报摘要。 */
export function contactIntelLabel(fleet: { contact_level?: number; contact_size?: string; contact_capitals?: string[]; name?: string }) {
  const level = fleet.contact_level || 0
  if (level <= 0) return ''
  if (level === 1) return '有敌舰'
  const size = contactSizeLabel(fleet.contact_size)
  if (level === 2) return size
  if (level >= 4) return fleet.name || ''
  const capitals = contactCapitalsLabel(fleet.contact_capitals)
  return capitals ? `${size} · ${capitals}` : size
}

export function fleetOrder(fleet: FleetGroup, orders: Order[]) {
  const ids = new Set(fleet.members.map((unit) => unit.id))
  return orders.find((order) => order.unit_id && ids.has(order.unit_id) && order.type === 'SET_MOVE')
}

export function groupFleets(units: Unit[], orders: Order[]) {
  const groups = new Map<string, Unit[]>()
  for (const unit of units) {
    const id = fleetID(unit)
    groups.set(id, [...(groups.get(id) || []), unit])
  }
  const fleets: FleetGroup[] = []
  for (const [id, members] of groups) {
    const typeOrder = ['CV', 'CVL', 'CVE', 'BB', 'BC', 'CA', 'CL', 'DD', 'TRS']
    members.sort((a, b) => {
      const typeRank = typeOrder.indexOf(a.type) - typeOrder.indexOf(b.type)
      return typeRank || a.id.localeCompare(b.id)
    })
    const leader = members[0]
    const fleet: FleetGroup = {
      ...leader,
      fleet_id: id,
      members,
      count: members.length,
      move_interval: 3,
      name: members.length > 1 ? `${leader.name}编队` : leader.name,
      strength: members.reduce((sum, unit) => sum + unit.strength, 0),
      max_strength: members.reduce((sum, unit) => sum + unit.max_strength, 0),
      moved: members.some((unit) => unit.moved),
    }
    fleet.coord = fleetOrder(fleet, orders)?.coord || leader.coord
    fleets.push(fleet)
  }
  return fleets.sort((a, b) => a.fleet_id.localeCompare(b.fleet_id))
}
