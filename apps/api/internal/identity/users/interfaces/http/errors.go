package http

import (
	"ideaboard/api/internal/platform/contract"
)

// Identity internal HTTP wire codes are stable. Do not rename a code
// without coordinating frontend callers that branch on it. Messages stay
// intentionally safe for public display.
//
// The values below alias the kernel registry
// (platform/contract.CodeIdentity*): the registry is the single source of
// truth, these names keep the edge diffs trivial. Wire values are
// unchanged.
const (
	codeIdentityRequestInvalid = string(contract.CodeIdentityRequestInvalid)
	codeIdentityUnauthorized   = string(contract.CodeIdentityUnauthorized)
)
