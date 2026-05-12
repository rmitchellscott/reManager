package errors

import (
	"encoding/json"
)

type UserError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Internal  string `json:"internal,omitempty"`
	Retryable bool   `json:"retryable"`
}

func (e *UserError) Error() string {
	return e.Message
}

func (e *UserError) JSON() string {
	b, _ := json.Marshal(e)
	return string(b)
}

func New(code, message string, internal error, retryable bool) *UserError {
	internalMsg := ""
	if internal != nil {
		internalMsg = internal.Error()
	}
	return &UserError{
		Code:      code,
		Message:   message,
		Internal:  internalMsg,
		Retryable: retryable,
	}
}

