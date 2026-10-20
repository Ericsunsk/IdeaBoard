// Display-name rename resource contract.
//
// MSW-level contract: exercises the real generated `updateUser` transport
// (`PATCH /api/v1/user`) against the AGREED envelope, so backend drift
// surfaces here instead of in UI. No generated-query mocks by design (see
// checkTestGuardrails).
import { describe, expect, test } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { normalizeDisplayName, renameDisplayName, validateDisplayName } from './resource'

const AGREED_USER = {
  data: {
    id: 'usr_renamed1',
    display_name: 'Renamed',
    role: 'user',
    avatar: '',
    status: 1,
  },
}

describe('display-name rename', () => {
  test('renames through PATCH with a display-name-only body and parses the envelope', async () => {
    const seen: Array<{ body: unknown; requestId: string | null }> = []
    useMswHandlers(
      http.patch('/api/v1/user', async ({ request }) => {
        seen.push({ body: await request.json(), requestId: request.headers.get('request-id') })
        return HttpResponse.json(AGREED_USER)
      }),
    )

    const updated = await renameDisplayName('  Renamed  ')

    expect(updated).toMatchObject({ public_id: 'usr_renamed1', display_name: 'Renamed' })
    expect(seen).toHaveLength(1)
    expect(seen[0]?.body).toEqual({ data: { display_name: 'Renamed' } })
    expect(seen[0]?.requestId).toBeTruthy()
  })

  test('validates the display name before touching the network', async () => {
    await expect(renameDisplayName('   ')).rejects.toThrow('Display name cannot be empty.')
    await expect(renameDisplayName('x'.repeat(101))).rejects.toThrow(
      `Display name must be no more than 100 characters.`,
    )
  })

  test('surfaces the server machine code instead of swallowing it', async () => {
    useMswHandlers(
      http.patch('/api/v1/user', () =>
        HttpResponse.json(
          { error: { code: 'display_name_invalid', message: 'No', request_id: 'r1' } },
          { status: 422 },
        ),
      ),
    )

    await expect(renameDisplayName('Nope')).rejects.toMatchObject({
      code: 'display_name_invalid',
    })
  })

  test('rejects an updated-user envelope without a public identifier', async () => {
    useMswHandlers(
      http.patch('/api/v1/user', () => HttpResponse.json({ data: { display_name: 'Renamed' } })),
    )

    await expect(renameDisplayName('Renamed')).rejects.toThrow(
      'Could not update your display name.',
    )
  })
})

describe('display-name validation', () => {
  test('trims surrounding whitespace before validating', () => {
    expect(normalizeDisplayName('  Ada  ')).toBe('Ada')
    expect(validateDisplayName('  Ada  ')).toBeNull()
  })

  test('rejects empty and overlong names with local copy', () => {
    expect(validateDisplayName('   ')).toBe('Display name cannot be empty.')
    expect(validateDisplayName('x'.repeat(101))).toBe(
      `Display name must be no more than 100 characters.`,
    )
  })
})
