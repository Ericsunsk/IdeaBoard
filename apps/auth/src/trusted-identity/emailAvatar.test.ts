import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { normalizeAvatarURL } from '../infrastructure/authUtils.js'
import { buildEmailAvatarUrl } from './emailAvatar.js'

describe('emailAvatar', () => {
  test('uses the internal user id as a stable, escaped seed', () => {
    const first = buildEmailAvatarUrl('auth-1')
    assert.equal(first, buildEmailAvatarUrl('auth-1'))
    assert.notEqual(first, buildEmailAvatarUrl('auth-2'))
    assert.equal(
      buildEmailAvatarUrl('auth&seed=other'),
      'https://api.dicebear.com/9.x/big-ears-neutral/svg?seed=auth%26seed%3Dother',
    )
  })

  test('ignores surrounding whitespace', () => {
    assert.equal(buildEmailAvatarUrl('  auth-1  '), buildEmailAvatarUrl('auth-1'))
  })

  test('avatar URL is a short https URL that survives normalizeAvatarURL', () => {
    const url = buildEmailAvatarUrl('auth-1')
    assert.equal(url, 'https://api.dicebear.com/9.x/big-ears-neutral/svg?seed=auth-1')
    assert.equal(normalizeAvatarURL(url), url)
    assert.ok(url.length <= 2_048)
  })

  test('unusable seeds keep the previous empty-avatar behavior', () => {
    assert.equal(buildEmailAvatarUrl(''), '')
    assert.equal(buildEmailAvatarUrl('   '), '')
    assert.equal(buildEmailAvatarUrl('a b'), '')
    assert.equal(buildEmailAvatarUrl('a'.repeat(192)), '')
  })
})
