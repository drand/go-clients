package drand

import (
	"errors"
)

// ErrInvalidChainHash means there was an error or a mismatch with the chain hash
var ErrInvalidChainHash = errors.New("incorrect chain hash")

// ErrEmptyClientUnsupportedGet means this client does not support Get
var ErrEmptyClientUnsupportedGet = errors.New("unsupported method Get was used")
