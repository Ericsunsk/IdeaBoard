import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/vue'
import { afterEach, expect, it } from 'vitest'
import { i18n } from '../../../../../core/i18n'
import { renderWithUser, screen } from '../../../../../test/render'
import '../../../../../app/style.css'
import Roster from './Roster.vue'

afterEach(cleanup)

it('spaces collaborator avatars with the shared spacing scale', () => {
  i18n.global.locale.value = 'en'
  renderWithUser(Roster, {
    global: { stubs: { transition: false, 'transition-group': false } },
    props: {
      peers: [7, 8].map((clientId) => ({
        clientId,
        state: {
          user: { id: `usr_${clientId}`, name: `Peer ${clientId}` },
          selection: { nodeIds: [] },
        },
      })),
      localUser: { id: 'usr_ada', name: 'Ada' },
    },
  })
  const stack = screen.getByRole('list')
  expect(getComputedStyle(stack).gap).toBe('8px')
  document.documentElement.style.setProperty('--spacing', '5px')
  try {
    expect(getComputedStyle(stack).gap).toBe('10px')
  } finally {
    document.documentElement.style.removeProperty('--spacing')
  }
})
