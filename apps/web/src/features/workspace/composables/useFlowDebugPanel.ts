import { computed, ref } from 'vue'

type UseFlowDebugPanelOptions = {
  canSaveBoard: { value: boolean }
  saveStatus: { value: 'idle' | 'saved' | 'failed' }
  isLoadingBoardSnapshot: { value: boolean }
  isDirty: { value: boolean }
  refreshSnapshot: () => void
}

export function useFlowDebugPanel(options: UseFlowDebugPanelOptions) {
  const debugPanelOpen = ref(false)
  const snapshotText = ref('')
  const snapshotView = ref<'flow' | 'board'>('flow')

  function setSnapshotView(view: 'flow' | 'board') {
    snapshotView.value = view
    options.refreshSnapshot()
  }

  function copyJson() {
    void navigator.clipboard.writeText(snapshotText.value || '')
  }

  const saveStateTag = computed<'dirty' | 'saved' | 'failed'>(() => {
    if (!options.canSaveBoard.value) return 'failed'
    if (options.saveStatus.value === 'saved') return 'saved'
    if (options.saveStatus.value === 'failed') return 'failed'
    return options.isDirty.value ? 'dirty' : 'saved'
  })

  const saveStateText = computed(() => {
    if (!options.canSaveBoard.value) {
      return options.isLoadingBoardSnapshot.value ? '未启用保存（加载中）' : '未启用保存'
    }
    if (options.saveStatus.value === 'saved' && !options.isDirty.value) return '已保存'
    if (options.saveStatus.value === 'failed') return '保存失败'
    return '未保存修改'
  })

  return {
    debugPanelOpen,
    snapshotText,
    snapshotView,
    setSnapshotView,
    copyJson,
    saveStateTag,
    saveStateText,
  }
}
