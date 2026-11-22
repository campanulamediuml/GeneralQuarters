<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import type { AirTask, DistanceReference, GameMap, Order, ReconSector, Unit } from '../types'
import { contactCapitalsLabel, contactSizeLabel, fleetOrder, type FleetGroup } from '../fleet'
import ShipSilhouette from './ShipSilhouette.vue'
import { shipMaskURL } from '../silhouette-masks'
import { aircraftSilhouette } from '../silhouettes'
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
}>()

const emit = defineEmits<{
  selectUnit: [unit: Unit]
  selectHex: [coord: string]
  manageHex: [coord: string]
  move: [coord: string]
  /** 拖动失败时的提示（冷却中 / 不是相邻格），交给上层显示 */
  notice: [text: string]
  selectAirfield: [airfieldID: string]
}>()

const size = 43
// 缩放记在 localStorage：地图是 v-if 挂载的，切到航空界面再回来组件会重建，
// 之前缩放跟着丢，每次回来都跳回 100%。
const ZOOM_STORAGE_KEY = 'gq_map_zoom'
function storedZoom() {
  const saved = Number(localStorage.getItem(ZOOM_STORAGE_KEY))
  return Number.isFinite(saved) && saved >= 0.6 && saved <= 2.4 ? saved : 1
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

const hexes = computed(() => {
  const result = []
  for (let row = 1; row <= props.map.rows; row++) {
    for (let col = 1; col <= props.map.cols; col++) {
      const coord = `${String(row).padStart(2, '0')}${String(col).padStart(2, '0')}`
      const special = props.map.cells.find((cell) => cell.coord === coord)
      result.push({ coord, ...center(coord), terrain: special?.terrain || 'water', name: special?.name || '' })
    }
  }
  return result
})

const viewBox = computed(() => `0 0 ${Math.sqrt(3) * size * (props.map.cols + 1)} ${size * (1.5 * props.map.rows + 0.7)}`)
const selectedMoves = computed(() => new Set(props.selectedUnit?.legal_moves || []))
// 已发现的敌方飞行编队按当前坐标分组。
function enemyAirAt(coord: string) {
  return (props.enemyAir || []).filter((contact) => contact.coord === coord)
}
const reconSectors = computed<ReconSector[]>(() => props.reconTasks.flatMap((task) => task.recon_sectors || []))
const reconCells = computed(() => new Set(reconSectors.value.flatMap((sector) => sector.sector_hexes || [])))
const reconCurrentCells = computed(() => new Set(reconSectors.value.flatMap((sector) => sector.current_ring_hexes || [])))
const flightTasks = computed(() => props.airTasks.filter((task) => (task.kind === 'attack' || task.kind === 'transfer') && task.airborne_count > 0 && task.current_coord))
const mapStyle = computed(() => ({ width: `${zoom.value * 100}%`, minWidth: `${620 * zoom.value}px`, maxHeight: zoom.value > 1 ? 'none' : '100%' }))

function reconAircraftAt(coord: string) {
  return reconSectors.value.filter((sector) => sector.current_coord === coord)
}

// CAP：掩护 = 巡逻点/前出点及周围 1 hex（共 7 格），但巡逻范围不绘制；
// 地图上只标出"前出拦截中"和"受损/弹药耗尽返航中"的 CAP，便于看清哪批飞机在脱离。
const capAircraft = computed(() => props.airTasks
  .filter((task) => task.kind === 'cap')
  .flatMap((task) => task.aircraft)
  .map((aircraft) => ({ ...aircraft, taskId: aircraft.task_id || '' }))
  .filter((aircraft) => ['cap', 'attack', 'recon'].includes(aircraft.status)))
function capAircraftAt(coord: string) {
  return capAircraft.value.filter((aircraft) => (aircraft.forward_coord || aircraft.coord) === coord)
}

function reconCurrentAircraftAt(coord: string) {
  return reconSectors.value.filter((sector) => sector.current_ring_hexes?.includes(coord)).map((sector) => sector.aircraft_id).join(',')
}

function reconPosition(coord: string, index: number, total: number) {
  const point = center(coord)
  return { x: point.x + (index - (total - 1) / 2) * 14, y: point.y - 30 - index * 7 }
}

function flightTasksAt(coord: string) {
  return flightTasks.value.filter((task) => task.current_coord === coord)
}

function flightPosition(coord: string, index: number, total: number) {
  const point = center(coord)
  return { x: point.x + (index - (total - 1) / 2) * 30, y: point.y - 52 - index * 8 }
}

function flightRange(task: AirTask) {
  const ranges = task.aircraft.filter((aircraft) => ['attack', 'attack_holding'].includes(aircraft.status)).map((aircraft) => aircraft.remaining_range)
  return ranges.length ? Math.min(...ranges) : task.aircraft[0]?.remaining_range || 0
}

function flightModels(task: AirTask) {
  return [...new Set(task.aircraft.map((aircraft) => aircraft.model))].join(' / ')
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

function distanceLabel(coord: string) {
  const distance = distanceAt(coord)
  // 偶数格画环，另外把 1/3 也标出来——贴身距离（相邻、隔一格）是操作里最常量的。
  return distance > 0 && (distance % 2 === 0 || distance <= 3) ? String(distance) : ''
}

function adjustZoom(amount: number) {
  zoom.value = Math.min(2.4, Math.max(0.6, Math.round((zoom.value + amount) * 10) / 10))
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

// 从其他地方规划过来的舰队：在目标 hex 上打一个"预定航向"标记，方便看清本回合的航线。
function plannedFleetsAt(coord: string) {
  return (props.fleets || [])
    .filter((fleet) => fleet.faction === props.playerFaction && fleet.coord !== coord && moveOrder(fleet)?.coord === coord)
    .map((fleet) => fleet.fleet_id)
}

// 自动航行的目的地：舰队设了目的地就一直朝那边开，地图上用虚线连过去 + 插一面小旗，
// 否则玩家设完目的地就只剩面板里一行字，海图上什么都看不出来。
const navRoutes = computed(() =>
  props.fleets
    .filter((fleet) => fleet.faction === props.playerFaction && fleet.destination && fleet.destination !== fleet.coord)
    .map((fleet) => ({ fleet, from: center(fleet.coord), to: center(fleet.destination || '') })),
)

function navDestinationAt(coord: string) {
  return navRoutes.value.filter((route) => route.fleet.destination === coord)
}

function fleetsAt(coord: string) {
  return props.fleets.filter((fleet) => fleet.coord === coord).sort((a, b) => Number(a.id === props.selectedUnit?.id) - Number(b.id === props.selectedUnit?.id))
}

// 同格两支舰队时标签分左右挂，否则两条长舰队名会直接叠在一起。
function stackedAt(coord: string) {
  return fleetsAt(coord).length > 1
}

// —— 机场与空军算子：机场画成算子，飞行中的机群用机型小旗标出 ——
const airfieldMarkers = computed(() => props.airfields || [])
const groundAirTaskTasks = computed(() => props.groundTasks || [])
function airfieldsAt(coord: string) {
  return airfieldMarkers.value.filter((airfield) => airfield.coord === coord)
}
// 敌方机场只在接触等级足够时显示类型，否则只显示"机场?"
// 机场点击：先取消可能残留的拖拽/长按（拖拽监听挂在 window 上，会吞掉紧随其后的 click），再回调父组件。
function airfieldClick(airfield: { id: string }) {
  cancelDrag()
  emit('selectAirfield', airfield.id)
}

function airfieldLabel(airfield: { name: string; contact_level: number; faction: string }) {
  if (airfield.faction === props.playerFaction || airfield.contact_level >= 3) return '机场'
  return '未知机场'
}
function groundAirAt(coord: string) {
  return (props.groundAircraft || []).filter((aircraft) => aircraft.coord === coord && ['cap', 'recon', 'attack', 'attack_holding'].includes(aircraft.status))
}
function groundTaskAt(coord: string) {
  return groundAirTaskTasks.value.filter((task) => task.current_coord === coord && task.airborne_count > 0)
}

// 两支舰队同格（接触状态）时反方向错开并左右挂名；单支舰队仍然居中。
// 兵牌 52×36，位移太小的话连旗舰剪影都会被压住。
function unitPosition(coord: string, index: number) {
  const point = center(coord)
  if (!stackedAt(coord)) return { x: point.x, y: point.y }
  return { x: point.x + index * 22 - 11, y: point.y - index * 20 + 10 }
}

function moveOrder(fleet: FleetGroup) {
  return fleetOrder(fleet, props.orders)
}

function clickHex(coord: string) {
  emit('selectHex', coord)
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
      emit('notice', `${draggedUnit.value.name}不能移动到 ${coord}：只能拖到绿色标记的相邻 hex`)
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

onUnmounted(cancelDrag)

// 地图上的舰队算子：优先用带头两艘舰的真实剪影（旗舰 + 次舰）
function leadMasks(unit: FleetGroup) {
  const members = unit.members?.length ? unit.members : [unit as unknown as Unit]
  return members
    .map((member) => ({ id: member.id, name: member.name, mask: shipMaskURL(member.card?.picture, member.name), picture: member.card?.picture }))
    .filter((item) => item.mask)
    .slice(0, 2) as { id: string; name: string; mask: string }[]
}

function singleMask(unit: Unit) {
  return shipMaskURL(unit.card?.picture, unit.name)
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
    <button aria-label="缩小地图" @click="adjustZoom(-0.2)">−</button>
    <button class="map-zoom-value" aria-label="地图缩放复位" @click="zoom = 1">{{ Math.round(zoom * 100) }}%</button>
    <button aria-label="放大地图" @click="adjustZoom(0.2)">+</button>
  </div>
  <!-- 缩放条钉在可视区，滚的是里面这层 .map-scroll；否则缩放条跟着 hex 一起滚出屏幕。 -->
  <div class="map-scroll">
  <svg ref="mapElement" class="hex-map" :viewBox="viewBox" :style="mapStyle" :data-zoom="zoom.toFixed(1)" role="img" aria-label="沙盒海图">
    <g v-for="hex in hexes" :key="hex.coord" class="hex-cell" :data-coord="hex.coord" :data-distance="distanceAt(hex.coord) >= 0 ? distanceAt(hex.coord) : undefined" :data-recon-current-aircraft="reconCurrentAircraftAt(hex.coord) || undefined" :class="[`terrain-${hex.terrain}`, { legal: selectedMoves.has(hex.coord), 'drop-hover': dropHover === hex.coord, 'recon-sector': reconCells.has(hex.coord), 'recon-current': reconCurrentCells.has(hex.coord), 'distance-reference': distanceAt(hex.coord) > 0 && distanceAt(hex.coord) <= 2, 'distance-outer': distanceAt(hex.coord) === 2, 'distance-origin': distanceAt(hex.coord) === 0 }]" @click="clickHex(hex.coord)">
      <polygon :points="points(hex.x, hex.y)" />
      <text class="coord-label" :x="hex.x" :y="hex.y - 23">{{ hex.coord }}</text>
      <text v-if="hex.name" class="terrain-label" :x="hex.x" :y="hex.y + 27">{{ hex.name }}</text>
      <circle v-if="selectedMoves.has(hex.coord)" class="legal-dot" :cx="hex.x" :cy="hex.y" r="5" />
      <g v-if="distanceLabel(hex.coord)" class="distance-label" :data-distance-label="distanceLabel(hex.coord)">
        <circle :cx="hex.x" :cy="hex.y" r="9" />
        <text :x="hex.x" :y="hex.y + 3">{{ distanceLabel(hex.coord) }}</text>
      </g>
      <g v-if="plannedFleetsAt(hex.coord).length" class="planned-here" :data-planned-here="plannedFleetsAt(hex.coord).join(',')">
        <circle :cx="hex.x + 24" :cy="hex.y - 20" r="7" />
        <path :d="'M' + (hex.x + 24) + ' ' + (hex.y - 24) + ' l3 4 h-6 z'" />
      </g>
      <g v-if="navDestinationAt(hex.coord).length" class="nav-destination" :data-nav-destination="navDestinationAt(hex.coord).map((route) => route.fleet.fleet_id).join(',')">
        <line :x1="hex.x - 5" :y1="hex.y + 10" :x2="hex.x - 5" :y2="hex.y - 16" />
        <path :d="'M' + (hex.x - 5) + ' ' + (hex.y - 16) + ' h15 l-4 5 4 5 h-15 z'" />
        <title>舰队自动航行目的地</title>
      </g>
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
    <g v-for="hex in hexes" :key="`units-${hex.coord}`">
      <g
        v-for="(unit, index) in fleetsAt(hex.coord)"
        :key="unit.fleet_id"
        class="map-unit"
        :class="[`faction-${unit.faction.toLowerCase()}`, `contact-${unit.contact_level || 0}`, { selected: selectedUnit?.fleet_id === unit.fleet_id, draggable: unit.faction === playerFaction && (unit.legal_moves?.length || 0) > 0, moved: unit.moved || (unit.move_cooldown || 0) > 0, planned: Boolean(moveOrder(unit)), 'drag-source': dragging && draggedUnit?.id === unit.id }]"
        :data-unit-id="unit.id"
        :data-fleet-id="unit.fleet_id"
        :data-display-coord="unit.coord"
        :data-moved="String(Boolean(unit.moved))"
        :transform="`translate(${unitPosition(hex.coord, index).x}, ${unitPosition(hex.coord, index).y})`"
        @pointerdown="startDrag(unit, $event)"
        @click.stop="emit('selectUnit', unit)"
      >
        <rect x="-26" y="-18" width="52" height="36" rx="3" />
        <path class="unit-stripe" d="M-26 11h52v7h-52z" />
        <template v-if="unit.contact_level && unit.contact_level < 4">
          <!-- 情报分级：1 级"有敌舰"；2 级只给规模；3 级再加"存在航母/战列舰/重巡"。
               4 级才画真实剪影和舰名（下面的 unit-name 分支）。 -->
          <text class="contact-size-label" text-anchor="middle" :y="unit.contact_level >= 3 && unit.contact_capitals?.length ? -1 : 5">{{ unit.contact_level <= 1 ? '有敌舰' : contactSizeLabel(unit.contact_size) }}</text>
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
            <template v-else><path d="M-21 5h42l-6 5h-29zM-5 0H6v5H-5zM-2-5H1V0H-2z" /><path d="M-21 5h42l-6 5h-29zM-5 0H6v5H-5zM-2-5H1V0H-2z" transform="translate(0 -7)" /></template>
          </g>
          <g v-else class="unit-ship-mark">
            <image v-if="singleMask(unit)" :href="singleMask(unit)!" x="-24" y="-13" width="48" height="20" preserveAspectRatio="xMidYMax meet" class="fleet-ship-mask" />
            <ShipSilhouette v-else :type="unit.type" :name="unit.name" :picture="unit.card?.picture" />
          </g>
          <text class="unit-strength" x="-20" y="-7">{{ unit.count }}</text>
        </template>
        <text class="unit-heading" :transform="`rotate(${headingAngle(unit.heading)} 19 -7)`" x="19" y="-7">▲</text>
        <!-- 情报 4 级时编成已知，标记上直接写舰队/旗舰名，别再写"情报4"这种没信息量的字。 -->
        <text class="unit-name" :text-anchor="stackedAt(hex.coord) ? (index === 0 ? 'end' : 'start') : 'middle'" :x="stackedAt(hex.coord) ? (index === 0 ? -10 : 10) : 0" y="34">{{ unit.contact_level && unit.contact_level < 4 ? `情报${unit.contact_level}` : unit.name }}</text>
        <text v-if="unit.contact_level && unit.contact_level < 4" class="unit-order" text-anchor="middle" y="48">最后已知</text>
        <text v-else-if="moveOrder(unit)" class="unit-order" text-anchor="middle" y="48">预定 → {{ moveOrder(unit)?.coord }}</text>
        <!-- 冷却回合数不再画在地图上：舰队面板里已经写了"还需N回合"，地图上挂一行红字太吵。 -->
        <text v-else-if="unit.moved && !stackedAt(hex.coord)" class="unit-order" text-anchor="middle" y="48">已移动</text>
      </g>
    </g>
    <g v-for="hex in hexes" :key="`recon-aircraft-${hex.coord}`">
      <g
        v-for="(sector, index) in reconAircraftAt(hex.coord)"
        :key="sector.aircraft_id"
        class="map-recon-aircraft"
        :class="{ embarked: sector.status !== 'recon' }"
        :data-aircraft-id="sector.aircraft_id"
        :data-status="sector.status"
        :data-display-coord="sector.current_coord"
        :transform="`translate(${reconPosition(hex.coord, index, reconAircraftAt(hex.coord).length).x}, ${reconPosition(hex.coord, index, reconAircraftAt(hex.coord).length).y})`"
      >
        <circle r="8" />
        <path d="M-6 3 0-7 6 3 0 1Z" />
        <path class="recon-aircraft-mark" :d="aircraftSilhouette('recon')" transform="translate(-7 -5) scale(.23)" />
        <text x="10" y="3">{{ sector.status === 'recon' ? '侦察' : '舰上' }}</text>
        <title>{{ sector.aircraft_id }} · {{ sector.status === 'recon' ? `第${sector.recon_distance || 0}距离圈` : '随母舰移动' }}</title>
      </g>
    </g>
    <g v-for="hex in hexes" :key="`flight-task-${hex.coord}`">
      <g
        v-for="(task, index) in flightTasksAt(hex.coord)"
        :key="task.id"
        class="map-flight-task"
        :class="task.kind"
        :data-flight-task-id="task.id"
        :data-task-kind="task.kind"
        :data-display-coord="task.current_coord"
        :data-target-coord="task.target_coord"
        :transform="`translate(${flightPosition(hex.coord, index, flightTasksAt(hex.coord).length).x}, ${flightPosition(hex.coord, index, flightTasksAt(hex.coord).length).y})`"
      >
        <rect x="-24" y="-15" width="48" height="30" rx="3" />
        <path class="flight-task-symbol" :data-task-kind="task.kind" :d="task.kind === 'attack' ? aircraftSilhouette('dive_bomber') : task.kind === 'cover' ? aircraftSilhouette('fighter') : aircraftSilhouette('recon')" transform="translate(-14 -16) scale(.46)" />
        <text x="30" y="-3">{{ task.kind === 'attack' ? '攻击' : task.kind === 'cover' ? '掩护' : '转场' }} {{ task.airborne_count }}/{{ task.aircraft_ids.length }}{{ task.cannot_land ? ' ⚠' : '' }}</text>
        <text class="flight-task-range" x="30" y="10">航程 {{ flightRange(task) }}</text>
        <text class="flight-task-coord" y="27">{{ task.current_coord }} → {{ task.target_coord || '等待目标' }}</text>
        <title>{{ flightModels(task) }} · {{ task.airborne_count }}/{{ task.aircraft_ids.length }} 已升空 · 当前 {{ task.current_coord }} · 目标 {{ task.target_coord || '未设置' }} · 剩余航程 {{ flightRange(task) }}</title>
      </g>
    </g>
    <g v-for="hex in hexes" :key="`cap-${hex.coord}`">
      <g
        v-for="(aircraft, index) in capAircraftAt(hex.coord)"
        :key="aircraft.id"
        class="map-cap-aircraft"
        :class="{ returning: aircraft.returning, forward: Boolean(aircraft.forward_coord) }"
        :data-aircraft-id="aircraft.id"
        :data-forward="aircraft.forward_coord || ''"
        :transform="`translate(${center(hex.coord).x + (index - 1) * 16}, ${center(hex.coord).y - 34})`"
      >
        <rect x="-13" y="-8" width="26" height="16" rx="3" />
        <path class="cap-symbol" :d="aircraftSilhouette('fighter')" transform="translate(-7 -5) scale(.23)" />
        <text x="0" y="2">{{ aircraft.returning ? '返' : aircraft.forward_coord ? '拦' : 'CAP' }}</text>
        <title>{{ aircraft.model }} · {{ aircraft.returning ? '受损/弹药耗尽，返航中' : aircraft.forward_coord ? '雷达引导前出拦截' : 'CAP 巡逻' }}</title>
      </g>
    </g>
    <g v-for="hex in hexes" :key="`airfield-${hex.coord}`">
      <g
        v-for="(airfield, index) in airfieldsAt(hex.coord)"
        :key="airfield.id"
        class="map-airfield"
        :class="[`faction-${airfield.faction.toLowerCase()}`, { enemy: airfield.faction !== playerFaction, unknown: airfield.contact_level < 3 }]"
        :data-airfield-id="airfield.id"
        :transform="`translate(${center(hex.coord).x + (index - (airfieldsAt(hex.coord).length - 1) / 2) * 20}, ${center(hex.coord).y + 16})`"
        @click.stop="airfieldClick(airfield)"
      >
        <rect x="-17" y="-8" width="34" height="16" rx="2" />
        <path class="airfield-runway" d="M-13 1h26v3h-26z" />
        <path class="airfield-stripes" d="M-9 1v3M-1 1v3M7 1v3" />
        <path v-if="airfield.contact_level >= 3" class="airfield-tower" d="M6 -7h5v8H6z" />
        <text class="airfield-type" text-anchor="middle" y="12">{{ airfieldLabel(airfield) }} · {{ airfield.runways }}跑道</text>
        <title>{{ airfield.name }} · {{ ({ field: '野战机场', heavy: '重型机场', major: '大型机场' } as Record<string,string>)[airfield.type] }} · {{ airfield.runways }} 跑道 · 情报{{ airfield.contact_level }}</title>
      </g>
    </g>
    <g v-for="hex in hexes" :key="`ground-air-${hex.coord}`">
      <g
        v-for="(task, index) in groundTaskAt(hex.coord)"
        :key="task.id"
        class="map-ground-task"
        :class="task.kind"
        :data-ground-task-id="task.id"
        :transform="`translate(${flightPosition(hex.coord, index, groundTaskAt(hex.coord).length).x}, ${flightPosition(hex.coord, index, groundTaskAt(hex.coord).length).y})`"
      >
        <rect x="-22" y="-13" width="44" height="26" rx="3" />
        <path class="flight-task-symbol" :d="task.kind === 'attack' ? aircraftSilhouette('dive_bomber') : task.kind === 'recon' ? aircraftSilhouette('recon') : aircraftSilhouette('fighter')" transform="translate(-13 -11) scale(.5)" />
        <text class="ground-task-label" x="14" y="-2">{{ task.kind === 'attack' ? '攻击' : task.kind === 'recon' ? '侦察' : 'CAP' }} {{ task.airborne_count }}</text>
        <text class="ground-task-coord" y="10">{{ task.current_coord }}</text>
        <title>{{ flightModels(task) }} · 来自机场 · {{ task.current_coord }}{{ task.target_coord ? ' → ' + task.target_coord : '' }}</title>
      </g>
    </g>
    <g v-for="hex in hexes" :key="`enemy-air-${hex.coord}`">
      <g
        v-for="(contact, index) in enemyAirAt(hex.coord)"
        :key="contact.task_id"
        class="map-enemy-air"
        :class="[`kind-${contact.kind}`, { converging: contact.returning }]"
        :data-enemy-air-id="contact.task_id"
        :data-display-coord="contact.coord"
        :data-detected-by="contact.detected_by"
        :transform="`translate(${center(hex.coord).x - 34 + index * 22}, ${center(hex.coord).y + 30})`"
      >
        <rect x="-10" y="-7" width="20" height="14" rx="2" />
        <path class="enemy-air-symbol" :d="aircraftSilhouette(contact.kind === 'attack' ? 'dive_bomber' : contact.kind === 'recon' ? 'recon' : 'fighter')" transform="translate(-6 -5) scale(.22)" />
        <text x="0" y="13">{{ contact.count }}</text>
        <title>敌方编队 · {{ contact.models?.join('/') }} · 约 {{ contact.count }} 个算子 · {{ contact.coord }} · 由{{ { radar: '雷达', recon: '侦察机', visual: '目视' }[contact.detected_by] || contact.detected_by }}发现</title>
      </g>
    </g>
    <g v-if="dragging && draggedUnit" ref="ghostElement" class="map-unit drag-ghost" :class="`faction-${draggedUnit.faction.toLowerCase()}`" transform="translate(-1000, -1000)">
      <rect x="-26" y="-18" width="52" height="36" rx="3" />
      <path class="unit-stripe" d="M-26 11h52v7h-52z" />
      <g v-if="draggedUnit.count > 1" class="fleet-silhouettes"><path d="M-21 5h42l-6 5h-29zM-5 0H6v5H-5zM-2-5H1V0H-2z" /><path d="M-21 5h42l-6 5h-29zM-5 0H6v5H-5zM-2-5H1V0H-2z" transform="translate(0 -7)" /></g>
      <g v-else class="unit-ship-mark"><ShipSilhouette :type="draggedUnit.type" :name="draggedUnit.name" :picture="draggedUnit.card?.picture" /></g>
      <text class="unit-strength" x="-20" y="-7">{{ draggedUnit.count }}</text>
      <text class="unit-heading" :transform="`rotate(${headingAngle(draggedUnit.heading)} 19 -7)`" x="19" y="-7">▲</text>
    </g>
  </svg>
  </div>
</template>
