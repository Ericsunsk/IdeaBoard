import {
  fireEvent,
  render as renderBase,
  screen,
  waitFor,
  within,
  type RenderOptions,
  type RenderResult,
} from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { cloneVNode, defineComponent, h, isVNode } from 'vue'
import { IconoirProvider } from '@iconoir/vue'
import { i18n } from '../core/i18n'

export type RenderedWithUser = RenderResult & {
  user: ReturnType<typeof userEvent.setup>
}

export type UserEventSetupOptions = Parameters<typeof userEvent.setup>[0]

// 与 App.vue 保持一致的测试图标上下文：生产全树包在 IconoirProvider 内，
// specs 若直接渲染图标会触发 injection not found warning，此处统一补上。
const testIconProps = {
  color: 'currentColor',
  width: '1.5em',
  height: '1.5em',
} as const

function readEmitsOf(component: unknown): string[] {
  const options = (component as { __vccOpts?: { emits?: unknown } } | null)?.__vccOpts ?? component
  const emits = (options as { emits?: unknown } | null)?.emits
  if (Array.isArray(emits)) return emits.filter((name): name is string => typeof name === 'string')
  if (emits !== null && typeof emits === 'object') return Object.keys(emits)
  return []
}

// IconoirProvider 只渲染 slot、无额外 DOM；wrapper 转发 attrs/slots 并把
// 被测组件的 emits 逐个透传，保证 emitted()/rerender/onX 行为与直接渲染一致。
function withTestProviders<C>(component: C) {
  // 部分 specs 直接以 h() 产生的 VNode 为根：此时用 cloneVNode 补齐透传，
  // 而不是把 vnode 当作组件定义再次 h()（那会渲染出空壳）。
  if (isVNode(component)) {
    const vnode = component
    const innerEmits = readEmitsOf((vnode as { type?: unknown }).type)
    const WrappedVNode = defineComponent({
      name: 'TestProvidersWrapper',
      inheritAttrs: false,
      emits: innerEmits,
      setup(_, { emit }) {
        return () =>
          h(
            IconoirProvider,
            { iconProps: { ...testIconProps } },
            {
              default: () => [cloneVNode(vnode, buildEmitForwarders(innerEmits, emit))],
            },
          )
      },
    })
    return WrappedVNode as never as C
  }
  const innerEmits = readEmitsOf(component)
  const Wrapped = defineComponent({
    name: 'TestProvidersWrapper',
    inheritAttrs: false,
    emits: innerEmits,
    setup(_, { slots, attrs, emit }) {
      return () =>
        h(
          IconoirProvider,
          { iconProps: { ...testIconProps } },
          {
            default: () =>
              h(component as never, { ...attrs, ...buildEmitForwarders(innerEmits, emit) }, slots),
          },
        )
    },
  })
  return Wrapped as never as C
}

function buildEmitForwarders(
  innerEmits: string[],
  emit: (event: string, ...args: unknown[]) => void,
): Record<string, (...args: unknown[]) => void> {
  return Object.fromEntries(
    innerEmits.map((name) => [
      `on${name.charAt(0).toUpperCase()}${name.slice(1)}`,
      (...args: unknown[]) => emit(name, ...args),
    ]),
  )
}

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

function createUser(options?: UserEventSetupOptions) {
  return userEvent.setup(options)
}

function renderWithI18n<C>(component: C, options?: RenderOptions<C>): RenderResult {
  return renderBase(withTestProviders(component), withI18n(options))
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

export { fireEvent, renderWithI18n as render, screen, userEvent, waitFor, within }
