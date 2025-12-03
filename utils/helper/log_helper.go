package helper

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"

	"github.com/faizauthar12/ledger/models"

	"github.com/sirupsen/logrus"
)

// var DefaultStatusText = map[int]string{
// 	http.StatusInternalServerError: "Something went wrong, please try again later",
// 	http.StatusNotFound:            "data not found",
// }

var DefaultStatusText = map[int]string{
	http.StatusInternalServerError: "Terjadi Kesalahan, Silahkan Coba lagi Nanti",
	http.StatusNotFound:            "Data tidak Ditemukan",
	http.StatusBadRequest:          "Ada kesalahan pada request data, silahkan dicek kembali",
}

func WriteLog(err error, errorCode int, message interface{}) *models.ErrorLog {
	if pc, file, line, ok := runtime.Caller(1); ok {
		file = file[strings.LastIndex(file, "/")+1:]
		funcName := runtime.FuncForPC(pc).Name()
		output := &models.ErrorLog{
			StatusCode: errorCode,
			Err:        err,
		}
		outputForPrint := &models.ErrorLog{
			StatusCode: errorCode,
			Err:        err,
			Line:       fmt.Sprintf("%d", line),
			Filename:   file,
			Function:   funcName,
		}

		output.SystemMessage = err.Error()
		if message == nil {
			output.Message = DefaultStatusText[errorCode]
			if output.Message == "" {
				output.Message = http.StatusText(errorCode)
				outputForPrint.Message = http.StatusText(errorCode)
			}
		} else {
			output.Message = message
			outputForPrint.Message = message
		}
		if errorCode == http.StatusInternalServerError {
			output.Line = fmt.Sprintf("%d", line)
			output.Filename = file
			output.Function = funcName
		}

		logForPrint := map[string]interface{}{}
		_ = DecodeMapType(outputForPrint, &logForPrint)

		log := map[string]interface{}{}
		_ = DecodeMapType(output, &log)
		logrus.WithFields(logForPrint).Error(err)
		return output
	}

	return nil
}
