import { beforeEach, describe, expect, it, vi } from 'vitest'
import { BoardWorkspaceUnavailableError } from '../adapters/boardsResource'
import { BOARD_STATE_SCHEMA_VERSION } from '../../core/document/nodes'
import { getWorkspaceSelectionStore, resetWorkspaceSelectionStore } from '../../../workspace'
import { joinSharedBoardByToken } from './joinSharedBoardWorkflow'

const WORKSPACE_ID = 'ws_test123'

const mocks = vi.hoisted(() => {
  const request = vi.fn()
  const hydrateBoardsFromServer = vi.fn()
  return { hydrateBoardsFromServer, request }
})

vi.mock('../../../../api/client', () => ({
  api: {
    request: mocks.request,
  },
}))

vi.mock('../catalog/repository', () => ({
  useBoardRepository: () => ({
    commands: { hydrate: mocks.hydrateBoardsFromServer },
  }),
}))

function joinEnvelope(boardId: string) {
  return {
    data: {
      id: boardId,
      workspace_id: WORKSPACE_ID,
      name: 'Shared Board',
      revision: 3,
      schema_version: BOARD_STATE_SCHEMA_VERSION,
      archived_at: null,
      created_at: '2026-09-25T00:00:00.000Z',
      updated_at: '2026-09-25T00:00:00.000Z',
    },
  }
}

describe('joinSharedBoardByToken', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
    resetWorkspaceSelectionStore()
    getWorkspaceSelectionStore().select(WORKSPACE_ID)
    mocks.request.mockResolvedValue(joinEnvelope('board-1'))
    mocks.hydrateBoardsFromServer.mockResolvedValue({ ok: true })
  })

  it('rejects an empty token before calling the API', async () => {
    await expect(joinSharedBoardByToken('  ')).rejects.toThrow('Share token is missing')

    expect(mocks.request).not.toHaveBeenCalled()
    expect(mocks.hydrateBoardsFromServer).not.toHaveBeenCalled()
  })

  it('fails closed with guidance when no workspace is selected', async () => {
    getWorkspaceSelectionStore().clear()

    await expect(joinSharedBoardByToken('share-token')).rejects.toBeInstanceOf(
      BoardWorkspaceUnavailableError,
    )
    expect(mocks.request).not.toHaveBeenCalled()
    expect(mocks.hydrateBoardsFromServer).not.toHaveBeenCalled()
  })

  it('joins with a trimmed token, hydrates the catalog, and returns the trimmed board id', async () => {
    mocks.request.mockResolvedValueOnce(joinEnvelope(' board-42 '))

    await expect(joinSharedBoardByToken(' share-token ')).resolves.toEqual({
      boardId: 'board-42',
    })

    expectJoinRequestBody()
    expect(mocks.hydrateBoardsFromServer).toHaveBeenCalledOnce()
  })

  it('rejects when the API response does not include a board id', async () => {
    mocks.request.mockResolvedValueOnce(joinEnvelope('   '))

    await expect(joinSharedBoardByToken('share-token')).rejects.toThrow(
      'The server did not return a valid canvas ID',
    )

    expect(mocks.hydrateBoardsFromServer).not.toHaveBeenCalled()
  })

  it('rejects when catalog hydrate fails after joining', async () => {
    const hydrateError = new Error('network failed')
    mocks.hydrateBoardsFromServer.mockResolvedValueOnce({
      ok: false,
      error: hydrateError,
    })

    await expect(joinSharedBoardByToken('share-token')).rejects.toThrow(
      'Canvas sync failed. Please try again later.',
    )

    expectJoinRequestBody()
    expect(mocks.hydrateBoardsFromServer).toHaveBeenCalledOnce()
  })

  it('replaces unsafe API errors with the workflow fallback', async () => {
    mocks.request.mockRejectedValueOnce(new Error('token=exposed'))

    await expect(joinSharedBoardByToken('share-token')).rejects.toThrow(
      'Could not join the shared canvas',
    )

    expect(mocks.hydrateBoardsFromServer).not.toHaveBeenCalled()
  })

  it('passes safe backend failures through untouched', async () => {
    const backendError = new Error('分享链接已失效') as Error & {
      hasSafeDisplayMessage?: boolean
    }
    backendError.hasSafeDisplayMessage = true
    mocks.request.mockRejectedValueOnce(backendError)

    await expect(joinSharedBoardByToken('share-token')).rejects.toThrow('分享链接已失效')

    expect(mocks.hydrateBoardsFromServer).not.toHaveBeenCalled()
  })
})

function expectJoinRequestBody() {
  expect(mocks.request).toHaveBeenCalledWith(
    `/v1/workspaces/${WORKSPACE_ID}/boards/join`,
    expect.objectContaining({
      method: 'POST',
    }),
  )
  const body = mocks.request.mock.calls[0]?.[1]?.body
  const parsed =
    typeof body === 'string'
      ? (JSON.parse(body) as Record<string, unknown>)
      : (body as Record<string, unknown>)
  expect(parsed).toEqual({ data: { token: 'share-token' } })
  const headers = mocks.request.mock.calls[0]?.[1]?.headers as Record<string, unknown>
  expect(headers['Idempotency-Key']).toMatch(/^board-join:/)
}
