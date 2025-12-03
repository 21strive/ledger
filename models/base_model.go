package models

type ErrorLog struct {
	Line          string      `json:"line,omitempty"`
	Filename      string      `json:"filename,omitempty"`
	Function      string      `json:"function,omitempty"`
	Message       interface{} `json:"message,omitempty"`
	SystemMessage interface{} `json:"system_message,omitempty"`
	Err           error       `json:"-"`
	StatusCode    int         `json:"-"`
}
