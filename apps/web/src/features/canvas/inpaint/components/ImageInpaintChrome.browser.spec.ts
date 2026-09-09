import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import ImageInpaintChrome from './ImageInpaintChrome.vue'

const ChromeFrameStub = { template: '<div><slot name="tools" /><slot name="dock" /></div>' }

const IconButtonStub = {
  template:
    '<button type="button" :aria-label="$attrs[\'aria-label\']" @click="$emit(\'click\')"><slot /></button>',
}

const PromptComposerStub = {
  template:
    '<button type="button" aria-label="Submit inpaint prompt" @click="$emit(\'submit\', \'paint the marked area\')">Submit</button>',
}

describe('ImageInpaintChrome browser smoke', () => {
  it('changes brush size and submits an inpaint prompt', async () => {
    const setBrushSize = vi.fn()
    const submit = vi.fn()
    const { user } = renderWithUser(ImageInpaintChrome, {
      props: {
        drawMode: 'paint',
        brushSize: 24,
        maskLayerOpacity: 72,
        anchorRect: { top: 0, left: 0, width: 320, height: 240 },
        'onSet-brush-size': setBrushSize,
        onSubmit: submit,
      },
      global: {
        stubs: {
          ChromeFrame: ChromeFrameStub,
          NodeToolIconButton: IconButtonStub,
          PromptTextComposer: PromptComposerStub,
        },
      },
    })

    const slider = screen.getByRole('slider', { name: /Brush size/ })
    ;(slider as HTMLInputElement).value = '48'
    slider.dispatchEvent(new Event('input', { bubbles: true }))
    await waitFor(() => expect(setBrushSize).toHaveBeenCalledWith(48))

    await user.click(screen.getByRole('button', { name: 'Submit inpaint prompt' }))
    expect(submit).toHaveBeenCalledWith('paint the marked area')
  })
})
