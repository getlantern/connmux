// Package connmux provides the ability to multiplex streams over a single
// underlying net.Conn. Streams implement the net.Conn interface so to the user
// they look and work just like regular net.Conns, including support for read
// and write deadlines.
//
// Definitions:
//
//   physical connection - an underlying (e.g. TCP) connection
//   stream              - a virtual connection multiplexed over a physical connection
//   session             - unit for managing multiplexed streams, corresponds 1 to 1 with a physical connection
//
// Protocol:
//
//   Seession initiation
//
//      client --> start of session --> server
//
//   Write
//
//      client --> frame --> server
//      client <--  ack  <-- server
//
//   Read (parallel to write)
//
//      client <-- frame <-- server
//      client -->  ack  --> server
//
// Wire format:
//
//   start of session, 11 bytes
//
//     \0cmstart\0<version><window>
//
//       \0cmstart\0 - hardcoded sequence beginning and ending with \0 (NUL)
//                     byte that indicates beginning of session
//
//       version     - 1 byte, the version of the protocol (currently 1)
//
//       window      - 1 byte, the size of the transmit window, expressed in
//                     # of frames
//
//
//   data and control frames (positional, not delimited), maximum 8198 bytes
//
//     <T><SID><DLEN>[<DATA>]
//
//       T (frame type)     - 1 byte, indicates the frame type.
//                                0 = data frame
//                                1 = ack
//                                2 = rst (close connection)
//
//       SID (stream id)    - 3 bytes, unique identifier for stream.
//                                (last field for non-data messages)
//
//       DLEN (data length) - 2 bytes, length of data section
//
//       DATA               - Up to 8192 bytes, the data being transmitted
package connmux

import (
	"encoding/binary"

	"github.com/getlantern/golog"
	"github.com/oxtoacart/bpool"
)

const (
	sessionStart = "\000cmstart\000"

	// framing
	idLen          = 4
	lenLen         = 2
	frameHeaderLen = idLen + lenLen
	MaxDataLen     = 8192
	maxFrameLen    = frameHeaderLen + MaxDataLen

	// frame types
	frameTypeData = 0
	frameTypeACK  = 1
	frameTypeRST  = 2

	protocolVersion1 = 1
)

var (
	log = golog.LoggerFor("connmux")

	ErrTimeout          = &netError{"i/o timeout", true, true}
	ErrConnectionClosed = &netError{"connection closed", false, false}
	ErrListenerClosed   = &netError{"listener closed", false, false}

	binaryEncoding = binary.BigEndian

	sessionStartBytes     = []byte(sessionStart)
	sessionStartHeaderLen = len(sessionStartBytes)
	sessionStartTotalLen  = sessionStartHeaderLen + 2
)

type netError struct {
	err       string
	timeout   bool
	temporary bool
}

func (e *netError) Error() string   { return e.err }
func (e *netError) Timeout() bool   { return e.timeout }
func (e *netError) Temporary() bool { return e.temporary }

// BufferPool is a pool of reusable buffers
type BufferPool interface {
	// getForFrame gets a complete buffer large enough to hold an entire connmux
	// frame (8198 bytes).
	getForFrame() []byte

	// Get gets a truncated buffer sized to hold the data portion of a connmux
	// frame (8192 bytes)
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
	return p.pool.Get()[:MaxDataLen]
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
