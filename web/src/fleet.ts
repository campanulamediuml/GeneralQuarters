import type { DistanceReference, Order, Unit } from './types'

export interface FleetGroup extends Unit {
  fleet_id: string
  members: Unit[]
  count: number
  move_interval: number
  move_cooldown?: number
  confusion?: number
  contact_level?: 1 | 2 | 3 | 4
  contact_size?: 'small' | 'medium' | 'large'
  distance_reference?: DistanceReference
}

export function fleetID(unit: Unit) {
  return unit.fleet_id || unit.id
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
