import { describe, expect, it, vi } from 'vitest'
import { render, renderWithUser, screen } from '../../../../../test/render'
import type { CanvasBillingQuoteState } from '../../../execution'
import PromptComposerControls from './PromptComposerControls.vue'

function renderControls(billingState?: CanvasBillingQuoteState | null) {
  return render(PromptComposerControls, {
    props: {
      modelValue: '生成一张海报',
      outputCount: 1,
      placeholder: '输入提示词',
      textareaClass: 'prompt-input',
      actionsClass: 'prompt-actions',
      sendButtonClass: 'prompt-send',
      billingState: billingState ?? null,
    },
  })
}

describe('PromptComposerControls billing contract', () => {
  it('shows the quoted total and keeps submit enabled when points are sufficient', () => {
    renderControls({
      feature: 'generation',
      status: 'ready',
      unitCredits: 4,
      availableCredits: 20,
      priceVersionId: 1,
    })

    expect(screen.getByRole('button', { name: 'Send' })).toHaveAttribute('title', 'Send · 4 Ideas')
    expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled()
  })

  it('shows the total quote for every requested output and preserves explicit free pricing', async () => {
    const paid = render(PromptComposerControls, {
      props: {
        modelValue: '生成海报',
        outputCount: 4,
        placeholder: '输入提示词',
        textareaClass: 'prompt-input',
        actionsClass: 'prompt-actions',
        sendButtonClass: 'prompt-send',
        billingState: {
          feature: 'generation',
          status: 'ready',
          unitCredits: 3,
          availableCredits: 20,
          sufficient: true,
        },
      },
    })

    expect(screen.getByRole('button', { name: 'Send' })).toHaveAttribute('title', 'Send · 12 Ideas')
    expect(screen.getByText('12 Ideas')).toHaveAttribute('data-touch-quote')

    await paid.rerender({
      modelValue: '生成海报',
      outputCount: 4,
      placeholder: '输入提示词',
      textareaClass: 'prompt-input',
      actionsClass: 'prompt-actions',
      sendButtonClass: 'prompt-send',
      billingState: {
        feature: 'generation',
        status: 'ready',
        unitCredits: 0,
        availableCredits: 0,
        sufficient: true,
      },
    })

    expect(screen.getByRole('button', { name: 'Send' })).toHaveAttribute('title', 'Send · Free')
    expect(screen.getByText('Free')).toHaveAttribute('data-touch-quote')
  })

  it('keeps submit actionable when a cached quote reports insufficient Idea余额', () => {
    renderControls({
      feature: 'generation',
      status: 'ready',
      unitCredits: 8,
      availableCredits: 3,
      sufficient: false,
      reason: 'Idea不足（需要 8 Ideas，当前 3 Ideas）',
    })

    expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled()
  })

  it('keeps local composers price-free when no billing state is provided', () => {
    renderControls(null)

    expect(screen.queryByText(/Idea/)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled()
  })

  it('can hide the independent submit control without affecting the draft input', async () => {
    const { user } = renderWithUser(PromptComposerControls, {
      props: {
        modelValue: '保留这段提示词',
        outputCount: 1,
        placeholder: '输入提示词',
        textareaClass: 'prompt-input',
        actionsClass: 'prompt-actions',
        sendButtonClass: 'prompt-send',
        showSubmit: false,
      },
    })

    const textarea = screen.getByRole('textbox')
    expect(screen.queryByRole('button', { name: 'Send' })).not.toBeInTheDocument()

    await user.click(textarea)
    await user.keyboard('{Enter}第二行')

    expect(textarea).toHaveValue('保留这段提示词\n第二行')
  })

  it('can hide output resolution without affecting the other composer actions', () => {
    render(PromptComposerControls, {
      props: {
        modelValue: '局部重绘',
        outputCount: 1,
        placeholder: '输入提示词',
        textareaClass: 'prompt-input',
        actionsClass: 'prompt-actions',
        sendButtonClass: 'prompt-send',
        showOutputResolution: false,
      },
    })

    expect(screen.queryByRole('button', { name: 'Output resolution' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled()
  })

  it('keeps Enter as a newline and submits only from the send button', async () => {
    const onSubmit = vi.fn()
    const { user } = renderWithUser(PromptComposerControls, {
      props: {
        modelValue: '第一行',
        outputCount: 1,
        placeholder: '输入提示词',
        textareaClass: 'prompt-input',
        actionsClass: 'prompt-actions',
        sendButtonClass: 'prompt-send',
        onSubmit,
      },
    })

    const textarea = screen.getByRole('textbox')
    await user.click(textarea)
    await user.keyboard('{Enter}第二行')

    expect(onSubmit).not.toHaveBeenCalled()
    expect(textarea).toHaveValue('第一行\n第二行')

    await user.click(screen.getByRole('button', { name: 'Send' }))
    expect(onSubmit).toHaveBeenCalledOnce()
  })
})
