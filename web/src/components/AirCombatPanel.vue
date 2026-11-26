<script setup lang="ts">
import { computed, onUnmounted, reactive, ref, watch } from 'vue'
import type { AirCombatReport } from '../types'
import { aircraftSilhouette } from '../silhouettes'
import { aircraftPictureURL } from '../silhouette-masks'
import ShipSilhouette from './ShipSilhouette.vue'

// 报告里只有机型名（如 "SB2C 地狱俯冲者" / "F6F 地狱猫"），按二战美军机代号反推剪影类型。
function kindOf(model: string) {
  const code = model.split(' ')[0].toUpperCase()
  if (code.startsWith('F4U') || code.startsWith('F6F') || code.startsWith('FM') || code === 'ZERO' || code.startsWith('A6M')) return 'fighter'
  if (code.startsWith('SBD') || code.startsWith('SB2C') || code.startsWith('D3A') || code.startsWith('D4Y')) return 'dive_bomber'
  if (code.startsWith('TBF') || code.startsWith('TBM') || code.startsWith('B5N')) return 'torpedo_bomber'
  // 水平轰炸机（B-25 / B-17 / B-24）没有专用剪影，借用俯冲轰炸机的轮廓。
  if (code.startsWith('B17') || code.startsWith('B-17') || code.startsWith('B24') || code.startsWith('B-24') || code.startsWith('B25') || code.startsWith('B-25')) return 'dive_bomber'
  if (code.startsWith('SOC') || code.startsWith('OS2U') || code.startsWith('E13A')) return 'seaplane'
  return 'recon'
}

// 动画里的算子优先用 units/*.json 里配置的机型图片（content/misc/image/aircraft/*.png），
// 图片缺失时才回退到内置矢量剪影，老存档和临时单位不会渲染成空白。
function formation(models: string[], pictures: string[] | undefined, count: number, ids: string[] = []) {
  const types = models.length ? models : ['F6F']
  const shots = pictures && pictures.length ? pictures : []
  const total = Math.max(1, Math.min(count || types.length, 8))
  return Array.from({ length: total }, (_, index) => {
    const model = types[index % types.length]
    return {
      key: index,
      id: ids[index] || `slot-${index}`,
      model,
      picture: shots.length ? aircraftPictureURL(shots[index % shots.length]) : undefined,
      path: aircraftSilhouette(kindOf(model)),
    }
  })
}

function lossCount(losses: number, total: number) {
  if (!losses) return 0
  const size = Math.max(1, Math.min(total || losses, 8))
  return Math.max(1, Math.min(size, Math.round((losses / Math.max(total, losses)) * size)))
}

const props = defineProps<{ report: AirCombatReport; hasNext?: boolean }>()
const emit = defineEmits<{ close: [] }>()
// 动画四个阶段：closing 对冲/俯冲 → clash 交火/投弹 → losses 损失 → resolved 结算
const phase = ref<'closing' | 'clash' | 'losses' | 'resolved'>('closing')
const timers: ReturnType<typeof setTimeout>[] = []
// 每个算子当前"打中并下沉"的舰船，用于逐次投弹的视觉反馈
const flashes = reactive<Record<string, number>>({})

function run() {
  timers.forEach(clearTimeout)
  timers.length = 0
  phase.value = 'closing'
  timers.push(setTimeout(() => { phase.value = 'clash' }, 1200))
  timers.push(setTimeout(() => { phase.value = 'losses' }, 2300))
  timers.push(setTimeout(() => { phase.value = 'resolved' }, 3600))
  // 后面还排着战报（一次结算的"拦截 + 空袭"就是两份）时自动翻到下一份，
  // 不然玩家不点关闭，第二份就永远排在队列里，看起来像动画丢了。
  if (props.hasNext) timers.push(setTimeout(() => emit('close'), 5600))
  // 交火阶段后按分配逐条闪红，时序由数据决定
  props.report.target_allocations?.forEach((allocation, index) => {
    if (!allocation.hits) return
    timers.push(setTimeout(() => {
      flashes[allocation.ship_id] = (flashes[allocation.ship_id] || 0) + allocation.hits
    }, 1250 + index * 140))
  })
}

watch(() => props.report.id, run, { immediate: true })
onUnmounted(() => timers.forEach(clearTimeout))

const targetKind = computed<'ship' | 'group' | 'airfield'>(() => (props.report.target === 'airfield' ? 'airfield' : props.report.target === 'ship' ? 'ship' : 'group'))
const isGround = computed(() => targetKind.value === 'airfield')
const hasCAP = computed(() => props.report.defender_count > 0)
// 打击报告（对舰/对地）：有 CAP 拦截时，closing 阶段先演双方对冲，交火阶段必须切回
// "俯冲 + 目标舰队"，否则一场被 CAP 掩护下的空袭只看得到两排飞机、下面连船都没有，
// 玩家会以为根本没打中（17 架打 1 架 CAP 的报告就是这种情况）。
const isInterceptReport = computed(() => props.report.target === 'intercept')
const showStrikeScene = computed(() => !isInterceptReport.value && (!hasCAP.value || phase.value !== 'closing'))
const showCAPScene = computed(() => hasCAP.value && !showStrikeScene.value)
// 没有 CAP 拦截的纯打击：飞机从上往下俯冲，下方是整支舰队（或机场）
const isStrike = computed(() => showStrikeScene.value && phase.value !== 'resolved')
const title = computed(() => (isGround.value ? '对地攻击' : '对舰攻击'))
const subtitle = computed(() => (isGround.value ? 'AIRFIELD STRIKE' : 'ANTI-SHIP STRIKE'))
const targetLabel = computed(() => props.report.target_name || (isGround.value ? '敌方机场' : '敌方舰队'))
const defenderTitle = computed(() => {
  if (hasCAP.value) return `CAP 防御 · ${props.report.defender_faction}`
  return isGround.value ? `机场防空 · ${props.report.defender_faction}` : `舰队防空 · ${props.report.defender_faction}`
})

const attacker = computed(() => formation(props.report.attacker_models, props.report.attacker_pictures, props.report.attacker_count, (props.report.bombers || []).map((bomber) => bomber.attacker)))
const defender = computed(() => formation(props.report.defender_models, props.report.defender_pictures, props.report.defender_count))
const attackerDown = computed(() => lossCount(props.report.attacker_losses, props.report.attacker_count))
const attackerHitIDs = computed(() => new Set((props.report.attacker_hits || []).filter((hit) => hit.outcome === '撤退' || hit.outcome === '损伤' || hit.outcome === '摧毁').map((hit) => hit.attacker)))
const showLosses = computed(() => phase.value === 'losses' || phase.value === 'resolved')
const showResult = computed(() => phase.value === 'resolved')
const phaseLabelBrief = computed(() => {
  if (isStrike.value) return ({ closing: '攻击编队俯冲进入', clash: '投弹 — 每架飞机攻击分配到的目标舰', losses: '损失结算' } as const)[phase.value as 'closing' | 'clash' | 'losses']
  return ({ closing: '攻击编队与 CAP 正在接近', clash: '交火 — 双方算子正在互相咬尾', losses: '损失结算' } as const)[phase.value as 'closing' | 'clash' | 'losses']
})
const phaseLabel = computed(() => {
  if (isStrike.value) return ({ closing: '俯冲进入', clash: '投弹', losses: '损失结算', resolved: '结算完成' } as const)[phase.value]
  return ({ closing: '双方对冲', clash: '交火中', losses: '损失结算', resolved: '结算完成' } as const)[phase.value]
})
const winnerText = computed(() => {
  if (props.report.winner === 'draw') return '双方脱离接触'
  if (hasCAP.value) return props.report.winner === props.report.attacker_faction ? '攻击编队突破 CAP 拦截' : 'CAP 成功阻止攻击编队'
  if (isGround.value) return '机场防御被压制'
  return props.report.winner === props.report.attacker_faction ? `攻击编队突破防空，${attackedShipCount.value} 艘舰受损` : '舰队防空击退了空袭'
})

// —— 对舰：下方舰队里每艘船、飞行中飞机瞄准的目标、以及未受攻击的舰船 ——
const fleetShips = computed(() => {
  const ships = props.report.ships || []
  const allocationByShip = new Map<string, { hits: number; models: string[] }>()
  for (const allocation of props.report.target_allocations || []) {
    const entry = allocationByShip.get(allocation.ship_id) || { hits: 0, models: [] }
    entry.hits += allocation.hits
    if (!entry.models.includes(allocation.model)) entry.models.push(allocation.model)
    allocationByShip.set(allocation.ship_id, entry)
  }
  return ships.map((ship) => {
    const allocation = allocationByShip.get(ship.id)
    return {
      ...ship,
      hits: allocation?.hits || 0,
      attackers: allocation?.models || [],
      attacked: Boolean(allocation),
    }
  })
})
const attackedShipCount = computed(() => fleetShips.value.filter((ship) => ship.attacked).length || props.report.ships_hit || 0)
// 把每架飞机放到它攻击的那艘船正上方：这样动画就是"飞机俯冲到自己分配到的船上"
function bomberLane(index: number) {
  const bomber = (props.report.bombers || [])[index]
  const allocation = props.report.target_allocations?.find((item) => item.attacker === bomber?.attacker)
  const shipIndex = allocation ? fleetShips.value.findIndex((ship) => ship.id === allocation.ship_id) : -1
  const total = Math.max(fleetShips.value.length, 1)
  const lane = shipIndex >= 0 ? (shipIndex + 0.5) / total : (index + 0.5) / Math.max(attacker.value.length, 1)
  return { left: `${(6 + lane * 78).toFixed(2)}%`, delay: `${(index * 0.12).toFixed(2)}s`, target: allocation?.ship_name || '' }
}
</script>

<template>
  <section class="air-combat-view" :class="[phase, { resolved: showResult, 'no-cap': !hasCAP, strike: showStrikeScene, ground: isGround }]" aria-live="polite">
    <header class="combat-header">
      <div><span class="section-kicker">{{ subtitle }} · {{ report.coord }} · {{ targetLabel }}</span><h1>{{ title }}</h1></div>
      <span class="combat-phase" :data-phase="phase">{{ phaseLabel }}</span>
    </header>

    <div class="combat-stage">
      <div class="combat-side attacker">
        <span>攻击编队 · {{ report.attacker_faction }}</span>
        <strong>{{ report.attacker_models.join(' / ') }}</strong>
        <small>{{ report.attacker_count }} 个航空算子</small>
      </div>
      <div class="combat-side defender">
        <span>{{ defenderTitle }}</span>
        <strong>{{ hasCAP ? report.defender_models.join(' / ') : (isGround ? '无 CAP，仅机场轻型防空' : report.target_allocations ? `${attackedShipCount} 艘舰被分配攻击` : '无 CAP，仅靠舰队防空') }}</strong>
        <small>{{ hasCAP ? `${report.defender_count} 个航空算子` : `舰队 ${fleetShips.length} 艘` }}</small>
      </div>

      <div class="combat-lane" aria-hidden="true"><i v-for="n in 7" :key="n" :style="{ left: (n * 14 - 6) + '%' }"></i></div>

      <!-- 空对空：双方对冲 -->
      <template v-if="showCAPScene">
        <div class="combat-formation formation-left" aria-hidden="true">
          <div v-for="plane in attacker" :key="plane.key" class="plane" :class="{ down: showLosses && plane.key < attackerDown }" :style="{ '--row': ((plane.key % 2) - 0.5) }">
            <img v-if="plane.picture" class="plane-art" :src="plane.picture" :alt="plane.model" /><svg v-else viewBox="0 0 62 44"><path :d="plane.path" /></svg>
          </div>
        </div>
        <div class="combat-formation formation-right" aria-hidden="true">
          <div v-for="plane in defender" :key="plane.key" class="plane" :class="{ down: showLosses && plane.key < lossCount(report.defender_losses, report.defender_count) }">
            <img v-if="plane.picture" class="plane-art" :src="plane.picture" :alt="plane.model" /><svg v-else viewBox="0 0 62 44"><path :d="plane.path" /></svg>
          </div>
        </div>
      </template>

      <!-- 对舰/对地：飞机在上方，目标编队/机场在下方，飞机俯冲到自己分配的目标上 -->
      <template v-if="showStrikeScene">
        <div class="strike-air" aria-hidden="true">
          <div v-for="(plane, index) in attacker" :key="plane.key" class="dive-plane" :class="{ 'lost': attackerHitIDs.has(plane.id), 'show-loss': showLosses }" :style="{ ...bomberLane(index) }">
            <img v-if="plane.picture" class="plane-art" :src="plane.picture" :alt="plane.model" /><svg v-else viewBox="0 0 62 44"><path :d="plane.path" /></svg>
            <span v-if="bomberLane(index).target" class="dive-target">{{ bomberLane(index).target }}</span>
          </div>
        </div>

        <div v-if="isGround" class="ground-target" :class="{ 'hit': report.strike_hits }">
          <svg viewBox="0 0 160 70" aria-label="机场">
            <path class="g-runway" d="M8 46h144v12H8z" />
            <path class="g-stripes" d="M22 46v12M48 46v12M74 46v12M100 46v12M126 46v12" />
            <path class="g-tower" d="M104 22h16v24h-16zM108 14h8v8h-8z" />
            <path class="g-hangar" d="M18 30h34v16H18z" />
            <path class="g-hangar" d="M58 34h22v12H58z" />
          </svg>
          <b>{{ targetLabel }}</b>
        </div>

        <div v-else class="fleet-deck" aria-label="被攻击舰队">
          <div v-for="ship in fleetShips" :key="ship.id" class="fleet-ship" :class="{ attacked: ship.hits > 0, target: ship.hits > 0 && phase !== 'resolved' }" :style="{ '--lane': fleetShips.length }">
            <!-- 用和地图兵牌同一套素材画剪影。以前这里套了一层 <svg viewBox>，
                 ShipSilhouette 的蒙版元素在 SVG 命名空间里画不出来，只能退成手绘剪影，
                 看起来和地图上的船牌完全不是一套资源。 -->
            <span class="fleet-ship-art"><ShipSilhouette :type="ship.type" :name="ship.name" :picture="ship.picture" /></span>
            <b>{{ ship.name }}</b>
            <small v-if="ship.hits > 0" :key="flashes[ship.id] || 0">{{ ship.attackers.join('/') }} 命中 {{ ship.hits }}</small>
            <small v-else>未受攻击</small>
            <span v-for="hit in ship.hits" :key="'h' + hit" class="hit-flash" :style="{ animationDelay: (hit * 0.14) + 's' }"></span>
          </div>
          <p v-if="!fleetShips.length" class="fleet-empty">舰队情报不足，未返回舰船清单</p>
        </div>
      </template>

      <div class="combat-contact" aria-hidden="true"><i></i><i></i><i></i><b>VS</b></div>
      <div class="combat-scan" aria-hidden="true"></div>

      <div v-if="showLosses" class="combat-loss-pops">
        <span class="loss attacker-loss">攻击方 -{{ report.attacker_losses }}</span>
        <span v-if="report.aa_hits" class="loss aa-loss">防空命中 {{ report.aa_hits }}</span>
        <span v-if="report.strike_hits" class="loss strike-loss">投弹命中 {{ report.strike_hits }}</span>
        <span v-else-if="report.bomb_attempts === 0" class="loss aa-loss">未投弹</span>
        <span v-else-if="report.bomb_attempts" class="loss strike-loss">投弹 {{ report.bomb_attempts }} 发 · 0 命中</span>
        <span v-if="hasCAP" class="loss defender-loss">CAP -{{ report.defender_losses }}</span>
      </div>
    </div>

    <div v-if="!showResult" class="combat-waiting">
      <span class="combat-loader"></span>
      <div>
        <strong>{{ phaseLabelBrief }}</strong>
        <small>{{ isGround ? '攻击机俯冲机场，逐点独立判定对地命中' : hasCAP ? '攻击机、护航战斗机与 CAP 按服务端判定的结果展开' : '攻击机按分配俯冲目标舰船，逐点独立判定对舰命中' }}</small>
      </div>
    </div>

    <section v-else class="combat-result">
      <span class="section-kicker">AUTHORITATIVE RESULT</span>
      <h2>{{ winnerText }}</h2>
      <p>{{ report.summary }}</p>
      <div class="combat-result-grid">
        <div><span>攻击方损失</span><strong>{{ report.attacker_losses }}</strong></div>
        <div v-if="hasCAP"><span>CAP 损失</span><strong>{{ report.defender_losses }}</strong></div>
        <div><span>防空命中</span><strong>{{ report.aa_hits }}</strong></div>
        <div><span>投弹命中</span><strong>{{ report.strike_hits }}</strong></div>
        <div v-if="!hasCAP && !isGround"><span>受损舰船</span><strong>{{ report.ships_hit ?? 0 }}</strong></div>
        <div v-if="isGround"><span>机场损失</span><strong>{{ report.airfield_hits || 0 }}</strong></div>
        <div v-if="isGround && report.airfield_after !== undefined" class="runway-loss">
          <span>跑道</span>
          <strong>{{ report.airfield_runways_before ?? '—' }} → {{ report.airfield_runways_after ?? '—' }}</strong>
          <div class="runway-stripes">
            <i v-for="n in (report.airfield_runways_before || 0)" :key="n" :class="{ lost: n > (report.airfield_runways_after || 0) }"></i>
          </div>
        </div>
        <div v-if="isGround && report.ground_losses"><span>地面飞机损失</span><strong>{{ report.ground_losses }}</strong></div>
      </div>
      <button class="apply-button" @click="emit('close')">返回大地图</button>
    </section>
  </section>
</template>

<style scoped>
.air-combat-view{position:relative;display:flex;flex-direction:column;width:100%;height:100%;min-height:0;padding:28px 38px;overflow:hidden;background:#09171c;color:#dce7e3}
.combat-header{display:flex;align-items:flex-start;justify-content:space-between;flex:none;border-bottom:1px solid #30474f;padding-bottom:16px}
.combat-header h1{margin:6px 0 0;font-size:28px;letter-spacing:2px}
.combat-phase{padding:5px 10px;border:1px solid #8b774d;color:var(--gold);font-size:10px}
.combat-phase[data-phase="clash"]{border-color:#c06a55;color:#f0a08a;animation:phase-pulse 1s ease-in-out infinite}
.combat-phase[data-phase="losses"],.combat-phase[data-phase="resolved"]{border-color:#7ea18c;color:#a8dcc0}
.combat-stage{position:relative;flex:1;min-height:340px;overflow:hidden}
.combat-stage:before{content:'';position:absolute;inset:0;background-image:linear-gradient(#27414a33 1px,transparent 1px),linear-gradient(90deg,#27414a33 1px,transparent 1px);background-size:48px 48px}
.combat-side{position:absolute;top:18px;z-index:3;display:flex;flex-direction:column;gap:4px}
.strike .combat-side.attacker{opacity:0;pointer-events:none}
.combat-side.attacker{left:0}
.combat-side.defender{right:0;text-align:right}
.combat-side span{color:#7f9da3;font-size:10px;letter-spacing:1px}
.combat-side strong{color:#d8e5e1;font-size:15px}
.combat-side small{color:#718e94;font-size:10px}
.combat-lane{position:absolute;left:14%;right:14%;top:44%;height:1px;background:linear-gradient(90deg,transparent,#3d5f66 25%,#3d5f66 75%,transparent)}
.combat-lane i{position:absolute;top:-4px;width:1px;height:9px;background:#3d5f66}

/* 空对空对冲 */
.combat-formation{position:absolute;top:50%;z-index:2;display:flex;flex-direction:column;gap:10px;transition:transform 1s cubic-bezier(.2,.8,.3,1),filter .4s}
.combat-formation .plane{display:grid;place-items:center}
.combat-formation svg{width:64px;height:46px;fill:#83bed0;filter:drop-shadow(0 0 10px #65bad055)}
.formation-left{left:6%;transform:translateY(-50%)}
.formation-right{right:6%;transform:translateY(-50%)}
.combat-formation img{width:64px;height:46px;object-fit:contain;filter:drop-shadow(0 0 10px #65bad055)}
.formation-left img{transform:rotate(90deg)}
.formation-right img{transform:rotate(-90deg)}
.formation-left svg{transform:rotate(90deg)}
.formation-right svg{transform:rotate(-90deg);fill:#d7bb7b}
.closing .formation-left{transform:translate(calc(50% + 10px), -50%)}
.closing .formation-right{transform:translate(calc(-50% - 10px), -50%)}
.clash .formation-left{transform:translate(calc(50% + 120px), -50%)}
.clash .formation-right{transform:translate(calc(-50% - 120px), -50%)}
.losses .formation-left,.resolved .formation-left,.losses .formation-right,.resolved .formation-right{transform:translateY(-50%)}
.losses .plane.down,.resolved .plane.down{animation:plane-down 1.1s ease-in forwards}

/* 空对舰 / 空对地：飞机在上，目标在下方，俯冲对准分配到的船 */
.strike-air{position:absolute;left:4%;right:4%;top:34px;bottom:42%;z-index:2}
.dive-plane{position:absolute;top:0;display:flex;flex-direction:column;align-items:center;gap:2px;margin-left:-34px;transition:transform 1.1s cubic-bezier(.3,.7,.3,1)}
.dive-plane svg{width:58px;height:42px;fill:#9ed6e8;filter:drop-shadow(0 0 10px #65bad066);transform:rotate(180deg)}
.dive-plane img{width:58px;height:42px;object-fit:contain;filter:drop-shadow(0 0 10px #65bad066);transform:rotate(180deg)}
.dive-plane .dive-target{color:#8fb7c4;font:9px 'Noto Sans SC',sans-serif;white-space:nowrap}
.closing .dive-plane{transform:translateY(0)}
.clash .dive-plane{animation:dive-bomb 1.3s ease-in forwards;animation-delay:var(--delay)}
.losses .dive-plane,.resolved .dive-plane{transform:translateY(20px);opacity:.85}
.dive-plane.lost.show-loss{opacity:0;transition:opacity .6s}
.dive-plane.lost.show-loss svg,.dive-plane.lost.show-loss img{animation:plane-down 1.1s ease-in forwards}

.fleet-deck{position:absolute;left:2%;right:2%;bottom:44px;z-index:2;display:flex;align-items:flex-end;justify-content:space-around;gap:4px;padding:10px 6px 6px;border-top:1px solid #2f4a52;background:linear-gradient(#0d2026cc,#0b1b20ee)}
.fleet-ship{position:relative;display:flex;flex:1 1 0;min-width:0;flex-direction:column;align-items:center;gap:1px;opacity:.82;transition:transform .4s,opacity .4s}
.fleet-ship-art{display:block;width:100%;max-width:92px;height:26px;color:#6f8c95}
.fleet-ship-art svg{width:100%;height:100%}
.fleet-ship b{overflow:hidden;max-width:100%;color:#a9c1c5;font:10px 'Noto Sans SC',sans-serif;text-overflow:ellipsis;white-space:nowrap}
.fleet-ship small{color:#62787c;font-size:8px}
.fleet-ship.attacked{opacity:1}
.fleet-ship.attacked .fleet-ship-art{color:#c98f6a}
.fleet-ship.attacked small{color:#e8b48e}
.fleet-ship.target{transform:translateY(-4px)}
.fleet-ship.target:before{content:'';position:absolute;inset:-4px -2px 18px;border:1px dashed #d7bb7b88;animation:target-lock 1.2s ease-out infinite}

.hit-flash{position:absolute;left:50%;top:18%;width:38px;height:38px;margin-left:-19px;border-radius:50%;background:radial-gradient(circle,#fff3c4,#ff9a3c 45%,transparent 70%);opacity:0;animation:hit-burst .7s ease-out forwards}
.fleet-empty{color:#7d989c;font-size:10px}
.ground-target{position:absolute;left:20%;right:20%;bottom:48px;z-index:2;display:flex;flex-direction:column;align-items:center;gap:4px}
.ground-target svg{width:100%;max-width:320px;height:auto;filter:drop-shadow(0 0 10px #d7bb7b33)}
.g-runway{fill:#b9aa7f}.g-stripes{stroke:#d7bb7b;stroke-width:1.2;opacity:.5}.g-tower{fill:#c9b98a}.g-hangar{fill:#8f8560}
.ground-target b{color:#c9b98a;font:10px 'Noto Sans SC',sans-serif}
.clash .ground-target{animation:hit-burst .7s ease-out}

.combat-contact{position:absolute;left:50%;top:44%;z-index:3;width:90px;height:90px;transform:translate(-50%,-50%);opacity:0;transition:opacity .3s}
.closing .combat-contact,.clash .combat-contact{opacity:1}
.strike .combat-contact{display:none}
.combat-contact b{position:absolute;inset:0;display:grid;place-items:center;color:#e5cf91;font:600 24px 'Rajdhani',sans-serif}
.combat-contact i{position:absolute;left:50%;top:50%;width:6px;height:6px;border-radius:50%;background:#f2d792;animation:contact-flash 1.2s ease-out infinite}
.combat-contact i:nth-child(2){animation-delay:.35s}
.combat-contact i:nth-child(3){animation-delay:.7s}
.combat-scan{position:absolute;left:50%;top:50%;width:220px;height:220px;transform:translate(-50%,-50%);border:1px solid #d7bb7b33;border-radius:50%;animation:scan-pulse 2.4s ease-out infinite}
.clash .combat-scan{border-color:#f0a08a66}

.combat-loss-pops{position:absolute;left:0;right:0;top:96px;z-index:4;display:flex;justify-content:center;gap:14px;pointer-events:none}
.strike .combat-loss-pops{top:auto;bottom:8px}
.combat-loss-pops .loss{padding:6px 12px;border:1px solid #a8615a;background:#2c1a1acc;color:#f2b0a2;font:600 13px 'Rajdhani','Noto Sans SC',sans-serif;letter-spacing:1px;animation:loss-pop .5s cubic-bezier(.2,1.4,.4,1) both}
.combat-loss-pops .aa-loss{border-color:#9a8a4f;background:#2c2818cc;color:#e6d394}
.combat-loss-pops .strike-loss{border-color:#8a5a4a;background:#2c1d18cc;color:#f0bfa0}
.combat-loss-pops .defender-loss{border-color:#5a7fa1;background:#18222ccc;color:#a8c8e6;animation-delay:.12s}
.combat-waiting{display:flex;align-items:center;gap:14px;padding:14px 18px;border:1px solid #30474f;color:#c9d8d4}
.combat-waiting>div{display:flex;flex-direction:column;gap:4px}
.combat-waiting small{color:#7d989c;font-size:10px}
.combat-loader{width:18px;height:18px;flex:none;border:2px solid #3d5f66;border-top-color:#d7bb7b;border-radius:50%;animation:spin 1s linear infinite}
.combat-result{display:flex;flex-direction:column;gap:10px}
.combat-result h2{margin:0;font-size:22px;color:#e4cf91}
.combat-result p{margin:0;color:#a9bcba;font-size:12px}
.combat-result-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:10px}
.combat-result-grid>div{display:flex;flex-direction:column;gap:4px;padding:10px 12px;border:1px solid #30474f;background:#0f2228}
.runway-loss .runway-stripes{display:flex;gap:3px;margin-top:4px}
.runway-loss .runway-stripes i{display:block;width:18px;height:4px;background:#d7bb7b}
.runway-loss .runway-stripes i.lost{background:#8a5b4f;position:relative}
.runway-loss .runway-stripes i.lost:after{content:'';position:absolute;left:0;right:0;top:1px;height:1px;background:#2a1612;transform:rotate(-12deg)}
.combat-result-grid span{color:#7d989c;font-size:10px}
.combat-result-grid strong{color:#e6dcc4;font:600 24px 'Rajdhani',sans-serif}
.combat-result .apply-button{align-self:flex-start;margin-top:6px}
@keyframes spin{to{transform:rotate(360deg)}}
@keyframes scan-pulse{0%{opacity:.8;transform:translate(-50%,-50%) scale(.4)}100%{opacity:0;transform:translate(-50%,-50%) scale(1.5)}}
@keyframes contact-flash{0%{filter:brightness(3);opacity:1}100%{filter:brightness(1);opacity:.2}}
@keyframes plane-down{0%{opacity:1}100%{opacity:0;transform:translateY(120px) rotate(140deg) scale(.7)}}
@keyframes dive-bomb{0%{transform:translateY(0) scale(1)}70%{transform:translateY(calc(100% + 220px)) scale(.55)}100%{transform:translateY(calc(100% + 250px)) scale(.35);opacity:0}}
@keyframes target-lock{0%{opacity:.9;transform:scale(.94)}100%{opacity:0;transform:scale(1.12)}}
@keyframes hit-burst{0%{opacity:1;transform:scale(.4)}60%{opacity:.9;transform:scale(1.1)}100%{opacity:0;transform:scale(1.35)}}
@keyframes loss-pop{0%{opacity:0;transform:translateY(14px) scale(.9)}100%{opacity:1;transform:translateY(0) scale(1)}}
@keyframes phase-pulse{0%,100%{opacity:1}50%{opacity:.55}}
</style>
