package models

import (
	"github.com/21strive/redifu"
)

type LedgerAccount struct {
	*redifu.Record
	Name       string `json:"name" db:"name"`
	ExternalId string `json:"external_id" db:"external_id"`
}
