import { describe, expect, test } from 'vitest'
import * as Y from 'yjs'
import { WebsocketProvider } from 'y-websocket'
import { toBareRealtimeUrl } from './provider'
import type { PresenceAwarenessState } from './state'

describe('presence upgrade url', () => {
  test('strips the provider-appended room suffix to bare /realtime', () => {
    expect(toBareRealtimeUrl('ws://localhost:6002/realtime/brd_abc123?ticket=tkt_1')).toBe(
      'ws://localhost:6002/realtime?ticket=tkt_1',
    )
  })

  test('keeps wss origins and drops empty room segments', () => {
    expect(toBareRealtimeUrl('wss://app.example/realtime/?ticket=tkt_2')).toBe(
      'wss://app.example/realtime?ticket=tkt_2',
    )
  })

  test('accepts URL instances', () => {
    expect(toBareRealtimeUrl(new URL('ws://localhost:6002/realtime/brd_x?ticket=t'))).toBe(
      'ws://localhost:6002/realtime?ticket=t',
    )
  })

  test('wires the stock provider to bare /realtime with an empty document', () => {
    const doc = new Y.Doc()
    const provider = new WebsocketProvider('ws://localhost:6002/realtime', 'brd_x', doc, {
      connect: false,
      params: { ticket: 'tkt_1' },
      disableBc: true,
      shouldReconnect: () => false,
    })
    try {
      expect(provider.url).toBe('ws://localhost:6002/realtime/brd_x?ticket=tkt_1')
      expect(toBareRealtimeUrl(provider.url)).toBe('ws://localhost:6002/realtime?ticket=tkt_1')
      const state: PresenceAwarenessState = {
        user: { id: 'usr_ada', name: 'Ada' },
        cursor: {
          position: { x: 1, y: 2 },
          dragging: false,
        },
        selection: { nodeIds: [], marquee: null },
      }
      provider.awareness.setLocalState(state)
      expect(Array.from(doc.share.keys())).toHaveLength(0)
    } finally {
      provider.destroy()
      doc.destroy()
    }
  })
})
