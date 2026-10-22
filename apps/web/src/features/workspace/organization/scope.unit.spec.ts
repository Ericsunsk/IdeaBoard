import { describe, expect, it } from 'vitest'
import { filterWorkspacesByScope, parseOrganizationScope, resolveOrganizationScope } from './scope'
import type { OrganizationSummary, WorkspaceSummary } from '../selection/workspaceResource'

function workspace(publicId: string, organizationId: string | null): WorkspaceSummary {
  return { publicId, name: publicId, organizationId, isDefault: false }
}

const ORGS: OrganizationSummary[] = [{ publicId: 'org_c3', name: 'Acme', status: 'active' }]

describe('parseOrganizationScope', () => {
  it('resolves an org id to the organization scope', () => {
    expect(parseOrganizationScope('org_c3')).toEqual({
      kind: 'organization',
      organizationId: 'org_c3',
    })
  })

  it('falls back to personal for empty or malformed input', () => {
    expect(parseOrganizationScope(null)).toEqual({ kind: 'personal' })
    expect(parseOrganizationScope('ws_x1')).toEqual({ kind: 'personal' })
  })
})

describe('resolveOrganizationScope', () => {
  it('keeps the stored organization while the caller still holds it', () => {
    expect(resolveOrganizationScope(ORGS, 'org_c3')).toEqual({
      kind: 'organization',
      organizationId: 'org_c3',
    })
  })

  it('falls back to personal when the stored organization is gone', () => {
    expect(resolveOrganizationScope(ORGS, 'org_gone')).toEqual({ kind: 'personal' })
    expect(resolveOrganizationScope([], 'org_c3')).toEqual({ kind: 'personal' })
  })
})

describe('filterWorkspacesByScope', () => {
  const items = [
    workspace('ws_personal', null),
    workspace('ws_org', 'org_c3'),
    workspace('ws_other', 'org_other'),
  ]

  it('shows only personal workspaces on the personal scope', () => {
    expect(filterWorkspacesByScope(items, { kind: 'personal' }).map((w) => w.publicId)).toEqual([
      'ws_personal',
    ])
  })

  it('shows only the active organization workspaces on the organization scope', () => {
    expect(
      filterWorkspacesByScope(items, { kind: 'organization', organizationId: 'org_c3' }).map(
        (w) => w.publicId,
      ),
    ).toEqual(['ws_org'])
  })
})
