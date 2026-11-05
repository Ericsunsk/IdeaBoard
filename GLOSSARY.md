# IdeaBoard

Ubiquitous language for Tenancy, Billing, Boards and Canvas execution. `DESIGN.md` owns contracts, `CONTEXT.md` owns local rules, this file owns canonical names.

## Language

**Idea**:
User-facing unit of AI consumption. Display only.
_Avoid_: Credit in UI copy, coin, point
Owner: `apps/api/internal/billing/CONTEXT.md`

**Credit**:
Internal integer unit of consumption. Code, API and schema only.
_Avoid_: Idea in code, Credit in UI copy
Owner: `apps/api/internal/credits/CONTEXT.md`

**Billing Account**:
Stable funds identity (`ba_`). Owns Wallets and Ledger.
_Avoid_: Workspace Billing Account, organization balance, account alone
Owner: `apps/api/internal/credits/CONTEXT.md`

**Credit Wallet**:
Balance container owned by one Billing Account. Projection of Ledger.
_Avoid_: funds pool, balance account
Owner: `apps/api/internal/credits/CONTEXT.md`

**Member Quota**:
Consumption limit for one Business User across Workspaces sharing an Organization Billing Account. Personal consumption has no Member Quota.
_Avoid_: balance, sub-wallet, grant
Owner: `apps/api/internal/memberquotas/CONTEXT.md`

**Tenant Workspace**:
Isolation boundary for Boards, Tasks, Library and Media. Records consumption position, holds no balance.
_Avoid_: Canvas Workspace, Workspace Shell, team
Owner: `apps/web/src/features/workspace/CONTEXT.md`

**Canvas Workspace**:
Single-Board editing interaction surface. Not a tenancy or funds boundary.
_Avoid_: Tenant Workspace, Workspace Shell
Owner: `apps/web/src/features/canvas/CONTEXT.md`

**Organization**:
Membership and administration boundary. Never consumes or holds boards.
_Avoid_: team account, organization balance
Owner: `apps/web/src/features/workspace/CONTEXT.md`

**Business User**:
Human or managed identity operating in product. Public reference is `public_id` (`usr_`).
_Avoid_: auth_user_id, internal id, display_name as key
Owner: `apps/web/src/features/auth/CONTEXT.md`

**Actor**:
Operation performer recorded on Ledger and snapshots. Person or service, never a funds identity.
_Avoid_: payer, member, holder as Actor
Owner: `apps/api/internal/credits/CONTEXT.md`

**Board Document**:
Persisted graph document. Sole fact source for a Board.
_Avoid_: Board page, Board card, Preview Scene
Owner: `apps/web/src/features/board/CONTEXT.md`

**Library Asset**:
Saved collection reference, either external inspiration or managed Media. Identity is `provider:sourceId` or `media:mediaAssetId`.
_Avoid_: Media Asset, Stored Object, MediaRef
Owner: `apps/web/src/features/library/CONTEXT.md`

**Media Asset**:
Managed material inside a Workspace with stable identity and original facts.
_Avoid_: Library Asset, Stored Object, URL as identity
Owner: `apps/api/internal/media/storage/CONTEXT.md`

**Task**:
Durable unit of AI work. Insert plus River enqueue in one transaction.
_Avoid_: River Job as Task, Attempt as Task, Execution Plan as Task
Owner: `apps/api/internal/tasking/CONTEXT.md`

**Task Readiness**:
Advisory assessment of whether an Actor can submit a Task. It reserves no Credits or work; submission remains authoritative.
_Avoid_: Credit Hold, Task reservation
Owner: `apps/api/internal/tasking/CONTEXT.md`

**Execution Plan**:
Preparation result turning nodes into one or more Tasks. Not yet submitted.
_Avoid_: Task, Pending Result Node
Owner: `apps/api/internal/tasking/CONTEXT.md`

**Service**:
Executable workflow capability (`generation`, `render`, `inpaint`, ...). Top-level discriminant is `service`.
_Avoid_: feature as discriminant, `params.service` as second source
Owner: `apps/api/internal/modelregistry/CONTEXT.md`

**PriceSnapshot**:
Immutable Billing pricing fact for quoting. No quantity, no routing.
_Avoid_: Quote as price fact, Task Billing Snapshot
Owner: `apps/api/internal/billing/CONTEXT.md`

**RoutingSnapshot**:
Immutable Model Pool selection pinned at submit. Worker reads snapshot only.
_Avoid_: live pool, Quote as snapshot
Owner: `apps/api/internal/modelregistry/CONTEXT.md`
