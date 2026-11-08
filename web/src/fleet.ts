import type { Order, Unit } from './types'

export interface FleetGroup extends Unit {
  fleet_id: string
  members: Unit[]
  count: number
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
    members.sort((a, b) => a.id.localeCompare(b.id))
    const leader = members[0]
    const fleet: FleetGroup = {
      ...leader,
      fleet_id: id,
      members,
      count: members.length,
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
