import {
  getControlCatalogModelPool,
  replaceControlCatalogModelPool,
} from '../../../../../api/generated-query/control/control-billing/control-billing'
import { requireContractData } from '../../../../../api/contractKernel'
import { createAdminGatewayResource } from '../../../models/resource'
import { buildRoutineControlMutationHeaders, type ControlMutationOptions } from '../../../shared'
import { withControlCopy } from '../../../shared'
import {
  OUTPUT_RESOLUTION_OPTIONS,
  parseOutputResolution,
} from '../../../../../core/contracts/generationContract'
import type {
  AdminModelPool,
  AdminModelPoolEntry,
  AdminModelPoolSaveResult,
  AdminPoolCatalogModel,
  PoolResolution,
} from '../../core/pricing/billableCatalogTypes'

const MODEL_POOL_CONTROL_MESSAGES = {
  conflict: '模型池已被更新，请刷新后重试',
  control_not_found: '功能不存在',
  not_found: '功能不存在',
  404: '功能不存在',
} as const

// Admin Service Model Pool boundary (#849). Reads and atomic replaces go
// through the Control Plane billing pool operations with the contract
// envelope (`{data}`); the PUT is a routine config write (backend
// ReasonOptional): no confirm dialog, the audit records an empty reason
// (mirrored into the body `reason` field), while the header-only
// Idempotency-Key stays required. Pages and dialogs consume this module,
// never generated clients directly.
//
// The add-from-catalog list reuses the existing Model catalog resource
// (no second catalog): entries are matched by catalog id. The lister is
// injectable so tests pass a fake without MSW.
type ModelPoolMutationOptions = ControlMutationOptions

type ModelPoolReplaceInput = {
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
        throw withControlCopy(error, '加载模型目录失败', MODEL_POOL_CONTROL_MESSAGES)
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
    throw withControlCopy(error, '加载模型池失败', MODEL_POOL_CONTROL_MESSAGES)
  }
}

export async function replaceAdminModelPool(
  sku: string,
  input: ModelPoolReplaceInput,
  options: ModelPoolMutationOptions,
): Promise<AdminModelPoolSaveResult> {
  // Routine config write (backend ReasonOptional): the reason may be empty
  // (audit records it as-is) while Idempotency-Key stays required.
  const operatorReason = options.operatorReason.trim()
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
      {
        headers: buildRoutineControlMutationHeaders(options),
      },
    )
    const data = requireContractData<AdminModelPoolSaveWire>(body, '保存模型池失败')
    return { ...normalizePool(data, sku), changed: data.changed === true }
  } catch (error) {
    throw withControlCopy(error, '保存模型池失败', MODEL_POOL_CONTROL_MESSAGES)
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
