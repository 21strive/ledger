package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Money struct {
	Amount   int64
	Currency Currency
}

type Currency string

const (
	CurrencyIDR Currency = "IDR"
	CurrencyUSD Currency = "USD"
)

type OwnerType string

const (
	OwnerTypeSeller         OwnerType = "SELLER"
	OwnerTypePlatform       OwnerType = "PLATFORM"
	OwnerTypePaymentGateway OwnerType = "PAYMENT_GATEWAY"
)

type Account struct {
	ID               string    `json:"id"`
	DokuSubAccountID string    `json:"doku_sub_account_id,omitempty"`
	OwnerType        OwnerType `json:"owner_type"`
	OwnerID          string    `json:"owner_id"`
	Currency         Currency  `json:"currency"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func NewAccount(ownerType OwnerType, dokuSubAccountID string, ownerID string, currency Currency) Account {
	return Account{
		ID:               uuid.New().String(),
		DokuSubAccountID: dokuSubAccountID,
		OwnerType:        ownerType,
		OwnerID:          ownerID,
		Currency:         currency,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
}

func NewPlatformAccount(dokuSubAccountID string, ownerID string, currency Currency) Account {
	return NewAccount(OwnerTypePlatform, dokuSubAccountID, ownerID, currency)
}

func NewSellerAccount(dokuSubAccountID string, sellerId string, currency Currency) Account {
	return NewAccount(OwnerTypeSeller, dokuSubAccountID, sellerId, currency)
}

func NewPaymentGatewayAccount(dokuSubAccountID string, ownerID string, currency Currency) Account {
	return NewAccount(OwnerTypePaymentGateway, dokuSubAccountID, ownerID, currency)
}

type AccountRepository interface {
	GetByID(ctx context.Context, id string) (*Account, error)
	GetByOwner(ctx context.Context, ownerType OwnerType, ownerID string) (*Account, error)
	GetByDokuSubAccountID(ctx context.Context, dokuSubAccountID string) (*Account, error)
	GetBySellerID(ctx context.Context, sellerId string) (*Account, error)
	GetPlatformAccount(ctx context.Context) (*Account, error)
	GetPaymentGatewayAccount(ctx context.Context) (*Account, error)
	Save(ctx context.Context, account *Account) error
	Delete(ctx context.Context, id string) error
}
