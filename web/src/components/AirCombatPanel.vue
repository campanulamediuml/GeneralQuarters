<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import type { AirCombatReport } from '../types'

const props = defineProps<{ report: AirCombatReport }>()
const emit = defineEmits<{ close: [] }>()
const resolved = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined

watch(() => props.report.id, () => {
  resolved.value = false
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => { resolved.value = true }, 1800)
}, { immediate: true })

onUnmounted(() => { if (timer) clearTimeout(timer) })

const winnerText = computed(() => {
  if (props.report.winner === 'draw') return '双方脱离接触'
  return props.report.winner === props.report.attacker_faction ? '攻击编队突破 CAP 拦截' : 'CAP 成功阻止攻击编队'
})
</script>

<template>
  <section class="air-combat-view" :class="{ resolved }" aria-live="polite">
    <header class="combat-header">
      <div><span class="section-kicker">AIR COMBAT · {{ report.coord }}</span><h1>空中交战</h1></div>
      <span class="combat-phase">{{ resolved ? '结算完成' : 'CAP 拦截' }}</span>
    </header>

    <div class="combat-stage">
      <div class="combat-side attacker">
        <span>攻击编队 · {{ report.attacker_faction }}</span>
        <strong>{{ report.attacker_models.join(' / ') }}</strong>
        <small>{{ report.attacker_count }} 个航空算子</small>
      </div>
      <div class="combat-side defender">
        <span>CAP 防御 · {{ report.defender_faction }}</span>
        <strong>{{ report.defender_models.join(' / ') }}</strong>
        <small>{{ report.defender_count }} 个航空算子</small>
      </div>

      <div class="combat-formation formation-left" aria-hidden="true">
        <svg v-for="index in Math.min(3, report.attacker_count)" :key="index" viewBox="0 0 80 80"><path d="M36 8h8l3 22 23 12v6l-24-6-2 12 9 5v4l-13-3-13 3v-4l9-5-2-12-24 6v-6l23-12Z" /></svg>
      </div>
      <div class="combat-formation formation-right" aria-hidden="true">
        <svg v-for="index in Math.min(3, report.defender_count)" :key="index" viewBox="0 0 80 80"><path d="M36 8h8l3 22 23 12v6l-24-6-2 12 9 5v4l-13-3-13 3v-4l9-5-2-12-24 6v-6l23-12Z" /></svg>
      </div>
      <div class="combat-contact" aria-hidden="true"><i></i><i></i><i></i><b>VS</b></div>
      <div class="combat-scan" aria-hidden="true"></div>
    </div>

    <div v-if="!resolved" class="combat-waiting">
      <span class="combat-loader"></span>
      <div><strong>正在结算空中交战</strong><small>攻击机、护航战斗机与 CAP 正在按照服务端判定交战</small></div>
    </div>

    <section v-else class="combat-result">
      <span class="section-kicker">AUTHORITATIVE RESULT</span>
      <h2>{{ winnerText }}</h2>
      <p>{{ report.summary }}</p>
      <div class="combat-result-grid">
        <div><span>攻击方损失</span><strong>{{ report.attacker_losses }}</strong></div>
        <div><span>CAP 损失</span><strong>{{ report.defender_losses }}</strong></div>
        <div><span>防空命中</span><strong>{{ report.aa_hits }}</strong></div>
        <div><span>投弹命中</span><strong>{{ report.strike_hits }}</strong></div>
      </div>
      <button class="apply-button" @click="emit('close')">返回大地图</button>
    </section>
  </section>
</template>

<style scoped>
.air-combat-view{position:relative;display:flex;flex-direction:column;width:100%;height:100%;min-height:0;padding:28px 38px;overflow:hidden;background:#09171c;color:#dce7e3}.combat-header{display:flex;align-items:flex-start;justify-content:space-between;flex:none;border-bottom:1px solid #30474f;padding-bottom:16px}.combat-header h1{margin:6px 0 0;font-size:28px;letter-spacing:2px}.combat-phase{padding:5px 10px;border:1px solid #8b774d;color:var(--gold);font-size:10px}.combat-stage{position:relative;flex:1;min-height:340px;overflow:hidden}.combat-stage:before{content:'';position:absolute;inset:0;background-image:linear-gradient(#27414a33 1px,transparent 1px),linear-gradient(90deg,#27414a33 1px,transparent 1px);background-size:48px 48px}.combat-side{position:absolute;top:28px;z-index:3;display:flex;flex-direction:column;gap:5px}.combat-side.attacker{left:0}.combat-side.defender{right:0;text-align:right}.combat-side span{color:#7f9da3;font-size:10px;letter-spacing:1px}.combat-side strong{color:#d8e5e1;font-size:16px}.combat-side small{color:#718e94;font-size:10px}.combat-formation{position:absolute;top:42%;z-index:2;display:flex;gap:20px;transition:1.5s ease-out}.combat-formation svg{width:88px;height:88px;fill:#83bed0;filter:drop-shadow(0 0 12px #65bad044)}.formation-left{left:5%;transform:translateY(-50%)}.formation-right{right:5%;transform:translateY(-50%) scaleX(-1)}.formation-right svg{fill:#d7bb7b}.resolved .formation-left{transform:translate(14vw,-50%)}.resolved .formation-right{transform:translate(-14vw,-50%) scaleX(-1)}.combat-contact{position:absolute;left:50%;top:46%;z-index:3;width:90px;height:90px;transform:translate(-50%,-50%)}.combat-contact b{position:absolute;inset:0;display:grid;place-items:center;color:#e5cf91;font:600 24px 'Rajdhani',sans-serif}.combat-contact i{position:absolute;left:50%;top:50%;width:5px;height:5px;border-radius:50%;background:#e8cc83;box-shadow:0 0 16px #e8cc83}.combat-contact i:nth-child(1){animation:tracer-one 1s infinite}.combat-contact i:nth-child(2){animation:tracer-two 1.2s infinite}.combat-contact i:nth-child(3){animation:tracer-three .9s infinite}.resolved .combat-contact{animation:contact-flash .7s ease-out}.combat-scan{position:absolute;left:50%;top:46%;width:280px;height:280px;border:1px solid #43616a;border-radius:50%;transform:translate(-50%,-50%);animation:scan-pulse 1.8s infinite}.combat-waiting{display:flex;align-items:center;justify-content:center;gap:14px;flex:none;min-height:88px;border-top:1px solid #2b444c}.combat-waiting>div{display:flex;flex-direction:column;gap:5px}.combat-waiting strong{font-size:12px;font-weight:500}.combat-waiting small{color:#728f95;font-size:9px}.combat-loader{width:18px;height:18px;border:2px solid #385761;border-top-color:var(--gold);border-radius:50%;animation:spin 1s linear infinite}.combat-result{flex:none;padding:22px 26px;border:1px solid #5b6048;background:#13262c;text-align:center}.combat-result h2{margin:8px 0;color:#ead69b;font-size:22px}.combat-result p{margin:0 0 18px;color:#91a8aa;font-size:11px}.combat-result-grid{display:grid;grid-template-columns:repeat(4,1fr);margin-bottom:20px;border-top:1px solid #334a51;border-bottom:1px solid #334a51}.combat-result-grid>div{display:flex;flex-direction:column;gap:5px;padding:14px;border-right:1px solid #334a51}.combat-result-grid>div:last-child{border:0}.combat-result-grid span{color:#76959a;font-size:9px}.combat-result-grid strong{color:#dce7e3;font:600 25px 'Rajdhani',sans-serif}.combat-result button{min-width:160px;padding:10px 18px}
@keyframes spin{to{transform:rotate(360deg)}}@keyframes scan-pulse{0%{opacity:.8;transform:translate(-50%,-50%) scale(.4)}100%{opacity:0;transform:translate(-50%,-50%) scale(1.5)}}@keyframes tracer-one{0%{transform:translate(-190px,-60px);opacity:0}40%{opacity:1}100%{transform:translate(0,0);opacity:0}}@keyframes tracer-two{0%{transform:translate(180px,50px);opacity:0}40%{opacity:1}100%{transform:translate(0,0);opacity:0}}@keyframes tracer-three{0%{transform:translate(-170px,80px);opacity:0}40%{opacity:1}100%{transform:translate(0,0);opacity:0}}@keyframes contact-flash{0%{filter:brightness(3);transform:translate(-50%,-50%) scale(1.4)}100%{filter:brightness(1);transform:translate(-50%,-50%) scale(1)}}
</style>
