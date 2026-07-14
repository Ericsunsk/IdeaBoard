import { describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../test/msw'
import { UPLOAD_OPERATION_TIMEOUT_MS, uploadApi } from './upload'

async function readUploadedFile(request: Request): Promise<File> {
  const formData = await request.formData()
  const file = formData.get('file')
  expect(file).toBeInstanceOf(File)
  return file as File
}

describe('uploadApi', () => {
  it('uploads a board asset through the HTTP boundary', async () => {
    const file = new File(['image'], 'image.png', { type: 'image/png' })
    const receivedRequests: Array<{
      method: string
      path: string
      fileName: string
      fileType: string
      fileText: string
    }> = []

    useMswHandlers(
      http.post('/api/user/boards/:boardId/uploads', async ({ request }) => {
        const uploadedFile = await readUploadedFile(request)
        receivedRequests.push({
          method: request.method,
          path: new URL(request.url).pathname,
          fileName: uploadedFile.name,
          fileType: uploadedFile.type,
          fileText: await uploadedFile.text(),
        })

        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            url: '/uploads/boards/board-1/image.png',
            width: 640,
            height: 480,
            mime: uploadedFile.type,
            ext: '.png',
          },
        })
      }),
    )

    await expect(uploadApi.upload(file, ' board 1 ')).resolves.toEqual({
      url: '/uploads/boards/board-1/image.png',
      width: 640,
      height: 480,
      mime: 'image/png',
      ext: '.png',
    })
    expect(receivedRequests).toEqual([
      {
        method: 'POST',
        path: '/api/user/boards/board%201/uploads',
        fileName: 'image.png',
        fileType: 'image/png',
        fileText: 'image',
      },
    ])
  })

  it('uploads a profile avatar through the HTTP boundary', async () => {
    const file = new File(['avatar'], 'avatar.jpg', { type: 'image/jpeg' })
    const receivedRequests: Array<{
      method: string
      path: string
      fileName: string
      fileType: string
      fileText: string
    }> = []

    useMswHandlers(
      http.post('/api/user/profile/avatar', async ({ request }) => {
        const uploadedFile = await readUploadedFile(request)
        receivedRequests.push({
          method: request.method,
          path: new URL(request.url).pathname,
          fileName: uploadedFile.name,
          fileType: uploadedFile.type,
          fileText: await uploadedFile.text(),
        })

        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            avatar: '/uploads/avatars/me.jpg',
            width: 128,
            height: 128,
            mime: uploadedFile.type,
            ext: '.jpg',
          },
        })
      }),
    )

    await expect(uploadApi.uploadAvatar(file)).resolves.toEqual({
      avatar: '/uploads/avatars/me.jpg',
      width: 128,
      height: 128,
      mime: 'image/jpeg',
      ext: '.jpg',
    })
    expect(receivedRequests).toEqual([
      {
        method: 'POST',
        path: '/api/user/profile/avatar',
        fileName: 'avatar.jpg',
        fileType: 'image/jpeg',
        fileText: 'avatar',
      },
    ])
  })

  it('keeps the upload timeout long enough for slow HTTP responses', async () => {
    vi.useFakeTimers()

    try {
      const slowResponseDelayMs = 10_050
      const file = new File(['slow avatar'], 'slow-avatar.jpg', { type: 'image/jpeg' })
      let resolveUploadStarted: () => void = () => {}
      const uploadStarted = new Promise<void>((resolve) => {
        resolveUploadStarted = resolve
      })

      useMswHandlers(
        http.post('/api/user/profile/avatar', async ({ request }) => {
          await readUploadedFile(request)
          resolveUploadStarted()
          await new Promise((resolve) => setTimeout(resolve, slowResponseDelayMs))

          return HttpResponse.json({
            code: 0,
            msg: 'ok',
            data: {
              avatar: '/uploads/avatars/slow-avatar.jpg',
              mime: 'image/jpeg',
            },
          })
        }),
      )

      const response = uploadApi.uploadAvatar(file)

      await uploadStarted
      expect(UPLOAD_OPERATION_TIMEOUT_MS).toBeGreaterThan(slowResponseDelayMs)
      await vi.advanceTimersByTimeAsync(slowResponseDelayMs)

      await expect(response).resolves.toEqual({
        avatar: '/uploads/avatars/slow-avatar.jpg',
        mime: 'image/jpeg',
      })
    } finally {
      vi.useRealTimers()
    }
  })

  it('surfaces upload API failures from the HTTP response body', async () => {
    useMswHandlers(
      http.post('/api/user/boards/:boardId/uploads', () => {
        return HttpResponse.json({ code: 413, msg: 'upload too large' })
      }),
    )

    await expect(
      uploadApi.upload(new File(['too large'], 'large.png', { type: 'image/png' }), 'board-1'),
    ).rejects.toThrow('upload too large')
  })

  it('rejects successful upload envelopes without domain data', async () => {
    useMswHandlers(
      http.post('/api/user/boards/:boardId/uploads', () => {
        return HttpResponse.json({ code: 0, msg: 'ok' })
      }),
    )

    await expect(
      uploadApi.upload(new File(['image'], 'image.png', { type: 'image/png' }), 'board-1'),
    ).rejects.toThrow('上传图片失败')
  })

  it('adds a contextual fallback to sanitized transport errors without dropping metadata', async () => {
    useMswHandlers(
      http.post('/api/user/boards/:boardId/uploads', () => {
        return HttpResponse.json(
          { code: 'upstream_failure', msg: 'authorization token leaked', data: { retry: true } },
          { status: 503 },
        )
      }),
    )

    await expect(
      uploadApi.upload(new File(['image'], 'image.png', { type: 'image/png' }), 'board-1'),
    ).rejects.toMatchObject({
      code: 'upstream_failure',
      data: { retry: true },
      message: '上传图片失败',
      status: 503,
    })
  })

  it('rejects avatar responses without a usable URL', async () => {
    useMswHandlers(
      http.post('/api/user/profile/avatar', () => {
        return HttpResponse.json({ code: 0, msg: 'ok', data: { avatar: '   ' } })
      }),
    )

    await expect(
      uploadApi.uploadAvatar(new File(['avatar'], 'avatar.png', { type: 'image/png' })),
    ).rejects.toThrow('头像上传失败，请稍后重试')
  })

  it('recycles a board asset and hides the response envelope', async () => {
    let receivedBody: unknown
    useMswHandlers(
      http.delete('/api/user/boards/:boardId/uploads', async ({ request }) => {
        receivedBody = await request.json()
        return HttpResponse.json({ code: 0, msg: 'ok', data: { deleted: true } })
      }),
    )

    await expect(
      uploadApi.recycle('/uploads/canvas/user/board-1/content/image.png', ' board 1 '),
    ).resolves.toBeUndefined()
    expect(receivedBody).toEqual({ url: '/uploads/canvas/user/board-1/content/image.png' })
  })
})
