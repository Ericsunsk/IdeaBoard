import { describe, expect, it } from 'vitest'
import {
  DEFAULT_OUTBOUND_POLICY_FORM,
  normalizeOutboundPolicyForm,
  outboundPolicyFormToRequest,
  validateOutboundPolicyForm,
} from './contract'

describe('outbound policy form contract', () => {
  it('normalizes the policy into an editable host list', () => {
    expect(
      normalizeOutboundPolicyForm({
        allow_hosts: ['models.internal', '10.0.0.8:8443'],
      }),
    ).toEqual({
      allow_hosts: 'models.internal\n10.0.0.8:8443',
    })
  })

  it('trims and deduplicates hosts before submission', () => {
    expect(
      outboundPolicyFormToRequest({
        allow_hosts: ' Models.Internal \n\nmodels.internal\n10.0.0.8:8443 ',
      }),
    ).toEqual({
      allow_hosts: ['models.internal', '10.0.0.8:8443'],
    })
  })

  it('accepts an empty host list so all private access can be removed', () => {
    expect(outboundPolicyFormToRequest(DEFAULT_OUTBOUND_POLICY_FORM)).toEqual({
      allow_hosts: [],
    })
  })

  it('normalizes pasted URLs into hosts before submission', () => {
    expect(
      outboundPolicyFormToRequest({
        allow_hosts:
          'https://models.internal:8443/v1\nHTTP://Models.Internal./health\nmodels.internal:8443',
      }),
    ).toEqual({
      allow_hosts: ['models.internal:8443', 'models.internal'],
    })
  })

  it('returns a user-readable validation error for unsafe hosts', () => {
    expect(
      validateOutboundPolicyForm({
        ...DEFAULT_OUTBOUND_POLICY_FORM,
        allow_hosts: 'https://user@models.internal',
      }),
    ).toBe('私有主机仅支持 hostname、IP 或 host:port，每行一个')
  })
})
