// Request-count gates: single provider + scalar keys, minimal invalidation,
// lazy + aggregate-first. See `src/api/CONTEXT.md` (Request-Count Hard
// Constraints). New hand-rolled `*QueryKeys` / `*_QUERY_ROOT` factories must
// register here (name must appear in this file); the registry scan fails CI
// otherwise. Same-screen cold-cache distinct GETs over 4 must land as
// aggregate-read or lazy-load, not as parallel endpoints.
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { getListWorkspaceMembersQueryKey } from '../../../api/generated-query/public/workspace-members/workspace-members'
import { billingAccountQueryKeys } from './billing-account/resource'
import { billingRecordsQueryKeys } from './billing/resource'
import { billingTransferQueryKeys } from './billing-transfer/resource'
import { creditsQueryKeys } from './credits/resource'
import { paymentsQueryKeys } from './payments/resource'
import { memberQuotaQueryKeys } from './quota/resource'
import { rewardRedemptionsQueryKeys } from './rewards/resource'
import { usageQueryKeys } from './usage/resource'
import { workspaceQueryKeys } from '../../workspace'
import { boardQueryKeys } from '../../board'

const SRC_ROOT = join(dirname(fileURLToPath(import.meta.url)), '../../..')
const SELF_FILE = fileURLToPath(import.meta.url)

function listTsFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = join(dir, entry.name)
    if (entry.isDirectory()) return listTsFiles(full)
    return full.endsWith('.ts') && !full.endsWith('.spec.ts') ? [full] : []
  })
}

function exportedKeyFactories(): Array<{ file: string; name: string }> {
  const found: Array<{ file: string; name: string }> = []
  for (const file of listTsFiles(SRC_ROOT)) {
    const rel = file.slice(SRC_ROOT.length + 1).replace(/\\/g, '/')
    if (rel.startsWith('api/generated-query/')) continue
    const source = readFileSync(file, 'utf8')
    for (const match of source.matchAll(/export const (\w*QueryKeys|\w+_QUERY_ROOT)\b/g)) {
      const name = match[1]
      if (name) found.push({ file: rel, name })
    }
  }
  return found
}

function assertScalarKey(key: readonly unknown[], label: string): void {
  for (const segment of key) {
    expect(
      segment === null || ['string', 'number', 'boolean', 'undefined'].includes(typeof segment),
      `${label}: non-scalar key segment ${String(segment)}`,
    ).toBe(true)
  }
}

describe('query key request budget', () => {
  it('registers every hand-rolled query key factory in this spec', () => {
    const selfSource = readFileSync(SELF_FILE, 'utf8')
    const unregistered = exportedKeyFactories().filter(({ name }) => !selfSource.includes(name))
    expect(
      unregistered.map(({ file, name }) => `${file}#${name}`),
      'register new key factories in queryKeyRequestBudget.contract.spec.ts',
    ).toEqual([])
  })

  it('keeps request keys scalar-only (workspaceId/billingAccountId/identity scalars)', () => {
    const ws = 'ws_x'
    const ba = 'ba_x'
    const usr = 'usr_x'
    const samples: Array<[string, readonly unknown[]]> = [
      ['billingAccount.all', billingAccountQueryKeys.all],
      ['billingAccount.detail', billingAccountQueryKeys.detail(ws, usr)],
      ['billingTransfer.all', billingTransferQueryKeys.all],
      ['billingTransfer.list', billingTransferQueryKeys.list(ws)],
      ['billingTransfer.members', billingTransferQueryKeys.members(ws, usr)],
      ['billingRecords.all', billingRecordsQueryKeys.all],
      ['billingRecords.list', billingRecordsQueryKeys.list(ws, ba, usr, 'account')],
      ['credits.all', creditsQueryKeys.all],
      ['credits.wallet', creditsQueryKeys.wallet(ws, ba, usr)],
      ['memberQuota.all', memberQuotaQueryKeys.all],
      ['memberQuota.self', memberQuotaQueryKeys.self(ws)],
      ['memberQuota.list', memberQuotaQueryKeys.list(ws)],
      ['usage.current', usageQueryKeys.current(ws, ba, usr, 'account', '30d')],
      ['rewardRedemptions.all', rewardRedemptionsQueryKeys.all],
      ['rewardRedemptions.list', rewardRedemptionsQueryKeys.list(ws)],
      ['payments.all', paymentsQueryKeys.all],
      ['payments.options', paymentsQueryKeys.options(ws)],
      ['payments.pending', paymentsQueryKeys.pending(ws)],
      ['payments.history', paymentsQueryKeys.history(ws)],
      ['payments.returnStatus', paymentsQueryKeys.returnStatus(ws, 'tok_x')],
      ['workspace.all', workspaceQueryKeys.all],
      ['workspace.organizations', workspaceQueryKeys.organizations],
      ['workspace.organizationMembers', workspaceQueryKeys.organizationMembers('org_x')],
      ['roster.list', getListWorkspaceMembersQueryKey(ws)],
      ['board.scopeRoot', boardQueryKeys.scopeRoot(ws)],
      ['board.list', boardQueryKeys.list(ws)],
      ['board.document', boardQueryKeys.document(ws, 'board_x')],
      ['board.access', boardQueryKeys.access(ws, 'board_x')],
    ]
    for (const [label, key] of samples) assertScalarKey(key, label)
  })
})
