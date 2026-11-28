<script setup lang="ts">
import type { AirTask, AirfieldContact } from '../types'
import { contactIntelLabel, type FleetGroup } from '../fleet'

// 点 hex 弹出来的"这一格有什么"清单：堆叠的舰队/编队/机场不用再靠牌子互相错开来点，
// 先在这一格里挑对象，再展开它的详情。对局和观战共用同一个组件。
const props = defineProps<{
  coord: string
  fleets: FleetGroup[]
  airfields: AirfieldContact[]
  airTasks: AirTask[]
  playerFaction: 'A' | 'B'
  spectate?: boolean
  // target 模式用于选打击/转场目标：面板只列候选，点一项就当目标提交。
  mode?: 'inspect' | 'target'
  hint?: string
}>()

const emit = defineEmits<{
  close: []
  selectFleet: [fleetID: string]
  selectAirfield: [airfieldID: string]
  selectAirTask: [taskID: string]
}>()

function factionName(faction?: string) {
  return faction === 'B' ? '日军' : '美军'
}

function fleetTitle(fleet: FleetGroup) {
  // 情报没到 4 级时连名字都不能写：按情报标签说"有敌舰/中型舰队"。
  const level = fleet.contact_level || 0
  if (level > 0 && level < 4) return contactIntelLabel(fleet) || '敌方舰队'
  return fleet.name || fleet.fleet_name || fleet.fleet_id
}

function taskTitle(task: AirTask) {
  const base = task.kind === 'attack' ? '打击编队' : task.kind === 'cover' ? '掩护编队' : task.kind === 'recon' ? '侦察' : '转场编队'
  return (task.returning ? '返航·' : '') + base
}

function taskFaction(task: AirTask) {
  return task.faction || props.playerFaction
}
</script>

<template>
  <aside class="hex-panel" :data-hex="coord">
    <header>
      <div>
        <span class="hex-panel-kicker">{{ mode === 'target' ? 'TARGET' : 'HEX' }}</span>
        <h2>{{ coord }}</h2>
        <p v-if="hint" class="hex-panel-hint">{{ hint }}</p>
      </div>
      <button type="button" aria-label="关闭格子清单" @click="emit('close')">×</button>
    </header>

    <p v-if="!fleets.length && !airfields.length && !airTasks.length" class="hex-panel-empty">这一格没有可见目标</p>

    <template v-if="fleets.length">
      <h3>舰队 · {{ fleets.length }}</h3>
      <button
        v-for="fleet in fleets"
        :key="fleet.fleet_id"
        type="button"
        class="hex-panel-row"
        :class="`faction-${fleet.faction.toLowerCase()}`"
        @click="emit('selectFleet', fleet.fleet_id)"
      >
        <i class="hex-panel-dot"></i>
        <span class="hex-panel-name">{{ fleetTitle(fleet) }}</span>
        <small>{{ fleet.count }} 艘 · {{ factionName(fleet.faction) }}</small>
      </button>
    </template>

    <template v-if="airfields.length">
      <h3>机场 · {{ airfields.length }}</h3>
      <button
        v-for="airfield in airfields"
        :key="airfield.id"
        type="button"
        class="hex-panel-row"
        :class="`faction-${airfield.faction.toLowerCase()}`"
        @click="emit('selectAirfield', airfield.id)"
      >
        <i class="hex-panel-dot"></i>
        <span class="hex-panel-name">{{ airfield.name }}</span>
        <small>生命 {{ airfield.strength }}/{{ airfield.max_strength }} · {{ factionName(airfield.faction) }}</small>
      </button>
    </template>

    <template v-if="airTasks.length">
      <h3>空中编队 · {{ airTasks.length }}</h3>
      <button
        v-for="task in airTasks"
        :key="task.id"
        type="button"
        class="hex-panel-row"
        :class="`faction-${taskFaction(task).toLowerCase()}`"
        @click="emit('selectAirTask', task.id)"
      >
        <i class="hex-panel-dot"></i>
        <span class="hex-panel-name">{{ taskTitle(task) }}</span>
        <small>{{ task.airborne_count }}/{{ task.aircraft_ids.length }} 升空 · {{ task.kind === 'attack' && task.target_coord ? '目标 ' + task.target_coord : task.current_coord }}</small>
      </button>
    </template>
  </aside>
</template>

<style scoped>
/* 贴在格子所在的那张海图左上角：选中某一格时不抢右边的详情面板。 */
.hex-panel { position: absolute; top: 12px; left: 12px; z-index: 9; width: 250px; max-height: calc(100% - 24px); overflow: auto; padding: 9px 10px; border: 1px solid #536d75; border-radius: 10px; background: linear-gradient(150deg, #12242af5, #1b3036f5); box-shadow: 0 12px 32px #0009; color: #d5e3df; }
.hex-panel>header { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; }
.hex-panel-kicker { font: 600 9px 'Rajdhani', sans-serif; letter-spacing: 1.6px; color: #7fa39d; }
.hex-panel h2 { margin: 1px 0 0; font: 600 15px 'Rajdhani', sans-serif; color: #e6dcc2; }
.hex-panel>header button { width: 22px; height: 22px; padding: 0; border: 1px solid #47626a; border-radius: 5px; background: transparent; color: #a9c0be; font-size: 15px; line-height: 1; cursor: pointer; }
.hex-panel>header button:hover { border-color: var(--gold); color: var(--gold); }
.hex-panel h3 { margin: 9px 0 4px; font: 600 10px 'Noto Sans SC', sans-serif; letter-spacing: .5px; color: #8fb0aa; }
.hex-panel-row { display: grid; grid-template-columns: 9px minmax(0, 1fr); grid-template-rows: auto auto; column-gap: 6px; width: 100%; padding: 5px 6px; border: 1px solid transparent; border-radius: 6px; background: rgba(255,255,255,.03); color: inherit; text-align: left; cursor: pointer; }
.hex-panel-row + .hex-panel-row { margin-top: 3px; }
.hex-panel-row:hover { border-color: var(--gold); background: rgba(215,187,123,.09); }
.hex-panel-dot { grid-row: 1 / span 2; align-self: center; width: 7px; height: 7px; border-radius: 50%; background: #6f8d93; }
.hex-panel-row.faction-a .hex-panel-dot { background: #5eb5c8; }
.hex-panel-row.faction-b .hex-panel-dot { background: #d77c6c; }
.hex-panel-name { min-width: 0; overflow: hidden; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.hex-panel-row small { min-width: 0; overflow: hidden; font-size: 10px; color: #8ba4a6; text-overflow: ellipsis; white-space: nowrap; }
.hex-panel-hint { margin: 4px 0 0; font-size: 10px; color: var(--gold); }
.hex-panel-empty { margin: 8px 0 0; font-size: 11px; color: #8ba4a6; }
</style>
