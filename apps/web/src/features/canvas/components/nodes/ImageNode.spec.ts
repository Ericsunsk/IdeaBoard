import { createApp, h, nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import { IMAGE_NODE_ACTION } from '../../config/imageActionRegistry'
import type { ImageActionBinding } from '../../image-tools'
import ImageNode from './ImageNode.vue'

const mountedHosts: HTMLElement[] = []
const imageDataUrl =
  'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII='

function createImageActionBinding(
  overrides: Partial<ImageActionBinding> = {},
): ImageActionBinding {
  return {
    actionMenuVisible: true,
    promptComposerVisible: false,
    promptPlaceholder: '',
    directGenerating: false,
    showStrengthControl: false,
    activeStrengthAction: null,
    getStrengthValue: () => 50,
    setStrengthRangeRef: vi.fn(),
    onStrengthRangePointerDown: vi.fn(),
    onStrengthInput: vi.fn(),
    onStrengthChange: vi.fn(),
    onStrengthPointerUp: vi.fn(),
    onStrengthClick: vi.fn(),
    ...overrides,
  }
}

function createReadyImageNode(overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: imageDataUrl,
    sourceImageRatio: 1,
    outputStatus: 'ready',
    outputImages: [{ url: imageDataUrl, ratio: 1 }],
    ...overrides,
  }
}

function mountImageNode(options: { actionBinding?: ImageActionBinding } = {}) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  mountedHosts.push(host)
  const onAction = vi.fn()

  const app = createApp({
    render: () =>
      h(ImageNode, {
        node: createReadyImageNode(),
        flowNodeId: 'flow-image-1',
        active: true,
        dragging: false,
        copyMode: false,
        stageHeight: 240,
        displayImage: { url: imageDataUrl },
        actionBinding: options.actionBinding ?? createImageActionBinding(),
        onAction,
      }),
  })

  app.mount(host)

  return {
    host,
    onAction,
    unmount: () => {
      app.unmount()
      host.remove()
    },
  }
}

afterEach(() => {
  for (const host of mountedHosts.splice(0)) {
    host.remove()
  }
})

describe('ImageNode', () => {
  it('shows image action tools when a displayed image has an action menu binding', async () => {
    const harness = mountImageNode()

    try {
      await nextTick()

      expect(harness.host.querySelector('.image-action-menu')).not.toBeNull()
      expect(harness.host.querySelector('[aria-label="HD"]')).not.toBeNull()
    } finally {
      harness.unmount()
    }
  })

  it('keeps strength action buttons interactive with tooltips in the default action menu', async () => {
    const harness = mountImageNode()

    try {
      await nextTick()

      for (const label of ['风格延展', 'HD']) {
        const button = harness.host.querySelector<HTMLButtonElement>(`[aria-label="${label}"]`)
        const control = button?.closest('.image-action-control')
        const strengthRange = control?.querySelector<HTMLInputElement>(
          '.image-action-strength-range',
        )

        expect(button?.classList.contains('node-tool-tooltip-trigger')).toBe(true)
        expect(button?.style.pointerEvents).not.toBe('none')
        expect(strengthRange?.tabIndex).toBe(-1)
        expect(strengthRange?.getAttribute('aria-hidden')).toBe('true')
      }
    } finally {
      harness.unmount()
    }
  })

  it('runs strength image actions from the default action buttons', async () => {
    const harness = mountImageNode()

    try {
      await nextTick()

      harness.host.querySelector<HTMLButtonElement>('[aria-label="风格延展"]')?.click()
      harness.host.querySelector<HTMLButtonElement>('[aria-label="HD"]')?.click()

      expect(harness.onAction).toHaveBeenCalledWith(IMAGE_NODE_ACTION.styleVariation)
      expect(harness.onAction).toHaveBeenCalledWith(IMAGE_NODE_ACTION.hdUpscale)
    } finally {
      harness.unmount()
    }
  })
})
