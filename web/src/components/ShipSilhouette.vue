<script setup lang="ts">
import { computed } from 'vue'
import { shipMaskURL } from '../silhouette-masks'

// 素材只认服务端下发的 picture（舰船卡里的 content/ 相对路径）：
// 前端不再画备用剪影，也不按舰名猜文件——缺图说明内容目录要补，界面就留空。
const props = withDefaults(defineProps<{ picture?: string; detail?: boolean }>(), { detail: false })
const mask = computed(() => shipMaskURL(props.picture))
</script>

<template>
  <span
    v-if="mask"
    class="ship-silhouette-mask"
    aria-hidden="true"
    :style="{ maskImage: 'url(\'' + mask + '\')', WebkitMaskImage: 'url(\'' + mask + '\')' }"
  ></span>
</template>

<style>
/* 识别图蒙版：currentColor 填充，阵营色/损伤样式自动跟随 */
.ship-silhouette-mask{display:block;width:100%;height:100%;background:currentColor;mask-repeat:no-repeat;mask-position:center bottom;mask-size:contain;-webkit-mask-repeat:no-repeat;-webkit-mask-position:center bottom;-webkit-mask-size:contain}
</style>
