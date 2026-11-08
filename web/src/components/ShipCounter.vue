<script setup lang="ts">
import { computed } from 'vue'
import type { Unit } from '../types'

const props = defineProps<{ unit: Unit }>()
const isCarrier = computed(() => ['CV', 'CVL', 'CVE'].includes(props.unit.type))
const payload = computed(() => {
  if (props.unit.card.hangar_capacity) return { value: props.unit.card.hangar_capacity, className: isCarrier.value ? 'aircraft' : 'seaplane' }
  if (props.unit.card.torpedo_salvos) return { value: props.unit.card.torpedo_salvos, className: 'torpedo' }
  if (props.unit.card.cargo) return { value: props.unit.card.cargo, className: 'cargo' }
  return undefined
})
</script>

<template>
  <figure class="ship-counter" :class="[`counter-${unit.faction.toLowerCase()}`, { armored: unit.card.armored_carrier }]" :data-definition-id="unit.definition_id">
    <div class="counter-face">
      <header class="counter-top">
        <strong>{{ unit.type }}</strong>
        <span class="counter-radars"><i v-if="unit.card.surface_radar" class="surface-radar"></i><i v-if="unit.card.air_radar" class="air-radar"></i></span>
        <b>{{ unit.strength }}</b>
      </header>
      <div class="counter-middle">
        <span class="counter-dc">{{ unit.card.damage_control }}</span>
        <svg class="counter-silhouette" viewBox="0 0 120 48" aria-hidden="true">
          <path d="M2 33h116l-16 11H21z" />
          <path v-if="isCarrier" d="M8 27h104v6H4zM74 14h21v13H74zM80 8h4v6h-4z" />
          <path v-else d="M41 22h39v11H41zM50 13h19v9H50zM58 5h4v8h-4zM16 27h21v6H16zM85 27h20v6H85z" />
        </svg>
        <span v-if="payload" class="counter-payload" :class="payload.className">{{ payload.value }}</span>
      </div>
      <footer class="counter-bottom"><span>{{ unit.card.heavy_aa }}-{{ unit.card.light_aa }}</span><strong>{{ unit.card.gunnery }}</strong></footer>
    </div>
    <figcaption>{{ unit.name }}</figcaption>
  </figure>
</template>
