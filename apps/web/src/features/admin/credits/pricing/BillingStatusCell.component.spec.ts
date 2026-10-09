import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../../test/render'
import BillingStatusCell from './BillingStatusCell.vue'
import type { AdminBillableItemStatus } from './billableCatalogTypes'

const cases: Array<{ status: AdminBillableItemStatus; label: string }> = [
  { status: 'unpriced', label: 'Unpriced' },
  { status: 'active', label: 'Active' },
  { status: 'disabled', label: 'Disabled' },
  { status: 'archived', label: 'Archived' },
]

describe('BillingStatusCell', () => {
  it.each(cases)(
    'locks $label ($status) to the fixed 84px slot with ping disabled',
    ({ status, label }) => {
      const { container } = renderWithUser(BillingStatusCell, {
        props: { status },
      })
      const text = screen.getByText(label)
      expect(text).toBeVisible()
      // 短文案 Active(6) 与长文案 Disabled(8) 同槽：槽与胶囊同锁 84px 并居中，不收缩。
      expect(text.parentElement).toHaveClass('justify-center')
      expect(text.parentElement?.style.minWidth).toBe('84px')
      // success↔neutral 翻转不再挂载/卸载 animate-ping，避免显隐推挤。
      expect(container.querySelector('.animate-ping')).toBeNull()
      expect(screen.getByTestId('switch-row-status-slot').style.minWidth).toBe('84px')
    },
  )

  it('keeps Active and Disabled slots at the same width', async () => {
    const { rerender } = renderWithUser(BillingStatusCell, { props: { status: 'active' } })
    const activeWidth = screen.getByTestId('switch-row-status-slot').style.minWidth

    await rerender({ status: 'disabled' })
    expect(screen.getByTestId('switch-row-status-slot').style.minWidth).toBe(activeWidth)
    expect(screen.getByText('Disabled').parentElement?.style.minWidth).toBe('84px')
  })
})
