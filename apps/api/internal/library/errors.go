package library

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
)
