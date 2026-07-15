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
      '名称已存在',
      '画布名称不能为空',
    ])
    expect(appToast.error.mock.calls.map(([message]) => message)).toEqual([
      '画布名称已存在，请重试',
      '创建同步失败，请稍后重试',
      '名称冲突，已重新加载画布列表',
      '重命名同步失败，已恢复原名称',
      '复制失败',
      '复制后的画布名称已存在，请重试',
      '复制同步失败，请稍后重试',
      '删除失败，请稍后重试',
      '删除失败',
      '画布同步失败，请稍后重试',
    ])
    expect(appToast.success).toHaveBeenCalledWith('画布列表已重新加载')
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
