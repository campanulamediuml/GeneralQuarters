<script setup lang="ts">
import { computed } from 'vue'
import { buildProfile, shipProfileSpecs } from '../shipProfiles'

const props = defineProps<{ name?: string; type?: string }>()
const spec = computed(() => (props.name ? shipProfileSpecs[props.name] : undefined))
const parts = computed(() => (spec.value ? buildProfile(spec.value) : undefined))
</script>

<template>
  <svg v-if="parts" class="ship-profile" viewBox="0 0 620 150" preserveAspectRatio="xMidYMid meet" aria-hidden="true">
    <path class="sp-hull" :d="parts.hull" />
    <path class="sp-boot" :d="parts.boot" />
    <path v-if="parts.deck" class="sp-deck" :d="parts.deck" />
    <path v-for="(d, i) in parts.superstructure" :key="`s${i}`" class="sp-super" :d="d" />
    <path v-for="(d, i) in parts.funnels" :key="`f${i}`" class="sp-funnel" :d="d" />
    <path v-for="(d, i) in parts.masts" :key="`m${i}`" class="sp-mast" :d="d" />
    <path v-for="(d, i) in parts.turrets" :key="`t${i}`" class="sp-turret" :d="d" />
    <path v-for="(d, i) in parts.extras" :key="`e${i}`" class="sp-extra" :d="d" />
  </svg>
</template>

<style>
.ship-profile{display:block;width:100%;height:auto;background:linear-gradient(180deg,#0d1c22,#0a161b)}
.ship-profile .sp-hull{fill:#9aa5ac}
.ship-profile .sp-boot{fill:#7c3b34}
.ship-profile .sp-deck{fill:#8d979d}
.ship-profile .sp-super{fill:#b8c2c8}
.ship-profile .sp-funnel{fill:#6f7a80}
.ship-profile .sp-mast{fill:none;stroke:#8b959b;stroke-width:1.6}
.ship-profile .sp-turret{fill:#a7b2b8}
.ship-profile .sp-extra{fill:#78838a}
</style>
