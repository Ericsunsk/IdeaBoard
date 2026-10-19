import type { BoardSnapshot } from '../../../../board/ports/canvas'
// Live doc-binding bridge specs over real Y.Doc instances (no sockets).

import { describe, expect, test } from 'vitest'
import { ref } from 'vue'
import * as Y from 'yjs'
import { useDocBinding } from './useDocBinding'
import { DOC_EDGES_MAP, DOC_NODES_MAP } from './mapping'

function nextFrame(): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, 0)
  })
}

describe('doc binding live bridge', () => {
  test('buffers committed changes through sync and clears them on board switches', async () => {
    const doc = ref<Y.Doc | null>(null)
    const isReady = ref(false)
    const boardId = ref('board-1')
    const bridge = useDocBinding({ boardId, isReady, doc, deviceId: ref(41), canWrite: ref(true) })
    const snapshot: BoardSnapshot = {
      schemaVersion: 4 as const,
      imageNodes: [
        {
          id: 1,
          type: 'image-node',
          x: 50,
          y: 60,
          width: 280,
          sourceImageUrl: '',
          sourceImageRatio: 1,
          outputStatus: 'idle',
          outputImages: [],
        },
      ],
      promptNodes: [],
      fusionNodes: [],
      groups: [],
      connections: [],
    }
    bridge.syncSnapshot(snapshot, ['image-1'], [])
    const live = new Y.Doc()
    doc.value = live
    await nextFrame()
    expect(live.getMap('nodes').size).toBe(0)
    isReady.value = true
    expect(live.getMap('nodes').get('image-1')).toMatchObject({ position: { x: 50, y: 60 } })
    isReady.value = false
    bridge.syncSnapshot({ ...snapshot, imageNodes: [] }, ['image-1'], [])
    boardId.value = 'board-2'
    isReady.value = true
    expect(live.getMap('nodes').has('image-1')).toBe(true)
    live.destroy()
  })

  test('attaches when doc and device id land, observes remote snapshots', async () => {
    const doc = ref<Y.Doc | null>(null)
    const deviceId = ref<number | null>(null)
    const canWrite = ref(true)
    const bridge = useDocBinding({
      boardId: ref('board-1'),
      isReady: ref(true),
      doc,
      deviceId,
      canWrite,
    })
    expect(bridge.remoteSnapshot.value).toBeNull()

    const live = new Y.Doc()
    doc.value = live
    deviceId.value = 41
    await nextFrame()

    live.transact(() => {
      live.getMap(DOC_NODES_MAP).set('image-1', {
        id: 'image-1',
        type: 'image',
        position: { x: 5, y: 6 },
        data: {
          id: 1,
          type: 'image-node',
          x: 5,
          y: 6,
          width: 280,
          sourceImageUrl: '',
          sourceImageRatio: 1,
          outputStatus: 'idle',
          outputImages: [],
        },
      })
      live.getMap(DOC_EDGES_MAP).set('connection-9', {
        id: 'connection-9',
        source: 'image-1',
        target: 'image-1',
        data: {
          id: 9,
          fromNodeId: 1,
          fromNodeType: 'image',
          fromPort: 'image',
          toNodeId: 1,
          toNodeType: 'image',
          toPort: 'image',
        },
      })
    }, 'remote-origin')
    await nextFrame()
    try {
      expect(bridge.remoteSnapshot.value?.imageNodes.map((node) => node.id)).toEqual([1])
      expect(bridge.remoteSnapshot.value?.connections.map((edge) => edge.id)).toEqual([9])
    } finally {
      live.destroy()
    }
  })
})
