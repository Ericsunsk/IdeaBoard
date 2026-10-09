import { SUPPORT_EMAIL } from '../../shared/constants/contacts'

export const LEGAL_DOCUMENT_VERSIONS = {
  terms: '2026-10-09',
  privacy: '2026-10-09',
  refund: '2026-10-09',
  acceptableUse: '2026-08-01',
} as const

export const LEGAL_CONTACTS = {
  support: SUPPORT_EMAIL,
  legal: 'legal@ideaboard.art',
  privacy: 'privacy@ideaboard.art',
  copyright: 'copyright@ideaboard.art',
} as const
