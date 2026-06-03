import { describe, expect, it, vi } from 'vitest'
import { isAllowedResultImageUrl, useTaskResultDetail } from './useTaskResultDetail'

describe('task result image URL validation', () => {
  it.each([
    'https://example.com/image.png',
    'http://localhost:6000/uploads/tasks/1/task/output.png',
    '/uploads/workspace/user/board/content/image.webp',
    '/assets/admin-preview.png',
  ])('allows %s', (url) => {
    expect(isAllowedResultImageUrl(url)).toBe(true)
  })

  it.each([
    'javascript:alert(1)',
    'data:image/png;base64,AAAA',
    'blob:https://example.com/id',
    '//example.com/image.png',
    'ftp://example.com/image.png',
    'uploads/workspace/image.png',
    '/api/user/download?url=https://example.com/image.png',
    '/\\evil',
  ])('rejects %s', (url) => {
    expect(isAllowedResultImageUrl(url)).toBe(false)
  })

  it('does not open rejected URLs', () => {
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null)
    const error = vi.fn()
    const detail = useTaskResultDetail({ message: { success: vi.fn(), error } })

    detail.openResultImage('javascript:alert(1)')

    expect(openSpy).not.toHaveBeenCalled()
    expect(error).toHaveBeenCalledWith('图片链接不安全，已拒绝打开')

    openSpy.mockRestore()
  })
})
