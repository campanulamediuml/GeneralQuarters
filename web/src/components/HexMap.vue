<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref } from 'vue'
import type { GameMap, Order, Unit } from '../types'
import { fleetOrder, type FleetGroup } from '../fleet'

const props = defineProps<{
  map: GameMap
  fleets: FleetGroup[]
  playerFaction: 'A' | 'B'
  selectedUnit?: Unit
  orders: Order[]
}>()

const emit = defineEmits<{
  selectUnit: [unit: Unit]
  selectHex: [coord: string]
  manageHex: [coord: string]
  move: [coord: string]
}>()

const size = 43
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
  if (selectedMoves.value.has(coord)) emit('move', coord)
  else emit('selectHex', coord)
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
</script>

<template>
  <svg ref="mapElement" class="hex-map" :viewBox="viewBox" role="img" aria-label="沙盒海图">
    <g v-for="hex in hexes" :key="hex.coord" class="hex-cell" :data-coord="hex.coord" :class="[`terrain-${hex.terrain}`, { legal: selectedMoves.has(hex.coord), 'drop-hover': dropHover === hex.coord }]" @click="clickHex(hex.coord)">
      <polygon :points="points(hex.x, hex.y)" />
      <text class="coord-label" :x="hex.x" :y="hex.y - 23">{{ hex.coord }}</text>
      <text v-if="hex.name" class="terrain-label" :x="hex.x" :y="hex.y + 27">{{ hex.name }}</text>
      <circle v-if="selectedMoves.has(hex.coord)" class="legal-dot" :cx="hex.x" :cy="hex.y" r="5" />
    </g>
    <g v-for="hex in hexes" :key="`units-${hex.coord}`">
      <g
        v-for="(unit, index) in fleetsAt(hex.coord)"
        :key="unit.fleet_id"
        class="map-unit"
        :class="[`faction-${unit.faction.toLowerCase()}`, { selected: selectedUnit?.fleet_id === unit.fleet_id, draggable: unit.faction === playerFaction && !unit.moved, moved: unit.moved, planned: Boolean(moveOrder(unit)), 'drag-source': dragging && draggedUnit?.id === unit.id }]"
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
        <g v-if="unit.count > 1" class="fleet-silhouettes"><path d="M-21 5h42l-6 5h-29zM-5 0H6v5H-5zM-2-5H1V0H-2z" /><path d="M-21 5h42l-6 5h-29zM-5 0H6v5H-5zM-2-5H1V0H-2z" transform="translate(0 -7)" /></g>
        <text v-else class="unit-type" text-anchor="middle" y="4">{{ unit.type }}</text>
        <text class="unit-strength" x="-20" y="-7">{{ unit.count }}</text>
        <text class="unit-heading" :transform="`rotate(${headingAngle(unit.heading)} 19 -7)`" x="19" y="-7">▲</text>
        <text class="unit-name" text-anchor="middle" y="34">{{ unit.name }}</text>
        <text v-if="moveOrder(unit)" class="unit-order" text-anchor="middle" y="48">预定 {{ unit.coord }}</text>
        <text v-else-if="unit.moved" class="unit-order" text-anchor="middle" y="48">已移动</text>
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
