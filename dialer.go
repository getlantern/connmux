package connmux

import (
	"net"
	"sync"

	"github.com/getlantern/framed"
)

// Dialer wraps the given dial function with support for multiplexing. The
// returned "streams" look and act just like regular net.Conns. The Dialer
// will multiplex everything over a single net.Conn until it encounters a read
// or write error on that Conn. At that point, it will dial a new conn for
// future streams, until there's a problem with that Conn, and so on and so
// forth.
//
// sessionBufferSource - a source of buffers for the session's read loop. A
// good width for these is 70,000 bytes.
//
// streamBufferSource - a source of buffers for each stream. These should be
// large enough to accomodate slow readers that may need to buffer a lot of
// data. If a buffer fills before the reader can drain it, the stream will fail
// with ErrBufferOverflowed.
func Dialer(sessionBufferSource BufferSource, streamBufferSource BufferSource, dial func() (net.Conn, error)) func() (net.Conn, error) {
	d := &dialer{
		doDial:              dial,
		sessionBufferSource: sessionBufferSource,
		streamBufferSource:  streamBufferSource,
	}
	return d.dial
}

type dialer struct {
	doDial              func() (net.Conn, error)
	sessionBufferSource BufferSource
	streamBufferSource  BufferSource
	current             *session
	id                  uint32
	mx                  sync.Mutex
}

func (d *dialer) dial() (net.Conn, error) {
	d.mx.Lock()
	current := d.current
	if current == nil {
		conn, err := d.doDial()
		if err != nil {
			d.mx.Unlock()
			return nil, err
		}
		_, writeErr := conn.Write(sessionStartBytes)
		if writeErr != nil {
			conn.Close()
			return nil, writeErr
		}
		current = &session{
			Conn:                conn,
			framed:              framed.NewReader(conn),
			sessionBufferSource: d.sessionBufferSource,
			streamBufferSource:  d.streamBufferSource,
			sessionBuffer:       d.sessionBufferSource.Get(),
			streams:             make(map[uint32]*stream),
		}
		go current.readLoop()
		d.current = current
	}
	id := d.id
	d.id += 1
	d.mx.Unlock()
	return current.getOrCreateStream(id), nil
}
