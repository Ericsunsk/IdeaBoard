import { afterEach, expect, it, vi } from 'vitest'
import { defineComponent, h, nextTick, ref, shallowRef } from 'vue'
import { render } from '../../../../../test/render'
import { FlowCanvasNode } from '../../../ui/nodes'
import { useCanvasVueFlowGraphAdapter } from '../../flow-adapter'
import type { Node } from '@vue-flow/core'
import { createCanvasGraphSessionRuntime } from './canvasGraphSessionRuntime'
import { createEmptyBoardSnapshot } from '../../../../board/ports/canvas'

vi.mock('@/api/media/mediaWorkspaceScope', () => ({
  readMediaWorkspaceId: () => 'ws_test123',
  setMediaWorkspaceIdResolver: () => {},
  resetMediaWorkspaceIdResolver: () => {},
}))

afterEach(() => {
  vi.restoreAllMocks()
})

async function imageBlob(color: string) {
  const canvas = document.createElement('canvas')
  canvas.width = 32
  canvas.height = 32
  canvas.getContext('2d')!.fillStyle = color
  canvas.getContext('2d')!.fillRect(0, 0, 32, 32)
  return new Promise<Blob>((resolve) => canvas.toBlob((blob) => resolve(blob!), 'image/png'))
}

function buildNodeHarness(localUrl: string) {
  const snapshot = {
    id: 1,
    type: 'image-node' as const,
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    outputStatus: 'idle' as const,
    outputImages: [],
  }
  const nodes = shallowRef<Node[]>([
    {
      id: 'image-1',
      position: { x: 0, y: 0 },
      data: {
        kind: 'image',
        title: 'local.png',
        snapshotNode: snapshot,
        localUpload: { previewUrl: localUrl },
      },
    },
  ])
  const graphAdapter = useCanvasVueFlowGraphAdapter({ nodes, edges: shallowRef([]) })
  type Presentation = InstanceType<typeof FlowCanvasNode>['$props']['imagePresentation']
  const imagePresentation = {
    actionBinding: {
      isActionMenuVisible: false,
      isPromptComposerVisible: false,
      isDirectGenerating: false,
    },
    stageBinding: {
      nodeId: 'image-1',
      isActive: false,
      mode: null,
      retouchStage: null,
      session: {
        inpaint: {
          inpaintNodeId: ref(null),
          inpaintOverlayStyle: ref(null),
          setMaskCanvasRef: vi.fn(),
        },
      },
    },
  } as unknown as Presentation
  const boardSnapshot = { ...createEmptyBoardSnapshot(), imageNodes: [snapshot] }
  const boardId = ref('board-a')
  type RuntimeOptions = Parameters<typeof createCanvasGraphSessionRuntime>[0]
  let runtime!: ReturnType<typeof createCanvasGraphSessionRuntime>
  const Harness = defineComponent({
    setup() {
      runtime = createCanvasGraphSessionRuntime({
        nodes,
        edges: shallowRef([]),
        boardBaseSnapshot: ref(boardSnapshot),
        boardSession: { currentBoardId: boardId } as RuntimeOptions['boardSession'],
        graphAdapter,
        executionCore: {
          attachBoard: vi.fn(),
          detachBoard: vi.fn(),
        } as unknown as RuntimeOptions['executionCore'],
        history: {
          clearBaseline: vi.fn(),
          record: vi.fn(),
        } as unknown as RuntimeOptions['history'],
        markDirty: vi.fn(),
        isNodeExecutionActive: () => false,
        preserveCurrentSelection: (graph) => graph,
        projectCurrentGroupNodeLayers: (items) => items,
        projectCommittedGroupNodeLayer: (node) => node,
        onGroupsChanged: vi.fn(),
      })
      runtime.replaceGraphSnapshot(boardSnapshot, 'hydrate')
      return () =>
        nodes.value[0]
          ? h(FlowCanvasNode, {
              nodeId: 'image-1',
              data: nodes.value[0].data,
              imagePresentation,
            })
          : null
    },
  })
  const view = render(Harness, { global: { stubs: { FlowCanvasNodeHandleRail: true } } })
  return { view, runtime, nodes, graphAdapter, boardSnapshot, boardId }
}

function readyMetadataResponse(assetId: string, managedUrl: string) {
  return new Response(
    JSON.stringify({
      data: {
        id: assetId,
        workspace_id: 'ws_test123',
        status: 'ready',
        media_kind: 'image',
        mime_type: 'image/png',
        width: 32,
        height: 32,
        size_bytes: 100,
        content_url: `/api/v1/workspaces/ws_test123/media-assets/${assetId}/content`,
        derivatives: { preview: { status: 'ready', url: managedUrl } },
        created_at: '2026-11-17T00:00:00Z',
        updated_at: '2026-11-17T00:00:00Z',
      },
    }),
    { headers: { 'content-type': 'application/json' } },
  )
}

it.each(['remote', 'save', 'recovery'] as const)(
  'keeps the complete node image through %s projection and upload handoff',
  async (projection) => {
    const assetId = `asset_handoff_${projection}`
    let resolveMetadata!: (response: Response) => void
    const metadata = new Promise<Response>((resolve) => {
      resolveMetadata = resolve
    })
    const nativeFetch = globalThis.fetch
    vi.spyOn(globalThis, 'fetch').mockImplementation((input, options) => {
      const url = input instanceof Request ? input.url : input.toString()
      if (url.endsWith(`/media-assets/${assetId}`)) return metadata
      if (url.includes('/events')) {
        return new Promise<Response>((_resolve, reject) => {
          options?.signal?.addEventListener('abort', () =>
            reject(new DOMException('Aborted', 'AbortError')),
          )
        })
      }
      return nativeFetch(input, options)
    })
    const localUrl = URL.createObjectURL(await imageBlob('red'))
    const managedUrl = URL.createObjectURL(await imageBlob('blue'))
    const { view, runtime, nodes, graphAdapter, boardSnapshot } = buildNodeHarness(localUrl)

    const nextSnapshot = {
      ...boardSnapshot,
      imageNodes: [{ ...boardSnapshot.imageNodes[0]!, title: 'canonical.png' }],
    }
    try {
      let image = view.container.querySelector('img')!
      await expect.poll(() => image.naturalWidth).toBe(32)
      // Complete the browser-preview work before upload finishes, as with a
      // normal network upload. Small images keep their decoded original frame.
      await expect.poll(() => image.classList.contains('is-tier-loading')).toBe(false)
      image = view.container.querySelector('img')!
      // A same-Board realtime snapshot contains canonical facts only while the
      // upload is still pending. It must not remove this attachment's preview.
      if (projection === 'remote') {
        runtime.applyRemoteSnapshot(nextSnapshot)
      } else if (projection === 'save') {
        runtime.replaceGraphSnapshot(nextSnapshot, 'save-reconcile')
        runtime.commitGraph(
          {
            nodes: [
              {
                id: 'image-1',
                position: { x: 0, y: 0 },
                data: {
                  kind: 'image',
                  title: 'canonical.png',
                  snapshotNode: nextSnapshot.imageNodes[0],
                },
              },
            ],
            edges: [],
          },
          'save-reconcile',
        )
      } else {
        vi.spyOn(graphAdapter, 'applyIncrementalProjection').mockImplementationOnce(() => {
          throw new Error('renderer unavailable')
        })
        runtime
          .readCanvasGraphStore()
          .beginTransaction('node-update')
          .updateNode('image-1', { title: 'canonical.png' })
          .commit()
      }
      await nextTick()
      expect(view.container.querySelector('img')).toBe(image)
      expect(nodes.value[0]?.data.localUpload?.previewUrl).toBe(localUrl)
      expect(nodes.value[0]?.data.snapshotNode.title).toBe('canonical.png')
      const persisted = JSON.stringify(runtime.exportGraphSnapshot(boardSnapshot))
      expect(persisted).not.toContain('localUpload')
      expect(persisted).not.toContain('blob:')
      expect(view.queryByRole('button', { name: 'Upload image as input' })).toBeNull()
      runtime
        .readCanvasGraphStore()
        .beginTransaction('node-update')
        .updateNode('image-1', {
          sourceImageUrl: `/api/v1/workspaces/ws_test123/media-assets/${assetId}/derivatives/preview`,
          sourceMediaRef: { assetId, derivativeTier: 'preview' },
          outputStatus: 'ready',
        })
        .commit()
      graphAdapter.updateNode('image-1', {
        data: { ...nodes.value[0]!.data, localUpload: undefined },
      })
      URL.revokeObjectURL(localUrl)
      await nextTick()
      expect(view.container.querySelector('img')?.naturalWidth).toBe(32)
      expect(view.container.querySelector('img')).toBe(image)
      expect(image).not.toHaveClass('is-tier-loading')
      expect(view.queryByLabelText('Preparing image preview')).toBeNull()

      resolveMetadata(readyMetadataResponse(assetId, managedUrl))
      await expect.poll(() => view.container.querySelector('img')?.src).toBe(managedUrl)
      await expect.poll(() => view.container.querySelector('img')?.naturalWidth).toBe(32)
      expect(nodes.value[0]?.data.localUpload).toBeUndefined()
      expect(JSON.stringify(runtime.exportGraphSnapshot(boardSnapshot))).not.toContain('blob:')
      runtime.applyRemoteSnapshot({ ...boardSnapshot, imageNodes: [] })
      await nextTick()
      expect(view.container.querySelector('.flow-canvas-node')).toBeNull()
    } finally {
      URL.revokeObjectURL(localUrl)
      URL.revokeObjectURL(managedUrl)
    }
  },
)

it.each(['delete', 'hydrate', 'clear'] as const)(
  'does not restore a pending preview after %s',
  async (action) => {
    const localUrl = URL.createObjectURL(await imageBlob('red'))
    const { view, runtime, nodes, graphAdapter, boardSnapshot, boardId } =
      buildNodeHarness(localUrl)
    try {
      await expect.poll(() => view.container.querySelector('img')?.naturalWidth).toBe(32)
      if (action === 'delete') {
        runtime.applyRemoteSnapshot({ ...boardSnapshot, imageNodes: [] })
      } else if (action === 'hydrate') {
        boardId.value = 'board-b'
        runtime.replaceGraphSnapshot(boardSnapshot, 'hydrate')
        runtime.commitGraph(
          {
            nodes: [
              {
                ...nodes.value[0]!,
                data: {
                  kind: 'image',
                  title: 'other-board.png',
                  snapshotNode: boardSnapshot.imageNodes[0],
                },
              },
            ],
            edges: [],
          },
          'hydrate-replace',
        )
      } else {
        graphAdapter.updateNode('image-1', {
          data: { ...nodes.value[0]!.data, localUpload: undefined },
        })
        runtime.applyRemoteSnapshot(boardSnapshot)
      }
      await nextTick()
      expect(view.container.querySelector('img')).toBeNull()
      expect(nodes.value[0]?.data.localUpload).toBeUndefined()
      if (action === 'delete') expect(nodes.value).toHaveLength(0)
    } finally {
      URL.revokeObjectURL(localUrl)
    }
  },
)
