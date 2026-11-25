<script setup lang="ts">
import { computed } from 'vue'
import type { Unit } from '../types'
import ShipSilhouette from './ShipSilhouette.vue'
import { counterDamageClass, damageTitles, shipDamageMarkers } from '../shipDamage'
import { formatNumber, speedActionTurns, speedKnots } from '../format'

const props = withDefaults(defineProps<{ unit: Unit; side?: 'front' | 'back' }>(), { side: 'front' })
const markers = computed(() => shipDamageMarkers(props.unit))
const damageClasses = computed(() => counterDamageClass(props.unit))
const damageTitle = computed(() => damageTitles(props.unit).join(' / '))
const isCarrier = computed(() => ['CV', 'CVL', 'CVE'].includes(props.unit.type))
// 背面中左显示速度：档位换算成节，动作间隔写进悬停说明。
const speedNames: Record<string, string> = { fast: '快速', normal: '常速', slow: '慢速', very_slow: '极慢' }
const speedValue = computed(() => speedKnots(props.unit.card.speed_class))
const speedTitle = computed(() => {
  const name = speedNames[props.unit.card.speed_class] || '常速'
  return `${name} · ${speedValue.value} 节 · 每 ${speedActionTurns(props.unit.card.speed_class)} 回合行动一次`
})
// 兵牌中部的剪影没有文字说明，这里用一行舰种中文名标注，避免只靠外形辨认。
const shipClassLabel = computed(() => ({ CV: '航空母舰', CVL: '轻型航母', CVE: '护航航母', BB: '战列舰', BC: '战列巡洋舰', CA: '重巡洋舰', CL: '轻巡洋舰', DD: '驱逐舰', TRS: '运输船' } as Record<string, string>)[props.unit.type] || props.unit.type)
// 角标显示的是"这艘船最多能装多少架"（机库容量），不是当前在编数量。
// 当前在编数量会随起降每回合变化，兵牌上要的是这艘舰的固有容量。
const aircraftPayload = computed(() => {
  const value = props.unit.card.hangar_capacity
  if (value <= 0) return undefined
  return {
    value,
    className: isCarrier.value ? 'aircraft' : 'seaplane',
    title: isCarrier.value ? `机库容量：${value}架` : `水上飞机位：${value}架`,
  }
})
</script>

<template>
  <figure class="ship-counter" :class="[`counter-${unit.faction.toLowerCase()}`, unit.card.nation ? `counter-${unit.card.nation}` : '', damageClasses, { armored: unit.card.armored_carrier }]" :data-definition-id="unit.definition_id" :data-side="side" :data-damage="damageTitle || undefined" :title="damageTitle || undefined" :aria-label="`${unit.name}兵牌${side === 'front' ? '正面' : '背面'}${damageTitle ? '，' + damageTitle : ''}`">
    <div v-if="side === 'front'" class="counter-face counter-front">
      <header class="counter-top">
        <strong>{{ unit.type }}</strong>
        <span class="counter-radars"><i v-if="unit.card.surface_radar" class="surface-radar"></i><i v-if="unit.card.air_radar" class="air-radar"></i></span>
        <b>{{ unit.strength }}</b>
        <b v-if="unit.type === 'TRS' || unit.card.cargo" class="counter-cargo counter-cargo-strength" :title="`货物：${unit.card.cargo}`">{{ unit.card.cargo }}</b>
      </header>
      <div class="counter-middle">
        <span class="counter-dc" title="损管能力">{{ unit.card.damage_control }}</span>
        <span v-if="unit.type !== 'TRS'" class="counter-class" :title="shipClassLabel">{{ shipClassLabel }}</span>
        <!-- 损伤标记：橙条=失去动力、白条=推进器受损，垫在剪影后面，和实体卡牌一个画法 -->
        <i v-if="markers.disabled" class="counter-band counter-band-disabled" title="失去动力"></i>
        <i v-else-if="markers.slow" class="counter-band counter-band-slow" :title="markers.slowLabel"></i>
        <span class="counter-silhouette" :class="{ tilted: markers.tilt, sunk: markers.sunk }" :title="unit.name"><ShipSilhouette :type="unit.type" :name="unit.name" :picture="unit.card?.picture" /></span>
        <!-- 红圈横杠：甲板损伤/摧毁、进水倾斜 -->
        <span v-if="markers.deck || markers.tilt" class="counter-deny" :class="{ destroyed: markers.deckDestroyed }" :title="markers.deck ? (markers.deckDestroyed ? '飞行甲板摧毁' : '飞行甲板损伤') : '进水倾斜'"></span>
        <!-- 起火：烟 + 火焰 -->
        <span v-if="markers.fire" class="counter-fire" :class="{ burning: unit.damage?.fire === 'burning' }" :title="markers.fireLabel"><i class="counter-smoke"></i><i class="counter-flame"></i></span>
        <span v-if="markers.oilLeak" class="counter-oil" title="漏油"></span>
        <span v-if="aircraftPayload" class="counter-payload" :class="aircraftPayload.className" :title="aircraftPayload.title">{{ aircraftPayload.value }}</span>
      </div>
      <footer class="counter-bottom">
        <span title="重型防空 / 轻型防空">{{ formatNumber(unit.card.heavy_aa) }}-{{ formatNumber(unit.card.light_aa) }}</span>
        <i></i>
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
        <span class="counter-back-speed" :title="speedTitle">{{ speedValue }}</span>
        <span class="counter-silhouette counter-silhouette-back"><ShipSilhouette :type="unit.type" :name="unit.name" :picture="unit.card?.picture" /></span>
        <span v-if="isCarrier" class="counter-payload aircraft counter-back-deck" title="基础甲板长度：每回合可起降的航空算子数，实际按风向修正">{{ unit.card.deck_capacity }}</span>
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
