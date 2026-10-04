import { defineComponent, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAdminStickyTitle } from './useAdminStickyTitle'

const Harness = defineComponent({
  setup() {
    return useAdminStickyTitle()
  },
  template: '<main><h1>  Orders\n  &amp; Billing </h1></main>',
})

describe('useAdminStickyTitle', () => {
  let raf: ReturnType<typeof vi.fn>

  beforeEach(() => {
    Object.defineProperty(window, 'scrollY', { configurable: true, value: 0, writable: true })
    raf = vi.fn((callback: FrameRequestCallback) => {
      callback(0)
      return 1
    })
    vi.stubGlobal('requestAnimationFrame', raf)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('hides at the threshold and extracts the heading after scrolling', async () => {
    const wrapper = mount(Harness, { attachTo: document.body })
    expect(wrapper.vm.showStickyTitle).toBe(false)

    window.scrollY = 11
    window.dispatchEvent(new Event('scroll'))
    await nextTick()

    expect(wrapper.vm.showStickyTitle).toBe(true)
    expect(wrapper.vm.dynamicTitle).toBe('Orders & Billing')
    expect(raf).toHaveBeenCalledTimes(1)

    window.scrollY = 10
    window.dispatchEvent(new Event('scroll'))
    await nextTick()
    expect(wrapper.vm.showStickyTitle).toBe(false)
    wrapper.unmount()
  })

  it('resets state and removes listeners on unmount', async () => {
    const wrapper = mount(Harness, { attachTo: document.body })
    window.scrollY = 20
    window.dispatchEvent(new Event('scroll'))
    await nextTick()
    expect(wrapper.vm.dynamicTitle).toBe('Orders & Billing')

    wrapper.vm.reset()
    expect(wrapper.vm.showStickyTitle).toBe(false)
    expect(wrapper.vm.dynamicTitle).toBe('')

    wrapper.unmount()
    const rafCallsBeforeDetachedScroll = raf.mock.calls.length
    window.scrollY = 20
    window.dispatchEvent(new Event('scroll'))
    await nextTick()
    expect(raf).toHaveBeenCalledTimes(rafCallsBeforeDetachedScroll)
  })
})
