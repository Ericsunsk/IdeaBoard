<template>
  <Dialog
    ref="dialogRef"
    :visible="visible"
    modal
    :dismissable-mask="canDismiss('mask')"
    :close-on-escape="canDismiss('escape')"
    :show-header="false"
    :draggable="false"
    :style="accountManagementDialogStyle"
    :pt="accountManagementDialogPt"
    :aria-busy="ariaBusy"
    aria-label="账户管理"
    @update:visible="handleVisibilityChange"
    @hide="disableVendorFocusRestore"
    @after-hide="completeClose"
  >
    <AccountManagementDialogContent
      v-if="visible"
      :initial-tab="initialTab"
      @close="requestOverlayClose('close-control')"
      @update:busy="busy = $event"
    />
  </Dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import Dialog from 'primevue/dialog'
import type { DialogPassThroughOptions } from 'primevue/dialog'
import { usePrimeDialogOverlayLifecycle } from '../../../shared/composables/usePrimeDialogOverlayLifecycle'
import { ibDialogMaskPt } from '../../../shared/theme/dialogMask'
import { createDialogMotionPt } from '../../../shared/motion/dialogMotion'
import { defaultAccountManagementTab, type AccountManagementTab } from '../accountTabRegistry'
import AccountManagementDialogContent from './AccountManagementDialogContent.vue'

const props = withDefaults(
  defineProps<{
    visible: boolean
    initialTab?: AccountManagementTab
  }>(),
  { initialTab: defaultAccountManagementTab },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
}>()
const busy = ref(false)
const {
  ariaBusy,
  canDismiss,
  completeClose,
  dialogRef,
  disableVendorFocusRestore,
  handleVisibilityChange,
  requestClose: requestOverlayClose,
} = usePrimeDialogOverlayLifecycle({
  visible: () => props.visible,
  busy,
  surface: {
    dismissibleBy: ['close-control', 'escape'],
    restoreFocus: true,
  },
  onReset: () => {
    busy.value = false
  },
  onVisibilityChange: (visible) => emit('update:visible', visible),
})

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
