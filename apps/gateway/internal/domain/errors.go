package domain

import "errors"

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
