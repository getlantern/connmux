// Package connmux provides the ability to multiplex streams over a single
// underlying net.Conn. Streams implement the net.Conn interface so to the user
// they look and work just like regular net.Conns, including support for read
// and write deadlines.
//
// Definitions:
//
//   physical connection - an underlying (e.g. TCP) connection
//
//   stream              - a virtual connection multiplexed over a physical
//                         connection
//
//   session             - unit for managing multiplexed streams, corresponds
//                         1 to 1 with a physical connection
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
//      client --> frame --> server
//      client --> frame --> server
//      ... continue up to window
//      client <--  ack  <-- server
//      client --> frame --> server
//      client <--  ack  <-- server
//      client <--  ack  <-- server
//      client --> frame --> server
//      client --> frame --> server
//      ... etc ...
//
//   Read (parallel to write)
//
//      client <-- frame <-- server
//      client <-- frame <-- server
//      client <-- frame <-- server
//      ... continue up to window
//      client -->  ack  --> server
//      client <-- frame <-- server
//      client -->  ack  --> server
//      client -->  ack  --> server
//      client <-- frame <-- server
//      client <-- frame <-- server
//      ... etc ...
//
// Wire format:
//
//   - all integers are unsigned BigEndian
//   - maximum length is 8,264
//
//   +------------+-----------+---------+----------+------+--------+
//   | Frame Type | Stream ID | MAC Len | Data Len |  MAC |  Data  |
//   +------------+-----------+---------+----------+------+--------+
//   |     1      |     3     |    1    |     3    | <=64 | <=8192 |
//   +------------+-----------+---------+----------+------+--------+
//
//   Frame Type - 1 byte, indicates the frame type.
//  		0 = data frame
//      1 = init crypto
//      2 = ack
//	  	3 = rst (close connection)
//
//   Stream ID - unique identifier for stream. (last field for ack and rst)
//
//   MAC Len - random length of MAC, only relevant if encrypting, else 0. The
//             actual length of the MAC is adjusted upward by a factor that's
//             determined uniquely for each session during the handshake.
//
//   Data Len - length of Data section
//
//   MAC - message authentication code, only used if encrypting.
//
//   Data - variable length data
//
// Init Crypto Message:
//
//   To initialize crypto, the client encrypts handshake information using the
//   server's RSA public key. The handshake information is:
//
//   +--------------+--------+----------------------------+--------------+
//   | Random Bytes | Secret | Initialization Vector (IV) | MAC Base Len |
//   +--------------+--------+----------------------------+--------------+
//   |    <= 32     |   16   |            16              |      1       |
//   +--------------+--------+----------------------------+--------------+
//
//   Random Bytes - some random bytes to vary the size of the message
//
//   Secret - 128 bits of secret for AES128
//
//   IV - 128 bits of initialization vector for CTR mode on AES128
//
//   MAC Base Len - Random integer between 0 and 32, used as a baseline for the
//   per-message MAC Len.
package connmux

import (
	"encoding/binary"
	"net"
	"time"

	"github.com/getlantern/golog"
	"github.com/oxtoacart/bpool"
)

const (
	sessionStart = "\000cmstart\000"

	// framing
	idLen          = 4
	lenLen         = 4
	frameHeaderLen = idLen + lenLen
	maxMacLen      = 64
	maxIVLen       = 64

	// MaxDataLen is the maximum length of data in a connmux frame.
	MaxDataLen = 8192

	maxFrameLen = frameHeaderLen + maxMacLen + maxIVLen + MaxDataLen

	// frame types
	frameTypeData = 0
	frameTypeACK  = 1
	frameTypeRST  = 2

	protocolVersion1 = 1

	maxID = (2 << 31) - 1
)

var (
	log = golog.LoggerFor("connmux")

	ErrTimeout          = &netError{"i/o timeout", true, true}
	ErrConnectionClosed = &netError{"connection closed", false, false}
	ErrBrokenPipe       = &netError{"broken pipe", false, false}
	ErrListenerClosed   = &netError{"listener closed", false, false}

	binaryEncoding = binary.BigEndian

	sessionStartBytes     = []byte(sessionStart)
	sessionStartHeaderLen = len(sessionStartBytes)
	sessionStartTotalLen  = sessionStartHeaderLen + 2

	largeTimeout  = 100000 * time.Hour
	largeDeadline = time.Now().Add(100000 * time.Hour)
)

// netError implements the interface net.Error
type netError struct {
	err       string
	timeout   bool
	temporary bool
}

func (e *netError) Error() string   { return e.err }
func (e *netError) Timeout() bool   { return e.timeout }
func (e *netError) Temporary() bool { return e.temporary }

// Session is a wrapper around a net.Conn that supports multiplexing.
type Session interface {
	net.Conn

	// Wrapped() exposes access to the net.Conn that's wrapped by this Session.
	Wrapped() net.Conn
}

// Stream is a net.Conn that also exposes access to the underlying Session
type Stream interface {
	net.Conn

	// Session() exposes access to the Session on which this Stream is running.
	Session() Session

	// Wrapped() exposes the wrapped connection (same thing as Session(), but
	// implements netx.WrappedConn interface)
	Wrapped() net.Conn
}

// BufferPool is a pool of reusable buffers
type BufferPool interface {
	// getForFrame gets a complete buffer large enough to hold an entire connmux
	// frame
	getForFrame() []byte

	// Get gets a truncated buffer sized to hold the data portion of a connmux
	// frame
	Get() []byte

	// Put returns a buffer back to the pool, indicating that it is safe to
	// reuse.
	Put([]byte)
}

// NewBufferPool constructs a BufferPool with the given maximumSize
func NewBufferPool(maxSize int) BufferPool {
	return &bufferPool{bpool.NewBytePool(maxSize, maxFrameLen)}
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
