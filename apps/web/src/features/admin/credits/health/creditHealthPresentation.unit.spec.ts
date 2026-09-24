import { describe, expect, it } from 'vitest'
import { CreditHealthStatus } from '../shared/api'
import { getCreditHealthPresentation } from './creditHealthPresentation'

describe('credit health presentation', () => {
  it('keeps healthy navigation quiet and labels every non-healthy state', () => {
    expect(getCreditHealthPresentation(CreditHealthStatus.healthy)).toMatchObject({
      badgeLabel: null,
      iconKey: 'healthy',
    })
    for (const status of [
      CreditHealthStatus.unchecked,
      CreditHealthStatus.attention,
      CreditHealthStatus.critical,
      CreditHealthStatus.stale,
      CreditHealthStatus.failed,
    ]) {
      expect(getCreditHealthPresentation(status).badgeLabel).toBeTruthy()
    }
  })

  it('owns the icon discriminator for every generated health status', () => {
    expect(getCreditHealthPresentation(CreditHealthStatus.unchecked).iconKey).toBe('time')
    expect(getCreditHealthPresentation(CreditHealthStatus.stale).iconKey).toBe('time')
    expect(getCreditHealthPresentation(CreditHealthStatus.failed).iconKey).toBe('failed')
    expect(getCreditHealthPresentation(CreditHealthStatus.attention).iconKey).toBe('warning')
    expect(getCreditHealthPresentation(CreditHealthStatus.critical).iconKey).toBe('warning')
  })
})
