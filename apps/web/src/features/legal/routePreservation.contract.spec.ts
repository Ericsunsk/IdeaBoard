import { ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '../../test/render'
import { LEGAL_CONTACTS, LEGAL_DOCUMENT_VERSIONS, SUPPORT_EMAIL } from './documents'
import { PUBLIC_LEGAL_LINKS } from './legalLinks'
import TermsPage from './pages/TermsPage.vue'
import {
  ACCEPTABLE_USE_PATH,
  PRIVACY_POLICY_PATH,
  REFUND_POLICY_PATH,
  TERMS_PATH,
} from './routePaths'

vi.mock('../../core/composables/useSiteBranding', () => ({
  useSiteBranding: () => ({ projectName: ref('IdeaBoard') }),
}))

describe('legal route preservation (P2 no-op)', () => {
  it('keeps legal route URLs stable with no structural migration', () => {
    expect(TERMS_PATH).toBe('/terms')
    expect(REFUND_POLICY_PATH).toBe('/refund-policy')
    expect(PRIVACY_POLICY_PATH).toBe('/privacy-policy')
    expect(ACCEPTABLE_USE_PATH).toBe('/acceptable-use')
  })

  it('keeps public legal link destinations and labels stable', () => {
    expect([...PUBLIC_LEGAL_LINKS]).toEqual([
      { to: '/privacy-policy', label: 'Privacy Policy', shortLabel: 'Privacy' },
      { to: '/terms', label: 'Terms of Service', shortLabel: 'Terms' },
      { to: '/refund-policy', label: 'Refund Policy', shortLabel: 'Refunds' },
      { to: '/acceptable-use', label: 'Acceptable Use', shortLabel: 'Acceptable' },
    ])
  })

  it('keeps document versions and contact addresses stable', () => {
    expect({ ...LEGAL_DOCUMENT_VERSIONS }).toEqual({
      terms: '2026-10-09',
      privacy: '2026-10-09',
      refund: '2026-10-09',
      acceptableUse: '2026-08-01',
    })
    expect(SUPPORT_EMAIL).toBe('support@ideaboard.art')
    expect({ ...LEGAL_CONTACTS }).toEqual({
      support: 'support@ideaboard.art',
      legal: 'legal@ideaboard.art',
      privacy: 'privacy@ideaboard.art',
      copyright: 'copyright@ideaboard.art',
    })
  })

  it('renders the terms shell with the versioned title and contact links', () => {
    render(TermsPage, {
      global: {
        stubs: {
          RouterLink: { props: ['to'], template: '<a><slot /></a>' },
        },
      },
    })

    expect(screen.getByRole('heading', { name: 'Terms of Service' })).toBeTruthy()
    expect(screen.getByText('Effective date: 2026-10-09')).toBeTruthy()
    expect(screen.getByText(/govern your access to and use of IdeaBoard/)).toBeTruthy()
    expect(screen.getByRole('link', { name: 'support@ideaboard.art' })).toHaveProperty(
      'href',
      'mailto:support@ideaboard.art',
    )
    expect(screen.getByRole('link', { name: 'legal@ideaboard.art' })).toHaveProperty(
      'href',
      'mailto:legal@ideaboard.art',
    )
  })
})
