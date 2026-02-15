package ledger

import (
	"context"
	"fmt"
	"time"

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
	SellerPrice int64          `json:"seller_price"` // Price set by seller (100% goes to seller)
	Currency    string         `json:"currency"`     // IDR or USD
	Metadata    map[string]any `json:"metadata"`     // Product details (title, resolution, etc.)

	// Payment configuration
	PaymentChannel string `json:"payment_channel"` // QRIS, VIRTUAL_ACCOUNT_MANDIRI, etc.
	ExpiresIn      int64  `json:"expires_in"`      // Payment expiration in seconds (default: 24 hours)
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
	sellerLedger, err := c.repoProvider.Ledger().GetByAccountID(ctx, req.SellerAccountID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(fmt.Errorf("seller ledger not found for account_id: %s", req.SellerAccountID))
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get seller ledger", err)
	}

	// Load fee configurations
	feeConfigs, err := c.repoProvider.FeeConfig().GetAllActive(ctx)
	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to load fee configurations", err)
	}

	// Calculate fees
	feeCalc := domain.NewFeeCalculator(feeConfigs)
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
	expiresIn := req.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 24 * 60 * 60 // Default: 24 hours
	}
	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)

	productTx := domain.NewProductTransaction(
		req.BuyerAccountID,
		req.SellerAccountID,
		req.ProductID,
		invoiceNumber,
		feeBreakdown,
		req.Metadata,
	)

	// Call DOKU API to create payment
	dokuResp, dokuErr := c.dokuClient.AcceptPayment(&dokuRequests.DokuCreatePaymentRequest{
		Amount:         feeBreakdown.TotalCharged,
		CustomerName:   req.BuyerName,
		CustomerEmail:  req.BuyerEmail,
		SacID:          sellerLedger.DokuSubAccountID,
		PaymentDueDate: expiresAt.Unix(),
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
		productTx.ID,
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
		"transaction_id", productTx.ID,
		"invoice_number", invoiceNumber,
		"total_charged", feeBreakdown.TotalCharged,
		"payment_channel", req.PaymentChannel,
		"checkout_url", paymentURL,
	)

	return &GeneratePaymentResponse{
		TransactionID: productTx.ID,
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
	return fmt.Sprintf("INV-%s-%d", now.Format("20060102150405"), now.UnixNano()%10000)
}
