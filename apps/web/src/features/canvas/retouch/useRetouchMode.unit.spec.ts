import type { Node } from '@vue-flow/core'
import { createApp, defineComponent, h, nextTick, ref, shallowRef } from 'vue'
import type { Ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useRetouchMode } from './useRetouchMode'

function mountRetouchMode(
  options: { nodeKind?: string; onBeforePointerCancel?: () => void } = {},
) {
  const activeTool = ref('select')
  const nodes: Ref<Node[]> = shallowRef([
    { id: 'node-1', data: { kind: options.nodeKind ?? 'image' } },
  ] as Node[])
  const onBeforeExit = vi.fn()
  let editor!: ReturnType<typeof useRetouchMode>
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        editor = useRetouchMode({
          nodes,
          activeTool,
          getImageNodeDisplayPayload: (nodeId) =>
            nodeId === 'node-1' ? { url: 'https://example.com/source.png', ratio: 1 } : null,
          onBeforePointerCancel: options.onBeforePointerCancel,
          onBeforeExit,
        })
        return () => h('div')
      },
    }),
  )
  app.mount(host)

  return {
    activeTool,
    editor,
    nodes,
    onBeforeExit,
    unmount: () => {
      app.unmount()
      host.remove()
    },
  }
}

describe('useRetouchMode', () => {
  it('enters adjust mode for image nodes and resolves the contained image rect', () => {
    const harness = mountRetouchMode()

    try {
      expect(harness.editor.enterRetouchMode('node-1', 'adjust')).toBe(true)

      expect(harness.editor.retouchNodeId.value).toBe('node-1')
      expect(harness.editor.retouchMode.value).toBe('adjust')
      expect(harness.editor.retouchState.value).toEqual({ nodeId: 'node-1', mode: 'adjust' })
    } finally {
      harness.unmount()
    }
  })

  it('rejects non-image nodes before entering editor mode', () => {
    const harness = mountRetouchMode({ nodeKind: 'prompt' })

    try {
      expect(harness.editor.enterRetouchMode('node-1', 'adjust')).toBe(false)
      expect(harness.editor.retouchState.value).toBeNull()
    } finally {
      harness.unmount()
    }
  })

  it('exits when the active canvas tool changes away from select', async () => {
    const harness = mountRetouchMode()

    try {
      expect(harness.editor.enterRetouchMode('node-1', 'inpaint')).toBe(true)

      harness.activeTool.value = 'hand'
      await nextTick()

      expect(harness.editor.retouchState.value).toBeNull()
      expect(harness.onBeforeExit).toHaveBeenCalledTimes(1)
    } finally {
      harness.unmount()
    }
  })

  it('removes global listeners after unmount', async () => {
    const onBeforePointerCancel = vi.fn()
    const harness = mountRetouchMode({ onBeforePointerCancel })

    expect(harness.editor.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    await nextTick()
    harness.unmount()

    window.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0 }))

    expect(onBeforePointerCancel).not.toHaveBeenCalled()
  })
})
