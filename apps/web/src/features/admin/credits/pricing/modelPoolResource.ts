import {
  getControlCatalogModelPool,
  replaceControlCatalogModelPool,
} from '../../../../shared/api/generated-query/control/control-billing/control-billing'
import { requireContractData } from '../../../../shared/api/contractKernel'
import { createAdminGatewayResource } from '../../models/infrastructure/adminGatewayResource'
import {
  buildControlMutationHeaders,
  requireOperatorReason,
} from '../../shared/lib/controlMutation'
import { withControlCopy } from '../../shared/lib/controlCopy'
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
        throw withControlCopy(error, '加载模型目录失败', {
          conflict: '模型池已被更新，请刷新后重试',
          control_not_found: '功能不存在',
          not_found: '功能不存在',
          404: '功能不存在',
        })
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
    throw withControlCopy(error, '加载模型池失败', {
      conflict: '模型池已被更新，请刷新后重试',
      control_not_found: '功能不存在',
      not_found: '功能不存在',
      404: '功能不存在',
    })
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
      { headers: buildControlMutationHeaders(options) },
    )
    const data = requireContractData<AdminModelPoolSaveWire>(body, '保存模型池失败')
    return { ...normalizePool(data, sku), changed: data.changed === true }
  } catch (error) {
    throw withControlCopy(error, '保存模型池失败', {
      conflict: '模型池已被更新，请刷新后重试',
      control_not_found: '功能不存在',
      not_found: '功能不存在',
      404: '功能不存在',
    })
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
