package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound              = errors.New("not found")
	ErrUnsupportedCapability = errors.New("unsupported capability")
	ErrAllSourcesFailed      = errors.New("all sources failed")
)

type RequestError struct {
	Code    string
	Message string
}

func (err *RequestError) Error() string { return err.Message }

// UpstreamError records only the status needed for retry and safe error
// mapping. Vendor response bodies never cross the adapter boundary.
type UpstreamError struct {
	StatusCode int
	Message    string
}

func (err *UpstreamError) Error() string {
	if err.Message != "" {
		return err.Message
	}
	return fmt.Sprintf("upstream returned HTTP %d", err.StatusCode)
}
