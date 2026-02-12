package ledger

import "time"

type BalanceResponse struct {
	PendingBalance   int64
	AvailableBalance int64
	TotalBalance     int64
	Currency         string
	LastSyncedAt     *time.Time
	Warning          string
}
