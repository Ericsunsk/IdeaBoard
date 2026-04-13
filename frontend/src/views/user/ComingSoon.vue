<template>
  <div class="coming-soon">
    <n-result status="info" title="功能开发中" :description="desc">
      <template #footer>
        <n-button type="primary" @click="goHome">返回文生图</n-button>
      </template>
    </n-result>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { NResult, NButton } from 'naive-ui'

const router = useRouter()
const route = useRoute()

const nameMap: Record<string, string> = {
  'pro': '专业模式',
  'sketch-gen': '线稿生成',
  'sketch-to-design': '线稿成款',
  'partial-edit': '局部改款',
  'style-innovate': '款式创新',
  'style-fusion': '风格融合',
  'series-color': '系列配色',
  'directed-recolor': '定向换色',
  'fabric-on-body': '面料上身',
  'pattern-on-body': '图案上身',
  'multi-angle': '模特多角度',
  'virtual-3d': '虚拟 3D',
  'mannequin-to-model': '人台变模特',
  'face-swap': '定向换脸',
  'bg-swap': '换背景',
  'garment-on-model': '款式上身',
  'pattern-gen': '图案生成',
  'style-clone': '画风复刻',
  'seamless-tile': '四方连续',
  'pattern-fusion': '图案融合',
  'craft-fusion': '工艺融合',
  'pattern-inpaint': '图案涂改',
  'pattern-upscale': '图案变清晰',
  'to-vector': '转矢量图',
  'remove-bg': '抠图褪底',
  'hd-upscale': '高清放大',
  'deblur': '模糊变清晰',
  'canvas-extend': '画布延伸',
  'my-works': '我的作品',
}

const desc = computed(() => {
  const slug = route.params.slug as string
  const name = nameMap[slug] || slug || '该功能'
  return `「${name}」正在开发中，敬请期待`
})

function goHome() {
  router.push('/user/picture')
}
</script>

<style scoped>
.coming-soon {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #111113;
}
</style>
