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
// takes about 8KB of memory.
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
	if current == nil {
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
			Conn:       conn,
			windowSize: d.windowSize,
			pool:       d.pool,
			out:        make(chan []byte),
			streams:    make(map[uint32]*stream),
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
