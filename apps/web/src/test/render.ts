import {
  render as renderBase,
  screen,
  waitFor,
  within,
  type RenderOptions,
  type RenderResult,
} from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { i18n } from '../shared/i18n'

export type RenderedWithUser = RenderResult & {
  user: ReturnType<typeof userEvent.setup>
}

export type UserEventSetupOptions = Parameters<typeof userEvent.setup>[0]

function withI18n<C>(options?: RenderOptions<C>): RenderOptions<C> {
  const globalOptions = options?.global ?? {}
  const plugins = globalOptions.plugins ?? []
  const hasI18n = plugins.some(
    (plugin) => plugin === i18n || (Array.isArray(plugin) && plugin[0] === i18n),
  )
  if (hasI18n) return { ...(options ?? {}) }
  return {
    ...(options ?? {}),
    global: { ...globalOptions, plugins: [...plugins, i18n] },
  }
}

export function createUser(options?: UserEventSetupOptions) {
  return userEvent.setup(options)
}

function renderWithI18n<C>(component: C, options?: RenderOptions<C>): RenderResult {
  return renderBase(component, withI18n(options))
}

export function renderWithUser<C>(
  component: C,
  options?: RenderOptions<C>,
  userOptions?: UserEventSetupOptions,
): RenderedWithUser {
  const user = createUser(userOptions)

  return {
    user,
    ...renderWithI18n(component, options),
  }
}

export { renderWithI18n as render, screen, userEvent, waitFor, within }
