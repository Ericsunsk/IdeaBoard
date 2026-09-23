# Account Context
Account owns the user-side surface for credentials, credit wallet, recharge checkout, usage details, reward redemption, and security settings, in strict DDD layers (`domain`, `application`, `infrastructure`, `presentation`).
It does NOT own the server-side ledger, the Stripe gateway itself, or pricing policy.

## Domain Language
| Term | Meaning |
| ---- | ------- |
| Idea / Credit | User-visible copy is always `Idea`/`Ideas`; code, domain models, schemas, and transport stay `Credit`/`credits`. |
| Wallet Balance | Credit account state (`balance`, `held`, `available = balance - held`); source of truth is the server PostgreSQL ledger, synced via `fetchCreditWallet` / `creditsQueryKeys` in `infrastructure/credits/resource.ts`. |
| Recharge Session | Stripe recharge state machine: pick a package (`PurchaseCard`) -> start checkout (Stripe Checkout Session) -> external redirect -> return polling (`ReturnStatus`). |
| Reward Redemption | Reward-code flow via `redeemRewardCode` with instance- and IP-level rate limits; safe to retry on 409. |
| Account Overlay | Global dialog host singleton in `accountOverlay.ts` for tabs (`profile` / `recharge` / `usage` / `billing` / `rewards` / `security`), rendered by `AccountDialogHost.vue`. |

## Public Seams
- `domain/index.ts` pure math, `centsToDollars`, `createUsageChartConfig`, `accountTabs`; display via `formatIdeaAmount` in `shared/i18n/userIdeaCurrency.ts`.
- `infrastructure/index.ts` unified export for `credits`, `payments`, `rewards`, `usage`, `billing` transport resources.
- `application/index.ts` use-case hooks: `useRecharge`, `useCreditWallet`, `useCreditsSync`, `useRewardRedemption`, `useUsage`, `useAvatarUpload`, `accountOverlay`.
- `presentation/index.ts` view layer: `RechargePane`, `UsagePane`, `RewardsPane`, `SecurityPane`, `BillingPane`, and `AccountDialogHost`.
- `accountOverlay.open(tab?)` the only entry to open the account dialog.

## Invariants
- External callers open the dialog only via `accountOverlay.open(tab?)`; never instantiate `AccountDialog` locally per page.
- Balance display follows the source of truth: after recharge or redemption success, explicitly call `invalidateCreditWallet()`.
- Never compute a permanent balance by local add/subtract; never optimistically mutate without a server round-trip.
- `domain/` never imports Vue or transport DTOs; it stays deterministic with full unit coverage.
- All money is stored and computed as integers (minor units); only `presentation` formats to currency strings.
- Amount display uses the shared `formatIdeaAmount` formatter.
- Transport normalization (snake_case to camelCase) happens in adapter/resource layers.
- `application` coordinates loading, error feedback, and Query Cache invalidation; it never renders UI templates.
- `presentation` is display plus event dispatch; all async work delegates to `application`.
- Recharge returns must always handle a terminal state; never mark an order successful before it is persisted.
- Redemption always respects rate limits and refreshes the wallet on success; no unbounded retries.

## Non-goals
- No user-visible `Credit` copy; no `Idea` renaming inside the domain layer.
- No per-route `AccountDialog` instances.
- No Vue reactivity or network dependencies in `domain/`.
