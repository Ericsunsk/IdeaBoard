import type { Node } from '@vue-flow/core'
import { createApp, defineComponent, h, nextTick, ref, shallowRef } from 'vue'
import type { Ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useImageEditorMode } from './useImageEditorMode'

function mountImageEditorMode(
  options: { nodeKind?: string; onBeforePointerCancel?: () => void } = {},
) {
  const activeTool = ref('select')
  const nodes: Ref<Node[]> = shallowRef([
    { id: 'node-1', data: { kind: options.nodeKind ?? 'image' } },
  ] as Node[])
  const onBeforeExit = vi.fn()
  let editor!: ReturnType<typeof useImageEditorMode>
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        editor = useImageEditorMode({
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

describe('useImageEditorMode', () => {
  it('enters adjust mode for image nodes and resolves the contained image rect', () => {
    const harness = mountImageEditorMode()

    try {
      expect(harness.editor.enterImageEditorMode('node-1', 'adjust')).toBe(true)

      expect(harness.editor.imageEditorNodeId.value).toBe('node-1')
      expect(harness.editor.imageEditorMode.value).toBe('adjust')
      expect(harness.editor.imageEditorState.value).toEqual({ nodeId: 'node-1', mode: 'adjust' })
    } finally {
      harness.unmount()
    }
  })

  it('rejects non-image nodes before entering editor mode', () => {
    const harness = mountImageEditorMode({ nodeKind: 'prompt' })

    try {
      expect(harness.editor.enterImageEditorMode('node-1', 'adjust')).toBe(false)
      expect(harness.editor.imageEditorState.value).toBeNull()
    } finally {
      harness.unmount()
    }
  })

  it('exits when the active canvas tool changes away from select', async () => {
    const harness = mountImageEditorMode()

    try {
      expect(harness.editor.enterImageEditorMode('node-1', 'inpaint')).toBe(true)

      harness.activeTool.value = 'hand'
      await nextTick()

      expect(harness.editor.imageEditorState.value).toBeNull()
      expect(harness.onBeforeExit).toHaveBeenCalledTimes(1)
    } finally {
      harness.unmount()
    }
  })

  it('removes global listeners after unmount', async () => {
    const onBeforePointerCancel = vi.fn()
    const harness = mountImageEditorMode({ onBeforePointerCancel })

    expect(harness.editor.enterImageEditorMode('node-1', 'inpaint')).toBe(true)
    await nextTick()
    harness.unmount()

    window.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0 }))

    expect(onBeforePointerCancel).not.toHaveBeenCalled()
  })
})
