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

    feedback.createFailed({ ok: false, reason: 'name-conflict', board: boardMeta(), error: null })
    feedback.createFailed({ ok: false, reason: 'sync-failed', board: boardMeta(), error: null })
    feedback.renameValidationFailed({ ok: false, reason: 'local-name-conflict' })
    feedback.renameValidationFailed({ ok: false, reason: 'invalid-name' })
    feedback.renameSyncFailed({
      ok: false,
      reason: 'remote-name-conflict',
      boardId: 'b1',
      previousBoard: boardMeta(),
      rollback: { ok: true, board: boardMeta() },
      hydrate: { ok: true },
      error: null,
    })
    feedback.renameSyncFailed({
      ok: false,
      reason: 'sync-failed',
      boardId: 'b1',
      previousBoard: boardMeta(),
      rollback: { ok: true, board: boardMeta() },
      error: null,
    })
    feedback.copyFailed({ ok: false, reason: 'source-missing' })
    feedback.copyFailed({ ok: false, reason: 'name-conflict', board: boardMeta(), error: null })
    feedback.copyFailed({ ok: false, reason: 'sync-failed', board: boardMeta(), error: null })
    feedback.deleteFailed({ ok: false, reason: 'sync-failed', error: null })
    feedback.deleteFailed({ ok: false, reason: 'local-missing' })
    feedback.catalogSyncFailed()
    feedback.catalogReloaded()

    expect(appToast.warning.mock.calls.map(([message]) => message)).toEqual([
      'Name already exists',
      'Canvas name cannot be empty',
    ])
    expect(appToast.error.mock.calls.map(([message]) => message)).toEqual([
      'A canvas with this name already exists. Please try again.',
      'Could not create the canvas. Please try again later.',
      'Name conflict. The canvas list has been reloaded.',
      'Rename sync failed. The original name has been restored.',
      'Copy failed',
      'A canvas with the copied name already exists. Please try again.',
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
