import { describe, expect, it, vi } from 'vitest'
import { createBoardFeedback } from './useBoardFeedback'

describe('createBoardFeedback', () => {
  it('keeps board failure reasons distinct', () => {
    const appToast = {
      success: vi.fn(),
      warning: vi.fn(),
      error: vi.fn(),
    }
    const feedback = createBoardFeedback(appToast)

    feedback.createFailed({
      isOk: false,
      reason: 'revision-conflict',
      board: boardMeta(),
      error: null,
    })
    feedback.createFailed({
      isOk: false,
      reason: 'idempotency-conflict',
      board: boardMeta(),
      error: null,
    })
    feedback.createFailed({ isOk: false, reason: 'sync-failed', board: boardMeta(), error: null })
    feedback.renameValidationFailed({ isOk: false, reason: 'local-name-conflict' })
    feedback.renameValidationFailed({ isOk: false, reason: 'invalid-name' })
    feedback.renameSyncFailed({
      isOk: false,
      reason: 'revision-conflict',
      boardId: 'b1',
      previousBoard: boardMeta(),
      rollback: { isOk: true, board: boardMeta() },
      hydrate: { isOk: true },
      error: null,
    })
    feedback.renameSyncFailed({
      isOk: false,
      reason: 'idempotency-conflict',
      boardId: 'b1',
      previousBoard: boardMeta(),
      rollback: { isOk: true, board: boardMeta() },
      hydrate: { isOk: false, error: new Error('hydrate failed') },
      error: null,
    })
    feedback.renameSyncFailed({
      isOk: false,
      reason: 'sync-failed',
      boardId: 'b1',
      previousBoard: boardMeta(),
      rollback: { isOk: true, board: boardMeta() },
      error: null,
    })
    feedback.copyFailed({ isOk: false, reason: 'source-missing' })
    feedback.copyFailed({
      isOk: false,
      reason: 'revision-conflict',
      board: boardMeta(),
      error: null,
    })
    feedback.copyFailed({
      isOk: false,
      reason: 'idempotency-conflict',
      board: boardMeta(),
      error: null,
    })
    feedback.copyFailed({ isOk: false, reason: 'sync-failed', board: boardMeta(), error: null })
    feedback.deleteFailed({ isOk: false, reason: 'sync-failed', error: null })
    feedback.deleteFailed({ isOk: false, reason: 'local-missing' })
    feedback.catalogSyncFailed()
    feedback.catalogReloaded()

    expect(appToast.warning.mock.calls.map(([message]) => message)).toEqual([
      'Name already exists',
      'Canvas name cannot be empty',
    ])
    expect(appToast.error.mock.calls.map(([message]) => message)).toEqual([
      'A sync conflict occurred. Please try again.',
      'A sync conflict occurred. Please try again.',
      'Could not create the canvas. Please try again later.',
      'Sync conflict. The canvas list has been reloaded.',
      'Sync conflict and recovery incomplete. Please reload the canvas list.',
      'Rename sync failed. The original name has been restored.',
      'Copy failed',
      'A sync conflict occurred while copying. Please try again.',
      'A sync conflict occurred while copying. Please try again.',
      'Copy sync failed. Please try again later.',
      'Delete failed. Please try again later.',
      'Delete failed',
      'Canvas sync failed. Please try again later.',
    ])
    expect(appToast.success).toHaveBeenCalledWith('Canvas list reloaded')
  })
})

function boardMeta() {
  return {
    id: 'board-1',
    name: 'Board',
    createdAt: 1,
    updatedAt: 1,
  }
}
