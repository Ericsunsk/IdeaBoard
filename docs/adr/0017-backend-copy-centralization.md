---
owner: API
decision_date: 2026-10-19
supersedes: none
---

# Backend Copy Centralization to platform/errorrender

## Context

Backend user-facing copy (mostly Simplified Chinese) lives in per-context
tables: five `internal/**/message.go` files plus Han-literal `internal/**/errors.go`
extensions and a handful of mixed files (`service.go`, `observed.go`,
`atomic.go`/`guard.go`, `ports/errors.go`). The working tree holds ~680
`msg*` constants across 59 files. Wording is managed centrally in the
"Backend Copy Vocabulary" table of `apps/api/CONTEXT.md`, but storage stays
distributed per the same section; every package owns its copy block as the
"single source of truth for one rendered message".

Distributed storage scales poorly: copy review needs 60+ files, renames
collide (`msgGet`, `msgList`, `msgPackage`, `msgImpersonateUser*` and 17 more
duplicated names hold different values in different packages), and the
mechanical pin (`scripts/check-backend-copy.sh`) must enumerate every
`message.go`/`errors.go` table. `apps/api/CONTEXT.md` says "storage stays
distributed"; centralizing storage directly conflicts with that sentence.

## Decision

1. All lowercase `msg*` user-facing copy centralizes into
   `apps/api/internal/platform/errorrender/copy.go` (package `errorrender`,
   pure string constants, no domain imports so no import cycle). Wording is
   byte-identical; only the identifier and the home move.
2. Central identifiers are exported and globally unique: a unique `msgFoo`
   becomes `MsgFoo`; a duplicated name becomes `Msg<PackagePrefix>Foo`
   where the prefix derives from the originating directory (e.g. `msgGet`
   splits into `MsgAigatewayApplicationGet` vs `MsgPromptsGet`). Call sites
   reference `errorrender.<Name>`.
3. Every package `errors.go` keeps only sentinels (`var Err*`); the
   `// Backend-owned user-facing copy` const blocks move out. Pure copy
   tables (the five `message.go` files, plus copy-only `errors.go` files
   with no sentinels) are deleted. Mixed files keep their code and lose
   only the copy block.
4. `scripts/check-backend-copy.sh` scans the centralized home (`copy.go`)
   in addition to the legacy names, so skipped or transitional packages
   stay pinned. Its header and the `apps/api/CONTEXT.md` vocabulary
   paragraph no longer claim distributed storage.
5. Scope boundary (deliberately NOT centralized):
   - The exported kernel envelope table
     `internal/runtime/interfaces/http/errors.go` (`Msg*`, consumed as
     `runtimehttp.Msg*` by `v1/` and `control/`, pinned by
     `TestErrorResponsesStayInsideSharedEnvelope`) stays where it is. It is
     already a single table with a kernel-owner contract; moving 212
     constants plus ~530 qualified references (with 4 `Msg`/`msg` name
     overlaps) is follow-up work, not part of this decision.
   - Three payments packages keep their local `msg*` tables because their
     architecture tests forbid internal imports: `application/contracts`
     (`TestContractsDependOnlyOnPaymentsDomainAndPorts`),
     `domain` (`TestDomainPackageHasNoInfrastructureImports`), and `ports`
     (`TestPortsPackageHasNoBusinessContextImports`). Centralizing them
     would force `platform/errorrender` imports into sentinel files and
     fail those gates. They stay scanned by `check-backend-copy.sh` via
     the legacy `errors.go` name.
6. Conflict ruling: against `apps/api/CONTEXT.md` "storage stays
   distributed", this ADR rules **centralized**. The old sentence is
   updated in the same change (CONTEXT.md vocabulary paragraph plus the
   `check-backend-copy.sh` header that mirrors it).

## Consequences

- Copy review, banned-variant pinning, and wording audits touch one file
   (`errorrender/copy.go`) plus the CONTEXT.md vocabulary table.
- `platform/errorrender` stays dependency-free (only `platform/response`
   plus std/third-party rendering deps); domains import it for copy, never
   the reverse, so no cycle.
- Per-package diffs are mechanical (identifier rename + import add +
   const-block delete); wire codes, statuses, and rendered strings are
   unchanged.
- Residual: the `runtimehttp.Msg*` kernel table remains the only Chinese
   copy outside `errorrender` alongside the three skipped payments tables;
   a future ADR can centralize them once the `v1`/`control` qualified
   references plus the envelope architecture test, and the payments
   dependency-free gates, are ready to move together.

## Follow-up: runtimehttp kernel convergence (2026-10-19)

Decision item 5 left the exported kernel envelope table
(`internal/runtime/interfaces/http/errors.go`, `Msg*`, consumed as
`runtimehttp.Msg*` by `v1/` and `control/`) in place as follow-up work.
That work is now done, converging into `errorrender/copy.go`:

1. Direction: the 212 kernel values moved into `errorrender/copy.go`
   (new "Runtime HTTP kernel copy" const block), and `errors.go` keeps
   alias-only consts (`MsgX = errorrender.MsgX`). The reverse (errorrender
   aliasing the kernel) was rejected: the kernel already imports
   `errorrender`, so canonical-in-kernel would be an import cycle.
2. The 4 identifiers colliding with existing centralized names
   (`MsgConflict`, `MsgCreateCheckoutSession`,
   `MsgCreateCheckoutSession2`, `MsgSearchProviderUnavailable` — all four
   hold different wording on each side) forward to new
   `MsgRuntimeInterfacesHTTP*` names per the duplicate-prefix rule.
   The other 208 kept their kernel names.
3. All ~530 qualified `runtimehttp.Msg*` references plus the kernel's own
   unqualified uses are untouched and resolve through the aliases;
   wording is byte-identical on all 212 constants (verified against the
   pre-move literals). No call-site, wire-code, or status change.
4. `scripts/check-backend-copy.sh` pins the alias-only shape (any string
   literal in the kernel table fails) and `check-backend-copy-test.sh`
   covers it; the banned-variant scan is unchanged.

Residual: the three skipped payments tables from item 5 stay as-is under
their dependency-free gates. Call sites may migrate from `runtimehttp.Msg*`
to `errorrender.Msg*` opportunistically; the aliases are the compatibility
shim and must not be removed until no qualified reference remains.

## Status

Accepted
