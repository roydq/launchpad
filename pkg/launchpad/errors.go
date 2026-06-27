package launchpad

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrForbidden     = errors.New("forbidden")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrBadRequest    = errors.New("bad request")
	ErrNotImplemented = errors.New("not implemented")
)