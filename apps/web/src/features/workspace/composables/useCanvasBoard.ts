import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import type { LocationQueryValue } from 'vue-router'
import {
  getCanvasBoardById,
  listCanvasBoards,
  renameCanvasBoard,
  touchCanvasBoard,
} from '../../boards/lib/canvasBoards'
import type { CanvasBoardMeta } from '../../boards/lib/canvasBoards'
import { getCanvasBoardState, setCanvasBoardState } from '../../boards/lib/canvasBoardState'
import type { CanvasBoardSnapshot } from '../types/canvasNodes'

type LeaveChoice = 'save' | 'discard' | 'cancel'

interface CanvasBoardMessageApi {
  success(content: string): unknown
  warning(content: string): unknown
  error(content: string): unknown
}

interface UseCanvasBoardOptions {
  defaultBoardName?: string
  message: CanvasBoardMessageApi
  createSnapshot: () => CanvasBoardSnapshot
  applySnapshot: (snapshot: CanvasBoardSnapshot | null) => void | Promise<void>
}

function resolveBoardIdFromQueryValue(
  raw: LocationQueryValue | LocationQueryValue[] | null | undefined,
): string | null {
  const value = Array.isArray(raw) ? raw[0] : raw
  if (typeof value !== 'string') return null
  const boardId = value.trim()
  return boardId || null
}

export function useCanvasBoard(options: UseCanvasBoardOptions) {
  const route = useRoute()
  const router = useRouter()
  const defaultBoardName = options.defaultBoardName ?? 'Untitled'

  const currentBoardName = ref(defaultBoardName)
  const boardOptions = ref<CanvasBoardMeta[]>([])
  const isBoardMenuOpen = ref(false)
  const isRenamingBoard = ref(false)
  const boardNameDraft = ref('')
  const boardNameInputRef = ref<HTMLInputElement | null>(null)
  const isBoardDirty = ref(false)
  const leaveConfirmVisible = ref(false)
  const isApplyingBoardState = ref(false)
  const currentBoardId = computed(() => resolveBoardIdFromQueryValue(route.query.boardId))

  let leaveConfirmResolver: ((choice: LeaveChoice) => void) | null = null
  let leaveConfirmPromise: Promise<LeaveChoice> | null = null

  function refreshBoardOptions() {
    boardOptions.value = listCanvasBoards()
  }

  function syncCurrentBoardName() {
    const boardId = currentBoardId.value
    if (!boardId) {
      currentBoardName.value = defaultBoardName
      refreshBoardOptions()
      return
    }
    const board = getCanvasBoardById(boardId)
    currentBoardName.value = board?.name || defaultBoardName
    refreshBoardOptions()
  }

  function toggleBoardMenu() {
    if (!isBoardMenuOpen.value) refreshBoardOptions()
    isBoardMenuOpen.value = !isBoardMenuOpen.value
  }

  function startBoardRename() {
    if (!currentBoardId.value) return
    boardNameDraft.value = currentBoardName.value
    isRenamingBoard.value = true
    isBoardMenuOpen.value = false
    void nextTick(() => {
      boardNameInputRef.value?.focus()
      boardNameInputRef.value?.select()
    })
  }

  function cancelBoardRename() {
    isRenamingBoard.value = false
    boardNameDraft.value = ''
  }

  function confirmBoardRename() {
    if (!isRenamingBoard.value) return
    const boardId = currentBoardId.value
    const nextName = boardNameDraft.value.trim()
    isRenamingBoard.value = false
    if (!boardId || !nextName || nextName === currentBoardName.value) {
      boardNameDraft.value = ''
      return
    }
    const renamed = renameCanvasBoard(boardId, nextName)
    if (!renamed) {
      options.message.warning('重命名失败')
      boardNameDraft.value = ''
      return
    }
    currentBoardName.value = renamed.name
    boardNameDraft.value = ''
    refreshBoardOptions()
    options.message.success('已重命名画布')
  }

  async function handleBoardSelect(boardId: string) {
    if (!boardId || boardId === currentBoardId.value) {
      isBoardMenuOpen.value = false
      return
    }
    isBoardMenuOpen.value = false
    void router.push({
      path: '/user/picture',
      query: {
        ...route.query,
        boardId,
      },
    })
  }

  async function loadBoardStateFor(boardId: string | null) {
    isApplyingBoardState.value = true
    try {
      await options.applySnapshot(boardId ? getCanvasBoardState(boardId) : null)
      isBoardDirty.value = false
    } finally {
      isApplyingBoardState.value = false
    }
  }

  function persistBoardStateNow(): boolean {
    const boardId = currentBoardId.value
    if (!boardId || isApplyingBoardState.value) return false
    const saved = setCanvasBoardState(boardId, options.createSnapshot())
    if (!saved) return false
    touchCanvasBoard(boardId)
    refreshBoardOptions()
    isBoardDirty.value = false
    return true
  }

  function markBoardDirty() {
    if (isApplyingBoardState.value || !currentBoardId.value) return
    isBoardDirty.value = true
  }

  function requestLeaveConfirm(): Promise<LeaveChoice> {
    if (leaveConfirmPromise) return leaveConfirmPromise
    leaveConfirmVisible.value = true
    leaveConfirmPromise = new Promise<LeaveChoice>((resolve) => {
      leaveConfirmResolver = resolve
    })
    return leaveConfirmPromise
  }

  function resolveLeaveConfirm(choice: LeaveChoice) {
    if (!leaveConfirmPromise) return
    leaveConfirmVisible.value = false
    const resolver = leaveConfirmResolver
    leaveConfirmResolver = null
    leaveConfirmPromise = null
    resolver?.(choice)
  }

  async function confirmSaveBeforeLeave(): Promise<boolean> {
    if (!isBoardDirty.value) return true
    const choice = await requestLeaveConfirm()
    if (choice === 'save') {
      const ok = persistBoardStateNow()
      if (!ok) {
        options.message.error('保存失败，请重试')
        return false
      }
      options.message.success('已保存画布')
      return true
    }
    if (choice === 'discard') return true
    return false
  }

  watch(
    () => route.query.boardId,
    async () => {
      isBoardMenuOpen.value = false
      isRenamingBoard.value = false
      syncCurrentBoardName()
      await loadBoardStateFor(currentBoardId.value)
    },
    { immediate: true },
  )

  onBeforeRouteLeave(async () => confirmSaveBeforeLeave())

  onBeforeRouteUpdate(async (to, from) => {
    const toBoardId = resolveBoardIdFromQueryValue(to.query.boardId)
    const fromBoardId = resolveBoardIdFromQueryValue(from.query.boardId)
    if (to.path === '/user/picture' && from.path === '/user/picture' && toBoardId !== fromBoardId) {
      return confirmSaveBeforeLeave()
    }
    return true
  })

  onUnmounted(() => {
    if (leaveConfirmPromise) {
      resolveLeaveConfirm('cancel')
    }
  })

  return {
    currentBoardId,
    currentBoardName,
    boardOptions,
    isBoardMenuOpen,
    isRenamingBoard,
    boardNameDraft,
    boardNameInputRef,
    isBoardDirty,
    leaveConfirmVisible,
    toggleBoardMenu,
    startBoardRename,
    cancelBoardRename,
    confirmBoardRename,
    handleBoardSelect,
    resolveLeaveConfirm,
    confirmSaveBeforeLeave,
    persistBoardStateNow,
    markBoardDirty,
    refreshBoardOptions,
    isApplyingBoardState,
  }
}
