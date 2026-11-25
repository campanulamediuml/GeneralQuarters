<script setup lang="ts">
import { computed } from 'vue'
import type { Unit } from '../types'
import ShipSilhouette from './ShipSilhouette.vue'
import { shipDamageMarkers } from '../shipDamage'
import { formatNumber } from '../format'

const props = defineProps<{ unit: Unit }>()
const markers = computed(() => shipDamageMarkers(props.unit))
// 兵牌和详情卡用同一套损伤标记，避免"图标上看得见、点开卡片反而没有"。
const badgeList = computed(() => {
  const list: { label: string; kind: string }[] = []
  if (markers.value.fire) list.push({ label: markers.value.fireLabel, kind: 'fire' })
  if (markers.value.deck) list.push({ label: markers.value.deckDestroyed ? '飞行甲板摧毁' : '飞行甲板损伤', kind: 'deny' })
  if (markers.value.tilt) list.push({ label: '进水倾斜', kind: 'deny' })
  if (markers.value.disabled) list.push({ label: '失去动力', kind: 'disabled' })
  else if (markers.value.slow) list.push({ label: markers.value.slowLabel, kind: 'slow' })
  if (markers.value.oilLeak) list.push({ label: '漏油', kind: 'fire' })
  return list
})
// 雷达一栏写的是装备，不是状态；损坏以后必须在这里标出来，否则"雷达 对海/对空"看起来跟完好一样。
// 具体坏的是火控还是搜索写在"当前损伤"里，这里只负责让这一栏不再是正常色。
const radarParts = computed(() => {
  const damage = props.unit.damage
  const parts: { label: string; title: string; down: boolean }[] = []
  if (props.unit.card.surface_radar) {
    const fireControl = Boolean(damage?.fire_control_radar_disabled)
    const search = Boolean(damage?.surface_radar_disabled)
    const reasons = [fireControl ? '火控雷达损坏' : '', search ? '对海搜索雷达损坏' : ''].filter(Boolean)
    parts.push({
      label: '对海',
      title: reasons.length ? reasons.join('、') : '对海雷达可用',
      down: reasons.length > 0,
    })
  }
  if (props.unit.card.air_radar) {
    parts.push({
      label: '对空',
      title: damage?.air_radar_disabled ? '对空搜索雷达损坏' : '对空雷达可用',
      down: Boolean(damage?.air_radar_disabled),
    })
  }
  return parts
})
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
  if (damage.fire_control_radar_disabled) labels.push('火控雷达损坏')
  if (damage.surface_radar_disabled) labels.push('对海搜索雷达损坏')
  if (damage.air_radar_disabled) labels.push('对空雷达损坏')
  if (damage.sonar_disabled) labels.push('声呐损坏')
  return labels
})

function number(value: number) {
  return Number.isInteger(value) ? String(value) : formatNumber(value)
}
</script>

<template>
  <article class="ship-card" :data-definition-id="unit.definition_id">
    <header class="ship-card-head">
      <div><span class="section-kicker">SHIP CHARACTER CARD</span><h3>{{ unit.name }}</h3></div>
      <span class="ship-class">{{ unit.type }}</span>
    </header>

    <div class="ship-identity">
      <ShipSilhouette class="ship-silhouette" :type="unit.type" :name="unit.name" :picture="unit.card?.picture" />
      <div class="ship-vitals">
        <span>结构生命值</span>
        <strong>{{ unit.strength }}<small> / {{ unit.max_strength }}</small></strong>
        <div class="ship-health"><i :style="{ width: `${healthPercent}%` }"></i></div>
        <code>{{ unit.coord }} · {{ speedNames[unit.card.speed_class] }}</code>
        <div v-if="badgeList.length" class="damage-badges">
          <span v-for="badge in badgeList" :key="badge.label" class="damage-badge" :class="badge.kind">{{ badge.label }}</span>
        </div>
      </div>
    </div>

    <dl class="ship-stats">
      <div><dt>火炮能力</dt><dd>{{ unit.card.gunnery }}</dd></div>
      <div><dt>损管能力</dt><dd>DCT ≤ {{ unit.card.damage_control }}</dd></div>
      <div><dt>重型防空</dt><dd>{{ number(unit.card.heavy_aa) }}</dd></div>
      <div><dt>轻型防空</dt><dd>{{ number(unit.card.light_aa) }}</dd></div>
      <div v-if="unit.card.main_gun_count"><dt>主炮配置</dt><dd>{{ unit.card.main_gun_count }} × {{ number(unit.card.main_gun_caliber) }}″</dd></div>
      <div><dt>雷达</dt><dd><template v-if="radarParts.length"><span v-for="(part, index) in radarParts" :key="part.label" class="radar-part" :class="{ down: part.down }" :title="part.title">{{ part.label }}<template v-if="index < radarParts.length - 1"> / </template></span></template><template v-else>无</template></dd></div>
      <div v-if="unit.card.hangar_capacity"><dt>{{ isCarrier ? '舰载机 / 机库' : '水机 / 机库' }}</dt><dd>{{ unit.card.aircraft }} / {{ unit.card.hangar_capacity }}</dd></div>
      <div v-if="unit.card.torpedo_salvos"><dt>鱼雷齐射</dt><dd>{{ unit.card.torpedo_salvos }} 轮</dd></div>
      <div v-if="unit.card.depth_charges"><dt>深水炸弹</dt><dd>{{ unit.card.depth_charges }} 组</dd></div>
      <div v-if="unit.type === 'TRS' || unit.card.cargo"><dt>货物</dt><dd>{{ unit.card.cargo }}</dd></div>
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
