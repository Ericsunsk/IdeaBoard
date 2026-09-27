// Admin test factories (structural types on purpose): `src/test` stays free of
// concrete feature imports (`shared` never imports `features`), while the
// returned literals remain structurally assignable to the feature view types.

export type AdminCommandTargetOverrides = {
  publicId?: string
  displayName?: string
  role?: string
  status?: number
}

// Converged from `admin/users/commands{,Invalidation}.contract.spec.ts`.
export function makeAdminUserCommandTarget(overrides: AdminCommandTargetOverrides = {}) {
  return {
    publicId: 'user-1',
    displayName: 'user',
    role: 'user',
    status: 1,
    ...overrides,
  }
}

export type AdminListItemOverrides = {
  public_id?: string
  email?: string
  providers?: string[]
  display_name?: string
  role?: 'admin' | 'user'
  status?: number
  available_credits?: number
  avatar?: string
  created_at?: string
  updated_at?: string
  last_login_ip?: string
  last_login_country?: string
}

// Converged from `admin/users/view.unit.spec.ts` (`AdminUserListItem` shape).
export function makeAdminUserListItem(overrides: AdminListItemOverrides = {}) {
  return {
    public_id: 'usr_000000000000000000000001',
    email: 'user@example.com',
    providers: ['email'],
    display_name: 'user',
    role: 'user' as const,
    status: 1,
    available_credits: 0,
    avatar: '',
    created_at: '',
    updated_at: '',
    last_login_ip: '',
    last_login_country: '',
    ...overrides,
  }
}

export type ControlUserPayloadOverrides = {
  id?: string
  email?: string
  providers?: string[]
  display_name?: string
  avatar?: string
  role?: string
  status?: number
  available_credits?: number
  created_at?: string
  updated_at?: string
}

// Raw control-plane user payload served by MSW in
// `admin/users/AdminUsersPage.component.spec.ts`.
export function makeControlUserPayload(overrides: ControlUserPayloadOverrides = {}) {
  return {
    id: 'usr_000000000000000000000012',
    email: 'admin@example.com',
    providers: ['email'],
    display_name: 'admin_balance_user',
    avatar: '',
    role: 'user',
    status: 1,
    available_credits: 125,
    created_at: '2026-07-24T08:00:00Z',
    updated_at: '2026-07-24T08:00:00Z',
    ...overrides,
  }
}

export const adminPaymentOrderNumber = 'IBP-20260803-ABCDEF123456'

export type PaymentOrderSummaryOverrides = {
  public_order_number?: string
  user?: { public_id: string; display_name: string }
  idea_quantity?: number
  base_amount_minor?: number
  base_currency?: string
  paid_amount_minor?: number
  paid_currency?: string
  status?: string
  stripe_status?: string
  credit_status?: string
  created_at?: string
  paid_at?: string
  last_checked_at?: string
  anomalous_at?: string
}

// Snake_case API shape from
// `admin/payments/orders/paymentOrderResource.contract.spec.ts`.
export function makePaymentOrderSummary(overrides: PaymentOrderSummaryOverrides = {}) {
  return {
    public_order_number: adminPaymentOrderNumber,
    user: {
      public_id: '00000000-0000-4000-8000-000000000042',
      display_name: 'Sunshi',
    },
    idea_quantity: 12_000,
    base_amount_minor: 12_000,
    base_currency: 'usd',
    paid_amount_minor: 12_750,
    paid_currency: 'hkd',
    status: 'paid_pending_credit',
    stripe_status: 'paid',
    credit_status: 'pending',
    created_at: '2026-08-01T01:00:00Z',
    paid_at: '2026-08-01T01:03:00Z',
    last_checked_at: '2026-08-03T01:00:00Z',
    anomalous_at: '2026-08-02T01:00:00Z',
    ...overrides,
  }
}

export function makePaymentOrderDetail(overrides: PaymentOrderSummaryOverrides = {}) {
  return {
    ...makePaymentOrderSummary(overrides),
    paid_subtotal_minor: 12_000,
    paid_tax_minor: 750,
    paid_total_minor: 12_750,
    stripe: {
      account_id: 'acct…7890',
      customer_id: 'cus_…7890',
      checkout_session_id: 'cs_t…7890',
      payment_intent_id: 'pi_1…7890',
      charge_id: 'ch_1…7890',
    },
    terms_version: '2026-08-01',
    refund_policy_version: '2026-08-01',
    receipt_url: 'https://pay.stripe.com/receipts/example',
    expires_at: '2026-08-01T01:30:00Z',
    updated_at: '2026-08-03T01:00:00Z',
    reconciliation: {
      anomalous: true,
      anomalous_at: '2026-08-02T01:00:00Z',
      last_checked_at: '2026-08-03T01:00:00Z',
      next_attempt_at: '2026-08-03T01:05:00Z',
      last_error_code: 'stripe_unavailable',
      last_error_summary: 'Stripe 暂时不可用',
    },
  }
}
