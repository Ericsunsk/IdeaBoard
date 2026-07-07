import { ref } from 'vue'

const accountManagementDialogVisible = ref(false)

export function openAccountManagementDialog() {
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
    open: openAccountManagementDialog,
    close: closeAccountManagementDialog,
    setVisible: setAccountManagementDialogVisible,
  }
}
