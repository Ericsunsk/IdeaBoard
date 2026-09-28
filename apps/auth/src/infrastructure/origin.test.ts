import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { normalizeOrigin } from './origin.js'

describe('normalizeOrigin', () => {
  it('normalizes origins to scheme plus lowercase host', () => {
    assert.equal(normalizeOrigin('https://APP.example.com/'), 'https://app.example.com')
    assert.equal(normalizeOrigin('http://localhost:5173'), 'http://localhost:5173')
  })

  it('drops default ports and keeps non-default ports', () => {
    assert.equal(normalizeOrigin('https://app.example.com:443'), 'https://app.example.com')
    assert.equal(normalizeOrigin('http://app.example.com:80'), 'http://app.example.com')
    assert.equal(normalizeOrigin('http://app.example.com:6001'), 'http://app.example.com:6001')
  })

  it('rejects non-origin URLs and malformed input', () => {
    assert.equal(normalizeOrigin('https://app.example.com/callback'), '')
    assert.equal(normalizeOrigin('https://app.example.com?x=1'), '')
    assert.equal(normalizeOrigin('ftp://app.example.com'), '')
    assert.equal(normalizeOrigin('https://user@app.example.com'), '')
    assert.equal(normalizeOrigin('not-a-url'), '')
    assert.equal(normalizeOrigin(''), '')
  })
})
