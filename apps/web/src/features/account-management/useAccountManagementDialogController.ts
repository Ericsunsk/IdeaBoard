import { ref } from 'vue'

const accountManagementDialogVisible = ref(false)
const accountManagementInitialTab = ref<AccountManagementTab>('profile')

export type AccountManagementTab =
  | 'profile'
  | 'security'
  | 'recharge'
  | 'subscriptions'
  | 'rewards'
  | 'billing'
  | 'usage'

export function openAccountManagementDialog(tab: AccountManagementTab = 'profile') {
  accountManagementInitialTab.value = tab
  accountManagementDialogVisible.value = true
}

export function closeAccountManagementDialog() {
  accountManagementDialogVisible.value = false
}

export function setAccountManagementDialogVisible(visible: boolean) {
  accountManagementDialogVisible.value = visible
}

export function useAccountManagementDialogController() {
  return {
    visible: accountManagementDialogVisible,
    initialTab: accountManagementInitialTab,
    open: openAccountManagementDialog,
    close: closeAccountManagementDialog,
    setVisible: setAccountManagementDialogVisible,
  }
}
