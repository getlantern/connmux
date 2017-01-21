package connmux

import (
	"encoding/binary"
	"errors"
	"time"
)

const (
	start = "\000cmstrt\000"
	stop  = "\000cmstop\000"
)

var (
	ErrBufferFull       = errors.New("buffer full")
	ErrBufferOverflowed = errors.New("buffer overflowed, connection no longer readable")
	ErrTimeout          = &timeoutError{}
	ErrListenerClosed   = errors.New("listener closed")

	binaryEncoding = binary.BigEndian

	controlLength = len(start)
)

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

type Buffer interface {
	Write(b []byte) error

	Read(b []byte, deadline time.Time) (int, error)

	Close()
}
