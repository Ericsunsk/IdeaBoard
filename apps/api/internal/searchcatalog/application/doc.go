// Package application documents the searchcatalog application split.
//
// The application cluster exceeded the package file limit, so logic now
// lives in two subpackages:
//
//   - search: keyword search over Unsplash/Pexels (SearchService).
//   - vision: vision-search extraction, billing, settlement, reconciliation.
//
// Shared bound QueryMaxRunes lives in domain/vocabulary.go (single
// source). Application subpackages alias it (search.MaxSearchQueryRunes,
// vision.MaxSearchQueryRunes) instead of redeclaring the literal, and the
// v1 workspace edge aliases it as well (V1MaxQueryRunes). Error copy stays
// with errorrender.MsgNormalizeQuery.
package application
