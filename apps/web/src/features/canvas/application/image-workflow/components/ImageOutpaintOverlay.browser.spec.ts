import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../../../test/render'
import ImageOutpaintOverlay from './ImageOutpaintOverlay.vue'

const PromptComposerStub = {
  template:
    '<button type="button" aria-label="Submit outpaint prompt" @click="$emit(\'submit\', \'extend the scene\')">Submit</button>',
}

describe('ImageOutpaintOverlay browser smoke', () => {
  it('submits an outpaint prompt and exposes resize handles', async () => {
    const submit = vi.fn()
    const { user } = renderWithUser(ImageOutpaintOverlay, {
      props: {
        overlayStyle: { left: '0px', top: '0px', width: '320px', height: '240px' },
        handleRadius: 4,
        onSubmit: submit,
      },
      global: { stubs: { NodePromptComposer: PromptComposerStub } },
    })

    expect(document.querySelectorAll('.image-node-outpaint-handle')).toHaveLength(8)
    await user.click(screen.getByRole('button', { name: 'Submit outpaint prompt' }))
    expect(submit).toHaveBeenCalledWith('extend the scene')
  })

  it('keeps handle pointer drags inside the overlay', async () => {
    // Happy-dom covers mouse-event containment in the component spec; only a
    // real browser dispatches pointer events with capture semantics, so the
    // pointerdown/move/up stop-modifiers are pinned here.
    const parentPointerDown = vi.fn()
    const parentPointerMove = vi.fn()
    const parentPointerUp = vi.fn()
    const handlePointerDown = vi.fn()
    const { container } = renderWithUser(ImageOutpaintOverlay, {
      props: {
        overlayStyle: { left: '0px', top: '0px', width: '320px', height: '240px' },
        handleRadius: 4,
        'onHandle-pointerdown': handlePointerDown,
      },
      global: { stubs: { NodePromptComposer: PromptComposerStub } },
    })
    container.addEventListener('pointerdown', parentPointerDown)
    container.addEventListener('pointermove', parentPointerMove)
    container.addEventListener('pointerup', parentPointerUp)

    const frame = container.querySelector('[data-outpaint-frame]') as HTMLElement
    const handle = container.querySelector('.image-node-outpaint-handle.is-se') as HTMLElement
    expect(frame).toBeInstanceOf(HTMLElement)
    expect(handle).toBeInstanceOf(HTMLElement)

    handle.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, cancelable: true }))
    frame.dispatchEvent(new PointerEvent('pointermove', { bubbles: true, cancelable: true }))
    frame.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, cancelable: true }))

    expect(handlePointerDown).toHaveBeenCalledOnce()
    expect(parentPointerDown).not.toHaveBeenCalled()
    expect(parentPointerMove).not.toHaveBeenCalled()
    expect(parentPointerUp).not.toHaveBeenCalled()
  })
})
