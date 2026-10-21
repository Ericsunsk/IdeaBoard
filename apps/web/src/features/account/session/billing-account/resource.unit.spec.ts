import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { WorkspaceBillingAccount } from '../../../../api/generated-query/public/model'
import { fetchBillingAccount, billingAccountQueryKeys } from './resource'
import { WORKSPACE_SELECTION_REQUIRED_CODE } from '../workspaceBilling'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../../api/client', () => ({
  api: { request: transport.request },
}))

const WORKSPACE_ID = 'ws_123'

describe('billingAccountResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('reads the stable billing account behind the workspace', async () => {
    // The wire shape is the generated external contract: `shared` stays
    // the backend field name here, the scope maps it to `isShared`.
    const body: WorkspaceBillingAccount = {
      billing_account_id: 'ba_1',
      workspace_id: WORKSPACE_ID,
      owner_type: 'workspace',
      consumption_scope: 'account',
      shared: true,
    }
    transport.request.mockResolvedValue({ data: body })

    const controller = new AbortController()
    await expect(fetchBillingAccount(WORKSPACE_ID, controller.signal)).resolves.toEqual({
      billingAccountId: 'ba_1',
      workspaceId: WORKSPACE_ID,
      isShared: true,
      consumptionScope: 'account',
    })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/billing-account`,
      expect.objectContaining({ method: 'GET', signal: controller.signal }),
    )
    expect(billingAccountQueryKeys.detail(WORKSPACE_ID)).toEqual([
      'workspace',
      'billing-account',
      'detail',
      WORKSPACE_ID,
      '',
    ])
  })

  it('defaults an unshared account to isShared=false', async () => {
    const body: WorkspaceBillingAccount = {
      billing_account_id: 'ba_2',
      workspace_id: WORKSPACE_ID,
      owner_type: 'organization',
      consumption_scope: 'member',
    }
    transport.request.mockResolvedValue({ data: body })

    await expect(fetchBillingAccount(WORKSPACE_ID)).resolves.toMatchObject({
      billingAccountId: 'ba_2',
      isShared: false,
    })
  })

  it('rejects an account response without a valid authorization scope', async () => {
    transport.request.mockResolvedValue({
      data: { billing_account_id: 'ba_1', workspace_id: WORKSPACE_ID, owner_type: 'workspace' },
    })
    await expect(fetchBillingAccount(WORKSPACE_ID)).rejects.toThrow()
  })

  it('fails closed with guidance when no workspace is selected', async () => {
    await expect(fetchBillingAccount('')).rejects.toMatchObject({
      code: WORKSPACE_SELECTION_REQUIRED_CODE,
    })
    expect(transport.request).not.toHaveBeenCalled()
  })

  it('surfaces contract error feedback instead of manufacturing an account', async () => {
    transport.request.mockResolvedValue({
      error: { code: 'service_unavailable', message: '账单服务暂不可用' },
    })

    await expect(fetchBillingAccount(WORKSPACE_ID)).rejects.toThrow('账单服务暂不可用')
  })
})
