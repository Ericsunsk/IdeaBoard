<template>
  <Dialog
    ref="dialogRef"
    :visible="visible"
    modal
    :dismissable-mask="canDismiss('mask')"
    :close-on-escape="canDismiss('escape')"
    :show-header="false"
    :draggable="false"
    :style="accountDialogStyle"
    :pt="accountDialogPt"
    :aria-busy="ariaBusy"
    :aria-label="t('account.dialog.title')"
    @update:visible="handleVisibilityChange"
    @hide="disableVendorFocusRestore"
    @after-hide="completeClose"
  >
    <AccountDialogContent
      v-if="visible"
      :initial-tab="initialTab"
      :payment-return="paymentReturn"
      @close="requestOverlayClose('close-control')"
      @update:busy="busy = $event"
    />
  </Dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Dialog from 'primevue/dialog'
import { usePrimeDialogOverlayLifecycle } from '../../../../shared/composables/usePrimeDialogOverlayLifecycle'
import { defaultAccountTab, type AccountTab, type PaymentReturnContext } from '../../domain'
import AccountDialogContent from './AccountDialogContent.vue'
import {
  accountDialogStyle,
  createAccountDialogPt,
} from '../common/ui'

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

const accountDialogPt = createAccountDialogPt()
</script>
