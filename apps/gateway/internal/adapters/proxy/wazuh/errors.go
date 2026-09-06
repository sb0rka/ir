package wazuh

import (
	"fmt"
	"net/http"
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

type HTTPError struct {
	Operation  string
	StatusCode int
}

func (err *HTTPError) Error() string {
	return fmt.Sprintf("Wazuh %s returned HTTP %d", err.Operation, err.StatusCode)
}

func (err *HTTPError) Retryable() bool {
	return err.StatusCode == http.StatusUnauthorized ||
		err.StatusCode == http.StatusForbidden ||
		err.StatusCode >= http.StatusInternalServerError
}

type ResponseError struct {
	Operation string
	Message   string
}

func (err *ResponseError) Error() string {
	return fmt.Sprintf("Wazuh %s response rejected: %s", err.Operation, err.Message)
}

type NotFoundError struct {
	ExternalID string
}

func (err *NotFoundError) Error() string {
	return fmt.Sprintf("Wazuh record %s was not found", err.ExternalID)
}
