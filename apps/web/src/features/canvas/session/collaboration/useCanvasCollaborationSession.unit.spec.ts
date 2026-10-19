import { describe, expect, it, test, vi } from 'vitest'
import { effectScope, nextTick, ref, shallowRef } from 'vue'
import * as Y from 'yjs'
import { createEmptyBoardSnapshot, type BoardSnapshot } from '../../../board/ports/canvas'
import { snapshotToDocEntries } from './internal/mapping'
import { useCanvasCollaborationSession } from './useCanvasCollaborationSession'

function snapshot(x: number): BoardSnapshot {
  return {
    ...createEmptyBoardSnapshot(),
    promptNodes: [
      {
        id: 1,
        type: 'prompt',
        x,
        y: 20,
        width: 280,
        text: 'hello',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    ],
  }
}

function publish(doc: Y.Doc, value: BoardSnapshot): void {
  const entries = snapshotToDocEntries(value)
  doc.transact(() => {
    for (const [id, node] of entries.nodes) doc.getMap('nodes').set(id, node)
    for (const [id, edge] of entries.edges) doc.getMap('edges').set(id, edge)
  }, 'remote-peer')
}

function setup() {
  const scope = effectScope()
  const boardId = ref('board-a')
  const accessState = ref('workspace')
  const canWrite = ref(true)
  const transport = {
    sharedDoc: shallowRef<Y.Doc | null>(null),
    localClientId: ref<number | null>(41),
    isDocSynced: ref(false),
  }
  const session = scope.run(() => {
    const session = useCanvasCollaborationSession({ createTransport: () => transport })
    session.bindBoard({ boardId, accessState, canWrite })
    return session
  })!
  return { scope, boardId, accessState, canWrite, transport, session }
}

describe('Canvas collaboration session', () => {
  it('waits for room synchronization and HTTP graph attachment before projecting the authoritative document', async () => {
    const harness = setup()
    const doc = new Y.Doc()
    publish(doc, snapshot(100))
    harness.transport.sharedDoc.value = doc
    await nextTick()
    const apply = vi.fn()
    harness.session.attachGraph(apply)
    expect(harness.session.canWrite.value).toBe(false)
    expect(harness.session.isReadOnly.value).toBe(true)
    expect(apply).not.toHaveBeenCalled()
    harness.transport.isDocSynced.value = true
    expect(apply).toHaveBeenLastCalledWith(snapshot(100))
    expect(harness.session.canWrite.value).toBe(true)

    harness.session.detachGraph()
    publish(doc, snapshot(200))
    publish(doc, snapshot(300))
    apply.mockClear()
    harness.session.attachGraph(apply)
    expect(apply).toHaveBeenCalledTimes(1)
    expect(apply).toHaveBeenCalledWith(snapshot(300))
    harness.scope.stop()
    doc.destroy()
  })

  it('reads current local changes when graph attachment or readiness resumes', async () => {
    const harness = setup()
    const doc = new Y.Doc()
    publish(doc, snapshot(10))
    harness.transport.sharedDoc.value = doc
    await nextTick()
    harness.transport.isDocSynced.value = true
    const apply = vi.fn()
    harness.session.attachGraph(apply)
    apply.mockClear()
    harness.session.syncSnapshot(snapshot(20), ['prompt-1'], [])
    expect(apply).not.toHaveBeenCalled()
    harness.session.detachGraph()
    harness.session.attachGraph(apply)
    expect(apply).toHaveBeenCalledExactlyOnceWith(snapshot(20))
    apply.mockClear()
    harness.session.syncSnapshot(snapshot(30), ['prompt-1'], [])
    harness.transport.isDocSynced.value = false
    harness.transport.isDocSynced.value = true
    expect(apply).toHaveBeenCalledExactlyOnceWith(snapshot(30))
    harness.scope.stop()
    doc.destroy()
  })

  it('projects an empty room when sync readiness arrives before its document attachment', async () => {
    const harness = setup()
    const apply = vi.fn()
    harness.session.attachGraph(apply)
    harness.transport.isDocSynced.value = true
    expect(apply).not.toHaveBeenCalled()
    const doc = new Y.Doc()
    harness.transport.sharedDoc.value = doc
    await nextTick()
    expect(apply).toHaveBeenCalledExactlyOnceWith(createEmptyBoardSnapshot())
    harness.scope.stop()
    doc.destroy()
  })

  it('writes changed ids without echo and rejects malformed remote records at the canonical Board seam', async () => {
    const harness = setup()
    const doc = new Y.Doc()
    harness.transport.sharedDoc.value = doc
    await nextTick()
    harness.transport.isDocSynced.value = true
    const apply = vi.fn()
    harness.session.attachGraph(apply)
    expect(apply).toHaveBeenCalledWith(createEmptyBoardSnapshot())
    apply.mockClear()
    harness.session.syncSnapshot(snapshot(40), ['prompt-1'], [])
    expect(doc.getMap('nodes').get('prompt-1')).toMatchObject({ position: { x: 40 } })
    expect(apply).not.toHaveBeenCalled()
    doc.getMap('nodes').set('prompt-1', {
      id: 'prompt-1',
      type: 'prompt',
      data: { id: 1 },
      position: { x: 99, y: 0 },
    })
    expect(apply).not.toHaveBeenCalled()
    harness.transport.isDocSynced.value = false
    harness.transport.isDocSynced.value = true
    expect(apply).not.toHaveBeenCalled()
    publish(doc, snapshot(80))
    expect(apply).toHaveBeenCalledWith(snapshot(80))
    harness.scope.stop()
    apply.mockClear()
    publish(doc, snapshot(90))
    expect(apply).not.toHaveBeenCalled()
    doc.destroy()
  })

  it('drops pending local and remote state when switching Boards and waits for the new room', async () => {
    const harness = setup()
    const docA = new Y.Doc()
    harness.transport.sharedDoc.value = docA
    await nextTick()
    harness.session.syncSnapshot(snapshot(20), ['prompt-1'], [])
    publish(docA, snapshot(30))
    harness.boardId.value = 'board-b'
    harness.transport.isDocSynced.value = true
    const applyB = vi.fn()
    harness.session.attachGraph(applyB)
    expect(applyB).not.toHaveBeenCalled()
    expect(docA.getMap('nodes').get('prompt-1')).toMatchObject({ position: { x: 30 } })
    harness.transport.isDocSynced.value = false
    const docB = new Y.Doc()
    publish(docB, snapshot(50))
    harness.transport.sharedDoc.value = docB
    await nextTick()
    harness.transport.isDocSynced.value = true
    expect(applyB).toHaveBeenCalledExactlyOnceWith(snapshot(50))
    harness.scope.stop()
    docA.destroy()
    docB.destroy()
  })

  it('revokes queued writes when edit capability disappears and leaves private Boards on snapshot persistence', async () => {
    const harness = setup()
    const doc = new Y.Doc()
    harness.transport.sharedDoc.value = doc
    await nextTick()
    harness.session.syncSnapshot(snapshot(20), ['prompt-1'], [])
    harness.canWrite.value = false
    harness.transport.isDocSynced.value = true
    harness.session.syncSnapshot(snapshot(30), ['prompt-1'], [])
    expect(doc.getMap('nodes').size).toBe(0)
    expect(harness.session.isReadOnly.value).toBe(true)
    harness.accessState.value = 'private'
    expect(harness.session.isEnabled.value).toBe(false)
    expect(harness.session.isReadOnly.value).toBe(false)
    harness.scope.stop()
    doc.destroy()
  })

  test('blocks edits until synchronization, then resumes after reconnect', async () => {
    const harness = setup()
    expect(harness.session.canWrite.value).toBe(false)
    expect(harness.session.isReadOnly.value).toBe(true)
    const doc = new Y.Doc()
    harness.transport.sharedDoc.value = doc
    await nextTick()
    harness.session.syncSnapshot(snapshot(20), ['prompt-1'], [])
    harness.transport.isDocSynced.value = true
    expect(doc.getMap('nodes').size).toBe(0)
    expect(harness.session.canWrite.value).toBe(true)
    expect(harness.session.isReadOnly.value).toBe(false)
    harness.session.syncSnapshot(snapshot(30), ['prompt-1'], [])
    expect(doc.getMap('nodes').get('prompt-1')).toMatchObject({ position: { x: 30 } })
    harness.transport.isDocSynced.value = false
    expect(harness.session.canWrite.value).toBe(false)
    expect(harness.session.isReadOnly.value).toBe(true)
    harness.session.syncSnapshot(snapshot(40), ['prompt-1'], [])
    harness.transport.isDocSynced.value = true
    expect(harness.session.canWrite.value).toBe(true)
    expect(doc.getMap('nodes').get('prompt-1')).toMatchObject({ position: { x: 30 } })
    harness.scope.stop()
    doc.destroy()
  })

  it('keeps a workspace viewer read-only with or without room sync', () => {
    const harness = setup()
    harness.canWrite.value = false
    expect(harness.session.canWrite.value).toBe(false)
    expect(harness.session.isReadOnly.value).toBe(true)
    expect(!harness.session.isEnabled.value || harness.session.canWrite.value).toBe(false)
    harness.transport.isDocSynced.value = true
    expect(harness.session.canWrite.value).toBe(false)
    expect(harness.session.isReadOnly.value).toBe(true)
    expect(!harness.session.isEnabled.value || harness.session.canWrite.value).toBe(false)
    harness.scope.stop()
  })
})
