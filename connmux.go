// Package connmux provides the ability to multiplex streams over a single
// underlying net.Conn. Streams implement the net.Conn interface so to the user
// they look and work just like regular net.Conns, including support for read
// and write deadlines.
package connmux

import (
	"encoding/binary"
	"errors"
	"time"
)

const (
	sessionStart = "\000cmstart\000"

	connClose = 1

	idlen = 4
)

var (
	ErrBufferFull       = errors.New("buffer full")
	ErrBufferOverflowed = errors.New("buffer overflowed, stream no longer readable")
	ErrTimeout          = &timeoutError{}
	ErrConnectionClosed = errors.New("connection closed") // TODO: make a net.Error?
	ErrListenerClosed   = errors.New("listener closed")   // TODO: make a net.Error?

	binaryEncoding = binary.BigEndian

	sessionStartBytes  = []byte(sessionStart)
	sessionStartLength = len(sessionStartBytes)
)

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

type Buffer interface {
	Write(b []byte) error

	Read(b []byte, deadline time.Time) (int, error)

	Close()

	Raw() []byte
}

type BufferSource interface {
	Get() []byte
	Put([]byte)
}
