# IdeaBoard

Ubiquitous language for Tenancy, Billing, Boards and Canvas execution. `DESIGN.md` owns contracts, `CONTEXT.md` owns local rules, this file owns canonical names.

## Language

**Idea**:
User-facing unit of AI consumption. Display only.
_Avoid_: Credit in UI copy, coin, point

**Credit**:
Internal integer unit of consumption. Code, API and schema only.
_Avoid_: Idea in code, Credit in UI copy

**Billing Account**:
Stable funds identity (`ba_`). Owns Wallets and Ledger.
_Avoid_: Workspace Billing Account, organization balance, account alone

**Credit Wallet**:
Balance container owned by one Billing Account. Projection of Ledger.
_Avoid_: funds pool, balance account

**Tenant Workspace**:
Isolation boundary for Boards, Tasks, Library and Media. Records consumption position, holds no balance.
_Avoid_: Canvas Workspace, Workspace Shell, team

**Canvas Workspace**:
Single-Board editing interaction surface. Not a tenancy or funds boundary.
_Avoid_: Tenant Workspace, Workspace Shell

**Organization**:
Membership and administration boundary. Never consumes or holds boards.
_Avoid_: team account, organization balance

**Business User**:
Human or managed identity operating in product. Public reference is `public_id` (`usr_`).
_Avoid_: auth_user_id, internal id, display_name as key

**Actor**:
Operation performer recorded on Ledger and snapshots. Person or service, never a funds identity.
_Avoid_: payer, member, holder as Actor

**Board Document**:
Persisted graph document. Sole fact source for a Board.
_Avoid_: Board page, Board card, Preview Scene

**Library Asset**:
Saved collection reference, either external inspiration or managed Media. Identity is `provider:sourceId` or `media:mediaAssetId`.
_Avoid_: Media Asset, Stored Object, MediaRef

**Media Asset**:
Managed material inside a Workspace with stable identity and original facts.
_Avoid_: Library Asset, Stored Object, URL as identity

**Task**:
Durable unit of AI work. Insert plus River enqueue in one transaction.
_Avoid_: River Job as Task, Attempt as Task, Execution Plan as Task

**Execution Plan**:
Preparation result turning nodes into one or more Tasks. Not yet submitted.
_Avoid_: Task, Pending Result Node

**Service**:
Executable workflow capability (`generation`, `render`, `inpaint`, ...). Top-level discriminant is `service`.
_Avoid_: feature as discriminant, `params.service` as second source

**PriceSnapshot**:
Immutable Billing pricing fact for quoting. No quantity, no routing.
_Avoid_: Quote as price fact, Task Billing Snapshot

**RoutingSnapshot**:
Immutable Model Pool selection pinned at submit. Worker reads snapshot only.
_Avoid_: live pool, Quote as snapshot
