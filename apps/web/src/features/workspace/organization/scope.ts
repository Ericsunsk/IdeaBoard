// Organization scope switching (issue #951, frontend slice).
//
// Personal and Organization are two switchable scopes: the caller holds at
// most one organization and switches between Personal and that
// organization. The organization itself is the management entry (no boards,
// content, or consumption); workspace access follows the role mapping
// (owner/admin automatic, member explicit, non-member hidden until join).

import { shallowRef } from 'vue'
import {
  readLocalStorageItem,
  removeLocalStorageItem,
  writeLocalStorageItem,
} from '../../../core/utils/storage'
import { isOpaqueId, opaqueIdPrefix } from '@/api/http/contractKernel'
import type { OrganizationSummary } from '../selection/workspaceResource'

const ORGANIZATION_SCOPE_STORAGE_KEY = 'ideaboard.organization.scope.v1'

export type OrganizationScope =
  { kind: 'personal' } | { kind: 'organization'; organizationId: string }

function asOrganizationId(value: unknown): string | null {
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  if (!isOpaqueId(trimmed) || opaqueIdPrefix(trimmed) !== 'org') return null
  return trimmed
}

function parseOrganizationScope(raw: unknown): OrganizationScope {
  const orgId = asOrganizationId(raw)
  if (orgId) return { kind: 'organization', organizationId: orgId }
  return { kind: 'personal' }
}

function serializeOrganizationScope(scope: OrganizationScope): string | null {
  if (scope.kind === 'organization') return scope.organizationId
  return null
}

// Resolution: an explicit stored organization wins when the caller still
// holds it; otherwise Personal. A caller with no organization always
// resolves Personal.
function resolveOrganizationScope(
  organizations: readonly OrganizationSummary[],
  storedRaw?: unknown,
): OrganizationScope {
  const stored = asOrganizationId(storedRaw)
  if (stored && organizations.some((org) => org.publicId === stored)) {
    return { kind: 'organization', organizationId: stored }
  }
  return { kind: 'personal' }
}

type OrganizationScopePersist = {
  readScope: () => OrganizationScope
  writeScope: (scope: OrganizationScope) => void
}

export function localStorageOrganizationScopePersist(
  key: string = ORGANIZATION_SCOPE_STORAGE_KEY,
): OrganizationScopePersist {
  return {
    readScope: () => parseOrganizationScope(readLocalStorageItem(key)),
    writeScope: (scope) => {
      const raw = serializeOrganizationScope(scope)
      if (raw == null) {
        removeLocalStorageItem(key)
        return
      }
      if (!writeLocalStorageItem(key, raw)) {
        console.warn('[organization] persist scope failed')
      }
    },
  }
}

export function createOrganizationScopeStore(deps?: {
  persist?: OrganizationScopePersist | undefined
}) {
  const persist = deps?.persist ?? localStorageOrganizationScopePersist()
  const scope = shallowRef<OrganizationScope>(persist.readScope())
  let isPrimed = false

  function current(): OrganizationScope {
    if (!isPrimed) {
      isPrimed = true
      scope.value = persist.readScope()
    }
    return scope.value
  }

  return {
    scope,
    getScope: current,
    select(scopeNext: OrganizationScope): void {
      if (scopeNext.kind === 'organization' && !asOrganizationId(scopeNext.organizationId)) {
        throw new Error('Invalid organization scope')
      }
      scope.value = scopeNext
      isPrimed = true
      persist.writeScope(scopeNext)
    },
    selectPersonal(): void {
      scope.value = { kind: 'personal' }
      isPrimed = true
      persist.writeScope({ kind: 'personal' })
    },
    reconcile(organizations: readonly OrganizationSummary[]): OrganizationScope {
      const currentScope = current()
      const resolved = resolveOrganizationScope(
        organizations,
        serializeOrganizationScope(currentScope),
      )
      if (
        resolved.kind !== currentScope.kind ||
        (resolved.kind === 'organization' &&
          currentScope.kind === 'organization' &&
          resolved.organizationId !== currentScope.organizationId)
      ) {
        scope.value = resolved
        isPrimed = true
        persist.writeScope(resolved)
      }
      return resolved
    },
  }
}

export type OrganizationScopeStore = ReturnType<typeof createOrganizationScopeStore>

let sharedScopeStore: OrganizationScopeStore | null = null

export function installOrganizationScopeStore(deps?: {
  persist?: OrganizationScopePersist | undefined
}): OrganizationScopeStore {
  if (!sharedScopeStore) sharedScopeStore = createOrganizationScopeStore(deps)
  return sharedScopeStore
}

export function getOrganizationScopeStore(): OrganizationScopeStore {
  return installOrganizationScopeStore()
}
