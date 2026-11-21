import { effectScope, shallowRef, type EffectScope } from 'vue'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import {
  clearStoredAuthState,
  forgetStoredAuthScope,
  setStoredAuth,
} from '../../../../../core/utils/auth'
import { http, HttpResponse, useMswHandlers } from '../../../../../test/msw'
import { createEmptyBoardSnapshot, type BoardSnapshot } from '../../../../board/ports/canvas'
import { loadBoardState, setBoardState } from '../../../../board/ports/canvasSession'
import { getWorkspaceSelectionStore, resetWorkspaceSelectionStore } from '../../../../workspace'
import { CanvasGraphSession } from '../../board-session'
import { BoardGraphSession, collectPendingImageTasks, createPendingImageExecutionCore } from '..'
import { useCanvasExecutionWorkspace } from './useCanvasExecutionWorkspace'

const WORKSPACE_ID = 'ws_origin123'
const BOARD_ID = 'board_origin123'
const TASK_ID = 'task_accepted123'
const USER_ID = '00000000-0000-0000-0000-000000000001'
const scopes: EffectScope[] = []

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

function login(publicId = USER_ID) {
  setStoredAuth({ public_id: publicId, display_name: 'tester', avatar: '', role: 'user' })
}

function buildSnapshot(text = 'draw') {
  const snapshot = createEmptyBoardSnapshot()
  snapshot.promptNodes.push({
    id: 1,
    type: 'prompt',
    x: 0,
    y: 0,
    width: 280,
    text,
    outputCount: 1,
    aspectRatio: 'auto',
    inputImages: [],
  })
  return snapshot
}

function buildSubmit() {
  const started = deferred<{ boardId: string; actionId: string | null }>()
  const response = deferred<number>()
  const requests = vi.fn()
  useMswHandlers(
    http.post('/api/v1/workspaces/:workspaceId/tasks', async ({ request, params }) => {
      const body = (await request.json()) as { data: { board_id: string } }
      requests(params.workspaceId, body)
      started.resolve({
        boardId: body.data.board_id,
        actionId: request.headers.get('Idempotency-Key'),
      })
      const status = await response.promise
      return status === 202
        ? HttpResponse.json(
            { data: { id: TASK_ID, workspace_id: WORKSPACE_ID, status: 'pending' } },
            { status },
          )
        : HttpResponse.json(
            { error: { code: 'validation_failed', message: 'Invalid submission' } },
            { status },
          )
    }),
  )
  return { started: started.promise, respond: response.resolve, requests }
}

function buildWorkspace(boardId = BOARD_ID, snapshot: BoardSnapshot = buildSnapshot()) {
  const scope = effectScope()
  scopes.push(scope)
  let currentBoardId = boardId
  let currentGraph = CanvasGraphSession.hydrate(boardId, snapshot)
  const originGraph = currentGraph
  const originStore = originGraph.store
  let canWriteOrigin = true
  originStore.setDomainWritePolicy(() => canWriteOrigin)
  const core = createPendingImageExecutionCore()
  core.attachBoard(BoardGraphSession.create(boardId, { graphSession: currentGraph }))
  const waitTaskImageOutcome = vi.fn(async () => ({
    kind: 'completed' as const,
    results: [
      {
        workspaceId: WORKSPACE_ID,
        assetId: 'asset_output123',
        resultIndex: 0,
        width: 400,
        height: 200,
        isPreviewReady: true,
      },
    ],
  }))
  const flushAutosave = vi.fn(async () => {
    setBoardState(currentBoardId, currentGraph.exportSnapshot())
  })
  const cancelTasks = vi.fn()
  const onWarning = vi.fn()
  const onSuccess = vi.fn()
  const workspace = scope.run(() =>
    useCanvasExecutionWorkspace({
      nodes: shallowRef([]),
      edges: shallowRef([]),
      readCanvasGraphStore: () => currentGraph.store,
      rendererMetrics: { readNodeMetrics: () => null },
      boardBaseSnapshot: shallowRef(snapshot),
      executionCore: core,
      createClientActionId: () => 'canvas:origin-action',
      flushAutosave,
      waitTaskImageOutcome,
      cancelTasks,
      onWarning,
      onSuccess,
    }),
  )!
  return {
    workspace,
    core,
    originGraph,
    originStore,
    flushAutosave,
    waitTaskImageOutcome,
    cancelTasks,
    onWarning,
    onSuccess,
    replaceBoard(nextBoardId: string) {
      canWriteOrigin = false
      scope.stop()
      currentGraph.dispose()
      core.detachBoard()
      currentBoardId = nextBoardId
      currentGraph = CanvasGraphSession.hydrate(nextBoardId, buildSnapshot('new board'))
      core.attachBoard(BoardGraphSession.create(nextBoardId, { graphSession: currentGraph }))
      setBoardState(nextBoardId, currentGraph.exportSnapshot())
      return currentGraph
    },
    run: () =>
      workspace.commands.runGeneration({
        kind: 'generation',
        activationId: 'origin-activation',
        sourceNodeId: 'prompt-1',
        source: { url: '', ratio: 1 },
        referenceImages: [],
        prompt: 'draw',
        outputCount: 1,
        aspectRatio: 'auto',
        resultOptions: {},
      }),
  }
}

beforeEach(() => {
  localStorage.clear()
  resetWorkspaceSelectionStore()
  login()
  getWorkspaceSelectionStore().select(WORKSPACE_ID)
  setBoardState(BOARD_ID, buildSnapshot())
})

afterEach(() => {
  scopes.splice(0).forEach((scope) => scope.stop())
  vi.restoreAllMocks()
})

describe('accepted Canvas submissions retain their original recovery ownership', () => {
  test.each(['workspace', 'logout', 'board'] as const)(
    'recovers the original accepted Task after %s with a disposed GraphSession',
    async (transition) => {
      const submit = buildSubmit()
      const origin = buildWorkspace()
      const execution = origin.run()
      expect(await submit.started).toEqual({ boardId: BOARD_ID, actionId: 'canvas:origin-action' })
      // Scope navigation already saves the reserved nodes, before the Task response exists.
      setBoardState(BOARD_ID, origin.originGraph.exportSnapshot())
      if (transition === 'workspace') getWorkspaceSelectionStore().select('ws_other123')
      if (transition === 'logout') {
        clearStoredAuthState()
        forgetStoredAuthScope()
        expect(loadBoardState(BOARD_ID)).toBeNull()
        login('00000000-0000-0000-0000-000000000002')
      }
      const nextGraph = origin.replaceBoard('board_next123')
      const nextSnapshot = nextGraph.exportSnapshot()
      const nextDraft = loadBoardState('board_next123')
      const oldSnapshot = origin.originStore.exportSnapshot()

      submit.respond(202)
      await execution

      expect(nextGraph.exportSnapshot()).toEqual(nextSnapshot)
      expect(loadBoardState('board_next123')).toEqual(nextDraft)
      expect(origin.originStore.exportSnapshot()).toEqual(oldSnapshot)
      expect(origin.flushAutosave).not.toHaveBeenCalled()
      expect(origin.waitTaskImageOutcome).not.toHaveBeenCalled()
      expect(origin.cancelTasks).not.toHaveBeenCalled()
      expect(origin.onWarning).not.toHaveBeenCalled()
      expect(origin.onSuccess).not.toHaveBeenCalled()
      expect(() => origin.originGraph.exportSnapshot()).toThrow('detached')
      if (transition !== 'board') expect(loadBoardState(BOARD_ID)).toBeNull()

      login()
      getWorkspaceSelectionStore().select(WORKSPACE_ID)
      const recovered = loadBoardState(BOARD_ID)
      expect(recovered).not.toBeNull()
      expect(collectPendingImageTasks(recovered!)).toEqual([
        expect.objectContaining({ taskId: TASK_ID, clientActionId: 'canvas:origin-action' }),
      ])
      const restored = buildWorkspace(BOARD_ID, recovered!)
      await restored.workspace.commands.reattachPendingImageResults()
      expect(restored.waitTaskImageOutcome).toHaveBeenCalledOnce()
      expect(restored.waitTaskImageOutcome).toHaveBeenCalledWith(TASK_ID, expect.any(Object))
      expect(restored.originGraph.exportSnapshot().imageNodes[0]).toMatchObject({
        outputStatus: 'ready',
        sourceMediaRef: { assetId: 'asset_output123' },
      })
      expect(submit.requests).toHaveBeenCalledOnce()
    },
  )

  test('continues observing and saving an ordinary accepted submission', async () => {
    const submit = buildSubmit()
    const origin = buildWorkspace()
    const execution = origin.run()
    await submit.started
    submit.respond(202)
    await execution
    expect(origin.waitTaskImageOutcome).toHaveBeenCalledOnce()
    expect(origin.flushAutosave).toHaveBeenCalled()
    expect(collectPendingImageTasks(loadBoardState(BOARD_ID)!)).toHaveLength(1)
    expect(origin.originGraph.exportSnapshot().imageNodes[0]?.outputStatus).toBe('ready')
    expect(submit.requests).toHaveBeenCalledOnce()
  })

  test('preserves the existing error path for an unaccepted submission', async () => {
    const submit = buildSubmit()
    const origin = buildWorkspace()
    const execution = origin.run()
    await submit.started
    submit.respond(422)
    await execution
    expect(origin.onWarning).toHaveBeenCalledOnce()
    expect(origin.waitTaskImageOutcome).not.toHaveBeenCalled()
    expect(collectPendingImageTasks(loadBoardState(BOARD_ID)!)).toEqual([])
    expect(submit.requests).toHaveBeenCalledOnce()
  })

  test('does not observe an old scope while its GraphSession has not yet detached', async () => {
    const submit = buildSubmit()
    const origin = buildWorkspace()
    const execution = origin.run()
    await submit.started
    getWorkspaceSelectionStore().select('ws_other123')
    submit.respond(202)
    await execution
    expect(origin.waitTaskImageOutcome).not.toHaveBeenCalled()
    expect(origin.flushAutosave).not.toHaveBeenCalled()
    expect(origin.cancelTasks).not.toHaveBeenCalled()
    getWorkspaceSelectionStore().select(WORKSPACE_ID)
    expect(collectPendingImageTasks(loadBoardState(BOARD_ID)!)).toHaveLength(1)
  })

  test('rejects unavailable recovery storage before sending a Task request', async () => {
    const submit = buildSubmit()
    const origin = buildWorkspace()
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.spyOn(localStorage, 'setItem').mockImplementationOnce(() => {
      throw new DOMException('Full', 'QuotaExceededError')
    })
    await origin.run()
    expect(submit.requests).not.toHaveBeenCalled()
    expect(origin.onWarning).toHaveBeenCalledOnce()
  })

  test('reports an accepted taskId when storage refuses it without retrying or cancelling', async () => {
    const submit = buildSubmit()
    const origin = buildWorkspace()
    const execution = origin.run()
    await submit.started
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.spyOn(localStorage, 'setItem').mockImplementationOnce(() => {
      throw new DOMException('Full', 'QuotaExceededError')
    })
    submit.respond(202)
    await execution
    expect(warn).toHaveBeenCalledWith(
      '[Canvas] Failed to retain accepted task identity',
      expect.objectContaining({ code: 'accepted_task_recovery_failed', taskId: TASK_ID }),
    )
    expect(origin.onWarning).toHaveBeenCalledOnce()
    expect(origin.waitTaskImageOutcome).toHaveBeenCalledOnce()
    expect(origin.originGraph.exportSnapshot().imageNodes[0]?.outputStatus).toBe('ready')
    expect(origin.cancelTasks).not.toHaveBeenCalled()
    expect(submit.requests).toHaveBeenCalledOnce()
  })

  test('keeps accepted runtime and local identity when graph attachment throws', async () => {
    const submit = buildSubmit()
    const origin = buildWorkspace()
    const execution = origin.run()
    await submit.started
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.spyOn(origin.originStore, 'beginTransaction').mockImplementationOnce(() => {
      throw new Error('Graph commit failed')
    })
    submit.respond(202)
    await execution
    expect(warn).toHaveBeenCalledWith(
      '[Canvas] Failed to retain accepted task identity',
      expect.objectContaining({ code: 'accepted_task_recovery_failed', taskId: TASK_ID }),
    )
    expect(origin.core.snapshot().executions[0]).toMatchObject({
      status: 'running',
      taskId: TASK_ID,
    })
    expect(collectPendingImageTasks(loadBoardState(BOARD_ID)!)).toHaveLength(1)
    expect(origin.originGraph.exportSnapshot().imageNodes).toHaveLength(1)
    expect(origin.onWarning).toHaveBeenCalledOnce()
    expect(origin.cancelTasks).not.toHaveBeenCalled()
    expect(submit.requests).toHaveBeenCalledOnce()
  })
})
