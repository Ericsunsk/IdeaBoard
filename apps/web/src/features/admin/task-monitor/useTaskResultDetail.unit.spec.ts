import { describe, expect, it, vi } from 'vitest'
import { useTaskResultDetail } from './useTaskResultDetail'
import { getSafeTaskResultImageUrl, isAllowedResultImageUrl } from './taskResultImages'
import type { Task } from '../../../shared/api/task'

describe('task result image URL validation', () => {
  it.each([
    '/uploads/canvas/user/board/content/image.webp',
    '/uploads/tasks/1/task/output.png?version=1',
    new URL('/uploads/tasks/1/task/output.png', window.location.origin).toString(),
  ])('allows %s', (url) => {
    expect(isAllowedResultImageUrl(url)).toBe(true)
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
    expect(isAllowedResultImageUrl(url)).toBe(false)
  })

  it('extracts the first safe task result image URL', () => {
    const result = JSON.stringify({
      image_url: 'https://example.com/unsafe.png',
      image_urls: ['data:image/png;base64,AAAA', '/uploads/tasks/1/task/safe.png'],
    })

    expect(getSafeTaskResultImageUrl(result)).toBe('/uploads/tasks/1/task/safe.png')
  })

  it('does not expose unsafe detail image URLs for rendering', () => {
    const detail = useTaskResultDetail({ feedback: createTaskResultFeedback() })

    detail.showDetail({
      result: '{"image_url":"https://example.com/unsafe.png"}',
    } as unknown as Task)

    expect(detail.resultImageUrl.value).toBe('')
  })

  it('does not open rejected URLs', () => {
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null)
    const feedback = createTaskResultFeedback()
    const detail = useTaskResultDetail({ feedback })

    detail.openResultImage('javascript:alert(1)')

    expect(openSpy).not.toHaveBeenCalled()
    expect(feedback.unsafeImageUrlRejected).toHaveBeenCalled()

    openSpy.mockRestore()
  })
})

function createTaskResultFeedback() {
  return {
    unsafeImageUrlRejected: vi.fn(),
    copiedToClipboard: vi.fn(),
    copyFailed: vi.fn(),
  }
}
