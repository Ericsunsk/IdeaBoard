import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../../test/render'
import BillingPricingTable from './BillingPricingTable.vue'
import type { AdminBillableItemCard } from './billableCatalogTypes'

const items: AdminBillableItemCard[] = [
  {
    sku: 'svc:active',
    namespace: 'n',
    productKind: 'k',
    displayName: 'Active 服务',
    sortOrder: 1,
    status: 'active',
    pricingVariants: [],
    currentPrices: {},
  },
  {
    sku: 'svc:disabled',
    namespace: 'n',
    productKind: 'k',
    displayName: 'Disabled 服务',
    sortOrder: 2,
    status: 'disabled',
    pricingVariants: [],
    currentPrices: {},
  },
]

describe('BillingPricingTable status column', () => {
  it('locks the Status column and both pills to the fixed slot without ping', () => {
    const { container } = renderWithUser(BillingPricingTable, {
      props: { items },
    })

    // 固定槽 84px + 单元格 px-3×2：列锁定 112px，不随 Active(6)→Disabled(8) 顶宽 enabled 列。
    const statusHeader = screen.getByText('Status')
    expect(statusHeader.closest('th')?.style.width).toBe('112px')

    const slots = screen.getAllByTestId('switch-row-status-slot')
    expect(slots).toHaveLength(2)
    for (const slot of slots) {
      expect(slot.style.minWidth).toBe('84px')
    }
    expect(screen.getByText('Active').parentElement?.style.minWidth).toBe('84px')
    expect(screen.getByText('Disabled').parentElement?.style.minWidth).toBe('84px')
    expect(container.querySelector('.animate-ping')).toBeNull()
  })
})
