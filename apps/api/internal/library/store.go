package library

import (
	"ideaboard/api/internal/library/ports"
)

// WorkspaceLibraryStore is the persistence seam for workspace-scoped
// library state (canonical definition in ports; signatures frozen).
type WorkspaceLibraryStore = ports.WorkspaceLibraryStore

// Compile-time convergence: the postgres Service and the test-only
// in-memory store both satisfy the seam through these aliases.
var _ WorkspaceLibraryStore = (*Service)(nil)
var _ WorkspaceLibraryStore = (*InMemoryWorkspaceLibraryStore)(nil)
