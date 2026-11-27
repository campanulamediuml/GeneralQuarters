<script setup lang="ts">
import { computed } from 'vue'
import type { GroundAirStatus } from '../types'
import { formatNumber } from '../format'
import { airfieldPictureURL, airfieldTypeHint, airfieldTypeLabel, normalizeAirfieldType } from '../airfields'

// compact：地图上和舰队挤在同一格时用的窄条版本，砍掉中间的机型示意图，
// 只留跑道/雷达/生命值、防空/岸炮和名字，把格子中间让给舰队兵牌。
const props = defineProps<{ airfield: GroundAirStatus; compact?: boolean }>()
const typeLabel = computed(() => airfieldTypeLabel(props.airfield.type))
const isSeaplane = computed(() => normalizeAirfieldType(props.airfield.type) === 'seaplane')
</script>

<template>
  <figure class="airfield-counter" :class="{ compact }" :data-faction="airfield.faction" :data-airfield-id="airfield.unit_id" :aria-label="`${airfield.name}机场算子`">
    <header class="af-top">
      <strong v-if="isSeaplane" class="af-seaplane" title="水上飞机基地没有跑道，飞机直接在泊地起降">水机</strong>
      <strong v-else :title="`可用跑道 ${airfield.runways} / 总跑道 ${airfield.max_runways}；可用跑道 = ceil(生命值 / 5)，不超过总跑道`">{{ airfield.runways }}<small>/{{ airfield.max_runways }}</small></strong>
      <span class="af-radar" :class="{ on: airfield.radar }" :title="airfield.radar ? '装备对空搜索雷达' : '无雷达'"></span>
      <b :title="`生命值 ${airfield.strength} / ${airfield.max_strength}`">{{ airfield.strength }}<small>/{{ airfield.max_strength }}</small></b>
    </header>
    <div class="af-middle" :class="{ 'has-supplies': airfield.enemy_supplies }" :title="airfieldTypeHint(airfield.type)">
      <b v-if="airfield.enemy_supplies" class="af-enemy-supplies" :title="`敌方已卸下 ${airfield.enemy_supplies} 点滩头物资：维修停摆，炮击清空后才恢复维修`">{{ airfield.enemy_supplies }}</b>
      <img v-if="airfieldPictureURL(airfield.picture)" class="af-image-picture" :src="airfieldPictureURL(airfield.picture)" alt="" :data-airfield-picture="airfield.picture" />
      <svg v-else class="af-image" viewBox="0 0 120 34" aria-hidden="true">
        <path class="af-runway" d="M4 22h112v7H4z" />
        <path class="af-stripes" d="M16 22v7M38 22v7M60 22v7M82 22v7M104 22v7" />
        <path class="af-tower" d="M78 8h14v14H78zM82 2h6v6h-6z" />
        <path class="af-hangar" d="M12 13h26v9H12z" />
        <path class="af-hangar" d="M44 16h18v6H44z" />
      </svg>
      <span class="af-type">{{ typeLabel }}</span>
    </div>
    <footer class="af-bottom">
      <span :title="`重型防空 ${formatNumber(airfield.heavy_aa)} / 轻型防空 ${formatNumber(airfield.light_aa)}`">{{ formatNumber(airfield.heavy_aa) }}-{{ formatNumber(airfield.light_aa) }}</span>
      <strong class="af-coastal" :title="`岸炮 ${airfield.coastal} 点；每点 30% 命中随机舰船，舰队岸轰/空袭都会削减`">{{ airfield.coastal }}</strong>
    </footer>
    <figcaption>{{ airfield.name }}</figcaption>
  </figure>
</template>

<style scoped>
.airfield-counter{--af-border:#cbbf7e;--af-bg1:#d9d2a3;--af-bg2:#a9a071;--af-fg:#1f2a14;--af-muted:#3a3524;--af-ink:#2f3524;display:block;width:126px;margin:0;color:var(--af-fg);text-align:left;filter:drop-shadow(0 3px 3px #0005)}
/* 机场卡按阵营上色：日军粉色、美军浅蓝色，和舰船兵牌的美军蓝/日军红保持同一套色系。 */
.airfield-counter[data-faction="B"]{--af-border:#dfa9b8;--af-bg1:#f2d3dc;--af-bg2:#cf9aa9;--af-fg:#3a2027;--af-muted:#5c3641;--af-ink:#47242d}
.airfield-counter[data-faction="A"]{--af-border:#a6c9dd;--af-bg1:#dcecf7;--af-bg2:#a9c9df;--af-fg:#162a36;--af-muted:#2f4a5c;--af-ink:#1d3b4c}
.airfield-counter>header,.af-middle,.af-bottom{border:1px solid var(--af-border);background:linear-gradient(145deg,var(--af-bg1),var(--af-bg2))}
.airfield-counter>header{border-bottom:0;border-radius:3px 3px 0 0}
.af-top{display:flex;align-items:center;justify-content:space-between;padding:2px 6px;font:700 18px 'Rajdhani',sans-serif}
.af-top small{font-size:9px;font-weight:500;opacity:.62}
.af-seaplane{font:700 12px 'Noto Sans SC',sans-serif;letter-spacing:1px}
.af-coastal{color:var(--af-muted)}
.af-radar{width:11px;height:11px;border-radius:50%;background:#6d6a53;opacity:.45}
.af-radar.on{background:#187cd0;opacity:1}
.af-middle{position:relative;height:42px;display:flex;align-items:center;justify-content:center;border-left:1px solid var(--af-border);border-right:1px solid var(--af-border)}
.af-image{width:112px;height:32px}
.af-image-picture{width:34px;height:34px;object-fit:contain}
.af-runway{fill:var(--af-ink)}.af-stripes{stroke:#fff;stroke-width:1.1;opacity:.5}.af-tower{fill:var(--af-ink)}.af-hangar{fill:var(--af-ink)}
.af-type{position:absolute;right:5px;top:1px;opacity:.7;font:500 8px 'Noto Sans SC',sans-serif}.af-enemy-supplies{position:absolute;left:4px;top:0;color:#b3121f;font:800 15px 'Rajdhani',sans-serif;text-shadow:0 0 2px #fff;pointer-events:none}
.af-bottom{display:flex;align-items:center;justify-content:space-between;padding:1px 6px;border-top:0;border-radius:0 0 3px 3px;font:700 15px 'Rajdhani',sans-serif}
.airfield-counter figcaption{overflow:hidden;padding:3px 2px 0;color:#c3d4d0;font-size:10px;text-align:center;text-overflow:ellipsis;white-space:nowrap}
/* 窄条模式：机场和舰队同格时用。中间那栏 42px 高的示意图正好把 52×36 的兵牌整块盖住，
   砍掉它以后卡片只剩两行数字加名字，能顶到格子上沿、把中间让给舰队。 */
.airfield-counter.compact>header{border-bottom:1px solid var(--af-border)}
.airfield-counter.compact .af-middle{display:none}
.airfield-counter.compact .af-middle.has-supplies{display:block;height:auto;padding:1px 0 0;border-top:0}
.airfield-counter.compact .af-middle .af-image,
.airfield-counter.compact .af-middle .af-image-picture,
.airfield-counter.compact .af-middle .af-type{display:none}
.airfield-counter.compact .af-enemy-supplies{position:static;display:block;text-align:center}
.airfield-counter.compact figcaption{padding-top:1px}
</style>
