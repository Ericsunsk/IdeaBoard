import { describe, expect, it } from 'vitest'
import { createEmptyBoardSnapshot } from '../../../board/domain'
import { CanvasGraphSession } from '..'
import { useCanvasGraphHistory } from './useCanvasGraphHistory'

function createHarness() {
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
    applyPatch: (patch, direction) =>
      session.applyCommitPatch(patch, direction, { recordHistory: false, markDirty: true })
        .didChange,
  })
  session.subscribe((event) => {
    if (event.sideEffects.recordHistory) history.record(event.patch)
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
      selected: false,
      snapshot: { x: 40, text: 'before' },
    })

    history.redo()
    expect(store.getNode('prompt-1')).toMatchObject({
      selected: true,
      snapshot: { x: 240, text: 'after' },
    })
  })

  it('does not record visual-only selection commits', () => {
    const { session, history } = createHarness()
    const store = session.store
    store.beginTransaction('node-update').selectNode('prompt-1', true).commit()

    history.undo()
    expect(store.getNode('prompt-1')?.selected).toBe(true)
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
      .beginTransaction('result-materialize', { recordHistory: false, markDirty: true })
      .updateNode('prompt-1', { text: 'materialized' })
      .commit()

    history.redo()

    expect((store.getNode('prompt-1')?.snapshot as { text?: string }).text).toBe('materialized')
  })
})
