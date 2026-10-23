import { describe, expect, it } from 'vitest'
import {
  createOrganizationScopeStore,
  getOrganizationScopeStore,
  installOrganizationScopeStore,
} from './scope'
import type { OrganizationScope } from './scope'
import type { OrganizationSummary } from '../selection/workspaceResource'

function memoryPersist(initial: OrganizationScope = { kind: 'personal' }) {
  let current = initial
  return {
    readScope: () => current,
    writeScope: (scope: OrganizationScope) => {
      current = scope
    },
  }
}

const ORGS: OrganizationSummary[] = [{ publicId: 'org_c3', name: 'Acme', status: 'active' }]

describe('organization scope store', () => {
  it('keeps the stored organization while the caller still holds it', () => {
    const store = createOrganizationScopeStore({
      persist: memoryPersist({ kind: 'organization', organizationId: 'org_c3' }),
    })

    expect(store.reconcile(ORGS)).toEqual({ kind: 'organization', organizationId: 'org_c3' })
  })

  it('falls back to personal when the stored organization is gone', () => {
    const store = createOrganizationScopeStore({
      persist: memoryPersist({ kind: 'organization', organizationId: 'org_gone' }),
    })

    expect(store.reconcile(ORGS)).toEqual({ kind: 'personal' })
    expect(store.getScope()).toEqual({ kind: 'personal' })
  })

  it('drops back to personal on selectPersonal (close/leave path)', () => {
    const store = createOrganizationScopeStore({
      persist: memoryPersist({ kind: 'organization', organizationId: 'org_c3' }),
    })
    store.select({ kind: 'organization', organizationId: 'org_c3' })

    store.selectPersonal()

    expect(store.getScope()).toEqual({ kind: 'personal' })
  })

  it('rejects an invalid organization scope', () => {
    const store = createOrganizationScopeStore({ persist: memoryPersist() })

    expect(() => store.select({ kind: 'organization', organizationId: 'ws_x1' })).toThrow(
      'Invalid organization scope',
    )
  })

  it('installs the shared store exactly once', () => {
    const installed = installOrganizationScopeStore({ persist: memoryPersist() })

    expect(getOrganizationScopeStore()).toBe(installed)
  })
})
