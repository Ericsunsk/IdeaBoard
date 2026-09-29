import { describe, expect, it } from 'vitest'
import { CONTROL_API_PREFIX } from './controlTransport'

describe('controlTransport', () => {
  it('pins the Control Plane prefix shared with the orval transport entry', () => {
    // Mirrors the private prefix in `shared/api/controlMutator.ts` (the
    // orval codegen entry, which cannot import this Admin module without
    // violating the shared-not-to-features boundary). Keep both in sync.
    expect(CONTROL_API_PREFIX).toBe('/control/v1')
  })
})
