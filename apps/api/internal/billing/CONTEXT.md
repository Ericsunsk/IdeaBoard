# Billing Context

Billing is the platform-level product identity and Credit pricing context. Users see the currency name `Idea/Ideas`; internal price fields and domain terminology keep using `Credit/credits`. It provides the global SKU Catalog, append-only price versions, and server-side Quote for current and future IdeaBoard products; it owns no Wallet, Ledger, Provider Account routing, subscription, or invoice. Layout: `domain` vocabulary, `application` catalog orchestration, `infrastructure/postgres` Store, `interfaces/http` handler.

## Domain Language

| Term | Meaning |
| ---- | ------- |
| SKU | Global immutable product code, `<namespace>:<local-key>`; canonicalization is trim + lowercase ASCII, max 128 chars |
| Billable Item | A product-context capability registered for pricing; carries stable SKU, namespace, product kind, display copy and ordering, never a Provider Model |
| Billable Item Registry | Billing-owned immutable registration seam; runtime composes product-context definitions, rejects duplicate SKUs, returns deterministic copies |
| Catalog Read Model | Dynamically merges the current Registry with persisted SKU/current-price facts into Admin cards; needs no startup write or sync job |
| SKU namespace | Product-owned 1–32 char lowercase ASCII namespace, may contain `-` |
| local key | 1–95 char product key inside a namespace, may contain `.`, `_`, `-` |
| Price Version | Append-only unit-price fact per SKU; `version` increases monotonically per SKU, `unit_credits` is an indivisible integer and may explicitly be 0 |
| Current Price | The SKU's atomic pointer at the newest Price Version; creating a version and switching the pointer complete in one database command |
| Archived | A persisted Catalog item no longer in the current Registry; cannot be Quoted or enabled, Price Versions survive until the whole aggregate is safely deleted |
| Price Snapshot | The immutable value object returned by Quote and persisted by later Billing Commands |
| Quote | The result of current Price Snapshot × positive integer quantity, computed server-side |
| Admin Action | One Admin write against Catalog/Price; carries authenticated actor, target, old/new, outcome and time, persisted as append-only audit |

## Public Seams

- `BillableItemRegistry.Items()` is the read-only product pricing-identity seam; the current Workflow adapter uses `ai-image:<feature-slug>`, future products register their own namespace.
- `BillableItemCatalog.List(ctx)` is the Admin feature-pricing read seam; Registry items without persisted prices render `Unpriced`, persisted items missing from the Registry render `Archived`.
- `BillableItemCatalog.PublishPrice(ctx, sku, unitCredits, actorID)` is the feature-price publish seam; accepts only Registry Billable Items, atomically materializes the Catalog item on first publish and flips it `Active`.
- `BillableItemCatalog.SetAvailability(ctx, sku, enabled, actorID)` is the Feature availability write seam; requires an existing current price, `Unpriced` cannot be enabled, repeated commands stay idempotent with truthful outcomes.
- `BillableItemCatalog.DeleteArchived(ctx, sku, actorID)` accepts only items absent from the current Registry; the database command re-checks durable references (Task Billing Snapshots, etc.) inside the SKU row lock. Success deletes the Catalog aggregate (SKU + Price Versions), keeps existing Admin Audit and appends a `delete_archived` audit.
- `BillableItemCatalog.Quote/QuoteInTx` is the Registry-aware runtime seam; `Unpriced`, `Disabled` and `Archived` all fail closed. Task submission uses `QuoteInTx` to keep Snapshot, Hold, Task and River enqueue inside one transaction boundary.
- `Service.Quote(ctx, sku, quantity)` is the main product price-read seam. Callers pass only SKU and quantity, never a price.
- `PriceSnapshot` carries `sku`, `display_name`, `price_version_id`, `price_version`, `unit_credits`, `quoted_at`; later Hold/Charge must save and reuse the snapshot, never re-quote. `display_name` freezes at Quote time; later SKU renames must not rewrite historical task billing.
- User-side Billing Records read only the frozen `display_name` from Task Billing Snapshots; they must not re-resolve current SKU display fields to rewrite historical names.
- Admin feature pricing only publishes new immutable Price Versions; no Price Version enable/disable command and no Admin-typed reason required. Database commands record actor, target, old/new and outcome automatically.
- `create_sku`, `update_sku`, `set_price_enabled` survive only as historical append-only Admin Audit vocabulary, not as runtime or Admin command contracts.
- Registry SKU identity has no rename/delete contract; deactivation uses `enabled=false`. Only Archived aggregates whose Registry entry is gone and which hold no durable reference may be hard-deleted.
- Admin Catalog, Quote and Task Billing uniformly use the Service-level identity `ai-image:<service>`. Provider Model belongs only to the execution/idempotency contract and never to user price identity; switching models never changes Service prices. Billing eligibility comes from the Workflow registry AI image product list, not from Provider Accounts.
- The application catalog orchestrates the immutable Registry against the `CatalogStore` persistence seam; the `infrastructure/postgres` Store owns all SQL, and Admin HTTP stays behind narrow handler interfaces.

## Invariants

- Registry SKUs are globally unique and cannot be renamed; display name and description may change. Re-registering a SKU after an Archived hard delete re-materializes it under the new Registry definition.
- Price Versions are strictly append-only with no mutable enabled state; restoring an old price must publish a new version.
- Every publish locks the SKU row and, inside one typed PostgreSQL command, materializes the Catalog item (if needed), allocates a monotonic version, inserts the price, moves the current pointer, and writes append-only audit.
- Items published for the first time or still without a current price auto-enable; Disabled items with a current price stay Disabled across repricing.
- Feature availability is owned solely by the Billing Catalog SKU enabled state; Prompt revisions carry no enabled/published state. Enable/disable commands record actor, target, old/new, outcome and time automatically.
- Quote reads SKU, current pointer and Price Version in one MVCC statement and never mixes concurrent versions.
- quantity must be a positive `int64`; `unit_credits × quantity` beyond `9_000_000_000_000_000` fails closed.
- Explicit 0 Credits is a free price; missing SKU, disabled SKU, or no current price fails closed.
- Billing never imports account-pool routing; prices never follow Provider Accounts.
- The Registry is a constructor-owned immutable value with no global mutable registration, avoiding concurrent-startup and test pollution.
- The API runtime role holds only SELECT on Catalog/Price tables with no direct rights on `billing_admin_actions`; all writes go through typed `SECURITY DEFINER` functions with fixed trusted schema, `pg_catalog`, `pg_temp` search path and revoked PUBLIC.
- Archived delete serializes concurrent Snapshot FK acquisition under the SKU row lock; any durable reference rolls the whole command back with neither Price/SKU deletion nor delete audit.
- Admin history renders dynamically whether the current Registry still contains each feature; user-side billing always reuses the immutable Snapshot `display_name` with no deletion-state suffix.
- Archived hard-delete migration down only supports schema rollback before any real deletion; exact recovery after deletion requires a pre-migration database snapshot.

## Non-goals

- No tiered pricing, scheduled pricing, subscription, invoice, tax, promotion, or physical DELETE of Catalog items carrying durable Billing history in v1.
