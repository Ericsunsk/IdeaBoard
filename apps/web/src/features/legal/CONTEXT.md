# Legal Context

Legal owns the platform terms, privacy policy, and compliance pages for anonymous visitors and registered users, with version tracking and print layout.
It does NOT own auth, business state, or checkout logic.

## Domain Language

| Term               | Meaning                                                                                                                              |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------ |
| Legal Document     | Versioned, effective-dated legal text structure in `legalDocuments.ts`: title, `version`, effective date, formatted paragraphs.      |
| Public Legal Shell | Shared legal display shell in `components/PublicLegalPage.vue`: doc TOC jumps, readability typography, print styles, safe back link. |

## Public Seams

- `legalDocuments` constant export: the fact source for legal text structures, version marks, and multi-language clauses.
- `components/PublicLegalPage.vue` base display component: TOC, responsive reader, top back nav; reader typography only.
- Route pages (`pages/*Page.vue` for Terms, Privacy, Refund, AcceptableUse): thin assembly reading `legalDocuments` into `PublicLegalPage`.

## Invariants

- Every legal text change must bump `version` and the update time for signup and checkout audit.
- Pages stay light with no auth dependency; fully SSR- or static-renderable.
- Never scatter hand-written static dead text across Vue components without version tracking.
- Never duplicate outer nav and typography CSS per legal page.

## Non-goals

- No business-side complex state in the legal shell.
- No Vue dependency in the `legalDocuments` data definitions.
