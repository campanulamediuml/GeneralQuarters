<script setup lang="ts">
import { computed } from 'vue'
import type { AirTask, GameMap } from '../types'

const props = defineProps<{ map: GameMap; task: AirTask }>()
const size = 15
const sector = computed(() => new Set(props.task.sector_hexes || []))
const currentRing = computed(() => new Set(props.task.current_ring_hexes || []))
const hexes = computed(() => {
  const result = []
  for (let row = 1; row <= props.map.rows; row++) {
    for (let col = 1; col <= props.map.cols; col++) {
      const coord = `${String(row).padStart(2, '0')}${String(col).padStart(2, '0')}`
      result.push({ coord, ...center(coord) })
    }
  }
  return result
})
const viewBox = computed(() => `0 0 ${Math.sqrt(3) * size * (props.map.cols + 1)} ${size * (1.5 * props.map.rows + 0.7)}`)

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
</script>

<template>
  <svg
    class="recon-sector-map"
    :viewBox="viewBox"
    role="img"
    :aria-label="`${task.sector_width}度${task.heading}扇面，当前第${task.recon_distance || 0}距离圈`"
    :data-sector-width="task.sector_width"
    :data-current-distance="task.recon_distance || 0"
    :data-sector-cells="task.sector_hexes?.length || 0"
  >
    <g v-for="hex in hexes" :key="hex.coord" :data-coord="hex.coord">
      <polygon :points="points(hex.x, hex.y)" :class="{ sector: sector.has(hex.coord), 'current-ring': currentRing.has(hex.coord) }" />
    </g>
    <circle v-if="task.origin_coord" class="recon-origin" :cx="center(task.origin_coord).x" :cy="center(task.origin_coord).y" r="3" />
    <g v-if="task.current_coord" class="recon-aircraft-marker" :transform="`translate(${center(task.current_coord).x}, ${center(task.current_coord).y})`">
      <circle r="6" />
      <path d="M-4 0h8M0-4v8" />
    </g>
  </svg>
</template>
