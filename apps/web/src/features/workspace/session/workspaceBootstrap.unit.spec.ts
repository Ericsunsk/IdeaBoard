import { beforeEach, describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { queryClient } from '../../../core/query/queryClient'
import { fetchWorkspaceBootstrap } from './workspaceBootstrap'
import { billingAccountQueryKeys, creditsQueryKeys } from '../../account'
import { SITE_BRANDING_QUERY_KEY } from '../../../core/composables/useSiteBranding'

const BOOTSTRAP_PATH = '/api/v1/workspaces/ws-100/bootstrap'

describe('workspaceBootstrap', () => {
  beforeEach(() => {
    queryClient.clear()
  })

  it('fetches consolidated bootstrap data and seeds query client cache', async () => {
    useMswHandlers(
      http.get(BOOTSTRAP_PATH, () =>
        HttpResponse.json({
          data: {
            workspace: {
              id: 'ws-100',
              name: 'Test Workspace',
              created_at: '2026-10-19T08:00:00Z',
            },
            billing_account: {
              billing_account_id: 'ba-100',
              workspace_id: 'ws-100',
              owner_type: 'workspace',
              consumption_scope: 'account',
              can_manage_member_quotas: true,
              // eslint-disable-next-line governance/boolean-prefix -- Wire contract field
              shared: false,
            },
            wallet: {
              billing_account_id: 'ba-100',
              public_id: 'ba-100',
              display_name: 'Personal Wallet',
              available_credits: 500,
              reserved_credits: 0,
            },
            branding: {
              project_name: 'IdeaBoard Pro',
              logo_url: 'https://example.com/logo.png',
            },
          },
        }),
      ),
    )

    const response = await fetchWorkspaceBootstrap('ws-100', 'usr-test')
    expect(response.workspace.id).toBe('ws-100')
    expect(response.billing_account.billing_account_id).toBe('ba-100')
    expect(response.wallet.available_credits).toBe(500)

    // Check seeded billing account
    const billingCache = queryClient.getQueryData(
      billingAccountQueryKeys.detail('ws-100', 'usr-test'),
    )
    expect(billingCache).toMatchObject({
      billingAccountId: 'ba-100',
      workspaceId: 'ws-100',
      identity: 'usr-test',
    })

    // Check seeded wallet
    const walletCache = queryClient.getQueryData(
      creditsQueryKeys.wallet('ws-100', 'ba-100', 'usr-test'),
    )
    expect(walletCache).toMatchObject({
      available_credits: 500,
      billing_account_id: 'ba-100',
    })

    // Check seeded branding
    const brandingCache = queryClient.getQueryData(SITE_BRANDING_QUERY_KEY)
    expect(brandingCache).toMatchObject({
      data: {
        project_name: 'IdeaBoard Pro',
        logo_url: 'https://example.com/logo.png',
      },
    })
  })
})
