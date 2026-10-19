// Doc-binding specs: canonical commit synchronization, origin and capability guards.

import { describe, expect, test } from 'vitest'
import { createDocBinding, type YDocPort, type YMapPort } from './binding'
import type { BoardSnapshot } from '../../../../board/ports/canvas'

type FakeMap = YMapPort & { store: Map<string, unknown>; emit: (origin: unknown) => void }

function buildFakeMap(): FakeMap {
  const store = new Map<string, unknown>()
  const listeners = new Set<(events: unknown[], transaction: { origin: unknown }) => void>()
  return {
    store,
    set: (key, value) => {
      store.set(key, value)
    },
    delete: (key) => {
      store.delete(key)
    },
    entries: () => [...store.entries()],
    observe: (listener) => {
      listeners.add(listener)
    },
    unobserve: (listener) => {
      listeners.delete(listener)
    },
    emit: (origin) => {
      for (const listener of [...listeners]) listener([], { origin })
    },
  }
}

function buildFakeDoc(): YDocPort & { maps: Record<string, FakeMap>; transactions: unknown[] } {
  const maps: Record<string, FakeMap> = { nodes: buildFakeMap(), edges: buildFakeMap() }
  const transactions: unknown[] = []
  return {
    maps,
    transactions,
    transact: (fn, origin) => {
      transactions.push(origin)
      fn()
    },
    getMap: (name) => {
      const map = maps[name]
      if (!map) throw new Error(`unknown map ${name}`)
      return map
    },
  }
}

function buildSnapshot(): BoardSnapshot {
  return {
    schemaVersion: 4,
    imageNodes: [
      {
        id: 1,
        type: 'image-node',
        x: 10,
        y: 20,
        width: 280,
        sourceImageUrl: '',
        sourceImageRatio: 1,
        outputStatus: 'idle',
        outputImages: [],
      },
    ],
    promptNodes: [],
    fusionNodes: [],
    connections: [],
    groups: [],
  }
}

describe('doc binding origin filtering', () => {
  test('syncs only committed ids and preserves remote records outside the commit', () => {
    const doc = buildFakeDoc()
    const binding = createDocBinding({ doc, deviceId: 41, canWrite: true })
    const remote = { id: 'prompt-99', type: 'prompt', position: { x: 0, y: 0 }, data: { id: 99 } }
    doc.maps.nodes!.set('prompt-99', remote)
    binding.syncSnapshot(buildSnapshot(), ['image-1'], [])
    expect(doc.maps.nodes!.store.get('image-1')).toMatchObject({
      id: 'image-1',
      position: { x: 10, y: 20 },
    })
    expect(doc.maps.nodes!.store.get('prompt-99')).toEqual(remote)
    binding.syncSnapshot({ ...buildSnapshot(), imageNodes: [] }, ['image-1'], [])
    expect(doc.maps.nodes!.store.get('image-1')).toBeUndefined()
    expect(doc.maps.nodes!.store.get('prompt-99')).toEqual(remote)
    binding.dispose()
  })

  test('tags every local mutation with the device origin', () => {
    const doc = buildFakeDoc()
    const binding = createDocBinding({ doc, deviceId: 41, canWrite: true })
    binding.syncSnapshot(buildSnapshot(), ['image-1'], [])
    binding.syncSnapshot({ ...buildSnapshot(), imageNodes: [] }, ['image-1'], [])
    expect(doc.transactions).toEqual([41, 41])
    binding.dispose()
  })

  test('skips echo transactions from the local device origin', () => {
    const doc = buildFakeDoc()
    const seen: unknown[] = []
    const binding = createDocBinding({
      doc,
      deviceId: 41,
      canWrite: true,
      onRemoteSnapshot: (snapshot) => {
        seen.push(snapshot)
      },
    })
    doc.maps.nodes?.emit(41)
    doc.maps.edges?.emit(41)
    expect(seen).toEqual([])
    binding.dispose()
  })

  test('forwards remote-origin transactions to the snapshot listener', () => {
    const doc = buildFakeDoc()
    const seen: BoardSnapshot[] = []
    const binding = createDocBinding({
      doc,
      deviceId: 41,
      canWrite: true,
      onRemoteSnapshot: (snapshot) => {
        seen.push(snapshot)
      },
    })
    doc.maps.nodes?.set('image-1', {
      id: 'image-1',
      type: 'image',
      position: { x: 1, y: 1 },
      data: buildSnapshot().imageNodes[0],
    })
    doc.maps.nodes?.emit('remote-origin')
    expect(seen).toHaveLength(1)
    expect(seen[0]?.imageNodes.map((node) => node.id)).toEqual([1])
    binding.dispose()
  })
})

describe('doc binding capability guard', () => {
  test('viewers cannot write canonical commits', () => {
    const doc = buildFakeDoc()
    const binding = createDocBinding({ doc, deviceId: 7, canWrite: false })
    binding.syncSnapshot(buildSnapshot(), ['image-1'], [])
    expect(doc.transactions).toEqual([])
    expect(doc.maps.nodes!.store.size).toBe(0)
    binding.dispose()
  })
})
