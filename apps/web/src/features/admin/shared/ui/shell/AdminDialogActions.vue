<template>
  <ActionButton variant="outline" :disabled="cancelDisabled" @click="emit('cancel')">
    {{ cancelLabel }}
  </ActionButton>
  <slot name="extra-actions" />
  <ActionButton
    :variant="confirmVariant"
    :severity="severity"
    :loading="loading"
    :disabled="disabled"
    @click="emit('confirm')"
  >
    {{ confirmLabel }}
  </ActionButton>
</template>

<script setup lang="ts">
import ActionButton from '@/core/components/ActionButton.vue'
import type { AdminButtonVariant } from '../AdminButton.vue'

withDefaults(
  defineProps<{
    cancelLabel?: string | undefined
    confirmLabel?: string | undefined
    loading?: boolean
    disabled?: boolean
    cancelDisabled?: boolean
    confirmVariant?: AdminButtonVariant
    severity?: 'primary' | 'secondary' | 'danger' | 'success' | 'info' | 'warn' | undefined
  }>(),
  {
    // 取消几乎永远叫"取消"，默认即可；i18n 场景（如兑换码弹窗）仍显式传 t()。
    // 确认动作各异，调用方显式传入，此处仅给通用兜底。
    cancelLabel: '取消',
    confirmLabel: '确认',
    loading: false,
    disabled: false,
    cancelDisabled: false,
    confirmVariant: 'default',
    severity: undefined,
  },
)

const emit = defineEmits<{
  cancel: []
  confirm: []
}>()

// 第三动作位（如“测试连接”）：渲染在取消与确认之间，不复用 confirm 通道，勿硬塞。
defineSlots<{
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- slot render output can be any VNode/renderable
  'extra-actions'?: () => any
}>()
</script>
