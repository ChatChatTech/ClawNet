package store

import "errors"

// ErrInsufficientCredits is returned when a peer doesn't have enough balance.
var ErrInsufficientCredits = errors.New("insufficient credits")
