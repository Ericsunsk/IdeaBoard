import { describe, expect, it, vi } from 'vitest'
import { createCanvasFeedback } from './useCanvasFeedback'

describe('createCanvasFeedback', () => {
  it('groups canvas runtime message ports behind one feedback boundary', () => {
    const loadingHandle = { id: 'loading', destroy: vi.fn() }
    const appToast = {
      success: vi.fn(),
      warning: vi.fn(),
      error: vi.fn(),
      loading: vi.fn(() => loadingHandle),
    }
    const feedback = createCanvasFeedback(appToast)

    feedback.boardSession.saveSucceeded('画布已保存')
    feedback.boardSession.saveFailed('保存失败')
    feedback.graph.operationWarning('连接被拒绝')
    const handle = feedback.graph.operationLoading('导出中', { duration: 1000 })
    feedback.taskExecution.succeeded('任务完成')
    feedback.imageTools.warning('当前图片不可编辑')
    feedback.imageTools.actionUnavailable('工具不可执行')

    expect(appToast.success).toHaveBeenNthCalledWith(1, '画布已保存')
    expect(appToast.error).toHaveBeenCalledWith('保存失败')
    expect(appToast.warning).toHaveBeenNthCalledWith(1, '连接被拒绝')
    expect(appToast.loading).toHaveBeenCalledWith('导出中', { duration: 1000 })
    expect(handle).toBe(loadingHandle)
    expect(appToast.success).toHaveBeenNthCalledWith(2, '任务完成')
    expect(appToast.warning).toHaveBeenNthCalledWith(2, '当前图片不可编辑')
    expect(appToast.warning).toHaveBeenNthCalledWith(3, '工具不可执行')
  })
})
