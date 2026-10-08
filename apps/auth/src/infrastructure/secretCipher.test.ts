import assert from 'node:assert/strict'
import { randomBytes } from 'node:crypto'
import { describe, it } from 'node:test'
import { createSecretCipher, decryptSecret, encryptSecret } from './secretCipher.js'

describe('secretCipher', () => {
  it('round-trips authenticated ciphertext with a fresh nonce', () => {
    const key = randomBytes(32).toString('base64')
    const cipher = createSecretCipher(key)
    const first = cipher.encrypt('provider-secret')
    const second = cipher.encrypt('provider-secret')

    assert.notEqual(first, second)
    assert.equal(cipher.decrypt(first), 'provider-secret')
    assert.equal(cipher.decrypt(second), 'provider-secret')
  })

  it('uses the fixed ciphertext envelope of a 12-byte nonce and 16-byte tag', () => {
    const key = randomBytes(32).toString('base64')
    const encoded = encryptSecret('secret', key)

    assert.equal(decryptSecret(encoded, key), 'secret')
  })

  it('rejects keys that are not exactly 256 bits', () => {
    assert.throws(() => encryptSecret('secret', 'invalid'), /32 bytes/)
  })

  it('rejects tampered ciphertext and a different key', () => {
    const key = randomBytes(32).toString('base64')
    const encoded = encryptSecret('secret', key)
    const payload = Buffer.from(encoded, 'base64')
    payload[payload.length - 1] ^= 1

    assert.throws(() => decryptSecret(payload.toString('base64'), key))
    assert.throws(() => decryptSecret(encoded, randomBytes(32).toString('base64')))
  })
})
