import { describe, expect, it } from 'vitest'
import { buildListControlUsersParams, mapControlUserToListItem } from './resource'
import type { ControlUser } from '../../../../api/generated-query/control/model'

describe('admin users control resource mapping', () => {
  it('maps control users onto the legacy row shape with usr_ identity', () => {
    const item = mapControlUserToListItem(makeControlUser())

    expect(item).toEqual({
      public_id: 'usr_abc123',
      email: 'op@example.com',
      providers: ['email'],
      display_name: 'Operator',
      avatar: '/a.png',
      role: 'admin',
      status: 1,
      created_at: '2026-09-23T00:00:00Z',
      updated_at: '2026-09-23T00:00:00Z',
      available_credits: 42,
      last_login_ip: '203.0.113.7',
      last_login_country: 'US',
    })
  })

  it('defaults absent display fields without dropping the row', () => {
    const item = mapControlUserToListItem({
      id: 'usr_minimal1',
      display_name: '',
      role: 'user',
      status: 0,
      created_at: '',
      updated_at: '',
    })

    expect(item.public_id).toBe('usr_minimal1')
    expect(item.email).toBe('')
    expect(item.providers).toEqual([])
    expect(item.available_credits).toBe(0)
    expect(item.last_login_ip).toBe('')
  })

  it('normalizes unknown roles to user instead of leaking them', () => {
    const item = mapControlUserToListItem({
      ...makeControlUser(),
      role: 'superadmin' as ControlUser['role'],
    })

    expect(item.role).toBe('user')
  })

  it('builds cursor list params without ever emitting page numbers', () => {
    expect(buildListControlUsersParams({ pageSize: 10 }, undefined)).toEqual({
      page_size: 10,
    })
    expect(
      buildListControlUsersParams(
        { keyword: '  alice  ', role: 'admin', status: 1, pageSize: 200 },
        'cursor-abc',
      ),
    ).toEqual({
      cursor: 'cursor-abc',
      page_size: 100,
      keyword: 'alice',
      role: 'admin',
      status: '1',
    })
  })
})

function makeControlUser(): ControlUser {
  return {
    id: 'usr_abc123',
    display_name: 'Operator',
    email: 'op@example.com',
    providers: ['email'],
    avatar: '/a.png',
    role: 'admin',
    status: 1,
    available_credits: 42,
    last_login_ip: '203.0.113.7',
    last_login_country: 'US',
    created_at: '2026-09-23T00:00:00Z',
    updated_at: '2026-09-23T00:00:00Z',
  }
}
