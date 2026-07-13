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
      totalCredits: 4,
      availableCredits: 20,
      priceVersionId: 1,
    })

    expect(screen.getByText('4 Ideas')).toBeVisible()
    expect(screen.getByRole('button', { name: '发送' })).toBeEnabled()
  })

  it('shows the Chinese reason and disables submit when billing is blocked', () => {
    renderControls({
      feature: 'pattern-gen',
      status: 'blocked',
      totalCredits: 8,
      availableCredits: 3,
      reason: 'Idea不足（需要 8 Ideas，当前 3 Ideas）',
    })

    expect(screen.getByText('Idea不足（需要 8 Ideas，当前 3 Ideas）')).toBeVisible()
    expect(screen.getByRole('button', { name: '发送' })).toBeDisabled()
  })

  it('keeps local composers price-free when no billing state is provided', () => {
    renderControls(null)

    expect(screen.queryByText(/Idea/)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '发送' })).toBeEnabled()
  })
})
