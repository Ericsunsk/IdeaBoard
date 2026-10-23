<template>
  <DialogRoot :open="props.isOpen" @update:open="handleOpenUpdate">
    <DialogPortal>
      <DialogOverlay data-dialog-shell-mask :class="props.overlayClass" @click="handleMaskClick" />
      <DialogContent
        :class="cn(dialogShellSizeClasses[props.size], props.contentClass)"
        :show-close="props.hasClose && canDismiss('close-control')"
        :close-label="props.closeLabel"
        :aria-label="props.title"
        :aria-busy="isAriaBusy"
        :data-size="props.size"
        @escape-key-down="handleEscape"
        @pointer-down-outside="blockOutsideDismiss"
        @open-auto-focus="handleOpenAutoFocus"
        @close-auto-focus="handleCloseAutoFocus"
      >
        <DialogTitle class="sr-only">{{ props.title }}</DialogTitle>
        <DialogDescription class="sr-only">
          {{ props.description ?? props.title }}
        </DialogDescription>
        <slot />
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup lang="ts">
import {
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import {
  defineDialogSurfaceIntent,
  useOverlayInteractionLifecycle,
} from '@/core/composables/useOverlayInteractionLifecycle'
import { dialogShellSizeClasses, type DialogShellSize } from './dialogShell'

const props = withDefaults(
  defineProps<{
    isOpen: boolean
    /** Accessible name for the dialog (also the visible title unless the caller renders a custom header). */
    title: string
    description?: string | undefined
    size?: DialogShellSize
    isBusy?: boolean
    hasClose?: boolean
    closeLabel?: string
    contentClass?: string | undefined
    overlayClass?: string | undefined
  }>(),
  {
    size: 'form',
    isBusy: false,
    hasClose: true,
    closeLabel: 'Close',
    contentClass: undefined,
    overlayClass: undefined,
    description: undefined,
  },
)

const emit = defineEmits<{
  'update:open': [isOpen: boolean]
  close: []
  openAutoFocus: [event: Event]
  closeAutoFocus: [event: Event]
}>()

const {
  ariaBusy: isAriaBusy,
  canDismiss,
  completeClose,
  handleVisibilityChange,
  requestClose,
} = useOverlayInteractionLifecycle({
  isVisible: () => props.isOpen,
  busy: () => props.isBusy,
  surface: defineDialogSurfaceIntent(),
  onVisibilityChange: (isOpen) => {
    emit('update:open', isOpen)
    if (!isOpen) emit('close')
  },
})

function handleOpenUpdate(isOpen: boolean) {
  if (!isOpen) {
    if (handleVisibilityChange(false)) completeClose()
  } else {
    handleVisibilityChange(true)
  }
}

function handleMaskClick() {
  if (requestClose('mask')) completeClose()
}

function handleEscape(event: KeyboardEvent) {
  if (!canDismiss('escape')) event.preventDefault()
}

function blockOutsideDismiss(event: Event) {
  event.preventDefault()
}

function handleOpenAutoFocus(event: Event) {
  emit('openAutoFocus', event)
}

function handleCloseAutoFocus(event: Event) {
  completeClose()
  emit('closeAutoFocus', event)
}
</script>
