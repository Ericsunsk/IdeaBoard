import {
  render,
  screen,
  waitFor,
  within,
  type RenderOptions,
  type RenderResult,
} from '@testing-library/vue'
import userEvent from '@testing-library/user-event'

export type RenderedWithUser = RenderResult & {
  user: ReturnType<typeof userEvent.setup>
}

export type UserEventSetupOptions = Parameters<typeof userEvent.setup>[0]

export function createUser(options?: UserEventSetupOptions) {
  return userEvent.setup(options)
}

export function renderWithUser<C>(
  component: C,
  options?: RenderOptions<C>,
  userOptions?: UserEventSetupOptions,
): RenderedWithUser {
  const user = createUser(userOptions)

  return {
    user,
    ...render(component, options),
  }
}

export { render, screen, userEvent, waitFor, within }
