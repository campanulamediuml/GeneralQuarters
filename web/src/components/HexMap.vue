<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import type { AirTask, DistanceReference, EnemyAirContact, GameMap, Order, ReconSector, Submarine, SubmarineBattle, Unit } from '../types'
import { contactCapitalsLabel, contactSizeLabel, fleetOrder, type FleetGroup } from '../fleet'
import { aircraftPictureURL, shipMaskURL } from '../silhouette-masks'
import { airfieldContactStatus } from '../airfields'
import { label } from '../display'
import AircraftCounter from './AircraftCounter.vue'
import AirfieldCounter from './AirfieldCounter.vue'

const props = defineProps<{
  map: GameMap
  fleets: FleetGroup[]
  playerFaction: 'A' | 'B'
  selectedUnit?: Unit
  orders: Order[]
  reconTasks: AirTask[]
  airTasks: AirTask[]
  distanceReference?: DistanceReference
  /** 距离原点（hex）：给了就按整张图现算，覆盖服务端那份 8 格参考。 */
  distanceOrigin?: string
  airfields?: import('../types').AirfieldContact[]
  groundAircraft?: import('../types').GroundAirUnit[]
  groundTasks?: AirTask[]
  /** 已发现的敌方飞行编队（雷达/侦察/目视） */
  enemyAir?: import('../types').EnemyAirContact[]
  /** 地图上点开的那支编队（打击/转场），用于高亮编队牌 */
  selectedAirTaskId?: string
  /** 本回合已就绪：只让看，不允许任何会改动状态的操作 */
  locked?: boolean
  /** 正在选打击目标：点敌方机场要设目标，不能弹情报卡 */
  strikeTargeting?: boolean
  /** 正在选自动航行目的地：点敌方机场要设目的地 */
  navTargeting?: boolean
  /** 逐格风向：行优先、每格一个字符，'0'..'5' 对应 NE/E/SE/SW/W/NW；空串=单一风向 */
  windCells?: string
  /** 逐格天气：行优先、每格一个字符 '0'..'3'（晴/多云/降雨/风暴）；空串=无天气层 */
  weatherCells?: string
  /** 观战模式：双方都按"完整可读"画，不套用情报等级/敌方样式，也不画己方航行目的地 */
  spectate?: boolean
  /** 玩家视图只给可见潜艇；观战视图给双方全部潜艇并按阵营着色。 */
  submarines?: Submarine[]
  /** 正在等玩家点海图选位置的那艘潜艇 */
  selectedSubmarineId?: string
  /** 当前回合的遭遇战与持续反潜共同驱动舰队红色状态条。 */
  subBattles?: SubmarineBattle[]
  turn?: number
}>()

const emit = defineEmits<{
  selectUnit: [unit: Unit]
  selectHex: [coord: string]
  manageHex: [coord: string]
  move: [coord: string]
  /** 拖动失败时的提示（冷却中 / 不是相邻格），交给上层显示 */
  notice: [text: string]
  selectAirfield: [airfieldID: string]
  /** 点敌方机场算子：打开只读情报卡（不改变任何状态） */
  inspectAirfield: [airfieldID: string]
  /** 点地图上的编队牌：打开编队详情（机型 / 状态 / 目标 / 改目标 / 召回） */
  selectAirTask: [taskID: string]
  /** 点地图上的潜艇算子：选中它，等下一次点海格摆位 */
  selectSubmarine: [submarineID: string]
}>()

const size = 43
// 缩放记在 localStorage：地图是 v-if 挂载的，切到航空界面再回来组件会重建，
// 之前缩放跟着丢，每次回来都跳回 100%。
const ZOOM_STORAGE_KEY = 'gq_map_zoom'
// 缩放档位而不是固定步长：50%–500% 用 20% 步长要点二十下才能拉到头，
// 所以做成档位，加减各跳一档，越往后步子越大。
const ZOOM_STEPS = [0.5, 0.6, 0.75, 1, 1.2, 1.5, 2, 2.5, 3, 4, 5]
function nearestZoomStep(value: number) {
  return ZOOM_STEPS.reduce((best, step) => (Math.abs(step - value) < Math.abs(best - value) ? step : best), ZOOM_STEPS[0])
}
function storedZoom() {
  const saved = Number(localStorage.getItem(ZOOM_STORAGE_KEY))
  return Number.isFinite(saved) && saved >= ZOOM_STEPS[0] && saved <= ZOOM_STEPS[ZOOM_STEPS.length - 1] ? nearestZoomStep(saved) : 1
}
const zoom = ref(storedZoom())
watch(zoom, (value) => {
  try {
    localStorage.setItem(ZOOM_STORAGE_KEY, String(value))
  } catch {
    // 隐私模式下写不了 localStorage，忽略即可，不影响本局缩放
  }
})
const mapElement = ref<SVGSVGElement>()
const ghostElement = ref<SVGGElement>()
const draggedUnit = ref<FleetGroup>()
const dragging = ref(false)
const dropHover = ref('')
let pointerStart = { x: 0, y: 0 }
let dragMatrix: DOMMatrix | null = null
let dragFrame = 0
let pendingPoint = { x: 0, y: 0 }
let longPressTimer: ReturnType<typeof setTimeout> | undefined
let longPressTriggered = false

// 地形格子列表。两处以前都很贵：
// 1) 每个格子都用 cells.find() 线性扫一遍样板表（52×64 × 438 ≈ 146 万次比较）；
// 2) 状态每次推送都会换掉 props.map 这个对象，于是 3328 个格子对象全重建一遍，
//    上层还带着它们的身份变化一路重渲。现在按内容做缓存，地图没变就复用同一份数组。
let hexGeneration = 0
let hexCacheKey = ''
let hexCache: { coord: string; x: number; y: number; terrain: string; name: string }[] = []
const hexes = computed(() => {
  const cells = props.map.cells || []
  let key = props.map.rows + 'x' + props.map.cols + '|' + cells.length
  for (const cell of cells) key += '|' + cell.coord + cell.terrain + (cell.name || '')
  if (key === hexCacheKey) return hexCache
  const index = new Map<string, { terrain?: string; name?: string }>()
  for (const cell of cells) index.set(cell.coord, cell)
  const result: typeof hexCache = []
  for (let row = 1; row <= props.map.rows; row++) {
    for (let col = 1; col <= props.map.cols; col++) {
      const coord = `${String(row).padStart(2, '0')}${String(col).padStart(2, '0')}`
      const special = index.get(coord)
      result.push({ coord, ...center(coord), terrain: special?.terrain || 'water', name: special?.name || '' })
    }
  }
  hexCacheKey = key
  hexCache = result
  hexGeneration += 1
  return result
})

const MAP_MARGIN = 14
const fullView = computed(() => {
  const width = Math.sqrt(3) * size * (props.map.cols + 1)
  const height = size * (1.5 * props.map.rows + 0.7)
  return { x: -MAP_MARGIN, y: -MAP_MARGIN, w: width + MAP_MARGIN * 2, h: height + MAP_MARGIN * 2 }
})
type Camera = { x: number; y: number; w: number; h: number }
function viewForZoom(level: number): Camera {
  const map = fullView.value
  const w = map.w / level
  const h = map.h / level
  return { x: map.x + (map.w - w) / 2, y: map.y + (map.h - h) / 2, w, h }
}
// 地图用 viewBox 当摄像机：缩放只改视野大小，拖动只改视野原点。
// 这样不再靠改 SVG 的 width 撑出滚动条，观战页和游戏页的手感可以对齐。
const camera = ref<Camera>(viewForZoom(zoom.value))
const viewBox = computed(() => `${camera.value.x} ${camera.value.y} ${camera.value.w} ${camera.value.h}`)
const selectedMoves = computed(() => new Set(props.selectedUnit?.legal_moves || []))
// 已发现的敌方飞行编队按当前坐标分组。
function enemyAirAt(coord: string) {
  return enemyAirByCoord.value.get(coord) || []
}
const reconSectors = computed<ReconSector[]>(() => props.reconTasks.flatMap((task) => task.recon_sectors || []))
const reconCells = computed(() => new Set(reconSectors.value.flatMap((sector) => sector.sector_hexes || [])))
const reconCurrentCells = computed(() => new Set(reconSectors.value.flatMap((sector) => sector.current_ring_hexes || [])))
// 观战时双方扇面同时在场：按阵营上色，美军蓝、日军红，两边叠在同一格用紫色。
// 玩家视图只下发己方侦察，继续走原来的金色蒙版。
const reconFactionTints = computed(() => {
  const sector = new Map<string, string>()
  const current = new Map<string, string>()
  const mark = (target: Map<string, string>, coord: string, faction: string) => {
    const existing = target.get(coord)
    target.set(coord, existing && existing !== faction ? 'both' : faction)
  }
  for (const task of props.reconTasks) {
    const faction = (task.faction || props.playerFaction).toLowerCase()
    for (const coord of task.sector_hexes || []) mark(sector, coord, faction)
    for (const coord of task.current_ring_hexes || []) mark(current, coord, faction)
  }
  return { sector, current }
})
function reconSectorClass(coord: string) {
  if (!props.spectate) return reconCells.value.has(coord) ? 'recon-sector' : ''
  const faction = reconFactionTints.value.sector.get(coord)
  return faction ? 'recon-sector-' + faction : ''
}
function reconCurrentClass(coord: string) {
  if (!props.spectate) return reconCurrentCells.value.has(coord) ? 'recon-current' : ''
  const faction = reconFactionTints.value.current.get(coord)
  return faction ? 'recon-current-' + faction : ''
}
const flightTasks = computed(() => props.airTasks.filter((task) => (task.kind === 'attack' || task.kind === 'transfer') && task.airborne_count > 0 && task.current_coord))

function reconAircraftAt(coord: string) {
  return reconAirByCoord.value.get(coord) || []
}

// CAP / 掩护：巡逻点/前出点及周围 1 hex（共 7 格），但巡逻范围不绘制。
// 同样一格只画一面牌：以前一个算子一张牌，一队 10 架 CAP 就横着一排 10 个牌子，
// 把海图盖住一大片。现在按"阵营 + 格子"聚合，牌上写"CAP ×10"或"掩护 ×4"，
// 具体机型和状态放在 title 里；观战时双方各一面，不会把两支舰队的 CAP 加成一个数。
type CapBadge = {
  key: string
  coord: string
  faction: string
  total: number
  returning: number
  forward: number
  cap: number
  cover: number
  models: string[]
}
const capBadges = computed(() => {
  const badges = new Map<string, CapBadge>()
  for (const task of props.airTasks) {
    if (task.kind !== 'cap' && task.kind !== 'cover') continue
    const faction = (task.faction || props.playerFaction).toLowerCase()
    for (const aircraft of task.aircraft) {
      if (!['cap', 'cover', 'attack', 'recon'].includes(aircraft.status)) continue
      const coord = aircraft.forward_coord || aircraft.coord || ''
      if (!coord) continue
      const key = faction + ':' + coord
      const badge = badges.get(key) || { key, coord, faction, total: 0, returning: 0, forward: 0, cap: 0, cover: 0, models: [] }
      badge.total += 1
      if (aircraft.returning) badge.returning += 1
      if (aircraft.forward_coord) badge.forward += 1
      if (task.kind === 'cover') badge.cover += 1
      else badge.cap += 1
      if (!badge.models.includes(aircraft.model)) badge.models.push(aircraft.model)
      badges.set(key, badge)
    }
  }
  return badges
})
// CAP/COV 牌固定 1 倍：牌面 48×18，本身就在一个 hex 宽度以内。
// 以前按缩放补到 2.5 倍，一块牌比两个 hex 还宽，压到隔壁格上分不清是谁的巡逻。
const capBadgeScale = 1
const capBadgeList = computed(() => {
  const list = [...capBadges.value.values()].sort((left, right) => (left.coord === right.coord ? left.faction.localeCompare(right.faction) : left.coord.localeCompare(right.coord)))
  return list.map((badge) => {
    const same = list.filter((item) => item.coord === badge.coord)
    const point = center(badge.coord)
    const slot = same.findIndex((item) => item.key === badge.key)
    // 42×16 的牌子放在 -31..-15 这一段：六边形在这个高度还有 42 宽，整块牌刚好装进去，
    // 又只压住兵牌顶上 3px。再往上牌子就骑到三个 hex 的交界上，看不出是谁的巡逻。
    return { ...badge, x: point.x + (slot - (same.length - 1) / 2) * 46, y: point.y - 23 }
  })
})
// 牌面只要两件事：是 CAP 还是掩护(COV)、几个算子。前出/返航这些细节放悬停提示，
// 不再往牌上塞矢量飞机图——48px 宽的牌里放机型图，字和图都看不清。
function capBadgeLabel(badge: CapBadge) {
  return (badge.cover > badge.cap ? 'COV' : 'CAP') + ' ×' + badge.total
}
function capBadgeTitle(badge: CapBadge) {
  const detail = [badge.forward ? '前出拦截 ' + badge.forward : '', badge.returning ? '返航 ' + badge.returning : ''].filter(Boolean).join(' · ')
  const side = props.spectate ? (badge.faction === 'b' ? '日军' : '美军') + ' · ' : ''
  const kind = badge.cover > badge.cap ? '掩护' : 'CAP'
  return side + badge.models.join(' / ') + ' · ' + kind + ' ' + badge.total + ' 个算子' + (detail ? '（' + detail + '）' : '') + ' · ' + badge.coord
}

function reconCurrentAircraftAt(coord: string) {
  return reconCurrentAirByCoord.value.get(coord) || ''
}

function reconPosition(coord: string, index: number, total: number) {
  const point = center(coord)
  return { x: point.x + (index - (total - 1) / 2) * 14, y: point.y - 30 - index * 7 }
}

function flightTasksAt(coord: string) {
  return flightTaskByCoord.value.get(coord) || []
}

// 同一格里可能同时有"还在执行任务"和"已经掉头返航"两支编队（拆组，之后还会继续拆），
// 一次超远程打击能连着拆出好几张牌。以前按 30px 横向偏移排，
// 两张 48px 宽的牌几乎完全重叠，点选会串到别的编队上。
// 现在改成竖排阶梯：牌面高 30，行距 34 正好不重叠，点击各归各的。
// 编队牌居中画在 hex 里：飞机在舰队上面，牌就压在兵牌上，格子在哪个距离圈一目了然。
// 同格多支编队（拆组后的攻击/返航）按舰队兵牌的斜叠法错开：左下 → 右上。
function flightPosition(coord: string, index: number, total: number) {
  const point = center(coord)
  if (total <= 1) return { x: point.x, y: point.y }
  const step = index - (total - 1) / 2
  return { x: point.x + step * 26, y: point.y - step * 22 }
}

// 编队牌写任务状态而不是型号：拆组之后同一格会并存"攻击"和"返航"两张牌，
// 玩家得一眼看出哪张还在打、哪张已经往家飞。
function flightLabel(task: AirTask) {
  const base = task.kind === 'attack' ? '打击' : task.kind === 'cover' ? '掩护' : '转场'
  return task.returning ? '返航·' + base : base
}

function flightRange(task: AirTask) {
  const ranges = task.aircraft.filter((aircraft) => ['attack', 'attack_holding'].includes(aircraft.status)).map((aircraft) => aircraft.remaining_range)
  return ranges.length ? Math.min(...ranges) : task.aircraft[0]?.remaining_range || 0
}

function flightModels(task: AirTask) {
  return [...new Set(task.aircraft.map((aircraft) => aircraft.model))].join(' / ')
}

// 编队牌上的机型图直接用编队里第一架飞机的素材（服务端下发的 picture），
// 前端不再按阵营挑一张硬编码的"代表机型"图。
function taskPicture(task: AirTask) {
  return aircraftPictureURL(task.aircraft?.[0]?.picture)
}
function taskPictureFile(task: AirTask) {
  return task.aircraft?.[0]?.picture || ''
}
function enemyAirPicture(contact: EnemyAirContact) {
  return aircraftPictureURL(contact.pictures?.[0])
}

// 距离一律按 hex 数算，而且整张图都算：服务端下发的 distance_reference 只覆盖
// 8 格（为了控制快照体积），选中舰队要量到地图另一头就量不了。
// 这里在客户端按同一个 offset→cube 公式现算，结果和引擎的 Distance() 完全一致。
function hexDistance(a: string, b: string) {
  if (a.length !== 4 || b.length !== 4) return -1
  const cube = (coord: string) => {
    const row = Number(coord.slice(0, 2)) - 1
    const col = Number(coord.slice(2, 4)) - 1
    const x = col - Math.floor((row + (row & 1)) / 2)
    const z = row
    return [x, -x - z, z] as const
  }
  const [ax, ay, az] = cube(a)
  const [bx, by, bz] = cube(b)
  return Math.max(Math.abs(ax - bx), Math.abs(ay - by), Math.abs(az - bz))
}

// 原点优先用上层给的选择（舰队列、侦察扇面、空中编队都能给），没有就退回服务端那份 8 格参考。
const distanceOrigin = computed(() => props.distanceOrigin || props.distanceReference?.origin || '')
const distanceByCoord = computed(() => {
  const origin = distanceOrigin.value
  const result = new Map<string, number>()
  if (!origin) return result
  for (const hex of hexes.value) {
    const distance = hexDistance(origin, hex.coord)
    if (distance >= 0) result.set(hex.coord, distance)
  }
  return result
})

function distanceAt(coord: string) {
  return distanceByCoord.value.get(coord) ?? -1
}

type DistanceRingLabel = { x: number; y: number; text: string }
type DistanceRing = { distance: number; path: string; labels: DistanceRingLabel[] }
type RingHex = { coord: string; x: number; y: number }

// 等高线的角必须正好落在环格中心上，不能只取“角度最接近”的格子，
// 否则地图边缘裁掉一个角后，数字会跑到直边上。
const LABEL_ANGLE_TOLERANCE = Math.PI / 180

function angleDelta(a: number, b: number) {
  const delta = Math.abs(a - b) % (Math.PI * 2)
  return delta > Math.PI ? Math.PI * 2 - delta : delta
}

function ringAngle(hex: { x: number; y: number }, origin: { x: number; y: number }) {
  return Math.atan2(hex.y - origin.y, hex.x - origin.x)
}

// 距离环是正六边形：六个顶点就是六个角上的数字位置，虚线只负责把它们连起来。
function ringOutline(origin: { x: number; y: number }, distance: number) {
  const radius = distance * Math.sqrt(3) * size
  const corners = Array.from({ length: 6 }, (_, index) => {
    const angle = index * Math.PI / 3
    return (origin.x + radius * Math.cos(angle)) + ',' + (origin.y + radius * Math.sin(angle))
  })
  return 'M' + corners.join(' L') + ' Z'
}

const distanceOriginPoint = computed(() => (distanceOrigin.value ? center(distanceOrigin.value) : null))

// 偶数距离一路画到地图边缘：每圈用一条淡虚线六边形连接六个角上的距离数字。
// 所有圈同样粗细、同样透明度，不再做 2/4 强弱区分。
const distanceRings = computed<DistanceRing[]>(() => {
  const origin = distanceOriginPoint.value
  if (!origin) return []
  const buckets = new Map<number, RingHex[]>()
  for (const hex of hexes.value) {
    const distance = hexDistance(distanceOrigin.value, hex.coord)
    if (distance > 0 && distance % 2 === 0) {
      const list = buckets.get(distance)
      if (list) list.push(hex)
      else buckets.set(distance, [hex])
    }
  }
  // 环的外轮廓是平顶六边形，六个角在 0°、60°、120°、180°、240°、300°。
  const cornerAngles = Array.from({ length: 6 }, (_, index) => index * Math.PI / 3)
  return Array.from(buckets.keys()).sort((a, b) => a - b).map((distance) => {
    const ring = buckets.get(distance) || []
    const ordered = ring.slice().sort((a, b) => ringAngle(a, origin) - ringAngle(b, origin))
    const path = ringOutline(origin, distance)
    const used = new Set<string>()
    const labels: DistanceRingLabel[] = []
    for (const target of cornerAngles) {
      let best: RingHex | null = null
      let bestDelta = Infinity
      for (const hex of ordered) {
        if (used.has(hex.coord)) continue
        const delta = angleDelta(ringAngle(hex, origin), target)
        if (delta < bestDelta) {
          bestDelta = delta
          best = hex
        }
      }
      if (best && bestDelta <= LABEL_ANGLE_TOLERANCE) {
        used.add(best.coord)
        labels.push({ x: best.x, y: best.y, text: String(distance) })
      }
    }
    return { distance, path, labels }
  })
})

function clampCamera() {
  const map = fullView.value
  const view = camera.value
  view.x = view.w >= map.w
    ? map.x + (map.w - view.w) / 2
    : Math.min(map.x + map.w - view.w, Math.max(map.x, view.x))
  view.y = view.h >= map.h
    ? map.y + (map.h - view.h) / 2
    : Math.min(map.y + map.h - view.h, Math.max(map.y, view.y))
}

function applyZoom(level: number) {
  const map = fullView.value
  const w = map.w / level
  const h = map.h / level
  const cx = camera.value.x + camera.value.w / 2
  const cy = camera.value.y + camera.value.h / 2
  camera.value = { x: cx - w / 2, y: cy - h / 2, w, h }
  clampCamera()
  zoom.value = level
}

function adjustZoom(direction: number) {
  const index = ZOOM_STEPS.indexOf(nearestZoomStep(zoom.value))
  applyZoom(ZOOM_STEPS[Math.min(ZOOM_STEPS.length - 1, Math.max(0, index + direction))])
}

const mapPanning = ref(false)
let mapPanState: { startX: number; startY: number; cameraX: number; cameraY: number; scaleX: number; scaleY: number; moved: boolean } | null = null
let mapPanLastMovedAt = 0

function startMapPan(event: PointerEvent) {
  if (event.pointerType === 'mouse' && event.button !== 0) return
  const target = event.target as Element | null
  // 舰队/机场/飞行编队自己有点击和拖拽语义，不能让地图平移抢走。
  if (target?.closest('.map-unit, .map-airfield-card, .map-flight-task, .map-ground-task, .map-cap-aircraft, .map-enemy-air, .map-recon-aircraft, button, a')) return
  const svg = mapElement.value
  const ctm = svg?.getScreenCTM()
  if (!svg || !ctm || !ctm.a || !ctm.d) return
  mapPanState = {
    startX: event.clientX,
    startY: event.clientY,
    cameraX: camera.value.x,
    cameraY: camera.value.y,
    scaleX: ctm.a,
    scaleY: ctm.d,
    moved: false,
  }
  mapPanning.value = true
  window.addEventListener('pointermove', moveMapPan)
  window.addEventListener('pointerup', endMapPan, { once: true })
  window.addEventListener('pointercancel', endMapPan, { once: true })
}

function moveMapPan(event: PointerEvent) {
  const state = mapPanState
  if (!state) return
  const dx = event.clientX - state.startX
  const dy = event.clientY - state.startY
  if (!state.moved && Math.hypot(dx, dy) < 4) return
  if (!state.moved) event.preventDefault()
  state.moved = true
  camera.value.x = state.cameraX - dx / state.scaleX
  camera.value.y = state.cameraY - dy / state.scaleY
  clampCamera()
}

function endMapPan() {
  if (!mapPanState) return
  if (mapPanState.moved) mapPanLastMovedAt = performance.now()
  mapPanState = null
  mapPanning.value = false
  window.removeEventListener('pointermove', moveMapPan)
  window.removeEventListener('pointerup', endMapPan)
  window.removeEventListener('pointercancel', endMapPan)
}

// 拖动结束后浏览器还会补一个 click；不吞掉的话松手就会顺手选中一格。
function onMapClickCapture(event: MouseEvent) {
  if (performance.now() - mapPanLastMovedAt > 250) return
  event.preventDefault()
  event.stopPropagation()
  mapPanLastMovedAt = 0
}

function center(coord: string) {
  const row = Number(coord.slice(0, 2))
  const col = Number(coord.slice(2, 4))
  return {
    x: Math.sqrt(3) * size * (col - 0.5 + (row % 2 ? 0.5 : 0)),
    y: size * (1.5 * (row - 1) + 1),
  }
}

function points(x: number, y: number) {
  return Array.from({ length: 6 }, (_, index) => {
    const angle = Math.PI / 180 * (60 * index - 30)
    return `${x + size * Math.cos(angle)},${y + size * Math.sin(angle)}`
  }).join(' ')
}

// 逐格风向箭头：服务端把整张图压成一行字符（行优先），'0'..'5' 是六方位。
// 箭头画在 hex 底部，指向"风吹向"的方向（风向按来向记：NE 的风吹向 SW）。
// SVG 的 0° 是 +x（向右），罗盘的 0° 是正北，所以旋转角 = 吹向方位 - 90°。
const windArrowOffsetY = 32
function windIndexAt(coord: string): number {
  const cells = props.windCells
  if (!cells || cells.length === 0) return -1
  const row = Number(coord.slice(0, 2))
  const col = Number(coord.slice(2))
  const cols = props.map.cols
  const index = (row - 1) * cols + (col - 1)
  if (index < 0 || index >= cells.length) return -1
  const code = cells.charCodeAt(index) - 48
  return code >= 0 && code <= 5 ? code : -1
}
// 逐格天气：0 晴 / 1 多云 / 2 降雨 / 3 风暴，直接来自服务端的天气层。
function weatherIndexAt(coord: string): number {
  const cells = props.weatherCells
  if (!cells || cells.length === 0) return 0
  const row = Number(coord.slice(0, 2))
  const col = Number(coord.slice(2))
  const index = (row - 1) * props.map.cols + (col - 1)
  if (index < 0 || index >= cells.length) return 0
  const code = cells.charCodeAt(index) - 48
  return code >= 0 && code <= 3 ? code : 0
}
// 天气不再是给格子刷底色，而是把云朵/水滴/闪电图案盖在格子上（单独一层压在最上层）。
// 图案比色块好认：色块只说明“这格有东西”，图案直接说明“多云/降雨/风暴”。
// 符号按 hex 半径 17 的坐标系画，本图半径 43，所以整体放大 size/17。
const weatherSymbolScale = size / 17
const weatherSymbolFor = ['', 'cloud', 'rain', 'storm']
const weatherHexes = computed(() => {
  const cells = props.weatherCells || ''
  if (!cells) return []
  const list: { coord: string; x: number; y: number; level: number; symbol: string }[] = []
  for (const hex of hexes.value) {
    const level = weatherIndexAt(hex.coord)
    if (level <= 0) continue
    list.push({ coord: hex.coord, x: hex.x, y: hex.y, level, symbol: weatherSymbolFor[level] || 'cloud' })
  }
  return list
})
function windRotation(coord: string): number | undefined {
  const index = windIndexAt(coord)
  if (index < 0) return undefined
  // 六方位：0=NE(30°)、1=E(90°)… 吹向 = 来向 + 180°
  const blowingTo = 30 + 60 * index + 180
  return (blowingTo - 90 + 360) % 360
}

// 从其他地方规划过来的舰队：在目标 hex 上打一个"预定航向"标记，方便看清本回合的航线。
function plannedFleetsAt(coord: string) {
  return plannedByCoord.value.get(coord) || []
}

// 自动航行的目的地：舰队设了目的地就一直朝那边开，地图上用虚线连过去 + 插一面小旗，
// 否则玩家设完目的地就只剩面板里一行字，海图上什么都看不出来。
const navRoutes = computed(() =>
  props.fleets
    .filter((fleet) => fleet.faction === props.playerFaction && fleet.destination && fleet.destination !== fleet.coord)
    .map((fleet) => ({ fleet, from: center(fleet.coord), to: center(fleet.destination || '') })),
)

function navDestinationAt(coord: string) {
  return navDestByCoord.value.get(coord) || []
}

function fleetsAt(coord: string) {
  return fleetsByCoord.value.get(coord) || []
}

// 同格两支舰队时标签分左右挂，否则两条长舰队名会直接叠在一起。
function stackedAt(coord: string) {
  return fleetsAt(coord).length > 1
}

// —— 机场与空军算子：机场画成算子，飞行中的机群用机型小旗标出 ——
const airfieldMarkers = computed(() => props.airfields || [])
const groundAirTaskTasks = computed(() => props.groundTasks || [])
function airfieldsAt(coord: string) {
  return airfieldsByCoord.value.get(coord) || []
}
// 地图上的机场直接画成机场算子卡片：生命值、可用跑道、防空、机库占用都在上面，
// 敌我双方都能看到，炮击/轰炸后不用再点开面板才知道效果。
// 敌方机场的规模和名字都公开（这些基地的位置是常识），细节仍要侦察。
// 机场点击：先取消可能残留的拖拽/长按（拖拽监听挂在 window 上，会吞掉紧随其后的 click），再回调父组件。
function airfieldClick(airfield: { id: string; coord: string; faction: string }) {
  cancelDrag()
  // 选打击目标/自动航行时，点任何机场（自己的也算）都只是"点这个 hex"。
  // 以前这条判断写在敌方分支里，于是转场选目的地时点自己的机场会被当成"打开这个机场"：
  // 面板直接跳到目标机场，转场目的永远设不上——转场目的地本来就只能是己方机场。
  if (props.strikeTargeting || props.navTargeting) {
    emit('selectHex', airfield.coord)
    return
  }
  // 敌方机场不是"自己的机场"，点它不能打开自己的航空面板：
  // 平时只打开只读情报卡，让缩小的算子也能看清生命值、跑道和防空。
  if (airfield.faction !== props.playerFaction) {
    emit('inspectAirfield', airfield.id)
    return
  }
  emit('selectAirfield', airfield.id)
}

function groundAirAt(coord: string) {
  return (props.groundAircraft || []).filter((aircraft) => aircraft.coord === coord && ['cap', 'recon', 'attack', 'attack_holding'].includes(aircraft.status))
}
// 地图上只给"正在飞出去干活"的陆基编队画飞行牌：攻击与转场。
// 侦察已经由搜索扇面（含当前距离圈上的小标记）表达，再叠一块 44×26 的大牌子只会挡住 hex；
// 巡逻（CAP／掩护）改用和舰载 CAP 一样的小方牌。
function groundFlightAt(coord: string) {
  return groundFlightByCoord.value.get(coord) || []
}
function groundPatrolAt(coord: string) {
  return groundPatrolByCoord.value.get(coord) || []
}

// —— 每帧按坐标建一次反查索引 ——
// 以前每个图层都是"遍历 3328 个 hex，再用坐标去 filter 全表"：一次推送就是
// 8 个图层 × 3328 格 × 各自的全量表，而且空图层也会生成 3328 个空 <g>。
// 现在索引建一次，图层只遍历"真的有东西"的那几格。
function groupByCoord<T>(items: T[] | undefined, key: (item: T) => string) {
  const index = new Map<string, T[]>()
  for (const item of items || []) {
    const coord = key(item)
    if (!coord) continue
    const list = index.get(coord)
    if (list) list.push(item)
    else index.set(coord, [item])
  }
  return index
}

const fleetsByCoord = computed(() => {
  const index = groupByCoord(props.fleets, (fleet) => fleet.coord)
  // 选中的舰队排在同格队首：兵牌上下错开时它压在最上面。
  const selectedID = props.selectedUnit?.id
  for (const list of index.values()) {
    if (list.length > 1) list.sort((a, b) => Number(a.id === selectedID) - Number(b.id === selectedID))
  }
  return index
})
const fleetCoords = computed(() => [...fleetsByCoord.value.keys()])

const airfieldsByCoord = computed(() => groupByCoord(airfieldMarkers.value, (airfield) => airfield.coord))
const airfieldCoords = computed(() => [...airfieldsByCoord.value.keys()])

const reconAirByCoord = computed(() => groupByCoord(reconSectors.value, (sector) => sector.current_coord || ''))
const reconAirCoords = computed(() => [...reconAirByCoord.value.keys()])

// 当前回合战报对双方公开，即使尚未形成持续追踪，也保留这一回合的遭遇算子。
const submarineMarkers = computed(() => {
  const markers = new Map<string, Submarine>()
  for (const battle of props.subBattles || []) {
    if (battle.turn !== props.turn) continue
    markers.set(battle.submarine_id, {
      id: battle.submarine_id,
      faction: battle.submarine_faction as 'A' | 'B',
      name: battle.submarine_name,
      coord: battle.coord,
      reserve: false,
      deployed: true,
      first_move_turn: 0,
      move_interval: 0,
      discovered: true,
      tracked_fleet_id: battle.fleet_id,
    })
  }
  for (const submarine of props.submarines || []) {
    if (submarine.deployed && submarine.coord) markers.set(submarine.id, submarine)
  }
  return [...markers.values()]
})
// 同格同阵营的潜艇居中堆叠；观战时双方即使同格也分成两堆，不会混成一种颜色。
const submarineStacks = computed(() => {
  const deployed = submarineMarkers.value
  return [...groupByCoord(deployed, (submarine) => `${submarine.coord}|${submarine.faction}`).values()].map((submarines) => ({
    coord: submarines[0].coord,
    faction: submarines[0].faction,
    count: submarines.length,
    submarine: submarines.find((submarine) => submarine.id === props.selectedSubmarineId) || submarines[0],
  }))
})
const submarineEngagedFleetIDs = computed(() => {
  const ids = new Set<string>()
  for (const submarine of props.submarines || []) {
    if (submarine.discovered && submarine.tracked_fleet_id) ids.add(submarine.tracked_fleet_id)
  }
  for (const battle of props.subBattles || []) {
    if (battle.turn === props.turn) ids.add(battle.fleet_id)
  }
  return ids
})
function submarineEngaged(unit: FleetGroup) {
  return submarineEngagedFleetIDs.value.has(unit.fleet_id)
}

// 当前距离圈上是谁（画在地形格上的 data-recon-current-aircraft）。
// 以前是每格 filter 一遍全部扇面：3328 × 扇面数，纯浪费。
const reconCurrentAirByCoord = computed(() => {
  const index = new Map<string, string[]>()
  for (const sector of reconSectors.value) {
    for (const coord of sector.current_ring_hexes || []) {
      const list = index.get(coord)
      if (list) list.push(sector.aircraft_id)
      else index.set(coord, [sector.aircraft_id])
    }
  }
  const joined = new Map<string, string>()
  for (const [coord, ids] of index) joined.set(coord, ids.join(','))
  return joined
})

const flightTaskByCoord = computed(() => groupByCoord(flightTasks.value, (task) => task.current_coord || ''))
const flightTaskCoords = computed(() => [...flightTaskByCoord.value.keys()])


const groundFlightByCoord = computed(() => groupByCoord(
  groundAirTaskTasks.value.filter((task) => task.airborne_count > 0 && (task.kind === 'attack' || task.kind === 'transfer')),
  (task) => task.current_coord || '',
))
const groundPatrolByCoord = computed(() => groupByCoord(
  groundAirTaskTasks.value.filter((task) => task.airborne_count > 0 && (task.kind === 'cap' || task.kind === 'cover')),
  (task) => task.current_coord || '',
))
const groundAirCoords = computed(() => [...new Set([...groundFlightByCoord.value.keys(), ...groundPatrolByCoord.value.keys()])])

const enemyAirByCoord = computed(() => groupByCoord(props.enemyAir, (contact) => contact.coord))
const enemyAirCoords = computed(() => [...enemyAirByCoord.value.keys()])

// 预定航向标记（"这支舰队本回合要去哪"）与自动航行目的地旗标，都按目标格索引。
const plannedByCoord = computed(() => {
  const index = new Map<string, string[]>()
  for (const fleet of props.fleets || []) {
    if (fleet.faction !== props.playerFaction) continue
    const coord = moveOrder(fleet)?.coord
    if (!coord || coord === fleet.coord) continue
    const list = index.get(coord)
    if (list) list.push(fleet.fleet_id)
    else index.set(coord, [fleet.fleet_id])
  }
  return index
})
const navDestByCoord = computed(() => groupByCoord(navRoutes.value, (route) => route.fleet.destination || ''))

// 地形格子上挂着的动态装饰（侦察色块、可走点、预定/目的地标记、距离、当前侦察机）。
// 每格压成一个签名字符串：绝大多数格子什么也没有，共用同一个空签名，
// 于是 v-memo 一次字符串比较就能跳过整棵子树——状态推送时不再重排 3328 个 hex。
const hexSigs = computed(() => {
  const list = hexes.value
  const sigs: string[] = new Array(list.length)
  const legal = selectedMoves.value
  const recon = reconCells.value
  const reconCurrent = reconCurrentCells.value
  const empty = 'g' + hexGeneration
  const windCells = props.windCells || ''
  for (let index = 0; index < list.length; index++) {
    const hex = list[index]
    const windChar = windCells ? windCells[index] || '' : ''
    const planned = plannedFleetsAt(hex.coord)
    const navDest = navDestinationAt(hex.coord)
    const reconAir = reconCurrentAircraftAt(hex.coord)
    const airfieldHere = airfieldsAt(hex.coord).length > 0
    const distance = distanceAt(hex.coord)
    const isLegal = legal.has(hex.coord)
    const inRecon = recon.has(hex.coord)
    const inCurrent = reconCurrent.has(hex.coord)
    const hovered = dropHover.value === hex.coord
    if (!planned.length && !navDest.length && !reconAir && !airfieldHere && distance < 0 && !isLegal && !inRecon && !inCurrent && !hovered && !windChar) {
      sigs[index] = empty
      continue
    }
    sigs[index] = [
      hex.terrain, hex.name, windChar,
      Number(isLegal), Number(hovered), Number(inRecon), Number(inCurrent),
      reconSectorClass(hex.coord), reconCurrentClass(hex.coord),
      distance, reconAir, Number(airfieldHere),
      planned.join(','),
      navDest.map((route) => route.fleet.fleet_id).join(','),
    ].join('|')
  }
  return sigs
})


// 两支舰队同格（接触状态）时反方向错开并左右挂名；单支舰队仍然居中。
// 兵牌 52×36，位移太小的话连旗舰剪影都会被压住。
function unitPosition(coord: string, index: number) {
  const point = center(coord)
  if (!stackedAt(coord)) return { x: point.x, y: point.y }
  return { x: point.x + index * 22 - 11, y: point.y - index * 20 + 10 }
}

function submarinePosition(coord: string, faction: 'A' | 'B') {
  const point = center(coord)
  const sides = new Set(submarineStacks.value.filter((stack) => stack.coord === coord).map((stack) => stack.faction))
  const x = point.x + (sides.size > 1 ? (faction === 'A' ? -12 : 12) : 0)
  const fleets = fleetsAt(coord)
  if (!fleets.some(submarineEngaged)) return { x, y: point.y }
  const fleetBottom = Math.max(...fleets.map((_, index) => unitPosition(coord, index).y + 18))
  return { x, y: fleetBottom + 11 }
}

function moveOrder(fleet: FleetGroup) {
  return fleetOrder(fleet, props.orders)
}

function clickHex(coord: string) {
  emit('selectHex', coord)
}

// 选目标 / 选航行目的地时，地图上任何牌子的点击都只算"点这一格"：
// 以前点编队牌、飞机编队牌会先打开详情面板，玩家以为"改目标没成功"，
// 实际上是面板把地图挡住了，目标根本没提交。
function markerClick(coord: string, action: () => void) {
  if (props.strikeTargeting || props.navTargeting) {
    emit('selectHex', coord || '')
    return
  }
  action()
}

// 拖动是纯前端手势，服务端一条日志都看不到——拖失败时只能靠浏览器控制台自证。
// 在控制台执行 localStorage.setItem('gq_drag_debug','1') 并刷新，就能看到每次拖动的判定过程。
function dragDebug(...args: unknown[]) {
  if (typeof localStorage === 'undefined' || localStorage.getItem('gq_drag_debug') !== '1') return
  // 拼成一行，方便直接复制给开发看；对象用 JSON，别打印 [object Object]。
  const parts = args.map((arg) => (typeof arg === 'string' ? arg : JSON.stringify(arg)))
  console.log('[gq drag] ' + parts.join(' '))
}

function startDrag(unit: FleetGroup, event: PointerEvent) {
  dragDebug('pointerdown', { unit: unit.id, faction: unit.faction, coord: unit.coord, moves: unit.legal_moves, cooldown: unit.move_cooldown, pointerType: event.pointerType, button: event.button })
  // 本回合已就绪 = 冻结：连拖都不该开始，否则玩家拖一半才发现服务端根本不收。
  if (props.locked) {
    emit('notice', '已就绪，操作已锁定：等双方都确认后由服务器统一结算')
    return
  }
  if (unit.faction !== props.playerFaction || (event.pointerType === 'mouse' && event.button !== 0)) return
  event.preventDefault()
  // 冷却中的舰队拖了也没用，之前是完全静默——玩家只会觉得"拖不动"。这里直接说明原因。
  if (!canPlanMove(unit)) {
    const cooldown = unit.move_cooldown || 0
    emit('notice', cooldown > 0 ? `${unit.name}还要 ${cooldown} 回合才能再次移动` : '这支舰队本回合无法移动')
    return
  }
  draggedUnit.value = unit
  pointerStart = { x: event.clientX, y: event.clientY }
  dragMatrix = mapElement.value?.getScreenCTM()?.inverse() || null
  longPressTriggered = false
  // 长按打开舰队面板只给触屏用：鼠标长按 550ms 会和"按住 → 慢慢拖"直接打架，
  // 之前的症状就是手一稳，面板弹出来，拖拽被取消。
  if (event.pointerType !== 'mouse') {
    longPressTimer = setTimeout(() => {
      longPressTriggered = true
      emit('manageHex', unit.coord)
      cancelDrag()
    }, 550)
  }
  window.addEventListener('pointermove', continueDrag)
  window.addEventListener('pointerup', finishDrag, { once: true })
  window.addEventListener('pointercancel', cancelDrag, { once: true })
}

// 移动是"本回合规划"：已经规划过的舰队仍然可以改规划，只有冷却中的舰队不能拖拽。
function canPlanMove(fleet?: FleetGroup) {
  return Boolean(fleet) && (fleet!.legal_moves?.length || 0) > 0
}

function continueDrag(event: PointerEvent) {
  if (!draggedUnit.value || !canPlanMove(draggedUnit.value) || !mapElement.value) return
  if (Math.hypot(event.clientX - pointerStart.x, event.clientY - pointerStart.y) > 5) {
    if (longPressTimer) clearTimeout(longPressTimer)
    dragging.value = true
  }
  if (!dragging.value) return
  pendingPoint = { x: event.clientX, y: event.clientY }
  if (dragFrame) return
  dragFrame = requestAnimationFrame(renderDrag)
}

function renderDrag() {
  dragFrame = 0
  if (!draggedUnit.value || !mapElement.value || !dragMatrix) return
  const point = mapElement.value.createSVGPoint()
  point.x = pendingPoint.x
  point.y = pendingPoint.y
  const local = point.matrixTransform(dragMatrix)
  nextTick(() => ghostElement.value?.setAttribute('transform', `translate(${local.x}, ${local.y})`))
  const coord = coordAtPoint(pendingPoint.x, pendingPoint.y)
  dropHover.value = draggedUnit.value.legal_moves?.includes(coord) ? coord : ''
  if (dropHover.value) dragDebug('hover legal hex', coord)
}

function finishDrag(event: PointerEvent) {
  if (longPressTimer) clearTimeout(longPressTimer)
  if (!longPressTriggered && dragging.value && draggedUnit.value) {
    const coord = coordAtPoint(event.clientX, event.clientY)
    if (draggedUnit.value.legal_moves?.includes(coord)) {
      emit('selectUnit', draggedUnit.value)
      emit('move', coord)
    } else if (coord) {
      // 落在不可移动的格子上必须说话，否则玩家以为是拖拽坏了。
      emit('notice', `${draggedUnit.value.name}不能移动到 ${coord}：只能拖到绿色标记的格子`)
    }
    dragDebug('drop', { from: draggedUnit.value.coord, to: coord, legal: draggedUnit.value.legal_moves, accepted: Boolean(draggedUnit.value.legal_moves?.includes(coord)) })
  }
  longPressTriggered = false
  cancelDrag()
}

function coordAtPoint(x: number, y: number) {
  // 松手时压在舰队面板/侧栏上就不算"拖到格子"：否则"最近格子"兜底会把
  // 面板边缘误判成旁边的 hex，拖一下面板就莫名其妙下了一条移动令。
  const top = document.elementsFromPoint(x, y)[0]
  if (top && top.closest('.fleet-workbench, .fleet-dialog, .map-toolbar, .command-sidebar, .turn-strip')) return ''
  for (const element of document.elementsFromPoint(x, y)) {
    const hex = element.closest('.hex-cell')
    if (hex) return hex.getAttribute('data-coord') || ''
  }
  // 六边形之间有缝，松手时掉在缝里就会"明明拖到了却放不上去"。
  // 这里退化到最近的格子：只要离中心不超过大半个格宽就认。
  let best = ''
  let bestDistance = Infinity
  let bestWidth = 0
  for (const hex of document.querySelectorAll('.hex-cell')) {
    const rect = hex.getBoundingClientRect()
    if (!rect.width) continue
    const distance = Math.hypot(x - (rect.left + rect.width / 2), y - (rect.top + rect.height / 2))
    if (distance < bestDistance) {
      bestDistance = distance
      bestWidth = rect.width
      best = hex.getAttribute('data-coord') || ''
    }
  }
  return bestDistance <= Math.max(24, bestWidth * 0.85) ? best : ''
}

function cancelDrag() {
  if (longPressTimer) clearTimeout(longPressTimer)
  if (dragFrame) cancelAnimationFrame(dragFrame)
  dragFrame = 0
  dragMatrix = null
  window.removeEventListener('pointermove', continueDrag)
  window.removeEventListener('pointerup', finishDrag)
  window.removeEventListener('pointercancel', cancelDrag)
  draggedUnit.value = undefined
  dragging.value = false
  dropHover.value = ''
}

onUnmounted(() => {
  cancelDrag()
  endMapPan()
})

// 地图上的舰队算子：用服务端下发的舰船卡素材（旗舰 + 次舰），没有素材就不画剪影。
function leadMasks(unit: FleetGroup) {
  const members = unit.members?.length ? unit.members : [unit as unknown as Unit]
  return members
    .map((member) => ({ id: member.id, mask: shipMaskURL(member.card?.picture) }))
    .filter((item) => item.mask)
    .slice(0, 2) as { id: string; mask: string }[]
}

function singleMask(unit: Unit) {
  // 拆分出来的单舰编队：编队投影本身没有 card.picture，旗舰图只挂在成员上。
  const members = (unit as unknown as FleetGroup).members
  if (members?.length) return shipMaskURL(members[0].card?.picture)
  return shipMaskURL(unit.card?.picture)
}

function headingAngle(heading?: string) {
  return { E: 90, SE: 150, SW: 210, W: 270, NW: 300, NE: 30 }[heading || 'NE'] ?? 30
}

// 服务端下发的是 small/medium/large（同时也是中文"小型舰队"等），两种都兼容。
function contactSizeName(size?: 'small' | 'medium' | 'large' | string) {
  if (size && size.length > 2) return size
  return { small: '小型舰队', medium: '中型舰队', large: '大型舰队' }[(size as 'small' | 'medium' | 'large') || 'medium']
}
</script>

<template>
  <div class="map-zoom-controls" @click.stop>
    <button aria-label="缩小地图" :disabled="zoom <= ZOOM_STEPS[0]" @click="adjustZoom(-1)">−</button>
    <button class="map-zoom-value" aria-label="地图缩放复位" @click="applyZoom(1)">{{ Math.round(zoom * 100) }}%</button>
    <button aria-label="放大地图" :disabled="zoom >= ZOOM_STEPS[ZOOM_STEPS.length - 1]" @click="adjustZoom(1)">+</button>
  </div>
  <!-- 摄像机层：+ / − 只改 viewBox，拖动地图改 viewBox 原点；滚轮不再改变缩放。 -->
  <div class="map-scroll">
  <svg ref="mapElement" class="hex-map" :class="{ panning: mapPanning }" :viewBox="viewBox" :data-zoom="zoom.toFixed(1)" role="img" aria-label="沙盒海图" @pointerdown="startMapPan" @click.capture="onMapClickCapture">
  <defs>
    <!-- 天气图案符号：云朵 / 水滴 / 闪电。按 hex 半径 17 的坐标系画，使用时整体缩放。 -->
    <symbol id="wx-cloud" overflow="visible">
      <g class="wx-cloud">
        <circle cx="-5.4" cy="1.4" r="3.4" />
        <circle cx="-0.6" cy="-1.2" r="4.8" />
        <circle cx="3.6" cy="1.2" r="3.2" />
        <rect x="-8.6" y="1.2" width="12.8" height="3.6" rx="1.8" />
      </g>
      <g class="wx-cloud wx-cloud-small">
        <circle cx="7.2" cy="-5.8" r="2.4" />
        <circle cx="9.8" cy="-4.6" r="1.8" />
        <rect x="4.6" y="-4.6" width="5.8" height="2.4" rx="1.2" />
      </g>
    </symbol>
    <symbol id="wx-rain" overflow="visible">
      <path d="M0 -7.4c0 0 3.2 4.8 3.2 7a3.2 3.2 0 0 1-6.4 0c0-2.2 3.2-7 3.2-7Z" />
      <path transform="translate(-7.6 4.2) scale(.76)" d="M0 -7.4c0 0 3.2 4.8 3.2 7a3.2 3.2 0 0 1-6.4 0c0-2.2 3.2-7 3.2-7Z" />
      <path transform="translate(7.6 4.2) scale(.76)" d="M0 -7.4c0 0 3.2 4.8 3.2 7a3.2 3.2 0 0 1-6.4 0c0-2.2 3.2-7 3.2-7Z" />
    </symbol>
    <symbol id="wx-storm" overflow="visible">
      <path d="M1.6 -9.6 L-5.6 0.9 H-1.4 L-2.4 9.6 L6 -1.2 H1.4 L4.8 -9.6 Z" />
    </symbol>
  </defs>
    <g v-for="(hex, index) in hexes" :key="hex.coord" v-memo="[hexSigs[index]]" class="hex-cell" :data-coord="hex.coord" :data-distance="distanceAt(hex.coord) >= 0 ? distanceAt(hex.coord) : undefined" :data-recon-current-aircraft="reconCurrentAircraftAt(hex.coord) || undefined" :class="[`terrain-${hex.terrain}`, reconSectorClass(hex.coord), reconCurrentClass(hex.coord), { legal: selectedMoves.has(hex.coord), 'drop-hover': dropHover === hex.coord }]" @click="clickHex(hex.coord)">
      <polygon :points="points(hex.x, hex.y)" />
      <!-- 侦察覆盖必须是叠在地形上的独立一层：直接改地块 polygon 的 fill 会把海水/岛屿
           整个替换掉，扇面就变成一块盖住地形的色块，观感非常刺眼。 -->
      <polygon v-if="reconSectorClass(hex.coord)" class="recon-tint" :points="points(hex.x, hex.y)" />
      <polygon v-if="reconCurrentClass(hex.coord)" class="recon-current-tint" :points="points(hex.x, hex.y)" />
      <text class="coord-label" :x="hex.x" :y="hex.y - 30">{{ hex.coord }}</text>
      <!-- 机场格的地名由机场算子自己写，地形标签再写一遍会变成上下两个"拉包尔"。 -->
      <text v-if="hex.name && !airfieldsAt(hex.coord).length" class="terrain-label" :x="hex.x" :y="hex.y + 18">{{ hex.name }}</text>
      <!-- 每格底部的风向小箭头：指向"风吹向"的方向，替代原来只有一个全局风向 -->
      <path v-if="windRotation(hex.coord) !== undefined" class="hex-wind" :transform="'translate(' + hex.x + ' ' + (hex.y + windArrowOffsetY) + ') rotate(' + windRotation(hex.coord) + ')'" d="M-5 0H3M0.5 -3L3.5 0L0.5 3" />
      <circle v-if="selectedMoves.has(hex.coord)" class="legal-dot" :cx="hex.x" :cy="hex.y" r="5" />
      <g v-if="plannedFleetsAt(hex.coord).length" class="planned-here" :data-planned-here="plannedFleetsAt(hex.coord).join(',')">
        <circle :cx="hex.x + 24" :cy="hex.y - 20" r="7.5" />
        <path :d="'M' + (hex.x + 24) + ' ' + (hex.y - 24) + ' l3 4 h-6 z'" />
      </g>
      <g v-if="navDestinationAt(hex.coord).length" class="nav-destination" :data-nav-destination="navDestinationAt(hex.coord).map((route) => route.fleet.fleet_id).join(',')">
        <line :x1="hex.x - 5" :y1="hex.y + 10" :x2="hex.x - 5" :y2="hex.y - 16" />
        <path :d="'M' + (hex.x - 5) + ' ' + (hex.y - 16) + ' h15 l-4 5 4 5 h-15 z'" />
        <title>舰队自动航行目的地</title>
      </g>
    </g>
    <g v-if="distanceRings.length" class="distance-contours">
      <g v-for="ring in distanceRings" :key="'distance-ring-' + ring.distance">
        <path class="distance-ring" :d="ring.path" :data-range-distance="ring.distance" />
        <g v-for="(label, index) in ring.labels" :key="'distance-label-' + ring.distance + '-' + index" class="distance-ring-label" :data-range-label="ring.distance">
          <circle :cx="label.x" :cy="label.y" r="6.5" />
          <text :x="label.x" :y="label.y + 2.8">{{ label.text }}</text>
        </g>
      </g>
      <circle v-if="distanceOriginPoint" class="distance-origin-marker" :cx="distanceOriginPoint.x" :cy="distanceOriginPoint.y" r="4" />
    </g>
    <g class="nav-routes">
      <line
        v-for="route in navRoutes"
        :key="'nav-' + route.fleet.fleet_id"
        :x1="route.from.x"
        :y1="route.from.y"
        :x2="route.to.x"
        :y2="route.to.y"
        :data-nav-fleet="route.fleet.fleet_id"
      />
    </g>
    <!-- 自下而上的层叠：机场 < 舰队 < 飞行编队 < 云层。机场卡 56×52 居中，几乎占满一个 hex，
         所以压在舰队层下面；舰队兵牌和编队牌盖在卡片上，三样东西都可以从『点格子』的清单里点开，
         不必靠牌面露出来的那点边缘。云层只画图案、不接点击。 -->
    <g v-for="coord in airfieldCoords" :key="`airfield-${coord}`">
      <foreignObject
        v-for="(airfield, index) in airfieldsAt(coord)"
        :key="airfield.id"
        class="map-airfield-card"
        :class="[`faction-${airfield.faction.toLowerCase()}`, { enemy: !spectate && airfield.faction !== playerFaction, unknown: !spectate && airfield.contact_level < 3 }]"
        :data-airfield-id="airfield.id"
        :x="center(coord).x - 28 + (index - (airfieldsAt(coord).length - 1) / 2) * 58"
        :y="center(coord).y - 25"
        width="56"
        height="52"
        @click.stop="airfieldClick(airfield)"
      >
        <div xmlns="http://www.w3.org/1999/xhtml" class="map-airfield-card-inner">
          <AirfieldCounter :airfield="airfieldContactStatus(airfield)" />
        </div>
      </foreignObject>
    </g>
    <g v-for="coord in fleetCoords" :key="`units-${coord}`">
      <g
        v-for="(unit, index) in fleetsAt(coord)"
        :key="unit.fleet_id"
        class="map-unit"
        :class="[`faction-${unit.faction.toLowerCase()}`, `contact-${unit.contact_level || 0}`, { selected: selectedUnit?.fleet_id === unit.fleet_id, draggable: unit.faction === playerFaction && (unit.legal_moves?.length || 0) > 0, moved: unit.moved || (unit.move_cooldown || 0) > 0, planned: Boolean(moveOrder(unit)), 'submarine-engaged': submarineEngaged(unit), 'drag-source': dragging && draggedUnit?.id === unit.id }]"
        :data-unit-id="unit.id"
        :data-fleet-id="unit.fleet_id"
        :data-display-coord="unit.coord"
        :data-moved="String(Boolean(unit.moved))"
        :transform="`translate(${unitPosition(coord, index).x}, ${unitPosition(coord, index).y})`"
        @pointerdown="startDrag(unit, $event)"
        @click.stop="markerClick(unit.coord, () => emit('selectUnit', unit))"
      >
        <rect x="-26" y="-18" width="52" height="36" rx="3" />
        <path class="unit-stripe" d="M-26 11h52v7h-52z" />
        <path v-if="submarineEngaged(unit)" class="unit-submarine-stripe" d="M-26 14.5h52V18h-52z" />
        <template v-if="unit.contact_level && unit.contact_level < 4">
          <!-- 情报分级：1 级"有敌舰"；2 级只给规模；3 级再加"存在航母/战列舰/重巡"。
               4 级才画真实剪影和舰名（下面的 unit-name 分支）。 -->
          <text class="contact-size-label" text-anchor="middle" :y="unit.contact_level >= 3 && unit.contact_capitals?.length ? -1 : 5">{{ unit.contact_level <= 1 ? label('contact_level', '1') : contactSizeLabel(unit.contact_size) }}</text>
          <text v-if="unit.contact_level >= 3 && unit.contact_capitals?.length" class="contact-size-label contact-capitals" text-anchor="middle" y="10">{{ contactCapitalsLabel(unit.contact_capitals) }}</text>
        </template>
        <template v-else>
          <g v-if="unit.count > 1" class="fleet-silhouettes">
            <template v-if="leadMasks(unit).length">
              <image
                v-for="(lead, index) in leadMasks(unit)"
                :key="lead.id"
                :href="lead.mask"
                :x="leadMasks(unit).length > 1 ? -23 + index * 25 : -24"
                y="-13"
                :width="leadMasks(unit).length > 1 ? 21 : 48"
                height="20"
                preserveAspectRatio="xMidYMax meet"
                class="fleet-ship-mask"
              />
            </template>
          </g>
          <g v-else class="unit-ship-mark">
            <image v-if="singleMask(unit)" :href="singleMask(unit)!" x="-24" y="-13" width="48" height="20" preserveAspectRatio="xMidYMax meet" class="fleet-ship-mask" />
          </g>
          <text class="unit-strength" x="-20" y="-7">{{ unit.count }}</text>
        </template>
        <!-- 航向用锐角三角形：重心靠近顶点，一眼能看出朝哪边，等边三角形和兵牌方框容易混。 -->
        <path class="unit-heading" :transform="`translate(19 -7) rotate(${headingAngle(unit.heading)})`" d="M0 -9 L3.4 5 L-3.4 5 Z" />
        <!-- 情报 4 级时编成已知，标记上直接写舰队/旗舰名，别再写"情报4"这种没信息量的字。 -->
        <text class="unit-name" :text-anchor="stackedAt(coord) ? (index === 0 ? 'end' : 'start') : 'middle'" :x="stackedAt(coord) ? (index === 0 ? -10 : 10) : 0" y="34">{{ unit.contact_level && unit.contact_level < 4 ? `情报${unit.contact_level}` : unit.name }}</text>
        <text v-if="unit.contact_level && unit.contact_level < 4" class="unit-order" text-anchor="middle" y="48">{{ label('contact_level', 'unknown') }}</text>
        <text v-else-if="moveOrder(unit)" class="unit-order" text-anchor="middle" y="48">预定 → {{ moveOrder(unit)?.coord }}</text>
        <!-- 冷却回合数不再画在地图上：舰队面板里已经写了"还需N回合"，地图上挂一行红字太吵。 -->
        <text v-else-if="unit.moved && !stackedAt(coord)" class="unit-order" text-anchor="middle" y="48">已移动</text>
      </g>
    </g>
    <!-- 同格同阵营潜艇居中堆叠，最多画两层背衬；观战双方同格时向左右错开。 -->
    <g
      v-for="stack in submarineStacks"
      :key="'sub-stack-' + stack.coord + '-' + stack.faction"
      class="map-submarine"
      :class="[`faction-${stack.faction.toLowerCase()}`, { spectator: spectate, selected: selectedSubmarineId === stack.submarine.id, spotted: stack.submarine.discovered }]"
      :data-submarine-id="stack.submarine.id"
      :data-submarine-coord="stack.coord"
      :data-submarine-faction="stack.faction"
      :data-submarine-count="stack.count"
      :data-submarine-discovered="stack.submarine.discovered ? '1' : '0'"
      :transform="`translate(${submarinePosition(stack.coord, stack.faction).x}, ${submarinePosition(stack.coord, stack.faction).y})`"
      @click.stop="!spectate && emit('selectSubmarine', stack.submarine.id)"
    >
      <rect v-for="layer in Math.min(stack.count - 1, 2)" :key="'sub-layer-' + layer" class="submarine-stack-edge" :x="-17 - layer * 2" :y="-9 - layer * 2" width="34" height="18" rx="3" />
      <rect class="submarine-body" x="-17" y="-9" width="34" height="18" rx="3" />
      <image v-if="shipMaskURL(stack.submarine.picture)" :href="shipMaskURL(stack.submarine.picture)!" x="-15" y="-7" width="30" height="14" preserveAspectRatio="xMidYMid meet" class="submarine-mask" />
      <g v-if="stack.count > 1" class="submarine-stack-count" transform="translate(16 -10)">
        <circle r="7" />
        <text y="2.5" text-anchor="middle">{{ stack.count }}</text>
      </g>
      <text class="submarine-caption" y="20" text-anchor="middle">{{ stack.submarine.discovered && !spectate ? '发现潜艇' : stack.submarine.name }}</text>
      <title>{{ spectate ? `${stack.faction === 'B' ? '日军' : '美军'} · ${stack.submarine.name} · ${stack.coord}` : stack.submarine.discovered ? `已发现敌方潜艇 · 在 ${stack.coord} 被钉住，逐回合投放深水炸弹` : `${stack.submarine.name} · 点击选中后再点海格部署` }}{{ stack.count > 1 ? ` · 本方本格共${stack.count}艘` : '' }}</title>
    </g>
    <g v-for="coord in reconAirCoords" :key="`recon-aircraft-${coord}`">
      <g
        v-for="(sector, index) in reconAircraftAt(coord)"
        :key="sector.aircraft_id"
        class="map-recon-aircraft"
        :class="{ embarked: sector.status !== 'recon' }"
        :data-aircraft-id="sector.aircraft_id"
        :data-status="sector.status"
        :data-display-coord="sector.current_coord"
        :transform="`translate(${reconPosition(coord, index, reconAircraftAt(coord).length).x}, ${reconPosition(coord, index, reconAircraftAt(coord).length).y})`"
      >
        <circle r="8" />
        <path d="M-6 3 0-7 6 3 0 1Z" />
        <text x="10" y="3">{{ sector.status === 'recon' ? '侦察' : '舰上' }}</text>
        <title>{{ sector.aircraft_id }} · {{ sector.status === 'recon' ? `第${sector.recon_distance || 0}距离圈` : '随母舰移动' }}</title>
      </g>
    </g>
    <g v-for="coord in flightTaskCoords" :key="`flight-task-${coord}`">
      <g
        v-for="(task, index) in flightTasksAt(coord)"
        :key="task.id"
        class="map-flight-task"
        :class="[task.kind, `faction-${(spectate && task.faction ? task.faction : playerFaction).toLowerCase()}`, { selected: selectedAirTaskId === task.id, returning: task.returning }]"
        :data-flight-task-id="task.id"
        :data-task-kind="task.kind"
        :data-task-faction="spectate && task.faction ? task.faction : playerFaction"
        :data-display-coord="task.current_coord"
        :data-target-coord="task.target_coord"
        :transform="`translate(${flightPosition(coord, index, flightTasksAt(coord).length).x}, ${flightPosition(coord, index, flightTasksAt(coord).length).y})`"
        @click.stop="markerClick(task.current_coord || coord, () => emit('selectAirTask', task.id))"
      >
        <rect x="-23" y="-15" width="46" height="30" rx="3" />
        <image v-if="taskPicture(task)" class="flight-task-picture" :href="taskPicture(task)" x="-20" y="-13" width="40" height="26" preserveAspectRatio="xMidYMid meet" :data-aircraft-picture="taskPictureFile(task)" />
        <text class="flight-task-count" x="21" y="13">{{ task.airborne_count }}/{{ task.aircraft_ids.length }}</text>
        <text class="flight-task-caption" y="23">{{ flightLabel(task) }} · {{ task.current_coord }}{{ task.cannot_land ? ' ⚠' : '' }}</text>
        <title>{{ flightModels(task) }} · {{ flightLabel(task) }} · {{ task.airborne_count }}/{{ task.aircraft_ids.length }} 已升空 · 当前 {{ task.current_coord }} · 目标 {{ task.target_coord || '未设置' }} · 剩余航程 {{ flightRange(task) }}</title>
      </g>
    </g>
    <g v-for="badge in capBadgeList" :key="`cap-${badge.key}`">
      <g
        class="map-cap-aircraft"
        :class="[spectate && badge.faction ? `faction-${badge.faction}` : '', { returning: badge.returning === badge.total, forward: badge.forward > 0 }]"
        :data-cap-coord="badge.coord"
        :data-cap-faction="badge.faction"
        :data-cap-count="badge.total"
        :transform="`translate(${badge.x}, ${badge.y}) scale(${capBadgeScale})`"
      >
        <rect x="-21" y="-8" width="42" height="16" rx="3" />
        <text x="0" y="3">{{ capBadgeLabel(badge) }}</text>
        <title>{{ capBadgeTitle(badge) }}</title>
      </g>
    </g>
    <g v-for="coord in groundAirCoords" :key="`ground-air-${coord}`">
      <g
        v-for="(task, index) in groundFlightAt(coord)"
        :key="task.id"
        class="map-ground-task"
        :class="[task.kind, `faction-${(spectate && task.faction ? task.faction : playerFaction).toLowerCase()}`, { selected: selectedAirTaskId === task.id, returning: task.returning }]"
        :data-ground-task-id="task.id"
        :data-task-faction="spectate && task.faction ? task.faction : playerFaction"
        :transform="`translate(${flightPosition(coord, index, groundFlightAt(coord).length).x}, ${flightPosition(coord, index, groundFlightAt(coord).length).y})`"
        @click.stop="markerClick(task.current_coord || coord, () => emit('selectAirTask', task.id))"
      >
        <rect x="-22" y="-13" width="44" height="26" rx="3" />
        <image v-if="taskPicture(task)" class="flight-task-picture" :href="taskPicture(task)" x="-19" y="-11" width="38" height="22" preserveAspectRatio="xMidYMid meet" :data-aircraft-picture="taskPictureFile(task)" />
        <text class="ground-task-count" x="20" y="11">{{ task.airborne_count }}</text>
        <text class="ground-task-caption" y="21">{{ flightLabel(task) }} · {{ task.current_coord }}</text>
        <title>{{ flightModels(task) }} · {{ flightLabel(task) }} · 来自机场 · {{ task.current_coord }}{{ task.target_coord ? ' → ' + task.target_coord : '' }}</title>
      </g>
      <g
        v-for="(task, index) in groundPatrolAt(coord)"
        :key="`ground-patrol-${task.id}`"
        class="map-ground-patrol"
        :class="[task.kind, spectate && task.faction ? `faction-${task.faction.toLowerCase()}` : '']"
        :data-ground-task-id="task.id"
        :transform="`translate(${center(coord).x + (index - (groundPatrolAt(coord).length - 1) / 2) * 40}, ${center(coord).y - 34}) scale(${capBadgeScale})`"
      >
        <rect x="-19" y="-8" width="38" height="16" rx="3" />
        <text x="0" y="3">{{ task.kind === 'cover' ? 'COV' : 'CAP' }} ×{{ task.airborne_count }}</text>
        <title>{{ flightModels(task) }} · 来自机场 · {{ task.current_coord }}{{ task.target_coord ? ' → ' + task.target_coord : '' }}</title>
      </g>
    </g>
    <g v-for="coord in enemyAirCoords" :key="`enemy-air-${coord}`">
      <g
        v-for="(contact, index) in enemyAirAt(coord)"
        :key="contact.task_id"
        class="map-enemy-air"
        :class="[`kind-${contact.kind}`, `faction-${contact.faction.toLowerCase()}`, { converging: contact.returning }]"
        :data-enemy-air-id="contact.task_id"
        :data-display-coord="contact.coord"
        :data-detected-by="contact.detected_by"
        :transform="`translate(${center(coord).x - 34 + index * 22}, ${center(coord).y + 30})`"
      >
        <rect x="-10" y="-7" width="20" height="14" rx="2" />
        <image v-if="enemyAirPicture(contact)" class="flight-task-picture enemy-air-picture" :href="enemyAirPicture(contact)" x="-9" y="-6" width="18" height="12" preserveAspectRatio="xMidYMid meet" :data-aircraft-picture="contact.pictures?.[0] || ''" />
        <text x="0" y="13">{{ contact.count }}</text>
        <title>敌方编队 · {{ contact.models?.join('/') }} · 约 {{ contact.count }} 个算子 · {{ contact.coord }} · 由{{ label('detected_by', contact.detected_by) }}发现</title>
      </g>
    </g>
    <g v-if="dragging && draggedUnit" ref="ghostElement" class="map-unit drag-ghost" :class="`faction-${draggedUnit.faction.toLowerCase()}`" transform="translate(-1000, -1000)">
      <rect x="-26" y="-18" width="52" height="36" rx="3" />
      <path class="unit-stripe" d="M-26 11h52v7h-52z" />
      <path v-if="submarineEngaged(draggedUnit)" class="unit-submarine-stripe" d="M-26 14.5h52V18h-52z" />
      <g class="unit-ship-mark">
        <image v-if="singleMask(draggedUnit)" :href="singleMask(draggedUnit)!" x="-24" y="-13" width="48" height="20" preserveAspectRatio="xMidYMax meet" class="fleet-ship-mask" />
      </g>
      <text class="unit-strength" x="-20" y="-7">{{ draggedUnit.count }}</text>
      <path class="unit-heading" :transform="`translate(19 -7) rotate(${headingAngle(draggedUnit.heading)})`" d="M0 -9 L3.4 5 L-3.4 5 Z" />
    </g>
    <!-- 天气图案层：云朵=多云、水滴=降雨、闪电=风暴。只盖图案，不给格子刷底色；
         单独一层压在所有元素之上，并关掉 pointer-events，点击照常落在下面的格子/兵牌上。 -->
    <g class="weather-overlay" aria-hidden="true">
      <use
        v-for="wx in weatherHexes"
        :key="'wx-' + wx.coord"
        class="wx-icon"
        :data-weather="wx.level"
        :href="'#wx-' + wx.symbol"
        :transform="'translate(' + wx.x + ' ' + wx.y + ') scale(' + weatherSymbolScale + ')'"
      />
    </g>
  </svg>
  </div>
</template>

