import { describe, expect, it, vi } from 'vitest'
import { createBoardFeedback } from './useBoardFeedback'

describe('createBoardFeedback', () => {
  it('maps board command outcomes to user-facing messages', () => {
    const appToast = {
      success: vi.fn(),
      warning: vi.fn(),
      error: vi.fn(),
    }
    const feedback = createBoardFeedback(appToast)

    feedback.createFailed({ ok: false, reason: 'name-conflict', board: boardMeta(), error: null })
    feedback.renameValidationFailed({ ok: false, reason: 'invalid-name' })
    feedback.renameSyncFailed({
      ok: false,
      reason: 'remote-name-conflict',
      boardId: 'b1',
      error: null,
    })
    feedback.renamed()
    feedback.copyFailed({ ok: false, reason: 'source-missing' })
    feedback.copied()
    feedback.deleteFailed({ ok: false, reason: 'sync-failed', error: null })
    feedback.deleted()
    feedback.catalogSyncFailed()

    expect(appToast.error).toHaveBeenNthCalledWith(1, '画布名称已存在，请重试')
    expect(appToast.warning).toHaveBeenCalledWith('画布名称不能为空')
    expect(appToast.error).toHaveBeenNthCalledWith(2, '名称已存在，请更换后再试')
    expect(appToast.success).toHaveBeenNthCalledWith(1, '画布已重命名')
    expect(appToast.error).toHaveBeenNthCalledWith(3, '复制失败')
    expect(appToast.success).toHaveBeenNthCalledWith(2, '已复制画布')
    expect(appToast.error).toHaveBeenNthCalledWith(4, '删除失败，请稍后重试')
    expect(appToast.success).toHaveBeenNthCalledWith(3, '已删除画布')
    expect(appToast.error).toHaveBeenNthCalledWith(5, '画布同步失败，请稍后重试')
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
