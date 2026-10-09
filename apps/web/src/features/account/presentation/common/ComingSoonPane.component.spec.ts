import { defineComponent, h } from 'vue'
import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../../test/render'
import { i18n } from '../../../../shared/i18n'
import ComingSoonPane from './ComingSoonPane.vue'

const IconStub = defineComponent({
  name: 'ComingSoonIconStub',
  setup: () => () => h('span', { 'data-testid': 'pane-icon' }),
})

function renderPane() {
  return renderWithUser(ComingSoonPane, {
    props: {
      categoryKey: 'account.groups.billing',
      titleKey: 'account.tabs.plans',
      icon: IconStub,
    },
  })
}

describe('ComingSoonPane', () => {
  it('renders the coming-soon capsule on StatusBadge neutral with the card surface passed through', () => {
    renderPane()

    const capsule = screen.getByText(i18n.global.t('account.comingSoon'))
    const root = capsule.parentElement!
    expect(root).toHaveClass('rounded-full', 'border', 'border-border')
    expect(root).toHaveClass('bg-card')
    expect(root.className.split(' ')).not.toContain('bg-muted')
    expect(root.querySelector('[aria-hidden="true"]')).toBeNull()
  })

  it('keeps the icon tile on the card surface', () => {
    renderPane()

    expect(screen.getByTestId('pane-icon').parentElement!).toHaveClass('bg-card')
  })
})
