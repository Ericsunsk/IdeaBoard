<template>
  <DialogRoot :open="props.visible" @update:open="handleOpenUpdate">
    <DialogPortal>
      <DialogOverlay :class="ibDialogMaskClass" @click="handleMaskClick" />
      <DialogContent
        :show-close="false"
        :class="cn(dialogContentBaseClass, accountDialogContentClass)"
        :aria-label="t('account.dialog.title')"
        :aria-busy="ariaBusy"
        @escape-key-down="handleEscape"
        @pointer-down-outside="blockOutsideDismiss"
      >
        <div class="min-h-0 overflow-y-auto">
          <DialogTitle class="sr-only">
            {{ t('account.dialog.title') }}
          </DialogTitle>
          <DialogDescription class="sr-only">
            {{ t('account.dialog.description') }}
          </DialogDescription>
          <AccountDialogContent
            v-if="visible"
            :initial-tab="initialTab"
            :payment-return="paymentReturn"
            @close="requestOverlayClose('close-control')"
            @update:busy="busy = $event"
          />
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from '@/components/ui/dialog'
import { dialogContentBaseClass } from '@/components/ui/dialog/dialogClasses'
import { cn } from '@/lib/utils'
import { ibDialogMaskClass } from '../../../../shared/theme/dialogMask'
import { defineDialogSurfaceIntent, useOverlayInteractionLifecycle } from '../../../../shared/composables/useOverlayInteractionLifecycle'
import { defaultAccountTab, type AccountTab, type PaymentReturnContext } from '../../domain'
import AccountDialogContent from './AccountDialogContent.vue'
import { accountDialogContentClass } from '../common/ui'

const props = withDefaults(
  defineProps<{
    visible: boolean
    initialTab?: AccountTab
    paymentReturn?: PaymentReturnContext | null
  }>(),
  { initialTab: defaultAccountTab, paymentReturn: null },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
}>()
const { t } = useI18n()
const busy = ref(false) // Overlay dismiss guard (not `useAsyncAction`): wired into `useOverlayInteractionLifecycle`.
const {
  ariaBusy,
  canDismiss,
  completeClose,
  handleVisibilityChange,
  requestClose: requestOverlayClose,
} = useOverlayInteractionLifecycle({
  visible: () => props.visible,
  busy,
  surface: defineDialogSurfaceIntent(),
  onReset: () => {
    busy.value = false
  },
  onVisibilityChange: (visible) => emit('update:visible', visible),
})

function handleOpenUpdate(open: boolean) {
  if (!open && handleVisibilityChange(false)) completeClose()
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
