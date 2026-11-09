import type { FleetGroup } from './fleet'

export interface MapCell {
  coord: string
  terrain: string
  weather?: 'cloud' | 'rain' | 'storm'
  name?: string
}

export interface GameMap {
  id: string
  version: number
  name: string
  rows: number
  cols: number
  cells: MapCell[]
}

export interface ShipCard {
  gunnery: number
  damage_control: number
  main_gun_count: number
  main_gun_caliber: number
  heavy_aa: number
  light_aa: number
  surface_radar: boolean
  air_radar: boolean
  speed_class: 'normal' | 'fast' | 'slow' | 'very_slow'
  torpedo_salvos: number
  aircraft: number
  hangar_capacity: number
  deck_capacity: number
  elevator_count: number
  depth_charges: number
  cargo: number
  armored_carrier: boolean
  historical_loss?: string
}

export interface Unit {
  id: string
  definition_id: string
  fleet_id?: string
  fleet_name?: string
  name: string
  type: string
  faction: 'A' | 'B'
  coord: string
  heading?: 'E' | 'SE' | 'SW' | 'W' | 'NW' | 'NE'
  moved?: boolean
  strength: number
  max_strength: number
  card: ShipCard
  legal_moves?: string[]
}

export type ReconHeading = 'E' | 'SE' | 'SW' | 'W' | 'NW' | 'NE'

export interface AirUnit {
  id: string
  definition_id: string
  carrier_id: string
  faction: 'A' | 'B'
  name: string
  model: string
  kind: 'fighter' | 'dive_bomber' | 'torpedo_bomber' | 'recon'
  status: 'hangar' | 'service' | 'deck' | 'ready' | 'cap' | 'recon'
  strength: number
  max_range: number
  remaining_range: number
  air_combat: number
  anti_ship: number
  anti_ground: number
  seaplane: boolean
  launched: boolean
  priority: number
  task_id?: string
  origin_coord?: string
  coord?: string
  heading?: ReconHeading
  sector_width?: 60 | 120
  recon_radius?: number
  recon_distance?: number
  recon_radius_limit?: number
  returning?: boolean
  tracking_fleet_id?: string
}

export interface DistanceHex {
  coord: string
  distance: number
}

export interface DistanceReference {
  origin: string
  max_distance: number
  cells: DistanceHex[]
}

export interface AirTask {
  id: string
  kind: 'cap' | 'recon' | 'asw' | 'attack'
  name: string
  carrier_id: string
  aircraft_ids: string[]
  aircraft: AirUnit[]
  priority: number
  origin_coord?: string
  current_coord?: string
  heading?: ReconHeading
  sector_width?: 60 | 120
  recon_radius?: number
  recon_distance?: number
  returning?: boolean
  tracking_fleet_id?: string
  sector_hexes?: string[]
  current_ring_hexes?: string[]
  distance_reference?: DistanceReference
}

export interface CarrierAirStatus {
  unit_id: string
  name: string
  type: string
  hangar_used: number
  hangar_capacity: number
  deck_capacity: number
  deck_used: number
  launch_used: number
  operation: '' | 'prepare' | 'launch' | 'recover'
  aircraft: AirUnit[]
}

export interface AviationView {
  carriers: CarrierAirStatus[]
  tasks: AirTask[]
}

export interface Player {
  id: string
  faction: 'A' | 'B'
  name: string
  is_host: boolean
  ready: boolean
  connected: boolean
}

export interface Order {
  type: 'SET_MOVE' | 'CLEAR_MOVE' | 'MERGE_FLEET' | 'REORGANIZE_HEX' | 'LAUNCH_CAP' | 'LAUNCH_RECON' | 'PREPARE_AIR' | 'RETURN_TO_HANGAR' | 'RECOVER_AIR' | 'PRIORITIZE_AIR_TASK'
  unit_id?: string
  target_unit_id?: string
  aircraft_id?: string
  aircraft_ids?: string[]
  coord?: string
  heading?: ReconHeading
  sector_width?: 60 | 120
  recon_radius?: number
  assignments?: Record<string, string>
}

export interface GameEvent {
  id: number
  turn: number
  sequence: number
  type: string
  text: string
  created_at: string
}

export interface GameView {
  room_id: string
  status: string
  turn: number
  version: number
  scenario: { id: string; name: string; description: string }
  map: GameMap
  units: Unit[]
  fleets: FleetGroup[]
  aviation: AviationView
  you: Player
  players: Player[]
  orders: Order[]
  events: GameEvent[]
  join_token?: string
  server_time: string
}

export interface RoomResponse {
  room_id: string
  player_id: string
  join_token?: string
}
