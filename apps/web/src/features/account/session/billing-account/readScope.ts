import type { BillingAccountScope } from './resource'

// Immutable query-key context. Wallet visibility does not depend on the
// consumption scope; both contexts stay isolated by the authenticated identity.
export type BillingWalletReadScope = Readonly<{
  workspaceId: string
  billingAccountId: string
  identity: string
}>

export type BillingConsumptionReadScope = BillingWalletReadScope &
  Readonly<{ consumptionScope: BillingAccountScope['consumptionScope'] | undefined }>
