package domain

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestDokuSettlementCSVParser_ParseTemplateFile(t *testing.T) {
	// Test parsing the actual report-template.csv file
	file, err := os.Open("../report-template.csv")
	if err != nil {
		t.Fatalf("Failed to open report-template.csv: %v", err)
	}
	defer file.Close()

	parser := NewDokuSettlementCSVParser("02-01-2006", 1)
	err = parser.Parse(file)
	if err != nil {
		t.Fatalf("Failed to parse CSV: %v", err)
	}

	// Validate metadata
	metadata := parser.GetMetadata()
	if metadata.BatchID != "B-BSN-0203-1761932477260-SBS-8298-20251109155312120-20260305210108875" {
		t.Errorf("Expected BatchID 'B-BSN-0203-1761932477260-SBS-8298-20251109155312120-20260305210108875', got '%s'", metadata.BatchID)
	}
	if metadata.TotalAmountPurchase != 504440 {
		t.Errorf("Expected TotalAmountPurchase 504440, got %d", metadata.TotalAmountPurchase)
	}
	if metadata.TotalFee != 4995 {
		t.Errorf("Expected TotalFee 4995, got %d", metadata.TotalFee)
	}
	if metadata.TotalPurchase != 1 {
		t.Errorf("Expected TotalPurchase 1, got %d", metadata.TotalPurchase)
	}
	if metadata.TotalSettlement != 499445 {
		t.Errorf("Expected TotalSettlement 499445, got %d", metadata.TotalSettlement)
	}
	if metadata.TotalTransactions != 1 {
		t.Errorf("Expected TotalTransactions 1, got %d", metadata.TotalTransactions)
	}

	// Validate rows
	rows := parser.GetRows()
	if len(rows) != 1 {
		t.Fatalf("Expected 1 data row, got %d", len(rows))
	}

	row := rows[0]
	if row.No != 1 {
		t.Errorf("Expected No 1, got %d", row.No)
	}
	if row.MerchantName != "Bernino Falya" {
		t.Errorf("Expected MerchantName 'Bernino Falya', got '%s'", row.MerchantName)
	}
	if row.PaymentChannelName != "Virtual Account BCA" {
		t.Errorf("Expected PaymentChannelName 'Virtual Account BCA', got '%s'", row.PaymentChannelName)
	}
	if row.InvoiceNumber != "aj-faharahnf-tambal-ban-1772718831" {
		t.Errorf("Expected InvoiceNumber 'aj-faharahnf-tambal-ban-1772718831', got '%s'", row.InvoiceNumber)
	}
	if row.CustomerName != "Fahar" {
		t.Errorf("Expected CustomerName 'Fahar', got '%s'", row.CustomerName)
	}
	if row.Amount != 504440 {
		t.Errorf("Expected Amount 504440, got %d", row.Amount)
	}
	if row.Fee != 4995 {
		t.Errorf("Expected Fee 4995, got %d", row.Fee)
	}
	if row.PayToMerchant != 499445 {
		t.Errorf("Expected PayToMerchant 499445, got %d", row.PayToMerchant)
	}
	if row.TransactionType != "Purchase" {
		t.Errorf("Expected TransactionType 'Purchase', got '%s'", row.TransactionType)
	}
	if row.SubAccount != "SAC-6004-1772309804461" {
		t.Errorf("Expected SubAccount 'SAC-6004-1772309804461', got '%s'", row.SubAccount)
	}

	// Validate dates
	expectedTxDate := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	expectedPayoutDate := time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC)
	if !row.TransactionDate.Equal(expectedTxDate) {
		t.Errorf("Expected TransactionDate %v, got %v", expectedTxDate, row.TransactionDate)
	}
	if !row.PayOutDate.Equal(expectedPayoutDate) {
		t.Errorf("Expected PayOutDate %v, got %v", expectedPayoutDate, row.PayOutDate)
	}

	// Check no errors
	if parser.HasErrors() {
		t.Errorf("Parser reported errors: %v", parser.GetParseErrors())
	}
}

func TestDokuSettlementCSVParser_WithBlankLine(t *testing.T) {
	csvContent := `Total Amount Purchase_,504440
Total Fee_,4995
Total Purchase_,1
Total Amount Refund_,0
Total Refund_,0
Total Settlement Amount_,499445
Total Discount_,0
Total Transactions_,1
Batch ID_,B-BSN-TEST-123

NO,MERCHANT NAME,PAYMENT CHANNEL NAME,TRANSACTION DATE,INVOICE NUMBER,CUSTOMER NAME,REPORT CODE,AMOUNT,RECON CODE,FEE,DISCOUNT,PAY TO MERCHANT,PAY OUT DATE,TRANSACTION TYPE,PROMO CODE,SUB ACCOUNT
1,Test Merchant,Virtual Account,05-03-2026,INV-001,Customer,RC-001,100000,RC-001,5000,0,95000,06-03-2026,Purchase,,SAC-001
`

	parser := NewDokuSettlementCSVParser("02-01-2006", 1)
	err := parser.Parse(strings.NewReader(csvContent))
	if err != nil {
		t.Fatalf("Failed to parse CSV with blank line: %v", err)
	}

	rows := parser.GetRows()
	if len(rows) != 1 {
		t.Errorf("Expected 1 row, got %d", len(rows))
	}
}

func TestDokuSettlementCSVParser_WithoutBlankLine(t *testing.T) {
	csvContent := `Total Amount Purchase_,504440
Total Fee_,4995
Total Purchase_,1
Total Amount Refund_,0
Total Refund_,0
Total Settlement Amount_,499445
Total Discount_,0
Total Transactions_,1
Batch ID_,B-BSN-TEST-123
NO,MERCHANT NAME,PAYMENT CHANNEL NAME,TRANSACTION DATE,INVOICE NUMBER,CUSTOMER NAME,REPORT CODE,AMOUNT,RECON CODE,FEE,DISCOUNT,PAY TO MERCHANT,PAY OUT DATE,TRANSACTION TYPE,PROMO CODE,SUB ACCOUNT
1,Test Merchant,Virtual Account,05-03-2026,INV-001,Customer,RC-001,100000,RC-001,5000,0,95000,06-03-2026,Purchase,,SAC-001
`

	parser := NewDokuSettlementCSVParser("02-01-2006", 1)
	err := parser.Parse(strings.NewReader(csvContent))
	if err != nil {
		t.Fatalf("Failed to parse CSV without blank line: %v", err)
	}

	rows := parser.GetRows()
	if len(rows) != 1 {
		t.Errorf("Expected 1 row, got %d", len(rows))
	}
}

func TestDokuSettlementCSVParser_WithMultipleBlankLines(t *testing.T) {
	csvContent := `Total Amount Purchase_,504440
Total Fee_,4995
Total Purchase_,1
Total Amount Refund_,0
Total Refund_,0
Total Settlement Amount_,499445
Total Discount_,0
Total Transactions_,1
Batch ID_,B-BSN-TEST-123


NO,MERCHANT NAME,PAYMENT CHANNEL NAME,TRANSACTION DATE,INVOICE NUMBER,CUSTOMER NAME,REPORT CODE,AMOUNT,RECON CODE,FEE,DISCOUNT,PAY TO MERCHANT,PAY OUT DATE,TRANSACTION TYPE,PROMO CODE,SUB ACCOUNT
1,Test Merchant,Virtual Account,05-03-2026,INV-001,Customer,RC-001,100000,RC-001,5000,0,95000,06-03-2026,Purchase,,SAC-001
`

	parser := NewDokuSettlementCSVParser("02-01-2006", 1)
	err := parser.Parse(strings.NewReader(csvContent))
	if err != nil {
		t.Fatalf("Failed to parse CSV with multiple blank lines: %v", err)
	}

	rows := parser.GetRows()
	if len(rows) != 1 {
		t.Errorf("Expected 1 row, got %d", len(rows))
	}
}

func TestDokuSettlementCSVParser_InvalidHeader(t *testing.T) {
	// CSV with data row instead of header after metadata
	csvContent := `Total Amount Purchase_,504440
Total Fee_,4995
Total Purchase_,1
Total Amount Refund_,0
Total Refund_,0
Total Settlement Amount_,499445
Total Discount_,0
Total Transactions_,1
Batch ID_,B-BSN-TEST-123
1,Test Merchant,Virtual Account,05-03-2026,INV-001,Customer,RC-001,100000,RC-001,5000,0,95000,06-03-2026,Purchase,,SAC-001
2,Test Merchant 2,QRIS,06-03-2026,INV-002,Customer2,RC-002,200000,RC-002,10000,0,190000,07-03-2026,Purchase,,SAC-002
`

	parser := NewDokuSettlementCSVParser("02-01-2006", 1)
	err := parser.Parse(strings.NewReader(csvContent))

	// Should return error because first row after metadata is not a valid header
	if err == nil {
		t.Error("Expected error for invalid header, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "expected header row") {
		t.Errorf("Expected error message about header, got: %v", err)
	}
}

func TestIsHeaderRow(t *testing.T) {
	tests := []struct {
		name     string
		row      []string
		expected bool
	}{
		{
			name: "Valid header",
			row: []string{
				"NO", "MERCHANT NAME", "PAYMENT CHANNEL NAME", "TRANSACTION DATE",
				"INVOICE NUMBER", "CUSTOMER NAME", "REPORT CODE", "AMOUNT",
				"RECON CODE", "FEE", "DISCOUNT", "PAY TO MERCHANT",
			},
			expected: true,
		},
		{
			name: "Valid header with different case",
			row: []string{
				"no", "merchant name", "payment channel name", "transaction date",
				"invoice number", "customer name", "report code", "amount",
				"recon code", "fee", "discount", "pay to merchant",
			},
			expected: true,
		},
		{
			name: "Data row (should not be detected as header)",
			row: []string{
				"1", "Muhammad Rifqi A", "Virtual Account Mandiri", "11-03-2026",
				"INV-20260311000630-KBEWQH", "rifqoi", "8889940000206089", "51000",
				"8889940000206089", "4995", "0", "46005",
			},
			expected: false,
		},
		{
			name: "Partial match (less than 3 key columns)",
			row: []string{
				"NO", "Something", "Else", "HERE",
				"INVOICE NUMBER", "Other", "Data", "Values",
				"More", "Data", "Here", "Now",
			},
			expected: false,
		},
		{
			name: "Too few columns",
			row: []string{
				"NO", "MERCHANT NAME", "TRANSACTION DATE",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isHeaderRow(tt.row)
			if result != tt.expected {
				t.Errorf("isHeaderRow() = %v, want %v for row: %v", result, tt.expected, tt.row)
			}
		})
	}
}

func TestDokuSettlementCSVParser_MultipleTransactions(t *testing.T) {
	csvContent := `Total Amount Purchase_,604440
Total Fee_,9990
Total Purchase_,2
Total Amount Refund_,0
Total Refund_,0
Total Settlement Amount_,594450
Total Discount_,0
Total Transactions_,2
Batch ID_,B-BSN-TEST-MULTI-123

NO,MERCHANT NAME,PAYMENT CHANNEL NAME,TRANSACTION DATE,INVOICE NUMBER,CUSTOMER NAME,REPORT CODE,AMOUNT,RECON CODE,FEE,DISCOUNT,PAY TO MERCHANT,PAY OUT DATE,TRANSACTION TYPE,PROMO CODE,SUB ACCOUNT
1,Merchant A,Virtual Account BCA,05-03-2026,INV-001,Customer A,RC-001,304440,RC-001,4995,0,299445,06-03-2026,Purchase,,SAC-001
2,Merchant B,QRIS,06-03-2026,INV-002,Customer B,RC-002,300000,RC-002,4995,0,295005,07-03-2026,Purchase,,SAC-002
`

	parser := NewDokuSettlementCSVParser("02-01-2006", 1)
	err := parser.Parse(strings.NewReader(csvContent))
	if err != nil {
		t.Fatalf("Failed to parse CSV: %v", err)
	}

	// Validate metadata
	metadata := parser.GetMetadata()
	if metadata.TotalAmountPurchase != 604440 {
		t.Errorf("Expected TotalAmountPurchase 604440, got %d", metadata.TotalAmountPurchase)
	}
	if metadata.TotalTransactions != 2 {
		t.Errorf("Expected TotalTransactions 2, got %d", metadata.TotalTransactions)
	}

	// Validate rows
	rows := parser.GetRows()
	if len(rows) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(rows))
	}

	// Check first row
	if rows[0].InvoiceNumber != "INV-001" {
		t.Errorf("Expected first invoice 'INV-001', got '%s'", rows[0].InvoiceNumber)
	}
	if rows[0].SubAccount != "SAC-001" {
		t.Errorf("Expected first SubAccount 'SAC-001', got '%s'", rows[0].SubAccount)
	}

	// Check second row
	if rows[1].InvoiceNumber != "INV-002" {
		t.Errorf("Expected second invoice 'INV-002', got '%s'", rows[1].InvoiceNumber)
	}
	if rows[1].SubAccount != "SAC-002" {
		t.Errorf("Expected second SubAccount 'SAC-002', got '%s'", rows[1].SubAccount)
	}
}

func TestDokuSettlementCSVParser_MetadataValidation(t *testing.T) {
	csvContent := `Total Amount Purchase_,504440
Total Fee_,4995
Total Purchase_,1
Total Amount Refund_,0
Total Refund_,0
Total Settlement Amount_,499445
Total Discount_,0
Total Transactions_,1
Batch ID_,

NO,MERCHANT NAME,PAYMENT CHANNEL NAME,TRANSACTION DATE,INVOICE NUMBER,CUSTOMER NAME,REPORT CODE,AMOUNT,RECON CODE,FEE,DISCOUNT,PAY TO MERCHANT,PAY OUT DATE,TRANSACTION TYPE,PROMO CODE,SUB ACCOUNT
1,Test Merchant,Virtual Account,05-03-2026,INV-001,Customer,RC-001,100000,RC-001,5000,0,95000,06-03-2026,Purchase,,SAC-001
`

	parser := NewDokuSettlementCSVParser("02-01-2006", 1)
	err := parser.Parse(strings.NewReader(csvContent))

	// Should return error because Batch ID is required
	if err == nil {
		t.Error("Expected error for missing Batch ID, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "Batch ID") {
		t.Errorf("Expected error about Batch ID, got: %v", err)
	}
}

func TestDokuSettlementCSVParser_AmountParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
		wantErr  bool
	}{
		{"Simple number", "504440", 504440, false},
		{"With comma", "504,440", 504440, false},
		{"Empty", "", 0, false},
		{"With Rp prefix", "Rp 504440", 504440, false},
		{"With IDR", "IDR 504440", 504440, false},
		{"With dots", "504.440", 504440, false},
	}

	parser := NewDokuSettlementCSVParser("02-01-2006", 1)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.parseAmount(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseAmount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("parseAmount() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDokuSettlementCSVParser_DateParsing(t *testing.T) {
	parser := NewDokuSettlementCSVParser("02-01-2006", 1)

	tests := []struct {
		name     string
		input    string
		expected time.Time
		wantErr  bool
	}{
		{
			name:     "DD-MM-YYYY format",
			input:    "05-03-2026",
			expected: time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "YYYY-MM-DD format",
			input:    "2026-03-05",
			expected: time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:    "Invalid format",
			input:   "invalid-date",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.tryParseDateAlternatives(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("tryParseDateAlternatives() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !result.Equal(tt.expected) {
				t.Errorf("tryParseDateAlternatives() = %v, want %v", result, tt.expected)
			}
		})
	}
}
