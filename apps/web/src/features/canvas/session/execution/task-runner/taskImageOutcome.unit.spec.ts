import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  getWorkspaceMediaAssetMetadata,
  resetMediaWorkspaceIdResolver,
  setMediaWorkspaceIdResolver,
} from '../../../../../api/mediaAssetDelivery'
import { extractWorkspaceTaskImageResults } from '../workspace/canvasTaskWaiting'
import { createTaskImageObserver } from './taskImageOutcome'

vi.mock('../../../../../api/mediaAssetDelivery', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../../../../api/mediaAssetDelivery')>()
  return { ...actual, getWorkspaceMediaAssetMetadata: vi.fn() }
})

const metadataMock = vi.mocked(getWorkspaceMediaAssetMetadata)
const workspaceId = 'ws_task123'
const taskId = 'task_result123'
const assetId = 'asset_result123'
const url = `/api/v1/workspaces/${workspaceId}/media-assets/${assetId}/derivatives/preview`

function waitOptions() {
  return { deadlineAt: Date.now() + 60_000, signal: new AbortController().signal }
}

describe('Task image observation Workspace identity', () => {
  beforeEach(() => {
    metadataMock.mockReset()
    metadataMock.mockResolvedValue({
      asset_id: assetId,
      status: 'ready',
      derivatives: { preview: { status: 'ready', url } },
    } as never)
  })

  afterEach(() => {
    resetMediaWorkspaceIdResolver()
  })

  it.each(['ws_other123', null])(
    'keeps terminal Task delivery bound when selection is %s',
    async (selection) => {
      const results = extractWorkspaceTaskImageResults(workspaceId, [
        { kind: 'asset', asset_id: assetId, role: 'image', result_index: 0 },
      ])
      setMediaWorkspaceIdResolver(() => selection)
      const wait = vi.fn(async () => ({ kind: 'completed' as const, results }))
      const loadImageSize = vi.fn(async () => ({ width: 400, height: 200 }))
      const observation = await createTaskImageObserver(wait, loadImageSize)(taskId, waitOptions())

      await expect(observation.resolve(0, 1, waitOptions())).resolves.toMatchObject({
        kind: 'completed',
        result: { url, mediaRef: { assetId, derivativeTier: 'preview' }, ratio: 0.5 },
      })
      expect(metadataMock).toHaveBeenCalledWith(workspaceId, assetId, expect.any(Object))
      expect(loadImageSize).toHaveBeenCalledWith(url, undefined, expect.any(AbortSignal))
      expect(wait).toHaveBeenCalledTimes(1)
    },
  )

  it('keeps partial cancellation scoped while missing slots stay cancelled', async () => {
    setMediaWorkspaceIdResolver(() => null)
    const results = extractWorkspaceTaskImageResults(workspaceId, [
      { kind: 'asset', asset_id: assetId, role: 'image', result_index: 1 },
    ])
    const wait = vi.fn(async () => ({ kind: 'cancelled' as const, results }))
    const loadImageSize = vi.fn(async () => ({ width: 400, height: 200 }))
    const observation = await createTaskImageObserver(wait, loadImageSize)(taskId, waitOptions())

    await expect(observation.resolve(0, 1, waitOptions())).resolves.toEqual({
      kind: 'cancelled',
      result: null,
    })
    await expect(observation.resolve(1, 1, waitOptions())).resolves.toMatchObject({
      kind: 'cancelled',
      result: { url, resultIndex: 1, mediaRef: { assetId, derivativeTier: 'preview' } },
    })
    expect(wait).toHaveBeenCalledTimes(1)
    expect(metadataMock).toHaveBeenCalledTimes(1)
    expect(metadataMock).toHaveBeenCalledWith(workspaceId, assetId, expect.any(Object))
  })
})
