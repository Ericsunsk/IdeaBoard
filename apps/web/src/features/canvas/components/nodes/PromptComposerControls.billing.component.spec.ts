import { describe, expect, it } from 'vitest'
import { render, screen } from '../../../../test/render'
import type { CanvasBillingQuoteState } from '../../execution'
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
      billingState,
    },
  })
}

describe('PromptComposerControls billing contract', () => {
  it('shows the quoted total and keeps submit enabled when points are sufficient', () => {
    renderControls({
      feature: 'pattern-gen',
      status: 'ready',
      unitCredits: 4,
      availableCredits: 20,
      priceVersionId: 1,
    })

    expect(screen.getByRole('button', { name: '发送' })).toHaveAttribute('title', '发送 · 4 Ideas')
    expect(screen.getByRole('button', { name: '发送' })).toBeEnabled()
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
          feature: 'pattern-gen',
          status: 'ready',
          unitCredits: 3,
          availableCredits: 20,
          sufficient: true,
        },
      },
    })

    expect(screen.getByRole('button', { name: '发送' })).toHaveAttribute('title', '发送 · 12 Ideas')
    expect(screen.getByText('12 Ideas')).toHaveAttribute('data-touch-quote')

    await paid.rerender({
      modelValue: '生成海报',
      outputCount: 4,
      placeholder: '输入提示词',
      textareaClass: 'prompt-input',
      actionsClass: 'prompt-actions',
      sendButtonClass: 'prompt-send',
      billingState: {
        feature: 'pattern-gen',
        status: 'ready',
        unitCredits: 0,
        availableCredits: 0,
        sufficient: true,
      },
    })

    expect(screen.getByRole('button', { name: '发送' })).toHaveAttribute('title', '发送 · 免费')
    expect(screen.getByText('免费')).toHaveAttribute('data-touch-quote')
  })

  it('keeps submit actionable when a cached quote reports insufficient Idea余额', () => {
    renderControls({
      feature: 'pattern-gen',
      status: 'ready',
      unitCredits: 8,
      availableCredits: 3,
      sufficient: false,
      reason: 'Idea不足（需要 8 Ideas，当前 3 Ideas）',
    })

    expect(screen.getByRole('button', { name: '发送' })).toBeEnabled()
  })

  it('keeps local composers price-free when no billing state is provided', () => {
    renderControls(null)

    expect(screen.queryByText(/Idea/)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '发送' })).toBeEnabled()
  })
})
