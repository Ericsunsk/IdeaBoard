import { getCurrentScope, onScopeDispose, readonly, ref } from 'vue'
import { getBoardScope, subscribeBoardScopeChange } from '../catalog/scope'

export type SharePanelTarget = {
  boardId: string
  boardName: string
}

export function useSharePanel() {
  const target = ref<SharePanelTarget | null>(null)
  const isOpen = ref(false)
  const generation = ref(0)

  const stopScope = subscribeBoardScopeChange(() => {
    target.value = null
    isOpen.value = false
    generation.value += 1
  })
  if (getCurrentScope()) onScopeDispose(stopScope)

  function open(boardId: string, boardName: string): boolean {
    const scope = getBoardScope()
    const normalizedBoardId = boardId.trim()
    if (!scope.userScope || !scope.workspaceId || !normalizedBoardId) return false
    target.value = { boardId: normalizedBoardId, boardName }
    isOpen.value = true
    generation.value += 1
    return true
  }

  function close() {
    isOpen.value = false
  }

  function handleHidden() {
    if (!isOpen.value) target.value = null
  }

  function dispose() {
    stopScope()
  }

  return {
    target: readonly(target),
    isOpen: readonly(isOpen),
    generation: readonly(generation),
    open,
    close,
    handleHidden,
    dispose,
  }
}

export type SharePanel = ReturnType<typeof useSharePanel>
