package repo

import "github.com/21strive/ledger/ledgererr"

var (
	ErrNotFound        = ledgererr.NewError(ledgererr.CodeNotFound, "record not found", nil)
	ErrFailedInsertSQL = ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to insert record", nil)
	ErrFailedQuerySQL  = ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to query records", nil)
)
