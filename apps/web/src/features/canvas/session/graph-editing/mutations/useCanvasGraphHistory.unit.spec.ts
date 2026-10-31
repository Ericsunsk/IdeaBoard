import { describe, expect, it, vi } from 'vitest'
import { createEmptyBoardSnapshot } from '../../../../board'
import { CanvasGraphSession } from '../../board-session'
import { useCanvasGraphHistory } from './useCanvasGraphHistory'

function createHarness(
  harnessOptions: {
    now?: () => number
    isAttached?: () => boolean
    getBoardId?: () => string | null | undefined
  } = {},
) {
  const snapshot = createEmptyBoardSnapshot()
  snapshot.promptNodes = [
    {
      id: 1,
      type: 'prompt',
      x: 40,
      y: 40,
      width: 280,
      text: 'before',
      outputCount: 1,
      aspectRatio: 'auto',
      inputImages: [],
    },
  ]
  const session = CanvasGraphSession.hydrate('board-1', snapshot)
  const history = useCanvasGraphHistory({
    ...(harnessOptions.now ? { now: harnessOptions.now } : {}),
    ...(harnessOptions.isAttached ? { isAttached: harnessOptions.isAttached } : {}),
    ...(harnessOptions.getBoardId ? { getBoardId: harnessOptions.getBoardId } : {}),
    applyPatch: (patch, direction) =>
      session.applyCommitPatch(patch, direction, {
        shouldRecordHistory: false,
        shouldMarkDirty: true,
      }).didChange,
  })
  session.subscribe((event) => {
    if (event.sideEffects.shouldRecordHistory) history.record(event.patch)
  })
  return { session, history }
}

describe('useCanvasGraphHistory', () => {
  it('undoes and redoes the canonical graph patch without snapshots', () => {
    const { session, history } = createHarness()
    const store = session.store
    store
      .beginTransaction('node-update')
      .updateNode('prompt-1', { x: 240, text: 'after' })
      .selectNode('prompt-1', true)
      .commit()

    history.undo()
    expect(store.getNode('prompt-1')).toMatchObject({
      isSelected: false,
      snapshot: { x: 40, text: 'before' },
    })

    history.redo()
    expect(store.getNode('prompt-1')).toMatchObject({
      isSelected: true,
      snapshot: { x: 240, text: 'after' },
    })
  })

  it('does not record visual-only selection commits', () => {
    const { session, history } = createHarness()
    const store = session.store
    store.beginTransaction('node-update').selectNode('prompt-1', true).commit()

    history.undo()
    expect(store.getNode('prompt-1')?.isSelected).toBe(true)
  })

  it('clears redo after a new canonical commit', () => {
    const { session, history } = createHarness()
    const store = session.store
    store.beginTransaction('node-update').updateNode('prompt-1', { x: 120 }).commit()
    history.undo()
    store.beginTransaction('node-update').updateNode('prompt-1', { x: 320 }).commit()
    history.redo()

    expect(store.getNode('prompt-1')?.snapshot.x).toBe(320)
  })

  it('does not overwrite a background materialization when redo is stale', () => {
    const { session, history } = createHarness()
    const store = session.store
    store.beginTransaction('node-update').updateNode('prompt-1', { text: 'edited' }).commit()
    history.undo()
    store
      .beginTransaction('result-materialize', { shouldRecordHistory: false, shouldMarkDirty: true })
      .updateNode('prompt-1', { text: 'materialized' })
      .commit()

    history.redo()

    expect((store.getNode('prompt-1')?.snapshot as { text?: string }).text).toBe('materialized')
  })

  it('coalesces per-keystroke text edits into a single history entry', () => {
    let now = 1_000
    const { session, history } = createHarness({ now: () => now })
    const store = session.store
    store.beginTransaction('node-update').updateNode('prompt-1', { x: 240 }).commit()

    // Twelve keystrokes would evict the structural entry above under
    // maxSteps: 10 without coalescing.
    let text = 'before'
    for (const char of ' typed text!') {
      text += char
      now += 100
      store.beginTransaction('node-update').updateNode('prompt-1', { text }).commit()
    }

    history.undo()
    expect((store.getNode('prompt-1')?.snapshot as { text?: string }).text).toBe('before')

    // The structural entry survived: a second undo restores the position.
    history.undo()
    expect(store.getNode('prompt-1')?.snapshot.x).toBe(40)
  })

  it('starts a new entry when text edits leave the coalescing window', () => {
    let now = 1_000
    const { session, history } = createHarness({ now: () => now })
    const store = session.store
    store.beginTransaction('node-update').updateNode('prompt-1', { text: 'before-a' }).commit()
    now += 10_000
    store.beginTransaction('node-update').updateNode('prompt-1', { text: 'before-a-b' }).commit()

    history.undo()
    expect((store.getNode('prompt-1')?.snapshot as { text?: string }).text).toBe('before-a')
    history.undo()
    expect((store.getNode('prompt-1')?.snapshot as { text?: string }).text).toBe('before')
  })

  it('does not coalesce text edits across structural commits', () => {
    let now = 1_000
    const { session, history } = createHarness({ now: () => now })
    const store = session.store
    store.beginTransaction('node-update').updateNode('prompt-1', { text: 'before-a' }).commit()
    now += 100
    store.beginTransaction('node-update').updateNode('prompt-1', { x: 240 }).commit()
    now += 100
    store.beginTransaction('node-update').updateNode('prompt-1', { text: 'before-a-b' }).commit()

    history.undo()
    expect((store.getNode('prompt-1')?.snapshot as { text?: string }).text).toBe('before-a')
    history.undo()
    expect(store.getNode('prompt-1')?.snapshot.x).toBe(40)
    history.undo()
    expect((store.getNode('prompt-1')?.snapshot as { text?: string }).text).toBe('before')
  })

  it('ignores undo/redo while detached and never applies a cross-board patch', () => {
    let isAttached = true
    let boardId: string | null = 'board-1'
    const applyPatch = vi.fn(() => true)
    const snapshot = createEmptyBoardSnapshot()
    snapshot.promptNodes = [
      {
        id: 1,
        type: 'prompt',
        x: 40,
        y: 40,
        width: 280,
        text: 'before',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    ]
    const boardSession = CanvasGraphSession.hydrate('board-1', snapshot)
    const boardHistory = useCanvasGraphHistory({
      isAttached: () => isAttached,
      getBoardId: () => boardId,
      applyPatch,
    })
    boardSession.subscribe((event) => {
      if (event.sideEffects.shouldRecordHistory) boardHistory.record(event.patch)
    })
    boardSession.store
      .beginTransaction('node-update')
      .updateNode('prompt-1', { text: 'board-1 edit' })
      .commit()

    // Mid-switch: the session is disposed and the board id already moved on.
    // The production readCanvasGraphStore throws here; the guard must bail
    // before the applier is ever reached.
    isAttached = false
    boardId = 'board-2'

    expect(() => boardHistory.undo()).not.toThrow()
    expect(() => boardHistory.redo()).not.toThrow()
    expect(applyPatch).not.toHaveBeenCalled()
  })
})
