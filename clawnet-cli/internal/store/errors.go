package store

import "errors"

// ErrInsufficientCredits is returned when a peer doesn't have enough balance.
var ErrInsufficientCredits = errors.New("insufficient credits")

// ErrTaskStateConflict is returned when a task state transition fails because
// the task is not in the expected status (e.g. approving a non-submitted task).
var ErrTaskStateConflict = errors.New("task state conflict: not in expected status")
