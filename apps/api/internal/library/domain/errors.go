package domain

import "errors"

var (
	ErrInvalidFolderName       = errors.New("library folder name is invalid")
	ErrFolderNameConflict      = errors.New("library folder name already exists")
	ErrSystemFolderImmutable   = errors.New("library system folder is immutable")
	ErrFolderNotFound          = errors.New("library folder not found")
	ErrInvalidProvider         = errors.New("library provider is invalid")
	ErrInvalidSource           = errors.New("library source is invalid")
	ErrAssetNotFound           = errors.New("library asset not found")
	ErrInvalidTargetFolder     = errors.New("library target folder is invalid")
	ErrInvalidCursor           = errors.New("library cursor is invalid")
	ErrInvalidPageSize         = errors.New("library page size is invalid")
	ErrInvalidOwner            = errors.New("library owner is invalid")
	ErrMaterializerUnavailable = errors.New("library asset materializer is unavailable")

	// ErrV1FolderNotFound reports an unknown folder within a visible
	// workspace (hidden/cross-workspace/malformed ids converge here).
	ErrV1FolderNotFound = errors.New("library v1 folder not found")
	// ErrV1AssetNotFound reports an unknown asset within a visible
	// workspace.
	ErrV1AssetNotFound = errors.New("library v1 asset not found")
	// ErrV1MaterializationNotFound reports an unknown materialization.
	ErrV1MaterializationNotFound = errors.New("library v1 materialization not found")
	// ErrV1FolderNameConflict reports a duplicate folder name in one
	// workspace.
	ErrV1FolderNameConflict = errors.New("library v1 folder name already exists")
	// ErrV1ResolveTooMany reports a resolve batch above V1MaxResolveItems.
	ErrV1ResolveTooMany = errors.New("library v1 resolve batch too large")
	// ErrV1MaterializationUnavailable reports that the durable
	// materialization job surface has no postgres mapping yet. The
	// workspace tables exist but completing a job needs a managed Media
	// Asset link (media resolution + settlement land in #820); the
	// postgres Service fails these methods closed so handlers render 503
	// instead of half-persisting jobs.
	ErrV1MaterializationUnavailable = errors.New("library v1 materialization is unavailable")
)
