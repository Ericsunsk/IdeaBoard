import { beforeEach, describe, expect, it, vi } from 'vitest'
import { joinSharedCanvasBoardByToken } from './joinSharedBoardWorkflow'

const mocks = vi.hoisted(() => {
  const request = vi.fn()
  const hydrateCanvasBoardsFromServer = vi.fn()
  return { hydrateCanvasBoardsFromServer, request }
})

vi.mock('../../../shared/api/client', () => ({
  api: {
    request: mocks.request,
  },
}))

vi.mock('./boardSync', () => ({
  hydrateCanvasBoardsFromServer: mocks.hydrateCanvasBoardsFromServer,
}))

describe('joinSharedCanvasBoardByToken', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.request.mockResolvedValue({
      data: { board: { board_id: 'board-1' } },
    })
    mocks.hydrateCanvasBoardsFromServer.mockResolvedValue({ ok: true })
  })

  it('rejects an empty token before calling the API', async () => {
    await expect(joinSharedCanvasBoardByToken('  ')).rejects.toThrow('缺少分享令牌')

    expect(mocks.request).not.toHaveBeenCalled()
    expect(mocks.hydrateCanvasBoardsFromServer).not.toHaveBeenCalled()
  })

  it('joins with a trimmed token, hydrates the catalog, and returns the trimmed board id', async () => {
    mocks.request.mockResolvedValueOnce({
      data: { board: { board_id: ' board-42 ' } },
    })

    await expect(joinSharedCanvasBoardByToken(' share-token ')).resolves.toEqual({
      boardId: 'board-42',
    })

    expect(expectJoinRequestBody()).toEqual({ token: 'share-token' })
    expect(mocks.hydrateCanvasBoardsFromServer).toHaveBeenCalledOnce()
  })

  it('rejects when the API response does not include a board id', async () => {
    mocks.request.mockResolvedValueOnce({
      data: { board: { board_id: '  ' } },
    })

    await expect(joinSharedCanvasBoardByToken('share-token')).rejects.toThrow(
      '服务端未返回可用画布 ID',
    )

    expect(mocks.hydrateCanvasBoardsFromServer).not.toHaveBeenCalled()
  })

  it('rejects when catalog hydrate fails after joining', async () => {
    const hydrateError = new Error('network failed')
    mocks.hydrateCanvasBoardsFromServer.mockResolvedValueOnce({
      ok: false,
      error: hydrateError,
    })

    await expect(joinSharedCanvasBoardByToken('share-token')).rejects.toThrow(
      '画布同步失败，请稍后重试',
    )

    expect(expectJoinRequestBody()).toEqual({ token: 'share-token' })
    expect(mocks.hydrateCanvasBoardsFromServer).toHaveBeenCalledOnce()
  })

  it('propagates API errors', async () => {
    const apiError = new Error('invalid share token')
    mocks.request.mockRejectedValueOnce(apiError)

    await expect(joinSharedCanvasBoardByToken('share-token')).rejects.toBe(apiError)

    expect(mocks.hydrateCanvasBoardsFromServer).not.toHaveBeenCalled()
  })
})

function expectJoinRequestBody() {
  expect(mocks.request).toHaveBeenCalledWith(
    '/user/boards/share-tokens/join',
    expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
    }),
  )
  const body = mocks.request.mock.calls[0]?.[1]?.body
  if (typeof body !== 'string') throw new Error('Missing join request body')
  return JSON.parse(body) as Record<string, unknown>
}
