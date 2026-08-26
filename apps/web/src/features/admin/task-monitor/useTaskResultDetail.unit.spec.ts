import { nextTick, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useTaskResultDetail } from './useTaskResultDetail'
import { getSafeResultImageUrl, getSafeTaskResultImageUrl } from './taskResultImages'
import type { AdminTaskMonitorDetailModel } from './taskMonitorResource'

describe('task result image URL validation', () => {
  it.each([
    '/uploads/canvas/user/board/content/image.webp',
    '/uploads/tasks/1/task/output.png?version=1',
    new URL('/uploads/tasks/1/task/output.png', window.location.origin).toString(),
  ])('allows %s', (url) => {
    expect(getSafeResultImageUrl(url)).not.toBe('')
  })

  it.each([
    'https://example.com/image.png',
    'http://localhost:6000/uploads/tasks/1/task/output.png',
    'javascript:alert(1)',
    'data:image/png;base64,AAAA',
    'blob:https://example.com/id',
    '//example.com/image.png',
    'ftp://example.com/image.png',
    'uploads/canvas/image.png',
    '/assets/admin-preview.png',
    '/api/user/download?url=https://example.com/image.png',
    '/uploads/../admin-secret.png',
    '/\\evil',
  ])('rejects %s', (url) => {
    expect(getSafeResultImageUrl(url)).toBe('')
  })

  it('extracts the first safe task result image URL', () => {
    const result = JSON.stringify({
      image_url: 'https://example.com/unsafe.png',
      image_urls: ['data:image/png;base64,AAAA', '/uploads/tasks/1/task/safe.png'],
    })

    expect(getSafeTaskResultImageUrl(result)).toBe('/uploads/tasks/1/task/safe.png')
  })

  it('does not expose unsafe detail image URLs for rendering', () => {
    const task = ref<AdminTaskMonitorDetailModel | null>(
      makeTaskDetail({ result: '{"image_url":"https://example.com/unsafe.png"}' }),
    )
    const detail = useTaskResultDetail(task, { feedback: createTaskResultFeedback() })

    expect(detail.resultImageUrl.value).toBe('')
  })

  it('identifies a result image retired by the user without treating the task as failed', () => {
    const detail = useTaskResultDetail(
      ref<AdminTaskMonitorDetailModel | null>(
        makeTaskDetail({ result: '{"status":"success","image_availability":"deleted_by_user"}' }),
      ),
      { feedback: createTaskResultFeedback() },
    )

    expect(detail.resultImageUrl.value).toBe('')
    expect(detail.isResultImageDeletedByUser.value).toBe(true)
  })

  it('does not open rejected URLs', () => {
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null)
    const feedback = createTaskResultFeedback()
    const detail = useTaskResultDetail(ref<AdminTaskMonitorDetailModel | null>(null), { feedback })

    detail.openResultImage('javascript:alert(1)')

    expect(openSpy).not.toHaveBeenCalled()
    expect(feedback.unsafeImageUrlRejected).toHaveBeenCalled()

    openSpy.mockRestore()
  })

  it('identifies result image candidates that cannot be rendered safely', () => {
    const detail = useTaskResultDetail(
      ref<AdminTaskMonitorDetailModel | null>(
        makeTaskDetail({ result: '{"image_url":"javascript:alert(1)"}' }),
      ),
      { feedback: createTaskResultFeedback() },
    )

    expect(detail.resultImageUrl.value).toBe('')
    expect(detail.hasResultImageCandidate.value).toBe(true)
  })

  it('resets debug expansion when the dialog switches tasks', async () => {
    const task = ref<AdminTaskMonitorDetailModel | null>(makeTaskDetail({ task_id: 'first' }))
    const detail = useTaskResultDetail(task, { feedback: createTaskResultFeedback() })

    detail.showDebugResult.value = true
    task.value = makeTaskDetail({ task_id: 'second' })
    await nextTick()

    expect(detail.showDebugResult.value).toBe(false)
  })

  it('reports clipboard failures', async () => {
    const feedback = createTaskResultFeedback()
    const writeText = vi.fn().mockRejectedValue(new Error('denied'))
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    const detail = useTaskResultDetail(ref<AdminTaskMonitorDetailModel | null>(null), { feedback })

    detail.copyToClipboard('task_1')
    await Promise.resolve()
    await Promise.resolve()

    expect(feedback.copyFailed).toHaveBeenCalledOnce()
    vi.unstubAllGlobals()
  })

  it('reports unavailable clipboard access as a copy failure', () => {
    const feedback = createTaskResultFeedback()
    vi.stubGlobal('navigator', { clipboard: undefined })
    const detail = useTaskResultDetail(ref<AdminTaskMonitorDetailModel | null>(null), { feedback })

    detail.copyToClipboard('task_1')

    expect(feedback.copyFailed).toHaveBeenCalledOnce()
    vi.unstubAllGlobals()
  })
})

function createTaskResultFeedback() {
  return {
    unsafeImageUrlRejected: vi.fn(),
    copiedToClipboard: vi.fn(),
    copyFailed: vi.fn(),
  }
}

function makeTaskDetail(
  overrides: Partial<AdminTaskMonitorDetailModel> = {},
): AdminTaskMonitorDetailModel {
  return {
    task_id: 'task-1',
    correlation_id: 'task-1',
    user_public_id: '00000000-0000-0000-0000-000000000001',
    user_display_name: 'User 1',
    prompt: '',
    user_prompt: '',
    params: '{}',
    feature_slug: 'render',
    priority: 2,
    status: 'completed',
    retry_count: 0,
    error_msg: '',
    result: '',
    created_at: '2026-07-27T08:00:00Z',
    updated_at: '2026-07-27T08:00:00Z',
    completed_at: null,
    cancelled_at: null,
    cancel_reason: '',
    execution_attempts: [],
    ...overrides,
  }
}
