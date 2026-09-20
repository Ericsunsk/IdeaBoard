import '@testing-library/jest-dom/vitest'

import { cleanup } from '@testing-library/vue'
import { afterAll, afterEach, beforeAll, beforeEach } from 'vitest'

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

let unhandledRequests: string[] = []

beforeAll(() => {
  server.listen({
    onUnhandledRequest(request) {
      unhandledRequests.push(`${request.method} ${request.url}`)
    },
  })
})

beforeEach(() => {
  unhandledRequests = []
})

afterEach(() => {
  const requests = unhandledRequests.join('\n')
  unhandledRequests = []
  try {
    if (requests) throw new Error(`Unhandled MSW request(s):\n${requests}`)
  } finally {
    server.resetHandlers()
    cleanup()
  }
})

afterAll(() => {
  server.close()
})
