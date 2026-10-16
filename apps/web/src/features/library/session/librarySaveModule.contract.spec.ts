import { mediaDisplayFixture } from '../../../test/mediaDisplay'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { createLibraryFolderFacts, createLibraryMutationPublisher } from './folderFacts'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import {
  createLibrarySaveModule,
  resolveLibrarySaveFolderId,
  type LibrarySaveModule,
} from './librarySaveModule'

const WORKSPACE_ID = 'ws_test1234'
const FOLDER_ID = 'lfolder_folder0001'

function externalAssetRow(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 'lasset_asset0001',
    workspace_id: WORKSPACE_ID,
    folder_id: FOLDER_ID,
    source: 'external',
    provider: 'unsplash',
    source_id: 'u1',
    source_url: 'https://unsplash.com/photos/u1',
    preview_url: 'https://images.unsplash.com/u1?w=640',
    original_url: 'https://images.unsplash.com/u1?w=2400',
    title: 'dunes',
    author: 'a',
    width: 1200,
    height: 800,
    created_at: '2026-09-23T00:00:00Z',
    display: mediaDisplayFixture(),
    updated_at: '2026-09-23T00:00:00Z',
    ...overrides,
  }
}

function mediaAssetRow(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 'lasset_media0001',
    workspace_id: WORKSPACE_ID,
    folder_id: FOLDER_ID,
    source: 'media',
    media_asset_id: 'asset_upload0001',
    title: 'canvas export',
    width: 1600,
    height: 900,
    created_at: '2026-10-11T09:30:00Z',
    display: mediaDisplayFixture(),
    updated_at: '2026-10-11T09:30:00Z',
    ...overrides,
  }
}

const searchReference = {
  provider: 'unsplash' as const,
  id: 'u1',
  source_url: 'https://unsplash.com/photos/u1',
  preview_url: 'https://images.unsplash.com/u1?w=640',
  original_url: 'https://images.unsplash.com/u1?w=2400',
  title: 'dunes',
  author: 'a',
  width: 1200,
  height: 800,
}

function harness(options: { workspaceId?: string | null } = {}) {
  const workspaceId = ref<string | null>(options.workspaceId ?? WORKSPACE_ID)
  const published: unknown[] = []
  const notifications: Array<{ level: string; message: string }> = []
  const fetchFolders = vi.fn(async () => [])
  const publisher = createLibraryMutationPublisher(createLibraryFolderFacts(fetchFolders))
  publisher.subscribe((outcomes) => {
    published.push(...outcomes)
  })
  const uploads: File[] = []
  const saveCalls: Array<{ body: Record<string, unknown>; idempotencyKey: string | null }> = []
  let uploadImpl: (file: File) => Promise<{ mediaAssetId: string }> = async (file) => {
    uploads.push(file)
    return { mediaAssetId: 'asset_upload0001' }
  }
  const module: LibrarySaveModule = createLibrarySaveModule({
    workspaceId,
    mutationPublisher: publisher,
    notify: {
      success: (message) => notifications.push({ level: 'success', message }),
      warning: (message) => notifications.push({ level: 'warning', message }),
      error: (message) => notifications.push({ level: 'error', message }),
    },
    // Production save transport (`workspaceLibraryApi.saveAsset`) under MSW;
    // uploads ride a controllable port so byte reuse is observable.
    uploadFile: (file) => uploadImpl(file),
  })
  return {
    workspaceId,
    publisher,
    fetchFolders,
    published,
    notifications,
    uploads,
    saveCalls,
    module,
    setUploadImpl(impl: (file: File) => Promise<{ mediaAssetId: string }>) {
      uploadImpl = impl
    },
  }
}

function saveHandler(
  saveCalls: Array<{ body: Record<string, unknown>; idempotencyKey: string | null }>,
  respond: (call: { body: Record<string, unknown>; index: number }) => Response | Promise<Response>,
) {
  return http.post('/api/v1/workspaces/:workspaceId/library/assets', async ({ request }) => {
    const body = (await request.json()) as Record<string, unknown>
    const call = { body, idempotencyKey: request.headers.get('Idempotency-Key') }
    saveCalls.push(call)
    return respond({ body, index: saveCalls.length - 1 })
  })
}

describe('library save module contract', () => {
  beforeEach(() => vi.clearAllMocks())

  it('saves a Search reference through the production transport with the external wire shape', async () => {
    const h = harness()
    useMswHandlers(
      saveHandler(h.saveCalls, () =>
        HttpResponse.json({ data: externalAssetRow() }, { status: 201 }),
      ),
    )

    const result = await h.module.submit({ source: 'external', reference: searchReference })

    expect(result.status).toBe('succeeded')
    expect(h.saveCalls).toHaveLength(1)
    expect(h.saveCalls[0]?.body).toEqual({
      data: {
        source: 'external',
        provider: 'unsplash',
        source_id: 'u1',
        source_url: 'https://unsplash.com/photos/u1',
        preview_url: 'https://images.unsplash.com/u1?w=640',
        original_url: 'https://images.unsplash.com/u1?w=2400',
        title: 'dunes',
        author: 'a',
        width: 1200,
        height: 800,
      },
    })
    expect(h.saveCalls[0]?.idempotencyKey).toMatch(/^library:asset-save:/)
    expect(h.uploads).toHaveLength(0)
    // One committed record: one publication, one notification.
    expect(h.published).toEqual([
      {
        kind: 'asset-saved',
        source: 'external',
        provider: 'unsplash',
        sourceID: 'u1',
        assetKey: 'unsplash:u1',
        affectedFolderKeys: ['all', FOLDER_ID],
        galleryEffect: 'reload-current',
      },
    ])
    expect(h.notifications.filter((item) => item.level === 'success')).toHaveLength(1)
    expect(result.items[0]?.asset).toMatchObject({ source: 'external', sourceId: 'u1' })
    // UI observes per-file state on the module.
    expect(h.module.operations.value).toHaveLength(1)
    expect(h.module.operations.value[0]?.items[0]?.status).toBe('saved')
  })

  it('uploads Canvas bytes once and saves the media wire shape without URL keys', async () => {
    const h = harness()
    useMswHandlers(
      saveHandler(h.saveCalls, () => HttpResponse.json({ data: mediaAssetRow() }, { status: 201 })),
    )

    const bytes = new Blob(['pixels'], { type: 'image/png' })
    const result = await h.module.submit(
      { source: 'media', file: bytes, filename: 'canvas.png', title: 'canvas export' },
      { folderId: FOLDER_ID },
    )

    expect(result.status).toBe('succeeded')
    expect(h.uploads).toHaveLength(1)
    expect(h.saveCalls).toHaveLength(1)
    const data = h.saveCalls[0]?.body.data as Record<string, unknown>
    // Dimensions project server-side from the managed Media row: the save
    // wire carries no width/height and no URL keys.
    expect(data).toEqual({
      source: 'media',
      media_asset_id: 'asset_upload0001',
      title: 'canvas export',
      folder_id: FOLDER_ID,
    })
    expect(h.published).toEqual([
      {
        kind: 'asset-saved',
        source: 'media',
        mediaAssetId: 'asset_upload0001',
        assetKey: 'media:asset_upload0001',
        affectedFolderKeys: ['all', FOLDER_ID],
        galleryEffect: 'reload-current',
      },
    ])
    expect(result.items[0]?.asset).toMatchObject({
      source: 'media',
      mediaAssetId: 'asset_upload0001',
    })
  })

  it('reads server-projected dimensions including unknown nulls', async () => {
    const h = harness()
    useMswHandlers(
      saveHandler(h.saveCalls, () =>
        HttpResponse.json({ data: mediaAssetRow({ width: null, height: null }) }, { status: 201 }),
      ),
    )

    const file = new File(['pixels'], 'drop.webp', { type: 'image/webp' })
    const result = await h.module.submit({ source: 'media', file })

    expect(result.status).toBe('succeeded')
    if (result.items[0]?.asset?.source !== 'media') throw new Error('expected media asset')
    expect(result.items[0]?.asset.width).toBeNull()
    expect(result.items[0]?.asset.height).toBeNull()
  })

  it('creates one operation target folder for mixed external and media inputs', async () => {
    const h = harness()
    const seenFolderBodies: unknown[] = []
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/library/folders', async ({ request }) => {
        seenFolderBodies.push(await request.json())
        return HttpResponse.json(
          {
            data: {
              id: FOLDER_ID,
              workspace_id: WORKSPACE_ID,
              name: 'fresh',
              created_at: '2026-09-23T00:00:00Z',
              updated_at: '2026-09-23T00:00:00Z',
            },
          },
          { status: 201 },
        )
      }),
      saveHandler(h.saveCalls, ({ body }) =>
        HttpResponse.json(
          {
            data:
              (body.data as { source: string }).source === 'media'
                ? mediaAssetRow()
                : externalAssetRow(),
          },
          { status: 201 },
        ),
      ),
    )

    const result = await h.module.submit(
      [
        { source: 'external', reference: searchReference },
        { source: 'media', mediaAssetId: 'asset_upload0001' },
      ],
      { newFolderName: '  fresh  ' },
    )

    expect(result.status).toBe('succeeded')
    expect(seenFolderBodies).toEqual([{ data: { name: 'fresh' } }])
    expect(result.createdFolderId).toBe(FOLDER_ID)
    // Both items land in the created folder.
    for (const call of h.saveCalls) {
      expect((call.body.data as Record<string, unknown>).folder_id).toBe(FOLDER_ID)
    }
    expect(h.published).toHaveLength(2)
    expect(h.fetchFolders).toHaveBeenCalledOnce()
  })

  it('reuses the uploaded identity and idempotency key when retrying after a save failure', async () => {
    const h = harness()
    let failuresLeft = 1
    useMswHandlers(
      saveHandler(h.saveCalls, () => {
        if (failuresLeft > 0) {
          failuresLeft -= 1
          return HttpResponse.json({ error: { code: 'service_unavailable' } }, { status: 503 })
        }
        return HttpResponse.json({ data: mediaAssetRow() }, { status: 201 })
      }),
    )

    const first = await h.module.submit(
      {
        source: 'media',
        file: new Blob(['pixels'], { type: 'image/png' }),
      },
      { galleryEffect: 'selection-will-reload' },
    )
    expect(first.status).toBe('failed')
    expect(h.uploads).toHaveLength(1)
    expect(h.saveCalls).toHaveLength(1)
    expect(h.published).toHaveLength(0)

    const retried = await h.module.retryOperation(first.operationId)
    expect(retried.status).toBe('succeeded')
    // No second upload, no second Media asset: the same bytes identity and
    // the same save idempotency key replay the failed step.
    expect(h.uploads).toHaveLength(1)
    expect(h.saveCalls).toHaveLength(2)
    expect(h.saveCalls[0]?.idempotencyKey).toBe(h.saveCalls[1]?.idempotencyKey)
    expect(h.published).toHaveLength(1)
    expect(h.published[0]).toMatchObject({ galleryEffect: 'reload-current' })
    expect(h.notifications.filter((item) => item.level === 'success')).toHaveLength(1)
    expect(h.notifications.filter((item) => item.level === 'error')).toHaveLength(1)
  })

  it('replays folder creation with the same key after a lost response', async () => {
    const h = harness()
    const folderKeys: Array<string | null> = []
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/library/folders', ({ request }) => {
        folderKeys.push(request.headers.get('Idempotency-Key'))
        if (folderKeys.length === 1) return HttpResponse.error()
        return HttpResponse.json({
          data: {
            id: FOLDER_ID,
            workspace_id: WORKSPACE_ID,
            name: 'fresh',
            created_at: '2026-09-23T00:00:00Z',
            updated_at: '2026-09-23T00:00:00Z',
          },
        })
      }),
      saveHandler(h.saveCalls, () => HttpResponse.json({ data: mediaAssetRow() })),
    )
    const first = await h.module.submit(
      { source: 'media', mediaAssetId: 'asset_upload0001' },
      { newFolderName: 'fresh' },
    )
    expect(first.status).toBe('failed')
    expect(h.saveCalls).toHaveLength(0)

    const retried = await h.module.retryOperation(first.operationId)
    expect(retried.status).toBe('succeeded')
    expect(retried.createdFolderId).toBe(FOLDER_ID)
    expect(folderKeys).toHaveLength(2)
    expect(folderKeys[0]).toBeTruthy()
    expect(folderKeys[1]).toBe(folderKeys[0])
    expect(h.saveCalls).toHaveLength(1)
    expect(h.published).toHaveLength(1)
  })

  it('converges a lost save response on retry instead of duplicating the asset', async () => {
    const h = harness()
    let calls = 0
    useMswHandlers(
      saveHandler(h.saveCalls, () => {
        calls += 1
        // First attempt: the server commits but the response never arrives.
        if (calls === 1) return HttpResponse.error()
        // Replay with the same idempotency key returns the committed asset.
        return HttpResponse.json({ data: externalAssetRow() }, { status: 200 })
      }),
    )

    const first = await h.module.submit({ source: 'external', reference: searchReference })
    expect(first.status).toBe('failed')
    const retried = await h.module.retryOperation(first.operationId)
    expect(retried.status).toBe('succeeded')
    expect(h.saveCalls).toHaveLength(2)
    expect(h.saveCalls[0]?.idempotencyKey).toBe(h.saveCalls[1]?.idempotencyKey)
    expect(h.published).toHaveLength(1)
    expect(retried.items[0]?.asset).toMatchObject({ source: 'external', sourceId: 'u1' })
  })

  it('does not duplicate saves, publications, or notifications on repeated retries', async () => {
    const h = harness()
    useMswHandlers(
      saveHandler(h.saveCalls, () =>
        HttpResponse.json({ data: externalAssetRow() }, { status: 201 }),
      ),
    )

    const first = await h.module.submit({ source: 'external', reference: searchReference })
    expect(first.status).toBe('succeeded')
    const second = await h.module.retryOperation(first.operationId)
    const third = await h.module.retryOperation(first.operationId)
    expect(second.status).toBe('succeeded')
    expect(third.status).toBe('succeeded')
    expect(h.saveCalls).toHaveLength(1)
    expect(h.published).toHaveLength(1)
    expect(h.notifications).toHaveLength(1)
  })

  it('joins in-flight retries without duplicating uploads or settling the operation early', async () => {
    const h = harness()
    let release!: () => void
    const gate = new Promise<void>((resolve) => {
      release = resolve
    })
    h.setUploadImpl(async (file) => {
      h.uploads.push(file)
      await gate
      return { mediaAssetId: 'asset_upload0001' }
    })
    useMswHandlers(saveHandler(h.saveCalls, () => HttpResponse.json({ data: mediaAssetRow() })))
    const initial = h.module.submit({ source: 'media', file: new File(['pixels'], 'image.png') })
    const operationId = h.module.operations.value[0]?.id
    if (!operationId) throw new Error('Expected an active operation')
    const retries = [h.module.retryOperation(operationId), h.module.retryOperation(operationId)]
    expect(h.module.operations.value[0]?.status).toBe('running')
    release()

    const results = await Promise.all([initial, ...retries])
    expect(results.map((result) => result.status)).toEqual(['succeeded', 'succeeded', 'succeeded'])
    expect(h.uploads).toHaveLength(1)
    expect(h.saveCalls).toHaveLength(1)
    expect(h.published).toHaveLength(1)
    expect(h.notifications).toHaveLength(1)
  })

  it('abandons late completions after a workspace switch without publishing or notifying', async () => {
    const h = harness()
    let releaseUpload!: () => void
    const gate = new Promise<void>((resolve) => {
      releaseUpload = resolve
    })
    h.setUploadImpl(async (file) => {
      h.uploads.push(file)
      await gate
      return { mediaAssetId: 'asset_upload0001' }
    })
    useMswHandlers(
      saveHandler(h.saveCalls, () => HttpResponse.json({ data: mediaAssetRow() }, { status: 201 })),
    )

    const pending = h.module.submit({
      source: 'media',
      file: new Blob(['pixels'], { type: 'image/png' }),
    })
    h.workspaceId.value = 'ws_other1234'
    releaseUpload()
    const result = await pending

    expect(result.status).toBe('abandoned')
    expect(h.saveCalls).toHaveLength(0)
    expect(h.published).toHaveLength(0)
    expect(h.notifications).toHaveLength(0)
    await expect(h.module.retryOperation(result.operationId)).rejects.toThrow()
    expect(h.saveCalls).toHaveLength(0)
    expect(h.module.operations.value).toHaveLength(0)
  })

  it('saves existing Media identity without uploading display bytes', async () => {
    const h = harness()
    useMswHandlers(saveHandler(h.saveCalls, () => HttpResponse.json({ data: mediaAssetRow() })))

    await h.module.submit({ source: 'media', mediaAssetId: 'asset_upload0001', title: 'Original' })

    expect(h.uploads).toHaveLength(0)
    expect(h.saveCalls[0]?.body).toEqual({
      data: {
        source: 'media',
        media_asset_id: 'asset_upload0001',
        title: 'Original',
      },
    })
  })

  it.each(['external', 'media'] as const)(
    'stops an unstarted %s save when Workspace changes',
    async (source) => {
      const h = harness()
      useMswHandlers(saveHandler(h.saveCalls, () => HttpResponse.json({ data: mediaAssetRow() })))
      const pending = h.module.submit(
        source === 'external'
          ? { source, reference: searchReference }
          : { source, mediaAssetId: 'asset_upload0001' },
      )
      h.workspaceId.value = 'ws_other1234'

      expect((await pending).status).toBe('abandoned')
      expect(h.saveCalls).toHaveLength(0)
      expect(h.published).toHaveLength(0)
      expect(h.notifications).toHaveLength(0)
      expect(h.module.operations.value).toHaveLength(0)
    },
  )

  it.each(['clear', 'dispose', 'workspace-roundtrip'] as const)(
    'invalidates pending preparation on %s without resurrecting its state',
    async (invalidation) => {
      const h = harness()
      let release!: (file: File) => void
      const prepared = new Promise<File>((resolve) => {
        release = resolve
      })
      const pending = h.module.submit(
        { source: 'media', file: () => prepared },
        { folderId: FOLDER_ID },
      )
      expect(h.module.operations.value[0]?.items[0]?.status).toBe('preparing')
      if (invalidation === 'workspace-roundtrip') {
        h.workspaceId.value = 'ws_other1234'
        h.workspaceId.value = WORKSPACE_ID
      } else {
        h.module[invalidation]()
      }
      release(new File(['pixels'], 'image.png', { type: 'image/png' }))

      expect((await pending).status).toBe('abandoned')
      expect(h.uploads).toHaveLength(0)
      expect(h.saveCalls).toHaveLength(0)
      expect(h.published).toHaveLength(0)
      expect(h.notifications).toHaveLength(0)
      expect(h.module.operations.value).toHaveLength(0)
    },
  )

  it('retains failed preparations in batch totals and retries only the failed item', async () => {
    const h = harness()
    useMswHandlers(saveHandler(h.saveCalls, () => HttpResponse.json({ data: mediaAssetRow() })))
    const file = new File(['pixels'], 'image.png', { type: 'image/png' })
    const prepare = vi
      .fn<() => Promise<File>>()
      .mockRejectedValueOnce(new Error('prepare failed'))
      .mockResolvedValue(file)
    const first = await h.module.submit([
      { source: 'media', file },
      { source: 'media', file: prepare },
    ])

    expect(first.status).toBe('partial')
    expect(first.items.map((item) => item.status)).toEqual(['saved', 'failed'])
    expect(h.uploads).toHaveLength(1)
    expect(h.notifications[0]?.level).toBe('warning')

    expect((await h.module.retryOperation(first.operationId)).status).toBe('succeeded')
    expect(prepare).toHaveBeenCalledTimes(2)
    expect(h.uploads).toHaveLength(2)
    expect(h.saveCalls).toHaveLength(2)
    expect(h.published).toHaveLength(2)
    expect(h.fetchFolders).toHaveBeenCalledTimes(2)
  })

  it('keeps per-item progress while publishing one batch after all saves settle', async () => {
    const h = harness()
    let release!: () => void
    const gate = new Promise<void>((resolve) => {
      release = resolve
    })
    useMswHandlers(
      saveHandler(h.saveCalls, async ({ index }) => {
        if (index === 1) await gate
        return HttpResponse.json({ data: externalAssetRow({ source_id: `u${index + 1}` }) })
      }),
    )
    const pending = h.module.submit([
      { source: 'external', reference: searchReference },
      { source: 'external', reference: { ...searchReference, id: 'u2' } },
    ])
    await vi.waitFor(() => {
      expect(h.module.operations.value[0]?.items.map((item) => item.status)).toEqual([
        'saved',
        'saving',
      ])
    })
    expect(h.published).toHaveLength(0)
    expect(h.fetchFolders).not.toHaveBeenCalled()

    release()
    expect((await pending).status).toBe('succeeded')
    expect(h.published).toHaveLength(2)
    expect(h.fetchFolders).toHaveBeenCalledOnce()
  })

  it('keeps committed saves successful when a mutation observer throws or rejects', async () => {
    const h = harness()
    const warning = vi.spyOn(console, 'warn').mockImplementation(() => {})
    h.publisher.subscribe(() => {
      throw new Error('observer failed')
    })
    h.publisher.subscribe(async () => {
      throw new Error('async observer failed')
    })
    const received = vi.fn()
    h.publisher.subscribe(received)
    useMswHandlers(saveHandler(h.saveCalls, () => HttpResponse.json({ data: externalAssetRow() })))
    try {
      const result = await h.module.submit([
        { source: 'external', reference: searchReference },
        { source: 'external', reference: { ...searchReference, id: 'u2' } },
      ])

      expect(result.status).toBe('succeeded')
      expect(result.items.map((item) => item.status)).toEqual(['saved', 'saved'])
      expect(received).toHaveBeenCalledOnce()
      expect(received.mock.calls[0]?.[0]).toHaveLength(2)
      expect(h.fetchFolders).toHaveBeenCalledOnce()
      expect((await h.module.retryOperation(result.operationId)).status).toBe('succeeded')
      expect(h.saveCalls).toHaveLength(2)
      expect(h.notifications).toHaveLength(1)
    } finally {
      warning.mockRestore()
    }
  })

  it('stops unstarted steps after dispose', async () => {
    const h = harness()
    useMswHandlers(
      saveHandler(h.saveCalls, () =>
        HttpResponse.json({ data: externalAssetRow() }, { status: 201 }),
      ),
    )

    const first = await h.module.submit({ source: 'external', reference: searchReference })
    expect(first.status).toBe('succeeded')
    h.module.dispose()
    expect(h.module.operations.value).toHaveLength(0)
    // Cleared operations cannot retry into a new scope; new submits fail fast.
    await expect(h.module.retryOperation(first.operationId)).rejects.toThrow()
    await expect(
      h.module.submit({ source: 'external', reference: searchReference }),
    ).rejects.toThrow()
    expect(h.saveCalls).toHaveLength(1)
  })
})

describe('library save folder priority', () => {
  const isKnown = (key: string) => key === FOLDER_ID

  it('prefers hovered over selected over unfiled', () => {
    expect(
      resolveLibrarySaveFolderId('lfolder_other', FOLDER_ID, (key) => key.startsWith('lfolder_')),
    ).toEqual({ kind: 'folder', folderId: 'lfolder_other' })
    expect(resolveLibrarySaveFolderId(undefined, FOLDER_ID, isKnown)).toEqual({
      kind: 'folder',
      folderId: FOLDER_ID,
    })
    expect(resolveLibrarySaveFolderId(undefined, 'all', isKnown)).toEqual({ kind: 'folder' })
  })

  it('treats an explicit all pill as unfiled and an unknown pill as invalid', () => {
    expect(resolveLibrarySaveFolderId('all', FOLDER_ID, isKnown)).toEqual({ kind: 'folder' })
    expect(resolveLibrarySaveFolderId('lfolder_missing', FOLDER_ID, isKnown)).toEqual({
      kind: 'invalid',
    })
  })
})
