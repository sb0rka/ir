package maxpatrol

import (
	"fmt"
	"net/http"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

// AccessError contains no credential material.
type AccessError struct {
	Message string
}

func (err *AccessError) Error() string {
	return "MaxPatrol access rejected: " + err.Message
}

// RequestError describes a locally rejected fixed-template request.
type RequestError struct {
	Operation string
	Message   string
}

func (err *RequestError) Error() string {
	return fmt.Sprintf("MaxPatrol %s: %s", err.Operation, err.Message)
}

// TransportError deliberately does not retain or unwrap the original
// url.Error, because it contains the requested vendor URL and query string.
type TransportError struct {
	Operation        string
	TimedOut         bool
	TemporaryFailure bool
}

func (err *TransportError) Error() string {
	return fmt.Sprintf("MaxPatrol %s transport failed", err.Operation)
}

func (err *TransportError) Timeout() bool { return err.TimedOut }

func (err *TransportError) Temporary() bool { return err.TemporaryFailure }

// HTTPError exposes only status and a fixed operation name. Response bodies,
// redirect locations, and URLs do not cross the adapter boundary.
type HTTPError struct {
	Operation  string
	StatusCode int
}

func (err *HTTPError) Error() string {
	return fmt.Sprintf("MaxPatrol %s returned HTTP %d", err.Operation, err.StatusCode)
}

func (err *HTTPError) Retryable() bool {
	return err.StatusCode == http.StatusUnauthorized ||
		err.StatusCode == http.StatusForbidden ||
		err.StatusCode >= http.StatusInternalServerError
}

// ResponseError reports an invalid bounded vendor response without retaining
// or quoting any part of that response.
type ResponseError struct {
	Operation string
	Message   string
}

func (err *ResponseError) Error() string {
	return fmt.Sprintf("MaxPatrol %s response rejected: %s", err.Operation, err.Message)
}

type NotFoundError struct {
	Kind       string
	ExternalID string
}

func (err *NotFoundError) Error() string {
	return fmt.Sprintf("MaxPatrol %s %s was not found", err.Kind, err.ExternalID)
}

// ContextError is safe to persist or return as a partial-context warning.
// Message is always adapter-owned and never contains a vendor response body,
// URL, cookie, or arbitrary error text.
type ContextError struct {
	Component string `json:"component"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func contextError(component string, err error) ContextError {
	value := ContextError{
		Component: component,
		Code:      "upstream_failed",
		Message:   component + " context is unavailable",
	}
	switch typed := err.(type) {
	case *AccessError:
		value.Code = "access_rejected"
		value.Retryable = true
	case *TransportError:
		value.Code = "transport_failed"
		value.Retryable = true
	case *HTTPError:
		value.Retryable = typed.Retryable()
		if typed.StatusCode == http.StatusNotFound {
			value.Code = "not_found"
		} else if typed.StatusCode == http.StatusUnauthorized || typed.StatusCode == http.StatusForbidden {
			value.Code = "access_rejected"
		}
	case *NotFoundError:
		value.Code = "not_found"
	case *RequestError:
		value.Code = "invalid_request"
	case *ResponseError:
		value.Code = "invalid_response"
	}
	return value
}

func retryableContextFailure(values []ContextError) error {
	for _, value := range values {
		if value.Retryable {
			return &domain.UpstreamError{StatusCode: http.StatusServiceUnavailable}
		}
	}
	return nil
}
