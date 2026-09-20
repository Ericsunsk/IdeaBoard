<template>
  <DialogRoot :open="true" @update:open="handleOpenUpdate">
    <DialogPortal>
      <DialogOverlay :class="ibDialogMaskClass" @click="handleMaskClick" />
      <DialogContent
        :class="cn(dialogContentBaseClass, authDialogContentClass)"
        :aria-label="props.ariaLabel"
        :aria-busy="props.busy || undefined"
        @escape-key-down="handleEscape"
        @pointer-down-outside="blockOutsideDismiss"
      >
        <div class="min-h-0 overflow-y-auto">
          <DialogDescription v-if="props.description" class="sr-only">
            {{ props.description }}
          </DialogDescription>
          <slot />
        </div>
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
} from '@/components/ui/dialog'
import { dialogContentBaseClass } from '@/components/ui/dialog/dialogClasses'
import { cn } from '@/lib/utils'
import { ibDialogMaskClass } from '../../../shared/theme/dialogMask'
import { useOverlayInteractionLifecycle } from '../../../shared/composables/useOverlayInteractionLifecycle'
import { authDialogContentClass } from './modalUi'

const props = withDefaults(
  defineProps<{
    busy: boolean
    ariaLabel?: string | undefined
    description?: string | undefined
  }>(),
  { ariaLabel: undefined, description: undefined },
)

const emit = defineEmits<{
  close: []
}>()

const { canDismiss, handleVisibilityChange } = useOverlayInteractionLifecycle({
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

function handleOpenUpdate(open: boolean) {
  if (!open) handleVisibilityChange(false)
}

function handleMaskClick() {
  if (canDismiss('mask')) handleVisibilityChange(false)
}

function handleEscape(event: KeyboardEvent) {
  if (!canDismiss('escape')) event.preventDefault()
}

function blockOutsideDismiss(event: Event) {
  event.preventDefault()
}
</script>
