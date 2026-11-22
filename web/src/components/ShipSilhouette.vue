<script setup lang="ts">
import { computed } from 'vue'
import { shipSilhouette } from '../silhouettes'
import { shipMaskURL } from '../silhouette-masks'

const props = withDefaults(defineProps<{ type?: string; name?: string; picture?: string; detail?: boolean }>(), { detail: false })
const shape = computed(() => shipSilhouette(props.type))
// 剪影路径跟着舰船卡走（content/misc/image/ship/*.png）；卡里没有就按舰名兜底，
// 再没有才回落到手绘 SVG。
const mask = computed(() => shipMaskURL(props.picture, props.name))
</script>

<template>
  <span
    v-if="mask"
    class="ship-silhouette-mask"
    aria-hidden="true"
    :data-ship-type="type"
    :style="{ maskImage: `url(${mask})`, WebkitMaskImage: `${mask}` }"
  ></span>
  <svg v-else class="ship-silhouette-drawing" viewBox="0 0 120 40" preserveAspectRatio="xMidYMid meet" aria-hidden="true" :data-ship-type="type">
    <path class="sil-hull" :d="shape.hull" />
    <path v-for="(part, index) in shape.parts" :key="`${part.tone}-${index}`" :class="`sil-${part.tone}`" :d="part.d" />
    <template v-if="detail">
      <path class="sil-detail" d="M6 31h108" />
    </template>
  </svg>
</template>

<style>
/* 识别图蒙版：currentColor 填充，阵营色/损伤样式自动跟随 */
.ship-silhouette-mask{display:block;width:100%;height:100%;background:currentColor;mask-repeat:no-repeat;mask-position:center bottom;mask-size:contain;-webkit-mask-repeat:no-repeat;-webkit-mask-position:center bottom;-webkit-mask-size:contain}
/* 舰体三色分层：深色舰体、亮色上层建筑、主色飞行甲板 / 炮塔 */
.ship-silhouette-drawing{display:block;width:100%;height:100%;overflow:visible}
.sil-hull{fill:currentColor}
.sil-super{fill:color-mix(in srgb,currentColor 70%,#fff)}
.sil-hangar{fill:color-mix(in srgb,currentColor 82%,#fff)}
.sil-deck{fill:color-mix(in srgb,currentColor 40%,#fff)}
.sil-turret{fill:color-mix(in srgb,currentColor 55%,#fff)}
.sil-funnel{fill:color-mix(in srgb,currentColor 62%,#000)}
.sil-detail{fill:none;stroke:color-mix(in srgb,currentColor 30%,#fff);stroke-width:.8;opacity:.5}
</style>
