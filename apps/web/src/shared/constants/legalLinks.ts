import {
  ACCEPTABLE_USE_PATH,
  PRIVACY_POLICY_PATH,
  REFUND_POLICY_PATH,
  TERMS_PATH,
} from './routePaths'

export const PUBLIC_LEGAL_LINKS = [
  { to: PRIVACY_POLICY_PATH, label: 'Privacy Policy', shortLabel: 'Privacy' },
  { to: TERMS_PATH, label: 'Terms of Service', shortLabel: 'Terms' },
  { to: REFUND_POLICY_PATH, label: 'Refund Policy', shortLabel: 'Refunds' },
  { to: ACCEPTABLE_USE_PATH, label: 'Acceptable Use', shortLabel: 'Acceptable' },
] as const
