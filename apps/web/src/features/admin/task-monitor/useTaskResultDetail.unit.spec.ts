import { nextTick, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useTaskResultDetail } from './useTaskResultDetail'
import { getSafeResultImageUrl, getSafeTaskResultImageUrl } from './taskResultImages'
import type { Task } from '../../../shared/api/task'

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
    const task = ref<Task | null>({
      result: '{"image_url":"https://example.com/unsafe.png"}',
    } as unknown as Task)
    const detail = useTaskResultDetail(task, { feedback: createTaskResultFeedback() })

    expect(detail.resultImageUrl.value).toBe('')
  })

  it('does not open rejected URLs', () => {
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null)
    const feedback = createTaskResultFeedback()
    const detail = useTaskResultDetail(ref<Task | null>(null), { feedback })

    detail.openResultImage('javascript:alert(1)')

    expect(openSpy).not.toHaveBeenCalled()
    expect(feedback.unsafeImageUrlRejected).toHaveBeenCalled()

    openSpy.mockRestore()
  })

  it('identifies result image candidates that cannot be rendered safely', () => {
    const detail = useTaskResultDetail(
      ref<Task | null>({ result: '{"image_url":"javascript:alert(1)"}' } as Task),
      { feedback: createTaskResultFeedback() },
    )

    expect(detail.resultImageUrl.value).toBe('')
    expect(detail.hasResultImageCandidate.value).toBe(true)
  })

  it('resets debug expansion when the dialog switches tasks', async () => {
    const task = ref<Task | null>({ task_id: 'first' } as Task)
    const detail = useTaskResultDetail(task, { feedback: createTaskResultFeedback() })

    detail.showDebugResult.value = true
    task.value = { task_id: 'second' } as Task
    await nextTick()

    expect(detail.showDebugResult.value).toBe(false)
  })

  it('reports clipboard failures', async () => {
    const feedback = createTaskResultFeedback()
    const writeText = vi.fn().mockRejectedValue(new Error('denied'))
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    const detail = useTaskResultDetail(ref<Task | null>(null), { feedback })

    detail.copyToClipboard('task_1')
    await Promise.resolve()
    await Promise.resolve()

    expect(feedback.copyFailed).toHaveBeenCalledOnce()
    vi.unstubAllGlobals()
  })

  it('reports unavailable clipboard access as a copy failure', () => {
    const feedback = createTaskResultFeedback()
    vi.stubGlobal('navigator', { clipboard: undefined })
    const detail = useTaskResultDetail(ref<Task | null>(null), { feedback })

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
