import { ref } from 'vue'
import { defaultAccountManagementTab, type AccountManagementTab } from './accountTabRegistry'
import type { PaymentReturnContext } from './paymentReturnRoute'

const accountManagementDialogVisible = ref(false)
const accountManagementInitialTab = ref<AccountManagementTab>(defaultAccountManagementTab)
const accountManagementPaymentReturn = ref<PaymentReturnContext | null>(null)

export function openAccountManagementDialog(
  tab: AccountManagementTab = defaultAccountManagementTab,
  paymentReturn: PaymentReturnContext | null = null,
) {
  accountManagementInitialTab.value = tab
  accountManagementPaymentReturn.value = paymentReturn
  accountManagementDialogVisible.value = true
}

export function closeAccountManagementDialog() {
  accountManagementDialogVisible.value = false
  accountManagementPaymentReturn.value = null
}

export function setAccountManagementDialogVisible(visible: boolean) {
  accountManagementDialogVisible.value = visible
  if (!visible) accountManagementPaymentReturn.value = null
}

export function useAccountManagementDialogController() {
  return {
    visible: accountManagementDialogVisible,
    initialTab: accountManagementInitialTab,
    paymentReturn: accountManagementPaymentReturn,
    open: openAccountManagementDialog,
    close: closeAccountManagementDialog,
    setVisible: setAccountManagementDialogVisible,
  }
}
