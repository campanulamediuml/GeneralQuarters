<script setup lang="ts">
import { computed } from 'vue'
import type { AirUnit, AviationView } from '../types'

const props = defineProps<{ aviation: AviationView; busy: boolean }>()
const emit = defineEmits<{ command: [type: 'LAUNCH_CAP' | 'RECOVER_AIR' | 'PRIORITIZE_AIR_TASK', aircraftID: string] }>()
const carriers = computed(() => props.aviation.carriers)
const tasks = computed(() => props.aviation.tasks)
const lanes = [
  { kind: 'recon', name: '侦察任务' },
  { kind: 'cap', name: '空中巡逻' },
  { kind: 'asw', name: '反潜巡逻' },
  { kind: 'attack', name: '攻击 / 转场' },
]
const kindNames = { fighter: '战斗机', dive_bomber: '俯冲轰炸机', torpedo_bomber: '鱼雷机', recon: '侦察机' }

function action(aircraft: AirUnit) {
  if (aircraft.status === 'cap') emit('command', 'RECOVER_AIR', aircraft.id)
  else if (aircraft.kind === 'fighter') emit('command', 'LAUNCH_CAP', aircraft.id)
}
</script>

<template>
  <section class="workspace-page aviation-workspace">
    <div class="aviation-title"><div><span class="section-kicker">AVIATION COMMAND</span><h1>航空指挥界面</h1></div><span>CAP通信测试</span></div>
    <p class="aviation-intro">每个算子代表4架飞机。点击任务边缘的小旗，可将该任务优先级提至最高；任务顺序由服务器保存。蓝色为舰载机，橙色为水上飞机。</p>

    <div class="air-task-lanes">
      <section v-for="lane in lanes" :key="lane.kind" class="air-task-lane">
        <header><span>{{ lane.name }}</span><small>{{ tasks.filter((task) => task.kind === lane.kind).length }} 项</small></header>
        <div v-if="tasks.some((task) => task.kind === lane.kind)" class="air-task-list">
          <article v-for="task in tasks.filter((item) => item.kind === lane.kind)" :key="task.id" class="air-task" :class="{ prioritized: task.priority === 1 }" :data-task-id="task.id">
            <div><strong>{{ task.name }}</strong><small>{{ task.carrier_id }} · {{ task.aircraft_ids.length }} 算子</small></div>
            <span class="task-priority">优先级 {{ task.priority }}</span>
            <button class="task-flag" :class="{ active: task.priority === 1 }" :aria-label="`将${task.name}设为最高优先级`" :disabled="busy" @click="emit('command', 'PRIORITIZE_AIR_TASK', task.id)">⚑</button>
          </article>
        </div>
        <p v-else>暂无任务</p>
      </section>
    </div>

    <div v-if="!carriers.length" class="aviation-placeholder"><span>△</span><h2>当前舰队没有航空作业舰</h2><p>在剧本中部署航母或搭载水上飞机的舰船。</p></div>
    <div class="carrier-air-grid">
      <section v-for="carrier in carriers" :key="carrier.unit_id" class="carrier-air-card" :data-carrier-id="carrier.unit_id">
        <header><div><span class="unit-type-badge">{{ carrier.type }}</span><h2>{{ carrier.name }}</h2></div><div class="hangar-meter"><span>机库</span><strong>{{ carrier.hangar_used }} / {{ carrier.hangar_capacity }}</strong></div></header>
        <div class="carrier-operation"><span :class="`operation-${carrier.operation || 'idle'}`">{{ carrier.operation === 'launch' ? '本回合：起飞作业' : carrier.operation === 'recover' ? '本回合：降落作业' : carrier.deck_capacity ? '甲板待命' : '水机机库' }}</span><small v-if="carrier.deck_capacity">本回合起飞 {{ carrier.deck_used }} / {{ carrier.deck_capacity }}</small><small v-else>{{ carrier.unit_id }}</small></div>
        <div class="air-counter-grid">
          <article v-for="aircraft in carrier.aircraft" :key="aircraft.id" class="air-counter" :class="[`air-${aircraft.status}`, `kind-${aircraft.kind}`, { seaplane: aircraft.seaplane }]" :data-aircraft-id="aircraft.id" :data-status="aircraft.status">
            <header><strong>{{ aircraft.seaplane ? '水上飞机' : kindNames[aircraft.kind] }}</strong><span>{{ aircraft.status === 'cap' ? 'CAP' : '机库' }}</span></header>
            <div class="aircraft-symbol">✈</div>
            <h3>{{ aircraft.model }}</h3>
            <dl><div><dt>机体</dt><dd>{{ aircraft.strength }}</dd></div><div><dt>空战</dt><dd>{{ aircraft.air_combat }}</dd></div><div><dt>对舰</dt><dd>{{ aircraft.anti_ship }}</dd></div><div><dt>航程</dt><dd :class="{ 'low-range': aircraft.remaining_range <= 1 }">{{ aircraft.remaining_range }} / {{ aircraft.max_range }}</dd></div></dl>
            <button v-if="aircraft.status === 'cap' || aircraft.kind === 'fighter'" :disabled="busy" @click="action(aircraft)">{{ aircraft.status === 'cap' ? '回收至机库' : 'CAP 升空' }}</button>
            <span v-else class="air-no-action">等待后续任务模块</span>
          </article>
          <p v-if="!carrier.aircraft.length" class="empty-hangar">机库空置</p>
        </div>
      </section>
    </div>
  </section>
</template>
