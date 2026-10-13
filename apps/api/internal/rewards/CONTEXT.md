# Rewards Context

Rewards owns RewardCode rules and Redemption facts for platform Idea rewards.
User-facing copy uses `Idea`/`Ideas`; internal amounts, codes, APIs, and schema use `Credit`/`credits`.
It does not own the Credits Ledger or Identity data; all ledger effects go through `credits.Service`, and user fields come from the authenticated `Principal` or the Runtime Identity projection adapter.

## Domain Language

| Term                 | Meaning                                                                                                                  |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| RewardCode           | Single aggregate of display name, HMAC-only bearer Code, Credit award, optional total cap and expiry, and enabled intent |
| Redemption           | Immutable fact of one user's successful RewardCode claim                                                                 |
| Admin Action Journal | Append-only record inside the Rewards context of actor, action, time, and safe metadata                                  |

## Public Seams

- `AdminCommands` offers Create, Update, SetEnabled, and Delete: Create uses a client idempotency key; Update saves rules and optionally rotates the bearer Code in one transaction; SetEnabled is set-state; Delete accepts only zero-Redemption aggregates.
- `AdminReads.SearchRewardCodes` searches via no-store POST body with fuzzy name search, full-Code HMAC lookup, effective-status filter, filtered total, and keyset pages, and results carry no plaintext or full digest.
- `AdminReads.ListAdminRedemptions` pages successful Redemption summaries by RewardCode scope with a keyset; `GetAdminRedemption` returns the Credits Transaction ID and masked Code hint only within the same RewardCode scope.
- `UserCommands.Redeem` synchronously redeems a RewardCode and returns the Redemption, credited amount, available balance, and replay marker.
- `UserReads.ListRedemptions` returns the current user's successful Redemption history by ID cursor with only RewardCode display name, credited amount, masked Code hint, and redeem time.

Implementation note: hash normalization, HMAC candidates, row locking, persistence, and Credits transaction orchestration stay inside the module implementation.

## Invariants

- RewardCode creation serializes on a key-independent PostgreSQL advisory transaction lock over the normalized Code; the HMAC digest alias covers current and previous keys together so the same plaintext cannot double-write during rolling rotation.
- Create idempotency double-checks a non-sensitive business-field fingerprint and the persisted digest alias; old-only and current-plus-previous instances may replay symmetrically during rotation overlap, while a different Code or business payload still conflicts.
- RewardCode mutation and Redeem first take the PostgreSQL advisory transaction lock by RewardCode ID, then the row lock via a controlled database function; same-user replay, eligibility and cap gates, capacity counter, Credits Grant, and Redemption insert all sit in one transaction, Runtime neither needs nor holds direct UPDATE privilege on the RewardCode table, and the Credit business reference uses the standalone `reward_code_redemption` namespace.
- Each user redeems each RewardCode successfully at most once; `max_redemptions = 0` means unlimited.
- RewardCode plaintext appears only in the first successful create response or a successful update response carrying a new Code; create replays, plain updates, Code replays, lists, history, Journal, logs, and errors never carry plaintext or full digest, and related HTTP responses are always `Cache-Control: no-store`.
- Update changes only the name, Credit amount, cap, and expiry for later Redemptions; historical Redemption amounts, Credit Transaction IDs, and times stay immutable, a finite cap must not drop below the current Redemption count, and `0` restores unlimited.
- `enabled` is Admin intent; effective status derives as `expired > exhausted > disabled > enabled`, and the frontend shows the configured intent with a standalone toggle so it never confuses the actual redeemable cause.
- Update converges rule changes and optional Code rotation into one Admin PATCH and one transaction; any validation, collision, or persistence failure rolls everything back, a new Code replaces the primary digest and all current and previous aliases with the old Code dead right after commit, and submitting a Code that already hits the target's current alias counts as replay with no repeated rotation Journal and no plaintext returned.
- Update and SetEnabled never double-Journal the same target state; a zero-Redemption RewardCode may hard-delete, and the Journal nullable FK uses `ON DELETE SET NULL` while permanently keeping the immutable target ID and safe target snapshot.
- Code plaintext or reversible ciphertext never enters the database, Admin lists, URLs, Journal, or logs; only key version, HMAC-SHA256 digest, and a four-character hint are stored.
- Admin search accepts POST bodies only; a query normalizable to a full Code generates only current and previous HMAC candidates and is never sent to PostgreSQL as a name parameter, and name, effective status, filtered total, and keyset page must come from one read snapshot.
- Rewards owns the caller transaction and writes the Credits Ledger only via `credits.Service.InTx(tx).Grant`; capacity, Grant, and Redemption commit or roll back together.
- The Runtime role executing `reserve_reward_code_redemption_v1` is a restricted capability that still consumes one eligible RewardCode cap entry; expiry is judged by database time only, and the Grant plus Redemption insert stay orchestrated by the application transaction.
- Credit Grants always use source `reward_redemption`, a fixed redemption-reward reason literal, and the Redemption ID business reference.
- An authenticated Admin may reverse a Grant's accounting effect via Credits Correction; the Correction is a new Ledger fact that deletes no Redemption and restores no RewardCode capacity.
- User Redemption history reads only committed Redemptions and pages them isolated by `(user_id, id DESC)`; it joins or copies no Identity PII, Code digest, or plaintext.
- The User Redeem response takes `user_public_id` and `display_name` only from the Auth middleware's verified request `Principal` snapshot; no fallible Identity read runs after commit.
- Admin Redemption reads only committed facts by `(reward_code_id, id DESC)`; the summary carries no technical detail, detail must match both RewardCode ID and Redemption ID, and neither joins Identity, Journal, or Code digest and plaintext; HTTP presentation composes user fields through the Runtime Identity projection adapter with at most one bounded batch read per list page and one single read per detail, and missing or inconsistent Identity mappings fail closed, while authenticated Redemption history reads no Identity.
- `reward_admin_actions` records actor, action, time, and safe metadata automatically, and stays append-only by both database trigger and runtime privileges; the guard admits only FK referential nullification from `ON DELETE SET NULL`, where the parent must already be gone and every other Journal field unchanged, and Runtime holds no direct UPDATE or DELETE on RewardCode, Alias, or Journal and only runs transaction-bound Rewards command functions so the Journal cannot be bypassed.
- `REWARDS_CODE_HMAC_KEYS` accepts only current plus optional previous; each base64 key decodes to at least 32 bytes, and startup fails closed on invalid config.
- Code normalization applies a 128-byte raw bound before any Unicode normalization, then NFKC, uppercase, trim, and Unicode whitespace and hyphen removal; the final payload accepts only 8 to 32 `A-Z0-9` characters.
- During HMAC rotation overlap, lookup verifies current and previous while new issuance uses current only; every persisted RewardCode (including disabled, expired, or unredeemed) still depends on its primary key version, so the previous key must not retire while records depending on the old version exist.
- The user Redeem endpoint owns a per-user in-process token bucket, 10 attempts per 10 minutes by default, cleaning idle entries on arrival; it is not a cross-instance business invariant and never grows into a Platform interface or persisted bucket.
- Unavailable-Code Redeem states share one public error; responses and logs carry no raw Code, full digest, or request body, and Redeem responses are always `no-store`.
- Redemption observability records only fixed outcome, credited Ideas, and latency; User, RewardCode, Code, and digest must never become metrics fields or labels.
- RewardCode schema is created from one canonical migration only and never replays the deleted Campaign and Batch evolution chain; if the database carries a removed migration version, Rewards Ledger reference, or existing RewardCode facts, migration and rollback must fail closed and demand an explicit Fresh Database reset, and must never auto-clean or orphan the Credits Ledger.

## Non-goals

- No partitioning, sharded counters, Redis or quota blocks, distributed limiters, Platform RateLimiter abstraction, or PostgreSQL buckets; the current design uses plain PostgreSQL B-trees, RewardCode row locks, and exact counters.
