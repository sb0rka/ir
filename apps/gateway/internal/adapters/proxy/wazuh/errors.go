package wazuh

import (
	"fmt"
)

type RequestError struct {
	Operation string
	Message   string
}

func (err *RequestError) Error() string {
	return fmt.Sprintf("Wazuh %s: %s", err.Operation, err.Message)
}

type TransportError struct {
	Operation        string
	TimedOut         bool
	TemporaryFailure bool
}

func (err *TransportError) Error() string {
	return fmt.Sprintf("Wazuh %s transport failed", err.Operation)
}

func (err *TransportError) Timeout() bool   { return err.TimedOut }
func (err *TransportError) Temporary() bool { return err.TemporaryFailure }

type ResponseError struct {
	Operation string
	Message   string
}

func (err *ResponseError) Error() string {
	return fmt.Sprintf("Wazuh %s response rejected: %s", err.Operation, err.Message)
}
