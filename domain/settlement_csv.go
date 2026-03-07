package domain

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/21strive/ledger/ledgererr"
)

// DokuSettlementCSVRow represents a single row from DOKU settlement CSV
// CSV Format: NO,MERCHANT NAME,PAYMENT CHANNEL NAME,TRANSACTION DATE,INVOICE NUMBER,CUSTOMER NAME,REPORT CODE,AMOUNT,RECON CODE,FEE,DISCOUNT,PAY TO MERCHANT,PAY OUT DATE,TRANSACTION TYPE,PROMO CODE,SUB ACCOUNT
type DokuSettlementCSVRow struct {
	RowNumber          int
	No                 int
	MerchantName       string
	PaymentChannelName string
	TransactionDate    time.Time
	InvoiceNumber      string // Maps to our product_transactions.invoice_number
	CustomerName       string
	ReportCode         string
	Amount             int64 // Total amount charged (in smallest unit, e.g., cents)
	ReconCode          string
	Fee                int64 // DOKU fee
	Discount           int64
	PayToMerchant      int64 // Net amount after fees
	PayOutDate         time.Time
	TransactionType    string
	PromoCode          string
	SubAccount         string            // DOKU sub-account ID (SAC-xxxx-xxxx)
	RawData            map[string]string // Original CSV row data
}

// DokuSettlementCSVMetadata contains extracted metadata from CSV header rows
type DokuSettlementCSVMetadata struct {
	TotalAmountPurchase int64  // Row 1: Total Amount Purchase
	TotalFee            int64  // Row 2: Total Fee
	TotalPurchase       int    // Row 3: Total Purchase (transaction count)
	TotalAmountRefund   int64  // Row 4: Total Amount Refund
	TotalRefund         int    // Row 5: Total Refund (refund count)
	TotalSettlement     int64  // Row 6: Total Settlement Amount
	TotalDiscount       int64  // Row 7: Total Discount
	TotalTransactions   int    // Row 8: Total Transactions
	BatchID             string // Row 9: Batch ID (DOKU batch identifier)
}

// DokuSettlementCSVParser handles parsing DOKU settlement CSV files
type DokuSettlementCSVParser struct {
	rows          []*DokuSettlementCSVRow
	metadata      *DokuSettlementCSVMetadata
	parseErrors   []error
	skippedRows   int
	dateFormat    string
	amountDivisor int64 // If DOKU returns amounts in larger units (e.g., 90000000 = 900000.00)
}

// NewDokuSettlementCSVParser creates a new CSV parser
// dateFormat: format for parsing date fields (default: "02-01-2006" for DD-MM-YYYY)
// amountDivisor: divisor for amounts if DOKU returns in larger units (default: 1)
func NewDokuSettlementCSVParser(dateFormat string, amountDivisor int64) *DokuSettlementCSVParser {
	if dateFormat == "" {
		dateFormat = "02-01-2006" // DD-MM-YYYY
	}
	if amountDivisor <= 0 {
		amountDivisor = 1
	}
	return &DokuSettlementCSVParser{
		rows:          make([]*DokuSettlementCSVRow, 0),
		metadata:      &DokuSettlementCSVMetadata{},
		parseErrors:   make([]error, 0),
		dateFormat:    dateFormat,
		amountDivisor: amountDivisor,
	}
}

// CSV column indices (0-based)
const (
	colNo                 = 0
	colMerchantName       = 1
	colPaymentChannelName = 2
	colTransactionDate    = 3
	colInvoiceNumber      = 4
	colCustomerName       = 5
	colReportCode         = 6
	colAmount             = 7
	colReconCode          = 8
	colFee                = 9
	colDiscount           = 10
	colPayToMerchant      = 11
	colPayOutDate         = 12
	colTransactionType    = 13
	colPromoCode          = 14
	colSubAccount         = 15
	minColumnCount        = 12 // Minimum required columns
)

// Parse reads and parses a DOKU settlement CSV from a reader
// The CSV format includes:
// - Lines 1-9: Metadata (Total Amount Purchase, Total Fee, etc.)
// - Line 10: Blank line
// - Line 11: Column headers (NO,MERCHANT NAME,...)
// - Line 12+: Data rows
func (p *DokuSettlementCSVParser) Parse(reader io.Reader) error {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true
	csvReader.FieldsPerRecord = -1 // Allow variable number of fields

	// Read and parse metadata header rows (first 9 rows)
	metadataRows := make([][]string, 9)
	for i := 0; i < 9; i++ {
		row, err := csvReader.Read()
		if err != nil {
			return ledgererr.ErrInvalidSettlementCSVFormat.WithError(fmt.Errorf("failed to read metadata row %d: %w", i+1, err))
		}
		metadataRows[i] = row
	}

	// Extract metadata from rows
	if err := p.parseMetadata(metadataRows); err != nil {
		return ledgererr.ErrInvalidSettlementCSVFormat.WithError(fmt.Errorf("failed to parse metadata: %w", err))
	}

	// Skip blank line (row 10)
	_, err := csvReader.Read()
	if err != nil {
		return ledgererr.ErrInvalidSettlementCSVFormat.WithError(fmt.Errorf("failed to skip blank row: %w", err))
	}

	// Read column header row (row 11)
	header, err := csvReader.Read()
	if err != nil {
		return ledgererr.ErrInvalidSettlementCSVFormat.WithError(fmt.Errorf("failed to read CSV header: %w", err))
	}

	// Validate header
	if len(header) < minColumnCount {
		return ledgererr.ErrInvalidSettlementCSVFormat.WithError(
			fmt.Errorf("CSV has %d columns, expected at least %d", len(header), minColumnCount),
		)
	}

	// Read data rows
	rowNumber := 1 // 1-based (header is row 0)
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			p.parseErrors = append(p.parseErrors, fmt.Errorf("row %d: %w", rowNumber, err))
			p.skippedRows++
			rowNumber++
			continue
		}

		row, err := p.parseRow(record, rowNumber, header)
		if err != nil {
			p.parseErrors = append(p.parseErrors, err)
			p.skippedRows++
			rowNumber++
			continue
		}

		p.rows = append(p.rows, row)
		rowNumber++
	}

	return nil
}

// parseRow parses a single CSV row
func (p *DokuSettlementCSVParser) parseRow(record []string, rowNumber int, header []string) (*DokuSettlementCSVRow, error) {
	if len(record) < minColumnCount {
		return nil, fmt.Errorf("row %d: insufficient columns (%d < %d)", rowNumber, len(record), minColumnCount)
	}

	// Build raw data map
	rawData := make(map[string]string)
	for i, val := range record {
		if i < len(header) {
			rawData[header[i]] = val
		}
	}

	// Parse No
	no, err := strconv.Atoi(strings.TrimSpace(record[colNo]))
	if err != nil {
		// No is sometimes empty, default to row number
		no = rowNumber
	}

	// Parse transaction date
	txDateStr := strings.TrimSpace(record[colTransactionDate])
	txDate, err := time.Parse(p.dateFormat, txDateStr)
	if err != nil {
		// Try alternative formats
		txDate, err = p.tryParseDateAlternatives(txDateStr)
		if err != nil {
			return nil, fmt.Errorf("row %d: invalid transaction date '%s': %w", rowNumber, txDateStr, err)
		}
	}

	// Parse amounts
	amount, err := p.parseAmount(record[colAmount])
	if err != nil {
		return nil, fmt.Errorf("row %d: invalid amount '%s': %w", rowNumber, record[colAmount], err)
	}

	fee, err := p.parseAmount(record[colFee])
	if err != nil {
		// Fee might be empty, default to 0
		fee = 0
	}

	discount, err := p.parseAmount(record[colDiscount])
	if err != nil {
		discount = 0
	}

	payToMerchant, err := p.parseAmount(record[colPayToMerchant])
	if err != nil {
		// Calculate if not provided: amount - fee - discount
		payToMerchant = amount - fee - discount
	}

	// Parse payout date
	payoutDateStr := strings.TrimSpace(record[colPayOutDate])
	payoutDate, err := time.Parse(p.dateFormat, payoutDateStr)
	if err != nil {
		payoutDate, err = p.tryParseDateAlternatives(payoutDateStr)
		if err != nil {
			// Default to transaction date if payout date invalid
			payoutDate = txDate
		}
	}

	// Get optional fields safely
	transactionType := ""
	if len(record) > colTransactionType {
		transactionType = strings.TrimSpace(record[colTransactionType])
	}
	promoCode := ""
	if len(record) > colPromoCode {
		promoCode = strings.TrimSpace(record[colPromoCode])
	}
	subAccount := ""
	if len(record) > colSubAccount {
		subAccount = strings.TrimSpace(record[colSubAccount])
	}

	return &DokuSettlementCSVRow{
		RowNumber:          rowNumber,
		No:                 no,
		MerchantName:       strings.TrimSpace(record[colMerchantName]),
		PaymentChannelName: strings.TrimSpace(record[colPaymentChannelName]),
		TransactionDate:    txDate,
		InvoiceNumber:      strings.TrimSpace(record[colInvoiceNumber]),
		CustomerName:       strings.TrimSpace(record[colCustomerName]),
		ReportCode:         strings.TrimSpace(record[colReportCode]),
		Amount:             amount,
		ReconCode:          strings.TrimSpace(record[colReconCode]),
		Fee:                fee,
		Discount:           discount,
		PayToMerchant:      payToMerchant,
		PayOutDate:         payoutDate,
		TransactionType:    transactionType,
		PromoCode:          promoCode,
		SubAccount:         subAccount,
		RawData:            rawData,
	}, nil
}

// parseAmount parses an amount string to int64
func (p *DokuSettlementCSVParser) parseAmount(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	// Remove commas and currency symbols
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, "Rp", "")
	s = strings.ReplaceAll(s, "IDR", "")
	s = strings.TrimSpace(s)

	amount, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}

	return amount / p.amountDivisor, nil
}

// tryParseDateAlternatives tries various date formats
func (p *DokuSettlementCSVParser) tryParseDateAlternatives(s string) (time.Time, error) {
	formats := []string{
		"02-01-2006", // DD-MM-YYYY
		"2006-01-02", // YYYY-MM-DD
		"02/01/2006", // DD/MM/YYYY
		"01-02-2006", // MM-DD-YYYY
		"01/02/2006", // MM/DD/YYYY
		"2006/01/02", // YYYY/MM/DD
		time.RFC3339,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("could not parse date: %s", s)
}

// parseMetadata extracts metadata from the first 9 rows of the CSV
// Format: "Label_,Value"
// Row 1: Total Amount Purchase_,504440
// Row 2: Total Fee_,4995
// Row 3: Total Purchase_,1
// Row 4: Total Amount Refund_,0
// Row 5: Total Refund_,0
// Row 6: Total Settlement Amount_,499445
// Row 7: Total Discount_,0
// Row 8: Total Transactions_,1
// Row 9: Batch ID_,B-BSN-0203-1761932477260-SBS-8298-20251109155312120-20260305210108875
func (p *DokuSettlementCSVParser) parseMetadata(rows [][]string) error {
	if len(rows) != 9 {
		return fmt.Errorf("expected 9 metadata rows, got %d", len(rows))
	}

	// Helper to extract value from "Label_,Value" format
	extractValue := func(row []string) string {
		if len(row) >= 2 {
			return strings.TrimSpace(row[1])
		}
		return ""
	}

	// Parse each metadata row
	var err error

	// Row 1: Total Amount Purchase
	if val := extractValue(rows[0]); val != "" {
		p.metadata.TotalAmountPurchase, err = p.parseAmount(val)
		if err != nil {
			return fmt.Errorf("invalid Total Amount Purchase: %w", err)
		}
	}

	// Row 2: Total Fee
	if val := extractValue(rows[1]); val != "" {
		p.metadata.TotalFee, err = p.parseAmount(val)
		if err != nil {
			return fmt.Errorf("invalid Total Fee: %w", err)
		}
	}

	// Row 3: Total Purchase
	if val := extractValue(rows[2]); val != "" {
		count, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("invalid Total Purchase: %w", err)
		}
		p.metadata.TotalPurchase = count
	}

	// Row 4: Total Amount Refund
	if val := extractValue(rows[3]); val != "" {
		p.metadata.TotalAmountRefund, err = p.parseAmount(val)
		if err != nil {
			return fmt.Errorf("invalid Total Amount Refund: %w", err)
		}
	}

	// Row 5: Total Refund
	if val := extractValue(rows[4]); val != "" {
		count, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("invalid Total Refund: %w", err)
		}
		p.metadata.TotalRefund = count
	}

	// Row 6: Total Settlement Amount
	if val := extractValue(rows[5]); val != "" {
		p.metadata.TotalSettlement, err = p.parseAmount(val)
		if err != nil {
			return fmt.Errorf("invalid Total Settlement Amount: %w", err)
		}
	}

	// Row 7: Total Discount
	if val := extractValue(rows[6]); val != "" {
		p.metadata.TotalDiscount, err = p.parseAmount(val)
		if err != nil {
			return fmt.Errorf("invalid Total Discount: %w", err)
		}
	}

	// Row 8: Total Transactions
	if val := extractValue(rows[7]); val != "" {
		count, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("invalid Total Transactions: %w", err)
		}
		p.metadata.TotalTransactions = count
	}

	// Row 9: Batch ID (most important - DOKU's unique batch identifier)
	p.metadata.BatchID = extractValue(rows[8])
	if p.metadata.BatchID == "" {
		return fmt.Errorf("Batch ID is required but not found in metadata")
	}

	return nil
}

// GetRows returns all successfully parsed rows
func (p *DokuSettlementCSVParser) GetRows() []*DokuSettlementCSVRow {
	return p.rows
}

// GetParseErrors returns all parsing errors
func (p *DokuSettlementCSVParser) GetParseErrors() []error {
	return p.parseErrors
}

// GetMetadata returns the parsed CSV metadata (totals and batch ID)
func (p *DokuSettlementCSVParser) GetMetadata() *DokuSettlementCSVMetadata {
	return p.metadata
}

// GetSkippedRows returns the count of skipped rows
func (p *DokuSettlementCSVParser) GetSkippedRows() int {
	return p.skippedRows
}

// GetTotalRows returns total rows processed (including skipped)
func (p *DokuSettlementCSVParser) GetTotalRows() int {
	return len(p.rows) + p.skippedRows
}

// HasErrors checks if there were any parsing errors
func (p *DokuSettlementCSVParser) HasErrors() bool {
	return len(p.parseErrors) > 0
}

// GetTotals calculates totals from parsed rows
func (p *DokuSettlementCSVParser) GetTotals() (grossAmount, netAmount, totalFee int64) {
	for _, row := range p.rows {
		grossAmount += row.Amount
		netAmount += row.PayToMerchant
		totalFee += row.Fee
	}
	return
}

// ToSettlementItem converts a CSV row to a SettlementItem
func (row *DokuSettlementCSVRow) ToSettlementItem(settlementBatchID string) (*SettlementItem, error) {
	return NewSettlementItem(
		settlementBatchID,
		row.InvoiceNumber,
		row.SubAccount,
		row.Amount,
		row.PayToMerchant,
		row.Fee,
		row.RowNumber,
		row.RawData,
	)
}
