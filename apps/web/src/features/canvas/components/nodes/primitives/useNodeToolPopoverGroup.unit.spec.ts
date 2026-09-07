import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import { useNodeToolPopoverGroup } from './useNodeToolPopoverGroup'

function mountPopoverGroup(options: { closeOnEscape?: boolean | undefined } = {}) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const rootRef = ref<HTMLElement | null>(null)
  let popovers: ReturnType<typeof useNodeToolPopoverGroup<'aspect-ratio' | 'output-count'>>

  const app = createApp(
    defineComponent({
      setup() {
        popovers = useNodeToolPopoverGroup({
          keys: ['aspect-ratio', 'output-count'] as const,
          rootRefs: [rootRef],
          closeOnEscape: options.closeOnEscape,
        })
        return () => h('div', { ref: rootRef }, 'actions')
      },
    }),
  )
  app.mount(host)

  return {
    host,
    rootRef,
    get popovers() {
      return popovers
    },
    async unmount() {
      app.unmount()
      host.remove()
      await nextTick()
    },
  }
}

function dispatchPointerDown(target: EventTarget = document.body) {
  target.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0 }))
}

function dispatchEscape() {
  document.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, key: 'Escape' }))
}

describe('useNodeToolPopoverGroup', () => {
  it('keeps grouped roots open and dismisses outside pointerdown', async () => {
    const harness = mountPopoverGroup()
    await nextTick()

    try {
      harness.popovers.open('aspect-ratio')
      await nextTick()

      dispatchPointerDown(harness.rootRef.value ?? undefined)
      expect(harness.popovers.activeKey.value).toBe('aspect-ratio')

      dispatchPointerDown()
      expect(harness.popovers.activeKey.value).toBeNull()
    } finally {
      await harness.unmount()
    }
  })

  it('respects the Escape close option', async () => {
    const harness = mountPopoverGroup({ closeOnEscape: false })
    await nextTick()

    try {
      harness.popovers.open('output-count')
      await nextTick()
      dispatchEscape()

      expect(harness.popovers.activeKey.value).toBe('output-count')
    } finally {
      await harness.unmount()
    }
  })

  it('dismisses only the topmost popover group', async () => {
    const lower = mountPopoverGroup()
    const upper = mountPopoverGroup()
    await nextTick()

    try {
      lower.popovers.open('aspect-ratio')
      await nextTick()
      upper.popovers.open('output-count')
      await nextTick()

      dispatchPointerDown()

      expect(upper.popovers.activeKey.value).toBeNull()
      expect(lower.popovers.activeKey.value).toBe('aspect-ratio')

      dispatchEscape()
      expect(lower.popovers.activeKey.value).toBeNull()
    } finally {
      await upper.unmount()
      await lower.unmount()
    }
  })

  it('ignores unknown keys', async () => {
    const harness = mountPopoverGroup()
    await nextTick()

    try {
      harness.popovers.open('unknown' as 'aspect-ratio')

      expect(harness.popovers.activeKey.value).toBeNull()
    } finally {
      await harness.unmount()
    }
  })
})
