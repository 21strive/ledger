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
	CodeInternal ErrorCode = 500
	CodeNotFound ErrorCode = 404

	CodeDatabaseError           ErrorCode = 50001
	CodeDokuAPIError            ErrorCode = 50002
	CodeSubaccountAlreadyExists ErrorCode = 409001
)

var (
	ErrLedgerAlreadyExists = NewError(409100, "ledger already exists", nil)
)
