package connmux

import (
	"net"
	"sync"
)

// Dialer wraps the given dial function with support for multiplexing. The
// returned "streams" look and act just like regular net.Conns. The Dialer
// will multiplex everything over a single net.Conn until it encounters a read
// or write error on that Conn. At that point, it will dial a new conn for
// future streams, until there's a problem with that Conn, and so on and so
// forth.
//
// windowSize - how many frames to queue, used to bound memory use. Each frame
// takes about 8KB of memory. 25 is a good default.
//
// pool - BufferPool to use
func Dialer(windowSize int, pool BufferPool, dial func() (net.Conn, error)) func() (net.Conn, error) {
	d := &dialer{
		doDial:     dial,
		windowSize: windowSize,
		pool:       pool,
	}
	return d.dial
}

type dialer struct {
	doDial     func() (net.Conn, error)
	windowSize int
	pool       BufferPool
	current    *session
	id         uint32
	mx         sync.Mutex
}

func (d *dialer) dial() (net.Conn, error) {
	d.mx.Lock()
	current := d.current
	idsExhausted := false
	if d.id > maxID {
		log.Debug("Exhausted maximum allowed IDs on one physical connection, will open new connection")
		idsExhausted = true
		d.id = 0
	}

	// TODO: support pooling of connections (i.e. keep multiple physical connections in flight)
	if current == nil || idsExhausted {
		conn, err := d.doDial()
		if err != nil {
			d.mx.Unlock()
			return nil, err
		}
		sessionStart := make([]byte, sessionStartTotalLen)
		copy(sessionStart, sessionStartBytes)
		sessionStart[sessionStartHeaderLen] = protocolVersion1
		sessionStart[sessionStartHeaderLen+1] = byte(d.windowSize)
		_, writeErr := conn.Write(sessionStart)
		if writeErr != nil {
			conn.Close()
			return nil, writeErr
		}
		current = &session{
			Conn:        conn,
			windowSize:  d.windowSize,
			pool:        d.pool,
			out:         make(chan []byte),
			streams:     make(map[uint32]*stream),
			beforeClose: d.sessionClosed,
		}
		go current.writeLoop()
		go current.readLoop()
		d.current = current
	}
	id := d.id
	d.id++
	d.mx.Unlock()
	return current.getOrCreateStream(id), nil
}

func (d *dialer) sessionClosed(s *session) {
	d.mx.Lock()
	if d.current == s {
		// Clear current session since it's no longer usable
		d.current = nil
	}
	d.mx.Unlock()
}
