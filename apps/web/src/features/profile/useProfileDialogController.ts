import { ref } from 'vue'

const profileDialogVisible = ref(false)

export function openProfileDialog() {
  profileDialogVisible.value = true
}

export function closeProfileDialog() {
  profileDialogVisible.value = false
}

export function setProfileDialogVisible(visible: boolean) {
  profileDialogVisible.value = visible
}

export function useProfileDialogController() {
  return {
    visible: profileDialogVisible,
    open: openProfileDialog,
    close: closeProfileDialog,
    setVisible: setProfileDialogVisible,
  }
}
