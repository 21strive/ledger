package repo

import "github.com/21strive/ledger/domain/domainerr"

var (
	ErrNotFound = domainerr.NewError(domainerr.CodeNotFound, "record not found", nil)
)
