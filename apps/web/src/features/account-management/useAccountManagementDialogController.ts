import { ref } from 'vue'
import { defaultAccountManagementTab, type AccountManagementTab } from './accountTabRegistry'

const accountManagementDialogVisible = ref(false)
const accountManagementInitialTab = ref<AccountManagementTab>(defaultAccountManagementTab)

export function openAccountManagementDialog(
  tab: AccountManagementTab = defaultAccountManagementTab,
) {
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
