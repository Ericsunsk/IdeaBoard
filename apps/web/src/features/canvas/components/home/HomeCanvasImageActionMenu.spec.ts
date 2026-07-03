import { createApp, defineComponent, h } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { IMAGE_NODE_ACTION } from '../../../../shared/domain/imageActions'
import type { ImageStrengthAction } from '../../execution'
import HomeCanvasImageActionMenu from './HomeCanvasImageActionMenu.vue'

function mountImageActionMenu(
  input: {
    activeStrengthAction?: ImageStrengthAction | null
    showStrengthControl?: boolean
    strengthValue?: number
  } = {},
) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const props = {
    showStrengthControl: input.showStrengthControl ?? false,
    directGenerating: false,
    activeStrengthAction: input.activeStrengthAction ?? null,
    getStrengthValue: () => input.strengthValue ?? 50,
    setStrengthRangeRef: vi.fn(),
    setStrengthTriggerRef: vi.fn(),
    onStrengthRangePointerDown: vi.fn(),
    onStrengthInput: vi.fn(),
    onStrengthChange: vi.fn(),
    onStrengthPointerUp: vi.fn(),
    onStrengthClick: vi.fn(),
  }
  const app = createApp(
    defineComponent({
      setup() {
        return () => h(HomeCanvasImageActionMenu, props)
      },
    }),
  )
  app.mount(host)

  return {
    host,
    props,
    unmount: () => {
      app.unmount()
      host.remove()
    },
  }
}

describe('HomeCanvasImageActionMenu', () => {
  it('keeps inactive strength ranges outside keyboard navigation', () => {
    const harness = mountImageActionMenu()

    try {
      const strengthRanges = Array.from(
        harness.host.querySelectorAll<HTMLInputElement>('.image-action-strength-range'),
      )

      expect(strengthRanges).toHaveLength(2)
      expect(strengthRanges.map((range) => range.getAttribute('aria-label'))).toEqual([
        '风格延展强度',
        'HD强度',
      ])
      expect(strengthRanges.map((range) => range.getAttribute('tabindex'))).toEqual(['-1', '-1'])
    } finally {
      harness.unmount()
    }
  })

  it('binds long press triggers to the visible strength buttons', () => {
    const harness = mountImageActionMenu()

    try {
      expect(harness.props.setStrengthTriggerRef).toHaveBeenCalledWith(
        IMAGE_NODE_ACTION.styleVariation,
        expect.anything(),
      )
      expect(harness.props.setStrengthTriggerRef).toHaveBeenCalledWith(
        IMAGE_NODE_ACTION.hdUpscale,
        expect.anything(),
      )
    } finally {
      harness.unmount()
    }
  })

  it('shows the active strength range for style variation and HD actions', () => {
    const styleHarness = mountImageActionMenu({
      showStrengthControl: true,
      activeStrengthAction: IMAGE_NODE_ACTION.styleVariation,
    })
    const hdHarness = mountImageActionMenu({
      showStrengthControl: true,
      activeStrengthAction: IMAGE_NODE_ACTION.hdUpscale,
    })

    try {
      const activeStyleRange = styleHarness.host.querySelector<HTMLInputElement>(
        'input[aria-label="风格延展强度"]',
      )
      const hiddenHdRange = styleHarness.host.querySelector<HTMLInputElement>(
        'input[aria-label="HD强度"]',
      )
      const activeHdRange = hdHarness.host.querySelector<HTMLInputElement>(
        'input[aria-label="HD强度"]',
      )

      expect(
        activeStyleRange
          ?.closest('.image-action-control')
          ?.classList.contains('is-strength-active'),
      ).toBe(true)
      expect(
        hiddenHdRange?.closest('.image-action-control')?.classList.contains('is-strength-hidden'),
      ).toBe(true)
      expect(
        activeHdRange?.closest('.image-action-control')?.classList.contains('is-strength-active'),
      ).toBe(true)
    } finally {
      styleHarness.unmount()
      hdHarness.unmount()
    }
  })
})
