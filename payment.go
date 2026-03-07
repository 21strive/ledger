package ledger

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/21strive/doku/app/requests"
	dokuRequests "github.com/21strive/doku/app/requests"
	"github.com/21strive/ledger/domain"
	"github.com/21strive/ledger/ledgererr"
	"github.com/21strive/ledger/repo"
)

// GeneratePaymentRequest contains the parameters to generate a payment
type GeneratePaymentRequest struct {
	// Seller information
	SellerAccountID string `json:"seller_account_id"`

	// Buyer information
	BuyerAccountID string `json:"buyer_account_id"`
	BuyerName      string `json:"buyer_name"`
	BuyerEmail     string `json:"buyer_email"`

	// Product information
	ProductID   string         `json:"product_id"`
	ProductType string         `json:"product_type"` // PHOTO, FOLDER, SUBSCRIPTION, etc.
	SellerPrice int64          `json:"seller_price"` // Price set by seller (100% goes to seller)
	Currency    string         `json:"currency"`     // IDR or USD
	Metadata    map[string]any `json:"metadata"`     // Product details (title, resolution, etc.)

	// Payment configuration
	PaymentChannel string `json:"payment_channel"` // QRIS, VIRTUAL_ACCOUNT_MANDIRI, etc.
	ExpiresIn      int64  `json:"expires_in"`      // Payment expiration in minutes (default: 60 minutes, max: 999999)
}

// GeneratePaymentResponse contains the result of payment generation
type GeneratePaymentResponse struct {
	TransactionID string `json:"transaction_id"`
	InvoiceNumber string `json:"invoice_number"`
	PaymentURL    string `json:"payment_url"`
	PaymentCode   string `json:"payment_code,omitempty"` // VA number, QRIS code, etc.
	ExpiresAt     int64  `json:"expires_at"`             // Unix timestamp

	// Fee breakdown for transparency
	SellerPrice  int64  `json:"seller_price"`
	PlatformFee  int64  `json:"platform_fee"`
	DokuFee      int64  `json:"doku_fee"`
	TotalCharged int64  `json:"total_charged"`
	Currency     string `json:"currency"`
}

// GeneratePayment creates a new payment for a product purchase
// Flow: Calculate fees → Create ProductTransaction → Call DOKU API → Create PaymentRequest
func (c *LedgerClient) GeneratePayment(ctx context.Context, req *GeneratePaymentRequest) (*GeneratePaymentResponse, error) {
	// Validate required fields
	if err := c.validateGeneratePaymentRequest(req); err != nil {
		return nil, err
	}

	// Get seller's ledger to obtain DOKU SAC ID
	sellerAcccount, err := c.repoProvider.Account().GetBySellerID(ctx, req.SellerAccountID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(fmt.Errorf("seller account not found for seller_id: %s", req.SellerAccountID))
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get seller account", err)
	}

	// Load fee configurations
	feeConfigs, err := c.repoProvider.FeeConfig().GetAllActive(ctx)
	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to load fee configurations", err)
	}

	// Calculate fees
	feeCalc := domain.NewFeeCalculator(feeConfigs)

	// Validate payment channel is supported
	if !feeCalc.HasPaymentChannel(req.PaymentChannel) {
		return nil, ledgererr.ErrUnsupportedPaymentChannel.WithError(
			fmt.Errorf("payment channel %q not found in fee configs, supported: %v", req.PaymentChannel, feeCalc.SupportedPaymentChannels()),
		)
	}

	currency := domain.Currency(req.Currency)
	feeBreakdown := feeCalc.GetFeeBreakdown(req.SellerPrice, req.PaymentChannel, currency)

	c.logger.InfoContext(ctx, "Calculated fee breakdown",
		"seller_price", feeBreakdown.SellerPrice,
		"platform_fee", feeBreakdown.PlatformFee,
		"doku_fee", feeBreakdown.DokuFee,
		"total_charged", feeBreakdown.TotalCharged,
		"currency", feeBreakdown.Currency,
	)

	// Generate invoice number
	invoiceNumber := generateInvoiceNumber()

	c.logger.InfoContext(ctx, "Generated invoice number", "invoice_number", invoiceNumber)

	// Calculate expiration time
	// Payment due date is in minutes (default: 60 minutes, max: 999999)
	expiresInMinutes := req.ExpiresIn
	if expiresInMinutes <= 0 {
		expiresInMinutes = 60 // Default: 60 minutes
	}
	expiresAt := time.Now().Add(time.Duration(expiresInMinutes) * time.Minute)

	productTx := domain.NewProductTransaction(
		req.BuyerAccountID,
		sellerAcccount.UUID,
		req.ProductID,
		req.ProductType,
		invoiceNumber,
		feeBreakdown,
		req.Metadata,
	)

	// Call DOKU API to create payment
	dokuResp, dokuErr := c.dokuClient.AcceptPayment(&dokuRequests.DokuCreatePaymentRequest{
		Amount:         feeBreakdown.TotalCharged,
		CustomerName:   req.BuyerName,
		CustomerEmail:  req.BuyerEmail,
		SacID:          sellerAcccount.DokuSubAccountID,
		PaymentDueDate: expiresInMinutes, // DOKU expects minutes
		InvoiceNumber:  invoiceNumber,
		PaymentMethod:  req.PaymentChannel,
	})

	if dokuErr != nil {
		c.logger.ErrorContext(ctx, "DOKU AcceptPayment failed",
			"invoice_number", invoiceNumber,
			"error", dokuErr.Err,
			"message", dokuErr.Message,
			"status_code", dokuErr.StatusCode,
		)
		return nil, ledgererr.NewError(ledgererr.CodeDokuAPIError, "failed to create payment with DOKU", fmt.Errorf("status: %d, error: %v", dokuErr.StatusCode, dokuErr.Message))
	}

	// Extract payment details from DOKU response
	var paymentURL string
	var paymentCode string
	var dokuRequestID string

	if dokuResp.Response.Payment != nil {
		if dokuResp.Response.Payment.URL.Valid {
			paymentURL = dokuResp.Response.Payment.URL.String
		}
		if dokuResp.Response.Payment.TokenID.Valid {
			dokuRequestID = dokuResp.Response.Payment.TokenID.String
		}
	}

	if dokuResp.Response.Order != nil {
		if dokuResp.Response.Order.SessionID.Valid {
			if dokuRequestID == "" {
				dokuRequestID = dokuResp.Response.Order.SessionID.String
			}
		}
	}

	// Create PaymentRequest
	paymentReq := domain.NewPaymentRequest(
		productTx.UUID,
		dokuRequestID,
		req.PaymentChannel,
		feeBreakdown.TotalCharged,
		currency,
		expiresAt,
	)
	paymentReq.SetPaymentURL(paymentURL)
	if paymentCode != "" {
		paymentReq.SetPaymentCode(paymentCode)
	}

	// Save both ProductTransaction and PaymentRequest in a transaction
	err = c.txProvider.Transact(ctx, func(tx repo.Tx) error {
		// Save ProductTransaction
		if err := tx.ProductTransaction().Save(ctx, productTx); err != nil {
			return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to save product transaction", err)
		}

		// Save PaymentRequest
		if err := tx.PaymentRequest().Save(ctx, paymentReq); err != nil {
			return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to save payment request", err)
		}

		return nil
	})

	if err != nil {
		c.logger.ErrorContext(ctx, "Failed to save payment records",
			"invoice_number", invoiceNumber,
			"error", err,
		)
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to save payment records", err)
	}

	c.logger.InfoContext(ctx, "Payment generated successfully",
		"transaction_id", productTx.UUID,
		"invoice_number", invoiceNumber,
		"total_charged", feeBreakdown.TotalCharged,
		"payment_channel", req.PaymentChannel,
		"checkout_url", paymentURL,
	)

	return &GeneratePaymentResponse{
		TransactionID: productTx.UUID,
		InvoiceNumber: invoiceNumber,
		PaymentURL:    paymentURL,
		PaymentCode:   paymentCode,
		ExpiresAt:     expiresAt.Unix(),
		SellerPrice:   feeBreakdown.SellerPrice,
		PlatformFee:   feeBreakdown.PlatformFee,
		DokuFee:       feeBreakdown.DokuFee,
		TotalCharged:  feeBreakdown.TotalCharged,
		Currency:      string(currency),
	}, nil
}

type NotifyPaymentSuccessRequest struct {
	TransactionID string
	InvoiceNumber string
	PaymentCode   string
	PaymentURL    string
	ExpiresAt     int64
	SellerPrice   int64
	PlatformFee   int64
	DokuFee       int64
	TotalCharged  int64
	Currency      string
}

func (c *LedgerClient) HandlePaymentSuccess(ctx context.Context, req *requests.DokuNotificationRequest) error {
	dokuResp, dokuErr := c.dokuClient.HandleNotification(req)
	if dokuErr != nil {
		return ledgererr.NewError(ledgererr.CodeDokuAPIError, "failed to notify payment success", fmt.Errorf("status: %d, error: %v", dokuErr.StatusCode, dokuErr.Message))
	}

	if dokuResp.Transaction.Status.String != "SUCCESS" {
		return ledgererr.NewError(ledgererr.CodeDokuAPIError, "payment status is not paid", fmt.Errorf("status: %s", dokuResp.Transaction.Status.String))
	}

	// 1. Get the invoice number from the DOKU response
	if dokuResp.Order == nil || !dokuResp.Order.InvoiceNumber.Valid {
		return ledgererr.NewError(ledgererr.CodeInvalidRequest, "missing invoice number in notification", nil)
	}
	invoiceNumber := dokuResp.Order.InvoiceNumber.String

	// 2. Fetch the corresponding ProductTransaction
	productTx, err := c.repoProvider.ProductTransaction().GetByInvoiceNumber(ctx, invoiceNumber)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return ledgererr.NewError(ledgererr.CodeNotFound, "product transaction not found", err)
		}
		return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to get product transaction", err)
	}

	// Double webhook idempotency check: if not PENDING, already processed or terminal
	if !productTx.IsPending() {
		c.logger.InfoContext(ctx, "Payment notification received for non-pending transaction", "invoice_number", invoiceNumber, "status", productTx.Status)
		return nil
	}

	// 3. Fetch related PaymentRequest
	paymentReq, err := c.repoProvider.PaymentRequest().GetByProductTransactionID(ctx, productTx.UUID)
	if err != nil {
		return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to get payment request", err)
	}

	// 4. Resolve the system accounts needed for ledger entries
	platformAccount, err := c.repoProvider.Account().GetPlatformAccount(ctx)
	if err != nil {
		return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to get platform account", err)
	}

	dokuAccount, err := c.repoProvider.Account().GetPaymentGatewayAccount(ctx)
	if err != nil {
		return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to get payment gateway account", err)
	}

	// 5. Create journal for PAYMENT_SUCCESS event
	journal := domain.NewJournal(
		domain.EventTypePaymentSuccess,
		domain.SourceTypeProductTransaction,
		productTx.UUID,
		map[string]any{
			"invoice_number": invoiceNumber,
			"seller_price":   productTx.Fee.SellerPrice,
			"platform_fee":   productTx.Fee.PlatformFee,
			"doku_fee":       productTx.Fee.DokuFee,
		},
	)

	// 6. Generate ledger entries (PENDING credit to seller, platform, and doku)
	ledgerEntries := domain.NewPaymentEntries(
		journal.UUID,
		productTx.UUID,
		productTx.SellerAccountID,
		productTx.Fee.SellerPrice,
		platformAccount.UUID,
		productTx.Fee.PlatformFee,
		dokuAccount.UUID,
		productTx.Fee.DokuFee,
	)

	// 7. Persist everything in single transaction
	err = c.txProvider.Transact(ctx, func(tx repo.Tx) error {
		// Save journal first
		if err := tx.Journal().Save(ctx, journal); err != nil {
			return err
		}

		// Update PaymentRequest to COMPLETED
		if err := paymentReq.MarkCompleted(); err != nil {
			return err
		}
		if err := tx.PaymentRequest().Update(ctx, paymentReq); err != nil {
			return err
		}

		// Update ProductTransaction to COMPLETED
		if err := productTx.MarkCompleted(); err != nil {
			return err
		}
		if err := tx.ProductTransaction().UpdateStatus(ctx, productTx.UUID, productTx.Status, *productTx.CompletedAt); err != nil {
			return err
		}

		// Insert immutable ledger entries
		if err := tx.LedgerEntry().SaveBatch(ctx, ledgerEntries); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		c.logger.ErrorContext(ctx, "Failed to persist payment success", "invoice_number", invoiceNumber, "error", err)
		return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to persist payment success transaction", err)
	}

	c.logger.InfoContext(ctx, "Payment success securely handled", "invoice_number", invoiceNumber, "product_tx_id", productTx.UUID)

	return nil
}

// CalculateFees returns the fee breakdown without creating a transaction
// Useful for showing the buyer the total cost before purchase
func (c *LedgerClient) CalculateFees(ctx context.Context, sellerPrice int64, paymentChannel string, currency string) (*domain.FeeBreakdown, error) {
	feeConfigs, err := c.repoProvider.FeeConfig().GetAllActive(ctx)
	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to load fee configurations", err)
	}

	feeCalc := domain.NewFeeCalculator(feeConfigs)
	breakdown := feeCalc.GetFeeBreakdown(sellerPrice, paymentChannel, domain.Currency(currency))

	return &breakdown, nil
}

// validateGeneratePaymentRequest validates the payment request fields
func (c *LedgerClient) validateGeneratePaymentRequest(req *GeneratePaymentRequest) error {
	if req.SellerAccountID == "" {
		return ledgererr.NewError(ledgererr.CodeInvalidRequest, "seller_account_id is required", nil)
	}
	if req.BuyerAccountID == "" {
		return ledgererr.NewError(ledgererr.CodeInvalidRequest, "buyer_account_id is required", nil)
	}
	if req.BuyerName == "" {
		return ledgererr.NewError(ledgererr.CodeInvalidRequest, "buyer_name is required", nil)
	}
	if req.BuyerEmail == "" {
		return ledgererr.NewError(ledgererr.CodeInvalidRequest, "buyer_email is required", nil)
	}
	if req.ProductID == "" {
		return ledgererr.NewError(ledgererr.CodeInvalidRequest, "product_id is required", nil)
	}
	if req.SellerPrice <= 0 {
		return ledgererr.NewError(ledgererr.CodeInvalidRequest, "seller_price must be positive", nil)
	}
	if req.Currency == "" {
		return ledgererr.NewError(ledgererr.CodeInvalidRequest, "currency is required", nil)
	}
	if req.PaymentChannel == "" {
		return ledgererr.NewError(ledgererr.CodeInvalidRequest, "payment_channel is required", nil)
	}

	// Validate currency
	if req.Currency != string(domain.CurrencyIDR) && req.Currency != string(domain.CurrencyUSD) {
		return ledgererr.NewError(ledgererr.CodeInvalidRequest, "currency must be IDR or USD", nil)
	}

	return nil
}

// generateInvoiceNumber creates a unique invoice number
func generateInvoiceNumber() string {
	now := time.Now()
	return fmt.Sprintf("INV-%s-%s", now.Format("20060102150405"), randomString(6))
}

func randomString(n int) string {
	return rand.Text()[:n]
}
