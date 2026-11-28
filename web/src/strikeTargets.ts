import type { AirTask, StrikeTarget } from './types'

export function airTaskTargetLabel(task: Pick<AirTask, 'target_coord' | 'aircraft'>, targets: StrikeTarget[]) {
  const coord = task.target_coord || ''
  if (!coord) return ''
  const candidates = targets.filter((target) => target.coord === coord)
  const source = task.aircraft.find((aircraft) => aircraft.target_coord === coord && aircraft.target_fleet_id)
    ?? task.aircraft.find((aircraft) => aircraft.target_coord === coord)
  const match = source?.target_fleet_id
    ? candidates.find((target) => target.fleet_id === source.target_fleet_id)
    : candidates.find((target) => target.airfield_id)
  return (match || candidates[0])?.label || coord
}
