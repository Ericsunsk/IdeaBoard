package domain

import "errors"

var (
	ErrAccountNotFound      = errors.New("account not found")
	ErrAccountNameConflict  = errors.New("account name conflict")
	ErrAccountModelNotFound = errors.New("account model not found")
)
