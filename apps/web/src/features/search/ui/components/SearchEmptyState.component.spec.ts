import { describe, expect, it } from 'vitest'
import { render, screen } from '../../../../test/render'
import userEvent from '@testing-library/user-event'
import SearchEmptyState from './SearchEmptyState.vue'

describe('SearchEmptyState', () => {
  it('renders the neutral empty feed without an inline error panel', () => {
    render(SearchEmptyState, {
      props: {
        resultsCount: 0,
        hasActiveSearch: false,
        canAddReference: true,
      },
    })
    // 初载失败只走 toast：空态保持中性，无行内错误面板与重试按钮。
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /重试|Retry/i })).not.toBeInTheDocument()
  })

  it('renders empty feed state and emits actions', async () => {
    const user = userEvent.setup()
    let wasClearCalled = false
    render(SearchEmptyState, {
      props: {
        resultsCount: 0,
        hasActiveSearch: true,
        canAddReference: true,
        onClearSearch: () => {
          wasClearCalled = true
        },
      },
    })
    const clearBtn = screen.getByRole('button', { name: /清空搜索|Clear/i })
    expect(clearBtn).toBeInTheDocument()
    await user.click(clearBtn)
    expect(wasClearCalled).toBe(true)
  })
})
