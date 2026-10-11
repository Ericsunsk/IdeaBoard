<template>
  <component :is="props.as" :class="cn(adminMonoClass, props.class)">
    <slot />
  </component>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { adminMonoClass } from '../core/cellClasses'

// 数字 / 金额 / 日期 / 单号单元格原子：只收口 font-mono，
// 字号 / 颜色 / 粗细 / tabular / nowrap 由调用方 class 透传（零行为迁移）。
// 非 prop attrs（title / datetime）透传到根元素。
const props = withDefaults(
  defineProps<{
    as?: 'span' | 'time' | 'div' | undefined
    class?: HTMLAttributes['class'] | undefined
  }>(),
  {
    as: 'span',
    class: undefined,
  },
)
</script>
