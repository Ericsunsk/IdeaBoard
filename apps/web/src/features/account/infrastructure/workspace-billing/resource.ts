// Workspace billing resource (issue #804, frontend slice).
//
// Sole feature-owned boundary over the Public contract for the workspace
// billing surface. Route pages and components must consume this module,
// never transport internals directly.
//
// Transport flows through the central `api.request` + `runtimeUrls`
// (canonical `/api/v1/workspaces/{workspace_id}/...` URLs) because the
// checked-in `generated-query/public` stubs do not carry the billing
// endpoints yet.
// TODO(#804-merger): regenerate `generated-query/public` from the merged
// `openapi.public.yaml` (see backend OpenAPI deltas in
// `contract_v1_billing.go`) and re-point this module at the generated
// client, keeping the domain types and query keys exported here stable.
//
// Display wording follows `account/CONTEXT.md`: user-visible copy is always
// `Idea`/`Ideas`; code, domain models, and transport stay
// `Credit`/`credits`. All money is integer minor units; only presentation
// formats via the shared `formatIdeaAmount` formatter.

import { api } from '../../../../shared/api/client'
import {
  buildContractRequestHeaders,
  isOpaqueId,
  newRequestId,
  opaqueIdPrefix,
  parseContractEnvelope,
  type ContractKernelError,
} from '../../../../shared/api/contractKernel'
import { workspaceBillingCheckoutIdempotencyKey } from '../../../../shared/api/idempotencyKey'
import { resolveApiUrl } from '../../../../shared/config/runtimeUrls'

// ---------------------------------------------------------------------------
// Domain models (camelCase; snake_case normalization happens here).
// ---------------------------------------------------------------------------

export type WorkspaceBillingAccount = {
  workspaceId: string
  ownerType: 'workspace'
  createdAt: string | null
  updatedAt: string | null
}

export type WorkspaceCreditsBalance = {
  workspaceId: string
  availableCredits: number
  reservedCredits: number
  status: string
  activeRestrictions: string[]
}

export type WorkspaceLedgerEntry = {
  transactionId: string
  commandType: string
  amountCredits: number
  availableCreditsAfter: number
  reservedCreditsAfter: number
  createdAt: string
}

export type WorkspaceLedgerPage = {
  entries: WorkspaceLedgerEntry[]
  nextCursor: string | null
  hasMore: boolean
}

export type WorkspaceBillingRecord = {
  recordId: string
  occurredAt: string
  kind: string
  title: string
  creditDelta: number
  availableCreditsAfter: number
}

export type WorkspaceBillingRecordsPage = {
  records: WorkspaceBillingRecord[]
  nextCursor: string | null
  hasMore: boolean
}

export type WorkspaceUsageLifetime = {
  totalCredits: number
  peakDailyCredits: number
  averageDailyCredits: number
  taskCount: number
  activeDays: number
}

export type WorkspaceUsage = {
  timezone: string
  lifetime: WorkspaceUsageLifetime
  trend: Array<{ date: string; credits: number }>
}

export type WorkspaceUsageRange = '7d' | '30d' | '1y'

export type WorkspaceCheckoutSession = {
  sessionId: string
  status?: string | undefined
  ideaQuantity?: number | undefined
  checkoutUrl?: string | undefined
  expiresAt?: string | undefined
  reused?: boolean | undefined
}

export type CreateWorkspaceCheckoutInput = {
  ideaQuantity: number
  termsAccepted: boolean
  termsVersion?: string | undefined
  refundPolicyVersion?: string | undefined
}

// Hidden (unknown workspace, never-shared) surfaces as 404; visible-but-
// forbidden as 403; frozen/restricted wallets as billing_wallet_restricted.
// Branch on `kind`, not on copy.
export type WorkspaceBillingAccessKind = 'hidden' | 'forbidden' | 'restricted'

export const WORKSPACE_BILLING_PAGE_SIZE_DEFAULT = 20
export const WORKSPACE_BILLING_PAGE_SIZE_MAX = 100

// ---------------------------------------------------------------------------
// URLs (canonical agreed-contract paths; transport stays in api.request).
// ---------------------------------------------------------------------------

function workspacePath(workspaceId: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceId)}`
}

function withCursorQuery(base: string, params?: { cursor?: string | null | undefined; pageSize?: number | undefined }): string {
  const search = new URLSearchParams()
  const cursor = typeof params?.cursor === 'string' ? params.cursor.trim() : ''
  if (cursor) search.set('cursor', cursor)
  const pageSize = params?.pageSize
  if (typeof pageSize === 'number' && Number.isFinite(pageSize)) {
    search.set('page_size', String(Math.min(WORKSPACE_BILLING_PAGE_SIZE_MAX, Math.max(1, Math.floor(pageSize)))))
  }
  const query = search.toString()
  return query ? `${base}?${query}` : base
}

export const workspaceBillingUrls = {
  billingAccount: (workspaceId: string): string =>
    resolveApiUrl(`${workspacePath(workspaceId)}/billing-account`),
  creditsBalance: (workspaceId: string): string =>
    resolveApiUrl(`${workspacePath(workspaceId)}/credits/balance`),
  creditsLedger: (workspaceId: string, params?: { cursor?: string | null | undefined; pageSize?: number | undefined }): string =>
    resolveApiUrl(withCursorQuery(`${workspacePath(workspaceId)}/credits/ledger`, params)),
  billingRecords: (workspaceId: string, params?: { cursor?: string | null | undefined; pageSize?: number | undefined }): string =>
    resolveApiUrl(withCursorQuery(`${workspacePath(workspaceId)}/billing-records`, params)),
  usage: (workspaceId: string, range: WorkspaceUsageRange): string =>
    resolveApiUrl(`${workspacePath(workspaceId)}/usage?range=${encodeURIComponent(range)}`),
  checkoutSessions: (workspaceId: string): string =>
    resolveApiUrl(`${workspacePath(workspaceId)}/checkout-sessions`),
  checkoutSession: (workspaceId: string, sessionId: string): string =>
    resolveApiUrl(`${workspacePath(workspaceId)}/checkout-sessions/${encodeURIComponent(sessionId)}`),
}

// ---------------------------------------------------------------------------
// Headers (Request-Id always; Idempotency-Key header-only on writes).
// ---------------------------------------------------------------------------

export type WorkspaceBillingRequestOptions = {
  signal?: AbortSignal | null | undefined
  requestId?: string | undefined
}

function asTrimmedString(value: unknown): string | null {
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  return trimmed ? trimmed : null
}

function readHeaders(options: WorkspaceBillingRequestOptions | undefined): Record<string, string> {
  return buildContractRequestHeaders({
    requestId: asTrimmedString(options?.requestId) ?? newRequestId(),
  })
}

export function buildWorkspaceBillingCheckoutHeaders(options?: WorkspaceBillingRequestOptions & { idempotencyKey?: string | undefined }): Record<string, string> {
  const idempotencyKey = asTrimmedString(options?.idempotencyKey) ?? workspaceBillingCheckoutIdempotencyKey()
  return buildContractRequestHeaders({
    requestId: asTrimmedString(options?.requestId) ?? newRequestId(),
    idempotencyKey,
  })
}

// ---------------------------------------------------------------------------
// Parsers (transport normalization + strict shape checks).
// ---------------------------------------------------------------------------

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function requireWorkspaceId(value: unknown): string {
  if (typeof value !== 'string' || !isOpaqueId(value) || opaqueIdPrefix(value) !== 'ws') {
    throw new Error('Invalid workspace billing payload')
  }
  return value
}

function asIntAmount(value: unknown): number {
  if (typeof value !== 'number' || !Number.isInteger(value)) {
    throw new Error('Invalid workspace billing amount')
  }
  return value
}

function asString(value: unknown, fallback = ''): string {
  return typeof value === 'string' ? value : fallback
}

function contractFailure(payload: unknown, fallback: string): Error {
  const parsed = parseContractEnvelope<unknown>(payload)
  if (!parsed.ok) {
    const error = new Error(parsed.error.message || fallback) as ContractKernelError
    error.code = parsed.error.code
    error.requestId = parsed.error.request_id
    error.details = parsed.error.details
    return error
  }
  return new Error(fallback)
}

function requireData(payload: unknown, fallback: string): unknown {
  const parsed = parseContractEnvelope<unknown>(payload)
  if (!parsed.ok) throw contractFailure(payload, fallback)
  if (parsed.data == null || !isRecord(parsed.data)) throw new Error(fallback)
  return parsed.data
}

function readPageMeta(payload: unknown): { nextCursor: string | null; hasMore: boolean } {
  const parsed = parseContractEnvelope<unknown>(payload)
  if (parsed.ok && isRecord(parsed.meta)) {
    const nextCursor = typeof parsed.meta.next_cursor === 'string' ? parsed.meta.next_cursor : null
    return { nextCursor, hasMore: parsed.meta.has_more === true }
  }
  return { nextCursor: null, hasMore: false }
}

export function parseWorkspaceBillingAccount(payload: unknown): WorkspaceBillingAccount {
  const data = requireData(payload, 'Invalid billing account response') as Record<string, unknown>
  return {
    workspaceId: requireWorkspaceId(data.workspace_id),
    ownerType: 'workspace',
    createdAt: typeof data.created_at === 'string' ? data.created_at : null,
    updatedAt: typeof data.updated_at === 'string' ? data.updated_at : null,
  }
}

export function parseWorkspaceCreditsBalance(payload: unknown): WorkspaceCreditsBalance {
  const data = requireData(payload, 'Invalid balance response') as Record<string, unknown>
  const restrictions = Array.isArray(data.active_restrictions)
    ? data.active_restrictions.filter((entry): entry is string => typeof entry === 'string')
    : []
  return {
    workspaceId: requireWorkspaceId(data.workspace_id),
    availableCredits: asIntAmount(data.available_credits),
    reservedCredits: asIntAmount(data.reserved_credits),
    status: asString(data.status, 'active'),
    activeRestrictions: restrictions,
  }
}

function parseLedgerEntry(raw: unknown): WorkspaceLedgerEntry {
  if (!isRecord(raw)) throw new Error('Invalid ledger entry')
  if (typeof raw.transaction_id !== 'string' || !raw.transaction_id) throw new Error('Invalid ledger entry')
  return {
    transactionId: raw.transaction_id,
    commandType: asString(raw.command_type),
    amountCredits: asIntAmount(raw.amount_credits),
    availableCreditsAfter: asIntAmount(raw.available_credits_after),
    reservedCreditsAfter: asIntAmount(raw.reserved_credits_after),
    createdAt: asString(raw.created_at),
  }
}

export function parseWorkspaceLedgerPage(payload: unknown): WorkspaceLedgerPage {
  const parsed = parseContractEnvelope<unknown>(payload)
  if (!parsed.ok) throw contractFailure(payload, 'Invalid ledger response')
  if (!Array.isArray(parsed.data)) throw new Error('Invalid ledger response')
  const { nextCursor, hasMore } = readPageMeta(payload)
  return { entries: parsed.data.map(parseLedgerEntry), nextCursor, hasMore }
}

function parseBillingRecord(raw: unknown): WorkspaceBillingRecord {
  if (!isRecord(raw)) throw new Error('Invalid billing record')
  if (typeof raw.record_id !== 'string' || !raw.record_id) throw new Error('Invalid billing record')
  return {
    recordId: raw.record_id,
    occurredAt: asString(raw.occurred_at),
    kind: asString(raw.kind),
    title: asString(raw.title),
    creditDelta: asIntAmount(raw.credit_delta),
    availableCreditsAfter: asIntAmount(raw.available_credits_after),
  }
}

export function parseWorkspaceBillingRecordsPage(payload: unknown): WorkspaceBillingRecordsPage {
  const parsed = parseContractEnvelope<unknown>(payload)
  if (!parsed.ok) throw contractFailure(payload, 'Invalid billing records response')
  if (!Array.isArray(parsed.data)) throw new Error('Invalid billing records response')
  const { nextCursor, hasMore } = readPageMeta(payload)
  return { records: parsed.data.map(parseBillingRecord), nextCursor, hasMore }
}

export function parseWorkspaceUsage(payload: unknown): WorkspaceUsage {
  const data = requireData(payload, 'Invalid usage response') as Record<string, unknown>
  const lifetime = isRecord(data.lifetime) ? data.lifetime : {}
  const trend = Array.isArray(data.trend)
    ? data.trend.filter(isRecord).map((entry) => ({
        date: asString(entry.date),
        credits: typeof entry.credits === 'number' ? entry.credits : 0,
      }))
    : []
  const average = typeof lifetime.average_daily_credits === 'number' ? lifetime.average_daily_credits : 0
  const asInt = (value: unknown): number =>
    typeof value === 'number' && Number.isInteger(value) ? value : 0
  return {
    timezone: asString(data.timezone, 'Asia/Shanghai'),
    lifetime: {
      totalCredits: asInt(lifetime.total_credits),
      peakDailyCredits: asInt(lifetime.peak_daily_credits),
      averageDailyCredits: average,
      taskCount: asInt(lifetime.task_count),
      activeDays: asInt(lifetime.active_days),
    },
    trend,
  }
}

export function parseWorkspaceCheckoutSession(payload: unknown): WorkspaceCheckoutSession {
  const data = requireData(payload, 'Invalid checkout session response') as Record<string, unknown>
  if (typeof data.session_id !== 'string' || !data.session_id) {
    throw new Error('Invalid checkout session response')
  }
  const session: WorkspaceCheckoutSession = { sessionId: data.session_id }
  if (typeof data.status === 'string') session.status = data.status
  if (typeof data.idea_quantity === 'number') session.ideaQuantity = data.idea_quantity
  if (typeof data.checkout_url === 'string') session.checkoutUrl = data.checkout_url
  if (typeof data.expires_at === 'string') session.expiresAt = data.expires_at
  if (typeof data.reused === 'boolean') session.reused = data.reused
  return session
}

// ---------------------------------------------------------------------------
// Error classification (branch on kind: hidden 404 vs forbidden 403 vs
// restricted wallet; message text is never branched on).
// ---------------------------------------------------------------------------

function readErrorStatus(error: unknown): number | null {
  if (!isRecord(error)) return null
  const status = (error as { status?: unknown }).status
  return typeof status === 'number' && Number.isFinite(status) ? status : null
}

function readErrorCode(error: unknown): string | null {
  if (!isRecord(error)) return null
  const record = error as Record<string, unknown>
  if (typeof record.code === 'string' && record.code) return record.code.toLowerCase()
  const data = isRecord(record.data) ? (record.data as Record<string, unknown>) : null
  const nested = data == null ? null : (data.error ?? data)
  if (isRecord(nested) && typeof nested.code === 'string' && nested.code) {
    return nested.code.toLowerCase()
  }
  if (data != null && typeof data.code === 'string' && data.code) return data.code.toLowerCase()
  return null
}

export function classifyWorkspaceBillingAccessError(error: unknown): WorkspaceBillingAccessKind | null {
  const code = readErrorCode(error)
  if (code === 'billing_wallet_restricted' || code === 'wallet_restricted') return 'restricted'
  const status = readErrorStatus(error)
  if (status === 404) return 'hidden'
  if (status === 403) return 'forbidden'
  if (!code) return null
  if (code.includes('not_found') || code === 'workspace_hidden') return 'hidden'
  if (code.includes('forbidden') || code.includes('denied')) return 'forbidden'
  return null
}

export function isWorkspaceBillingRestricted(error: unknown): boolean {
  return classifyWorkspaceBillingAccessError(error) === 'restricted'
}

// ---------------------------------------------------------------------------
// Fetchers (single source of truth for server state; TanStack Query caches
// under workspaceBillingQueryKeys below).
// ---------------------------------------------------------------------------

export async function fetchWorkspaceBillingAccount(
  workspaceId: string,
  options?: WorkspaceBillingRequestOptions,
): Promise<WorkspaceBillingAccount> {
  requireWorkspaceId(workspaceId)
  const payload = await api.request<unknown>(workspaceBillingUrls.billingAccount(workspaceId), {
    method: 'GET',
    ...(options?.signal == null ? {} : { signal: options.signal }),
    headers: readHeaders(options),
  })
  return parseWorkspaceBillingAccount(payload)
}

export async function fetchWorkspaceCreditsBalance(
  workspaceId: string,
  options?: WorkspaceBillingRequestOptions,
): Promise<WorkspaceCreditsBalance> {
  requireWorkspaceId(workspaceId)
  const payload = await api.request<unknown>(workspaceBillingUrls.creditsBalance(workspaceId), {
    method: 'GET',
    ...(options?.signal == null ? {} : { signal: options.signal }),
    headers: readHeaders(options),
  })
  return parseWorkspaceCreditsBalance(payload)
}

export async function fetchWorkspaceLedgerPage(
  workspaceId: string,
  params?: { cursor?: string | null | undefined; pageSize?: number | undefined },
  options?: WorkspaceBillingRequestOptions,
): Promise<WorkspaceLedgerPage> {
  requireWorkspaceId(workspaceId)
  const payload = await api.request<unknown>(workspaceBillingUrls.creditsLedger(workspaceId, params), {
    method: 'GET',
    ...(options?.signal == null ? {} : { signal: options.signal }),
    headers: readHeaders(options),
  })
  return parseWorkspaceLedgerPage(payload)
}

export async function fetchWorkspaceBillingRecordsPage(
  workspaceId: string,
  params?: { cursor?: string | null | undefined; pageSize?: number | undefined },
  options?: WorkspaceBillingRequestOptions,
): Promise<WorkspaceBillingRecordsPage> {
  requireWorkspaceId(workspaceId)
  const payload = await api.request<unknown>(workspaceBillingUrls.billingRecords(workspaceId, params), {
    method: 'GET',
    ...(options?.signal == null ? {} : { signal: options.signal }),
    headers: readHeaders(options),
  })
  return parseWorkspaceBillingRecordsPage(payload)
}

export async function fetchWorkspaceUsage(
  workspaceId: string,
  range: WorkspaceUsageRange,
  options?: WorkspaceBillingRequestOptions,
): Promise<WorkspaceUsage> {
  requireWorkspaceId(workspaceId)
  if (range !== '7d' && range !== '30d' && range !== '1y') throw new Error('Invalid usage range')
  const payload = await api.request<unknown>(workspaceBillingUrls.usage(workspaceId, range), {
    method: 'GET',
    ...(options?.signal == null ? {} : { signal: options.signal }),
    headers: readHeaders(options),
  })
  return parseWorkspaceUsage(payload)
}

export async function createWorkspaceCheckoutSession(
  workspaceId: string,
  input: CreateWorkspaceCheckoutInput,
  options?: WorkspaceBillingRequestOptions & { idempotencyKey?: string | undefined },
): Promise<WorkspaceCheckoutSession> {
  requireWorkspaceId(workspaceId)
  // No client price input: the server mints the immutable snapshot
  // (Payments PurchaseQuote) and binds it to the workspace billing account.
  const payload = await api.request<unknown>(workspaceBillingUrls.checkoutSessions(workspaceId), {
    method: 'POST',
    ...(options?.signal == null ? {} : { signal: options.signal }),
    headers: {
      ...readHeaders(options),
      ...buildWorkspaceBillingCheckoutHeaders(options),
    },
    body: {
      data: {
        idea_quantity: input.ideaQuantity,
        terms_accepted: input.termsAccepted,
        ...(input.termsVersion === undefined ? {} : { terms_version: input.termsVersion }),
        ...(input.refundPolicyVersion === undefined ? {} : { refund_policy_version: input.refundPolicyVersion }),
      },
    },
  })
  return parseWorkspaceCheckoutSession(payload)
}

export async function fetchWorkspaceCheckoutSession(
  workspaceId: string,
  sessionId: string,
  options?: WorkspaceBillingRequestOptions,
): Promise<WorkspaceCheckoutSession> {
  requireWorkspaceId(workspaceId)
  if (!sessionId.trim()) throw new Error('Checkout session id is required')
  const payload = await api.request<unknown>(workspaceBillingUrls.checkoutSession(workspaceId, sessionId), {
    method: 'GET',
    ...(options?.signal == null ? {} : { signal: options.signal }),
    headers: readHeaders(options),
  })
  return parseWorkspaceCheckoutSession(payload)
}

// ---------------------------------------------------------------------------
// Query keys (TanStack Query owns the list lifecycle).
// ---------------------------------------------------------------------------

export const workspaceBillingQueryKeys = {
  all: ['workspace', 'billing'] as const,
  account: (workspaceId: string) => ['workspace', 'billing', workspaceId, 'account'] as const,
  balance: (workspaceId: string) => ['workspace', 'billing', workspaceId, 'credits', 'balance'] as const,
  ledger: (workspaceId: string) => ['workspace', 'billing', workspaceId, 'credits', 'ledger'] as const,
  records: (workspaceId: string) => ['workspace', 'billing', workspaceId, 'billing-records'] as const,
  usage: (workspaceId: string, range: WorkspaceUsageRange) =>
    ['workspace', 'billing', workspaceId, 'usage', range] as const,
  checkoutSession: (workspaceId: string, sessionId: string) =>
    ['workspace', 'billing', workspaceId, 'checkout-sessions', sessionId] as const,
}
