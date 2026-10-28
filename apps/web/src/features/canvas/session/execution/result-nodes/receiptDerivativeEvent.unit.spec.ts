import { beforeEach, describe, expect, test, vi } from 'vitest'
import { getWorkspaceMediaAssetMetadata } from '../../../../../api/mediaAssetDelivery'
import { subscribeWorkspaceMediaInvalidation } from '../../../../../api/workspaceEventStream'
import { waitForReceiptDerivativeReady } from './receiptDerivativeEvent'

vi.mock('../../../../../api/mediaAssetDelivery', async (importOriginal) => {
  const original = await importOriginal<typeof import('../../../../../api/mediaAssetDelivery')>()
  return { ...original, getWorkspaceMediaAssetMetadata: vi.fn() }
})

vi.mock('../../../../../api/workspaceEventStream', async (importOriginal) => {
  const original = await importOriginal<typeof import('../../../../../api/workspaceEventStream')>()
  return { ...original, subscribeWorkspaceMediaInvalidation: vi.fn() }
})

const metadataMock = vi.mocked(getWorkspaceMediaAssetMetadata)
const subscribeMock = vi.mocked(subscribeWorkspaceMediaInvalidation)

function pendingMetadata(assetId = 'asset_1') {
  return {
    asset_id: assetId,
    status: 'ready',
    derivatives: { preview: { status: 'pending' } },
  } as never
}

function readyMetadata(assetId = 'asset_1') {
  return {
    asset_id: assetId,
    status: 'ready',
    derivatives: { preview: { status: 'ready', url: '/api/preview' } },
  } as never
}

type InvalidationListener = (assetId: string) => void

function captureSubscription() {
  const calls = subscribeMock.mock.calls
  expect(calls.length).toBeGreaterThan(0)
  const [, onInvalidated, onError] = calls[calls.length - 1] as [
    string,
    InvalidationListener,
    (() => void) | undefined,
  ]
  return { onInvalidated, onError }
}

describe('receiptDerivativeEvent', () => {
  beforeEach(() => {
    metadataMock.mockReset()
    subscribeMock.mockReset()
    subscribeMock.mockReturnValue(() => {})
  })

  test('resolves on the fast-path confirm and releases the subscription', async () => {
    const stop = vi.fn()
    subscribeMock.mockReturnValue(stop)
    metadataMock.mockResolvedValue(readyMetadata())

    await expect(waitForReceiptDerivativeReady('ws_test123', 'asset_1')).resolves.toBeUndefined()
    expect(metadataMock).toHaveBeenCalledTimes(1)
    expect(metadataMock).toHaveBeenCalledWith(
      'ws_test123',
      'asset_1',
      expect.objectContaining({ force: true }),
    )
    expect(subscribeMock).toHaveBeenCalledTimes(1)
    expect(stop).toHaveBeenCalledTimes(1)
  })

  test('re-confirms over GET when the same-asset hint fires', async () => {
    metadataMock.mockResolvedValueOnce(pendingMetadata()).mockResolvedValue(readyMetadata())
    const waited = waitForReceiptDerivativeReady('ws_test123', 'asset_1')
    await vi.waitFor(() => expect(subscribeMock).toHaveBeenCalledTimes(1))
    expect(subscribeMock).toHaveBeenCalledWith(
      'ws_test123',
      expect.any(Function),
      expect.any(Function),
    )

    captureSubscription().onInvalidated('asset_1')
    await expect(waited).resolves.toBeUndefined()
    expect(metadataMock).toHaveBeenCalledTimes(2)
    for (const call of metadataMock.mock.calls) {
      expect(call[2]).toEqual(expect.objectContaining({ force: true }))
    }
  })

  test('a hint only prompts a GET confirm and never resolves by itself', async () => {
    metadataMock.mockResolvedValueOnce(pendingMetadata())
    let releaseConfirm!: (metadata: never) => void
    metadataMock.mockImplementationOnce(
      () =>
        new Promise<never>((resolve) => {
          releaseConfirm = resolve
        }),
    )
    const waited = waitForReceiptDerivativeReady('ws_test123', 'asset_1')
    await vi.waitFor(() => expect(subscribeMock).toHaveBeenCalledTimes(1))
    let isSettled = false
    void waited.then(
      () => {
        isSettled = true
      },
      () => {
        isSettled = true
      },
    )

    // A hint for another asset is filtered: no new confirm starts.
    captureSubscription().onInvalidated('asset_other')
    await new Promise((resolve) => setTimeout(resolve, 10))
    expect(metadataMock).toHaveBeenCalledTimes(1)
    expect(isSettled).toBe(false)

    // The matching hint prompts a confirm, but the waiter still waits for it.
    captureSubscription().onInvalidated('asset_1')
    await new Promise((resolve) => setTimeout(resolve, 10))
    expect(metadataMock).toHaveBeenCalledTimes(2)
    expect(isSettled).toBe(false)

    releaseConfirm(readyMetadata())
    await expect(waited).resolves.toBeUndefined()
  })

  test('observes a hint that lands while the confirm GET is in flight', async () => {
    let releaseConfirm!: (metadata: never) => void
    metadataMock.mockImplementationOnce(
      () =>
        new Promise<never>((resolve) => {
          releaseConfirm = resolve
        }),
    )
    metadataMock.mockResolvedValue(readyMetadata())
    const waited = waitForReceiptDerivativeReady('ws_test123', 'asset_1')
    await vi.waitFor(() => expect(metadataMock).toHaveBeenCalledTimes(1))

    // The matching hint arrives before the initial GET resolves.
    captureSubscription().onInvalidated('asset_1')
    releaseConfirm(pendingMetadata())
    await expect(waited).resolves.toBeUndefined()
    expect(metadataMock).toHaveBeenCalledTimes(2)
    expect(metadataMock.mock.calls[1]?.[2]).toEqual(metadataMock.mock.calls[0]?.[2])
  })

  test('re-confirms when the racing invalidation aborts the in-flight GET', async () => {
    let rejectConfirm!: (error: unknown) => void
    metadataMock.mockImplementationOnce(
      () =>
        new Promise<never>((_, reject) => {
          rejectConfirm = reject
        }),
    )
    metadataMock.mockResolvedValue(readyMetadata())
    const waited = waitForReceiptDerivativeReady('ws_test123', 'asset_1')
    await vi.waitFor(() => expect(metadataMock).toHaveBeenCalledTimes(1))

    // Invalidation aborts the GET while the matching hint is latched.
    captureSubscription().onInvalidated('asset_1')
    rejectConfirm(new DOMException('The operation was aborted.', 'AbortError'))
    await expect(waited).resolves.toBeUndefined()
    expect(metadataMock).toHaveBeenCalledTimes(2)
    expect(metadataMock.mock.calls[1]?.[2]).toEqual(metadataMock.mock.calls[0]?.[2])
  })

  test('falls back to the bounded poll on timeout', async () => {
    metadataMock.mockResolvedValue(pendingMetadata())

    await expect(
      waitForReceiptDerivativeReady('ws_test123', 'asset_1', undefined, 30),
    ).resolves.toBeUndefined()
    expect(subscribeMock).toHaveBeenCalledTimes(1)
    // No poll budget remains after waiting for the hint.
    expect(metadataMock).toHaveBeenCalled()
    for (const call of metadataMock.mock.calls) {
      expect(call[2]).toEqual(expect.objectContaining({ force: true }))
    }
  })

  test('falls back to the bounded poll when the transport errors', async () => {
    metadataMock.mockResolvedValueOnce(pendingMetadata()).mockResolvedValue(readyMetadata())
    const waited = waitForReceiptDerivativeReady('ws_test123', 'asset_1')
    await vi.waitFor(() => expect(subscribeMock).toHaveBeenCalledTimes(1))

    captureSubscription().onError?.()
    await expect(waited).resolves.toBeUndefined()
    // The fallback poll re-reads over GET with the existing retry semantics.
    expect(metadataMock).toHaveBeenCalledWith(
      'ws_test123',
      'asset_1',
      expect.not.objectContaining({ force: true }),
    )
  })

  test('falls back to the bounded poll when subscribing throws', async () => {
    metadataMock.mockResolvedValue(readyMetadata())
    subscribeMock.mockImplementation(() => {
      throw new Error('EventSource unavailable')
    })

    await expect(waitForReceiptDerivativeReady('ws_test123', 'asset_1')).resolves.toBeUndefined()
    expect(metadataMock).toHaveBeenCalled()
  })

  test('rejects without subscribing when already aborted', async () => {
    metadataMock.mockResolvedValue(readyMetadata())
    const controller = new AbortController()
    controller.abort()

    await expect(
      waitForReceiptDerivativeReady('ws_test123', 'asset_1', controller.signal),
    ).rejects.toThrow()
    expect(subscribeMock).not.toHaveBeenCalled()
    expect(metadataMock).not.toHaveBeenCalled()
  })
})
