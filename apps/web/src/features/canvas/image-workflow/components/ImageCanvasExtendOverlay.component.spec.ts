import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '../../../../test/render'
import ImageCanvasExtendOverlay from './ImageCanvasExtendOverlay.vue'

function renderOverlay() {
  return render(ImageCanvasExtendOverlay, {
    props: {
      overlayStyle: {
        left: '0px',
        top: '0px',
        width: '320px',
        height: '240px',
      },
      handleRadius: 4,
    },
  })
}

function dispatchBubblingMouseEvent(target: Element, type: 'mousedown' | 'mouseup' | 'click') {
  target.dispatchEvent(new MouseEvent(type, { bubbles: true, cancelable: true, button: 0 }))
}

describe('ImageCanvasExtendOverlay', () => {
  it('hides output resolution from the expand composer', () => {
    renderOverlay()

    expect(screen.queryByRole('button', { name: '输出清晰度' })).not.toBeInTheDocument()
  })

  it('keeps expand drag mouse events inside the overlay', () => {
    const view = renderOverlay()
    const parentMouseDown = vi.fn()
    const parentMouseUp = vi.fn()
    const parentClick = vi.fn()
    view.container.addEventListener('mousedown', parentMouseDown)
    view.container.addEventListener('mouseup', parentMouseUp)
    view.container.addEventListener('click', parentClick)

    const frame = view.container.querySelector('[data-outpaint-frame]')
    const handle = view.container.querySelector('.image-node-expand-handle')

    expect(frame).toBeInstanceOf(HTMLElement)
    expect(handle).toBeInstanceOf(HTMLElement)

    for (const target of [frame as HTMLElement, handle as HTMLElement]) {
      dispatchBubblingMouseEvent(target, 'mousedown')
      dispatchBubblingMouseEvent(target, 'mouseup')
      dispatchBubblingMouseEvent(target, 'click')
    }

    expect(parentMouseDown).not.toHaveBeenCalled()
    expect(parentMouseUp).not.toHaveBeenCalled()
    expect(parentClick).not.toHaveBeenCalled()
  })
})
