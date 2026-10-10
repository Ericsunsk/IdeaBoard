import { describe, expect, it } from 'vitest'
import { render, screen } from '../../../../../test/render'
import ImageInpaintChrome from './ImageInpaintChrome.vue'

function renderChrome() {
  return render(ImageInpaintChrome, {
    props: {
      drawMode: 'paint',
      brushSize: 24,
      maskLayerOpacity: 72,
      anchorRect: { top: 0, left: 0, width: 320, height: 240 },
    },
    global: {
      stubs: {
        ChromeFrame: { template: '<div><slot name="tools" /><slot name="dock" /></div>' },
        NodeToolIconButton: { template: '<button><slot /></button>' },
        PromptTextComposer: true,
      },
    },
  })
}

describe('ImageInpaintChrome', () => {
  it('gives brush size and mask opacity sliders a visible progress track', () => {
    renderChrome()

    for (const slider of [
      screen.getByRole('slider', { name: /^Brush size/ }),
      screen.getByRole('slider', { name: /^Mask/ }),
    ]) {
      const trackBackground = slider.parentElement?.style.getPropertyValue(
        '--canvas-range-track-background',
      )
      expect(trackBackground).toContain('linear-gradient')
      expect(trackBackground).toContain('var(--canvas-accent)')
      expect(trackBackground).toContain('var(--canvas-accent-outline)')
    }
  })
})
