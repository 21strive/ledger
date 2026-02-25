package ledger

import (
	"context"

	"github.com/21strive/ledger/domain"
	"github.com/21strive/ledger/ledgererr"
	"github.com/21strive/ledger/repo"
	"github.com/google/uuid"
)

func (c *LedgerClient) SetupDummyData(platformEmail string, sellerEmail string) {
	// Setup Platform Account
	platformAccount, err := c.CreatePlatformAccount(context.Background(), platformEmail, domain.CurrencyIDR)
	if err != nil {
		c.logger.ErrorContext(context.Background(), "Failed to create platform account: skipping...", "error", err)
	} else {
		c.logger.InfoContext(context.Background(), "Platform account created", "account_id", platformAccount.ID)
	}

	// Setup DOKU Account
	var dokuAccount domain.Account
	c.txProvider.Transact(context.Background(), func(tx repo.Tx) error {

		existingDokuAcc, err := tx.Account().GetPaymentGatewayAccount(context.Background())
		if err != nil && err == repo.ErrNotFound {
			c.logger.InfoContext(context.Background(), "No existing DOKU account found, creating new one...")
		} else if err != nil {
			c.logger.ErrorContext(context.Background(), "Failed to check existing DOKU account: skipping...", "error", err)
			return nil
		} else {
			c.logger.InfoContext(context.Background(), "DOKU account already exists", "account_id", existingDokuAcc.ID)
			dokuAccount = *existingDokuAcc
			return nil
		}

		dokuAccount = domain.NewPaymentGatewayAccount("", "DOKU", domain.CurrencyIDR)
		err = tx.Account().Save(context.Background(), &dokuAccount)
		if err != nil {
			c.logger.ErrorContext(context.Background(), "Failed to create DOKU account: skipping...", "error", err)
		} else {
			c.logger.InfoContext(context.Background(), "DOKU account created", "account_id", dokuAccount.ID)
		}

		return nil
	})

	// Creating dummy seller account
	sellerAccount, err := c.CreateAccount(context.Background(), sellerEmail, sellerEmail, "Testing Name", domain.CurrencyIDR)
	if err != nil {
		c.logger.ErrorContext(context.Background(), "Failed to create seller account: skipping...", "error", err)
		sellerAccount, err = c.repoProvider.Account().GetBySellerID(context.Background(), sellerEmail)
		if err != nil {
			c.logger.ErrorContext(context.Background(), "Failed to get seller account by seller ID", "error", err)
			return
		}
	} else {
		c.logger.InfoContext(context.Background(), "Seller account created", "account_id", sellerAccount.ID)
	}

	dummyBuyerUUID := uuid.New().String()
	dummyProductID := uuid.New().String()

	dummyPendingTransactions := []map[string]any{
		{
			"buyer_id":       dummyBuyerUUID,
			"seller_id":      sellerAccount.ID,
			"product_id":     dummyProductID,
			"price":          100000,
			"invoice_number": "INV-001-001",
			"metadata": map[string]any{
				"title":     "Ngawi Lari Santai",
				"full_name": "Alice Johnson",
				"type":      "Photos",
			},
		},
		{
			"buyer_id":       dummyBuyerUUID,
			"seller_id":      sellerAccount.ID,
			"product_id":     dummyProductID,
			"price":          200000,
			"invoice_number": "INV-001-002",
			"metadata": map[string]any{
				"title":     "Jakarta Fun Run",
				"full_name": "Charlie Krik",
				"type":      "Folder",
			},
		},
		{
			"buyer_id":       dummyBuyerUUID,
			"seller_id":      sellerAccount.ID,
			"product_id":     dummyProductID,
			"price":          300000,
			"invoice_number": "INV-001-003",
			"metadata": map[string]any{
				"title":     "Bali Sunset Photos",
				"full_name": "Emily Carter",
				"type":      "Folder",
			},
		},
		{
			"buyer_id":       dummyBuyerUUID,
			"seller_id":      sellerAccount.ID,
			"product_id":     dummyProductID,
			"price":          312012,
			"invoice_number": "INV-001-004",
			"metadata": map[string]any{
				"title":     "Surabaya Marathon",
				"full_name": "David Lee",
				"type":      "Photos",
			},
		},
		{
			"buyer_id":       dummyBuyerUUID,
			"seller_id":      sellerAccount.ID,
			"product_id":     dummyProductID,
			"price":          150000,
			"invoice_number": "INV-001-005",
			"metadata": map[string]any{
				"title":     "Yogyakarta Street Food",
				"full_name": "Sophia Kim",
				"type":      "Folder",
			},
		},
		{
			"buyer_id":       dummyBuyerUUID,
			"seller_id":      sellerAccount.ID,
			"product_id":     dummyProductID,
			"price":          250000,
			"invoice_number": "INV-001-006",
			"metadata": map[string]any{
				"title":     "Bandung Artisanal Coffee",
				"full_name": "Michael Chen",
				"type":      "Photos",
			},
		},
	}

	dummyPaidTransactions := []map[string]any{
		{
			"buyer_id":       dummyBuyerUUID,
			"seller_id":      sellerAccount.ID,
			"product_id":     dummyProductID,
			"price":          175000,
			"invoice_number": "INV-002-001",
			"metadata": map[string]any{
				"title":     "Medan Culinary Tour",
				"full_name": "Olivia Davis",
				"type":      "Folder",
			},
		},
		{
			"buyer_id":       dummyBuyerUUID,
			"seller_id":      sellerAccount.ID,
			"product_id":     dummyProductID,
			"price":          225000,
			"invoice_number": "INV-002-002",
			"metadata": map[string]any{
				"title":     "Semarang Historical Sites",
				"full_name": "James Wilson",
				"type":      "Photos",
			},
		},
		{
			"buyer_id":       dummyBuyerUUID,
			"seller_id":      sellerAccount.ID,
			"product_id":     dummyProductID,
			"price":          275000,
			"invoice_number": "INV-002-003",
			"metadata": map[string]any{
				"title":     "Makassar Beach Sunset",
				"full_name": "Isabella Martinez",
				"type":      "Folder",
			},
		},
		{
			"buyer_id":       dummyBuyerUUID,
			"seller_id":      sellerAccount.ID,
			"product_id":     dummyProductID,
			"price":          125000,
			"invoice_number": "INV-002-004",
			"metadata": map[string]any{
				"title":     "Balikpapan Nature Hike",
				"full_name": "William Anderson",
				"type":      "Photos",
			},
		},
		{
			"buyer_id":       dummyBuyerUUID,
			"seller_id":      sellerAccount.ID,
			"product_id":     dummyProductID,
			"price":          300000,
			"invoice_number": "INV-002-005",
			"metadata": map[string]any{
				"title":     "Pontianak River Cruise",
				"full_name": "Mia Thomas",
				"type":      "Folder",
			},
		},
		{
			"buyer_id":       dummyBuyerUUID,
			"seller_id":      sellerAccount.ID,
			"product_id":     dummyProductID,
			"price":          200000,
			"invoice_number": "INV-002-006",
			"metadata": map[string]any{
				"title":     "Manado Diving Experience",
				"full_name": "Benjamin Garcia",
				"type":      "Photos",
			},
		},
		{
			"buyer_id":       dummyBuyerUUID,
			"seller_id":      sellerAccount.ID,
			"product_id":     dummyProductID,
			"price":          180000,
			"invoice_number": "INV-002-007",
			"metadata": map[string]any{
				"title":     "Padang Culinary Delights",
				"full_name": "Charlotte Rodriguez",
				"type":      "Folder",
			},
		},
	}

	// Creating dummy transactions
	err = c.txProvider.Transact(context.Background(), func(tx repo.Tx) error {
		// Dummy transaction creation logic would go here
		// Load fee configurations
		feeConfigs, err := tx.FeeConfig().GetAllActive(context.Background())
		if err != nil {
			return ledgererr.NewError(ledgererr.CodeInternal, "failed to load fee configurations", err)
		}

		// Calculate fees
		feeCalc := domain.NewFeeCalculator(feeConfigs)

		// PENDING Transactions
		for _, txData := range dummyPendingTransactions {
			feeBreakdown := feeCalc.GetFeeBreakdown(int64(txData["price"].(int)), "QRIS", domain.CurrencyIDR)
			productTx := domain.NewProductTransaction(
				txData["buyer_id"].(string),
				txData["seller_id"].(string),
				txData["product_id"].(string),
				txData["invoice_number"].(string),
				feeBreakdown,
				txData["metadata"].(map[string]any),
			)
			invoiceNum := txData["invoice_number"].(string)

			existingProductTx, err := tx.ProductTransaction().GetByInvoiceNumber(context.Background(), invoiceNum)
			if err != nil && !ledgererr.IsAppError(err, repo.ErrNotFound) {
				c.logger.ErrorContext(context.Background(), "Failed to get existing product transaction", "invoice_number", invoiceNum, "error", err)
				return err
			}
			if existingProductTx != nil {
				c.logger.InfoContext(context.Background(), "Skipping existing product transaction", "invoice_number", invoiceNum)
				continue
			}

			ledgerEntries := domain.NewPaymentEntries(
				productTx.ID,
				productTx.SellerAccountID,
				productTx.Fee.SellerPrice,
				platformAccount.ID,
				productTx.Fee.PlatformFee,
				dokuAccount.ID,
				productTx.Fee.DokuFee,
			)

			productTx.MarkCompleted()
			if err := tx.ProductTransaction().Save(context.Background(), productTx); err != nil {
				c.logger.ErrorContext(context.Background(), "Failed to save product transaction", "error", err)
				return err
			}

			// Insert immutable ledger entries
			if err := tx.LedgerEntry().SaveBatch(context.Background(), ledgerEntries); err != nil {
				return err
			}

			// Here you would save the product transaction and ledger entries to the database
			// For this dummy setup, we're just generating the entries without persisting them

			c.logger.InfoContext(context.Background(), "Generated dummy transaction and ledger entries", "transaction_id", productTx.ID, "seller_amount", productTx.Fee.SellerPrice, "platform_fee", productTx.Fee.PlatformFee, "doku_fee", productTx.Fee.DokuFee)
		}

		for _, txData := range dummyPaidTransactions {
			feeBreakdown := feeCalc.GetFeeBreakdown(int64(txData["price"].(int)), "QRIS", domain.CurrencyIDR)
			productTx := domain.NewProductTransaction(
				txData["buyer_id"].(string),
				txData["seller_id"].(string),
				txData["product_id"].(string),
				txData["invoice_number"].(string),
				feeBreakdown,
				txData["metadata"].(map[string]any),
			)
			invoiceNum := txData["invoice_number"].(string)

			existingProductTx, err := tx.ProductTransaction().GetByInvoiceNumber(context.Background(), invoiceNum)
			if err != nil && !ledgererr.IsAppError(err, repo.ErrNotFound) {
				c.logger.ErrorContext(context.Background(), "Failed to get existing product transaction", "invoice_number", invoiceNum, "error", err)
				return err
			}
			if existingProductTx != nil {
				c.logger.InfoContext(context.Background(), "Skipping existing product transaction", "invoice_number", invoiceNum)
				continue
			}

			productTx.MarkSettled()
			if err := tx.ProductTransaction().Save(context.Background(), productTx); err != nil {
				c.logger.ErrorContext(context.Background(), "Failed to save product transaction", "error", err)
				return err
			}

			// Seller Entries
			batchID := uuid.New().String()
			sellerEntry := domain.NewSettlementEntriesForAccount(
				batchID,
				sellerAccount.ID,
				feeBreakdown.SellerPrice,
			)

			// Platform Fee Entry
			platformEntry := domain.NewSettlementEntriesForAccount(
				batchID,
				platformAccount.ID,
				feeBreakdown.PlatformFee,
			)

			// DOKU Fee Entry
			dokuEntry := domain.NewDokuFeeSettlementEntry(
				batchID,
				dokuAccount.ID,
				feeBreakdown.DokuFee,
			)

			paidLedgerEntries := append(sellerEntry, platformEntry...)
			paidLedgerEntries = append(paidLedgerEntries, dokuEntry)

			// Insert immutable ledger entries
			if err := tx.LedgerEntry().SaveBatch(context.Background(), paidLedgerEntries); err != nil {
				return err
			}

			c.logger.InfoContext(context.Background(), "Generated dummy transaction and ledger entries", "transaction_id", productTx.ID, "seller_amount", productTx.Fee.SellerPrice, "platform_fee", productTx.Fee.PlatformFee, "doku_fee", productTx.Fee.DokuFee)
		}

		return nil

		// feeBreakdown := feeCalc.GetFeeBreakdown(100000, "QRIS", domain.CurrencyIDR)
		// productTx := domain.NewProductTransaction(
		// 	"dummy-buyer-account-id",
		// 	sellerEmail,
		// 	"123-456-789-1011",
		// 	generateInvoiceNumber(),
		// 	feeBreakdown,
		// 	map[string]any{
		// 		"title": "Dummy Product",
		// 	},
		// )

		// // PENDING Transactions
		// ledgerEntries := domain.NewPaymentEntries(
		// 	productTx.ID,
		// 	productTx.SellerAccountID,
		// 	productTx.Fee.SellerPrice,
		// 	platformAccount.ID,
		// 	productTx.Fee.PlatformFee,
		// 	dokuAccount.ID,
		// 	productTx.Fee.DokuFee,
		// )

		// return nil
	})

	if err != nil {
		c.logger.ErrorContext(context.Background(), "Failed to create dummy transactions: skipping...", "error", err)
	}
}
