import {
  getControlCatalogModelPool,
  replaceControlCatalogModelPool,
} from '../../../../shared/api/generated-query/control/control-billing/control-billing'
import {
  buildContractRequestHeaders,
  newRequestId,
  requireContractData,
} from '../../../../shared/api/contractKernel'
import { createAdminGatewayResource } from '../../models/infrastructure/adminGatewayResource'
import {
  OUTPUT_RESOLUTION_OPTIONS,
  parseOutputResolution,
} from '../../../../shared/domain/generationContract'
import type {
  AdminModelPool,
  AdminModelPoolEntry,
  AdminModelPoolSaveResult,
  AdminPoolCatalogModel,
  PoolResolution,
} from './billableCatalogTypes'

// Admin Service Model Pool boundary (#849). Reads and atomic replaces go
// through the Control Plane billing pool operations with the contract
// envelope (`{data}`); the PUT carries a header-only Idempotency-Key
// plus an operator reason (mirrored into the body `reason` field) and
// emits a control audit record. Pages and dialogs consume this module,
// never generated clients directly.
//
// The add-from-catalog list reuses the existing Model catalog resource
// (no second catalog): entries are matched by catalog id. The lister is
// injectable so tests pass a fake without MSW.
export type ModelPoolMutationOptions = {
  operatorReason: string
  idempotencyKey?: string | undefined
}

export type ModelPoolReplaceInput = {
  version: number
  entries: AdminModelPoolEntry[]
}

export type CatalogModelLister = () => Promise<AdminPoolCatalogModel[]>

export interface AdminModelPoolResource {
  get: (sku: string) => Promise<AdminModelPool>
  replace: (
    sku: string,
    input: ModelPoolReplaceInput,
    options: ModelPoolMutationOptions,
  ) => Promise<AdminModelPoolSaveResult>
  listCatalogModels: () => Promise<AdminPoolCatalogModel[]>
}

export function createAdminModelPoolResource(
  listCatalogModels: CatalogModelLister = defaultCatalogModelLister,
): AdminModelPoolResource {
  return {
    get: fetchAdminModelPool,
    replace: replaceAdminModelPool,
    listCatalogModels: () =>
      listCatalogModels().catch((error) => {
        throw withPoolCopy(error, '加载模型目录失败')
      }),
  }
}

async function defaultCatalogModelLister(): Promise<AdminPoolCatalogModel[]> {
  const items = await createAdminGatewayResource().catalog.list()
  return items.map((item) => ({ id: item.id, name: item.name }))
}

export async function fetchAdminModelPool(sku: string): Promise<AdminModelPool> {
  try {
    const body = await getControlCatalogModelPool(sku)
    return normalizePool(requireContractData<AdminModelPoolWire>(body, '加载模型池失败'), sku)
  } catch (error) {
    throw withPoolCopy(error, '加载模型池失败')
  }
}

export async function replaceAdminModelPool(
  sku: string,
  input: ModelPoolReplaceInput,
  options: ModelPoolMutationOptions,
): Promise<AdminModelPoolSaveResult> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  try {
    const body = await replaceControlCatalogModelPool(
      sku,
      {
        expected_version: input.version,
        entries: input.entries.map((entry) => ({
          model_id: entry.modelId,
          enabled: entry.enabled,
          resolutions: [...entry.resolutions],
        })),
        reason: operatorReason,
      },
      { headers: mutationHeaders(options, operatorReason) },
    )
    const data = requireContractData<AdminModelPoolSaveWire>(body, '保存模型池失败')
    return { ...normalizePool(data, sku), changed: data.changed === true }
  } catch (error) {
    throw withPoolCopy(error, '保存模型池失败')
  }
}

type AdminModelPoolWire = {
  service: string
  sku: string
  version: number
  entries: Array<{
    model_id: string
    enabled: boolean
    resolutions: unknown
  }>
}

type AdminModelPoolSaveWire = AdminModelPoolWire & {
  changed: boolean
}

const POOL_RESOLUTIONS: readonly PoolResolution[] = OUTPUT_RESOLUTION_OPTIONS

function normalizePool(wire: AdminModelPoolWire, fallbackSku: string): AdminModelPool {
  return {
    service: wire.service,
    sku: wire.sku || fallbackSku,
    version: wire.version,
    entries: (wire.entries ?? []).map((entry) => ({
      modelId: entry.model_id,
      enabled: entry.enabled === true,
      resolutions: normalizeResolutions(entry.resolutions),
    })),
  }
}

function normalizeResolutions(raw: unknown): PoolResolution[] {
  if (!Array.isArray(raw)) return []
  const seen = new Set<PoolResolution>()
  for (const value of raw) {
    const parsed = parseOutputResolution(value)
    if (parsed !== null) seen.add(parsed)
  }
  return POOL_RESOLUTIONS.filter((resolution) => seen.has(resolution))
}

function mutationHeaders(options: ModelPoolMutationOptions, operatorReason: string) {
  return buildContractRequestHeaders({
    requestId: newRequestId(),
    idempotencyKey: options.idempotencyKey ?? newRequestId(),
    operatorReason,
  })
}

function requireOperatorReason(reason: string): string {
  const trimmed = reason.trim()
  if (!trimmed) throw poolError('control_reason_required', '需要填写操作原因')
  return trimmed
}

function poolError(code: string, message: string): Error & { code?: string | undefined } {
  const error = new Error(message) as Error & { code?: string | undefined }
  error.code = code
  return error
}

// withPoolCopy maps control error codes to operator-facing copy.
// Branch on code, never on message text. `conflict` is the pool
// version race (expected_version missed): the caller must refresh and
// re-apply, never silently overwrite.
function withPoolCopy(error: unknown, fallback: string): Error {
  const code = readErrorCode(error)
  if (code === 'conflict' || code === 409) {
    return poolError('conflict', '模型池已被更新，请刷新后重试')
  }
  if (code === 'idempotency_conflict') {
    return poolError(code, '操作已提交，请勿重复操作')
  }
  if (code === 'control_reason_required') {
    return poolError(code, '需要填写操作原因')
  }
  if (code === 'control_not_found' || code === 'not_found' || code === 404) {
    return poolError('control_not_found', '功能不存在')
  }
  if (error instanceof Error) return error
  return poolError('control_failed', fallback)
}

function readErrorCode(error: unknown): string | number | undefined {
  if (error && typeof error === 'object' && 'code' in error) {
    const code = (error as { code?: unknown }).code
    if (typeof code === 'string' || typeof code === 'number') return code
  }
  if (error && typeof error === 'object' && 'status' in error) {
    const status = (error as { status?: unknown }).status
    if (status === 404 || status === 409) return status
  }
  return undefined
}
