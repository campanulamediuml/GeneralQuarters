<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import type { AirUnit, AviationView, GameMap, ReconHeading } from '../types'
import ReconSectorMap from './ReconSectorMap.vue'

const props = defineProps<{ aviation: AviationView; gameMap: GameMap; focusUnitId?: string; busy: boolean }>()
const emit = defineEmits<{
  command: [type: 'LAUNCH_CAP' | 'LAUNCH_RECON' | 'RECOVER_AIR' | 'PRIORITIZE_AIR_TASK', aircraftID: string, options?: { heading?: ReconHeading; sector_width?: 60 | 120; recon_radius?: number }]
}>()
const carriers = computed(() => props.aviation.carriers)
const tasks = computed(() => props.aviation.tasks)
watch(() => props.focusUnitId, async (unitID) => {
  if (!unitID) return
  await nextTick()
  document.querySelector(`[data-carrier-id="${unitID}"]`)?.scrollIntoView({ block: 'center' })
}, { immediate: true, flush: 'post' })
const reconAircraft = ref<AirUnit>()
const reconHeading = ref<ReconHeading>('W')
const reconWidth = ref<60 | 120>(60)
const reconRadius = ref(1)
type TaskLane = { id: string; name: string; kind: string; slot?: number }
const lanes: TaskLane[] = [
  { id: 'recon-1', name: '空中侦察 I', kind: 'recon', slot: 0 },
  { id: 'recon-2', name: '空中侦察 II', kind: 'recon', slot: 1 },
  { id: 'strike-1', name: '打击任务 I', kind: 'attack', slot: 0 },
  { id: 'strike-2', name: '打击任务 II', kind: 'attack', slot: 1 },
  { id: 'asw', name: '反潜巡逻', kind: 'asw' },
  { id: 'cap', name: 'CAP', kind: 'cap' },
  { id: 'escort', name: '空中掩护', kind: 'escort' },
]
const kindNames = { fighter: '战斗机', dive_bomber: '俯冲轰炸机', torpedo_bomber: '鱼雷机', recon: '侦察机' }
const headings: { id: ReconHeading; arrow: string; name: string }[] = [
  { id: 'NE', arrow: '↗', name: '东北' },
  { id: 'E', arrow: '→', name: '东' },
  { id: 'SE', arrow: '↘', name: '东南' },
  { id: 'SW', arrow: '↙', name: '西南' },
  { id: 'W', arrow: '←', name: '西' },
  { id: 'NW', arrow: '↖', name: '西北' },
]

function action(aircraft: AirUnit) {
  if (aircraft.status === 'cap') emit('command', 'RECOVER_AIR', aircraft.id)
  else if (aircraft.kind === 'fighter') emit('command', 'LAUNCH_CAP', aircraft.id)
}

function openRecon(aircraft: AirUnit) {
  reconAircraft.value = aircraft
  reconHeading.value = 'W'
  reconWidth.value = 60
  reconRadius.value = aircraft.recon_radius_limit || 1
}

function launchRecon() {
  if (!reconAircraft.value) return
  emit('command', 'LAUNCH_RECON', reconAircraft.value.id, { heading: reconHeading.value, sector_width: reconWidth.value, recon_radius: reconRadius.value })
  reconAircraft.value = undefined
}

function statusName(aircraft: AirUnit) {
  if (aircraft.status === 'cap') return 'CAP'
  if (aircraft.status === 'recon') return aircraft.returning ? '返航' : aircraft.tracking_fleet_id ? '跟踪' : '侦察'
  return '机库'
}

function taskSummary(task: (typeof tasks.value)[number]) {
  if (task.kind !== 'recon') return `${task.carrier_id} · ${task.aircraft_ids.length} 算子`
  const phase = task.tracking_fleet_id ? '持续跟踪' : task.returning ? '返航搜索' : `第 ${task.recon_distance || 0} 距离圈`
  return `${task.sector_width}° ${task.heading} · 半径 ${task.recon_radius} · ${phase}`
}

function laneTasks(lane: TaskLane) {
  const matching = tasks.value.filter((task) => task.kind === lane.kind)
  if (lane.slot === undefined) return matching
  return matching.filter((_, index) => index % 2 === lane.slot)
}
</script>

<template>
  <section class="workspace-page aviation-workspace">
    <div class="aviation-title"><div><span class="section-kicker">AVIATION COMMAND</span><h1>航空指挥界面</h1></div><span>CAP / 扇面侦察</span></div>
    <p class="aviation-intro">侦察以起飞位置为顶点，沿预定航向搜索60°扇面；120°扇面的最终发现率减半。普通飞机每回合飞行2 hex，水上飞机1 hex，飞出与返航都会逐圈搜索。所有扇面、判定和情报均由服务器计算。</p>

    <div class="air-task-lanes">
      <section v-for="lane in lanes" :key="lane.id" class="air-task-lane" :data-lane-id="lane.id">
        <header><span>{{ lane.name }}</span><small>{{ laneTasks(lane).length }} 项</small></header>
        <div v-if="laneTasks(lane).length" class="air-task-list">
          <article v-for="task in laneTasks(lane)" :key="task.id" class="air-task" :class="{ prioritized: task.priority === 1, 'recon-task': task.kind === 'recon' }" :data-task-id="task.id">
            <div class="air-task-copy"><strong>{{ task.name }}</strong><small>{{ taskSummary(task) }}</small></div>
            <ReconSectorMap v-if="task.kind === 'recon'" :map="gameMap" :task="task" />
            <span class="task-priority">优先级 {{ task.priority }}</span>
            <button class="task-flag" :class="{ active: task.priority === 1 }" :aria-label="`将${task.name}设为最高优先级`" :disabled="busy" @click="emit('command', 'PRIORITIZE_AIR_TASK', task.id)">⚑</button>
          </article>
        </div>
        <p v-else>暂无任务</p>
      </section>
    </div>

    <div v-if="!carriers.length" class="aviation-placeholder"><span>△</span><h2>当前舰队没有航空作业舰</h2><p>在剧本中部署航母或搭载水上飞机的舰船。</p></div>
    <div class="carrier-air-grid">
      <section v-for="carrier in carriers" :key="carrier.unit_id" class="carrier-air-card" :class="{ focused: focusUnitId === carrier.unit_id }" :data-carrier-id="carrier.unit_id">
        <header><div><span class="unit-type-badge">{{ carrier.type }}</span><h2>{{ carrier.name }}</h2></div><div class="hangar-meter"><span>机库</span><strong>{{ carrier.hangar_used }} / {{ carrier.hangar_capacity }}</strong></div></header>
        <div class="carrier-operation"><span :class="`operation-${carrier.operation || 'idle'}`">{{ carrier.operation === 'launch' ? '本回合：起飞作业' : carrier.operation === 'recover' ? '本回合：降落作业' : carrier.deck_capacity ? '甲板待命' : '水机机库' }}</span><small v-if="carrier.deck_capacity">本回合起飞 {{ carrier.deck_used }} / {{ carrier.deck_capacity }}</small><small v-else>{{ carrier.unit_id }}</small></div>
        <div class="air-counter-grid">
          <article v-for="aircraft in carrier.aircraft" :key="aircraft.id" class="air-counter" :class="[`air-${aircraft.status}`, `kind-${aircraft.kind}`, { seaplane: aircraft.seaplane }]" :data-aircraft-id="aircraft.id" :data-status="aircraft.status">
            <header><strong>{{ aircraft.seaplane ? '水上飞机' : kindNames[aircraft.kind] }}</strong><span>{{ statusName(aircraft) }}</span></header>
            <div class="aircraft-symbol">✈</div>
            <h3>{{ aircraft.model }}</h3>
            <dl><div><dt>机体</dt><dd>{{ aircraft.strength }}</dd></div><div><dt>空战</dt><dd>{{ aircraft.air_combat }}</dd></div><div><dt>对舰</dt><dd>{{ aircraft.anti_ship }}</dd></div><div><dt>航程</dt><dd :class="{ 'low-range': aircraft.remaining_range <= 1 }">{{ aircraft.remaining_range }} / {{ aircraft.max_range }}</dd></div></dl>
            <button v-if="aircraft.status === 'cap'" :disabled="busy" @click="action(aircraft)">回收至机库</button>
            <button v-else-if="aircraft.status === 'recon'" disabled>{{ aircraft.returning ? '正在返航' : aircraft.tracking_fleet_id ? '正在跟踪目标' : '扇面搜索中' }}</button>
            <button v-else-if="aircraft.kind === 'recon'" :disabled="busy" @click="openRecon(aircraft)">扇面侦察</button>
            <button v-else-if="aircraft.kind === 'fighter'" :disabled="busy" @click="action(aircraft)">CAP 升空</button>
            <span v-else class="air-no-action">等待后续任务模块</span>
          </article>
          <p v-if="!carrier.aircraft.length" class="empty-hangar">机库空置</p>
        </div>
      </section>
    </div>

    <div v-if="reconAircraft" class="recon-dialog" @click.self="reconAircraft = undefined">
      <section class="recon-modal" role="dialog" aria-modal="true" aria-label="配置扇面侦察">
        <header><div><span class="section-kicker">SECTOR RECONNAISSANCE</span><h2>配置扇面侦察</h2></div><button aria-label="关闭" @click="reconAircraft = undefined">×</button></header>
        <p>{{ reconAircraft.model }} · 从母舰当前位置起飞。扇面中轴决定航线，飞机进入每个距离圈时由服务器进行发现判定。</p>
        <fieldset class="recon-heading-picker"><legend>预定侦察航向</legend><button v-for="heading in headings" :key="heading.id" :class="{ active: reconHeading === heading.id }" :aria-label="`${heading.id} ${heading.name}`" @click="reconHeading = heading.id"><span>{{ heading.arrow }}</span><b>{{ heading.id }}</b><small>{{ heading.name }}</small></button></fieldset>
        <div class="recon-options">
          <fieldset><legend>扇面角度</legend><div class="segmented-control"><button :class="{ active: reconWidth === 60 }" @click="reconWidth = 60">60°<small>标准发现率</small></button><button :class="{ active: reconWidth === 120 }" @click="reconWidth = 120">120°<small>发现率减半</small></button></div></fieldset>
          <label class="recon-radius">侦察半径（hex）<input v-model.number="reconRadius" type="number" min="1" :max="reconAircraft.recon_radius_limit" :aria-label="`侦察半径，最大${reconAircraft.recon_radius_limit}`" /><small>最大 {{ reconAircraft.recon_radius_limit }} hex，保留返航燃料</small></label>
        </div>
        <div class="recon-brief"><span>顶点</span><strong>母舰当前位置</strong><i>→</i><span>航线</span><strong>{{ reconHeading }}</strong><i>→</i><span>扇面</span><strong>{{ reconWidth }}° / {{ reconRadius }} hex</strong></div>
        <footer><button class="reset-button" @click="reconAircraft = undefined">取消</button><button class="apply-button" :disabled="busy" @click="launchRecon">起飞执行扇面侦察</button></footer>
      </section>
    </div>
  </section>
</template>
