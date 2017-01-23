// Package connmux provides the ability to multiplex streams over a single
// underlying net.Conn. Streams implement the net.Conn interface so to the user
// they look and work just like regular net.Conns, including support for read
// and write deadlines.
//
// Framing format:
//
//   \0cmstart\0 - this special sequence is sent to indicate the start of a
//                 a multiplexed session, to distinguish it from regular traffic.
//
//   T|SID|DLEN|DATA - format of frames in multiplexed session
//
//     T (frame type)     - 1 byte, indicates the frame type. 0 = data frame, 1 = ack, 2 = rst (close connection)
//     SID (stream id)    - 3 bytes, unique identifier for stream
//     DLEN (data length) - 4 bytes, indicates the length of the following data section
//
// This makes the maximum total frame size 65544 bytes.
package connmux

import (
	"encoding/binary"
	"errors"

	"github.com/oxtoacart/bpool"
)

const (
	sessionStart = "\000cmstart\000"

	// framing
	idLen          = 4
	lenLen         = 2
	frameHeaderLen = idLen + lenLen
	maxDataLen     = 2<<15 - 1
	maxFrameLen    = frameHeaderLen + maxDataLen

	// frame types
	frameTypeData = 0
	frameTypeACK  = 1
	frameTypeRST  = 2
)

var (
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

// BufferPool is a pool of reusable buffers
type BufferPool interface {
	// getForFrame gets a complete buffer large enough to hold an entire connmux frame (65,541 bytes)
	getForFrame() []byte

	// Get gets a truncated buffer sized to hold the data portion of a connmux frame (65,535 bytes)
	Get() []byte

	// Put returns a buffer back to the pool, indicating that it is safe to
	// reuse.
	Put([]byte)
}

func NewBufferPool(size int) BufferPool {
	return &bufferPool{bpool.NewBytePool(size, maxFrameLen)}
}

type bufferPool struct {
	pool *bpool.BytePool
}

func (p *bufferPool) getForFrame() []byte {
	return p.pool.Get()
}

func (p *bufferPool) Get() []byte {
	return p.pool.Get()[:maxDataLen]
}

func (p *bufferPool) Put(b []byte) {
	p.pool.Put(b)
}

func frameType(b []byte) byte {
	return b[0]
}

func setFrameType(b []byte, frameType byte) {
	b[0] = frameType
}
