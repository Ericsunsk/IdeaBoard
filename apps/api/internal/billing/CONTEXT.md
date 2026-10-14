# Billing Context

Billing is the platform-level product identity and Credit pricing context. Users see the currency name `Idea/Ideas`; internal price fields and domain terminology keep using `Credit/credits`. It provides the global SKU Catalog, append-only price versions, and server-side Quote for current and future IdeaBoard products; it owns no Wallet, Ledger, Model selection, subscription, or invoice. Layout: root vocabulary and contracts, `application` catalog orchestration, `infrastructure/postgres` Store, `interfaces/http` handler.

## Domain Language

| Term                   | Meaning                                                                                                                                                     |
| ---------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| SKU                    | The immutable global identity of a billable product capability.                                                                                             |
| Billable Item          | A product capability registered for pricing, independent of the Model used to execute it.                                                                   |
| Billable Item Registry | Billing-owned immutable registration seam; runtime composes product-context definitions, rejects duplicate SKUs, returns deterministic copies               |
| Catalog Read Model     | Dynamically merges the current Registry with persisted SKU/current-price facts into Admin cards; needs no startup write or sync job                         |
| SKU namespace          | Product-owned 1–32 char lowercase ASCII namespace, may contain `-`                                                                                          |
| local key              | 1–95 char product key inside a namespace, may contain `.`, `_`, `-`                                                                                         |
| Price Version          | An immutable unit price for a SKU and Output Resolution, ordered within that pair.                                                                          |
| Current Price          | The Price Version currently used to Quote a SKU at a particular Output Resolution.                                                                          |
| Archived               | A persisted Catalog item no longer in the current Registry; cannot be Quoted or enabled, Price Versions survive until the whole aggregate is safely deleted |
| Price Snapshot         | The immutable price facts accepted when a Quote is issued.                                                                                                  |
| Quote                  | A server-calculated price for a particular SKU, Output Resolution, and quantity; it reserves no Credits.                                                    |
| Admin Action           | One Admin write against Catalog/Price; carries authenticated actor, target, old/new, outcome and time, persisted as append-only audit                       |

## Public Seams

- `BillableItemRegistry.Items()` is the read-only product pricing-identity seam; the current Workflow adapter uses `ai-image:<feature-slug>`, future products register their own namespace.
- `BillableItemCatalog.List(ctx)` is the Admin feature-pricing read seam; Registry items without persisted prices render `Unpriced`, persisted items missing from the Registry render `Archived`.
- `BillableItemCatalog.PublishPrices(ctx, sku, prices, actorID)` is the feature-price publish seam; it takes one `unitCredits` per canonical `OutputResolution`, accepts only Registry Billable Items, atomically materializes the Catalog item on first publish and flips it `Active`.
- `BillableItemCatalog.SetAvailability(ctx, sku, enabled, actorID)` is the Feature availability write seam; requires an existing current price, `Unpriced` cannot be enabled, repeated commands stay idempotent with truthful outcomes.
- `BillableItemCatalog.DeleteArchived(ctx, sku, actorID)` accepts only items absent from the current Registry; the database command re-checks durable references (Task Billing Snapshots, etc.) inside the SKU row lock. Success deletes the Catalog aggregate (SKU + Price Versions), keeps existing Admin Audit and appends a `delete_archived` audit.
- `BillableItemCatalog.QuoteAtResolution/QuoteAtResolutionInTx` is the Registry-aware runtime seam; `Unpriced`, `Disabled` and `Archived` all fail closed. Task submission uses `QuoteAtResolutionInTx` to keep Snapshot, Hold, Task and River enqueue inside one transaction boundary.
- `PriceSnapshot` carries `sku`, `display_name`, `output_resolution`, `price_version_id`, `price_version`, `unit_credits`, `quoted_at`; later Hold/Capture must save and reuse the snapshot, never re-quote. `display_name` freezes at Quote time; later SKU renames must not rewrite historical task billing.
- User-side Billing Records read only the frozen `display_name` from Task Billing Snapshots; they must not re-resolve current SKU display fields to rewrite historical names.
- Admin feature pricing only publishes new immutable Price Versions; no Price Version enable/disable command. Database commands record actor, target, old/new and outcome automatically.
- `create_sku`, `update_sku`, `set_price_enabled` survive only as historical append-only Admin Audit vocabulary, not as runtime or Admin command contracts.
- Registry SKU identity has no rename/delete contract; deactivation uses `enabled=false`. Only Archived aggregates whose Registry entry is gone and which hold no durable reference may be hard-deleted.
- Admin Catalog, Quote and Task Billing uniformly use the Service-level identity `ai-image:<service>`. Provider Model belongs only to the execution/idempotency contract and never to user price identity; switching models never changes Service prices. Billing eligibility comes from the Workflow registry AI image product list, not from Models or provider credentials.
- The application catalog orchestrates the immutable Registry against the `CatalogStore` persistence seam; the `infrastructure/postgres` Store owns all SQL, and Admin HTTP stays behind narrow handler interfaces.

## Invariants

- SKU canonicalization trims whitespace and lowercases ASCII; the form is `<namespace>:<local-key>` with a maximum length of 128 characters.
- Registry SKUs are globally unique and cannot be renamed; display name and description may change. Re-registering a SKU after an Archived hard delete re-materializes it under the new Registry definition.
- Price Versions are strictly append-only with no mutable enabled state; restoring an old price must publish a new version.
- Every publish locks the SKU row and, inside one typed PostgreSQL command, materializes the Catalog item (if needed), allocates a monotonic version per SKU and Output Resolution, inserts the prices, moves the corresponding current pointers, and writes append-only audit.
- Items published for the first time or still without a current price auto-enable; Disabled items with a current price stay Disabled across repricing.
- Feature availability is owned solely by the Billing Catalog SKU enabled state; Feature Prompts carry no enabled/published state. Enable/disable commands record actor, target, old/new, outcome and time automatically.
- Quote reads SKU, current pointer and Price Version in one MVCC statement and never mixes concurrent versions.
- quantity must be a positive `int64`; `unit_credits × quantity` beyond `9_000_000_000_000_000` fails closed.
- Explicit 0 Credits is a free price; missing SKU, disabled SKU, or no current price fails closed.
- Billing never selects Models; changing a Service Model Pool cannot change its prices.
- The Registry is a constructor-owned immutable value with no global mutable registration, avoiding concurrent-startup and test pollution.
- The API runtime role holds only SELECT on Catalog/Price tables with no direct rights on `billing_admin_actions`; all writes go through typed `SECURITY DEFINER` functions with fixed trusted schema, `pg_catalog`, `pg_temp` search path and revoked PUBLIC.
- Archived delete serializes concurrent Snapshot FK acquisition under the SKU row lock; any durable reference rolls the whole command back with neither Price/SKU deletion nor delete audit.
- Admin history renders dynamically whether the current Registry still contains each feature; user-side billing always reuses the immutable Snapshot `display_name` with no deletion-state suffix.
- Archived aggregate deletion is irreversible without a pre-deletion database snapshot; it never deletes linked Ledger facts.

## Non-goals

- No tiered pricing, scheduled pricing, subscription, invoice, tax, promotion, or physical DELETE of Catalog items carrying durable Billing history in v1.
