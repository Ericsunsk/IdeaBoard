import { computed, type ComputedRef } from 'vue'
import type { BoardSnapshot } from '../../../board/domain'
import { isBoardNameConflictError } from '../../../board/application/sync/boardSync'
import { i18n } from '../../../../shared/i18n'
import {
  createBoardSessionRepository,
  isBoardSessionRemoteSyncError,
  type BoardSessionRepository,
} from './repository'
import {
  defaultBoardSessionPreviewRenderer,
  refreshBoardSessionPreview,
  type BoardSessionPreviewRenderer,
} from './preview'
import { createBoardSessionUploadLifecycle } from './uploadLifecycle'

type BoardSessionIdentity = {
  userId: number
  userUid: string
}

function createAuthUnavailableError(): Error {
  return new Error(i18n.global.t('canvas.session.saveAuthUnavailable'))
}

function normalizeBoardSessionIdentity(
  identity: BoardSessionIdentity,
): BoardSessionIdentity | null {
  const userId = identity.userId
  const userUid = typeof identity.userUid === 'string' ? identity.userUid.trim() : ''
  if (!Number.isFinite(userId) || userId <= 0 || !userUid) return null
  return { userId, userUid }
}

export type BoardSessionMessage = {
  warning: (message: string) => void
}

export type BoardSessionSaveOptions = {
  waitForPreview?: boolean | undefined
}

export type BoardSessionRemoteSyncFailureReason = 'sync-failed' | 'conflict'

export type BoardSessionRemoteSyncResult =
  { ok: true } | { ok: false; reason: BoardSessionRemoteSyncFailureReason; error: unknown }

export type BoardSessionSaveResult = {
  snapshot: BoardSnapshot
  remoteSync: BoardSessionRemoteSyncResult
}

export type BoardSession = {
  currentBoardId: ComputedRef<string | null>
  exists: (boardId?: string | null) => boolean
  load: (boardId?: string | null) => Promise<BoardSnapshot | null>
  save: (
    snapshot: BoardSnapshot,
    options?: BoardSessionSaveOptions,
  ) => Promise<BoardSessionSaveResult>
  refreshPreview: (boardId: string, snapshot: BoardSnapshot) => Promise<void>
}

export type UseBoardSessionOptions = {
  getBoardIdQuery: () => unknown
  getUserIdentity: () => BoardSessionIdentity
  repository?: BoardSessionRepository
  previewRenderer?: BoardSessionPreviewRenderer
  message: BoardSessionMessage
}

type BoardSessionUploadLifecycleFactory = typeof createBoardSessionUploadLifecycle

export type BoardSessionTestOptions = UseBoardSessionOptions & {
  createUploadLifecycle?: BoardSessionUploadLifecycleFactory
}

function classifyRemoteSyncFailure(error: unknown): BoardSessionRemoteSyncFailureReason {
  return isBoardNameConflictError(error) ? 'conflict' : 'sync-failed'
}

export function useBoardSession(options: BoardSessionTestOptions): BoardSession {
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

  async function load(boardId?: string | null): Promise<BoardSnapshot | null> {
    const normalized = resolveTargetBoardId(boardId)
    if (!normalized) return null
    return repository.load(normalized)
  }

  async function save(
    snapshot: BoardSnapshot,
    saveOptions: BoardSessionSaveOptions = {},
  ): Promise<BoardSessionSaveResult> {
    const boardId = currentBoardId.value
    if (!boardId) {
      throw new Error(i18n.global.t('canvas.session.missingBoardIdSave'))
    }
    if (!exists(boardId)) {
      throw new Error('BOARD_NOT_FOUND')
    }
    const identity = normalizeBoardSessionIdentity(options.getUserIdentity())
    if (!identity) {
      await repository.saveLocal(boardId, snapshot)
      return {
        snapshot,
        remoteSync: {
          ok: false,
          reason: 'sync-failed',
          error: createAuthUnavailableError(),
        },
      }
    }

    const uploadLifecycle = createUploadLifecycle({
      userUid: identity.userUid,
      boardId,
      warning: options.message.warning,
    })
    let stabilizedSnapshot: BoardSnapshot
    try {
      stabilizedSnapshot = await uploadLifecycle.stabilizeSnapshotForSave(snapshot)
    } catch (error) {
      await repository.saveLocal(boardId, snapshot)
      throw error
    }
    try {
      await repository.save(boardId, stabilizedSnapshot)
    } catch (error) {
      if (!isBoardSessionRemoteSyncError(error)) {
        throw error
      }
      const cause = error.cause ?? error
      return {
        snapshot: stabilizedSnapshot,
        remoteSync: {
          ok: false,
          reason: classifyRemoteSyncFailure(cause),
          error: cause,
        },
      }
    }
    const previewRefreshTask = refreshPreview(boardId, stabilizedSnapshot).catch((error) => {
      console.warn('[canvas] refresh board preview after save failed', boardId, error)
    })
    if (saveOptions.waitForPreview) {
      await previewRefreshTask
    }
    return { snapshot: stabilizedSnapshot, remoteSync: { ok: true } }
  }

  async function refreshPreview(boardId: string, snapshot: BoardSnapshot): Promise<void> {
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

function resolveBoardIdFromQueryValue(raw: unknown): string | null {
  const value = Array.isArray(raw) ? raw[0] : raw
  if (typeof value !== 'string') return null
  const boardId = value.trim()
  return boardId || null
}
