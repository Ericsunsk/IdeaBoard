import { defineComponent, h, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { render } from '../../../test/render'
import {
  createAccountOverlay,
  provideAccountOverlay,
  useAccountOverlay,
  type AccountOverlay,
} from './overlay'

function makeRoute(path = '/user/boards') {
  return { fullPath: path, hash: '', path, query: {} as Record<string, unknown> }
}

function createHostOverlay() {
  return createAccountOverlay({
    currentUser: ref<object | null>({ id: 'user-1' }),
    reportError: vi.fn(),
    navigation: {
      replace: vi.fn().mockResolvedValue(undefined),
      resolve: (location) => ({ fullPath: location.path }),
    },
    route: makeRoute() as never,
  })
}

// Two Workspace hosts keep separately installed Account overlays;
// open/close controls target their own host and unmount releases its overlay.
// Replaces the source-text ownership matchers in
// `accountOwnership.contract.spec.ts` with observable provider behavior.
describe('Account overlay provider lifecycle', () => {
  it('keeps two installed hosts isolated: open/close targets only its own overlay', () => {
    let overlayA: AccountOverlay | undefined
    let overlayB: AccountOverlay | undefined
    let seenA: AccountOverlay | undefined
    let seenB: AccountOverlay | undefined

    const ConsumerA = defineComponent({
      setup() {
        seenA = useAccountOverlay()
        return () => h('div')
      },
    })
    const HostA = defineComponent({
      setup() {
        overlayA = createHostOverlay()
        provideAccountOverlay(overlayA)
        return () => h(ConsumerA)
      },
    })
    const ConsumerB = defineComponent({
      setup() {
        seenB = useAccountOverlay()
        return () => h('div')
      },
    })
    const HostB = defineComponent({
      setup() {
        overlayB = createHostOverlay()
        provideAccountOverlay(overlayB)
        return () => h(ConsumerB)
      },
    })

    const appA = render(HostA)
    const appB = render(HostB)

    expect(overlayA).toBeDefined()
    expect(overlayB).toBeDefined()
    expect(overlayA).not.toBe(overlayB)
    expect(seenA).toBe(overlayA)
    expect(seenB).toBe(overlayB)

    overlayA!.commands.open('billing')
    expect(overlayA!.viewModel.isVisible.value).toBe(true)
    expect(overlayA!.viewModel.initialTab.value).toBe('billing')
    expect(overlayB!.viewModel.isVisible.value).toBe(false)

    overlayB!.commands.open('recharge')
    expect(overlayB!.viewModel.isVisible.value).toBe(true)
    expect(overlayB!.viewModel.initialTab.value).toBe('recharge')
    expect(overlayA!.viewModel.isVisible.value).toBe(true)

    overlayA!.commands.close()
    expect(overlayA!.viewModel.isVisible.value).toBe(false)
    expect(overlayB!.viewModel.isVisible.value).toBe(true)

    overlayB!.commands.close()
    expect(overlayB!.viewModel.isVisible.value).toBe(false)

    appA.unmount()
    appB.unmount()
  })

  it('releases a host overlay on unmount without touching the surviving host', () => {
    let overlayA: AccountOverlay | undefined
    let overlayB: AccountOverlay | undefined

    const HostA = defineComponent({
      setup() {
        overlayA = createHostOverlay()
        provideAccountOverlay(overlayA)
        return () => h('div')
      },
    })
    const HostB = defineComponent({
      setup() {
        overlayB = createHostOverlay()
        provideAccountOverlay(overlayB)
        return () => h('div')
      },
    })

    const appA = render(HostA)
    const appB = render(HostB)

    overlayA!.commands.open('usage')
    overlayB!.commands.open('billing')
    expect(overlayA!.viewModel.isVisible.value).toBe(true)
    expect(overlayB!.viewModel.isVisible.value).toBe(true)

    appA.unmount()

    expect(overlayB!.viewModel.isVisible.value).toBe(true)
    overlayB!.commands.close()
    expect(overlayB!.viewModel.isVisible.value).toBe(false)

    appB.unmount()
  })

  it('has no implicit global fallback: useAccountOverlay without a host throws', () => {
    const Consumer = defineComponent({
      setup() {
        useAccountOverlay()
        return () => h('div')
      },
    })
    expect(() => render(Consumer)).toThrow('Account overlay is not installed')
  })
})
