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
      :payment-return="paymentReturn"
      @close="requestOverlayClose('close-control')"
      @update:busy="busy = $event"
    />
  </Dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import Dialog from 'primevue/dialog'
import { usePrimeDialogOverlayLifecycle } from '../../../shared/composables/usePrimeDialogOverlayLifecycle'
import { defaultAccountManagementTab, type AccountManagementTab } from '../accountTabRegistry'
import type { PaymentReturnContext } from '../paymentReturnRoute'
import AccountManagementDialogContent from './AccountManagementDialogContent.vue'
import {
  accountManagementDialogStyle,
  createAccountManagementDialogPt,
} from './accountManagementUi'

const props = withDefaults(
  defineProps<{
    visible: boolean
    initialTab?: AccountManagementTab
    paymentReturn?: PaymentReturnContext | null
  }>(),
  { initialTab: defaultAccountManagementTab, paymentReturn: null },
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

const accountManagementDialogPt = createAccountManagementDialogPt()
</script>
