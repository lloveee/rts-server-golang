package replay

import "errors"

var (
	ErrTruncated = errors.New("replay: data truncated")
	ErrBadMagic  = errors.New("replay: invalid magic bytes")
	ErrDesync    = errors.New("replay: hash mismatch (desync)")
)
