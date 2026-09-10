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
})
