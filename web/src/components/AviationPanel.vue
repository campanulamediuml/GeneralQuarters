<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { AirUnit, AviationView, CarrierAirStatus, GameMap, ReconHeading } from '../types'
import ReconSectorMap from './ReconSectorMap.vue'

type AirCommandType = 'LAUNCH_CAP' | 'LAUNCH_RECON' | 'PREPARE_AIR' | 'RETURN_TO_HANGAR' | 'RECOVER_AIR' | 'PRIORITIZE_AIR_TASK'
type TaskLane = { id: string; name: string; kind: string; slot?: number }

const props = defineProps<{ aviation: AviationView; gameMap: GameMap; focusUnitId?: string; selectedAircraftId?: string; busy: boolean }>()
const emit = defineEmits<{
  command: [type: AirCommandType, aircraftIDs: string[], options?: { heading?: ReconHeading; sector_width?: 60 | 120; recon_radius?: number }]
  selectAircraft: [aircraftID: string]
}>()
const carriers = computed(() => props.aviation.carriers)
const tasks = computed(() => props.aviation.tasks)
const selectedCarrierID = ref('')
const selectionMode = ref<'single' | 'type'>('single')
const selectedIDs = ref<string[]>([])
const draggingIDs = ref<string[]>([])
const dragOver = ref('')
const selectionHint = ref('')
const reconAircraftIDs = ref<string[]>([])
const reconHeading = ref<ReconHeading>('W')
const reconWidth = ref<60 | 120>(60)
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
const headingRotation: Record<ReconHeading, number> = { E: 0, SE: 60, SW: 120, W: 180, NW: 240, NE: 300 }
const selectedCarrier = computed(() => carriers.value.find((carrier) => carrier.unit_id === selectedCarrierID.value))
const reconAircraft = computed(() => {
  for (const carrier of carriers.value) {
    const aircraft = carrier.aircraft.find((item) => item.id === reconAircraftIDs.value[0])
    if (aircraft) return aircraft
  }
  return undefined
})

watch(carriers, (items) => {
  if (!items.some((item) => item.unit_id === selectedCarrierID.value)) selectedCarrierID.value = props.focusUnitId || items[0]?.unit_id || ''
}, { immediate: true })
watch(() => props.focusUnitId, (unitID) => {
  if (unitID && carriers.value.some((item) => item.unit_id === unitID)) {
    selectedCarrierID.value = unitID
    selectedIDs.value = []
  }
})
watch(selectedCarrier, (carrier) => {
  const valid = new Set(carrier?.aircraft.map((aircraft) => aircraft.id) || [])
  selectedIDs.value = selectedIDs.value.filter((id) => valid.has(id))
})

function selectCarrier(carrier: CarrierAirStatus) {
  selectedCarrierID.value = carrier.unit_id
  selectedIDs.value = []
  selectionHint.value = ''
}

function selectCounter(aircraft: AirUnit) {
  if (aircraft.status !== 'hangar' && aircraft.status !== 'deck') {
    emit('selectAircraft', aircraft.id)
    return
  }
  selectionHint.value = ''
  const carrier = selectedCarrier.value
  if (!carrier) return
  const group = selectionMode.value === 'type'
    ? carrier.aircraft.filter((item) => item.definition_id === aircraft.definition_id && item.status === aircraft.status).map((item) => item.id)
    : [aircraft.id]
  const allSelected = group.every((id) => selectedIDs.value.includes(id))
  if (selectionMode.value === 'single') {
    selectedIDs.value = allSelected ? [] : group
  } else if (allSelected) {
    selectedIDs.value = selectedIDs.value.filter((id) => !group.includes(id))
  } else {
    selectedIDs.value = [...new Set([...selectedIDs.value, ...group])]
  }
}

function startDrag(aircraft: AirUnit, event: DragEvent) {
  if (!selectedIDs.value.includes(aircraft.id)) selectedIDs.value = [aircraft.id]
  draggingIDs.value = [...selectedIDs.value]
  if (event.dataTransfer && event.currentTarget instanceof HTMLElement) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', draggingIDs.value.join(','))
    event.dataTransfer.setDragImage(event.currentTarget, 40, 40)
  }
}

function endDrag() {
  draggingIDs.value = []
  dragOver.value = ''
}

function assignTarget(target: string) {
  const ids = [...(draggingIDs.value.length ? draggingIDs.value : selectedIDs.value)]
  endDrag()
  if (!ids.length) {
    selectionHint.value = '先选择机库或甲板上的飞机算子，再点击或拖入目标区域'
    return
  }
  selectionHint.value = ''
  if (target === 'deck') {
    emitBatch('PREPARE_AIR', ids)
  } else if (target === 'hangar') {
    emitBatch('RETURN_TO_HANGAR', ids)
  } else if (target === 'cap') {
    emitBatch('LAUNCH_CAP', ids)
  } else if (target.startsWith('recon-')) {
    reconAircraftIDs.value = ids
    reconHeading.value = 'W'
    reconWidth.value = 60
  } else {
    selectionHint.value = '该任务类型尚未接入结算，目前可分配到甲板、CAP或扇面侦察'
  }
}

function emitBatch(type: AirCommandType, ids: string[]) {
  emit('command', type, ids)
  selectedIDs.value = []
}

function action(aircraft: AirUnit) {
  if (aircraft.status === 'cap') emit('command', 'RECOVER_AIR', [aircraft.id])
  else if (aircraft.kind === 'fighter') emitBatch('LAUNCH_CAP', [aircraft.id])
}

function openRecon(aircraft: AirUnit) {
  reconAircraftIDs.value = [aircraft.id]
  reconHeading.value = 'W'
  reconWidth.value = 60
}

function launchRecon() {
  if (!reconAircraftIDs.value.length) return
  emit('command', 'LAUNCH_RECON', reconAircraftIDs.value, { heading: reconHeading.value, sector_width: reconWidth.value })
  reconAircraftIDs.value = []
  selectedIDs.value = []
}

function statusName(aircraft: AirUnit) {
  if (aircraft.status === 'cap') return 'CAP'
  if (aircraft.status === 'deck') return '甲板'
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

function aircraftAt(status: 'hangar' | 'deck' | 'airborne') {
  const aircraft = selectedCarrier.value?.aircraft || []
  if (status === 'airborne') return aircraft.filter((item) => item.status === 'cap' || item.status === 'recon')
  return aircraft.filter((item) => item.status === status)
}

function operationName(carrier: CarrierAirStatus) {
  if (carrier.operation === 'prepare') return '本回合：提前整备'
  if (carrier.operation === 'launch') return '本回合：起飞作业'
  if (carrier.operation === 'recover') return '本回合：降落作业'
  return carrier.deck_capacity ? '甲板待命' : '水机机库'
}
</script>

<template>
  <section class="workspace-page aviation-workspace">
    <div class="aviation-title"><div><span class="section-kicker">AVIATION COMMAND</span><h1>航空指挥界面</h1></div><span>拖拽分配 / 批量编组</span></div>
    <p class="aviation-intro">选择一艘作业舰，将机库算子拖入飞行甲板提前整备，或拖入CAP、侦察任务栏直接起飞。也可先选中算子，再点击目标栏。所有批量命令由服务端一次结算。</p>

    <div class="air-task-lanes">
      <section v-for="lane in lanes" :key="lane.id" class="air-task-lane" :class="{ 'drop-active': dragOver === lane.id }" :data-lane-id="lane.id" @click="assignTarget(lane.id)" @dragover.prevent="dragOver = lane.id" @dragleave="dragOver = ''" @drop.prevent="assignTarget(lane.id)">
        <header><span>{{ lane.name }}</span><small>{{ laneTasks(lane).length }} 项</small></header>
        <div v-if="laneTasks(lane).length" class="air-task-list">
          <article v-for="task in laneTasks(lane)" :key="task.id" class="air-task" :class="{ prioritized: task.priority === 1, 'recon-task': task.kind === 'recon' }" :data-task-id="task.id" @click.stop="emit('selectAircraft', task.aircraft_ids[0])">
            <div class="air-task-copy"><strong>{{ task.name }}</strong><small>{{ taskSummary(task) }}</small></div>
            <ReconSectorMap v-if="task.kind === 'recon'" :map="gameMap" :task="task" />
            <span class="task-priority">优先级 {{ task.priority }}</span>
            <button class="task-flag" :class="{ active: task.priority === 1 }" :aria-label="`将${task.name}设为最高优先级`" :disabled="busy" @click.stop="emit('command', 'PRIORITIZE_AIR_TASK', [task.id])">⚑</button>
          </article>
        </div>
        <p v-else>拖入或点击分配任务</p>
      </section>
    </div>

    <div v-if="!carriers.length" class="aviation-placeholder"><span>△</span><h2>当前舰队没有航空作业舰</h2><p>在剧本中部署航母或搭载水上飞机的舰船。</p></div>
    <div v-else class="aviation-operations">
      <aside class="carrier-roster">
        <header><span class="section-kicker">AIR CAPABLE SHIPS</span><b>{{ carriers.length }}艘</b></header>
        <button v-for="carrier in carriers" :key="carrier.unit_id" :class="{ active: selectedCarrierID === carrier.unit_id }" :data-roster-id="carrier.unit_id" @click="selectCarrier(carrier)">
          <span class="unit-type-badge">{{ carrier.type }}</span><div><strong>{{ carrier.name }}</strong><small>机库 {{ carrier.hangar_used }} · 甲板 {{ carrier.deck_used }}</small></div><b>{{ carrier.hangar_used + carrier.deck_used }}</b>
        </button>
      </aside>

      <section v-if="selectedCarrier" class="carrier-air-card" :class="{ focused: focusUnitId === selectedCarrier.unit_id }" :data-carrier-id="selectedCarrier.unit_id">
        <header><div><span class="unit-type-badge">{{ selectedCarrier.type }}</span><h2>{{ selectedCarrier.name }}</h2></div><div class="hangar-meter"><span>机库</span><strong>{{ selectedCarrier.hangar_used }} / {{ selectedCarrier.hangar_capacity }}</strong></div></header>
        <div class="carrier-operation"><span :class="`operation-${selectedCarrier.operation || 'idle'}`">{{ operationName(selectedCarrier) }}</span><small v-if="selectedCarrier.deck_capacity">本回合起飞 {{ selectedCarrier.launch_used }} / {{ selectedCarrier.deck_capacity }}</small><small v-else>{{ selectedCarrier.unit_id }}</small></div>
        <div class="air-selection-toolbar">
          <span>选择模式</span><button :class="{ active: selectionMode === 'single' }" @click="selectionMode = 'single'; selectedIDs = []">单选</button><button :class="{ active: selectionMode === 'type' }" @click="selectionMode = 'type'; selectedIDs = []">同型全选</button><small>已选 {{ selectedIDs.length }} 个算子</small><button class="clear-selection" @click="selectedIDs = []">清除</button>
        </div>
        <p v-if="selectionHint" class="air-selection-hint" role="alert">{{ selectionHint }}</p>

        <section class="air-storage-zone flight-deck-zone" :class="{ 'drop-active': dragOver === 'deck' }" data-drop-target="deck" @dragover.prevent="dragOver = 'deck'" @dragleave="dragOver = ''" @drop.prevent="assignTarget('deck')">
          <header><div><span class="section-kicker">FLIGHT DECK</span><h3>飞行甲板</h3></div><strong>{{ selectedCarrier.deck_used }} / {{ selectedCarrier.deck_capacity }}</strong></header>
          <p>拖入机库算子提前整备；点击甲板算子可继续分配任务。</p>
          <div class="air-counter-grid">
            <article v-for="aircraft in aircraftAt('deck')" :key="aircraft.id" class="air-counter" :class="[`air-${aircraft.status}`, `kind-${aircraft.kind}`, { seaplane: aircraft.seaplane, selected: selectedIDs.includes(aircraft.id) || selectedAircraftId === aircraft.id, selectable: true }]" :data-aircraft-id="aircraft.id" :data-status="aircraft.status" draggable="true" @click="selectCounter(aircraft)" @dragstart="startDrag(aircraft, $event)" @dragend="endDrag">
              <header><strong>{{ aircraft.seaplane ? '水上飞机' : kindNames[aircraft.kind] }}</strong><span>{{ statusName(aircraft) }}</span></header><div class="aircraft-symbol">✈</div><h3>{{ aircraft.model }}</h3>
              <dl><div><dt>机体</dt><dd>{{ aircraft.strength }}</dd></div><div><dt>空战</dt><dd>{{ aircraft.air_combat }}</dd></div><div><dt>对舰</dt><dd>{{ aircraft.anti_ship }}</dd></div><div><dt>航程</dt><dd>{{ aircraft.remaining_range }} / {{ aircraft.max_range }}</dd></div></dl>
              <button v-if="aircraft.kind === 'fighter'" :disabled="busy" @click.stop="action(aircraft)">CAP 起飞</button><button v-else-if="aircraft.kind === 'recon'" :disabled="busy" @click.stop="openRecon(aircraft)">扇面侦察</button><button v-else class="air-no-action" disabled>等待打击模块</button>
            </article>
            <button v-if="selectedCarrier.deck_used" class="return-hangar-button" @click="assignTarget('hangar')">将已选甲板算子拉回机库</button>
            <span v-else class="empty-air-zone">甲板空置</span>
          </div>
        </section>

        <section class="air-storage-zone hangar-zone" :class="{ 'drop-active': dragOver === 'hangar' }" data-drop-target="hangar" @click="assignTarget('hangar')" @dragover.prevent="dragOver = 'hangar'" @dragleave="dragOver = ''" @drop.prevent="assignTarget('hangar')">
          <header><div><span class="section-kicker">HANGAR</span><h3>机库</h3></div><strong>{{ selectedCarrier.hangar_used }} / {{ selectedCarrier.hangar_capacity }}</strong></header>
          <div class="air-counter-grid">
            <article v-for="aircraft in aircraftAt('hangar')" :key="aircraft.id" class="air-counter" :class="[`air-${aircraft.status}`, `kind-${aircraft.kind}`, { seaplane: aircraft.seaplane, selected: selectedIDs.includes(aircraft.id) || selectedAircraftId === aircraft.id, selectable: true }]" :data-aircraft-id="aircraft.id" :data-status="aircraft.status" draggable="true" @click.stop="selectCounter(aircraft)" @dragstart="startDrag(aircraft, $event)" @dragend="endDrag">
              <header><strong>{{ aircraft.seaplane ? '水上飞机' : kindNames[aircraft.kind] }}</strong><span>{{ statusName(aircraft) }}</span></header><div class="aircraft-symbol">✈</div><h3>{{ aircraft.model }}</h3>
              <dl><div><dt>机体</dt><dd>{{ aircraft.strength }}</dd></div><div><dt>空战</dt><dd>{{ aircraft.air_combat }}</dd></div><div><dt>对舰</dt><dd>{{ aircraft.anti_ship }}</dd></div><div><dt>航程</dt><dd>{{ aircraft.remaining_range }} / {{ aircraft.max_range }}</dd></div></dl>
              <button v-if="aircraft.kind === 'fighter'" :disabled="busy" @click.stop="action(aircraft)">CAP 升空</button><button v-else-if="aircraft.kind === 'recon'" :disabled="busy" @click.stop="openRecon(aircraft)">扇面侦察</button><span v-else class="air-no-action">等待后续任务模块</span>
            </article>
            <p v-if="!aircraftAt('hangar').length" class="empty-hangar">机库空置</p>
          </div>
        </section>

        <section v-if="aircraftAt('airborne').length" class="air-storage-zone airborne-zone">
          <header><div><span class="section-kicker">AIRBORNE</span><h3>空中算子</h3></div><strong>{{ aircraftAt('airborne').length }}</strong></header>
          <div class="air-counter-grid">
            <article v-for="aircraft in aircraftAt('airborne')" :key="aircraft.id" class="air-counter" :class="[`air-${aircraft.status}`, `kind-${aircraft.kind}`, { seaplane: aircraft.seaplane, selected: selectedAircraftId === aircraft.id, selectable: true }]" :data-aircraft-id="aircraft.id" :data-status="aircraft.status" @click="selectCounter(aircraft)">
              <header><strong>{{ aircraft.seaplane ? '水上飞机' : kindNames[aircraft.kind] }}</strong><span>{{ statusName(aircraft) }}</span></header><div class="aircraft-symbol">✈</div><h3>{{ aircraft.model }}</h3>
              <dl><div><dt>机体</dt><dd>{{ aircraft.strength }}</dd></div><div><dt>空战</dt><dd>{{ aircraft.air_combat }}</dd></div><div><dt>对舰</dt><dd>{{ aircraft.anti_ship }}</dd></div><div><dt>航程</dt><dd :class="{ 'low-range': aircraft.remaining_range <= 1 }">{{ aircraft.remaining_range }} / {{ aircraft.max_range }}</dd></div></dl>
              <button v-if="aircraft.status === 'cap'" :disabled="busy" @click.stop="action(aircraft)">回收至机库</button><button v-else disabled @click.stop>{{ aircraft.returning ? '正在返航' : aircraft.tracking_fleet_id ? '正在跟踪目标' : '扇面搜索中' }}</button>
            </article>
          </div>
        </section>
      </section>
    </div>

    <div v-if="reconAircraft" class="recon-dialog" @click.self="reconAircraftIDs = []">
      <section class="recon-modal" role="dialog" aria-modal="true" aria-label="配置扇面侦察">
        <header><div><span class="section-kicker">SECTOR RECONNAISSANCE</span><h2>配置扇面侦察</h2></div><button aria-label="关闭" @click="reconAircraftIDs = []">×</button></header>
        <p>{{ reconAircraft.model }} · {{ reconAircraftIDs.length }}个算子从母舰当前位置起飞。扇面中轴决定航线，飞机进入每个距离圈时由服务器进行发现判定。</p>
        <fieldset class="recon-heading-picker"><legend>预定侦察航向</legend><button v-for="heading in headings" :key="heading.id" :class="{ active: reconHeading === heading.id }" :aria-label="`${heading.id} ${heading.name}`" @click="reconHeading = heading.id"><span>{{ heading.arrow }}</span><b>{{ heading.id }}</b><small>{{ heading.name }}</small></button></fieldset>
        <div class="recon-options">
          <fieldset><legend>扇面角度</legend><div class="segmented-control"><button :class="{ active: reconWidth === 60 }" @click="reconWidth = 60">60°<small>标准发现率</small></button><button :class="{ active: reconWidth === 120 }" @click="reconWidth = 120">120°<small>发现率减半</small></button></div></fieldset>
          <div class="recon-radius"><span>最远侦察距离</span><strong>{{ reconAircraft.recon_radius_limit }} hex</strong><small>根据飞机航程自动确定，保留返航燃料</small></div>
        </div>
        <div class="recon-preview">
          <svg viewBox="0 0 200 200" role="img" aria-label="侦察方向预览" :data-heading="reconHeading" :data-width="reconWidth">
            <g :transform="`rotate(${headingRotation[reconHeading]} 100 100)`"><path class="recon-preview-sector" :d="reconWidth === 60 ? 'M100 100 L169.28 60 A80 80 0 0 1 169.28 140 Z' : 'M100 100 L140 30.72 A80 80 0 0 1 140 169.28 Z'" /></g>
            <g class="recon-compass-lines"><circle cx="100" cy="100" r="80" /><line x1="100" y1="100" x2="180" y2="100" /><line x1="100" y1="100" x2="140" y2="169.28" /><line x1="100" y1="100" x2="60" y2="169.28" /><line x1="100" y1="100" x2="20" y2="100" /><line x1="100" y1="100" x2="60" y2="30.72" /><line x1="100" y1="100" x2="140" y2="30.72" /></g>
            <g class="recon-compass-labels"><text x="184" y="104">E</text><text x="143" y="164">SE</text><text x="47" y="164">SW</text><text x="7" y="104">W</text><text x="46" y="43">NW</text><text x="139" y="43">NE</text></g>
            <circle class="recon-compass-center" cx="100" cy="100" r="4" />
          </svg>
          <div><b>预览方向</b><span>{{ reconHeading }} · {{ reconWidth }}°</span><small>实际覆盖hex由服务端在起飞后计算</small></div>
        </div>
        <div class="recon-brief"><span>顶点</span><strong>母舰当前位置</strong><i>→</i><span>航线</span><strong>{{ reconHeading }}</strong><i>→</i><span>扇面</span><strong>{{ reconWidth }}° / {{ reconAircraft.recon_radius_limit }} hex</strong></div>
        <footer><button class="reset-button" @click="reconAircraftIDs = []">取消</button><button class="apply-button" :disabled="busy" @click="launchRecon">起飞执行扇面侦察</button></footer>
      </section>
    </div>
  </section>
</template>
