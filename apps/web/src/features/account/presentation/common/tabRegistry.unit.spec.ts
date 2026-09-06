import { describe, expect, it } from 'vitest'
import {
  accountTabRegistry,
  resolveAccountTab,
  resolveAccountTabKey,
} from './tabRegistry'

describe('accountTabRegistry', () => {
  it('defines each account tab exactly once', () => {
    const keys = accountTabRegistry.flatMap((group) => group.tabs.map((tab) => tab.key))

    expect(new Set(keys)).toHaveLength(keys.length)
  })

  it('uses the registry for unknown tab fallback', () => {
    expect(resolveAccountTabKey('unknown-tab')).toBe('profile')
    expect(resolveAccountTab(undefined).tab.key).toBe('profile')
  })

  it.each(['billing', 'usage'] as const)('passes active state to the %s query pane', (key) => {
    const entry = resolveAccountTab(key)

    expect(entry.tab.pane.props({ active: false, paymentReturn: null, ...entry })).toEqual({
      active: false,
    })
    expect(entry.tab.pane.props({ active: true, paymentReturn: null, ...entry })).toEqual({
      active: true,
    })
  })
})
