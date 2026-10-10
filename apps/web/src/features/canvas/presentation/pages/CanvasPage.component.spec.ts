import { defineComponent, h, type PropType } from 'vue'
import {
  createMemoryHistory,
  createRouter,
  type RouteLocationNormalizedLoaded,
  type Router,
} from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useUserStore } from '../../../auth'
import { userErrorCode } from '../../../../shared/errors/errorCatalog'
import { makeSessionUser } from '../../../../test/factories'
import { render } from '../../../../test/render'
import type { BoardSnapshot } from '../../../board/ports/canvas'
import type { BoardSessionRepository } from '../../application/board-session/repository'
import {
  BOARD_SESSION_AUTH_UNAVAILABLE_CODE,
  useBoardSession,
} from '../../application/board-session/useBoardSession'
import { makeBoardSnapshot, makeBoardState } from '../../application/testing/boardFixtures'
import CanvasPage from './CanvasPage.vue'

let getUserIdentity: () => { userId: number; userUid: string }

const CanvasScreenStub = defineComponent({
  props: {
    route: { type: Object as PropType<RouteLocationNormalizedLoaded>, required: true },
    router: { type: Object as PropType<Router>, required: true },
    getUserIdentity: {
      type: Function as PropType<() => { userId: number; userUid: string }>,
      required: true,
    },
    logoutAndRedirect: { type: Function as PropType<(path: string) => unknown>, required: true },
  },
  setup(props) {
    getUserIdentity = props.getUserIdentity
    return () =>
      h('section', {
        'data-testid': 'canvas-screen',
        'data-route-name': String(props.route.name ?? ''),
        'data-user-id': `${props.getUserIdentity().userId}`,
        'data-has-router': `${!!props.router}`,
        'data-has-logout': `${typeof props.logoutAndRedirect === 'function'}`,
      })
  },
})

async function renderCanvasPage() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/user/canvas', name: 'UserCanvas', component: CanvasPage }],
  })
  await router.push('/user/canvas?boardId=board-a')
  await router.isReady()

  const view = render(CanvasPage, {
    global: {
      plugins: [router],
      stubs: { CanvasScreen: CanvasScreenStub },
    },
  })
  const snapshot = makeBoardSnapshot({
    imageNodes: [
      {
        id: 1,
        type: 'image-node',
        x: 100,
        y: 200,
        width: 280,
        sourceImageUrl: 'blob:canvas-import',
        sourceImageRatio: 1,
        outputStatus: 'idle',
        outputImages: [],
      },
    ],
  })
  const repository: BoardSessionRepository = {
    exists: vi.fn(() => true),
    load: vi.fn(async () => snapshot),
    saveLocal: vi.fn(async (_boardId: string, saved: BoardSnapshot) => makeBoardState(saved)),
    save: vi.fn(async (_boardId: string, saved: BoardSnapshot) => makeBoardState(saved)),
    setPreview: vi.fn(() => makeBoardState(snapshot)),
    sync: vi.fn(async () => undefined),
  }
  const stabilizeSnapshotForSave = vi.fn(async (saved: BoardSnapshot) => saved)
  const createUploadLifecycle = vi.fn(() => ({ stabilizeSnapshotForSave }))
  const session = useBoardSession({
    getBoardIdQuery: () => router.currentRoute.value.query.boardId,
    getUserIdentity,
    getWorkspaceId: () => 'ws_test123',
    repository,
    previewRenderer: { rendererVersion: 1, render: vi.fn(async () => '') },
    createUploadLifecycle,
    message: { warning: vi.fn() },
  })

  return { view, snapshot, repository, session, createUploadLifecycle, stabilizeSnapshotForSave }
}

describe('CanvasPage', () => {
  beforeEach(() => useUserStore().clearAuth())
  afterEach(() => useUserStore().clearAuth())

  it('delegates Canvas assembly to the Screen module with route and user integration', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/user/canvas', name: 'UserCanvas', component: CanvasPage }],
    })
    await router.push('/user/canvas')
    await router.isReady()

    const view = render(CanvasPage, {
      global: {
        plugins: [router],
        stubs: { CanvasScreen: CanvasScreenStub },
      },
    })

    expect(view.getByTestId('canvas-screen')).toHaveAttribute('data-route-name', 'UserCanvas')
    expect(view.getByTestId('canvas-screen')).toHaveAttribute('data-user-id', '0')
    expect(view.getByTestId('canvas-screen')).toHaveAttribute('data-has-router', 'true')
    expect(view.getByTestId('canvas-screen')).toHaveAttribute('data-has-logout', 'true')
  })

  it('allows an authenticated imported-image save using the session storage namespace', async () => {
    useUserStore().setAuth(makeSessionUser({ storage_namespace: ' USER01 ' }))
    const { snapshot, repository, session, createUploadLifecycle, stabilizeSnapshotForSave } =
      await renderCanvasPage()

    const result = await session.save(snapshot, { waitForPreview: true })

    expect(result.remoteSync).toEqual({ ok: true })
    expect(createUploadLifecycle).toHaveBeenCalledWith({
      userUid: 'USER01',
      boardId: 'board-a',
      workspaceId: 'ws_test123',
      warning: expect.any(Function),
    })
    expect(stabilizeSnapshotForSave).toHaveBeenCalledWith(snapshot)
    expect(repository.save).toHaveBeenCalledWith('board-a', snapshot)
    expect(repository.saveLocal).not.toHaveBeenCalled()
  })

  it('retains a local draft until a refreshed session supplies the missing namespace', async () => {
    const userStore = useUserStore()
    userStore.setAuth(makeSessionUser())
    const { snapshot, repository, session, createUploadLifecycle } = await renderCanvasPage()

    const unavailable = await session.save(snapshot)

    expect(unavailable.remoteSync.ok).toBe(false)
    if (!unavailable.remoteSync.ok) {
      expect(userErrorCode(unavailable.remoteSync.error)).toBe(BOARD_SESSION_AUTH_UNAVAILABLE_CODE)
    }
    expect(repository.saveLocal).toHaveBeenCalledWith('board-a', snapshot)
    expect(repository.save).not.toHaveBeenCalled()
    expect(createUploadLifecycle).not.toHaveBeenCalled()

    userStore.setAuth(makeSessionUser({ storage_namespace: 'USER01' }))
    const recovered = await session.save(snapshot, { waitForPreview: true })

    expect(recovered.remoteSync).toEqual({ ok: true })
    expect(repository.save).toHaveBeenCalledWith('board-a', snapshot)
  })
})
