import { afterEach, it, expect, vi } from 'vitest'
import { defineComponent, h, onMounted, ref } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory, RouterView } from 'vue-router'
import {
  createMockWorkspaceBoardApiState,
  createMockWorkspaceBoardApiRequest,
  primeAuthenticatedWorkspace,
  makeServerMeta,
} from '@/features/board/testing'
import { useBoardRepository } from '@/features/board/session/catalog/repository'
import { createBoardSessionRepository } from '@/features/canvas/session/board-session'
import { createGalleryController } from '@/features/board/ui/gallery/controllers/galleryController'
import { queryClient } from '@/core/query/queryClient'
import { createEmptyBoardSnapshot } from '../../core/document/nodes'
const cleanups: Array<() => void> = []
afterEach(() => {
  cleanups
    .splice(0)
    .reverse()
    .forEach((cleanup) => cleanup())
  queryClient.clear()
})
const mocks = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/api/http/client', () => ({ api: { request: mocks.request } }))
it.each(['auto-open', 'create'])(
  '%s: auto-open first Canvas load must hydrate successfully',
  async (mode) => {
    localStorage.clear()
    queryClient.clear()
    const workspaceId = mode === 'auto-open' ? 'ws_autoopen123' : 'ws_create123'
    primeAuthenticatedWorkspace(workspaceId)
    const state = createMockWorkspaceBoardApiState()
    const request = createMockWorkspaceBoardApiRequest(state)
    const order: string[] = []
    mocks.request.mockImplementation(async (url, config) => {
      const method = config?.method ?? 'GET'
      order.push(method + ' ' + url)
      const response = await request(url, config)
      if (method === 'POST' && /\/boards$/.test(url)) {
        const meta = state.createdBoards.at(-1)!
        const boardId = String(meta.id)
        // Match PostgreSQL V1InitialDocument: the use case must seed schema 4.
        meta.schema_version = 1
        meta.capabilities = makeServerMeta().capabilities
        state.boardsResponse.data.push(meta)
        state.documentById[boardId] = {
          board_id: meta.id,
          revision: 1,
          schema_version: 1,
          document: { schema_version: 1, payload: {} },
          updated_at: meta.updated_at,
        }
      }
      if (method === 'PUT' && url.endsWith('/document')) {
        const meta = state.boardsResponse.data[0]!
        const document = state.documentById[String(meta.id)]!
        meta.revision = document.revision
        meta.schema_version = document.schema_version
      }
      return response
    })
    let session: ReturnType<typeof createBoardSessionRepository> | undefined
    let loadResult: unknown
    let loadDone: Promise<void> | undefined
    const Canvas = defineComponent({
      setup() {
        const repo = createBoardSessionRepository()
        session = repo
        onMounted(() => {
          loadDone = repo.load('board_server_created').then(
            (v) => {
              loadResult = v
            },
            (e) => {
              loadResult = e
            },
          )
        })
        return () => h('div', 'canvas')
      },
    })
    const Gallery = defineComponent({ setup: () => () => h('div', 'gallery') })
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', component: Gallery },
        { path: '/canvas', component: Canvas },
      ],
    })
    await router.push('/')
    const app = mount(defineComponent({ setup: () => () => h(RouterView) }), {
      global: { plugins: [router] },
    })
    cleanups.push(() => app.unmount())
    const repository = useBoardRepository()
    const seenAtNavigate: string[][] = []
    const catalog = createGalleryController({
      catalog: repository,
      scenes: { sceneMap: ref({}), workspaceId: ref(workspaceId) },
      navigation: {
        openBoard: (boardId) => {
          seenAtNavigate.push(repository.viewModel.boards.value.map((b) => b.id))
          return router.push({ path: '/canvas', query: { boardId } })
        },
      },
      feedback: {
        createFailed: vi.fn(),
        renameValidationFailed: vi.fn(),
        renameSyncFailed: vi.fn(),
        renamed: vi.fn(),
        copyFailed: vi.fn(),
        copied: vi.fn(),
        deleteFailed: vi.fn(),
        deleted: vi.fn(),
        catalogSyncFailed: vi.fn(),
        catalogReloaded: vi.fn(),
      },
      warn: vi.fn(),
    })
    cleanups.push(() => catalog.dispose())
    if (mode === 'auto-open') await catalog.commands.createAndOpenBoard()
    else {
      const created = await repository.commands.create()
      if (!created.isOk) throw new Error('baseline create failed')
      await router.push({ path: '/canvas', query: { boardId: created.board.id } })
    }
    await flushPromises()
    await loadDone
    expect(loadResult).toMatchObject(createEmptyBoardSnapshot())
    const seedIndex = order.findIndex((entry) => entry.startsWith('PUT '))
    expect(seedIndex).toBeGreaterThan(-1)
    expect(order.findIndex((entry) => entry.endsWith('/bundle'))).toBeGreaterThan(seedIndex)
    if (mode === 'auto-open') expect(seenAtNavigate).toEqual([[]])
    expect(repository.viewModel.boards.value.map((b) => b.id)).toContain('board_server_created')
    expect(repository.viewModel.byId.value.get('board_server_created')?.revision).toBe(2)
    await session!.save('board_server_created', createEmptyBoardSnapshot())
    expect(state.savedCalls.map((call) => call.headers['If-Match'])).toEqual(['1', '2'])
  },
)
