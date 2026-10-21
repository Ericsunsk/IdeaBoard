// Workspace resource contract (issue #801).
//
// MSW-level contract: exercises the real Public generated transport against
// the AGREED envelopes, so backend drift surfaces here instead of in UI.
// No generated-query mocks by design (see checkTestGuardrails).
import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import {
  buildWorkspaceCreateHeaders,
  classifyWorkspaceAccessError,
  deleteWorkspace,
  fetchAllWorkspaces,
  fetchOrganizations,
  provisionOrganization,
  fetchPrincipal,
  fetchWorkspacePage,
  normalizeWorkspaceListParams,
  parsePrincipal,
  parseWorkspaceItem,
  parseWorkspacePage,
  provisionWorkspace,
  renameWorkspace,
  workspaceApiUrls,
  WORKSPACE_NAME_MAX_LENGTH,
  WORKSPACE_PAGE_SIZE_DEFAULT,
  WORKSPACE_PAGE_SIZE_MAX,
} from './workspaceResource'

/**
 * Delete-confirmation wire shape.
 * @externalContract Public API DeletedEnvelope: consumed verbatim from the
 * generated contract (`DELETE /api/v1/workspaces/{id}` renders
 * `{ data: { deleted: true } }`).
 */
type DeletedConfirmation = { data: { deleted: boolean } }

const AGREED_PAGE = {
  data: [
    {
      public_id: 'ws_a1',
      name: 'Alpha',
      organization_id: null,
      is_default: true,
    },
    {
      public_id: 'ws_b2',
      name: 'Beta',
      organization_id: 'org_c3',
      is_default: false,
    },
  ],
  meta: { next_cursor: 'cursor2', has_more: true },
}

describe('workspace list envelope', () => {
  it('maps the agreed snake_case page to camelCase domain items', async () => {
    useMswHandlers(http.get('/api/v1/workspaces', () => HttpResponse.json(AGREED_PAGE)))

    const page = await fetchWorkspacePage()

    expect(page.items).toEqual([
      {
        publicId: 'ws_a1',
        name: 'Alpha',
        organizationId: null,
        isDefault: true,
      },
      {
        publicId: 'ws_b2',
        name: 'Beta',
        organizationId: 'org_c3',
        isDefault: false,
      },
    ])
    expect(page).toMatchObject({ nextCursor: 'cursor2', hasMore: true })
  })

  it('tolerates the kernel stub shape until the backend slice lands', () => {
    const page = parseWorkspacePage({
      data: [{ id: 'ws_stub1', created_at: '2026-09-01T00:00:00Z' }],
      meta: { next_cursor: null, has_more: false },
    })

    expect(page.items[0]).toMatchObject({ publicId: 'ws_stub1', name: '' })
    expect(page).toMatchObject({ nextCursor: null, hasMore: false })
  })

  it('walks cursor pagination and clamps page size to the agreed bounds', async () => {
    const seenUrls: string[] = []
    useMswHandlers(
      http.get('/api/v1/workspaces', ({ request }) => {
        seenUrls.push(request.url)
        const cursor = new URL(request.url).searchParams.get('cursor')
        if (cursor === 'cursor2') {
          return HttpResponse.json({
            data: [{ public_id: 'ws_c3', name: 'Gamma' }],
            meta: { next_cursor: null, has_more: false },
          })
        }
        return HttpResponse.json(AGREED_PAGE)
      }),
    )

    const items = await fetchAllWorkspaces({ pageSize: 999, maxPages: 5 })

    expect(items.map((item) => item.publicId)).toEqual(['ws_a1', 'ws_b2', 'ws_c3'])
    expect(seenUrls[0]).toContain(`page_size=${WORKSPACE_PAGE_SIZE_MAX}`)
    expect(seenUrls[1]).toContain('cursor=cursor2')
  })

  it('defaults to page size 20 and rejects opaque-ID violations', () => {
    expect(normalizeWorkspaceListParams()).toMatchObject({
      page_size: WORKSPACE_PAGE_SIZE_DEFAULT,
    })
    expect(normalizeWorkspaceListParams({ pageSize: 0 }).page_size).toBe(1)

    for (const raw of [
      { public_id: '123', name: 'Legacy' },
      { public_id: 'org_c3', name: 'Wrong prefix' },
      { public_id: 'ws_', name: 'Empty payload' },
      { id: 42, name: 'Numeric' },
    ]) {
      expect(() => parseWorkspaceItem(raw)).toThrow()
    }
    expect(() => parseWorkspacePage({ data: [{ public_id: 'usr_u1' }] })).toThrow()
    expect(() => parseWorkspacePage({ bogus: true })).toThrow()
  })

  it('propagates Request-Id on reads', async () => {
    let requestId: string | null = null
    useMswHandlers(
      http.get('/api/v1/workspaces', ({ request }) => {
        requestId = request.headers.get('request-id')
        return HttpResponse.json(AGREED_PAGE)
      }),
    )

    await fetchWorkspacePage(undefined, { requestId: 'req_workspace_1' })

    expect(requestId).toBe('req_workspace_1')
  })

  it('keeps hidden (404) distinct from forbidden (403)', async () => {
    useMswHandlers(
      http.get('/api/v1/workspaces', () =>
        HttpResponse.json(
          {
            error: {
              code: 'workspace_not_found',
              message: 'Not found',
              request_id: 'req_404',
            },
          },
          { status: 404 },
        ),
      ),
    )

    const hidden = await fetchWorkspacePage().catch((error: unknown) => error)
    expect(classifyWorkspaceAccessError(hidden)).toBe('hidden')

    expect(classifyWorkspaceAccessError({ status: 403 })).toBe('forbidden')
    expect(classifyWorkspaceAccessError({ status: 500 })).toBeNull()
    expect(classifyWorkspaceAccessError(new Error('boom'))).toBeNull()
  })

  it('redirects to login by default on a list 401', async () => {
    localStorage.setItem('user', JSON.stringify({ public_id: 'ws_owner1' }))
    window.history.pushState({}, '', '/user/boards')
    useMswHandlers(
      http.get('/api/v1/workspaces', () =>
        HttpResponse.json({ code: 401, msg: '' }, { status: 401 }),
      ),
    )

    await expect(fetchWorkspacePage()).rejects.toThrow()

    expect(localStorage.getItem('user')).toBeNull()
    expect(window.location.pathname).toBe('/user/login')
    window.history.pushState({}, '', '/')
  })

  it('returns a silent list 401 without navigating', async () => {
    localStorage.setItem('user', JSON.stringify({ public_id: 'ws_owner1' }))
    window.history.pushState({}, '', '/user/boards')
    useMswHandlers(
      http.get('/api/v1/workspaces', () =>
        HttpResponse.json({ code: 401, msg: '' }, { status: 401 }),
      ),
    )

    await expect(fetchWorkspacePage(undefined, { authFailure: 'silent' })).rejects.toThrow()

    expect(localStorage.getItem('user')).toBeNull()
    expect(window.location.pathname).toBe('/user/boards')
    window.history.pushState({}, '', '/')
  })
})

describe('workspace creation', () => {
  it('injects Idempotency-Key and Request-Id on every create', async () => {
    const seen: Array<{ idempotencyKey: string | null; requestId: string | null; body: unknown }> =
      []
    useMswHandlers(
      http.post('/api/v1/workspaces', async ({ request }) => {
        seen.push({
          idempotencyKey: request.headers.get('idempotency-key'),
          requestId: request.headers.get('request-id'),
          body: await request.json(),
        })
        return HttpResponse.json({
          data: {
            public_id: 'ws_new1',
            name: 'New',
            organization_id: null,
            billing_account_id: 'ba_personal_1',
          },
        })
      }),
    )

    const created = await provisionWorkspace(
      { name: 'New' },
      { idempotencyKey: 'workspace-create:retry-1', requestId: 'req_create_1' },
    )

    expect(created).toMatchObject({
      publicId: 'ws_new1',
      name: 'New',
      billingAccountId: 'ba_personal_1',
    })
    expect(seen).toHaveLength(1)
    expect(seen[0]?.idempotencyKey).toBe('workspace-create:retry-1')
    expect(seen[0]?.requestId).toBe('req_create_1')
    expect(seen[0]?.body).toEqual({ data: { name: 'New' } })
  })

  it('sends an explicit shared account or a fresh independent flag', async () => {
    const seen: Array<{ body: unknown }> = []
    useMswHandlers(
      http.post('/api/v1/workspaces', async ({ request }) => {
        seen.push({ body: await request.json() })
        return HttpResponse.json({
          data: { public_id: 'ws_new2', name: 'Biz', organization_id: null },
        })
      }),
    )

    await provisionWorkspace(
      { name: 'Biz', billingAccountId: 'ba_personal_1' },
      { idempotencyKey: 'workspace-create:shared-1' },
    )
    await provisionWorkspace(
      { name: 'Solo', isNewBillingAccount: true },
      { idempotencyKey: 'workspace-create:new-1' },
    )

    expect(seen[0]?.body).toEqual({
      data: { name: 'Biz', billing_account_id: 'ba_personal_1' },
    })
    // eslint-disable-next-line governance/boolean-prefix -- Orval wire field mirrors openapi.public.yaml.
    expect(seen[1]?.body).toEqual({ data: { name: 'Solo', new_billing_account: true } })
  })

  it('generates a fresh idempotency key per new intent', () => {
    const first = buildWorkspaceCreateHeaders()
    const second = buildWorkspaceCreateHeaders()

    expect(first['Idempotency-Key']).toMatch(/^workspace-create:/)
    expect(second['Idempotency-Key']).toMatch(/^workspace-create:/)
    expect(first['Idempotency-Key']).not.toBe(second['Idempotency-Key'])
    expect(first['Request-Id']).toBeTruthy()
  })

  it('validates name and organization inputs before touching the network', async () => {
    await expect(provisionWorkspace({ name: '   ' })).rejects.toThrow()
    await expect(
      provisionWorkspace({ name: 'x'.repeat(WORKSPACE_NAME_MAX_LENGTH + 1) }),
    ).rejects.toThrow()
    await expect(provisionWorkspace({ name: 'Ok', organizationId: 'nope' })).rejects.toThrow()
    await expect(provisionWorkspace({ name: 'Ok', billingAccountId: 'nope' })).rejects.toThrow()
    await expect(
      provisionWorkspace({
        name: 'Ok',
        billingAccountId: 'ba_personal_1',
        isNewBillingAccount: true,
      }),
    ).rejects.toThrow()
    await expect(
      provisionWorkspace({
        name: 'Ok',
        organizationId: 'org_c3',
        billingAccountId: 'ba_personal_1',
      }),
    ).rejects.toThrow()
  })

  it('parses the creation-time billing account without breaking legacy payloads', () => {
    expect(
      parseWorkspaceItem({
        public_id: 'ws_b1',
        name: 'Biz',
        organization_id: null,
        billing_account_id: 'ba_personal_1',
      }),
    ).toMatchObject({ publicId: 'ws_b1', billingAccountId: 'ba_personal_1' })
    expect(parseWorkspaceItem({ public_id: 'ws_l1', name: 'Legacy' })).toMatchObject({
      publicId: 'ws_l1',
    })
    expect(() =>
      parseWorkspaceItem({
        public_id: 'ws_b2',
        name: 'Bad',
        billing_account_id: 'nope',
      }),
    ).toThrow()
  })
})

describe('workspace rename', () => {
  it('renames through PATCH with a name-only body and parses the envelope', async () => {
    const seen: Array<{ body: unknown; requestId: string | null }> = []
    useMswHandlers(
      http.patch('/api/v1/workspaces/:workspaceId', async ({ request }) => {
        seen.push({ body: await request.json(), requestId: request.headers.get('request-id') })
        return HttpResponse.json({
          data: {
            public_id: 'ws_a1',
            name: 'Renamed',
            organization_id: null,
          },
        })
      }),
    )

    const updated = await renameWorkspace('ws_a1', { name: '  Renamed  ' })

    expect(updated).toMatchObject({ publicId: 'ws_a1', name: 'Renamed' })
    expect(seen).toHaveLength(1)
    expect(seen[0]?.body).toEqual({ data: { name: 'Renamed' } })
    expect(seen[0]?.requestId).toBeTruthy()
  })

  it('validates the workspace id and name before touching the network', async () => {
    await expect(renameWorkspace('123', { name: 'Ok' })).rejects.toThrow()
    await expect(renameWorkspace('ws_a1', { name: '   ' })).rejects.toThrow()
    await expect(
      renameWorkspace('ws_a1', { name: 'x'.repeat(WORKSPACE_NAME_MAX_LENGTH + 1) }),
    ).rejects.toThrow()
  })

  it('surfaces the server machine code instead of swallowing it', async () => {
    useMswHandlers(
      http.patch('/api/v1/workspaces/:workspaceId', () =>
        HttpResponse.json(
          { error: { code: 'workspace_default_protected', message: 'No', request_id: 'r1' } },
          { status: 403 },
        ),
      ),
    )

    await expect(renameWorkspace('ws_d1', { name: 'Nope' })).rejects.toMatchObject({
      code: 'workspace_default_protected',
    })
  })
})

describe('workspace default marker', () => {
  it('parses is_default and defaults absent markers to false', () => {
    expect(
      parseWorkspaceItem({ public_id: 'ws_d1', name: 'Default', is_default: true }),
    ).toMatchObject({ publicId: 'ws_d1', isDefault: true })
    expect(
      parseWorkspaceItem({ public_id: 'ws_p1', name: 'Plain', is_default: false }),
    ).toMatchObject({ publicId: 'ws_p1', isDefault: false })
    expect(parseWorkspaceItem({ public_id: 'ws_l1', name: 'Legacy' })).toMatchObject({
      publicId: 'ws_l1',
      isDefault: false,
    })
  })
})

describe('workspace deletion', () => {
  it('deletes through DELETE with Idempotency-Key and Request-Id', async () => {
    const seen: Array<{
      workspaceId: string
      idempotencyKey: string | null
      requestId: string | null
    }> = []
    useMswHandlers(
      http.delete('/api/v1/workspaces/:workspaceId', ({ params, request }) => {
        seen.push({
          workspaceId: String(params.workspaceId),
          idempotencyKey: request.headers.get('idempotency-key'),
          requestId: request.headers.get('request-id'),
        })
        return HttpResponse.json({ data: { deleted: true } } satisfies DeletedConfirmation)
      }),
    )

    await expect(
      deleteWorkspace('ws_a1', {
        idempotencyKey: 'workspace-delete:retry-1',
        requestId: 'req_delete_1',
      }),
    ).resolves.toBeUndefined()

    expect(seen).toEqual([
      {
        workspaceId: 'ws_a1',
        idempotencyKey: 'workspace-delete:retry-1',
        requestId: 'req_delete_1',
      },
    ])
  })

  it('mints a fresh workspace-delete idempotency key per attempt', async () => {
    const seenKeys: Array<string | null> = []
    useMswHandlers(
      http.delete('/api/v1/workspaces/:workspaceId', ({ request }) => {
        seenKeys.push(request.headers.get('idempotency-key'))
        return HttpResponse.json({ data: { deleted: true } } satisfies DeletedConfirmation)
      }),
    )

    await deleteWorkspace('ws_a1')
    await deleteWorkspace('ws_b2')

    expect(seenKeys).toHaveLength(2)
    expect(seenKeys[0]).toMatch(/^workspace-delete:/)
    expect(seenKeys[1]).toMatch(/^workspace-delete:/)
    expect(seenKeys[0]).not.toBe(seenKeys[1])
  })

  it('rejects a non-deleted confirmation instead of resolving', async () => {
    useMswHandlers(
      http.delete('/api/v1/workspaces/:workspaceId', () =>
        HttpResponse.json({ data: { deleted: false } } satisfies DeletedConfirmation),
      ),
    )

    await expect(deleteWorkspace('ws_a1')).rejects.toThrow()
  })

  it('validates the workspace id before touching the network', async () => {
    await expect(deleteWorkspace('123')).rejects.toThrow()
    await expect(deleteWorkspace('org_c3')).rejects.toThrow()
  })

  it('surfaces the server machine code instead of swallowing it', async () => {
    useMswHandlers(
      http.delete('/api/v1/workspaces/:workspaceId', () =>
        HttpResponse.json(
          { error: { code: 'workspace_default_protected', message: 'No', request_id: 'r1' } },
          { status: 403 },
        ),
      ),
    )

    await expect(deleteWorkspace('ws_d1')).rejects.toMatchObject({
      code: 'workspace_default_protected',
    })
  })
})

describe('principal envelope', () => {
  it('maps the agreed principal shape to camelCase', async () => {
    useMswHandlers(
      http.get('/api/v1/user', () =>
        HttpResponse.json({
          data: { public_id: 'usr_u1', display_name: 'Ada', avatar: 'https://cdn.example/a.png' },
        }),
      ),
    )

    await expect(fetchPrincipal()).resolves.toEqual({
      publicId: 'usr_u1',
      displayName: 'Ada',
      avatar: 'https://cdn.example/a.png',
    })
  })

  it('tolerates the stub User shape and rejects non-user IDs', () => {
    expect(parsePrincipal({ data: { id: 'usr_stub9' } })).toEqual({
      publicId: 'usr_stub9',
      displayName: '',
    })
    expect(() => parsePrincipal({ data: { id: 'ws_a1' } })).toThrow()
    expect(() => parsePrincipal({ data: null })).toThrow()
  })
})

describe('workspace URLs', () => {
  it('builds canonical agreed-contract URLs without legacy user routes', () => {
    const urls = [
      workspaceApiUrls.user(),
      workspaceApiUrls.workspaces(),
      workspaceApiUrls.workspaces({ cursor: 'c 1', pageSize: 50 }),
      workspaceApiUrls.workspaceDetail('ws_a1'),
      workspaceApiUrls.organizations(),
      workspaceApiUrls.organizationDetail('org_c3'),
      workspaceApiUrls.organizationWorkspaces('org_c3'),
    ]

    for (const url of urls) {
      expect(url).toContain('/api/v1/')
      expect(url).not.toContain('/api/user')
      expect(url).not.toContain('/api/v1/me')
    }
    expect(workspaceApiUrls.workspaces({ cursor: 'c 1', pageSize: 50 })).toContain(
      'cursor=c+1&page_size=50',
    )
    expect(workspaceApiUrls.workspaceDetail('ws a/1')).toContain('ws%20a%2F1')
  })
})

describe('organization account setup', () => {
  it('walks all organizations so an organization without workspaces remains selectable', async () => {
    useMswHandlers(
      http.get('/api/v1/organizations', ({ request }) => {
        const cursor = new URL(request.url).searchParams.get('cursor')
        return HttpResponse.json(
          cursor === 'next'
            ? {
                data: [
                  { id: 'org_new', name: 'No workspaces yet', created_at: '2026-10-22T00:00:00Z' },
                ],
                meta: { has_more: false, next_cursor: null },
              }
            : {
                data: [
                  { id: 'org_existing', name: 'Existing', created_at: '2026-10-22T00:00:00Z' },
                ],
                meta: { has_more: true, next_cursor: 'next' },
              },
        )
      }),
    )
    expect(await fetchOrganizations()).toEqual([
      { publicId: 'org_existing', name: 'Existing' },
      { publicId: 'org_new', name: 'No workspaces yet' },
    ])
  })
  it('creates an organization with a caller-pinned retry key', async () => {
    const seenKeys: (string | null)[] = []
    useMswHandlers(
      http.post('/api/v1/organizations', async ({ request }) => {
        seenKeys.push(request.headers.get('Idempotency-Key'))
        expect(await request.json()).toEqual({ data: { name: 'Acme' } })
        return HttpResponse.json(
          { data: { id: 'org_acme', name: 'Acme', created_at: '2026-10-22T00:00:00Z' } },
          { status: 201 },
        )
      }),
    )
    expect(await provisionOrganization(' Acme ', 'organization-create:attempt')).toEqual({
      publicId: 'org_acme',
      name: 'Acme',
    })
    await provisionOrganization('Acme', 'organization-create:attempt')
    expect(seenKeys).toEqual(['organization-create:attempt', 'organization-create:attempt'])
  })
  it('fails closed if organization pagination stops advancing', async () => {
    useMswHandlers(
      http.get('/api/v1/organizations', () =>
        HttpResponse.json({ data: [], meta: { has_more: true, next_cursor: 'same' } }),
      ),
    )
    await expect(fetchOrganizations()).rejects.toThrow('Invalid organization cursor')
  })
})
