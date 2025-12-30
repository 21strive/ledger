package usecases

import (
	"fmt"
	"net/http"
	"time"

	"github.com/21strive/ledger/models"
	"github.com/21strive/ledger/repositories"
	"github.com/21strive/ledger/requests"
	"github.com/21strive/ledger/utils/helper"
	"github.com/21strive/redifu"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

type LedgerPaymentUseCaseInterface interface {
	CreatePayment(sqlTransaction *sqlx.Tx, request *requests.LedgerPaymentCreatePaymentRequest) (*models.LedgerPayment, *models.ErrorLog)
	ConfirmPayment(sqlTransaction *sqlx.Tx, request *requests.LedgerPaymentConfirmPaymentRequest) (*models.LedgerPayment, *models.ErrorLog)
	FailPayment(sqlTransaction *sqlx.Tx, request *requests.LedgerPaymentFailPaymentRequest) (*models.LedgerPayment, *models.ErrorLog)
	ExpirePayments(sqlTransaction *sqlx.Tx) (int, *models.ErrorLog)
	GetPaymentByUUID(uuid string) (*models.LedgerPayment, *models.ErrorLog)
	GetPaymentByInvoiceNumber(invoiceNumber string) (*models.LedgerPayment, *models.ErrorLog)
	GetPaymentByGatewayRequestId(gatewayRequestId string) (*models.LedgerPayment, *models.ErrorLog)
	GetPendingPaymentByInvoice(invoiceNumber string) (*models.LedgerPayment, *models.ErrorLog)
	GetPaymentsByLedgerAccountUUID(ledgerAccountUUID string) ([]*models.LedgerPayment, *models.ErrorLog)
	GetPaymentsByStatus(status string) ([]*models.LedgerPayment, *models.ErrorLog)
}

type ledgerPaymentUseCase struct {
	ledgerPaymentRepository     repositories.LedgerPaymentRepositoryInterface
	ledgerWalletRepository      repositories.LedgerWalletRepositoryInterface
	ledgerTransactionRepository repositories.LedgerTransactionRepositoryInterface
	ledgerWalletUseCase         LedgerWalletUseCaseInterface
	ledgerPendingBalanceUseCase LedgerPendingBalanceUseCaseInterface
}

func NewLedgerPaymentUseCase(
	dbRead *sqlx.DB,
	dbWrite *sqlx.DB,
	redis redis.UniversalClient,
) LedgerPaymentUseCaseInterface {

	ledgerPaymentRepository := repositories.NewLedgerPaymentRepository(dbRead, dbWrite)
	ledgerWalletRepository := repositories.NewLedgerWalletRepository(dbRead, dbWrite)
	ledgerTransactionRepository := repositories.NewLedgerTransactionRepository(dbRead, dbWrite)
	ledgerWalletUseCase := NewLedgerWalletUseCase(dbRead, dbWrite, redis)
	ledgerPendingBalanceUseCase := NewLedgerPendingBalanceUseCase(dbRead, dbWrite)

	return &ledgerPaymentUseCase{
		ledgerPaymentRepository:     ledgerPaymentRepository,
		ledgerWalletRepository:      ledgerWalletRepository,
		ledgerTransactionRepository: ledgerTransactionRepository,
		ledgerWalletUseCase:         ledgerWalletUseCase,
		ledgerPendingBalanceUseCase: ledgerPendingBalanceUseCase,
	}
}

// CreatePayment creates a new payment record with PENDING status
// If an existing PENDING payment for the same invoice exists and is not expired,
// it returns the existing payment (so caller can reuse the payment URL)
func (u *ledgerPaymentUseCase) CreatePayment(sqlTransaction *sqlx.Tx, request *requests.LedgerPaymentCreatePaymentRequest) (*models.LedgerPayment, *models.ErrorLog) {

	now := time.Now().UTC()

	// 1. Check for existing PENDING payment with same invoice
	existing, _ := u.ledgerPaymentRepository.GetPendingByInvoiceNumber(request.InvoiceNumber)
	if existing != nil {
		// Check if not expired
		if existing.ExpiresAt.After(now) {
			// Return existing payment - caller should use existing URL
			return existing, nil
		}
		// If expired, mark as EXPIRED
		existing.Status = models.PaymentStatusExpired
		errorLog := u.ledgerPaymentRepository.Update(sqlTransaction, existing)
		if errorLog != nil {
			return nil, errorLog
		}
	}

	// 2. Get or Create LedgerWallet for this account + currency
	wallet, errorLog := u.ledgerWalletUseCase.CreateWallet(sqlTransaction, request.LedgerAccountUUID, request.Currency)
	if errorLog != nil {
		return nil, errorLog
	}

	// 3. Create new LedgerPayment with PENDING status
	payment := &models.LedgerPayment{}
	redifu.InitRecord(payment)

	uuid7, _ := uuid.NewV7()
	payment.UUID = uuid7.String()
	payment.LedgerAccountUUID = request.LedgerAccountUUID
	payment.LedgerWalletUUID = wallet.UUID
	payment.InvoiceNumber = request.InvoiceNumber
	payment.Amount = request.Amount
	payment.Currency = request.Currency
	payment.Status = models.PaymentStatusPending
	payment.ExpiresAt = request.ExpiresAt

	// Gateway references (agnostic)
	payment.GatewayRequestId = request.GatewayRequestId
	payment.GatewayTokenId = request.GatewayTokenId
	payment.GatewayPaymentUrl = request.GatewayPaymentUrl

	// These will be filled on confirm
	payment.PaymentMethod = ""
	payment.PaymentDate = nil
	payment.GatewayReferenceNumber = ""
	payment.LedgerSettlementUUID = ""

	// 4. Insert to database
	errorLog = u.ledgerPaymentRepository.Insert(sqlTransaction, payment)
	if errorLog != nil {
		return nil, errorLog
	}

	// 5. Return created payment
	return payment, nil
}

// ConfirmPayment updates a PENDING payment to PAID status
// It also creates a transaction record and updates the wallet balance
// This method is idempotent - if payment is already PAID, it returns success
func (u *ledgerPaymentUseCase) ConfirmPayment(sqlTransaction *sqlx.Tx, request *requests.LedgerPaymentConfirmPaymentRequest) (*models.LedgerPayment, *models.ErrorLog) {

	// 1. Find the payment by gateway request ID
	payment, errorLog := u.ledgerPaymentRepository.GetByGatewayRequestId(request.GatewayRequestId)
	if errorLog != nil {
		return nil, errorLog
	}

	// 2. Idempotency check - if already PAID, return success
	if payment.Status == models.PaymentStatusPaid {
		return payment, nil
	}

	// 3. Validate current status
	if payment.Status != models.PaymentStatusPending {
		err := fmt.Errorf("payment cannot be confirmed, current status: %s", payment.Status)
		return nil, helper.WriteLog(err, http.StatusBadRequest, fmt.Sprintf("Payment cannot be confirmed, current status: %s", payment.Status))
	}

	// 4. Update payment status
	payment.Status = models.PaymentStatusPaid
	payment.PaymentMethod = request.PaymentMethod
	payment.PaymentDate = request.PaymentDate
	payment.GatewayReferenceNumber = request.GatewayReferenceNumber

	errorLog = u.ledgerPaymentRepository.Update(sqlTransaction, payment)
	if errorLog != nil {
		return nil, errorLog
	}

	// 5. Create LedgerTransaction
	transaction := &models.LedgerTransaction{}
	redifu.InitRecord(transaction)

	uuid7, _ := uuid.NewV7()

	transaction.UUID = uuid7.String()
	transaction.TransactionType = models.TransactionTypePayment
	transaction.LedgerPaymentUUID = payment.UUID
	transaction.LedgerWalletUUID = payment.LedgerWalletUUID
	transaction.Amount = payment.Amount
	description := fmt.Sprintf("Payment received for invoice %s via %s", payment.InvoiceNumber, request.PaymentMethod)
	transaction.Description = description

	errorLog = u.ledgerTransactionRepository.Insert(sqlTransaction, transaction)
	if errorLog != nil {
		return nil, errorLog
	}

	// 6. Update LedgerWallet
	wallet, errorLog := u.ledgerWalletRepository.GetByUUID(payment.LedgerWalletUUID)
	if errorLog != nil {
		return nil, errorLog
	}

	now := time.Now().UTC()
	wallet.PendingBalance += payment.Amount
	wallet.IncomeAccumulation += payment.Amount
	wallet.LastReceive = &now

	errorLog = u.ledgerWalletRepository.Update(sqlTransaction, wallet)
	if errorLog != nil {
		return nil, errorLog
	}

	// 7. Set Ledger Pending Balance, will be used for settlement if successful
	_, errorLog = u.ledgerPendingBalanceUseCase.SetPendingBalance(sqlTransaction, payment.LedgerAccountUUID, payment.LedgerWalletUUID, payment.LedgerSettlementUUID, "", payment.Amount)
	if errorLog != nil {
		return nil, errorLog
	}

	// 8. Return updated payment
	return payment, nil
}

// FailPayment updates a PENDING payment to FAILED status
// Only PENDING payments can be failed
// Will be used in scheduler or manual process when payment is not completed
func (u *ledgerPaymentUseCase) FailPayment(sqlTransaction *sqlx.Tx, request *requests.LedgerPaymentFailPaymentRequest) (*models.LedgerPayment, *models.ErrorLog) {

	// 1. Find the payment
	payment, errorLog := u.ledgerPaymentRepository.GetByInvoiceNumber(request.InvoiceNumber)
	if errorLog != nil {
		return nil, errorLog
	}

	// 2. Only PENDING payments can be failed
	if payment.Status != models.PaymentStatusPending {
		err := fmt.Errorf("payment cannot be failed, current status: %s", payment.Status)
		return nil, helper.WriteLog(err, http.StatusBadRequest, fmt.Sprintf("Payment cannot be failed, current status: %s", payment.Status))
	}

	// 3. Update status to FAILED
	payment.Status = models.PaymentStatusFailed

	errorLog = u.ledgerPaymentRepository.Update(sqlTransaction, payment)
	if errorLog != nil {
		return nil, errorLog
	}

	// 4. No wallet update needed (no money was received)

	return payment, nil
}

// ExpirePayments is a batch job that expires old PENDING payments
// It finds all PENDING payments where expires_at < now and marks them as EXPIRED
// Returns the count of expired payments
func (u *ledgerPaymentUseCase) ExpirePayments(sqlTransaction *sqlx.Tx) (int, *models.ErrorLog) {

	now := time.Now().UTC()

	// 1. Find all PENDING payments where expires_at < now
	expiredPayments, errorLog := u.ledgerPaymentRepository.GetExpiredPendingPayments(now)
	if errorLog != nil {
		return 0, errorLog
	}

	// 2. Update each to EXPIRED
	count := 0
	for _, payment := range expiredPayments {
		payment.Status = models.PaymentStatusExpired
		errorLog = u.ledgerPaymentRepository.Update(sqlTransaction, payment)
		if errorLog != nil {
			// Log error but continue with others
			continue
		}
		count++
	}

	return count, nil
}

// GetPaymentByUUID retrieves a payment by its UUID
func (u *ledgerPaymentUseCase) GetPaymentByUUID(uuid string) (*models.LedgerPayment, *models.ErrorLog) {
	return u.ledgerPaymentRepository.GetByUUID(uuid)
}

// GetPaymentByInvoiceNumber retrieves a payment by invoice number
func (u *ledgerPaymentUseCase) GetPaymentByInvoiceNumber(invoiceNumber string) (*models.LedgerPayment, *models.ErrorLog) {
	return u.ledgerPaymentRepository.GetByInvoiceNumber(invoiceNumber)
}

// GetPaymentByGatewayRequestId retrieves a payment by gateway request ID
func (u *ledgerPaymentUseCase) GetPaymentByGatewayRequestId(gatewayRequestId string) (*models.LedgerPayment, *models.ErrorLog) {
	return u.ledgerPaymentRepository.GetByGatewayRequestId(gatewayRequestId)
}

// GetPendingPaymentByInvoice retrieves a pending payment by invoice number
func (u *ledgerPaymentUseCase) GetPendingPaymentByInvoice(invoiceNumber string) (*models.LedgerPayment, *models.ErrorLog) {
	return u.ledgerPaymentRepository.GetPendingByInvoiceNumber(invoiceNumber)
}

// GetPaymentsByLedgerAccountUUID retrieves all payments for a specific account
func (u *ledgerPaymentUseCase) GetPaymentsByLedgerAccountUUID(ledgerAccountUUID string) ([]*models.LedgerPayment, *models.ErrorLog) {
	return u.ledgerPaymentRepository.GetByLedgerAccountUUID(ledgerAccountUUID)
}

// GetPaymentsByStatus retrieves all payments with a specific status
func (u *ledgerPaymentUseCase) GetPaymentsByStatus(status string) ([]*models.LedgerPayment, *models.ErrorLog) {
	return u.ledgerPaymentRepository.GetByStatus(status)
}
