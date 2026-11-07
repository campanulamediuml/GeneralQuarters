<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue'
import type { GameMap, Order, Unit } from '../types'

const props = defineProps<{
  map: GameMap
  units: Unit[]
  playerFaction: 'A' | 'B'
  selectedUnit?: Unit
  orders: Order[]
}>()

const emit = defineEmits<{
  selectUnit: [unit: Unit]
  move: [coord: string]
}>()

const size = 43
const mapElement = ref<SVGSVGElement>()
const draggedUnit = ref<Unit>()
const dragging = ref(false)
const dragPosition = ref({ x: 0, y: 0 })
const dropHover = ref('')
let pointerStart = { x: 0, y: 0 }

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

function unitsAt(coord: string) {
  return props.units.filter((unit) => unit.coord === coord)
}

function unitPosition(coord: string, index: number) {
  const point = center(coord)
  return { x: point.x + index * 13 - 6, y: point.y - index * 13 + 5 }
}

function orderFor(unitID: string) {
  return props.orders.find((order) => order.unit_id === unitID && order.type === 'SET_MOVE')
}

function startDrag(unit: Unit, event: PointerEvent) {
  if (unit.faction !== props.playerFaction || (event.pointerType === 'mouse' && event.button !== 0)) return
  event.preventDefault()
  emit('selectUnit', unit)
  draggedUnit.value = unit
  pointerStart = { x: event.clientX, y: event.clientY }
  dragPosition.value = unitPosition(unit.coord, 0)
  window.addEventListener('pointermove', continueDrag)
  window.addEventListener('pointerup', finishDrag, { once: true })
  window.addEventListener('pointercancel', cancelDrag, { once: true })
}

function continueDrag(event: PointerEvent) {
  if (!draggedUnit.value || !mapElement.value) return
  if (Math.hypot(event.clientX - pointerStart.x, event.clientY - pointerStart.y) > 5) dragging.value = true
  if (!dragging.value) return
  const matrix = mapElement.value.getScreenCTM()
  if (!matrix) return
  const point = mapElement.value.createSVGPoint()
  point.x = event.clientX
  point.y = event.clientY
  const local = point.matrixTransform(matrix.inverse())
  dragPosition.value = { x: local.x, y: local.y }
  const target = document.elementFromPoint(event.clientX, event.clientY)?.closest('.hex-cell')
  const coord = target?.getAttribute('data-coord') || ''
  dropHover.value = draggedUnit.value.legal_moves?.includes(coord) ? coord : ''
}

function finishDrag(event: PointerEvent) {
  if (dragging.value && draggedUnit.value) {
    const target = document.elementFromPoint(event.clientX, event.clientY)?.closest('.hex-cell')
    const coord = target?.getAttribute('data-coord') || ''
    if (draggedUnit.value.legal_moves?.includes(coord)) emit('move', coord)
  }
  cancelDrag()
}

function cancelDrag() {
  window.removeEventListener('pointermove', continueDrag)
  window.removeEventListener('pointerup', finishDrag)
  window.removeEventListener('pointercancel', cancelDrag)
  draggedUnit.value = undefined
  dragging.value = false
  dropHover.value = ''
}

onUnmounted(cancelDrag)
</script>

<template>
  <svg ref="mapElement" class="hex-map" :viewBox="viewBox" role="img" aria-label="沙盒海图">
    <g v-for="hex in hexes" :key="hex.coord" class="hex-cell" :data-coord="hex.coord" :class="[`terrain-${hex.terrain}`, { legal: selectedMoves.has(hex.coord), 'drop-hover': dropHover === hex.coord }]" @click="selectedMoves.has(hex.coord) && emit('move', hex.coord)">
      <polygon :points="points(hex.x, hex.y)" />
      <text class="coord-label" :x="hex.x" :y="hex.y - 23">{{ hex.coord }}</text>
      <text v-if="hex.name" class="terrain-label" :x="hex.x" :y="hex.y + 27">{{ hex.name }}</text>
      <circle v-if="selectedMoves.has(hex.coord)" class="legal-dot" :cx="hex.x" :cy="hex.y" r="5" />
    </g>
    <g v-for="hex in hexes" :key="`units-${hex.coord}`">
      <g
        v-for="(unit, index) in unitsAt(hex.coord)"
        :key="unit.id"
        class="map-unit"
        :class="[`faction-${unit.faction.toLowerCase()}`, { selected: selectedUnit?.id === unit.id, draggable: unit.faction === playerFaction, 'drag-source': dragging && draggedUnit?.id === unit.id }]"
        :transform="`translate(${unitPosition(hex.coord, index).x}, ${unitPosition(hex.coord, index).y})`"
        @pointerdown="startDrag(unit, $event)"
        @click.stop="emit('selectUnit', unit)"
      >
        <rect x="-26" y="-18" width="52" height="36" rx="3" />
        <path class="unit-stripe" d="M-26 11h52v7h-52z" />
        <text class="unit-type" text-anchor="middle" y="4">{{ unit.type }}</text>
        <text class="unit-strength" x="20" y="-8">{{ unit.strength }}</text>
        <text class="unit-name" text-anchor="middle" y="34">{{ unit.name }}</text>
        <text v-if="orderFor(unit.id)" class="unit-order" text-anchor="middle" y="48">→ {{ orderFor(unit.id)?.coord }}</text>
      </g>
    </g>
    <g v-if="dragging && draggedUnit" class="map-unit drag-ghost" :class="`faction-${draggedUnit.faction.toLowerCase()}`" :transform="`translate(${dragPosition.x}, ${dragPosition.y})`">
      <rect x="-26" y="-18" width="52" height="36" rx="3" />
      <path class="unit-stripe" d="M-26 11h52v7h-52z" />
      <text class="unit-type" text-anchor="middle" y="4">{{ draggedUnit.type }}</text>
      <text class="unit-strength" x="20" y="-8">{{ draggedUnit.strength }}</text>
    </g>
  </svg>
</template>
