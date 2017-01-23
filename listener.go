package connmux

import (
	"io"
	"net"

	"github.com/oxtoacart/bpool"
)

type listener struct {
	wrapped    net.Listener
	frameDepth int
	pool       BufferPool
	errCh      chan error
	connCh     chan net.Conn
}

// WrapListener wraps the given listener with support for multiplexing. Only
// connections that start with the special session start header will be
// multiplexed, otherwise connections behave as normal. This means that a single
// listener can be used to serve clients that do multiplex and clients that
// don't.
//
// sessionBufferSource - a source of buffers for the session's read loop. A
// good width for these is 70,000 bytes.
//
// streamBufferSource - a source of buffers for each stream. These should be
// large enough to accomodate slow readers that may need to buffer a lot of
// data. If a buffer fills before the reader can drain it, the stream will fail
// with ErrBufferOverflowed.
func WrapListener(wrapped net.Listener, frameDepth int, poolSize int) net.Listener {
	l := &listener{
		wrapped:    wrapped,
		frameDepth: frameDepth,
		pool:       bpool.NewBytePool(poolSize, maxFrameLen),
		connCh:     make(chan net.Conn),
		errCh:      make(chan error),
	}
	go l.process()
	return l
}

func (l *listener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connCh:
		return conn, nil
	case err := <-l.errCh:
		return nil, err
	}
}

func (l *listener) Addr() net.Addr {
	return l.wrapped.Addr()
}

func (l *listener) Close() error {
	go func() {
		l.errCh <- ErrListenerClosed
	}()
	return l.wrapped.Close()
}

func (l *listener) process() {
	for {
		conn, err := l.wrapped.Accept()
		if err != nil {
			l.errCh <- err
			return
		}
		go l.onConn(conn)
	}
}

func (l *listener) onConn(conn net.Conn) {
	b := make([]byte, sessionStartLength)
	// Try to read start sequence
	n, err := io.ReadFull(conn, b)
	if err != nil {
		l.errCh <- err
		return
	}
	if n == sessionStartLength && string(b) == sessionStart {
		// It's a multiplexed connection
		s := &session{
			Conn:       conn,
			frameDepth: l.frameDepth,
			pool:       l.pool,
			out:        make(chan []byte),
			streams:    make(map[uint32]*stream),
			connCh:     l.connCh,
		}
		go s.readLoop()
		go s.writeLoop()
		return
	}

	// It's a normal connection
	l.connCh <- &preReadConn{conn, b}
}

// preReadConn is a conn that takes care of the fact that we've already read a
// little from it
type preReadConn struct {
	net.Conn
	buf []byte
}

func (prc *preReadConn) Read(b []byte) (int, error) {
	buffered := len(prc.buf)
	if buffered == 0 {
		return prc.Conn.Read(b)
	}
	needed := len(b)
	n := copy(b, prc.buf)
	var err error
	prc.buf = prc.buf[n:]
	remain := needed - n
	if remain > 0 {
		var n2 int
		n2, err = prc.Conn.Read(b[n:])
		n += n2
	}
	return n, err
}
