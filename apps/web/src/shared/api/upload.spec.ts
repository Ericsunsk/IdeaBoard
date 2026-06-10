import { beforeEach, describe, expect, it, vi } from 'vitest'
import { UPLOAD_OPERATION_TIMEOUT_MS, uploadApi } from './upload'

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('./client', () => ({
  api: {
    request: mocks.request,
  },
}))

describe('uploadApi', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.request.mockResolvedValue({ code: 0, msg: 'ok' })
  })

  it('uses a longer per-operation timeout for board uploads', async () => {
    const file = new File(['image'], 'image.png', { type: 'image/png' })

    await uploadApi.upload(file, ' board 1 ')

    expect(mocks.request).toHaveBeenCalledWith(
      '/user/boards/board%201/uploads',
      expect.objectContaining({
        method: 'POST',
        timeout: UPLOAD_OPERATION_TIMEOUT_MS,
      }),
    )
    const body = mocks.request.mock.calls[0]?.[1]?.body
    expect(body).toBeInstanceOf(FormData)
    expect((body as FormData).get('file')).toBe(file)
  })

  it('uses a longer per-operation timeout for avatar uploads', async () => {
    const file = new File(['avatar'], 'avatar.jpg', { type: 'image/jpeg' })

    await uploadApi.uploadAvatar(file)

    expect(mocks.request).toHaveBeenCalledWith(
      '/user/profile/avatar',
      expect.objectContaining({
        method: 'POST',
        timeout: UPLOAD_OPERATION_TIMEOUT_MS,
      }),
    )
    const body = mocks.request.mock.calls[0]?.[1]?.body
    expect(body).toBeInstanceOf(FormData)
    expect((body as FormData).get('file')).toBe(file)
  })
})
