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

export interface Unit {
  id: string
  fleet_id?: string
  name: string
  type: string
  faction: 'A' | 'B'
  coord: string
  heading?: 'E' | 'SE' | 'SW' | 'W' | 'NW' | 'NE'
  moved?: boolean
  strength: number
  max_strength: number
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
