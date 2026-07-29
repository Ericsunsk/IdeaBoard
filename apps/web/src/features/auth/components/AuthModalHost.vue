<template>
  <Dialog
    ref="dialogRef"
    unstyled
    visible
    modal
    :dismissable-mask="canDismiss('mask')"
    :closable="false"
    :close-on-escape="canDismiss('escape')"
    :draggable="false"
    :show-header="false"
    :pt="authDialogPt"
    :aria-busy="ariaBusy"
    @update:visible="handleVisibleChange"
    @hide="disableVendorFocusRestore"
    @after-hide="completeClose"
  >
    <slot />
  </Dialog>
</template>

<script setup lang="ts">
import Dialog from 'primevue/dialog'
import { usePrimeDialogOverlayLifecycle } from '../../../shared/composables/usePrimeDialogOverlayLifecycle'
import { createAuthDialogPt } from './authModalUi'

const props = defineProps<{
  busy: boolean
}>()

const emit = defineEmits<{
  close: []
}>()

const authDialogPt = createAuthDialogPt()
const {
  ariaBusy,
  canDismiss,
  completeClose,
  dialogRef,
  disableVendorFocusRestore,
  handleVisibilityChange,
} = usePrimeDialogOverlayLifecycle({
  visible: () => true,
  busy: () => props.busy,
  surface: {
    dismissibleBy: ['close-control', 'escape', 'mask'],
    restoreFocus: true,
  },
  onVisibilityChange: (visible) => {
    if (!visible) emit('close')
  },
})

function handleVisibleChange(visible: boolean) {
  handleVisibilityChange(visible)
}
</script>
