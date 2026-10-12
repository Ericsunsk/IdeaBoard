import { describe, expect, it } from 'vitest'
import { h } from 'vue'
import { render, screen, userEvent } from '../../../test/render'
import { Tabs, TabsContent, TabsList, TabsTrigger } from './index'

/**
 * Quota: final/tabs-keyboard. The roving tabindex, arrow/Home/End navigation
 * and disabled handling live in reka `Tabs`; this spec pins them through our
 * wrapper so a regression in forwarding (e.g. dropping `value`, `disabled`
 * or `loop`) turns red here rather than in a consumer.
 */
function renderAccountTabs() {
  return render(Tabs, {
    props: { defaultValue: 'overview' },
    slots: {
      default: () => [
        h(TabsList, { 'aria-label': 'Account sections' }, () => [
          h(TabsTrigger, { value: 'overview' }, () => 'Overview'),
          h(TabsTrigger, { value: 'activity', disabled: true }, () => 'Activity'),
          h(TabsTrigger, { value: 'settings' }, () => 'Settings'),
        ]),
        h(TabsContent, { value: 'overview' }, () => 'Overview panel'),
        h(TabsContent, { value: 'activity' }, () => 'Activity panel'),
        h(TabsContent, { value: 'settings' }, () => 'Settings panel'),
      ],
    },
  })
}

function tabAt(tabs: HTMLElement[], index: number): HTMLElement {
  const tab = tabs[index]
  if (!tab) throw new Error(`missing tab at index ${index}`)
  return tab
}

describe('Tabs primitive', () => {
  it('advertises tablist semantics with a single selected tab', () => {
    renderAccountTabs()

    expect(screen.getByRole('tablist', { name: 'Account sections' })).toBeInTheDocument()
    const tabs = screen.getAllByRole('tab')
    expect(tabs).toHaveLength(3)
    expect(tabs[0]).toHaveClass('min-h-8')
    expect(tabs[0]).toHaveAttribute('aria-selected', 'true')
    expect(tabs[1]).toHaveAttribute('aria-selected', 'false')
    expect(tabs[2]).toHaveAttribute('aria-selected', 'false')
  })

  it('keeps a single tab stop with roving tabindex', async () => {
    // Reka assigns the tab stop when focus first enters the list; from then
    // on exactly one tab carries tabindex 0 and it follows focus.
    const user = userEvent.setup()
    renderAccountTabs()

    const tabs = screen.getAllByRole('tab')
    await user.click(tabAt(tabs, 2))
    expect(tabs.filter((tab) => tab.getAttribute('tabindex') === '0')).toEqual([tabAt(tabs, 2)])

    // ArrowLeft skips the disabled tab and the tab stop follows.
    await user.keyboard('{ArrowLeft}')
    expect(document.activeElement).toBe(tabs[0])
    expect(tabs[0]).toHaveAttribute('tabindex', '0')
    expect(tabs[1]).toHaveAttribute('tabindex', '-1')
    expect(tabs[2]).toHaveAttribute('tabindex', '-1')
  })

  it('shows only the selected tab panel', () => {
    renderAccountTabs()

    expect(screen.getByRole('tabpanel', { name: 'Overview' })).toHaveTextContent('Overview panel')
    expect(screen.queryByRole('tabpanel', { name: 'Settings' })).not.toBeInTheDocument()
    expect(screen.queryByRole('tabpanel', { name: 'Activity' })).not.toBeInTheDocument()
  })

  it('marks the disabled tab inert so it never takes selection', async () => {
    const user = userEvent.setup()
    renderAccountTabs()

    const activity = screen.getByRole('tab', { name: 'Activity' })
    expect((activity as HTMLButtonElement).disabled).toBe(true)

    await user.click(activity)
    expect(activity).toHaveAttribute('aria-selected', 'false')
    expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.queryByRole('tabpanel', { name: 'Activity' })).not.toBeInTheDocument()
  })

  it('moves focus with arrow keys and skips disabled tabs', async () => {
    const user = userEvent.setup()
    renderAccountTabs()

    const tabs = screen.getAllByRole('tab')
    await user.click(tabAt(tabs, 0))
    await user.keyboard('{ArrowRight}')

    // Automatic activation: focus carries selection past the disabled tab.
    expect(document.activeElement).toBe(tabs[2])
    expect(tabs[2]).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tabpanel', { name: 'Settings' })).toHaveTextContent('Settings panel')
  })

  it('jumps to the first and last enabled tabs with Home and End', async () => {
    const user = userEvent.setup()
    renderAccountTabs()

    const tabs = screen.getAllByRole('tab')
    await user.click(tabAt(tabs, 2))

    await user.keyboard('{Home}')
    expect(document.activeElement).toBe(tabs[0])

    await user.keyboard('{End}')
    expect(document.activeElement).toBe(tabs[2])
  })

  it('selects a tab and reveals its panel on click', async () => {
    const user = userEvent.setup()
    renderAccountTabs()

    await user.click(screen.getByRole('tab', { name: 'Settings' }))

    expect(screen.getByRole('tab', { name: 'Settings' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'false')
    expect(screen.getByRole('tabpanel', { name: 'Settings' })).toHaveTextContent('Settings panel')
    expect(screen.queryByRole('tabpanel', { name: 'Overview' })).not.toBeInTheDocument()
  })

  it('links the visible panel back to its tab', () => {
    renderAccountTabs()

    const panel = screen.getByRole('tabpanel', { name: 'Overview' })
    const labelledBy = panel.getAttribute('aria-labelledby')
    expect(labelledBy).toBeTruthy()
    const tab = document.getElementById(labelledBy ?? '')
    expect(tab?.getAttribute('role')).toBe('tab')
    expect(tab).toHaveTextContent('Overview')
  })
})
