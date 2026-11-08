import type { FleetGroup } from './fleet'

export interface MapCell {
  coord: string
  terrain: string
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

export interface Player {
  id: string
  faction: 'A' | 'B'
  name: string
  is_host: boolean
  ready: boolean
  connected: boolean
}

export interface Order {
  type: 'SET_MOVE' | 'CLEAR_MOVE' | 'MERGE_FLEET' | 'REORGANIZE_HEX'
  unit_id: string
  target_unit_id?: string
  coord?: string
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
