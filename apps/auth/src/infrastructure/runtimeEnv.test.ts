import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { isProductionRuntime } from './runtimeEnv.js'

describe('isProductionRuntime', () => {
  it('detects production runtimes via NODE_ENV or APP_ENV', () => {
    assert.equal(isProductionRuntime({ NODE_ENV: 'production' }), true)
    assert.equal(isProductionRuntime({ NODE_ENV: 'PRODUCTION' }), true)
    assert.equal(isProductionRuntime({ APP_ENV: 'prod' }), true)
    assert.equal(isProductionRuntime({ NODE_ENV: 'release' }), true)
  })

  it('treats development and empty environments as non-production', () => {
    assert.equal(isProductionRuntime({ NODE_ENV: 'development' }), false)
    assert.equal(isProductionRuntime({ NODE_ENV: 'test' }), false)
    assert.equal(isProductionRuntime({}), false)
  })
})
