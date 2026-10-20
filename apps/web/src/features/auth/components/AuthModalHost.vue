<template>
  <DialogRoot :open="true" @update:open="handleOpenUpdate">
    <DialogPortal>
      <DialogOverlay @click="handleMaskClick" />
      <DialogContent
        :show-close="false"
        :class="cn(dialogContentBaseClass, authDialogContentClass)"
        :aria-label="props.ariaLabel"
        :aria-busy="props.busy || undefined"
        @escape-key-down="handleEscape"
        @open-auto-focus="handleOpenAutoFocus"
        @pointer-down-outside="blockOutsideDismiss"
      >
        <div ref="panelRef" class="min-h-0 overflow-y-auto">
          <DialogTitle class="sr-only">
            {{ props.ariaLabel ?? t('auth.signIn') }}
          </DialogTitle>
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
  DialogTitle,
} from '@/components/ui/dialog'
import { dialogContentBaseClass } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  defineDialogSurfaceIntent,
  useOverlayInteractionLifecycle,
} from '../../../core/composables/useOverlayInteractionLifecycle'
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

const { t } = useI18n()

const { canDismiss, handleVisibilityChange } = useOverlayInteractionLifecycle({
  isVisible: () => true,
  busy: () => props.busy,
  surface: defineDialogSurfaceIntent(),
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

/*
 * reka-ui autofocuses the first tabbable node in the dialog, which is the
 * trailing close action. Landing there puts a focus ring on a dismiss control
 * instead of the first field, so redirect autofocus to the first form control.
 */
const FIRST_FORM_CONTROL =
  'input:not([type="hidden"]):not([disabled]), select:not([disabled]), textarea:not([disabled])'

const panelRef = ref<HTMLElement | null>(null)

function handleOpenAutoFocus(event: Event) {
  const target = panelRef.value?.querySelector<HTMLElement>(FIRST_FORM_CONTROL)
  if (!target) return
  event.preventDefault()
  target.focus()
}

function blockOutsideDismiss(event: Event) {
  event.preventDefault()
}
</script>
