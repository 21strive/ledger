package models

import "time"

type LedgerAccount struct {
	UUID      string    `json:"uuid" db:"uuid"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
	Name      string    `json:"name" db:"name"`
	Email     string    `json:"email" db:"email"`
}
