/**
 * @deprecated P1b compatibility shim for the `shared/seo` → `core/seo` move (§5.5).
 * TTL: 1 phase — delete after P1 (retirement is a later package, not this one).
 * Deletion owner: p1b-core. Do not import this module in new code; use `core/seo` instead.
 */
export * from '../../core/seo/seoStaticHtml'
