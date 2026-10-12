import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../../../test/render'
import PromptComposerControls from './PromptComposerControls.vue'

function renderControls(isDisabled = false) {
  return renderWithUser(PromptComposerControls, {
    props: {
      modelValue: '生成一张海报',
      outputCount: 1,
      outputResolution: '1k',
      placeholder: '输入提示词',
      textareaClass: 'prompt-input',
      actionsClass: 'prompt-actions',
      sendButtonClass: 'prompt-send',
      isDisabled,
      shouldShowAspectRatio: false,
      shouldShowSubmit: false,
    },
  })
}

describe('PromptComposerControls output options', () => {
  it('preserves count selection and existing style hooks', async () => {
    const view = renderControls()
    const trigger = screen.getByRole('button', { name: 'Output count' })

    expect(trigger).toHaveClass('image-node-output-count', 'node-tool-output-count')
    await view.user.click(trigger)

    const option = screen.getByRole('button', { name: '4' })
    expect(option).toHaveClass('image-node-output-count-option', 'node-tool-output-count-option')
    expect(screen.queryByRole('menuitemradio', { name: '4' })).not.toBeInTheDocument()

    await view.user.click(option)
    expect(view.emitted()['update:output-count']).toEqual([[4]])
  })

  it('formats resolution options as radio menu items', async () => {
    const view = renderControls()

    await view.user.click(screen.getByRole('button', { name: 'Output resolution' }))

    expect(screen.getByRole('menuitemradio', { name: '1K' })).toHaveAttribute(
      'aria-checked',
      'true',
    )
    const option = screen.getByRole('menuitemradio', { name: '2K' })
    expect(option).toHaveAttribute('aria-checked', 'false')

    await view.user.click(option)
    expect(view.emitted()['update:output-resolution']).toEqual([['2k']])
  })

  it('disables output triggers with the composer', () => {
    renderControls(true)
    expect(screen.getByRole('button', { name: 'Output count' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Output resolution' })).toBeDisabled()
  })
})
