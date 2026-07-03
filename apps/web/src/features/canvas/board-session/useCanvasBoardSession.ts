import { computed, type ComputedRef } from 'vue'
import type { CanvasBoardSnapshot } from '../../canvas-board/domain'
import {
  createBoardSessionRepository,
  isBoardSessionRemoteSyncError,
  type BoardSessionRepository,
} from './boardSessionRepository'
import {
  defaultBoardSessionPreviewRenderer,
  refreshBoardSessionPreview,
  type BoardSessionPreviewRenderer,
} from './boardSessionPreview'
import { createBoardSessionUploadLifecycle } from './boardSessionUploadLifecycle'

type BoardSessionIdentity = {
  userId: number
  userUid: string
}

export type CanvasBoardSessionMessage = {
  warning: (message: string) => void
}

export type CanvasBoardSessionSaveOptions = {
  waitForPreview?: boolean
}

export type CanvasBoardSessionSaveResult = {
  snapshot: CanvasBoardSnapshot
  remoteSync: { ok: true } | { ok: false; error: unknown }
}

export type CanvasBoardSession = {
  currentBoardId: ComputedRef<string | null>
  exists: (boardId?: string | null) => boolean
  load: (boardId?: string | null) => Promise<CanvasBoardSnapshot | null>
  save: (
    snapshot: CanvasBoardSnapshot,
    options?: CanvasBoardSessionSaveOptions,
  ) => Promise<CanvasBoardSessionSaveResult>
  refreshPreview: (boardId: string, snapshot: CanvasBoardSnapshot) => Promise<void>
}

export type UseCanvasBoardSessionOptions = {
  getBoardIdQuery: () => unknown
  getUserIdentity: () => BoardSessionIdentity
  repository?: BoardSessionRepository
  previewRenderer?: BoardSessionPreviewRenderer
  message: CanvasBoardSessionMessage
}

type BoardSessionUploadLifecycleFactory = typeof createBoardSessionUploadLifecycle

export type CanvasBoardSessionTestOptions = UseCanvasBoardSessionOptions & {
  createUploadLifecycle?: BoardSessionUploadLifecycleFactory
}

export function useCanvasBoardSession(
  options: CanvasBoardSessionTestOptions,
): CanvasBoardSession {
  const repository = options.repository ?? createBoardSessionRepository()
  const previewRenderer = options.previewRenderer ?? defaultBoardSessionPreviewRenderer
  const createUploadLifecycle = options.createUploadLifecycle ?? createBoardSessionUploadLifecycle

  const currentBoardId = computed(() => resolveBoardIdFromQueryValue(options.getBoardIdQuery()))

  function resolveTargetBoardId(boardId?: string | null): string | null {
    if (typeof boardId === 'string') {
      const normalized = boardId.trim()
      return normalized || null
    }
    return currentBoardId.value
  }

  function exists(boardId?: string | null): boolean {
    const normalized = resolveTargetBoardId(boardId)
    if (!normalized) return false
    return repository.exists(normalized)
  }

  async function load(boardId?: string | null): Promise<CanvasBoardSnapshot | null> {
    const normalized = resolveTargetBoardId(boardId)
    if (!normalized) return null
    return repository.load(normalized)
  }

  async function save(
    snapshot: CanvasBoardSnapshot,
    saveOptions: CanvasBoardSessionSaveOptions = {},
  ): Promise<CanvasBoardSessionSaveResult> {
    const boardId = currentBoardId.value
    if (!boardId) {
      throw new Error('缺少 boardId，无法保存')
    }
    if (!exists(boardId)) {
      throw new Error('BOARD_NOT_FOUND')
    }
    const { userId, userUid } = options.getUserIdentity()
    if (!Number.isFinite(userId) || userId <= 0 || !userUid) {
      throw new Error('保存失败：缺少用户信息，请刷新后重试')
    }

    const uploadLifecycle = createUploadLifecycle({
      userId,
      userUid,
      boardId,
      warning: options.message.warning,
    })
    const previousSnapshot = uploadLifecycle.readPreviousSnapshot()
    const stabilizedSnapshot = await uploadLifecycle.stabilizeSnapshotForSave(snapshot)
    try {
      await repository.save(boardId, stabilizedSnapshot)
    } catch (error) {
      if (!isBoardSessionRemoteSyncError(error)) {
        throw error
      }
      return {
        snapshot: stabilizedSnapshot,
        remoteSync: { ok: false, error: error.cause ?? error },
      }
    }
    const previewRefreshTask = refreshPreview(boardId, stabilizedSnapshot).catch((error) => {
      console.warn('[canvas] refresh board preview after save failed', boardId, error)
    })
    if (saveOptions.waitForPreview) {
      await previewRefreshTask
    }
    await uploadLifecycle.recycleRemovedUploadsAfterSave(previousSnapshot, stabilizedSnapshot)
    return { snapshot: stabilizedSnapshot, remoteSync: { ok: true } }
  }

  async function refreshPreview(boardId: string, snapshot: CanvasBoardSnapshot): Promise<void> {
    const normalized = boardId.trim()
    if (!normalized) return
    await refreshBoardSessionPreview({
      boardId: normalized,
      snapshot,
      repository,
      renderer: previewRenderer,
    })
  }

  return {
    currentBoardId,
    exists,
    load,
    save,
    refreshPreview,
  }
}

export function resolveBoardIdFromQueryValue(raw: unknown): string | null {
  const value = Array.isArray(raw) ? raw[0] : raw
  if (typeof value !== 'string') return null
  const boardId = value.trim()
  return boardId || null
}
