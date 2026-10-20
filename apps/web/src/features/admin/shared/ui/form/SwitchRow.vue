<template>
  <!-- 单页防抖行原语：status 槽以 statusMinWidth 锁宽（短文案居中不收缩），
    switch 槽 shrink-0 固定位。规则：Switch 永不绑状态 key（同一节点翻转，
    不 remount）；StatusPill 只换色（label/variant/pulse，不重建行）；
    pulse 默认关闭，开启时也只是绝对定位透明度动画，不触发布局。 -->
  <div class="flex items-center" :class="gapClass">
    <span
      class="inline-flex shrink-0 items-center"
      :style="{ minWidth: statusMinWidth }"
      data-testid="switch-row-status-slot"
    >
      <AdminStatusPill
        v-if="pill !== undefined"
        :label="pill"
        :variant="pillVariant"
        :pulse="pulse"
        :size="pillSize"
        class="justify-center whitespace-nowrap"
        :style="{ minWidth: statusMinWidth }"
      />
      <span
        v-else
        class="inline-flex shrink-0"
        :class="pillSize === 'medium' ? 'h-6' : 'h-5'"
        :style="{ minWidth: statusMinWidth }"
        aria-hidden="true"
        data-testid="switch-row-status-placeholder"
      />
    </span>
    <AdminSwitch
      v-if="switchAttrs"
      v-bind="switchAttrs ?? {}"
      :model-value="switchState ?? false"
      class="shrink-0"
      @update:model-value="switchState = $event"
    />
  </div>
</template>

<script setup lang="ts">
import AdminStatusPill, { type AdminStatusPillVariant } from '../feedback/AdminStatusPill.vue'
import AdminSwitch from './AdminSwitch.vue'

export type AdminSwitchRowPillSize = 'small' | 'medium'

withDefaults(
  defineProps<{
    /** Status 文案；undefined 时渲染同宽空占位（加载中/加载失败开关不动）。 */
    pill?: string | undefined
    pillVariant?: AdminStatusPillVariant | undefined
    /** 默认 false：success↔neutral 翻转不挂载/卸载 pulse，少一次 DOM 增删。 */
    pulse?: boolean | undefined
    /** status 槽最小宽（style 绑定，任意值都生效，如 '68px'）。 */
    statusMinWidth: string
    pillSize?: AdminSwitchRowPillSize | undefined
    /** 行内 pill↔switch 间距；邮件头用 gap-2 对齐 header-actions，其余 gap-3。 */
    gapClass?: string | undefined
    /**
     * 透传给 AdminSwitch 的其余 attrs（disabled / aria-label / id …）。
     * 不传则为纯 status 展示，不渲染 switch。开关状态一律走本组件的
     * v-model / update:modelValue，不走 switchAttrs，更不绑状态 key。
     */
    switchAttrs?: Record<string, unknown> | undefined
  }>(),
  {
    pill: undefined,
    pillVariant: 'neutral',
    pulse: false,
    pillSize: 'small',
    gapClass: 'gap-3',
    switchAttrs: undefined,
  },
)

const switchState = defineModel<boolean>('modelValue', { default: false })
</script>
