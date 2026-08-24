import { describe, expect, it } from 'vitest'
import { render } from '../../../../test/render'
import ImageToolChromeFrame from './ImageToolChromeFrame.vue'

const anchorRect = { top: 10, left: 20, width: 300, height: 200 }

describe('ImageToolChromeFrame', () => {
  it('does not render an empty tool rail when the tools slot is absent', () => {
    const view = render(ImageToolChromeFrame, {
      props: { anchorRect, mode: 'adjust' },
      slots: { dock: '<div>Dock</div>' },
    })

    expect(view.container.querySelector('.image-tool-chrome-frame__tool-rail')).toBeNull()
    expect(view.getByText('Dock')).toBeInTheDocument()
  })

  it('renders the tool rail when a tool surface is provided', () => {
    const view = render(ImageToolChromeFrame, {
      props: { anchorRect, mode: 'inpaint' },
      slots: {
        tools: '<button>Brush</button>',
        dock: '<div>Dock</div>',
      },
    })

    expect(view.container.querySelector('.image-tool-chrome-frame__tool-rail')).not.toBeNull()
    expect(view.getByRole('button', { name: 'Brush' })).toBeInTheDocument()
  })
})
