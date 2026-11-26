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
  /** 兵牌剪影：相对 content/ 的路径，由舰船卡下发（例如 misc/image/ship/new_orleans.png）。 */
  picture?: string
  /** 国别标识：目前用于澳大利亚舰船的土黄色兵牌；为空时按阵营上色。 */
  nation?: string
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

export interface AircraftElevator {
  id: string
  kind: 'elevator' | 'crane'
  capacity: number
  status: 'idle' | 'lifting' | 'lowering' | 'damaged'
  task_id?: string
  aircraft_ids: string[]
}

export interface FlightDeck {
  length: number
  operation: '' | 'launch' | 'recover'
  elevators: AircraftElevator[]
}

export interface ShipDamage {
  fire?: 'small' | 'burning'
  flooding?: number[]
  disabled?: boolean
  tilt?: boolean
  oil_leak?: boolean
  deck_damaged?: boolean
  deck_destroyed?: boolean
  elevator_damage?: number
  torpedo_disabled?: boolean
  surface_radar_disabled?: boolean
  fire_control_radar_disabled?: boolean
  air_radar_disabled?: boolean
  sonar_disabled?: boolean
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
  move_cooldown?: number
  confusion?: number
  contact_level?: 1 | 2 | 3 | 4
  strength: number
  max_strength: number
  card: ShipCard
  damage?: ShipDamage
  deck?: FlightDeck
  legal_moves?: string[]
  // 服务端算好的实际航速档位（进水降档之后），兵牌的白条按它画。
  effective_speed?: 'fast' | 'normal' | 'slow' | 'very_slow' | 'disabled'
  /** 自动航行目的地（规则书 280–283 行） */
  destination?: string
  /** 自动航行交战设置：aggressive / advantage / avoid（规则书 362–370 行） */
  engagement?: 'aggressive' | 'advantage' | 'avoid'
}

export type ReconHeading = 'E' | 'SE' | 'SW' | 'W' | 'NW' | 'NE'

export interface AirUnit {
  id: string
  definition_id: string
  carrier_id: string
  faction: 'A' | 'B'
  name: string
  model: string
  /** 算子图片：相对 content/ 的路径，由 units/*.json 的 aircraft[].picture 下发。 */
  picture?: string
  /** image（默认）原样显示彩图；mask 只取 alpha 当剪影。 */
  picture_mode?: 'image' | 'mask'
  kind: 'fighter' | 'dive_bomber' | 'torpedo_bomber' | 'recon' | 'level_bomber' | 'medium_bomber' | 'heavy_bomber' | 'strategic_bomber'
  status: 'hangar' | 'service' | 'elevator' | 'deck' | 'ready' | 'recovery' | 'cap' | 'recon' | 'attack_holding' | 'attack'
  strength: number
  max_range: number
  remaining_range: number
  loadout?: string
  tank_dropped?: boolean
  bombed?: boolean
  fuel?: { tank?: number; internal?: number; reserve?: number }
  air_combat: number
  anti_ship: number
  anti_ground: number
  seaplane: boolean
  /** 陆基机型：只能在机场起降，航程不按舰载机显示。 */
  airfield_only?: boolean
  /** 有鱼雷挂架；挂鱼雷出击时按鱼雷机结算（B-26）。 */
  torpedo_capable?: boolean
  /** 挂炸弹对舰时按高空轰炸结算：免疫轻防空，但命中率减半。 */
  high_altitude_bomber?: boolean
  launched: boolean
  elevator_id?: string
  priority: number
  task_id?: string
  task_kind?: 'cap' | 'recon' | 'attack' | 'transfer'
  origin_coord?: string
  coord?: string
  target_coord?: string
  /** SET_STRIKE_TARGET / LAUNCH_STRIKE：同格机场+舰队时显式指定打舰队 */
  target_fleet_id?: string
  heading?: ReconHeading
  sector_width?: 60 | 120
  recon_radius?: number
  recon_distance?: number
  recon_radius_limit?: number
  returning?: boolean
  tracking_fleet_id?: string
  confusion?: boolean
  damaged?: boolean
  maintenance?: number
  // CAP 巡逻空域与前出接敌点（掩护范围 = 该点及周围 1 hex，不在地图绘制范围）
  patrol_coord?: string
  forward_coord?: string
  /** 自动任务循环（规则书 188–191 行）：CAP、侦察、空中掩护通用 */
  on_cycle?: boolean
  cycle_kind?: 'cap' | 'cover' | 'recon'
  cycle_opt_out?: boolean
  cycle_target?: string
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

export interface ReconSector {
  aircraft_id: string
  status: AirUnit['status']
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

export interface AirTask {
  id: string
  kind: 'cap' | 'recon' | 'asw' | 'attack' | 'transfer' | 'cover'
  phase: 'servicing' | 'lifting' | 'preparing' | 'launching' | 'assembling' | 'active' | 'recovering'
  name: string
  carrier_id: string
  carrier_ids: string[]
  fleet_id?: string
  aircraft_ids: string[]
  aircraft: AirUnit[]
  airborne_count: number
  priority: number
  queue_position: number
  origin_coord?: string
  current_coord?: string
  target_coord?: string
  /** 攻击编队的攻击方式：true = 分散攻击（规则书 624–631 行） */
  disperse?: boolean
  /** 走完任务再落回母舰还要几回合（含排队着舰余量） */
  return_turns: number
  /** 天黑前能否回来：false 表示这波任务注定赶不上 19:40 的回收截止 */
  night_safe: boolean
  /** 距天黑还剩几回合；没有昼夜循环的剧本为 -1 */
  turns_to_dark: number
  /** 富余回合数 = turns_to_dark - return_turns，小于 0 就是这波回不来 */
  night_margin: number
  heading?: ReconHeading
  sector_width?: 60 | 120
  recon_radius?: number
  recon_distance?: number
  returning?: boolean
  tracking_fleet_id?: string
  /** 转场到达后目标舰队机库已满，降不下去：前端要打红色感叹号 */
  cannot_land?: boolean
  sector_hexes?: string[]
  current_ring_hexes?: string[]
  recon_sectors?: ReconSector[]
  distance_reference?: DistanceReference
}

export interface CarrierAirStatus {
  unit_id: string
  name: string
  type: string
  hangar_used: number
  hangar_capacity: number
  deck_length: number
  deck_capacity: number
  deck_used: number
  launch_used: number
  operation: '' | 'launch' | 'recover'
  /** 这艘航空舰当前为什么不能起降（倾斜、起火、甲板摧毁……），空串表示正常 */
  blocked_reason?: string
  /** 甲板被占满时，指出挡路的具体来源，例如还没设目标的攻击编队 */
  prevented_by?: string
  elevators: AircraftElevator[]
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
  type: 'SET_MOVE' | 'CLEAR_MOVE' | 'MERGE_FLEET' | 'REORGANIZE_HEX' | 'SET_HEADING' | 'SET_LOADOUT' | 'LAUNCH_CAP' | 'LAUNCH_RECON' | 'LAUNCH_STRIKE' | 'LAUNCH_COVER' | 'LAUNCH_TRANSFER' | 'PREPARE_AIR' | 'RETURN_TO_HANGAR' | 'RECOVER_AIR' | 'PRIORITIZE_AIR_TASK' | 'SET_STRIKE_TARGET' | 'SET_STRIKE_DISPERSION' | 'SET_AUTO_NAV' | 'SET_ENGAGEMENT' | 'SET_AIR_CYCLE' | 'RECALL_AIR' | 'RETASK_AIR' | 'RETARGET_AIR'
  unit_id?: string
  target_unit_id?: string
  aircraft_id?: string
  aircraft_ids?: string[]
  task_id?: string
  coord?: string
  target_coord?: string
  heading?: ReconHeading
  sector_width?: 60 | 120
  assignments?: Record<string, string>
  task_kind?: string
  disperse?: boolean
  engagement?: 'aggressive' | 'advantage' | 'avoid'
  /** SET_AIR_CYCLE：true 开启自动任务循环，false 取消 */
  cycle?: boolean
}

export interface GameEvent {
  id: number
  turn: number
  sequence: number
  type: string
  text: string
  created_at: string
}

export interface AirHitDetail {
  attacker: string
  model: string
  outcome: '撤退' | '损伤' | '摧毁' | '无效果'
}

export interface AirShipCard {
  id: string
  name: string
  type: string
  strength: number
  /** 舰船卡素材路径（misc/image/ship/*.png），空战动画按它画剪影 */
  picture?: string
}

export interface StrikeAllocation {
  attacker: string
  model: string
  ship_id: string
  ship_name: string
  hits: number
}

export interface AirCombatReport {
  id: string
  turn?: number
  coord: string
  // airfield = 对地攻击；ship = 单舰目标；group = 舰队目标；缺省按对舰处理
  target?: 'ship' | 'group' | 'airfield' | 'intercept'
  target_name?: string
  attacker_faction: Player['faction']
  defender_faction: Player['faction']
  attacker_models: string[]
  defender_models: string[]
  /** 与 attacker_models / defender_models 一一对应的算子图片（相对 content/ 的路径）。 */
  attacker_pictures?: string[]
  defender_pictures?: string[]
  attacker_count: number
  defender_count: number
  attacker_losses: number
  defender_losses: number
  aa_hits: number
  strike_hits: number
  ships_hit?: number
  ship_hits?: number
  bombers?: AirHitDetail[]
  bomb_attempts?: number
  ships?: AirShipCard[]
  target_allocations?: StrikeAllocation[]
  airfield_hits?: number
  ground_losses?: number
  airfield?: AirShipCard
  airfield_before?: number
  airfield_after?: number
  airfield_runways_before?: number
  airfield_runways_after?: number
  attacker_hits?: AirHitDetail[]
  winner: Player['faction'] | 'draw'
  summary: string
}

export interface AirfieldContact {
  id: string
  name: string
  faction: Player['faction']
  coord: string
  /** 机场规模：small / medium / large，水上飞机基地是 seaplane */
  type: 'small' | 'medium' | 'large' | 'seaplane'
  /** 机场标识图片（小/中/大三条跑道符号），相对 content/ 的路径 */
  picture?: string
  /** 当前可用跑道 / 总跑道 */
  runways: number
  max_runways?: number
  strength?: number
  max_strength?: number
  radar?: boolean
  heavy_aa?: number
  light_aa?: number
  coastal?: number
  hangar_used?: number
  /** 停机上限：跑道数×6，剧本可覆盖（转场进入已满机场会被拒绝） */
  hangar_capacity?: number
  /** 敌方卸在这个机场上的滩头物资；有货时机场瘫痪 */
  enemy_supplies?: number
  contact_level: number
}

export interface GroundAirUnit {
  id: string
  definition_id: string
  airfield_id: string
  faction: Player['faction']
  model: string
  /** 算子图片：与 AirUnit.picture 同源，机场算子转场时保持同一张图。 */
  picture?: string
  picture_mode?: 'image' | 'mask'
  kind: string
  status: string
  strength: number
  max_range: number
  remaining_range: number
  loadout?: string
  tank_dropped?: boolean
  bombed?: boolean
  fuel?: { tank?: number; internal?: number; reserve?: number }
  air_combat: number
  anti_ship: number
  anti_ground: number
  /** 陆基机型标记 */
  airfield_only?: boolean
  /** 有鱼雷挂架；挂鱼雷出击时按鱼雷机结算（B-26）。 */
  torpedo_capable?: boolean
  /** 挂炸弹对舰时按高空轰炸结算。 */
  high_altitude_bomber?: boolean
  task_id?: string
  task_kind?: string
  priority?: number
  coord?: string
  origin_coord?: string
  target_coord?: string
  heading?: string
  sector_width?: number
  recon_radius?: number
  recon_distance?: number
  recon_radius_limit?: number
  returning?: boolean
  damaged?: boolean
  maintenance?: number
  /** 自动任务循环（规则书 188–191 行）：机场算子与舰载机同一套开关 */
  on_cycle?: boolean
  cycle_kind?: 'cap' | 'cover' | 'recon'
  cycle_target?: string
  cycle_opt_out?: boolean
}

export interface GroundAirStatus {
  unit_id: string
  faction: Player['faction']
  name: string
  type: 'small' | 'medium' | 'large' | 'seaplane'
  /** 机场标识图片（小/中/大三条跑道符号），相对 content/ 的路径 */
  picture?: string
  coord: string
  strength: number
  max_strength: number
  max_runways: number
  runways: number
  radar: boolean
  heavy_aa: number
  light_aa: number
  coastal: number
  hangar_used: number
  /** 停机上限（航空算子数）：跑道数×6，剧本可覆盖；转场进入已满机场会被拒绝 */
  hangar_capacity: number
  enemy_supplies?: number
  aircraft: GroundAirUnit[]
}

/** 剧本时间线快照：第几天几点、天色、以及整备/放飞窗口是否开放 */
export interface GameClock {
  day: number
  time: string
  night: boolean
  service_open: boolean
  launch_open: boolean
  service_start: string
  launch_start: string
  recover_by: string
  turn_minutes: number
  /** 距回收截止还有几个回合（-1 = 没配时间线） */
  night_in_turns: number
}

export interface GameView {
  room_id: string
  status: string
  turn: number
  /** 单人房：一名玩家 ready 就结算 */
  solo?: boolean
  scenario: { id: string; name: string; description: string }
  map: GameMap
  units: Unit[]
  fleets: FleetGroup[]
  aviation: AviationView
  airfields?: AirfieldContact[]
  ground_air?: GroundAirStatus[]
  ground_tasks?: AirTask[]
  ground_aircraft?: GroundAirUnit[]
  fleet_stays?: Record<string, string>
  fleet_sessions?: Record<string, number>
  shore_supplies?: Record<string, number>
  /** 敌方卸在己方机场上的滩头物资：坐标 → 数量 */
  enemy_shore_supplies?: Record<string, number>
  /** 逐格天气：行优先、每格一个字符 '0'..'3'（晴/多云/降雨/风暴） */
  weather_cells?: string
  wind?: {
    wind: string
    /** 逐格风向：行优先、每格一个字符，'0'..'5' 对应 NE/E/SE/SW/W/NW */
    cells?: string
    fleets: { fleet_id: string; heading: string; relation: string; direction?: string; has_carrier: boolean; deck_modifier: number; changed: boolean }[]
  }
  radio_used?: string[]
  /** 剧本时间线快照；没配 DayHours/TurnsPerDay 的剧本为 null */
  clock?: GameClock | null
  radio_tasks?: { task_id: string; kind: string; coord: string; airborne: number; used: boolean; chance: number; distance: number }[]
  air_combats: AirCombatReport[]
  you: Player
  players: Player[]
  orders: Order[]
  events: GameEvent[]
  /** 昼夜状态（开启昼夜循环时随回合变化） */
  night?: boolean
  /** 已发现的敌方飞行编队（雷达/侦察/目视） */
  enemy_air?: EnemyAirContact[]
  /** 战果台账：按阵营（A/B）分组的沉没舰船与飞机损失 */
  losses?: Record<string, FactionLosses>
  /** 战果分：击沉/击落折算值 + 已完成的投送目标，剧本声明 kind:"score" 目标时用于判胜负 */
  scores?: Record<string, number>
  /** 对局结果：存在即表示已经结束。level 是 VP 比值给出的等级。 */
  outcome?: { winner?: string; level?: 'draw' | 'tactical' | 'strategic'; reason: string; scores: Record<string, number>; turn: number; turn_cap?: boolean }
  objectives?: { id: string; text?: string; kind: string; coord?: string; faction?: string; value?: number; points?: number; decisive?: boolean }[]
  join_token?: string
}

export interface ShipLoss {
  turn: number
  id?: string
  name: string
  type: string
  fleet?: string
  card?: ShipCard
}

export interface AircraftLoss {
  model: string
  kind: string
  count: number
}

export interface FactionLosses {
  ships?: ShipLoss[]
  aircraft?: AircraftLoss[]
  ship_total?: number
  aircraft_total?: number
}

export interface EnemyAirContact {
  task_id: string
  faction: 'A' | 'B'
  kind: string
  models: string[]
  count: number
  coord: string
  heading?: string
  target_coord?: string
  detected_by: 'radar' | 'recon' | 'visual'
  turn: number
  returning?: boolean
  attack_holding?: boolean
}

export interface RoomResponse {
  room_id: string
  player_id: string
  join_token?: string
}

export interface StrikeTarget {
  coord: string
  label: string
  count: number
  contact_level?: 1 | 2 | 3 | 4
}
