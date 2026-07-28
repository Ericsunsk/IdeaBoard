<template>
  <Dialog
    :visible="visible"
    modal
    :dismissable-mask="false"
    close-on-escape
    :show-header="false"
    :draggable="false"
    :style="accountManagementDialogStyle"
    :pt="accountManagementDialogPt"
    aria-label="账户管理"
    @update:visible="emit('update:visible', $event)"
  >
    <AccountManagementDialogContent
      v-if="visible"
      :initial-tab="initialTab"
      @close="emit('update:visible', false)"
    />
  </Dialog>
</template>

<script setup lang="ts">
import Dialog from 'primevue/dialog'
import type { DialogPassThroughOptions } from 'primevue/dialog'
import { ibDialogMaskPt } from '../../../shared/theme/dialogMask'
import { createDialogMotionPt } from '../../../shared/motion/dialogMotion'
import { defaultAccountManagementTab, type AccountManagementTab } from '../accountTabRegistry'
import AccountManagementDialogContent from './AccountManagementDialogContent.vue'

withDefaults(
  defineProps<{
    visible: boolean
    initialTab?: AccountManagementTab
  }>(),
  { initialTab: defaultAccountManagementTab },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
}>()

const accountManagementDialogStyle = {
  width: 'min(1120px, calc(100vw - 32px))',
  '--ib-dialog-motion-from-transform': 'translateY(8px) scale(0.99)',
} as const

const accountManagementDialogPt = {
  ...ibDialogMaskPt,
  root: {
    class:
      '!max-h-[calc(100dvh-32px)] !overflow-hidden !rounded-[var(--radius-ib-overlay)] !border !border-ib-border-soft !bg-[var(--color-ib-surface)] !shadow-[var(--shadow-ib-overlay)] max-[640px]:!max-h-[calc(100dvh-20px)]',
  },
  content: {
    class: '!overflow-hidden !bg-[var(--color-ib-surface)] !p-0',
  },
  ...createDialogMotionPt(),
} satisfies DialogPassThroughOptions
</script>
