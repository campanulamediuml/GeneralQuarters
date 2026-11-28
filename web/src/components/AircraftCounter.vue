<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { AirUnit, Player } from '../types'
import { aircraftPictureURL } from '../silhouette-masks'
import { label } from '../display'

// cycleClickable 默认 true：布尔型 prop 缺省时 Vue 会补成 false，必须用 withDefaults 显式给默认值，
// 否则没传这个属性的地方（地图上的兵牌）循环角标会被渲染成拉不动的 static 按钮。
const props = withDefaults(defineProps<{ aircraft: AirUnit; playerFaction: Player['faction']; stackCount?: number; cycleClickable?: boolean }>(), { cycleClickable: true })
const emit = defineEmits<{ (event: 'toggle-cycle', aircraft: AirUnit): void }>()
const friendly = computed(() => props.aircraft.faction === props.playerFaction)
const range = computed(() => friendly.value ? props.aircraft.remaining_range : props.aircraft.max_range)
const modelCode = computed(() => props.aircraft.model.split(' ')[0])
// 兵牌图片只认服务端下发的 picture（units/*.json 的 aircraft[].picture → /content/...）：
// 飞机默认按彩图原样渲染，picture_mode=mask 时才切成单色剪影。
// 前端不再内置矢量备用图：缺图说明内容目录要补，界面就留空。
const pictureURL = computed(() => aircraftPictureURL(props.aircraft.picture))
const pictureMode = computed(() => props.aircraft.picture_mode === 'mask' ? 'mask' : 'image')
const pictureBroken = ref(false)
watch(pictureURL, (url) => {
  pictureBroken.value = false
  if (!url || typeof Image === 'undefined') return
  const image = new Image()
  image.onerror = () => {
    if (pictureURL.value === url) pictureBroken.value = true
  }
  image.src = url
}, { immediate: true })
const usesPicture = computed(() => Boolean(pictureURL.value) && !pictureBroken.value)
// 兵牌上的剪影逐机型不同，用 title 标出机型，方便一眼区分俯冲轰炸机、鱼雷机和水上侦察机。
// 陆基机型（陆攻、P-40、B-25）不能上舰，标题和航程样式都按陆基来。
// 机型名是"枚举 → 文案"的映射，文案表由服务端下发（content/misc/display.json）。
const kindLabel = computed(() => label(props.aircraft.airfield_only ? 'aircraft_kind_ground' : 'aircraft_kind', props.aircraft.kind))
// 任务循环：CAP、侦察、空中掩护可以自动再来一轮（规则书 188–191 行）。
// 蓝色 🔁 表示循环已开启；灰的表示这次任务不会自动续，点一下可以打开。
const cycleKind = computed(() => props.aircraft.cycle_kind || props.aircraft.task_kind)
const canCycle = computed(() => friendly.value && ['cap', 'cover', 'recon'].includes(cycleKind.value || '') && props.aircraft.status !== 'attack')
const cycleOn = computed(() => Boolean(props.aircraft.on_cycle))
const cycleLabel = computed(() => (cycleOn.value ? '自动循环已开启，点击取消' : '自动循环已关闭，点击开启'))
// 飞行员熟练度：普通不画角标，新/老/精三档各占一个字，放在机型名前面。
// 熟练度不改卡面数值，只决定 DC 临场发挥那一掷的骰子档位，角标负责让人一眼看出这队是谁在飞。
// 角标一个字、说明里的档位名，同样查服务端文案表；默认按"普通"。
const skillKey = computed(() => props.aircraft.skill || 'regular')
const skillBadge = computed(() => (props.aircraft.skill ? label('skill_badge', props.aircraft.skill) : ''))
const skillLabel = computed(() => label('skill_label', skillKey.value))
// 档位名和规则书同一套：无加成 / 2 骰劣势 / 单骰 / 2 骰优势。
const skillTier = computed(() => label('skill_tier', skillKey.value))
const modelTitle = computed(() => skillBadge.value ? `${props.aircraft.model} · 飞行员：${skillLabel.value}（${skillTier.value}）` : props.aircraft.model)
</script>

<template>
  <figure
    class="air-counter"
    :class="[`air-${aircraft.status}`, `kind-${aircraft.kind}`, { seaplane: aircraft.seaplane }]"
    :data-range-kind="friendly ? 'remaining' : 'maximum'"
    :data-range-value="range"
    :data-stack-count="stackCount || 1"
    :aria-label="`${aircraft.model}${stackCount && stackCount > 1 ? `，${stackCount}个算子` : ''}，对空${aircraft.air_combat}，对地${aircraft.anti_ground}，对舰${aircraft.anti_ship}，机体${aircraft.strength}，${friendly ? '剩余' : '最大'}航程${range}`"
    :title="`${aircraft.model} · ${kindLabel}`"
  >
    <div class="aircraft-ratings">
      <span title="对空能力">{{ aircraft.air_combat }}</span>
      <span title="对地能力">{{ aircraft.anti_ground }}</span>
      <span title="对舰攻击">{{ aircraft.anti_ship }}</span>
    </div>
    <div class="aircraft-body">
      <span class="aircraft-strength" title="机体强度">{{ aircraft.strength }}</span>
      <img
        v-if="usesPicture && pictureMode === 'image'"
        class="aircraft-symbol aircraft-picture-image"
        :src="pictureURL"
        alt=""
        :data-aircraft-kind="aircraft.kind"
        :data-aircraft-picture="aircraft.picture"
        @error="pictureBroken = true"
      />
      <span
        v-else-if="usesPicture"
        class="aircraft-symbol aircraft-picture"
        aria-hidden="true"
        :data-aircraft-kind="aircraft.kind"
        :data-aircraft-picture="aircraft.picture"
        :style="{ '--aircraft-picture': `url('${pictureURL}')` }"
      />
      <span class="aircraft-loadout" title="挂载：未配置">—</span>
    </div>
    <div class="aircraft-footer">
      <strong
        class="aircraft-range"
        :class="{ 'carrier-based': !aircraft.seaplane && !aircraft.airfield_only, 'low-range': friendly && aircraft.remaining_range <= 1 }"
        :title="friendly ? '剩余航程' : '此挂载下最大航程'"
      >{{ range }}</strong>
      <span class="aircraft-model" :title="modelTitle"><i v-if="skillBadge" class="aircraft-skill" :data-skill="aircraft.skill">{{ skillBadge }}</i>{{ modelCode }}</span>
    </div>
    <span v-if="stackCount && stackCount > 1" class="aircraft-stack-count" :title="`${stackCount}个同型算子`">×{{ stackCount }}</span>
    <span
      v-if="aircraft.damaged || (aircraft.maintenance || 0) > 0"
      class="aircraft-damaged"
      :title="aircraft.damaged ? '机体损伤：必须修复后才能再次出动' : `检修中：还需 ${aircraft.maintenance} 回合`"
    >损</span>
    <button
      v-if="canCycle"
      type="button"
      class="aircraft-cycle"
      :class="{ on: cycleOn, static: cycleClickable === false }"
      :title="cycleLabel"
      :aria-label="cycleLabel"
      :aria-pressed="cycleOn"
      :disabled="cycleClickable === false"
      @click.stop.prevent="cycleClickable !== false && emit('toggle-cycle', aircraft)"
      @mousedown.stop
      @dragstart.stop.prevent
    >🔁</button>
  </figure>
</template>

<style scoped>
.air-counter{position:relative;display:grid;grid-template-rows:18px 43px 22px;width:84px;height:85px;margin:0;overflow:hidden;border:1px solid #89aab5;border-radius:2px;background:linear-gradient(145deg,#d6e2e2,#9db6bd);color:#132a33;box-shadow:0 2px 4px #0005}
.air-counter.seaplane{background:linear-gradient(145deg,#edba73,#bf7d38);border-color:#e5b675}
.aircraft-ratings{display:grid;grid-template-columns:repeat(3,1fr);align-items:center;padding:0 4px;font:700 13px/1 'Rajdhani',sans-serif;text-align:center}
.aircraft-body{display:grid;grid-template-columns:14px minmax(0,1fr) 14px;align-items:center;min-height:0}
.aircraft-strength{align-self:start;padding-top:3px;font:700 12px/1 'Rajdhani',sans-serif;text-align:center}
.aircraft-symbol{width:100%;height:40px;color:#245b72;fill:currentColor;stroke:none}
.aircraft-picture-image{display:block;width:100%;height:40px;object-fit:contain;object-position:center}
.aircraft-picture{display:block;background:currentColor;-webkit-mask-image:var(--aircraft-picture);mask-image:var(--aircraft-picture);-webkit-mask-repeat:no-repeat;mask-repeat:no-repeat;-webkit-mask-position:center;mask-position:center;-webkit-mask-size:contain;mask-size:contain}
.aircraft-symbol .aircraft-floats{fill:color-mix(in srgb,#245b72 70%,#fff)}
.seaplane .aircraft-symbol{color:#70441f}
.aircraft-loadout{align-self:start;padding-top:3px;color:#42565b;font:10px/1 sans-serif;text-align:center}
.aircraft-footer{display:grid;grid-template-columns:25px minmax(0,1fr);align-items:end;gap:3px;padding:0 4px 3px}
.aircraft-range{display:grid;place-items:center;width:25px;height:20px;background:#426b7b;color:#eef5f3;font:700 14px/1 'Rajdhani',sans-serif}
.aircraft-range.carrier-based{background:#187caf}
.aircraft-range.low-range{background:#9c4036}
.aircraft-model{display:flex;align-items:center;justify-content:flex-end;gap:2px;overflow:hidden;padding-bottom:2px;font:700 9px/1 'Rajdhani',sans-serif;text-align:right;text-overflow:ellipsis;white-space:nowrap}
.aircraft-skill{flex:none;padding:0 2px;border-radius:2px;background:#c9b37a;color:#1a2228;font:700 8px/11px sans-serif;font-style:normal}
.aircraft-skill[data-skill=rookie]{background:#8a3428;color:#f6e7c9}
.aircraft-skill[data-skill=veteran]{background:#7fb3d5;color:#0b2530}
.aircraft-skill[data-skill=ace]{background:#e0b030;color:#2a1a00}
.air-counter.air-cap,.air-counter.air-recon,.air-counter.air-deck,.air-counter.air-ready,.air-counter.air-attack,.air-counter.air-attack_holding{border-color:#f0d58d}.air-counter.air-service,.air-counter.air-elevator{border-style:dashed;border-color:#d7bb7b}.air-counter.air-recovery{border-color:#7ec8db}
.aircraft-stack-count{position:absolute;right:3px;bottom:27px;z-index:2;min-width:25px;padding:3px 4px;border:1px solid #f0d58d;background:#142a31e8;color:#f0d58d;font:700 13px/1 'Rajdhani',sans-serif;text-align:center;box-shadow:0 1px 3px #0007}
.aircraft-cycle{position:absolute;left:3px;top:19px;z-index:2;display:grid;place-items:center;width:17px;height:17px;padding:0;border:1px solid #2f6f96;border-radius:50%;background:#12242be0;color:#7cc4ee;font:11px/1 sans-serif;cursor:pointer;opacity:.55;filter:grayscale(1)}
.aircraft-cycle.on{opacity:1;border-color:#5db6e8;background:#0b3f63f2;color:#9fdcff;filter:none;box-shadow:0 0 6px #4aa8e0aa}
.aircraft-cycle:hover{opacity:1}
.aircraft-cycle.static{cursor:default}
</style>
