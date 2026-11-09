<script setup lang="ts">
import { computed } from 'vue'
import type { AirUnit, Player } from '../types'

const props = defineProps<{ aircraft: AirUnit; playerFaction: Player['faction']; stackCount?: number }>()
const friendly = computed(() => props.aircraft.faction === props.playerFaction)
const range = computed(() => friendly.value ? props.aircraft.remaining_range : props.aircraft.max_range)
const modelCode = computed(() => props.aircraft.model.split(' ')[0])
</script>

<template>
  <figure
    class="air-counter"
    :class="[`air-${aircraft.status}`, `kind-${aircraft.kind}`, { seaplane: aircraft.seaplane }]"
    :data-range-kind="friendly ? 'remaining' : 'maximum'"
    :data-range-value="range"
    :data-stack-count="stackCount || 1"
    :aria-label="`${aircraft.model}${stackCount && stackCount > 1 ? `，${stackCount}个算子` : ''}，对空${aircraft.air_combat}，对地${aircraft.anti_ground}，对舰${aircraft.anti_ship}，机体${aircraft.strength}，${friendly ? '剩余' : '最大'}航程${range}`"
  >
    <div class="aircraft-ratings">
      <span title="对空能力">{{ aircraft.air_combat }}</span>
      <span title="对地能力">{{ aircraft.anti_ground }}</span>
      <span title="对舰攻击">{{ aircraft.anti_ship }}</span>
    </div>
    <div class="aircraft-body">
      <span class="aircraft-strength" title="机体强度">{{ aircraft.strength }}</span>
      <svg class="aircraft-symbol" viewBox="0 0 64 44" aria-hidden="true">
        <path d="M29 3h6l2 14 23 12v5l-23-6-1 8 8 4v3l-12-2-12 2v-3l8-4-1-8-23 6v-5l23-12z" />
        <path v-if="aircraft.seaplane" d="M15 36h34M20 32l-3 5m27-5 3 5" />
      </svg>
      <span class="aircraft-loadout" title="挂载：未配置">—</span>
    </div>
    <div class="aircraft-footer">
      <strong
        class="aircraft-range"
        :class="{ 'carrier-based': !aircraft.seaplane, 'low-range': friendly && aircraft.remaining_range <= 1 }"
        :title="friendly ? '剩余航程' : '此挂载下最大航程'"
      >{{ range }}</strong>
      <span class="aircraft-model" :title="aircraft.model">{{ modelCode }}</span>
    </div>
    <span v-if="stackCount && stackCount > 1" class="aircraft-stack-count" :title="`${stackCount}个同型算子`">×{{ stackCount }}</span>
  </figure>
</template>

<style scoped>
.air-counter{position:relative;display:grid;grid-template-rows:18px 43px 22px;width:84px;height:85px;margin:0;overflow:hidden;border:1px solid #89aab5;border-radius:2px;background:linear-gradient(145deg,#d6e2e2,#9db6bd);color:#132a33;box-shadow:0 2px 4px #0005}
.air-counter.seaplane{background:linear-gradient(145deg,#edba73,#bf7d38);border-color:#e5b675}
.aircraft-ratings{display:grid;grid-template-columns:repeat(3,1fr);align-items:center;padding:0 4px;font:700 13px/1 'Rajdhani',sans-serif;text-align:center}
.aircraft-body{display:grid;grid-template-columns:14px minmax(0,1fr) 14px;align-items:center;min-height:0}
.aircraft-strength{align-self:start;padding-top:3px;font:700 12px/1 'Rajdhani',sans-serif;text-align:center}
.aircraft-symbol{width:100%;height:40px;color:#245b72;transform:none;fill:currentColor;stroke:currentColor;stroke-width:1}
.seaplane .aircraft-symbol{color:#70441f}
.aircraft-loadout{align-self:start;padding-top:3px;color:#42565b;font:10px/1 sans-serif;text-align:center}
.aircraft-footer{display:grid;grid-template-columns:25px minmax(0,1fr);align-items:end;gap:3px;padding:0 4px 3px}
.aircraft-range{display:grid;place-items:center;width:25px;height:20px;background:#426b7b;color:#eef5f3;font:700 14px/1 'Rajdhani',sans-serif}
.aircraft-range.carrier-based{background:#187caf}
.aircraft-range.low-range{background:#9c4036}
.aircraft-model{overflow:hidden;padding-bottom:2px;font:700 9px/1 'Rajdhani',sans-serif;text-align:right;text-overflow:ellipsis;white-space:nowrap}
.air-counter.air-cap,.air-counter.air-recon,.air-counter.air-deck,.air-counter.air-ready,.air-counter.air-attack,.air-counter.air-attack_holding{border-color:#f0d58d}.air-counter.air-service,.air-counter.air-elevator{border-style:dashed;border-color:#d7bb7b}.air-counter.air-recovery{border-color:#7ec8db}
.aircraft-stack-count{position:absolute;right:3px;bottom:27px;z-index:2;min-width:25px;padding:3px 4px;border:1px solid #f0d58d;background:#142a31e8;color:#f0d58d;font:700 13px/1 'Rajdhani',sans-serif;text-align:center;box-shadow:0 1px 3px #0007}
</style>
