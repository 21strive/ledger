package models

import (
	"github.com/21strive/redifu"
)

type LedgerAccount struct {
	*redifu.Record
	Name  string `json:"name" db:"name"`
	Email string `json:"email" db:"email"`
}
