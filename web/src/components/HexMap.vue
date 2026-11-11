<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref } from 'vue'
import type { AirTask, DistanceReference, GameMap, Order, ReconSector, Unit } from '../types'
import { fleetOrder, type FleetGroup } from '../fleet'

const props = defineProps<{
  map: GameMap
  fleets: FleetGroup[]
  playerFaction: 'A' | 'B'
  selectedUnit?: Unit
  orders: Order[]
  reconTasks: AirTask[]
  airTasks: AirTask[]
  distanceReference?: DistanceReference
}>()

const emit = defineEmits<{
  selectUnit: [unit: Unit]
  selectHex: [coord: string]
  manageHex: [coord: string]
  move: [coord: string]
}>()

const size = 43
const zoom = ref(1)
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
const reconSectors = computed<ReconSector[]>(() => props.reconTasks.flatMap((task) => task.recon_sectors || []))
const reconCells = computed(() => new Set(reconSectors.value.flatMap((sector) => sector.sector_hexes || [])))
const reconCurrentCells = computed(() => new Set(reconSectors.value.flatMap((sector) => sector.current_ring_hexes || [])))
const flightTasks = computed(() => props.airTasks.filter((task) => (task.kind === 'attack' || task.kind === 'transfer') && task.airborne_count > 0 && task.current_coord))
const mapStyle = computed(() => ({ width: `${zoom.value * 100}%`, minWidth: `${620 * zoom.value}px`, maxHeight: zoom.value > 1 ? 'none' : '100%' }))

function reconAircraftAt(coord: string) {
  return reconSectors.value.filter((sector) => sector.current_coord === coord)
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
const distanceByCoord = computed(() => new Map((props.distanceReference?.cells || []).map((cell) => [cell.coord, cell.distance])))

function distanceAt(coord: string) {
  return distanceByCoord.value.get(coord) ?? -1
}

function distanceLabel(coord: string) {
  const distance = distanceAt(coord)
  return distance > 0 && distance % 2 === 0 ? String(distance) : ''
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

function fleetsAt(coord: string) {
  return props.fleets.filter((fleet) => fleet.coord === coord).sort((a, b) => Number(a.id === props.selectedUnit?.id) - Number(b.id === props.selectedUnit?.id))
}

function unitPosition(coord: string, index: number) {
  const point = center(coord)
  return { x: point.x + index * 13 - 6, y: point.y - index * 13 + 5 }
}

function moveOrder(fleet: FleetGroup) {
  return fleetOrder(fleet, props.orders)
}

function clickHex(coord: string) {
  emit('selectHex', coord)
}

function startDrag(unit: FleetGroup, event: PointerEvent) {
  if (unit.faction !== props.playerFaction || (event.pointerType === 'mouse' && event.button !== 0)) return
  event.preventDefault()
  draggedUnit.value = unit
  pointerStart = { x: event.clientX, y: event.clientY }
  dragMatrix = mapElement.value?.getScreenCTM()?.inverse() || null
  longPressTriggered = false
  longPressTimer = setTimeout(() => {
    longPressTriggered = true
    emit('manageHex', unit.coord)
    cancelDrag()
  }, 550)
  window.addEventListener('pointermove', continueDrag)
  window.addEventListener('pointerup', finishDrag, { once: true })
  window.addEventListener('pointercancel', cancelDrag, { once: true })
}

function continueDrag(event: PointerEvent) {
  if (!draggedUnit.value || draggedUnit.value.moved || !mapElement.value) return
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
}

function finishDrag(event: PointerEvent) {
  if (longPressTimer) clearTimeout(longPressTimer)
  if (!longPressTriggered && dragging.value && draggedUnit.value) {
    const coord = coordAtPoint(event.clientX, event.clientY)
    if (draggedUnit.value.legal_moves?.includes(coord)) {
      emit('selectUnit', draggedUnit.value)
      emit('move', coord)
    }
  }
  longPressTriggered = false
  cancelDrag()
}

function coordAtPoint(x: number, y: number) {
  for (const element of document.elementsFromPoint(x, y)) {
    const hex = element.closest('.hex-cell')
    if (hex) return hex.getAttribute('data-coord') || ''
  }
  return ''
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

function headingAngle(heading?: string) {
  return { E: 90, SE: 150, SW: 210, W: 270, NW: 300, NE: 30 }[heading || 'NE'] ?? 30
}

function contactSizeName(size?: 'small' | 'medium' | 'large') {
  return { small: '小型舰队', medium: '中型舰队', large: '大型舰队' }[size || 'medium']
}
</script>

<template>
  <div class="map-zoom-controls" @click.stop>
    <button aria-label="缩小地图" @click="adjustZoom(-0.2)">−</button>
    <button class="map-zoom-value" aria-label="地图缩放复位" @click="zoom = 1">{{ Math.round(zoom * 100) }}%</button>
    <button aria-label="放大地图" @click="adjustZoom(0.2)">+</button>
  </div>
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
          <text v-if="unit.contact_level === 1" class="contact-size-label" text-anchor="middle" y="5">{{ contactSizeName(unit.contact_size) }}</text>
          <g v-else-if="unit.contact_level === 2" class="fleet-silhouettes"><path d="M-21 5h42l-6 5h-29zM-5 0H6v5H-5zM-2-5H1V0H-2z" /></g>
          <g v-else class="fleet-silhouettes"><path d="M-21 5h42l-6 5h-29zM-5 0H6v5H-5zM-2-5H1V0H-2z" /><path d="M-21 5h42l-6 5h-29zM-5 0H6v5H-5zM-2-5H1V0H-2z" transform="translate(0 -7)" /></g>
        </template>
        <template v-else>
          <g v-if="unit.count > 1" class="fleet-silhouettes"><path d="M-21 5h42l-6 5h-29zM-5 0H6v5H-5zM-2-5H1V0H-2z" /><path d="M-21 5h42l-6 5h-29zM-5 0H6v5H-5zM-2-5H1V0H-2z" transform="translate(0 -7)" /></g>
          <text v-else class="unit-type" text-anchor="middle" y="4">{{ unit.type }}</text>
          <text class="unit-strength" x="-20" y="-7">{{ unit.count }}</text>
        </template>
        <text class="unit-heading" :transform="`rotate(${headingAngle(unit.heading)} 19 -7)`" x="19" y="-7">▲</text>
        <text class="unit-name" text-anchor="middle" y="34">{{ unit.contact_level ? `情报${unit.contact_level}` : unit.name }}</text>
        <text v-if="unit.contact_level && unit.contact_level < 4" class="unit-order" text-anchor="middle" y="48">最后已知</text>
        <text v-else-if="moveOrder(unit)" class="unit-order" text-anchor="middle" y="48">预定 {{ unit.coord }}</text>
        <text v-else-if="(unit.move_cooldown || 0) > 0" class="unit-order" text-anchor="middle" y="48">{{ unit.move_cooldown }}回合后可移动</text>
        <text v-else-if="unit.moved" class="unit-order" text-anchor="middle" y="48">已移动</text>
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
        <path class="flight-task-symbol" d="M-3-11h6l2 8 15 7v4L5 5 4 11l6 3v3l-10-2-10 2v-3l6-3-1-6-15 3V4L-5-3Z" />
        <text x="30" y="-3">{{ task.kind === 'attack' ? '攻击' : '转场' }} {{ task.airborne_count }}/{{ task.aircraft_ids.length }}</text>
        <text class="flight-task-range" x="30" y="10">航程 {{ flightRange(task) }}</text>
        <text class="flight-task-coord" y="27">{{ task.current_coord }} → {{ task.target_coord || '等待目标' }}</text>
        <title>{{ flightModels(task) }} · {{ task.airborne_count }}/{{ task.aircraft_ids.length }} 已升空 · 当前 {{ task.current_coord }} · 目标 {{ task.target_coord || '未设置' }} · 剩余航程 {{ flightRange(task) }}</title>
      </g>
    </g>
    <g v-if="dragging && draggedUnit" ref="ghostElement" class="map-unit drag-ghost" :class="`faction-${draggedUnit.faction.toLowerCase()}`" transform="translate(-1000, -1000)">
      <rect x="-26" y="-18" width="52" height="36" rx="3" />
      <path class="unit-stripe" d="M-26 11h52v7h-52z" />
      <g v-if="draggedUnit.count > 1" class="fleet-silhouettes"><path d="M-21 5h42l-6 5h-29zM-5 0H6v5H-5zM-2-5H1V0H-2z" /><path d="M-21 5h42l-6 5h-29zM-5 0H6v5H-5zM-2-5H1V0H-2z" transform="translate(0 -7)" /></g>
      <text v-else class="unit-type" text-anchor="middle" y="4">{{ draggedUnit.type }}</text>
      <text class="unit-strength" x="-20" y="-7">{{ draggedUnit.count }}</text>
      <text class="unit-heading" :transform="`rotate(${headingAngle(draggedUnit.heading)} 19 -7)`" x="19" y="-7">▲</text>
    </g>
  </svg>
</template>
