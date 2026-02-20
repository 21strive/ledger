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
// CSV Format: No,MERCHANT NAME,PAYMENT CHANNEL NAME,TRANSACTION DATE,INVOICE NUMBER,CUSTOMER NAME,REPORT CODE,AMOUNT,RECON CODE,FEE,DISCOUNT,PAY TO MERCHANT,PAY OUT DATE,TRANSACTION TYPE,PROMO CODE
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
	RawData            map[string]string // Original CSV row data
}

// DokuSettlementCSVParser handles parsing DOKU settlement CSV files
type DokuSettlementCSVParser struct {
	rows          []*DokuSettlementCSVRow
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
	minColumnCount        = 12 // Minimum required columns
)

// Parse reads and parses a DOKU settlement CSV from a reader
func (p *DokuSettlementCSVParser) Parse(reader io.Reader) error {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true
	csvReader.FieldsPerRecord = -1 // Allow variable number of fields

	// Read header row
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

// GetRows returns all successfully parsed rows
func (p *DokuSettlementCSVParser) GetRows() []*DokuSettlementCSVRow {
	return p.rows
}

// GetParseErrors returns all parsing errors
func (p *DokuSettlementCSVParser) GetParseErrors() []error {
	return p.parseErrors
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
		row.Amount,
		row.PayToMerchant,
		row.Fee,
		row.RowNumber,
		row.RawData,
	)
}
