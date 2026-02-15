package ledgererr

import (
	"errors"
	"fmt"
)

type ErrorCode int

type AppError struct {
	Code        ErrorCode
	Msg         string
	OriginError error
}

func NewError(code ErrorCode, msg string, origin error) AppError {
	return AppError{
		Code:        code,
		Msg:         msg,
		OriginError: origin,
	}
}

func (err *AppError) WithError(origin error) AppError {
	return AppError{
		Code:        err.Code,
		Msg:         err.Msg,
		OriginError: origin,
	}
}

func (err AppError) Error() string {
	if err.OriginError != nil {
		return fmt.Sprintf("%s: %s", err.Msg, err.OriginError.Error())
	}
	return err.Msg
}

func (err AppError) ErrCode() ErrorCode {
	var originErr AppError
	if errors.As(err.OriginError, &originErr) {
		return originErr.ErrCode()
	}
	return err.Code
}

func (err AppError) Unwrap() error {
	return err.OriginError
}

func IsAppError(target error, err AppError) bool {
	var appErr AppError
	if errors.As(target, &appErr) {
		return appErr.ErrCode() == err.ErrCode()
	}
	return false
}

func IsErrorCode(code ErrorCode, err error) bool {
	var appErr AppError
	if errors.As(err, &appErr) {
		return appErr.ErrCode() == code
	}
	return false
}

const (
	CodeInternal       ErrorCode = 500
	CodeNotFound       ErrorCode = 404
	CodeInvalidRequest ErrorCode = 400

	CodeDatabaseError           ErrorCode = 500001
	CodeDokuAPIError            ErrorCode = 500002
	CodeSubaccountAlreadyExists ErrorCode = 409001

	// Ledger error codes
	CodeLedgerNotFound                 ErrorCode = 404001
	CodeLedgerAlreadyExists            ErrorCode = 409002
	CodeReconciliationDiscrepancyFound ErrorCode = 409003

	// ProductTransaction error codes
	CodeProductTransactionNotFound      ErrorCode = 404002
	CodeProductTransactionAlreadyExists ErrorCode = 409004
	CodeInvalidTransactionStatus        ErrorCode = 400001
	CodeInvalidFeeBreakdown             ErrorCode = 400002

	// PaymentRequest error codes
	CodePaymentRequestNotFound      ErrorCode = 404003
	CodePaymentRequestAlreadyExists ErrorCode = 409005
	CodeInvalidPaymentStatus        ErrorCode = 400003
	CodePaymentExpired              ErrorCode = 400004

	// FeeConfig error codes
	CodeFeeConfigNotFound ErrorCode = 404004
)

// Ledger errors
var (
	ErrLedgerNotFound                 = NewError(CodeLedgerNotFound, "ledger not found", nil)
	ErrLedgerAlreadyExists            = NewError(CodeLedgerAlreadyExists, "ledger already exists", nil)
	ErrReconciliationDiscrepancyFound = NewError(CodeReconciliationDiscrepancyFound, "reconciliation discrepancy found", nil)
)

// ProductTransaction errors
var (
	ErrProductTransactionNotFound      = NewError(CodeProductTransactionNotFound, "product transaction not found", nil)
	ErrProductTransactionAlreadyExists = NewError(CodeProductTransactionAlreadyExists, "product transaction already exists", nil)
	ErrInvalidTransactionStatus        = NewError(CodeInvalidTransactionStatus, "invalid transaction status transition", nil)
	ErrInvalidFeeBreakdown             = NewError(CodeInvalidFeeBreakdown, "invalid fee breakdown", nil)
)

// PaymentRequest errors
var (
	ErrPaymentRequestNotFound      = NewError(CodePaymentRequestNotFound, "payment request not found", nil)
	ErrPaymentRequestAlreadyExists = NewError(CodePaymentRequestAlreadyExists, "payment request already exists", nil)
	ErrInvalidPaymentStatus        = NewError(CodeInvalidPaymentStatus, "invalid payment status transition", nil)
	ErrPaymentExpired              = NewError(CodePaymentExpired, "payment request has expired", nil)
)

// FeeConfig errors
var (
	ErrFeeConfigNotFound = NewError(CodeFeeConfigNotFound, "fee config not found", nil)
)
