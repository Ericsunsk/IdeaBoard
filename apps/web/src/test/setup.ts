import '@testing-library/jest-dom/vitest'

import { cleanup } from '@testing-library/vue'
import { afterAll, afterEach, beforeAll } from 'vitest'

if (typeof MutationObserver !== 'undefined' && MutationObserver.prototype) {
  const originalDisconnect = MutationObserver.prototype.disconnect
  MutationObserver.prototype.disconnect = function (this: MutationObserver) {
    try {
      originalDisconnect.call(this)
    } catch {
      // ignore happy-dom private identifier proxy disconnect quirk
    }
  }
}

import { server } from './msw'

beforeAll(() => {
  server.listen({ onUnhandledRequest: 'error' })
})

afterEach(() => {
  server.resetHandlers()
  cleanup()
})

afterAll(() => {
  server.close()
})
