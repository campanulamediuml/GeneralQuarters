<script setup lang="ts">
import { computed } from 'vue'
import type { GroundAirStatus } from '../types'
import { formatNumber } from '../format'

const props = defineProps<{ airfield: GroundAirStatus }>()
const typeLabel = computed(() => ({ field: '野战机场', heavy: '重型机场', major: '大型机场' } as Record<string, string>)[props.airfield.type] || '机场')
</script>

<template>
  <figure class="airfield-counter" :data-airfield-id="airfield.unit_id" :aria-label="`${airfield.name}机场算子`">
    <header class="af-top">
      <strong>{{ airfield.runways }}</strong>
      <span class="af-radar" :class="{ on: airfield.radar }" :title="airfield.radar ? '装备对空搜索雷达' : '无雷达'"></span>
      <b>{{ airfield.strength }}</b>
    </header>
    <div class="af-middle">
      <svg class="af-image" viewBox="0 0 120 34" aria-hidden="true">
        <path class="af-runway" d="M4 22h112v7H4z" />
        <path class="af-stripes" d="M16 22v7M38 22v7M60 22v7M82 22v7M104 22v7" />
        <path class="af-tower" d="M78 8h14v14H78zM82 2h6v6h-6z" />
        <path class="af-hangar" d="M12 13h26v9H12z" />
        <path class="af-hangar" d="M44 16h18v6H44z" />
      </svg>
      <span class="af-type">{{ typeLabel }}</span>
    </div>
    <footer class="af-bottom">
      <span title="重型/轻型防空">{{ formatNumber(airfield.heavy_aa) }}-{{ formatNumber(airfield.light_aa) }}</span>
      <b v-if="airfield.coastal" class="af-coastal" title="岸防炮">{{ airfield.coastal }}</b>
      <i v-else></i>
      <strong class="af-hangar" title="机库内算子">{{ airfield.hangar_used }}</strong>
    </footer>
    <figcaption>{{ airfield.name }}</figcaption>
  </figure>
</template>

<style scoped>
.airfield-counter{display:block;width:126px;margin:0;color:#1f2a14;text-align:left;filter:drop-shadow(0 3px 3px #0005)}
.airfield-counter>header,.af-middle,.af-bottom{border:1px solid #cbbf7e;background:linear-gradient(145deg,#d9d2a3,#a9a071)}
.airfield-counter>header{border-bottom:0;border-radius:3px 3px 0 0}
.af-top{display:flex;align-items:center;justify-content:space-between;padding:2px 6px;font:700 18px 'Rajdhani',sans-serif}
.af-radar{width:11px;height:11px;border-radius:50%;background:#6d6a53;opacity:.45}
.af-radar.on{background:#187cd0;opacity:1}
.af-middle{position:relative;height:42px;display:flex;align-items:center;justify-content:center;border-left:1px solid #cbbf7e;border-right:1px solid #cbbf7e}
.af-image{width:112px;height:32px}
.af-runway{fill:#4c4a35}.af-stripes{stroke:#e6dfb4;stroke-width:1.1;opacity:.5}.af-tower{fill:#2f3524}.af-hangar{fill:#3c4130}
.af-type{position:absolute;right:5px;top:1px;opacity:.7;font:500 8px 'Noto Sans SC',sans-serif}
.af-bottom{display:flex;align-items:center;justify-content:space-between;padding:1px 6px;border-top:0;border-radius:0 0 3px 3px;font:700 15px 'Rajdhani',sans-serif}
.af-coastal{background:#8a6d2c;color:#fff4cf;padding:0 4px}
.airfield-counter figcaption{overflow:hidden;padding:3px 2px 0;color:#d6d2b4;font-size:10px;text-align:center;text-overflow:ellipsis;white-space:nowrap}
</style>
