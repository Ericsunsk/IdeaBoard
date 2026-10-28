import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  resetMediaWorkspaceIdResolver,
  setMediaWorkspaceIdResolver,
} from '../../../../../api/mediaAssetDelivery'
import { extractWorkspaceTaskImageResults } from '../workspace/canvasTaskWaiting'
import type { ReceiptDerivativeWaiter } from '../result-nodes/resultDisplay'
import { createTaskImageObserver } from './taskImageOutcome'
import { waitForReceiptDerivativeReady } from '../result-nodes/receiptDerivativeEvent'

vi.mock('../result-nodes/receiptDerivativeEvent', () => ({
  waitForReceiptDerivativeReady: vi.fn(async () => {}),
}))

const receiptDerivativeReadyMock = vi.mocked(waitForReceiptDerivativeReady)

const workspaceId = 'ws_task123'
const taskId = 'task_result123'
const assetId = 'asset_result123'
const url = `/api/v1/workspaces/${workspaceId}/media-assets/${assetId}/derivatives/preview`

function waitOptions() {
  return { deadlineAt: Date.now() + 60_000, signal: new AbortController().signal }
}

describe('Task image observation Workspace identity', () => {
  beforeEach(() => {
    receiptDerivativeReadyMock.mockClear()
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
      expect(receiptDerivativeReadyMock).toHaveBeenCalledWith(
        workspaceId,
        assetId,
        expect.any(Object),
      )
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
    expect(receiptDerivativeReadyMock).toHaveBeenCalledTimes(1)
    expect(receiptDerivativeReadyMock).toHaveBeenCalledWith(
      workspaceId,
      assetId,
      expect.any(Object),
    )
  })

  it('threads an injected receipt waiter to the resolve seam', async () => {
    setMediaWorkspaceIdResolver(() => null)
    const results = extractWorkspaceTaskImageResults(workspaceId, [
      { kind: 'asset', asset_id: assetId, role: 'image', result_index: 1 },
    ])
    const wait = vi.fn(async () => ({ kind: 'completed' as const, results }))
    const loadImageSize = vi.fn(async () => ({ width: 400, height: 200 }))
    const waitForReceiptDerivative: ReceiptDerivativeWaiter = vi.fn(async () => {})
    const observation = await createTaskImageObserver(
      wait,
      loadImageSize,
      waitForReceiptDerivative,
    )(taskId, waitOptions())

    await expect(observation.resolve(1, 1, waitOptions())).resolves.toMatchObject({
      kind: 'completed',
      result: { url, resultIndex: 1, mediaRef: { assetId, derivativeTier: 'preview' } },
    })
    expect(waitForReceiptDerivative).toHaveBeenCalledTimes(1)
    expect(waitForReceiptDerivative).toHaveBeenCalledWith(
      workspaceId,
      assetId,
      expect.any(AbortSignal),
    )
    expect(receiptDerivativeReadyMock).not.toHaveBeenCalled()
  })
})
