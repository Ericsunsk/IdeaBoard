import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  createRewardCodeBatchCsv,
  downloadRewardCodeBatchCsv,
} from './rewardCodeBatchCsv'

describe('reward Code Batch CSV', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('writes UTF-8 CSV with the fixed columns, formula defense, and RFC-style escaping', () => {
    const csv = createRewardCodeBatchCsv({
      codes: ['ABCD-EFGH-JKMN-PQRS'],
      campaignName: '=SUM(1,2),"VIP"\n第二行',
      creditAmount: 80,
      expiresAt: '2026-08-01T00:00:00Z',
    })

    expect(csv).toBe(
      '\uFEFFcode,campaign_name,credit_amount,expires_at\r\n' +
        '"ABCD-EFGH-JKMN-PQRS","\'=SUM(1,2),""VIP""\n第二行","80","2026-08-01T00:00:00Z"\r\n',
    )
  })

  it.each(['=1+1', '+cmd', '-2+3', '@SUM(A1)', '\tformula', '\rformula'])(
    'neutralizes spreadsheet formula prefix %j',
    (value) => {
      const csv = createRewardCodeBatchCsv({
        codes: [value],
        campaignName: '安全活动',
        creditAmount: 1,
        expiresAt: '',
      })

      expect(csv.split('\r\n')[1]).toMatch(/^"'/)
    },
  )

  it('downloads through a temporary object URL and revokes it immediately', () => {
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined)
    const createObjectURL = vi.fn(() => 'blob:reward-export')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { createObjectURL, revokeObjectURL })

    downloadRewardCodeBatchCsv({
      batchId: 21,
      campaignId: 7,
      csv: '\uFEFFcode,campaign_name,credit_amount,expires_at\r\n',
    })

    expect(createObjectURL).toHaveBeenCalledWith(expect.any(Blob))
    expect(click).toHaveBeenCalledOnce()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:reward-export')
  })
})
