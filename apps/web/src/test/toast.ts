import { expect, vi, type Mock } from 'vitest'
import { useAppToast } from '../core/composables/useAppToast'
import { screen, waitFor } from './render'

// Converged `useAppToast` test double: the superset of every inline
// `vi.mock(.../useAppToast)` shim (success/error/warning/info/loading/dismiss).
// Specs register it with a bare `vi.mock('<relpath>/useAppToast')` automock and
// a `mockAppToast()` call in `beforeEach`, keeping one stable identity per test.
function createAppToastMocks() {
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

// Assert toast feedback without a duplicate inline error in the chosen scope.
export async function expectToastFeedback(
  toast: AppToastMocks,
  message: string,
  scope: {
    queryByRole: (role: 'alert') => HTMLElement | null
    queryByText: (text: string | RegExp) => HTMLElement | null
  } = screen,
  absentText: Array<string | RegExp> = [],
): Promise<void> {
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith(message))
  expect(scope.queryByRole('alert')).not.toBeInTheDocument()
  for (const text of absentText) {
    expect(scope.queryByText(text)).not.toBeInTheDocument()
  }
}

// Feedback factories must be called once without a duplicate inline alert.
export async function expectFeedbackSent(
  feedbackFn: Mock,
  scope: { queryByRole: (role: 'alert') => HTMLElement | null } = screen,
): Promise<void> {
  await waitFor(() => expect(feedbackFn).toHaveBeenCalledTimes(1))
  expect(scope.queryByRole('alert')).not.toBeInTheDocument()
}
