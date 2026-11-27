<script setup lang="ts">
import { computed } from 'vue'
import type { GameView } from '../types'
import ShipCounter from './ShipCounter.vue'

const props = defineProps<{ state: GameView }>()

const sides = computed(() => {
  const own = props.state.you.faction
  const other = own === 'A' ? 'B' : 'A'
  const order = [own, other]
  return order.map((faction, index) => {
    const ledger = props.state.losses?.[faction] || {}
    const ships = ledger.ships || []
    const crippled = ledger.crippled || []
    const aircraft = ledger.aircraft || []
    return {
      faction,
      title: faction === 'A' ? '美军' : '日军',
      subtitle: faction === own ? (index === 0 ? '我方' : '') : '敌方',
      own: faction === own,
      ships,
      crippled,
      aircraft,
      aircraftTotal: ledger.aircraft_total || aircraft.reduce((sum, item) => sum + item.count, 0),
    }
  })
})

const verdict = computed(() => props.state.outcome)
const verdictTitle = computed(() => {
  const outcome = props.state.outcome
  if (!outcome) return ''
  const faction = outcome.winner === 'A' ? '美军' : outcome.winner === 'B' ? '日军' : ''
  if (outcome.level === 'draw') return '平分秋色'
  if (outcome.level === 'tactical') return `${faction} · 战术胜利`
  if (outcome.level === 'strategic') return `${faction} · 战略胜利`
  return outcome.winner ? `${faction}获胜` : '平局'
})
</script>

<template>
  <section class="workspace-page results-page">
    <span class="section-kicker">AFTER ACTION</span>
    <h1>损失情况</h1>
    <p class="results-note">
      击沉的舰船与打掉的航空算子按回合记账。飞机按机型统计，含空战被击落、随舰沉没、机库被炸和燃油耗尽。
    </p>

    <div v-if="verdict" class="results-verdict">
      <strong>对局结束 · {{ verdictTitle }}</strong>
      <span>{{ verdict.reason }}</span>
      <span v-if="verdict.winner">胜方：{{ verdict.winner === 'A' ? '美军' : '日军' }}</span>
    </div>

    <div class="results-grid">
      <article v-for="side in sides" :key="side.faction" class="results-column" :class="`results-${side.faction.toLowerCase()}`">
        <header>
          <h2>{{ side.title }}<small v-if="side.own">我方</small><small v-else>敌方</small></h2>
          <div class="results-tally">
            <span>沉没 <b>{{ side.ships.length }}</b> 艘</span>
            <span>损失 <b>{{ side.aircraftTotal }}</b> 个算子</span>
          </div>
        </header>

        <section class="results-block">
          <h3>沉没舰船</h3>
          <p v-if="!side.ships.length" class="results-empty">无</p>
          <div v-else class="lost-ship-grid">
            <div v-for="ship in side.ships" :key="ship.id || ship.name" class="lost-ship">
              <ShipCounter
                :unit="{
                  id: ship.id || ship.name,
                  definition_id: ship.type.toLowerCase(),
                  fleet_id: '',
                  fleet_name: ship.fleet || '',
                  name: ship.name,
                  type: ship.type,
                  faction: side.faction as 'A' | 'B',
                  coord: '—',
                  heading: 'NE',
                  strength: 0,
                  max_strength: 0,
                  moved: false,
                  card: ship.card || {
                    gunnery: 0, damage_control: 0, main_gun_count: 0, main_gun_caliber: 0,
                    heavy_aa: 0, light_aa: 0, surface_radar: false, air_radar: false,
                    speed_class: 'normal', torpedo_salvos: 0, aircraft: 0, hangar_capacity: 0,
                    deck_capacity: 0, elevator_count: 0, depth_charges: 0, cargo: 0, armored_carrier: false,
                  },
                  damage: {},
                }"
              />
              <span class="lost-turn">T{{ ship.turn }} · 沉没</span>
            </div>
          </div>
        </section>

        <section class="results-block">
          <h3>瘫痪航母</h3>
          <!-- 瘫痪是"还能浮着但打不了了"：甲板摧毁、结构损失超过 80%、失去动力，
               战果分按机库容量的一半计，只记一次。 -->
          <p v-if="!side.crippled.length" class="results-empty">无</p>
          <ul v-else class="lost-air-list">
            <li v-for="ship in side.crippled" :key="ship.id || ship.name">
              <span class="lost-air-model">{{ ship.name }}</span>
              <span class="lost-air-count">T{{ ship.turn }} · {{ ship.reason }}</span>
            </li>
          </ul>
        </section>

        <section class="results-block">
          <h3>损失飞机</h3>
          <!-- 敌方的飞机损失在对局结束前是保密的：沉没随舰、机库被炸、燃油耗尽这些
               对方看不见，累计台账会暴露他还剩多少航空力量。 -->
          <p v-if="!side.own && !side.aircraft.length" class="results-empty">情报未确认（对局结束后公开）</p>
          <p v-else-if="!side.aircraft.length" class="results-empty">无</p>
          <ul v-else class="lost-air-list">
            <li v-for="plane in side.aircraft" :key="plane.model + plane.kind">
              <span class="lost-air-model">{{ plane.model }}</span>
              <span class="lost-air-count">×{{ plane.count }}</span>
            </li>
          </ul>
        </section>
      </article>
    </div>
  </section>
</template>
