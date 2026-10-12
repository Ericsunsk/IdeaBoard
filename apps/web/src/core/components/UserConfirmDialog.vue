<template>
  <DialogRoot :open="props.visible" @update:open="handleOpenUpdate">
    <DialogPortal>
      <DialogOverlay :class="ibDialogMaskClass" @click="handleMaskClick" />
      <DialogContent
        :class="dialogContentBaseClass"
        :style="{ width: 'min(420px, calc(100vw - 32px))' }"
        :aria-label="props.title"
        :aria-busy="props.loading || undefined"
        @escape-key-down="handleEscape"
        @pointer-down-outside="blockOutsideDismiss"
      >
        <div class="min-h-0 overflow-y-auto">
          <DialogDescription class="sr-only">
            {{ props.message }}
          </DialogDescription>
          <div class="p-6">
            <DialogTitle as="h3" class="m-0 text-lg font-semibold text-foreground">
              {{ props.title }}
            </DialogTitle>
            <p class="mt-5 mb-0 text-sm leading-6 text-muted-foreground">
              {{ props.message }}
            </p>
            <div class="mt-6 flex justify-end gap-2">
              <Button
                type="button"
                variant="ghost"
                :disabled="props.loading"
                class="rounded-full px-4 py-2 text-sm font-semibold text-muted-foreground hover:bg-muted hover:text-foreground"
                @click="emit('update:visible', false)"
              >
                {{ t('common.actions.cancel') }}
              </Button>
              <Button
                type="button"
                :disabled="props.loading"
                :class="[
                  'rounded-full px-4 py-2 text-sm font-semibold disabled:opacity-50',
                  props.intent === 'danger'
                    ? 'bg-destructive text-destructive-foreground hover:bg-destructive/90'
                    : 'bg-primary text-primary-foreground hover:bg-primary/90',
                ]"
                @click="emit('confirm')"
              >
                {{ props.confirmLabel || t('common.actions.confirm') }}
              </Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import {
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from '@/components/ui/dialog'
import { dialogContentBaseClass } from '@/components/ui/dialog'
import { ibDialogMaskClass } from '../../theme/dialogMask'
import {
  defineDialogSurfaceIntent,
  useOverlayInteractionLifecycle,
} from '../../core/composables/useOverlayInteractionLifecycle'

const props = withDefaults(
  defineProps<{
    visible: boolean
    title: string
    message: string
    confirmLabel?: string
    intent?: 'primary' | 'danger'
    loading?: boolean
  }>(),
  {
    confirmLabel: '',
    intent: 'primary',
    loading: false,
  },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  confirm: []
}>()

const { t } = useI18n()

// Unified dialog contract: mask/Escape/close-control dismiss while idle; the
// irreversible confirm commit (loading) locks every path. Promise settlement
// on parent unmount stays in useUserConfirmDialog.dispose.
const { canDismiss, requestClose } = useOverlayInteractionLifecycle({
  visible: () => props.visible,
  busy: () => props.loading,
  surface: defineDialogSurfaceIntent(),
  onVisibilityChange: (visible) => {
    if (!visible) emit('update:visible', false)
  },
})

function handleOpenUpdate(open: boolean) {
  if (!open) requestClose('close-control')
}

function handleMaskClick() {
  requestClose('mask')
}

function handleEscape(event: KeyboardEvent) {
  if (!canDismiss('escape')) event.preventDefault()
}

function blockOutsideDismiss(event: Event) {
  event.preventDefault()
}
</script>
