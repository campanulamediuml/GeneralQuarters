<script setup lang="ts">
import { computed } from 'vue'
import type { Unit } from '../types'

const props = withDefaults(defineProps<{ unit: Unit; side?: 'front' | 'back' }>(), { side: 'front' })
const isCarrier = computed(() => ['CV', 'CVL', 'CVE'].includes(props.unit.type))
const aircraftPayload = computed(() => {
  const value = props.unit.card.aircraft
  if (value <= 0) return undefined
  return {
    value,
    className: isCarrier.value ? 'aircraft' : 'seaplane',
    title: isCarrier.value ? `当前舰载机：${value}个算子` : `当前水上飞机：${value}个算子`,
  }
})
</script>

<template>
  <figure class="ship-counter" :class="[`counter-${unit.faction.toLowerCase()}`, { armored: unit.card.armored_carrier }]" :data-definition-id="unit.definition_id" :data-side="side" :aria-label="`${unit.name}兵牌${side === 'front' ? '正面' : '背面'}`">
    <div v-if="side === 'front'" class="counter-face counter-front">
      <header class="counter-top">
        <strong>{{ unit.type }}</strong>
        <span class="counter-radars"><i v-if="unit.card.surface_radar" class="surface-radar"></i><i v-if="unit.card.air_radar" class="air-radar"></i></span>
        <b>{{ unit.strength }}</b>
      </header>
      <div class="counter-middle">
        <span class="counter-dc" title="损管能力">{{ unit.card.damage_control }}</span>
        <svg class="counter-silhouette" viewBox="0 0 120 48" aria-hidden="true">
          <path d="M2 33h116l-16 11H21z" />
          <path v-if="isCarrier" d="M8 27h104v6H4zM74 14h21v13H74zM80 8h4v6h-4z" />
          <path v-else d="M41 22h39v11H41zM50 13h19v9H50zM58 5h4v8h-4zM16 27h21v6H16zM85 27h20v6H85z" />
        </svg>
        <span v-if="aircraftPayload" class="counter-payload" :class="aircraftPayload.className" :title="aircraftPayload.title">{{ aircraftPayload.value }}</span>
      </div>
      <footer class="counter-bottom">
        <span title="重型防空 / 轻型防空">{{ unit.card.heavy_aa }}-{{ unit.card.light_aa }}</span>
        <b v-if="unit.card.cargo" class="counter-cargo" :title="`货物：${unit.card.cargo}`">{{ unit.card.cargo }}</b>
        <i v-else></i>
        <strong title="火炮能力">{{ unit.card.gunnery }}</strong>
      </footer>
    </div>

    <div v-else class="counter-face counter-back">
      <header class="counter-top counter-back-top">
        <span></span>
        <small class="counter-back-history" :title="unit.card.historical_loss ? `历史沉没：${unit.card.historical_loss}` : '历史沉没时间：无记录'">{{ unit.card.historical_loss || '—' }}</small>
        <b v-if="unit.card.main_gun_count" class="counter-back-main-guns" title="主炮数量">{{ unit.card.main_gun_count }}</b>
        <b v-else></b>
      </header>
      <div class="counter-middle">
        <span class="counter-dc" title="损管能力">{{ unit.card.damage_control }}</span>
        <svg class="counter-silhouette" viewBox="0 0 120 48" aria-hidden="true">
          <path d="M2 33h116l-16 11H21z" />
          <path v-if="isCarrier" d="M8 27h104v6H4zM74 14h21v13H74zM80 8h4v6h-4z" />
          <path v-else d="M41 22h39v11H41zM50 13h19v9H50zM58 5h4v8h-4zM16 27h21v6H16zM85 27h20v6H85z" />
        </svg>
        <span v-if="isCarrier" class="counter-payload aircraft counter-back-deck" title="甲板长度 / 每回合航空算子容量">{{ unit.card.deck_capacity }}</span>
      </div>
      <footer class="counter-bottom counter-back-bottom">
        <span v-if="unit.card.torpedo_salvos" class="counter-back-torpedo" :title="`鱼雷齐射：${unit.card.torpedo_salvos}轮，每轮4枚`">{{ unit.card.torpedo_salvos }}</span>
        <span v-else></span>
        <strong v-if="isCarrier" class="counter-back-elevator" title="升降机数量">{{ unit.card.elevator_count }}</strong>
        <span v-else></span>
        <b v-if="unit.card.depth_charges" class="counter-back-depth" :title="`深水炸弹：${unit.card.depth_charges}组`">{{ unit.card.depth_charges }}</b>
        <span v-else></span>
      </footer>
    </div>
    <figcaption>{{ unit.name }}</figcaption>
  </figure>
</template>
