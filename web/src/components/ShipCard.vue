<script setup lang="ts">
import { computed } from 'vue'
import type { Unit } from '../types'

const props = defineProps<{ unit: Unit }>()
const isCarrier = computed(() => ['CV', 'CVL', 'CVE'].includes(props.unit.type))
const healthPercent = computed(() => Math.max(0, Math.min(100, props.unit.strength / props.unit.max_strength * 100)))
const speedNames = { normal: '常速', fast: '快速', slow: '慢速', very_slow: '极慢' }
const damageLabels = computed(() => {
  const damage = props.unit.damage
  if (!damage) return []
  const labels: string[] = []
  if (damage.fire === 'small') labels.push('小型火灾')
  if (damage.fire === 'burning') labels.push('起火燃烧')
  if (damage.flooding?.length) labels.push(`进水 ${damage.flooding.length} 层`)
  if (damage.disabled) labels.push('失去动力')
  if (damage.tilt) labels.push('倾斜')
  if (damage.oil_leak) labels.push('漏油')
  if (damage.deck_destroyed) labels.push('飞行甲板摧毁')
  else if (damage.deck_damaged) labels.push('飞行甲板损伤')
  if (damage.elevator_damage) labels.push(`升降机损坏 ${damage.elevator_damage} 部`)
  if (damage.torpedo_disabled) labels.push('鱼雷发射器毁坏')
  if (damage.surface_radar_disabled) labels.push('对海雷达损坏')
  if (damage.air_radar_disabled) labels.push('对空雷达损坏')
  if (damage.sonar_disabled) labels.push('声呐损坏')
  return labels
})

function number(value: number) {
  return Number.isInteger(value) ? String(value) : value.toFixed(1)
}
</script>

<template>
  <article class="ship-card" :data-definition-id="unit.definition_id">
    <header class="ship-card-head">
      <div><span class="section-kicker">SHIP CHARACTER CARD</span><h3>{{ unit.name }}</h3></div>
      <span class="ship-class">{{ unit.type }}</span>
    </header>

    <div class="ship-identity">
      <svg class="ship-silhouette" viewBox="0 0 120 64" aria-hidden="true">
        <path class="ship-hull" d="M4 43h112l-17 13H23z" />
        <template v-if="isCarrier">
          <path class="ship-structure" d="M9 36h103v7H4zM75 21h21v15H75zM80 15h4v6h-4z" />
        </template>
        <template v-else>
          <path class="ship-structure" d="M43 30h35v13H43zM51 20h18v10H51zM58 11h4v9h-4zM18 36h20v7H18zM83 36h20v7H83z" />
        </template>
      </svg>
      <div class="ship-vitals">
        <span>结构生命值</span>
        <strong>{{ unit.strength }}<small> / {{ unit.max_strength }}</small></strong>
        <div class="ship-health"><i :style="{ width: `${healthPercent}%` }"></i></div>
        <code>{{ unit.coord }} · {{ speedNames[unit.card.speed_class] }}</code>
      </div>
    </div>

    <dl class="ship-stats">
      <div><dt>火炮能力</dt><dd>{{ unit.card.gunnery }}</dd></div>
      <div><dt>损管能力</dt><dd>DCT ≤ {{ unit.card.damage_control }}</dd></div>
      <div><dt>重型防空</dt><dd>{{ number(unit.card.heavy_aa) }}</dd></div>
      <div><dt>轻型防空</dt><dd>{{ number(unit.card.light_aa) }}</dd></div>
      <div v-if="unit.card.main_gun_count"><dt>主炮配置</dt><dd>{{ unit.card.main_gun_count }} × {{ number(unit.card.main_gun_caliber) }}″</dd></div>
      <div><dt>雷达</dt><dd>{{ unit.card.surface_radar ? '对海' : '' }}{{ unit.card.surface_radar && unit.card.air_radar ? ' / ' : '' }}{{ unit.card.air_radar ? '对空' : '' }}{{ !unit.card.surface_radar && !unit.card.air_radar ? '无' : '' }}</dd></div>
      <div v-if="unit.card.hangar_capacity"><dt>{{ isCarrier ? '舰载机 / 机库' : '水机 / 机库' }}</dt><dd>{{ unit.card.aircraft }} / {{ unit.card.hangar_capacity }}</dd></div>
      <div v-if="unit.card.torpedo_salvos"><dt>鱼雷齐射</dt><dd>{{ unit.card.torpedo_salvos }} 轮</dd></div>
      <div v-if="unit.card.depth_charges"><dt>深水炸弹</dt><dd>{{ unit.card.depth_charges }} 组</dd></div>
      <div v-if="unit.card.cargo"><dt>货物</dt><dd>{{ unit.card.cargo }}</dd></div>
    </dl>

    <section v-if="damageLabels.length" class="ship-damage-report"><strong>当前损伤</strong><div><span v-for="label in damageLabels" :key="label">{{ label }}</span></div></section>

    <div v-if="isCarrier" class="carrier-systems">
      <span>航空作业</span>
      <div><b>甲板容量</b><strong>{{ unit.card.deck_capacity }}</strong></div>
      <div><b>升降机</b><strong>{{ unit.card.elevator_count }}</strong></div>
      <div><b>装甲航母</b><strong>{{ unit.card.armored_carrier ? '是' : '否' }}</strong></div>
    </div>
    <p v-if="unit.card.historical_loss" class="ship-history">历史沉没：{{ unit.card.historical_loss }}</p>
    <footer><span>沙盒通用舰型 · 测试参数</span><code>{{ unit.definition_id }}</code></footer>
  </article>
</template>
