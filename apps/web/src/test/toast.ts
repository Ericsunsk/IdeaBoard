import { vi } from 'vitest'
import { useAppToast } from '../core/composables/useAppToast'

// Converged `useAppToast` test double: the superset of every inline
// `vi.mock(.../useAppToast)` shim (success/error/warning/info/loading/dismiss).
// Specs register it with a bare `vi.mock('<relpath>/useAppToast')` automock and
// a `mockAppToast()` call in `beforeEach`, keeping one stable identity per test.
export function createAppToastMocks() {
  return {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
    loading: vi.fn(() => ({ id: 'test-toast', destroy: vi.fn(), update: vi.fn() })),
    dismiss: vi.fn(),
  }
}

export type AppToastMocks = ReturnType<typeof createAppToastMocks>

export function mockAppToast(): AppToastMocks {
  const mocks = createAppToastMocks()
  vi.mocked(useAppToast).mockReturnValue(mocks as unknown as ReturnType<typeof useAppToast>)
  return mocks
}
