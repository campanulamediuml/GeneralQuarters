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
  name: string
  type: string
  faction: 'A' | 'B'
  coord: string
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
  type: 'SET_MOVE' | 'CLEAR_MOVE'
  unit_id: string
  coord?: string
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
