import type { AirTask, AirUnit } from './types'

export interface AirFormationGroup {
  key: string
  aircraft: AirUnit
  count: number
  range: number
}

export function isAirFormation(task?: AirTask) {
  return task?.kind === 'attack' || task?.kind === 'transfer'
}

export function airFormationTitle(task: AirTask) {
  return task.kind === 'attack' ? '打击编队' : '转场编队'
}

export function groupAirFormationAircraft(task?: AirTask): AirFormationGroup[] {
  if (!task) return []
  const groups = new Map<string, AirFormationGroup>()
  for (const aircraft of task.aircraft || []) {
    const range = aircraft.remaining_range
    const key = `${aircraft.model}|${aircraft.loadout || ''}|${aircraft.skill || 'regular'}|${range}`
    const existing = groups.get(key)
    if (existing) {
      existing.count += 1
      continue
    }
    groups.set(key, { key, aircraft, count: 1, range })
  }
  return [...groups.values()].sort((left, right) => {
    if (left.aircraft.model !== right.aircraft.model) return left.aircraft.model.localeCompare(right.aircraft.model)
    return right.range - left.range
  })
}
